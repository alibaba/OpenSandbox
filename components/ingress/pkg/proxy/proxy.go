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
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/alibaba/opensandbox/ingress/pkg/sandbox"
	slogger "github.com/alibaba/opensandbox/internal/logger"
)

type Proxy struct {
	sandboxProvider sandbox.Provider
	mode            Mode
}

func NewProxy(_ context.Context, sandboxProvider sandbox.Provider, mode Mode) *Proxy {
	proxy := &Proxy{
		sandboxProvider: sandboxProvider,
		mode:            mode,
	}

	return proxy
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			Logger.With(slogger.Field{Key: "error", Value: err}).Errorf("Proxy: proxy causes panic")
			var errMsg string
			if e, ok := err.(error); ok {
				errMsg = e.Error()
			} else {
				errMsg = fmt.Sprintf("%v", err)
			}
			http.Error(w, errMsg, http.StatusBadGateway)
		}
	}()

	host, err := p.getSandboxHostDefinition(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("OpenSandbox Ingress: %v", err), http.StatusBadRequest)
		return
	}

	targetHost, err, code := p.resolveRealHost(host)
	if err != nil {
		http.Error(w, fmt.Sprintf("OpenSandbox Ingress: %v", err), code)
		return
	}

	// modify if requestURI is not empty
	if host.requestURI != "" {
		r.URL.Path = host.requestURI
	}

	r.Host = targetHost
	r.URL.Host = targetHost
	r.Header.Del(SandboxIngress)

	Logger.With(
		slogger.Field{Key: "target", Value: targetHost},
		slogger.Field{Key: "client", Value: p.getClientIP(r)},
		slogger.Field{Key: "uri", Value: r.RequestURI},
		slogger.Field{Key: "method", Value: r.Method},
	).Infof("ingress requested")
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

func (p *Proxy) resolveRealHost(host *sandboxHost) (string, error, int) {
	// Get endpoint IP from sandbox provider
	endpointIP, err := p.sandboxProvider.GetEndpoint(host.ingressKey)
	if err != nil {
		// Map sandbox errors to HTTP status codes
		switch {
		case errors.Is(err, sandbox.ErrSandboxNotFound):
			return "", err, http.StatusNotFound
		case errors.Is(err, sandbox.ErrSandboxNotReady):
			return "", err, http.StatusServiceUnavailable
		default:
			return "", err, http.StatusBadGateway
		}
	}

	// Construct target host with port
	targetHost := fmt.Sprintf("%s:%d", endpointIP, host.port)
	return targetHost, nil, 0
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
