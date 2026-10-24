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
	"net/netip"
	"sort"

	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

// EstablishedPortDrop is a destination:port teardown target for flows that
// were allowed only on specific TCP ports.
type EstablishedPortDrop struct {
	Addr   netip.Addr
	Prefix netip.Prefix
	Ports  []int
	Ranges []policy.PortRange
}

// EstablishedDrops lists destinations that a policy change no longer allows.
// ApplyStaticDropEstablished installs drop rules for these destinations ahead
// of the usual `ct state established,related accept`, so keep-alive flows to
// removed allows die on the next packet instead of surviving on conntrack.
//
// When CatchAll is set (default-action tighten allow→deny), keep rules for
// KeepAddrs/KeepPrefixes/PortDrops-as-keeps are emitted first, then a blanket
// established drop replaces the generic accept.
type EstablishedDrops struct {
	Addrs        []netip.Addr
	Prefixes     []netip.Prefix
	PortDrops    []EstablishedPortDrop
	CatchAll     bool
	KeepAddrs    []netip.Addr
	KeepPrefixes []netip.Prefix
	KeepPorts    []EstablishedPortDrop
}

// Empty reports whether there is nothing to drop.
func (d EstablishedDrops) Empty() bool {
	return len(d.Addrs) == 0 && len(d.Prefixes) == 0 && len(d.PortDrops) == 0 && !d.CatchAll
}

// Matches reports whether addr is covered by this drop set (port-agnostic).
// Port-scoped drops are ignored here; callers that know a dport should use
// MatchesFlow.
func (d EstablishedDrops) Matches(addr netip.Addr) bool {
	return d.MatchesFlow(addr, 0)
}

// MatchesFlow reports whether an ORIGINAL-direction TCP flow to addr[:dport]
// should be torn down. dport 0 means "any/unknown port" and only matches
// portless address/prefix/catch-all targets.
func (d EstablishedDrops) MatchesFlow(addr netip.Addr, dport int) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if d.isKept(addr, dport) {
		return false
	}
	if d.CatchAll {
		return true
	}
	for _, a := range d.Addrs {
		if a == addr {
			return true
		}
	}
	for _, p := range d.Prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	if dport > 0 {
		for _, pd := range d.PortDrops {
			if !portDropCoversAddr(pd, addr) {
				continue
			}
			if portDropCoversPort(pd, dport) {
				return true
			}
		}
	}
	return false
}

func (d EstablishedDrops) isKept(addr netip.Addr, dport int) bool {
	for _, a := range d.KeepAddrs {
		if a == addr {
			return true
		}
	}
	for _, p := range d.KeepPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	if dport > 0 {
		for _, pd := range d.KeepPorts {
			if portDropCoversAddr(pd, addr) && portDropCoversPort(pd, dport) {
				return true
			}
		}
	}
	return false
}

func portDropCoversAddr(pd EstablishedPortDrop, addr netip.Addr) bool {
	if pd.Addr.IsValid() {
		return pd.Addr == addr
	}
	return pd.Prefix.IsValid() && pd.Prefix.Contains(addr)
}

func portDropCoversPort(pd EstablishedPortDrop, dport int) bool {
	for _, p := range pd.Ports {
		if p == dport {
			return true
		}
	}
	for _, r := range pd.Ranges {
		if r.Contains(dport) {
			return true
		}
	}
	return false
}

// SnapshotResolvedDomains returns a copy of the DNS-learned address map the
// manager currently tracks. Safe to call without holding an external lock.
func (m *Manager) SnapshotResolvedDomains() map[string][]netip.Addr {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotResolvedDomainsLocked()
}

func (m *Manager) snapshotResolvedDomainsLocked() map[string][]netip.Addr {
	out := make(map[string][]netip.Addr, len(m.domains))
	for domain, entry := range m.domains {
		if entry == nil || len(entry.addresses) == 0 {
			continue
		}
		addrs := make([]netip.Addr, 0, len(entry.addresses))
		for addr := range entry.addresses {
			addrs = append(addrs, addr)
		}
		sort.Slice(addrs, func(i, j int) bool { return addrs[i].Compare(addrs[j]) < 0 })
		out[domain] = addrs
	}
	return out
}

