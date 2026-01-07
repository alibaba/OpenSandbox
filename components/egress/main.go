package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alibaba/opensandbox/egress/pkg/dnsproxy"
	"github.com/alibaba/opensandbox/egress/pkg/iptables"
)

// Linux MVP: DNS proxy + iptables REDIRECT. No nftables/full isolation yet.
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	policy, err := dnsproxy.LoadPolicyFromEnv()
	if err != nil {
		log.Fatalf("failed to parse network policy: %v", err)
	}
	if policy == nil {
		log.Println("OPENSANDBOX_NETWORK_POLICY empty; skip egress control")
		return
	}

	proxy, err := dnsproxy.New(policy, "")
	if err != nil {
		log.Fatalf("failed to init dns proxy: %v", err)
	}
	if err := proxy.Start(ctx); err != nil {
		log.Fatalf("failed to start dns proxy: %v", err)
	}
	log.Println("dns proxy started on 127.0.0.1:15353")

	if err := iptables.SetupRedirect(15353); err != nil {
		log.Fatalf("failed to install iptables redirect: %v", err)
	}
	log.Println("iptables redirect configured (OUTPUT 53 -> 15353)")

	<-ctx.Done()
	log.Println("received shutdown signal; exiting")
	_ = os.Stderr.Sync()
}
