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

package policy

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"strings"
)

const (
	ActionAllow = "allow"
	ActionDeny  = "deny"
)

type targetKind int

const (
	targetUnknown targetKind = iota
	targetDomain
	targetIP
	targetCIDR
	// targetPortOnly: no Target, scopes by TCP port(s) across all destinations.
	targetPortOnly
)

// PortRange is an inclusive TCP port interval.
type PortRange struct {
	Lo int `json:"start"`
	Hi int `json:"end"`
}

// Contains reports whether port falls inside the range.
func (r PortRange) Contains(port int) bool {
	return port >= r.Lo && port <= r.Hi
}

// covers reports whether every port in o is inside r.
func (r PortRange) covers(o PortRange) bool {
	return o.Lo >= r.Lo && o.Hi <= r.Hi
}

// DefaultDenyPolicy is deny-by-default with an empty egress list.
func DefaultDenyPolicy() *NetworkPolicy {
	return &NetworkPolicy{
		DefaultAction: ActionDeny,
		domainIndex:   compileDomainIndex(nil),
	}
}

// NetworkPolicy: JSON defaultAction + egress; domain rules use first-match (see compiled index).
type NetworkPolicy struct {
	Egress        []EgressRule `json:"egress"`
	DefaultAction string       `json:"defaultAction"`

	domainIndex *compiledDomainIndex
}

type EgressRule struct {
	Action string `json:"action"`
	Target string `json:"target"`
	// Ports restricts the rule to TCP destination ports (1-65535); empty
	// with an empty Target means all destinations. Domain targets with
	// Ports are rejected by normalizePolicy.
	Ports []int `json:"ports,omitempty"`
	// PortRanges is the interval form of Ports; the two must not overlap.
	PortRanges []PortRange `json:"portRanges,omitempty"`
	// FloorPortRanges is PortRanges set only by the platform
	// port floor (never serialized: the always overlay is re-read from file
	// on every reload).
	FloorPortRanges []PortRange `json:"-"`

	targetKind targetKind
	ip         netip.Addr
	prefix     netip.Prefix
}

// ParsePolicy unmarshals JSON; empty/null/{} → default deny. defaultAction defaults to deny if unset in JSON.
func ParsePolicy(raw string) (*NetworkPolicy, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return DefaultDenyPolicy(), nil
	}

	var p NetworkPolicy
	if err := json.Unmarshal([]byte(trimmed), &p); err != nil {
		return nil, err
	}
	if err := normalizePolicy(&p); err != nil {
		return nil, err
	}
	return ensureDefaults(&p), nil
}

// Evaluate returns allow or deny for a query name (FQDN with or without trailing dot, lowercased).
func (p *NetworkPolicy) Evaluate(domain string) string {
	if p == nil {
		return ActionDeny
	}
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	if p.domainIndex != nil {
		if action, ok := p.domainIndex.match(domain); ok {
			if action == "" {
				return ActionDeny
			}
			return action
		}
	} else {
		// Keep compatibility for policies built manually without ParsePolicy/ensureDefaults.
		if action, ok := p.evaluateLinear(domain); ok {
			return action
		}
	}
	if p.DefaultAction == "" {
		return ActionDeny
	}
	return p.DefaultAction
}

func (p *NetworkPolicy) evaluateLinear(domain string) (string, bool) {
	for _, r := range p.Egress {
		if r.targetKind != targetDomain {
			continue
		}
		if r.matchesDomain(domain) {
			if r.Action == "" {
				return ActionDeny, true
			}
			return r.Action, true
		}
	}
	return "", false
}

func ensureDefaults(p *NetworkPolicy) *NetworkPolicy {
	if p == nil {
		return DefaultDenyPolicy()
	}
	if p.DefaultAction == "" {
		p.DefaultAction = ActionDeny
	}
	p.domainIndex = compileDomainIndex(p.Egress)
	return p
}

