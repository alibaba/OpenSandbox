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
	"net/http"
	"net/http/httputil"
	"net/url"
)

type HTTPProxy struct{}

func NewHTTPProxy() *HTTPProxy {
	return &HTTPProxy{}
}

func (hp *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.String()

	proxy, err := hp.newReverseProxy(targetURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	proxy.ServeHTTP(w, r)
}

func (hp *HTTPProxy) newReverseProxy(targetHost string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = url.Scheme
		req.URL.Host = url.Host
		req.Host = url.Host
		req.Header.Del(SandboxIngress)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		response.Header.Add(ReverseProxyServerPowerBy, "opensandbox-router")
		return nil
	}
	return proxy, nil
}
