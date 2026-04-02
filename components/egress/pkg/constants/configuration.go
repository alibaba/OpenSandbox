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

import (
	"os"
	"strconv"
	"strings"
)

const (
	EnvBlockDoH443      = "OPENSANDBOX_EGRESS_BLOCK_DOH_443"
	EnvDoHBlocklist     = "OPENSANDBOX_EGRESS_DOH_BLOCKLIST" // comma-separated IP/CIDR
	EnvEgressMode       = "OPENSANDBOX_EGRESS_MODE"          // + -separated tokens: dns (required), nft — see ParseEgressMode
	EnvEgressHTTPAddr   = "OPENSANDBOX_EGRESS_HTTP_ADDR"
	EnvEgressToken      = "OPENSANDBOX_EGRESS_TOKEN"
	EnvEgressRules      = "OPENSANDBOX_EGRESS_RULES"
	EnvEgressPolicyFile = "OPENSANDBOX_EGRESS_POLICY_FILE" // optional JSON snapshot; if present and valid, overrides EnvEgressRules at startup
	EnvEgressLogLevel   = "OPENSANDBOX_EGRESS_LOG_LEVEL"
	EnvMaxEgressRules   = "OPENSANDBOX_EGRESS_MAX_RULES" // max egress rules for POST/PATCH; 0 = unlimited; empty = default
	EnvMaxNameservers   = "OPENSANDBOX_EGRESS_MAX_NS"
	EnvBlockedWebhook   = "OPENSANDBOX_EGRESS_DENY_WEBHOOK"
	EnvSandboxID        = "OPENSANDBOX_EGRESS_SANDBOX_ID"
	// EnvEgressMetricsExtraAttrs optional comma-separated key=value pairs appended to every egress OTLP metric datapoint (first '=' splits key/value per segment).
	EnvEgressMetricsExtraAttrs = "OPENSANDBOX_EGRESS_METRICS_EXTRA_ATTRS"

	// EnvNameserverExempt comma-separated IPs; proxy upstream to these is not marked and is allowed in nft allow set
	EnvNameserverExempt = "OPENSANDBOX_EGRESS_NAMESERVER_EXEMPT"

	// Python mitmproxy (mitmdump) transparent mode — Linux + CAP_NET_ADMIN only.
	EnvMitmproxyTransparent = "OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT"
	EnvMitmproxyPort        = "OPENSANDBOX_EGRESS_MITMPROXY_PORT"
	EnvMitmproxyConfDir     = "OPENSANDBOX_EGRESS_MITMPROXY_CONFDIR"
	EnvMitmproxyScript      = "OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT"
)

const (
	PolicyDnsOnly = "dns"
	PolicyDnsNft  = "dns+nft"
)

const (
	DefaultEgressServerAddr = ":18080"
	DefaultMitmproxyPort    = 18081
	DefaultMaxNameservers   = 3
	DefaultMaxEgressRules   = 4096
)

func EnvIntOrDefault(key string, defaultVal int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

func IsTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
