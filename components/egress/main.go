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
	"context"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/dnsproxy"
	"github.com/alibaba/opensandbox/egress/pkg/iptables"
	"github.com/alibaba/opensandbox/egress/pkg/nftables"
)

// Linux MVP: DNS proxy + iptables REDIRECT. No nftables/full isolation yet.
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Optional bootstrap via env; still allow runtime HTTP updates.
	initialPolicy, err := dnsproxy.LoadPolicyFromEnvVar(constants.EnvEgressRules)
	if err != nil {
		log.Fatalf("failed to parse %s: %v", constants.EnvEgressRules, err)
	}
	if initialPolicy != nil {
		log.Printf("loaded initial egress policy from %s", constants.EnvEgressRules)
	}

	requestedMode := parseMode()
	enforcementMode := requestedMode

	var nftMgr nftApplier
	if requestedMode == constants.PolicyDnsNft {
		nftOpts := parseNftOptions()
		nftMgr = nftables.NewManagerWithOptions(nftOpts)
	}

	proxy, err := dnsproxy.New(initialPolicy, "")
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
	log.Printf("iptables redirect configured (OUTPUT 53 -> 15353) with SO_MARK bypass for proxy upstream traffic")

	if nftMgr != nil {
		if err := nftMgr.ApplyStatic(ctx, initialPolicy); err != nil {
			log.Fatalf("nftables static apply failed; please check logs): %v", err)
		} else {
			log.Printf("nftables static policy applied (table inet opensandbox)")
		}
	}

	httpAddr := os.Getenv(constants.EnvEgressHTTPAddr)
	if httpAddr == "" {
		httpAddr = constants.DefaultEgressServerAddr
	}
	token := os.Getenv(constants.EnvEgressToken)
	if err := startPolicyServer(ctx, proxy, nftMgr, enforcementMode, httpAddr, token); err != nil {
		log.Fatalf("failed to start policy server: %v", err)
	}
	log.Printf("policy server listening on %s (POST /policy)", httpAddr)

	<-ctx.Done()
	log.Println("received shutdown signal; exiting")
	_ = os.Stderr.Sync()
}

func parseNftOptions() nftables.Options {
	opts := nftables.Options{BlockDoT: true}
	if isTruthy(os.Getenv(constants.EnvBlockDoH443)) {
		opts.BlockDoH443 = true
	}
	if raw := os.Getenv(constants.EnvDoHBlocklist); strings.TrimSpace(raw) != "" {
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			target := strings.TrimSpace(p)
			if target == "" {
				continue
			}
			if addr, err := netip.ParseAddr(target); err == nil {
				if addr.Is4() {
					opts.DoHBlocklistV4 = append(opts.DoHBlocklistV4, target)
				} else if addr.Is6() {
					opts.DoHBlocklistV6 = append(opts.DoHBlocklistV6, target)
				}
				continue
			}
			if prefix, err := netip.ParsePrefix(target); err == nil {
				if prefix.Addr().Is4() {
					opts.DoHBlocklistV4 = append(opts.DoHBlocklistV4, target)
				} else if prefix.Addr().Is6() {
					opts.DoHBlocklistV6 = append(opts.DoHBlocklistV6, target)
				}
				continue
			}
			log.Printf("ignoring invalid DoH blocklist entry: %s", target)
		}
	}
	return opts
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(constants.EnvEgressMode)))
	switch mode {
	case "", constants.PolicyDnsOnly:
		return constants.PolicyDnsOnly
	case constants.PolicyDnsNft:
		return constants.PolicyDnsNft
	default:
		log.Printf("invalid %s=%s, falling back to dns", constants.EnvEgressMode, mode)
		return constants.PolicyDnsOnly
	}
}