// ComputeEstablishedDrops derives the destinations a policy change removed.
// priorDomains is the pre-apply DNS-learned map (from SnapshotResolvedDomains).
//
// A destination is dropped when the new policy no longer allows it via a
// surviving domain lease or a portless IP/CIDR allow (EvaluateIP). Removed
// static allow CIDRs are retained as the *subtracted* prefixes so still-allowed
// subnets are not torn down. Port-scoped allows contribute PortDrops.
func ComputeEstablishedDrops(oldPol, newPol *policy.NetworkPolicy, priorDomains map[string][]netip.Addr) EstablishedDrops {
	if newPol == nil {
		newPol = policy.DefaultDenyPolicy()
	}

	survivingDomainIPs := make(map[netip.Addr]struct{})
	var addrs []netip.Addr
	seenAddr := make(map[netip.Addr]struct{})
	addAddr := func(addr netip.Addr) {
		if !addr.IsValid() {
			return
		}
		addr = addr.Unmap()
		if _, ok := seenAddr[addr]; ok {
			return
		}
		seenAddr[addr] = struct{}{}
		addrs = append(addrs, addr)
	}

	// Pass 1: collect IPs still covered by a surviving domain lease.
	for domain, ips := range priorDomains {
		if newPol.Evaluate(domain) != policy.ActionAllow {
			continue
		}
		for _, ip := range ips {
			if ip.IsValid() {
				survivingDomainIPs[ip.Unmap()] = struct{}{}
			}
		}
	}

	// Pass 2: removed-domain candidates, excluding shared CDN IPs that a
	// surviving lease or an explicit IP/CIDR allow still permits.
	for domain, ips := range priorDomains {
		if newPol.Evaluate(domain) == policy.ActionAllow {
			continue
		}
		for _, ip := range ips {
			if !ip.IsValid() {
				continue
			}
			ip = ip.Unmap()
			if _, keep := survivingDomainIPs[ip]; keep {
				continue
			}
			if newPol.EvaluateIP(ip) == policy.ActionAllow {
				continue
			}
			addAddr(ip)
		}
	}

	var prefixes []netip.Prefix
	seenPrefix := make(map[string]struct{})
	addPrefix := func(pfx netip.Prefix) {
		if !pfx.IsValid() {
			return
		}
		key := pfx.Masked().String()
		if _, ok := seenPrefix[key]; ok {
			return
		}
		seenPrefix[key] = struct{}{}
		prefixes = append(prefixes, pfx.Masked())
	}

	var portDrops []EstablishedPortDrop

	if oldPol != nil {
		for _, r := range oldPol.Egress {
			if r.Action != policy.ActionAllow {
				continue
			}
			hasPorts := len(r.Ports) > 0 || len(r.PortRanges) > 0
			switch {
			case hasPorts && r.Target != "" && looksLikeIP(r):
				ip := parseRuleIP(r)
				if !ip.IsValid() {
					continue
				}
				ports, ranges := portsNoLongerAllowed(newPol, ip, r.Ports, r.PortRanges)
				if len(ports) == 0 && len(ranges) == 0 {
					continue
				}
				portDrops = append(portDrops, EstablishedPortDrop{Addr: ip, Ports: ports, Ranges: ranges})
			case hasPorts && r.Target != "" && looksLikeCIDR(r):
				pfx := parseRulePrefix(r)
				if !pfx.IsValid() {
					continue
				}
				// Port+CIDR: if the whole prefix is still portlessly allowed, skip;
				// otherwise emit the old ports against the prefix (conservative).
				if prefixFullyAllowedPortless(newPol, pfx) {
					continue
				}
				if portsFullyAllowedOnPrefix(newPol, pfx, r.Ports, r.PortRanges) {
					continue
				}
				portDrops = append(portDrops, EstablishedPortDrop{
					Prefix: pfx.Masked(),
					Ports:  append([]int(nil), r.Ports...),
					Ranges: append([]policy.PortRange(nil), r.PortRanges...),
				})
			case hasPorts:
				// Port-only (no target) or domain+ports (rejected by normalize): skip.
				continue
			case r.Target != "" && looksLikeIP(r):
				ip := parseRuleIP(r)
				if ip.IsValid() && newPol.EvaluateIP(ip) != policy.ActionAllow {
					if _, keep := survivingDomainIPs[ip.Unmap()]; !keep {
						addAddr(ip)
					}
				}
			case r.Target != "" && looksLikeCIDR(r):
				pfx := parseRulePrefix(r)
				if !pfx.IsValid() {
					continue
				}
				for _, removed := range removedPrefixParts(newPol, pfx) {
					addPrefix(removed)
				}
			}
		}
	}

	// New deny rules for literal IP/CIDR must also cut established flows even
	// when the previous policy was default-allow with no explicit allow entry.
	for _, r := range newPol.Egress {
		if r.Action != policy.ActionDeny {
			continue
		}
		hasPorts := len(r.Ports) > 0 || len(r.PortRanges) > 0
		if hasPorts {
			if looksLikeIP(r) {
				ip := parseRuleIP(r)
				if ip.IsValid() {
					portDrops = append(portDrops, EstablishedPortDrop{
						Addr:   ip,
						Ports:  append([]int(nil), r.Ports...),
						Ranges: append([]policy.PortRange(nil), r.PortRanges...),
					})
				}
			} else if looksLikeCIDR(r) {
				pfx := parseRulePrefix(r)
				if pfx.IsValid() {
					portDrops = append(portDrops, EstablishedPortDrop{
						Prefix: pfx.Masked(),
						Ports:  append([]int(nil), r.Ports...),
						Ranges: append([]policy.PortRange(nil), r.PortRanges...),
					})
				}
			}
			continue
		}
		if looksLikeIP(r) {
			addAddr(parseRuleIP(r))
			continue
		}
		if looksLikeCIDR(r) {
			addPrefix(parseRulePrefix(r))
		}
	}

	var keepAddrs []netip.Addr
	var keepPrefixes []netip.Prefix
	var keepPorts []EstablishedPortDrop
	catchAll := oldPol != nil &&
		oldPol.DefaultAction == policy.ActionAllow &&
		newPol.DefaultAction != policy.ActionAllow

	if catchAll {
		seenKeep := make(map[netip.Addr]struct{})
		for ip := range survivingDomainIPs {
			if _, ok := seenKeep[ip]; ok {
				continue
			}
			seenKeep[ip] = struct{}{}
			keepAddrs = append(keepAddrs, ip)
		}
		seenKeepPfx := make(map[string]struct{})
		for _, r := range newPol.Egress {
			if r.Action != policy.ActionAllow {
				continue
			}
			hasPorts := len(r.Ports) > 0 || len(r.PortRanges) > 0
			if hasPorts {
				if looksLikeIP(r) {
					ip := parseRuleIP(r)
					if ip.IsValid() {
						keepPorts = append(keepPorts, EstablishedPortDrop{
							Addr:   ip,
							Ports:  append([]int(nil), r.Ports...),
							Ranges: append([]policy.PortRange(nil), r.PortRanges...),
						})
					}
				} else if looksLikeCIDR(r) {
					pfx := parseRulePrefix(r)
					if pfx.IsValid() {
						keepPorts = append(keepPorts, EstablishedPortDrop{
							Prefix: pfx.Masked(),
							Ports:  append([]int(nil), r.Ports...),
							Ranges: append([]policy.PortRange(nil), r.PortRanges...),
						})
					}
				}
				continue
			}
			if looksLikeIP(r) {
				ip := parseRuleIP(r)
				if !ip.IsValid() {
					continue
				}
				if _, ok := seenKeep[ip]; ok {
					continue
				}
				seenKeep[ip] = struct{}{}
				keepAddrs = append(keepAddrs, ip)
			} else if looksLikeCIDR(r) {
				pfx := parseRulePrefix(r)
				if !pfx.IsValid() {
					continue
				}
				key := pfx.Masked().String()
				if _, ok := seenKeepPfx[key]; ok {
					continue
				}
				seenKeepPfx[key] = struct{}{}
				keepPrefixes = append(keepPrefixes, pfx.Masked())
			}
		}
		sort.Slice(keepAddrs, func(i, j int) bool { return keepAddrs[i].Compare(keepAddrs[j]) < 0 })
		sort.Slice(keepPrefixes, func(i, j int) bool { return keepPrefixes[i].String() < keepPrefixes[j].String() })
	}

	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Compare(addrs[j]) < 0 })
	sort.Slice(prefixes, func(i, j int) bool { return prefixes[i].String() < prefixes[j].String() })
	return EstablishedDrops{
		Addrs:        addrs,
		Prefixes:     prefixes,
		PortDrops:    portDrops,
		CatchAll:     catchAll,
		KeepAddrs:    keepAddrs,
		KeepPrefixes: keepPrefixes,
		KeepPorts:    keepPorts,
	}
}

