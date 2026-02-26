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

package metrics

import (
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
)

const (
	namespace = "opensandbox"
	subsystem = "egress"
)

var (
	// instanceID is set once at init from OPENSANDBOX_EGRESS_INSTANCE_ID or hostname.
	instanceID string
	// startTime is used for uptime_seconds.
	startTime time.Time
)

func init() {
	if v := os.Getenv(constants.EnvEgressInstanceID); v != "" {
		instanceID = v
	} else {
		hostname, _ := os.Hostname()
		instanceID = hostname
	}
	startTime = time.Now()
}

// InstanceID returns the instance identifier for this sidecar (from env or hostname).
func InstanceID() string { return instanceID }

func constLabels() prometheus.Labels {
	return prometheus.Labels{"instance_id": instanceID}
}

// DNS layer (Layer 1)
var (
	// DNSQueriesTotal counts DNS queries by result: allowed, denied, forward_error.
	DNSQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "dns_queries_total",
			Help:        "Total DNS queries handled by the proxy, by result.",
			ConstLabels: constLabels(),
		},
		[]string{"result"},
	)

	// DNSForwardDurationSeconds is the latency of upstream DNS forward.
	DNSForwardDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "dns_forward_duration_seconds",
			Help:        "Latency of forwarding DNS queries to upstream.",
			ConstLabels: constLabels(),
			Buckets:     prometheus.DefBuckets,
		},
	)
)

// Policy and runtime
var (
	// PolicyUpdatesTotal counts successful POST /policy updates.
	PolicyUpdatesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "policy_updates_total",
			Help:        "Total number of successful policy updates via POST /policy.",
			ConstLabels: constLabels(),
		},
	)

	// PolicyRuleCount is the current number of egress rules in the active policy.
	PolicyRuleCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "policy_rule_count",
			Help:        "Current number of egress rules in the active policy.",
			ConstLabels: constLabels(),
		},
		[]string{"default_action"},
	)

	// EnforcementMode is 1 for the current mode (label: mode=dns or dns+nft).
	EnforcementMode = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "enforcement_mode",
			Help:        "Current enforcement mode (1 for the active mode).",
			ConstLabels: constLabels(),
		},
		[]string{"mode"},
	)
)

// nftables (Layer 2)
var (
	// NftApplyTotal counts nftables ApplyStatic calls by result: success, failure.
	NftApplyTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "nft_apply_total",
			Help:        "Total number of nftables static rule apply operations.",
			ConstLabels: constLabels(),
		},
		[]string{"result"},
	)

	// NftResolvedIPsAddedTotal counts IPs added to nftables dynamic set from DNS.
	NftResolvedIPsAddedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "nft_resolved_ips_added_total",
			Help:        "Total number of resolved IPs added to nftables dynamic allow set.",
			ConstLabels: constLabels(),
		},
	)

	// NftDohDotPacketsDroppedTotal counts packets dropped by DoH/DoT blocking.
	NftDohDotPacketsDroppedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "nft_doh_dot_packets_dropped_total",
			Help:        "Total packets dropped due to DoH/DoT blocking.",
			ConstLabels: constLabels(),
		},
		[]string{"reason"},
	)
)

// Violations (R7)
var (
	// ViolationsTotal counts policy denials (e.g. DNS NXDOMAIN).
	ViolationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "violations_total",
			Help:        "Total number of policy violations (e.g. DNS denied).",
			ConstLabels: constLabels(),
		},
		[]string{"type"},
	)
)

// Process / runtime info
var (
	// EgressInfo is 1 with labels identifying this instance (enforcement_mode, version).
	EgressInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "info",
			Help:        "Info metric with labels for instance and environment.",
			ConstLabels: constLabels(),
		},
		[]string{"enforcement_mode", "version"},
	)

	// UptimeSeconds is process uptime in seconds (updated on each scrape via GaugeFunc).
	UptimeSeconds = promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "uptime_seconds",
			Help:        "Process uptime in seconds.",
			ConstLabels: constLabels(),
		},
		func() float64 { return time.Since(startTime).Seconds() },
	)
)

// Result label values for DNS and nft.
const (
	ResultAllowed      = "allowed"
	ResultDenied       = "denied"
	ResultForwardError = "forward_error"
	ResultSuccess      = "success"
	ResultFailure      = "failure"
)

// Violation type label values.
const (
	ViolationTypeDNSDeny = "dns_deny"
)

// DoH/DoT drop reason label values.
const (
	ReasonDot853 = "dot_853"
	ReasonDoh443 = "doh_443"
)

// Version may be set at build time (-ldflags).
var Version = "0.0.0"

// SetEnforcementMode sets the current mode for opensandbox_egress_enforcement_mode and opensandbox_egress_info.
// Should be called once from main after mode is known.
func SetEnforcementMode(mode string) {
	EnforcementMode.Reset()
	EnforcementMode.WithLabelValues(mode).Set(1)
	EgressInfo.Reset()
	EgressInfo.WithLabelValues(mode, Version).Set(1)
}

// SetPolicyRuleCount updates the policy_rule_count gauge for the given default_action.
// Call when policy is loaded or updated.
func SetPolicyRuleCount(defaultAction string, ruleCount int) {
	PolicyRuleCount.Reset()
	PolicyRuleCount.WithLabelValues(defaultAction).Set(float64(ruleCount))
}
