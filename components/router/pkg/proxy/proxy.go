// Copyright 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	listerv1 "k8s.io/client-go/listers/core/v1"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/controller"

	"github.com/alibaba/opensandbox/router/pkg/flag"
)

type Proxy struct {
	lister listerv1.PodLister
}

func NewProxy(ctx context.Context) *Proxy {
	proxy := &Proxy{}
	proxy.watchSandboxPods(ctx)

	return proxy
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			Logger.Errorw("Proxy: proxy causes panic", "error", err)
			var errMsg string
			if e, ok := err.(error); ok {
				errMsg = e.Error()
			} else {
				errMsg = fmt.Sprintf("%v", err)
			}
			http.Error(w, errMsg, http.StatusBadGateway)
		}
	}()

	// parse backend pod metadata from Header 'OPEN-SANDBOX-INGRESS'
	targetHost := r.Header.Get(SandboxIngress)
	if targetHost == "" {
		Logger.Warnw("Proxy: proxy target host from header 'OPEN-SANDBOX-INGRESS' is empty. Try parse from 'Host'", zap.String("host", r.Host))
		targetHost = r.Host
		if targetHost == "" {
			Logger.Errorw("Proxy: proxy target host is empty", zap.Any("request", *r))
			http.Error(w, "missing header 'OPEN-SANDBOX-INGRESS' or 'Host'", http.StatusBadRequest)
			return
		}
	}

	host, err := p.parseSandboxHost(targetHost)
	if err != nil || host.ingressKey == "" || host.port == "" {
		http.Error(w, fmt.Sprintf("Proxy: invalid host: %s", targetHost), http.StatusNotAcceptable)
		return
	}

	targetHost, err, code := p.fetchRealHost(host)
	if err != nil {
		if code == http.StatusNotFound {
			Logger.Warnw("Proxy: no pod found for ingress rule", "ingress", host.ingressKey)
		}
		http.Error(w, fmt.Sprintf("Proxy: %v", err), code)
		return
	}

	Logger.Infow("proxy requested", "target", targetHost, "client", p.getClientIP(r), "headers", r.Header, "uri", r.RequestURI, "method", r.Method)

	r.Host = targetHost
	r.URL.Host = targetHost
	r.Header.Del(SandboxIngress)

	p.serve(w, r)
}

func (p *Proxy) serve(w http.ResponseWriter, r *http.Request) {
	if p.isWebSocketRequest(r) {
		if r.URL == nil {
			http.Error(w, "invalid request URL", http.StatusBadRequest)
			return
		}

		if r.URL.Scheme == "" {
			if r.TLS != nil {
				r.URL.Scheme = "wss"
			} else {
				r.URL.Scheme = "ws"
			}
		}
		NewWebSocketProxy(r.URL).ServeHTTP(w, r)
	} else {
		if r.URL.Scheme == "" {
			if r.TLS != nil {
				r.URL.Scheme = "https"
			} else {
				r.URL.Scheme = "http"
			}
		}
		NewHTTPProxy().ServeHTTP(w, r)
	}
}

func (p *Proxy) isWebSocketRequest(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.Header.Get("Upgrade") != "websocket" {
		return false
	}
	if r.Header.Get("Connection") != "Upgrade" {
		return false
	}
	return true
}

type sandboxHost struct {
	ingressKey string
	port       string
}

func (p *Proxy) parseSandboxHost(s string) (sandboxHost, error) {
	domain := strings.Split(strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://"), ".")
	if len(domain) < 1 {
		return sandboxHost{}, fmt.Errorf("invalid host: %s", s)
	}

	ingressAndPort := strings.Split(domain[0], "-")
	if len(ingressAndPort) <= 1 || ingressAndPort[0] == "" {
		return sandboxHost{}, fmt.Errorf("invalid host: %s", s)
	}

	port := ingressAndPort[len(ingressAndPort)-1]
	ingress := strings.Join(ingressAndPort[:len(ingressAndPort)-1], "-")
	return sandboxHost{ingress, port}, nil
}

func (p *Proxy) fetchRealHost(host sandboxHost) (string, error, int) {
	pods, err := p.lister.List(labels.Set{
		flag.IngressLabelKey: host.ingressKey,
	}.AsSelector())
	if err != nil {
		return "", err, http.StatusBadGateway
	}

	switch {
	case len(pods) == 1 && pods[0].Status.PodIP != "":
		return pods[0].Status.PodIP + ":" + host.port, nil, 0
	case len(pods) > 1:
		return "", fmt.Errorf("multiple sandboxes found for host: %s", host.ingressKey), http.StatusConflict
	default:
		return "", fmt.Errorf("no sandboxes found for host: %s", host.ingressKey), http.StatusNotFound
	}
}

func (p *Proxy) getClientIP(r *http.Request) string {
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if len(r.Header.Get(XForwardedFor)) != 0 {
		xff := r.Header.Get(XForwardedFor)
		s := strings.Index(xff, ", ")
		if s == -1 {
			s = len(r.Header.Get(XForwardedFor))
		}
		clientIP = xff[:s]
	} else if len(r.Header.Get(XRealIP)) != 0 {
		clientIP = r.Header.Get(XRealIP)
	}

	return clientIP
}

func (p *Proxy) watchSandboxPods(ctx context.Context) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		kubeclient.Get(ctx),
		controller.GetResyncPeriod(ctx),
		informers.WithNamespace(flag.Namespace),
		informers.WithTweakListOptions(func(options *v1.ListOptions) {
			options.LabelSelector = flag.IngressLabelKey
		}),
	)

	podInformer := factory.Core().V1().Pods()
	p.lister = podInformer.Lister()

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
}