func looksLikeIP(r policy.EgressRule) bool {
	_, err := netip.ParseAddr(r.Target)
	return err == nil
}

func looksLikeCIDR(r policy.EgressRule) bool {
	_, err := netip.ParsePrefix(r.Target)
	return err == nil
}

func parseRuleIP(r policy.EgressRule) netip.Addr {
	ip, err := netip.ParseAddr(r.Target)
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap()
}

func parseRulePrefix(r policy.EgressRule) netip.Prefix {
	pfx, err := netip.ParsePrefix(r.Target)
	if err != nil {
		return netip.Prefix{}
	}
	return pfx
}

// removedPrefixParts returns the portions of oldPfx that newPol no longer
// allows portlessly. When a retained allow covers only a subnet, the
// complementary prefixes are returned instead of the whole old range.
func removedPrefixParts(newPol *policy.NetworkPolicy, oldPfx netip.Prefix) []netip.Prefix {
	if !oldPfx.IsValid() {
		return nil
	}
	oldPfx = oldPfx.Masked()
	if prefixFullyAllowedPortless(newPol, oldPfx) {
		return nil
	}
	keepers := overlappingPortlessAllows(newPol, oldPfx)
	if len(keepers) == 0 {
		return []netip.Prefix{oldPfx}
	}
	return prefixDiff(oldPfx, keepers)
}

