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

package nftables

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
	"github.com/alibaba/opensandbox/egress/pkg/log"
	"github.com/alibaba/opensandbox/egress/pkg/policy"
)

const (
	tableName     = "opensandbox"
	chainName     = "egress"
	allowV4Set    = "allow_v4"
	allowV6Set    = "allow_v6"
	denyV4Set     = "deny_v4"
	denyV6Set     = "deny_v6"
	dohBlockV4Set = "doh_block_v4"
	dohBlockV6Set = "doh_block_v6"
)

type runner func(ctx context.Context, script string) ([]byte, error)

// Options controls nftables enforcement extras.
type Options struct {
	// BlockDoT drops tcp/udp 853 to prevent DNS-over-TLS bypass.
	BlockDoT bool
	// BlockDoH443 drops HTTPS DoH endpoints; when blocklist is empty and enabled, 443 is dropped.
	BlockDoH443    bool
	DoHBlocklistV4 []string
	DoHBlocklistV6 []string
}

// Manager applies static IP/CIDR policy into nftables and dynamic DNS-learned IPs.
type Manager struct {
	run  runner
	opts Options
	mu   sync.Mutex
}

// NewManager builds an nftables manager that shells out to `nft -f -` with defaults.
func NewManager() *Manager {
	return &Manager{run: defaultRunner, opts: Options{BlockDoT: true}}
}

// NewManagerWithRunner is for tests; allows capturing the rendered ruleset (defaults to BlockDoT=true).
func NewManagerWithRunner(r runner) *Manager {
	return &Manager{run: r, opts: Options{BlockDoT: true}}
}

// NewManagerWithRunnerAndOptions is for tests needing custom options.
func NewManagerWithRunnerAndOptions(r runner, opts Options) *Manager {
	return &Manager{run: r, opts: opts}
}

// NewManagerWithOptions allows customizing behavior (used by main()).
func NewManagerWithOptions(opts Options) *Manager {
	return &Manager{run: defaultRunner, opts: opts}
}

// ApplyStatic reconciles static allow/deny IP and CIDR entries into nftables.
// It creates a dedicated table/chain and overwrites previous state.
// Uses the same mutex as AddResolvedIPs so a /policy update never overlaps a DNS
// callback: without this, add-element could run while the table is being deleted/recreated
// and fail, causing a transient deny for a client that already got an allowed DNS answer.
func (m *Manager) ApplyStatic(ctx context.Context, p *policy.NetworkPolicy) error {
	if p == nil {
		p = policy.DefaultDenyPolicy()
	}
	allowV4, allowV6, denyV4, denyV6 := p.StaticIPSets()
	log.Infof("nftables: applying static policy: default=%s, allow_v4=%d, allow_v6=%d, deny_v4=%d, deny_v6=%d",
		p.DefaultAction, len(allowV4), len(allowV6), len(denyV4), len(denyV6))
	m.mu.Lock()
	defer m.mu.Unlock()
	script := buildRuleset(p, m.opts)
	if _, err := m.run(ctx, script); err != nil {
		// On a fresh host the delete-table may fail; retry once without the delete line.
		if isMissingTableError(err) {
			fallback := removeDeleteTableLine(script)
			if fallback != script {
				if _, retryErr := m.run(ctx, fallback); retryErr == nil {
					return nil
				}
			}
		}
		return err
	}
	log.Infof("nftables: static policy applied successfully")
	return nil
}

// AddResolvedIPs adds DNS-learned IPs to dynamic allow sets with TTL-based timeout.
// TTL is clamped to minTTLSec–maxTTLSec. Call only when table exists (dns+nft mode).
func (m *Manager) AddResolvedIPs(ctx context.Context, ips []ResolvedIP) error {
	if len(ips) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	script := buildAddResolvedIPsScript(tableName, ips)
	if script == "" {
		return nil
	}
	log.Infof("nftables: adding %d resolved IP(s) to dynamic allow sets with script statement %s", len(ips), script)
	_, err := m.run(ctx, script)
	return err
}

