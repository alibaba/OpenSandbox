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
