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
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/alibaba/opensandbox/egress/pkg/nftables"
	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

func TestProxyUpdatePolicy(t *testing.T) {
	proxy, err := New(nil, "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("init proxy: %v", err)
	}

	if proxy.CurrentPolicy() == nil {
		t.Fatalf("expected default deny policy (non-nil)")
	}
	if got := proxy.CurrentPolicy().Evaluate("example.com."); got != policy.ActionDeny {
		t.Fatalf("expected default deny, got %s", got)
	}

	pol, err := policy.ParsePolicy(`{"defaultAction":"deny","egress":[{"action":"allow","target":"example.com"}]}`)
	if err != nil {
		t.Fatalf("parse policy: %v", err)
	}

	proxy.UpdatePolicy(pol)
	if proxy.CurrentPolicy() == nil {
		t.Fatalf("expected policy after update")
	}
	if got := proxy.CurrentPolicy().Evaluate("example.com."); got != policy.ActionAllow {
		t.Fatalf("policy evaluation mismatch, want allow got %s", got)
	}

	proxy.UpdatePolicy(nil)
	if proxy.CurrentPolicy() == nil {
		t.Fatalf("expected default deny policy after clearing")
	}
	if got := proxy.CurrentPolicy().Evaluate("example.com."); got != policy.ActionDeny {
		t.Fatalf("expected default deny after clearing, got %s", got)
	}
}

func TestLoadPolicyFromEnvVar(t *testing.T) {
	const envName = "TEST_EGRESS_POLICY"
	t.Setenv(envName, `{"defaultAction":"deny","egress":[{"action":"allow","target":"example.com"}]}`)

	pol, err := LoadPolicyFromEnvVar(envName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pol == nil || pol.Evaluate("example.com.") != policy.ActionAllow {
		t.Fatalf("expected parsed policy to allow example.com")
	}

	t.Setenv(envName, "")
	pol, err = LoadPolicyFromEnvVar(envName)
	if err != nil {
		t.Fatalf("unexpected error on empty env: %v", err)
	}
	if pol == nil {
		t.Fatalf("expected default deny policy when env is empty")
	}
	if pol.DefaultAction != policy.ActionDeny {
		t.Fatalf("expected default deny when env is empty, got %+v", pol)
	}
}

func TestExtractResolvedIPs(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Ttl: 120}, A: net.ParseIP("1.2.3.4")},
		&dns.AAAA{Hdr: dns.RR_Header{Name: "example.com.", Ttl: 60}, AAAA: net.ParseIP("2001:db8::1")},
		&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Ttl: 90}, A: net.ParseIP("5.6.7.8")},
	}
	ips := extractResolvedIPs(msg)
	if len(ips) != 3 {
		t.Fatalf("expected 3 IPs, got %d", len(ips))
	}
	// Order follows Answer; check first A and AAAA
	if ips[0].Addr.String() != "1.2.3.4" || ips[0].TTL != 120*time.Second {
		t.Fatalf("first IP: got %s TTL %v", ips[0].Addr, ips[0].TTL)
	}
	if ips[1].Addr.String() != "2001:db8::1" || ips[1].TTL != 60*time.Second {
		t.Fatalf("second IP: got %s TTL %v", ips[1].Addr, ips[1].TTL)
	}
	if ips[2].Addr.String() != "5.6.7.8" || ips[2].TTL != 90*time.Second {
		t.Fatalf("third IP: got %s TTL %v", ips[2].Addr, ips[2].TTL)
	}
}

func TestExtractResolvedIPs_EmptyOrNil(t *testing.T) {
	if got := extractResolvedIPs(nil); got != nil {
		t.Fatalf("nil msg: expected nil, got %v", got)
	}
	msg := new(dns.Msg)
	if got := extractResolvedIPs(msg); got != nil {
		t.Fatalf("empty answer: expected nil, got %v", got)
	}
	msg.Answer = []dns.RR{&dns.CNAME{Hdr: dns.RR_Header{Name: "x."}, Target: "y."}}
	if got := extractResolvedIPs(msg); got != nil {
		t.Fatalf("CNAME only: expected nil, got %v", got)
	}
}

func TestSetOnResolved(t *testing.T) {
	proxy, err := New(policy.DefaultDenyPolicy(), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var called bool
	var capturedDomain string
	var capturedIPs []nftables.ResolvedIP
	proxy.SetOnResolved(func(domain string, ips []nftables.ResolvedIP) {
		called = true
		capturedDomain = domain
		capturedIPs = ips
	})
	if proxy.onResolved == nil {
		t.Fatalf("SetOnResolved did not set callback")
	}
	proxy.SetOnResolved(nil)
	if proxy.onResolved != nil {
		t.Fatalf("SetOnResolved(nil) did not clear callback")
	}
	_ = called
	_ = capturedDomain
	_ = capturedIPs
}

func TestMaybeNotifyResolved_CallsCallbackWhenAOrAAAA(t *testing.T) {
	proxy, err := New(policy.DefaultDenyPolicy(), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch := make(chan struct {
		domain string
		ips    []nftables.ResolvedIP
	}, 1)
	proxy.SetOnResolved(func(domain string, ips []nftables.ResolvedIP) {
		ch <- struct {
			domain string
			ips    []nftables.ResolvedIP
		}{domain, ips}
	})

	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Ttl: 120}, A: net.ParseIP("1.2.3.4")},
	}
	proxy.maybeNotifyResolved("example.com.", msg)

	select {
	case got := <-ch:
		if got.domain != "example.com." {
			t.Fatalf("domain: got %q", got.domain)
		}
		if len(got.ips) != 1 || got.ips[0].Addr.String() != "1.2.3.4" {
			t.Fatalf("ips: got %v", got.ips)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback was not invoked")
	}
}

func TestMaybeNotifyResolved_NoCallWhenOnResolvedNil(t *testing.T) {
	proxy, err := New(policy.DefaultDenyPolicy(), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "x.", Ttl: 60}, A: net.ParseIP("10.0.0.1")}}
	proxy.maybeNotifyResolved("x.", msg)
	// No callback set; should not panic. No assertion needed.
}

func TestMaybeNotifyResolved_NoCallWhenNoAOrAAAA(t *testing.T) {
	proxy, err := New(policy.DefaultDenyPolicy(), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch := make(chan struct {
		domain string
		ips    []nftables.ResolvedIP
	}, 1)
	proxy.SetOnResolved(func(domain string, ips []nftables.ResolvedIP) {
		ch <- struct {
			domain string
			ips    []nftables.ResolvedIP
		}{domain, ips}
	})

	msg := new(dns.Msg)
	msg.Answer = []dns.RR{&dns.CNAME{Hdr: dns.RR_Header{Name: "x."}, Target: "y."}}
	proxy.maybeNotifyResolved("x.", msg)

	select {
	case <-ch:
		t.Fatal("callback should not be invoked when resp has no A/AAAA")
	case <-time.After(200 * time.Millisecond):
		// Expected: no callback
	}
}