func normalizePolicy(p *NetworkPolicy) error {
	p.DefaultAction = strings.ToLower(strings.TrimSpace(p.DefaultAction))
	if p.DefaultAction == "" {
		p.DefaultAction = ActionDeny
	}

	for i := range p.Egress {
		r := &p.Egress[i]
		r.Action = strings.ToLower(strings.TrimSpace(r.Action))
		if r.Action == "" {
			r.Action = ActionDeny
		}
		if r.Action != ActionAllow && r.Action != ActionDeny {
			return fmt.Errorf("unsupported action %q", r.Action)
		}

		r.Target = strings.TrimSpace(r.Target)
		if err := normalizePorts(r.Ports, r.PortRanges); err != nil {
			return fmt.Errorf("egress target %q: %w", r.Target, err)
		}
		portScoped := len(r.Ports) > 0 || len(r.PortRanges) > 0

		if r.Target == "" {
			if !portScoped {
				return fmt.Errorf("egress target cannot be empty")
			}
			// No target: this rule scopes by port alone, across all destinations.
			r.targetKind = targetPortOnly
			continue
		}
		if ip, err := netip.ParseAddr(r.Target); err == nil {
			r.targetKind = targetIP
			r.ip = ip
			continue
		}
		if prefix, err := netip.ParsePrefix(r.Target); err == nil {
			r.targetKind = targetCIDR
			r.prefix = prefix
			continue
		}
		r.targetKind = targetDomain
		if portScoped {
			return fmt.Errorf("egress target %q: ports are not supported for domain targets yet", r.Target)
		}
	}
	return nil
}

// maxPortsPerRule bounds one rule's inline nft dport set size.
const maxPortsPerRule = 256

// normalizePorts validates a rule's Ports and PortRanges in place: each value must be a
// valid TCP port, the combined list must not exceed maxPortsPerRule, and it must not
// cover a port twice. An empty list is valid (the rule is simply not
// port-scoped).
func normalizePorts(ports []int, ranges []PortRange) error {
	if len(ports) == 0 && len(ranges) == 0 {
		return nil
	}
	if n := len(ports) + len(ranges); n > maxPortsPerRule {
		return fmt.Errorf("too many ports (%d), max %d per rule", n, maxPortsPerRule)
	}
	seen := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("port %d out of range 1-65535", port)
		}
		if _, dup := seen[port]; dup {
			return fmt.Errorf("duplicate port %d", port)
		}
		seen[port] = struct{}{}
	}
	for i, rg := range ranges {
		if rg.Lo < 1 || rg.Hi > 65535 || rg.Lo > rg.Hi {
			return fmt.Errorf("port range %d-%d out of 1-65535 or inverted", rg.Lo, rg.Hi)
		}
		for _, port := range ports {
			if rg.Contains(port) {
				return fmt.Errorf("port %d overlaps port range %d-%d", port, rg.Lo, rg.Hi)
			}
		}
		for _, prev := range ranges[:i] {
			if rg.Lo <= prev.Hi && prev.Lo <= rg.Hi {
				return fmt.Errorf("port range %d-%d overlaps port range %d-%d", rg.Lo, rg.Hi, prev.Lo, prev.Hi)
			}
		}
	}
	return nil
}

// WithExtraAllowIPs appends per-IP allow rules (e.g. resolv nameservers, explicit upstream) so client and
// proxy can reach the same address; does not change domain-mode egress rules.
func (p *NetworkPolicy) WithExtraAllowIPs(ips []netip.Addr) *NetworkPolicy {
	if p == nil || len(ips) == 0 {
		return p
	}
	out := *p
	n, m := len(p.Egress), len(ips)
	if m > math.MaxInt-n {
		panic("policy: egress rule slice capacity overflow")
	}
	out.Egress = make([]EgressRule, n, n+m)
	copy(out.Egress, p.Egress)
	for _, ip := range ips {
		out.Egress = append(out.Egress, EgressRule{
			Action:     ActionAllow,
			Target:     ip.String(),
			targetKind: targetIP,
			ip:         ip,
		})
	}
	return &out
}

