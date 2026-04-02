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

package dnsproxy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/alibaba/opensandbox/egress/pkg/events"
	"github.com/alibaba/opensandbox/egress/pkg/log"
	"github.com/alibaba/opensandbox/egress/pkg/nftables"
	"github.com/alibaba/opensandbox/egress/pkg/policy"
	"github.com/alibaba/opensandbox/egress/pkg/telemetry"
	slogger "github.com/alibaba/opensandbox/internal/logger"
)

const defaultListenAddr = "127.0.0.1:15353"

type Proxy struct {
	policyMu        sync.RWMutex
	userPolicy      *policy.NetworkPolicy
	effectivePolicy *policy.NetworkPolicy
	alwaysDeny      []policy.EgressRule
	alwaysAllow     []policy.EgressRule
	listenAddr      string
	upstream        string // single upstream for MVP
	servers         []*dns.Server

	// optional; called in goroutine when A/AAAA are present
	onResolved func(domain string, ips []nftables.ResolvedIP)

	// optional broadcaster to notify blocked hostnames
	blockedBroadcaster *events.Broadcaster
}

// New builds a proxy with resolved upstream; listenAddr can be empty for default.
// alwaysDeny and alwaysAllow are optional operator rules merged ahead of user egress
// (see policy.MergeAlwaysOverlay); they are not persisted via the policy API.
func New(p *policy.NetworkPolicy, listenAddr string, alwaysDeny, alwaysAllow []policy.EgressRule) (*Proxy, error) {
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	if p == nil {
		p = policy.DefaultDenyPolicy()
	}
	_, err := discoverUpstream()
	if err != nil {
		return nil, err
	}
	proxy := &Proxy{
		listenAddr:  listenAddr,
		upstream:    "29.29.29.29",
		userPolicy:  ensurePolicyDefaults(p),
		alwaysDeny:  append([]policy.EgressRule(nil), alwaysDeny...),
		alwaysAllow: append([]policy.EgressRule(nil), alwaysAllow...),
	}
	proxy.refreshEffectivePolicy()
	return proxy, nil
}

func (p *Proxy) refreshEffectivePolicy() {
	p.effectivePolicy = policy.MergeAlwaysOverlay(p.userPolicy, p.alwaysDeny, p.alwaysAllow)
}

func (p *Proxy) Start(ctx context.Context) error {
	handler := dns.HandlerFunc(p.serveDNS)

	udpServer := &dns.Server{Addr: p.listenAddr, Net: "udp", Handler: handler}
	tcpServer := &dns.Server{Addr: p.listenAddr, Net: "tcp", Handler: handler}
	p.servers = []*dns.Server{udpServer, tcpServer}

	errCh := make(chan error, len(p.servers))
	for _, srv := range p.servers {
		s := srv
		go func() {
			if err := s.ListenAndServe(); err != nil {
				errCh <- err
			}
		}()
	}

	// Shutdown on context done
	go func() {
		<-ctx.Done()
		for _, srv := range p.servers {
			_ = srv.Shutdown()
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("dns proxy failed: %w", err)
	case <-time.After(200 * time.Millisecond):
		// small grace window; running fine
		return nil
	}
}

func (p *Proxy) serveDNS(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		_ = w.WriteMsg(new(dns.Msg)) // empty response
		return
	}
	q := r.Question[0]
	domain := q.Name
	host := normalizeDNSHost(domain)

	p.policyMu.RLock()
	currentPolicy := p.effectivePolicy
	p.policyMu.RUnlock()
	if currentPolicy != nil && currentPolicy.Evaluate(domain) == policy.ActionDeny {
		telemetry.RecordDNSDenied()
		p.publishBlocked(domain)
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(resp)
		return
	}

	start := time.Now()
	resp, err := p.forward(r)
	elapsed := time.Since(start).Seconds()
	if err != nil {
		telemetry.RecordDNSForward(elapsed)
		logOutboundDNS(host, nil, "", err.Error())
		log.Warnf("[dns] forward error for %s: %v", domain, err)
		fail := new(dns.Msg)
		fail.SetRcode(r, dns.RcodeServerFailure)
		_ = w.WriteMsg(fail)
		return
	}
	telemetry.RecordDNSForward(elapsed)
	logOutboundDNS(host, resolvedIPStrings(resp), "", "")
	p.maybeNotifyResolved(domain, resp)
	_ = w.WriteMsg(resp)
}

// maybeNotifyResolved calls onResolved synchronously when resp contains A/AAAA,
// so that IPs are in nft before the client receives the DNS response and connects.
func (p *Proxy) maybeNotifyResolved(domain string, resp *dns.Msg) {
	if p.onResolved == nil {
		return
	}
	ips := extractResolvedIPs(resp)
	if len(ips) == 0 {
		return
	}
	p.onResolved(domain, ips)
}

func (p *Proxy) forward(r *dns.Msg) (*dns.Msg, error) {
	c := &dns.Client{
		Timeout: 5 * time.Second,
		Dialer:  p.dialerWithMark(),
	}
	resp, _, err := c.Exchange(r, p.upstream)
	return resp, err
}

// UpstreamHost returns the host part of the upstream resolver, empty on parse error.
func (p *Proxy) UpstreamHost() string {
	host, _, err := net.SplitHostPort(p.upstream)
	if err != nil {
		return ""
	}
	return host
}