func prefixFullyAllowedPortless(newPol *policy.NetworkPolicy, pfx netip.Prefix) bool {
	if newPol == nil || !pfx.IsValid() {
		return false
	}
	if newPol.EvaluateIP(pfx.Addr()) != policy.ActionAllow {
		return false
	}
	last := lastAddr(pfx)
	if last.IsValid() && last != pfx.Addr() && newPol.EvaluateIP(last) != policy.ActionAllow {
		return false
	}
	return true
}

func overlappingPortlessAllows(newPol *policy.NetworkPolicy, oldPfx netip.Prefix) []netip.Prefix {
	if newPol == nil {
		return nil
	}
	var out []netip.Prefix
	for _, r := range newPol.Egress {
		if r.Action != policy.ActionAllow || len(r.Ports) > 0 || len(r.PortRanges) > 0 {
			continue
		}
		if looksLikeIP(r) {
			ip := parseRuleIP(r)
			if ip.IsValid() && oldPfx.Contains(ip) {
				bits := 32
				if ip.Is6() {
					bits = 128
				}
				p, err := ip.Prefix(bits)
				if err == nil {
					out = append(out, p)
				}
			}
			continue
		}
		if looksLikeCIDR(r) {
			pfx := parseRulePrefix(r).Masked()
			if pfx.IsValid() && prefixesOverlap(oldPfx, pfx) {
				out = append(out, pfx)
			}
		}
	}
	return out
}

func prefixesOverlap(a, b netip.Prefix) bool {
	if !a.IsValid() || !b.IsValid() {
		return false
	}
	return a.Overlaps(b)
}

// prefixDiff returns the parts of whole not covered by any keeper.
func prefixDiff(whole netip.Prefix, keepers []netip.Prefix) []netip.Prefix {
	remaining := []netip.Prefix{whole.Masked()}
	for _, k := range keepers {
		if !k.IsValid() {
			continue
		}
		k = k.Masked()
		var next []netip.Prefix
		for _, r := range remaining {
			next = append(next, excludePrefix(r, k)...)
		}
		remaining = next
		if len(remaining) == 0 {
			break
		}
	}
	return remaining
}

// excludePrefix returns the parts of whole that are not inside exclude.
func excludePrefix(whole, exclude netip.Prefix) []netip.Prefix {
	if !whole.IsValid() {
		return nil
	}
	whole = whole.Masked()
	if !exclude.IsValid() || !whole.Overlaps(exclude) {
		return []netip.Prefix{whole}
	}
	exclude = exclude.Masked()
	// exclude covers whole entirely.
	if exclude.Bits() <= whole.Bits() && exclude.Contains(whole.Addr()) {
		return nil
	}
	// whole is more specific than exclude and contained → fully excluded.
	if whole.Bits() >= exclude.Bits() && exclude.Contains(whole.Addr()) {
		return nil
	}
	// Bisect whole and recurse until exclude can be carved out.
	if whole.Bits() >= bitLen(whole) {
		if exclude.Contains(whole.Addr()) {
			return nil
		}
		return []netip.Prefix{whole}
	}
	left, right := splitPrefix(whole)
	var out []netip.Prefix
	out = append(out, excludePrefix(left, exclude)...)
	out = append(out, excludePrefix(right, exclude)...)
	return out
}

func bitLen(p netip.Prefix) int {
	if p.Addr().Is4() {
		return 32
	}
	return 128
}

func splitPrefix(p netip.Prefix) (netip.Prefix, netip.Prefix) {
	bits := p.Bits() + 1
	addr := p.Addr()
	left, err := addr.Prefix(bits)
	if err != nil {
		return p, netip.Prefix{}
	}
	left = left.Masked()
	// Flip the bit that distinguishes the two halves.
	rightAddr := setHostBit(addr, bits)
	right, err := rightAddr.Prefix(bits)
	if err != nil {
		return left, netip.Prefix{}
	}
	return left, right.Masked()
}

