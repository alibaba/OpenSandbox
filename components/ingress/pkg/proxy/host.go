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
		targetHost := r.Header.Get(SandboxIngress)
		if targetHost == "" {
			targetHost = r.Host
			if targetHost == "" {
				return nil, errors.New("missing header 'OPEN-SANDBOX-INGRESS' or 'Host'")
			}
		}

		host, err := p.parseSandboxHost(targetHost)
		if err != nil || host.ingressKey == "" || host.port == "" {
			return nil, fmt.Errorf("invalid host: %s", targetHost)
		}
		return host, nil
	case ModeURI:
		// TODO: implement ModeURI
		return nil, errors.New("mode URI is not implemented")
	}

	return nil, fmt.Errorf("unknown ingress mode: %s", p.mode)
}

type sandboxHost struct {
	ingressKey string
	port       string
	requestURI *string
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

	port := ingressAndPort[len(ingressAndPort)-1]
	ingress := strings.Join(ingressAndPort[:len(ingressAndPort)-1], "-")
	return &sandboxHost{ingress, port, nil}, nil
}
