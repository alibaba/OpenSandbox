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

package constants

const (
	EnvBlockDoH443      = "OPENSANDBOX_EGRESS_BLOCK_DOH_443"
	EnvDoHBlocklist     = "OPENSANDBOX_EGRESS_DOH_BLOCKLIST" // comma-separated IP/CIDR
	EnvEgressMode       = "OPENSANDBOX_EGRESS_MODE"          // dns | dns+nft
	EnvEgressHTTPAddr   = "OPENSANDBOX_EGRESS_HTTP_ADDR"
	EnvEgressToken      = "OPENSANDBOX_EGRESS_TOKEN"
	EnvEgressRules      = "OPENSANDBOX_EGRESS_RULES"
	EnvEgressPolicyFile = "OPENSANDBOX_EGRESS_POLICY_FILE" // optional JSON snapshot; if present and valid, overrides EnvEgressRules at startup
	EnvEgressLogLevel   = "OPENSANDBOX_EGRESS_LOG_LEVEL"
	EnvMaxEgressRules   = "OPENSANDBOX_EGRESS_MAX_RULES" // max egress rules for POST/PATCH; 0 = unlimited; empty = default
	EnvBlockedWebhook   = "OPENSANDBOX_EGRESS_DENY_WEBHOOK"
	ENVSandboxID        = "OPENSANDBOX_EGRESS_SANDBOX_ID"
	// EnvEgressMetricsExtraAttrs optional comma-separated key=value pairs appended to every egress OTLP metric datapoint (first '=' splits key/value per segment).
	EnvEgressMetricsExtraAttrs = "OPENSANDBOX_EGRESS_METRICS_EXTRA_ATTRS"

	// EnvNameserverExempt comma-separated IPs; proxy upstream to these is not marked and is allowed in nft allow set
	EnvNameserverExempt = "OPENSANDBOX_EGRESS_NAMESERVER_EXEMPT"

	// EnvDNSUpstream comma-separated upstream resolvers; each address must be a literal IPv4/IPv6 (optional :port). Hostnames are rejected (DNS recursion via REDIRECT).
	EnvDNSUpstream = "OPENSANDBOX_EGRESS_DNS_UPSTREAM"

	// EnvDNSUpstreamTimeout is the per-upstream DNS forward timeout in whole seconds (default 5). Invalid/empty uses default.
	EnvDNSUpstreamTimeout = "OPENSANDBOX_EGRESS_DNS_UPSTREAM_TIMEOUT"
)

const (
	PolicyDnsOnly = "dns"
	PolicyDnsNft  = "dns+nft"
)

const (
	DefaultEgressServerAddr = ":18080"
	// ResolvNameserverCap is the max number of nameserver lines read from /etc/resolv.conf for nft allow-list merge and auto upstream chain (not configurable).
	ResolvNameserverCap          = 10
	DefaultMaxEgressRules        = 4096
	DefaultDNSUpstreamTimeoutSec = 5
)