func setHostBit(addr netip.Addr, bitIndexAfterMSB int) netip.Addr {
	// bitIndexAfterMSB is the new prefix length (1..bitLen); the bit at
	// position (bitIndexAfterMSB-1) from the MSB is the one that splits the
	// parent prefix into left (0) and right (1).
	if addr.Is4() {
		var b [4]byte
		copy(b[:], addr.AsSlice())
		bit := bitIndexAfterMSB - 1 // 0..31
		b[bit/8] |= 1 << (7 - uint(bit%8))
		return netip.AddrFrom4(b)
	}
	var b [16]byte
	copy(b[:], addr.AsSlice())
	bit := bitIndexAfterMSB - 1 // 0..127
	b[bit/8] |= 1 << (7 - uint(bit%8))
	return netip.AddrFrom16(b)
}

func portsNoLongerAllowed(newPol *policy.NetworkPolicy, addr netip.Addr, ports []int, ranges []policy.PortRange) ([]int, []policy.PortRange) {
	if newPol.EvaluateIP(addr) == policy.ActionAllow {
		return nil, nil
	}
	var outPorts []int
	for _, p := range ports {
		if !tcpPortAllowed(newPol, addr, p) {
			outPorts = append(outPorts, p)
		}
	}
	var outRanges []policy.PortRange
	for _, r := range ranges {
		// Expand small ranges conservatively into ports still denied; for
		// large floor ranges keep the range if any port inside is denied.
		if r.Hi-r.Lo > 64 {
			if !tcpPortAllowed(newPol, addr, r.Lo) || !tcpPortAllowed(newPol, addr, r.Hi) {
				outRanges = append(outRanges, r)
			}
			continue
		}
		for p := r.Lo; p <= r.Hi; p++ {
			if !tcpPortAllowed(newPol, addr, p) {
				outPorts = append(outPorts, p)
			}
		}
	}
	return outPorts, outRanges
}

func portsFullyAllowedOnPrefix(newPol *policy.NetworkPolicy, pfx netip.Prefix, ports []int, ranges []policy.PortRange) bool {
	// Sample network + last address; if both still allow every listed port,
	// treat the port-scoped allow as retained for the whole prefix.
	samples := []netip.Addr{pfx.Addr()}
	if last := lastAddr(pfx); last.IsValid() && last != pfx.Addr() {
		samples = append(samples, last)
	}
	for _, addr := range samples {
		leftPorts, leftRanges := portsNoLongerAllowed(newPol, addr, ports, ranges)
		if len(leftPorts) > 0 || len(leftRanges) > 0 {
			return false
		}
	}
	return true
}

func tcpPortAllowed(newPol *policy.NetworkPolicy, addr netip.Addr, port int) bool {
	if newPol == nil || !addr.IsValid() || port < 1 || port > 65535 {
		return false
	}
	if newPol.EvaluateIP(addr) == policy.ActionAllow {
		return true
	}
	deny, allow := newPol.PortScopedRules()
	for _, r := range deny {
		if portScopedMatches(r, addr, port) {
			return false
		}
	}
	for _, r := range allow {
		if portScopedMatches(r, addr, port) {
			return true
		}
	}
	return newPol.DefaultAction == policy.ActionAllow
}

func portScopedMatches(r policy.PortScopedRule, addr netip.Addr, port int) bool {
	if r.Target != "" {
		if pfx, err := netip.ParsePrefix(r.Target); err == nil {
			if !pfx.Contains(addr) {
				return false
			}
		} else if ip, err := netip.ParseAddr(r.Target); err == nil {
			if ip.Unmap() != addr {
				return false
			}
		} else {
			return false
		}
	}
	for _, p := range r.Ports {
		if p == port {
			return true
		}
	}
	for _, rg := range r.Ranges {
		if rg.Contains(port) {
			return true
		}
	}
	return false
}

func lastAddr(pfx netip.Prefix) netip.Addr {
	if !pfx.IsValid() {
		return netip.Addr{}
	}
	addr := pfx.Addr()
	bits := pfx.Bits()
	if addr.Is4() {
		var b [4]byte
		copy(b[:], addr.AsSlice())
		hostBits := 32 - bits
		if hostBits <= 0 {
			return addr
		}
		mask := uint32((1 << hostBits) - 1)
		v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		v |= mask
		return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	}
	var b [16]byte
	copy(b[:], addr.AsSlice())
	hostBits := 128 - bits
	for i := 15; hostBits > 0; i-- {
		take := hostBits
		if take > 8 {
			take = 8
		}
		b[i] |= byte((1 << take) - 1)
		hostBits -= take
	}
	return netip.AddrFrom16(b)
}
