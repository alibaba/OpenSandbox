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
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/dnsproxy"
	"github.com/alibaba/opensandbox/egress/pkg/iptables"
	"github.com/alibaba/opensandbox/egress/pkg/metrics"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	initialRules, err := dnsproxy.LoadPolicyFromEnvVar(constants.EnvEgressRules)
	if err != nil {
		log.Fatalf("failed to parse %s: %v", constants.EnvEgressRules, err)
	}

	allowIPs := AllowIPsForNft("/etc/resolv.conf")

	mode := parseMode()
	metrics.SetEnforcementMode(mode)
	nftMgr := createNftManager(mode)
	proxy, err := dnsproxy.New(initialRules, "")
	if err != nil {
		log.Fatalf("failed to init dns proxy: %v", err)
	}
	metrics.SetPolicyRuleCount(initialRules.DefaultAction, len(initialRules.Egress))
	if err := proxy.Start(ctx); err != nil {
		log.Fatalf("failed to start dns proxy: %v", err)
	}
	log.Println("dns proxy started on 127.0.0.1:15353")

	if err := iptables.SetupRedirect(15353); err != nil {
		log.Fatalf("failed to install iptables redirect: %v", err)
	}
	log.Printf("iptables redirect configured (OUTPUT 53 -> 15353) with SO_MARK bypass for proxy upstream traffic")

	setupNft(ctx, nftMgr, initialRules, proxy, allowIPs)

	// start policy server
	httpAddr := envOrDefault(constants.EnvEgressHTTPAddr, constants.DefaultEgressServerAddr)
	if err = startPolicyServer(ctx, proxy, nftMgr, mode, httpAddr, os.Getenv(constants.EnvEgressToken), allowIPs); err != nil {
		log.Fatalf("failed to start policy server: %v", err)
	}
	log.Printf("policy server listening on %s (POST /policy)", httpAddr)

	<-ctx.Done()
	log.Println("received shutdown signal; exiting")
	_ = os.Stderr.Sync()
}

func envOrDefault(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
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
