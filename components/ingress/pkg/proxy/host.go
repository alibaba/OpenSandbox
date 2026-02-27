// Copyright 2026 Alibaba Group Holding Ltd.
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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Mode string

const (
	// ModeHeader is the mode that uses the Host or SandboxIngress header
	// to determine the sandbox instance.
	ModeHeader Mode = "header"

	// ModeURI is the mode that uses the URI path to determine the
	// sandbox instance.
	//
	// Pattern is 'hostname/<sandbox-id>/<sandbox-port>/<path-to-request>'.
	ModeURI Mode = "uri"
)

func (p *Proxy) getSandboxHostDefinition(r *http.Request) (*sandboxHost, error) {
	switch p.mode {
	case ModeHeader:
		targetHost := p.parseTargetHostByHeader(r)
		if targetHost == "" {
			return nil, fmt.Errorf("missing header '%s' or 'Host'", SandboxIngress)
		}

		host, err := p.parseSandboxHost(targetHost)
		if err != nil || host.ingressKey == "" || host.port == 0 {
			return nil, fmt.Errorf("invalid host: %s", targetHost)
		}
		return host, nil
	case ModeURI:
		return p.parseSandboxURI(r)
	}

	return nil, fmt.Errorf("unknown ingress mode: %s", p.mode)
}

func (p *Proxy) parseTargetHostByHeader(r *http.Request) string {
	targetHost := r.Header.Get(SandboxIngress)
	if targetHost != "" {
		return targetHost
	}
	deprecatedTargetHost := r.Header.Get(DeprecatedSandboxIngress)
	if deprecatedTargetHost != "" {
		return deprecatedTargetHost
	}

	return r.Host
}

type sandboxHost struct {
	ingressKey string
	port       int
	requestURI string
}

func (p *Proxy) parseSandboxHost(s string) (*sandboxHost, error) {
	domain := strings.Split(strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://"), ".")
	if len(domain) < 1 {
		return &sandboxHost{}, fmt.Errorf("invalid host: %s", s)
	}

	ingressAndPort := strings.Split(domain[0], "-")
	if len(ingressAndPort) <= 1 || ingressAndPort[0] == "" {
		return &sandboxHost{}, fmt.Errorf("invalid host: %s", s)
	}

	ingress := strings.Join(ingressAndPort[:len(ingressAndPort)-1], "-")
	port, err := strconv.Atoi(ingressAndPort[len(ingressAndPort)-1])
	if err != nil {
		return &sandboxHost{}, fmt.Errorf("invalid port format: %w", err)
	}
	return &sandboxHost{ingress, port, ""}, nil
}

func (p *Proxy) parseSandboxURI(r *http.Request) (*sandboxHost, error) {
	path := r.URL.Path
	if path == "" {
		return nil, errors.New("missing URI path")
	}

	// Remove leading slash and split by '/'
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid URI path format: expected '/<sandbox-id>/<sandbox-port>/<path-to-request>', got: %s", r.URL.Path)
	}

	sandboxID := parts[0]
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid port format: %w", err)
	}
	if sandboxID == "" || port <= 0 {
		return nil, errors.New("missing sandbox-id or sandbox-port in URI path")
	}

	// Extract the remaining path (user's target request URI)
	var requestURI string
	if len(parts) >= 3 && parts[2] != "" {
		requestURI = "/" + parts[2]
	} else {
		requestURI = "/"
	}

	return &sandboxHost{
		ingressKey: sandboxID,
		port:       port,
		requestURI: requestURI,
	}, nil
}
