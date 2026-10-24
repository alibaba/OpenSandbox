// Copyright 2026 The OpenSandbox Authors
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

package nftables

import (
	"context"
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/opensandbox/egress/pkg/policy"
	"github.com/stretchr/testify/require"
)

// TestPortScopedHostSurvivesDNSRebind reproduces a sandbox escape: the
// policy allows a public domain suffix, denies all private space except one
// host, and allows that host on a single port. Any name under the allowed
// suffix that resolves to the host lands in the dynamic allow set, which
// accepts every port, so the host's other listeners become reachable. A
// portRanges deny for the complement must close that path.
func TestPortScopedHostSurvivesDNSRebind(t *testing.T) {
	host := netip.MustParseAddr("10.0.0.5")
	const allowedPort = 8000

	rules := []map[string]any{{"action": "allow", "target": "*.org"}}
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		for _, rest := range excludeAddr(netip.MustParsePrefix(cidr), host) {
			rules = append(rules, map[string]any{"action": "deny", "target": rest.String()})
		}
	}
	rules = append(rules, map[string]any{"action": "allow", "target": host.String(), "ports": []int{allowedPort}})

	complement := []map[string]int{{"start": 1, "end": allowedPort - 1}, {"start": allowedPort + 1, "end": 65535}}
	withComplementDeny := append(append([]map[string]any(nil), rules...),
		map[string]any{"action": "deny", "target": host.String(), "portRanges": complement})

	cases := []struct {
		name  string
		rules []map[string]any
		want  map[netip.AddrPort]string
	}{
		{
			name:  "port allow alone is bypassed by DNS",
			rules: rules,
			want: map[netip.AddrPort]string{
				netip.AddrPortFrom(host, allowedPort): "accept",
				netip.AddrPortFrom(host, 6379):        "accept",
			},
		},
		{
			name:  "complement portRanges deny holds",
			rules: withComplementDeny,
			want: map[netip.AddrPort]string{
				netip.AddrPortFrom(host, allowedPort):                            "accept",
				netip.AddrPortFrom(host, 6379):                                   "drop",
				netip.AddrPortFrom(host, 10001):                                  "drop",
				netip.AddrPortFrom(host, 1):                                      "drop",
				netip.AddrPortFrom(host, 65535):                                  "drop",
				netip.AddrPortFrom(netip.MustParseAddr("10.0.0.6"), allowedPort): "drop",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"defaultAction": "deny", "egress": tc.rules})
			require.NoError(t, err)
			p, err := policy.ParsePolicy(string(raw))
			require.NoError(t, err)

			var scripts []string
			m := NewManagerWithRunner(func(_ context.Context, script string) ([]byte, error) {
				scripts = append(scripts, script)
				return nil, nil
			})
			ctx := context.Background()
			require.NoError(t, m.ApplyStatic(ctx, p))
			require.NoError(t, m.AddResolvedDomain(ctx, "malicious.org", []ResolvedIP{{Addr: host, TTL: time.Minute}}),
				"*.org must admit malicious.org so its answer reaches the dynamic allow set")

			chain := loadChain(t, scripts)
			require.True(t, chain.sets[dynAllowV4Set][netip.PrefixFrom(host, 32)], "the DNS answer must be in the dynamic allow set")
			for dst, verdict := range tc.want {
				require.Equalf(t, verdict, chain.verdictTCP(t, dst), "TCP to %s", dst)
			}
		})
	}
}

// excludeAddr returns prefixes covering p minus addr.
func excludeAddr(p netip.Prefix, addr netip.Addr) []netip.Prefix {
	if !p.Contains(addr) {
		return []netip.Prefix{p}
	}
	if p.Bits() == addr.BitLen() {
		return nil
	}
	lo := netip.PrefixFrom(p.Addr(), p.Bits()+1)
	hiAddr := p.Addr().As4()
	bit := p.Bits()
	hiAddr[bit/8] |= 0x80 >> (bit % 8)
	hi := netip.PrefixFrom(netip.AddrFrom4(hiAddr), p.Bits()+1)
	return append(excludeAddr(lo, addr), excludeAddr(hi, addr)...)
}

// testChain is a minimal model of the rendered egress chain for a new,
// unmarked, non-loopback IPv4 TCP connection.
type testChain struct {
	sets   map[string]map[netip.Prefix]bool
	rules  []string
	policy string
}