// UpdatePolicy swaps the user-facing policy (without always-deny/allow file overlay).
// Passing nil reverts to the default deny-all policy.
func (p *Proxy) UpdatePolicy(newPolicy *policy.NetworkPolicy) {
	p.policyMu.Lock()
	p.userPolicy = ensurePolicyDefaults(newPolicy)
	p.refreshEffectivePolicy()
	p.policyMu.Unlock()
}

// CurrentPolicy returns the user policy (POST/PATCH/GET), not the always-deny/allow overlay.
func (p *Proxy) CurrentPolicy() *policy.NetworkPolicy {
	p.policyMu.RLock()
	defer p.policyMu.RUnlock()
	return p.userPolicy
}

// SetOnResolved sets the callback invoked when an allowed domain resolves to A/AAAA.
// Called in a goroutine; pass nil to disable. Only used when L2 dynamic IP is enabled (e.g. dns+nft mode).
func (p *Proxy) SetOnResolved(fn func(domain string, ips []nftables.ResolvedIP)) {
	p.onResolved = fn
}

// SetBlockedBroadcaster wires a broadcaster used to notify blocked hostnames.
func (p *Proxy) SetBlockedBroadcaster(b *events.Broadcaster) {
	p.blockedBroadcaster = b
}

func (p *Proxy) publishBlocked(domain string) {
	if p.blockedBroadcaster == nil {
		return
	}
	normalized := strings.ToLower(strings.TrimSuffix(domain, "."))
	if normalized == "" {
		return
	}

	p.blockedBroadcaster.Publish(events.BlockedEvent{
		Hostname:  normalized,
		Timestamp: time.Now().UTC(),
	})
}

// extractResolvedIPs parses A and AAAA records from resp.Answer into ResolvedIP slice.
//
// Uses netip.ParseAddr(v.A.String()) which allocates a temporary string per record; typically
// one or a few records per resolution, so the cost is small compared to DNS RTT and nft writes.
func extractResolvedIPs(resp *dns.Msg) []nftables.ResolvedIP {
	if resp == nil || len(resp.Answer) == 0 {
		return nil
	}

	var out []nftables.ResolvedIP
	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.A:
			if v.A == nil {
				continue
			}
			addr, err := netip.ParseAddr(v.A.String())
			if err != nil {
				continue
			}
			out = append(out, nftables.ResolvedIP{Addr: addr, TTL: time.Duration(v.Hdr.Ttl) * time.Second})
		case *dns.AAAA:
			if v.AAAA == nil {
				continue
			}
			addr, err := netip.ParseAddr(v.AAAA.String())
			if err != nil {
				continue
			}
			out = append(out, nftables.ResolvedIP{Addr: addr, TTL: time.Duration(v.Hdr.Ttl) * time.Second})
		}
	}
	return out
}

const fallbackUpstream = "8.8.8.8:53"

func discoverUpstream() (string, error) {
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || len(cfg.Servers) == 0 {
		if err != nil {
			log.Warnf("[dns] fallback upstream resolver due to error: %v", err)
		}
		return fallbackUpstream, nil
	}
	// Prefer first non-loopback nameserver (e.g. K8s cluster DNS after 127.0.0.11).
	// If only loopback exists (e.g. Docker 127.0.0.11), use it: proxy upstream traffic
	// is marked and bypasses the redirect, so loopback is reachable from the sidecar.
	var chosen string
	for _, s := range cfg.Servers {
		if ip := net.ParseIP(s); ip != nil && ip.IsLoopback() {
			if chosen == "" {
				chosen = s
			}
			continue
		}
		chosen = s
		break
	}
	if chosen == "" {
		chosen = cfg.Servers[0]
	}
	return net.JoinHostPort(chosen, cfg.Port), nil
}

// ResolvNameserverIPs reads nameserver lines from resolvPath and returns parsed IPv4/IPv6 addresses.
// Used at startup to whitelist the system DNS so client traffic to it is allowed and proxy can use it as upstream.
func ResolvNameserverIPs(resolvPath string) ([]netip.Addr, error) {
	cfg, err := dns.ClientConfigFromFile(resolvPath)
	if err != nil || len(cfg.Servers) == 0 {
		return nil, nil
	}
	var out []netip.Addr
	for _, s := range cfg.Servers {
		ip, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		out = append(out, ip)
	}
	return out, nil
}

func normalizeDNSHost(domain string) string {
	return strings.ToLower(strings.TrimSuffix(domain, "."))
}

func resolvedIPStrings(resp *dns.Msg) []string {
	ri := extractResolvedIPs(resp)
	if len(ri) == 0 {
		return nil
	}
	out := make([]string, 0, len(ri))
	for _, x := range ri {
		out = append(out, x.Addr.String())
	}
	return out
}

func logOutboundDNS(host string, ips []string, peer string, errStr string) {
	fields := []slogger.Field{
		{Key: "opensandbox.event", Value: "egress.outbound"},
	}
	if host != "" {
		fields = append(fields, slogger.Field{Key: "target.host", Value: host})
	}
	if peer != "" {
		fields = append(fields, slogger.Field{Key: "peer", Value: peer})
	}
	if len(ips) > 0 {
		fields = append(fields, slogger.Field{Key: "target.ips", Value: ips})
	}
	if errStr != "" {
		fields = append(fields, slogger.Field{Key: "error", Value: errStr})
	}
	log.Logger.With(fields...).Infof("egress outbound")
}

func ensurePolicyDefaults(p *policy.NetworkPolicy) *policy.NetworkPolicy {
	if p == nil {
		return policy.DefaultDenyPolicy()
	}
	if p.DefaultAction == "" {
		p.DefaultAction = policy.ActionDeny
	}
	return p
}