// StaticIPSets buckets static IP/CIDR egress into allow/deny v4/v6 for nft element generation.
func (p *NetworkPolicy) StaticIPSets() (allowV4, allowV6, denyV4, denyV6 []string) {
	if p == nil {
		return
	}
	for _, r := range p.Egress {
		if r.portScoped() {
			// Port-scoped IP/CIDR rules are enforced separately via
			// PortScopedRules; including them here would additionally
			// allow/deny the target on every port, defeating the scoping.
			continue
		}
		switch r.targetKind {
		case targetIP:
			addr := r.ip
			target := addr.String()
			if r.Action == ActionAllow {
				if addr.Is4() {
					allowV4 = append(allowV4, target)
				} else if addr.Is6() {
					allowV6 = append(allowV6, target)
				}
			} else {
				if addr.Is4() {
					denyV4 = append(denyV4, target)
				} else if addr.Is6() {
					denyV6 = append(denyV6, target)
				}
			}
		case targetCIDR:
			pfx := r.prefix
			target := pfx.String()
			if r.Action == ActionAllow {
				if pfx.Addr().Is4() {
					allowV4 = append(allowV4, target)
				} else if pfx.Addr().Is6() {
					allowV6 = append(allowV6, target)
				}
			} else {
				if pfx.Addr().Is4() {
					denyV4 = append(denyV4, target)
				} else if pfx.Addr().Is6() {
					denyV6 = append(denyV6, target)
				}
			}
		default:
			continue
		}
	}
	return
}

// PortScopedRule is an egress rule constrained to specific TCP destination
// ports, optionally further constrained to a single IP or CIDR target.
type PortScopedRule struct {
	Action string
	// Target is "" (all IPv4/IPv6 destinations), an IP, or a CIDR.
	Target string
	IsV6   bool
	Ports  []int
	// Ranges is the interval form of Ports.
	Ranges []PortRange
}

// portScoped reports whether the rule is constrained by destination port.
func (r *EgressRule) portScoped() bool {
	return len(r.Ports) > 0 || len(r.PortRanges) > 0 || len(r.FloorPortRanges) > 0
}

// IsPortFloor reports whether the rule is a platform port-floor rule.
func (r *EgressRule) IsPortFloor() bool {
	return len(r.FloorPortRanges) > 0
}

// PortScopedRules returns the policy's port-constrained rules split into deny
// and allow buckets, each preserving declaration order. Callers generating
// nftables rules must apply the deny bucket before both the plain (portless)
// deny set and the allow bucket, so a deny rule always wins over any allow
// rule regardless of how each is scoped — the same fail-closed precedence
// StaticIPSets already gives to plain IP/CIDR rules.
func (p *NetworkPolicy) PortScopedRules() (deny, allow []PortScopedRule) {
	if p == nil {
		return nil, nil
	}
	for _, r := range p.Egress {
		if !r.portScoped() {
			continue
		}
		var ranges []PortRange
		if n := len(r.PortRanges) + len(r.FloorPortRanges); n > 0 {
			ranges = make([]PortRange, 0, n)
			ranges = append(ranges, r.PortRanges...)
			ranges = append(ranges, r.FloorPortRanges...)
		}
		rule := PortScopedRule{Action: r.Action, Ports: r.Ports, Ranges: ranges}
		switch r.targetKind {
		case targetIP:
			rule.Target = r.ip.String()
			rule.IsV6 = r.ip.Is6()
		case targetCIDR:
			rule.Target = r.prefix.String()
			rule.IsV6 = r.prefix.Addr().Is6()
		case targetPortOnly:
			// rule.Target stays "": applies to all destinations.
		default:
			// Domain targets never carry ports; normalizePolicy rejects that
			// combination before a policy reaches this point.
			continue
		}
		if r.Action == ActionAllow {
			allow = append(allow, rule)
		} else {
			deny = append(deny, rule)
		}
	}
	return deny, allow
}

func (r *EgressRule) matchesDomain(domain string) bool {
	pattern := strings.ToLower(strings.TrimSpace(r.Target))
	domain = strings.ToLower(domain)

	if pattern == "" {
		return false
	}
	if pattern == domain {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		// "*.example.com" matches "a.example.com" but not "example.com"
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(domain, suffix) && domain != strings.TrimPrefix(pattern, "*.")
	}
	return false
}