func loadChain(t *testing.T, scripts []string) *testChain {
	t.Helper()
	prefix := "inet " + tableName + " "
	c := &testChain{}
	for _, script := range scripts {
		for _, line := range strings.Split(script, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "", strings.HasPrefix(line, "add table "), strings.HasPrefix(line, "add set "):
			case strings.HasPrefix(line, "delete table "):
				c.sets, c.rules, c.policy = map[string]map[netip.Prefix]bool{}, nil, ""
			case strings.HasPrefix(line, "add chain "):
				if strings.Contains(line, "policy drop;") {
					c.policy = "drop"
				} else {
					c.policy = "accept"
				}
			case strings.HasPrefix(line, "add rule "+prefix+chainName+" "):
				c.rules = append(c.rules, strings.TrimPrefix(line, "add rule "+prefix+chainName+" "))
			case strings.HasPrefix(line, "add element "+prefix), strings.HasPrefix(line, "delete element "+prefix):
				add := strings.HasPrefix(line, "add ")
				rest := strings.TrimPrefix(strings.TrimPrefix(line, "add element "+prefix), "delete element "+prefix)
				name, body, _ := strings.Cut(rest, " ")
				if c.sets[name] == nil {
					c.sets[name] = map[netip.Prefix]bool{}
				}
				for _, elem := range splitBraces(t, body) {
					elem, _, _ = strings.Cut(elem, " timeout ")
					p, err := parseAsPrefix(elem)
					require.NoError(t, err)
					if add {
						c.sets[name][p] = true
					} else {
						delete(c.sets[name], p)
					}
				}
			default:
				t.Fatalf("test chain model does not understand %q", line)
			}
		}
	}
	return c
}

// verdictTCP walks the chain in order and returns the first terminal verdict.
func (c *testChain) verdictTCP(t *testing.T, dst netip.AddrPort) string {
	t.Helper()
	for _, rule := range c.rules {
		if verdict, ok := c.match(t, rule, dst); ok {
			return verdict
		}
	}
	return c.policy
}

func (c *testChain) match(t *testing.T, rule string, dst netip.AddrPort) (string, bool) {
	t.Helper()
	switch {
	case strings.HasPrefix(rule, "ct state "), strings.HasPrefix(rule, "meta mark "),
		strings.HasPrefix(rule, `oifname "lo" `), strings.HasPrefix(rule, "icmpv6 "),
		strings.HasPrefix(rule, "ip6 "), strings.HasPrefix(rule, "udp "):
		return "", false
	}
	rest := rule
	if after, ok := strings.CutPrefix(rest, "ip daddr "); ok {
		target, tail, _ := strings.Cut(after, " ")
		if name, isSet := strings.CutPrefix(target, "@"); isSet {
			if !c.setContains(name, dst.Addr()) {
				return "", false
			}
		} else {
			p, err := parseAsPrefix(target)
			require.NoError(t, err)
			if !p.Contains(dst.Addr()) {
				return "", false
			}
		}
		rest = tail
	}
	if strings.HasPrefix(rest, "udp dport ") {
		return "", false
	}
	if after, ok := strings.CutPrefix(rest, "tcp dport "); ok {
		var members []string
		if strings.HasPrefix(after, "{") {
			end := strings.Index(after, "}")
			require.GreaterOrEqual(t, end, 0, "unterminated dport set in %q", rule)
			members = splitBraces(t, after[:end+1])
			rest = strings.TrimSpace(after[end+1:])
		} else {
			port, tail, _ := strings.Cut(after, " ")
			members, rest = []string{port}, tail
		}
		if !portInMembers(t, members, int(dst.Port())) {
			return "", false
		}
	}
	switch rest {
	case "accept", "drop":
		return rest, true
	}
	t.Fatalf("test chain model does not understand rule %q", rule)
	return "", false
}

func (c *testChain) setContains(name string, addr netip.Addr) bool {
	for p := range c.sets[name] {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func splitBraces(t *testing.T, body string) []string {
	t.Helper()
	body = strings.TrimSpace(body)
	require.Truef(t, strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}"), "expected {...}, got %q", body)
	var out []string
	for _, elem := range strings.Split(strings.Trim(body, "{}"), ",") {
		if elem = strings.TrimSpace(elem); elem != "" {
			out = append(out, elem)
		}
	}
	return out
}

func portInMembers(t *testing.T, members []string, port int) bool {
	t.Helper()
	for _, member := range members {
		loStr, hiStr, isRange := strings.Cut(member, "-")
		lo, err := strconv.Atoi(loStr)
		require.NoError(t, err)
		hi := lo
		if isRange {
			hi, err = strconv.Atoi(hiStr)
			require.NoError(t, err)
		}
		if port >= lo && port <= hi {
			return true
		}
	}
	return false
}
