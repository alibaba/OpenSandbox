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

package iptables

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/log"
)

// SetupRedirect installs OUTPUT nat redirect for DNS (udp/tcp 53 -> port).
//
// exemptDst: optional list of destination IP or CIDR; traffic to these is not redirected (direct forward).
// Packets carrying mark are also RETURNed (proxy's own upstream). Requires CAP_NET_ADMIN.
func SetupRedirect(port int, exemptDst []string) error {
	log.Infof("installing iptables DNS redirect: OUTPUT port 53 -> %d (mark %s bypass)", port, constants.MarkHex)
	targetPort := strconv.Itoa(port)

	var rules [][]string
	for _, d := range exemptDst {
		if d == "" {
			continue
		}
		// Exempt by destination: don't redirect DNS to these IPs/CIDRs. Use iptables for IPv4, ip6tables for IPv6 only.
		if addr, err := netip.ParseAddr(d); err == nil {
			if addr.Is4() {
				rules = append(rules,
					[]string{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-d", d, "-j", "RETURN"},
					[]string{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-d", d, "-j", "RETURN"},
				)
			} else {
				rules = append(rules,
					[]string{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-d", d, "-j", "RETURN"},
					[]string{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-d", d, "-j", "RETURN"},
				)
			}
			continue
		}
		if p, err := netip.ParsePrefix(d); err == nil {
			if p.Addr().Is4() {
				rules = append(rules,
					[]string{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-d", d, "-j", "RETURN"},
					[]string{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-d", d, "-j", "RETURN"},
				)
			} else {
				rules = append(rules,
					[]string{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-d", d, "-j", "RETURN"},
					[]string{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-d", d, "-j", "RETURN"},
				)
			}
		}
	}
	// Bypass packets marked by the proxy itself (see dnsproxy dialer).
	markAndRedirect := [][]string{
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-m", "mark", "--mark", constants.MarkHex, "-j", "RETURN"},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-m", "mark", "--mark", constants.MarkHex, "-j", "RETURN"},
		// Redirect all other DNS traffic to local proxy port.
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-port", targetPort},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-port", targetPort},
		// IPv6 equivalents (ip6tables)
		{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-m", "mark", "--mark", constants.MarkHex, "-j", "RETURN"},
		{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-m", "mark", "--mark", constants.MarkHex, "-j", "RETURN"},
		{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-port", targetPort},
		{"ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-port", targetPort},
	}
	rules = append(rules, markAndRedirect...)

	for _, args := range rules {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables command failed: %v (output: %s)", err, output)
		}
	}
	log.Infof("iptables DNS redirect installed successfully")
	return nil
}
