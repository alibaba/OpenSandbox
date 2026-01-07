package dnsproxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/miekg/dns"

	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

const defaultListenAddr = "127.0.0.1:15353"

type Proxy struct {
	policy     *policy.NetworkPolicy
	listenAddr string
	upstream   string // single upstream for MVP
	servers    []*dns.Server
}

// New builds a proxy with resolved upstream; listenAddr can be empty for default.
func New(p *policy.NetworkPolicy, listenAddr string) (*Proxy, error) {
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	upstream, err := discoverUpstream()
	if err != nil {
		return nil, err
	}
	return &Proxy{
		policy:     p,
		listenAddr: listenAddr,
		upstream:   upstream,
	}, nil
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

	if p.policy != nil && p.policy.Evaluate(domain) == policy.ActionDeny {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(resp)
		return
	}

	resp, err := p.forward(r)
	if err != nil {
		log.Printf("[dns] forward error for %s: %v", domain, err)
		fail := new(dns.Msg)
		fail.SetRcode(r, dns.RcodeServerFailure)
		_ = w.WriteMsg(fail)
		return
	}
	_ = w.WriteMsg(resp)
}

func (p *Proxy) forward(r *dns.Msg) (*dns.Msg, error) {
	c := &dns.Client{Timeout: 5 * time.Second}
	resp, _, err := c.Exchange(r, p.upstream)
	return resp, err
}

func discoverUpstream() (string, error) {
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err == nil && len(cfg.Servers) > 0 {
		return net.JoinHostPort(cfg.Servers[0], cfg.Port), nil
	}
	// fallback to public resolver; comment to explain deterministic behavior
	log.Printf("[dns] fallback upstream resolver due to error: %v", err)
	return "8.8.8.8:53", nil
}

// LoadPolicyFromEnv reads OPENSANDBOX_NETWORK_POLICY and parses it.
func LoadPolicyFromEnv() (*policy.NetworkPolicy, error) {
	raw := os.Getenv("OPENSANDBOX_NETWORK_POLICY")
	if raw == "" {
		return nil, nil
	}
	return policy.ParsePolicy(raw)
}
