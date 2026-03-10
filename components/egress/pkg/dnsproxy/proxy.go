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
	"os"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/alibaba/opensandbox/egress/pkg/log"
	"github.com/alibaba/opensandbox/egress/pkg/nftables"
	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

const defaultListenAddr = "127.0.0.1:15353"

type Proxy struct {
	policyMu   sync.RWMutex
	policy     *policy.NetworkPolicy
	listenAddr string
	upstream   string // single upstream for MVP
	servers    []*dns.Server

	// optional; called in goroutine when A/AAAA are present
	onResolved func(domain string, ips []nftables.ResolvedIP)
}

// New builds a proxy with resolved upstream; listenAddr can be empty for default.
func New(p *policy.NetworkPolicy, listenAddr string) (*Proxy, error) {
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	if p == nil {
		p = policy.DefaultDenyPolicy()
	}
	upstream, err := discoverUpstream()
	if err != nil {
		return nil, err
	}
	proxy := &Proxy{
		listenAddr: listenAddr,
		upstream:   upstream,
		policy:     ensurePolicyDefaults(p),
	}
	return proxy, nil
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

	p.policyMu.RLock()
	currentPolicy := p.policy
	p.policyMu.RUnlock()
	if currentPolicy != nil && currentPolicy.Evaluate(domain) == policy.ActionDeny {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(resp)
		return
	}

	resp, err := p.forward(r)
	if err != nil {
		log.Warnf("[dns] forward error for %s: %v", domain, err)
		fail := new(dns.Msg)
		fail.SetRcode(r, dns.RcodeServerFailure)
		_ = w.WriteMsg(fail)
		return
	}
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

// UpdatePolicy swaps the in-memory policy used by the proxy.
// Passing nil reverts to the default deny-all policy.
func (p *Proxy) UpdatePolicy(newPolicy *policy.NetworkPolicy) {
	p.policyMu.Lock()
	p.policy = ensurePolicyDefaults(newPolicy)
	p.policyMu.Unlock()
}

// CurrentPolicy returns the policy currently enforced by the proxy.
func (p *Proxy) CurrentPolicy() *policy.NetworkPolicy {
	p.policyMu.RLock()
	defer p.policyMu.RUnlock()
	return p.policy
}

// SetOnResolved sets the callback invoked when an allowed domain resolves to A/AAAA.
// Called in a goroutine; pass nil to disable. Only used when L2 dynamic IP is enabled (e.g. dns+nft mode).
func (p *Proxy) SetOnResolved(fn func(domain string, ips []nftables.ResolvedIP)) {
	p.onResolved = fn
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

// LoadPolicyFromEnvVar reads the given env var and parses a policy; empty falls back to default deny-all.
func LoadPolicyFromEnvVar(envName string) (*policy.NetworkPolicy, error) {
	raw := os.Getenv(envName)
	if raw == "" {
		return policy.DefaultDenyPolicy(), nil
	}
	return policy.ParsePolicy(raw)
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