func buildRuleset(p *policy.NetworkPolicy, opts Options) string {
	allowV4, allowV6, denyV4, denyV6 := p.StaticIPSets()

	var b strings.Builder
	// Reset and re-create table, sets, and chain.
	fmt.Fprintf(&b, "delete table inet %s\n", tableName)
	fmt.Fprintf(&b, "add table inet %s\n", tableName)

	fmt.Fprintf(&b, "add set inet %s %s { type ipv4_addr; flags interval; }\n", tableName, allowV4Set)
	fmt.Fprintf(&b, "add set inet %s %s { type ipv4_addr; flags interval; }\n", tableName, denyV4Set)
	fmt.Fprintf(&b, "add set inet %s %s { type ipv6_addr; flags interval; }\n", tableName, allowV6Set)
	fmt.Fprintf(&b, "add set inet %s %s { type ipv6_addr; flags interval; }\n", tableName, denyV6Set)
	fmt.Fprintf(&b, "add set inet %s %s { type ipv4_addr; timeout %ds; }\n", tableName, dynAllowV4Set, dynSetTimeoutS)
	fmt.Fprintf(&b, "add set inet %s %s { type ipv6_addr; timeout %ds; }\n", tableName, dynAllowV6Set, dynSetTimeoutS)

	if len(opts.DoHBlocklistV4) > 0 {
		fmt.Fprintf(&b, "add set inet %s %s { type ipv4_addr; flags interval; }\n", tableName, dohBlockV4Set)
	}
	if len(opts.DoHBlocklistV6) > 0 {
		fmt.Fprintf(&b, "add set inet %s %s { type ipv6_addr; flags interval; }\n", tableName, dohBlockV6Set)
	}

	writeElements(&b, allowV4Set, allowV4)
	writeElements(&b, denyV4Set, denyV4)
	writeElements(&b, allowV6Set, allowV6)
	writeElements(&b, denyV6Set, denyV6)
	writeElements(&b, dohBlockV4Set, opts.DoHBlocklistV4)
	writeElements(&b, dohBlockV6Set, opts.DoHBlocklistV6)

	chainPolicy := "drop"
	if p.DefaultAction == policy.ActionAllow {
		chainPolicy = "accept"
	}
	fmt.Fprintf(&b, "add chain inet %s %s { type filter hook output priority 0; policy %s; }\n", tableName, chainName, chainPolicy)
	fmt.Fprintf(&b, "add rule inet %s %s ct state established,related accept\n", tableName, chainName)
	fmt.Fprintf(&b, "add rule inet %s %s meta mark %s accept\n", tableName, chainName, constants.MarkHex)
	fmt.Fprintf(&b, "add rule inet %s %s oifname \"lo\" accept\n", tableName, chainName)
	if opts.BlockDoT {
		fmt.Fprintf(&b, "add rule inet %s %s tcp dport 853 drop\n", tableName, chainName)
		fmt.Fprintf(&b, "add rule inet %s %s udp dport 853 drop\n", tableName, chainName)
	}
	if opts.BlockDoH443 {
		if len(opts.DoHBlocklistV4) == 0 && len(opts.DoHBlocklistV6) == 0 {
			// strict: drop all 443 when enabled but no blocklist provided
			fmt.Fprintf(&b, "add rule inet %s %s tcp dport 443 drop\n", tableName, chainName)
		} else {
			if len(opts.DoHBlocklistV4) > 0 {
				fmt.Fprintf(&b, "add rule inet %s %s ip daddr @%s tcp dport 443 drop\n", tableName, chainName, dohBlockV4Set)
			}
			if len(opts.DoHBlocklistV6) > 0 {
				fmt.Fprintf(&b, "add rule inet %s %s ip6 daddr @%s tcp dport 443 drop\n", tableName, chainName, dohBlockV6Set)
			}
		}
	}
	fmt.Fprintf(&b, "add rule inet %s %s ip daddr @%s drop\n", tableName, chainName, denyV4Set)
	fmt.Fprintf(&b, "add rule inet %s %s ip6 daddr @%s drop\n", tableName, chainName, denyV6Set)
	fmt.Fprintf(&b, "add rule inet %s %s ip daddr @%s accept\n", tableName, chainName, dynAllowV4Set)
	fmt.Fprintf(&b, "add rule inet %s %s ip6 daddr @%s accept\n", tableName, chainName, dynAllowV6Set)
	fmt.Fprintf(&b, "add rule inet %s %s ip daddr @%s accept\n", tableName, chainName, allowV4Set)
	fmt.Fprintf(&b, "add rule inet %s %s ip6 daddr @%s accept\n", tableName, chainName, allowV6Set)
	if chainPolicy == "drop" {
		fmt.Fprintf(&b, "add rule inet %s %s counter drop\n", tableName, chainName)
	}

	return b.String()
}

func writeElements(b *strings.Builder, setName string, elems []string) {
	if len(elems) == 0 {
		return
	}
	fmt.Fprintf(b, "add element inet %s %s { %s }\n", tableName, setName, strings.Join(elems, ", "))
}

func defaultRunner(ctx context.Context, script string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("nft apply failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file or directory") && strings.Contains(msg, "delete table inet "+tableName)
}

func removeDeleteTableLine(script string) string {
	lines := strings.Split(script, "\n")
	var filtered []string
	for _, l := range lines {
		if strings.HasPrefix(l, "delete table inet "+tableName) {
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		filtered = append(filtered, l)
	}
	return strings.Join(filtered, "\n")
}
