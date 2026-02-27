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

package main

import (
	"net/netip"
	"os"
	"strconv"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/dnsproxy"
	"github.com/alibaba/opensandbox/egress/pkg/log"
)

// AllowIPsForNft returns the list of IPs to merge into the nft allow set for DNS in dns+nft mode:
// 127.0.0.1 (proxy listen / iptables redirect target) plus validated, capped nameserver IPs from resolvPath.
// Validation: skips unspecified (0.0.0.0, ::) and loopback (127.x, ::1).
// Cap: at most max nameservers (default 3; set EGRESS_MAX_NAMESERVERS=0 for no cap, or 1–10).
func AllowIPsForNft(resolvPath string) []netip.Addr {
	raw, _ := dnsproxy.ResolvNameserverIPs(resolvPath)
	maxNsCount := maxNameserversFromEnv()

	var validated []netip.Addr
	for _, ip := range raw {
		if maxNsCount > 0 && len(validated) >= maxNsCount {
			break
		}
		if !isValidNameserverIP(ip) {
			continue
		}
		validated = append(validated, ip)
	}

	// 127.0.0.1 first so packets redirected to proxy are accepted by nft.
	out := make([]netip.Addr, 0, 1+len(validated))
	out = append(out, netip.MustParseAddr("127.0.0.1"))
	out = append(out, validated...)

	if len(out) > 1 {
		log.Infof("[dns] whitelisting proxy listen + %d nameserver(s) for nft: %v", len(validated), formatIPs(out))
	} else {
		log.Infof("[dns] whitelisting proxy listen (127.0.0.1); no valid nameserver IPs from %s", resolvPath)
	}
	return out
}

func maxNameserversFromEnv() int {
	s := os.Getenv(constants.EnvMaxNameservers)
	if s == "" {
		return constants.DefaultMaxNameservers
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return constants.DefaultMaxNameservers
	}
	if n > 10 {
		return 10
	}
	// 0 = no cap
	return n
}

func isValidNameserverIP(ip netip.Addr) bool {
	if ip.IsUnspecified() {
		return false
	}
	if ip.IsLoopback() {
		return false
	}
	return true
}

func formatIPs(ips []netip.Addr) []string {
	out := make([]string, len(ips))
	for i, ip := range ips {
		out[i] = ip.String()
	}
	return out
}
