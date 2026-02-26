# Egress Sidecar Metrics

This document describes the Prometheus metrics exposed by the Egress Sidecar: name, type, description, and optional labels.  
All metrics use the prefix `opensandbox_egress_*` and are exposed via HTTP at `GET /metrics` (same port as the policy server, default `:18080`).

---

## 1. DNS Proxy (Layer 1)

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `opensandbox_egress_dns_queries_total` | Counter | Total DNS queries handled by the proxy, by result. | `result`: `allowed` (policy allowed and forward succeeded), `denied` (policy denied, NXDOMAIN returned), `forward_error` (policy allowed but upstream DNS failed). |
| `opensandbox_egress_dns_forward_duration_seconds` | Histogram / Summary | Latency in seconds of forwarding DNS queries to upstream. | For Summary, `quantile`; for Histogram, default buckets. |

---

## 2. Policy and Runtime

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `opensandbox_egress_policy_updates_total` | Counter | Number of successful policy updates via `POST /policy`. | None. |
| `opensandbox_egress_policy_rule_count` | Gauge | Current number of egress rules in the active policy. | Optional: `default_action` (`allow` / `deny`). |
| `opensandbox_egress_enforcement_mode` | Gauge | Current enforcement mode for observability (OSEP R6). Value is 1; label distinguishes mode. | `mode`: `dns` (DNS proxy only) or `dns+nft` (DNS + nftables). |

---

## 3. nftables (Layer 2, dns+nft mode)

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `opensandbox_egress_nft_apply_total` | Counter | Number of nftables ApplyStatic (static rule apply) operations. | `result`: `success` or `failure`. On failure the sidecar falls back to DNS-only mode. |
| `opensandbox_egress_nft_resolved_ips_added_total` | Counter | Number of resolved IPs added to the nftables dynamic set (count of IPs or invocations, implementation-defined). | Optional: `domain` (use with care to avoid high cardinality). |
| `opensandbox_egress_nft_doh_dot_packets_dropped_total` | Counter | Number of packets dropped due to DoH/DoT blocking. | `reason`: `dot_853` (DoT port 853), `doh_443` (DoH over 443 when enabled). |

---

## 4. Violations and Security (aligned with OSEP R7 / violation logging)

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `opensandbox_egress_violations_total` | Counter | Number of policy denials (e.g. DNS NXDOMAIN). Can be instrumented alongside violation logs. | `type`: `dns_deny`; add e.g. `l2_deny` for L2 denials if implemented. |

---

## 5. Process / Runtime (optional)

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `opensandbox_egress_info` | Gauge | Constant 1; labels identify the instance and environment in Prometheus. | See "Instance identification" below: `instance_id` (recommended), `enforcement_mode`, `version`, etc. |
| `opensandbox_egress_uptime_seconds` | Gauge | Process uptime in seconds. | None. |

---

## Instance identification (keeping metrics per container)

Each sidecar container corresponds to a different sandbox; metrics must be distinguishable per instance and must not be mixed in the same time series. How this works depends on how metrics are collected:

### Instance ID source

Instance identification is **provided only via an environment variable**; the sidecar reads env and does not distinguish K8s vs Docker:

- **Env var**: `OPENSANDBOX_EGRESS_INSTANCE_ID`
- **Meaning**: Unique ID for this sidecar instance (e.g. sandbox_id, pod name, container_id), **injected by the orchestrator when creating the container**.
- **Examples**:
  - Kubernetes: set via Downward API in the Pod, e.g. `OPENSANDBOX_EGRESS_INSTANCE_ID=$(POD_NAME).$(POD_NAMESPACE)` or `$(POD_UID)`.
  - Docker / OpenSandbox server: pass when creating the container, e.g. `-e OPENSANDBOX_EGRESS_INSTANCE_ID=<sandbox_id>`.

Implementation notes:

- Attach the **same set of instance labels** to all metrics: read `OPENSANDBOX_EGRESS_INSTANCE_ID` and use it as the `instance_id` label, consistent with `opensandbox_egress_info`.
- If the env is unset, `instance_id` may be empty or a fallback (e.g. hostname). **When using push, configuring it is strongly recommended**, or multiple instances will share the same grouping key.

---

## Metric types

- **Counter**: Monotonically increasing value; use for request counts, error counts, etc. Prometheus typically uses `rate()` / `increase()` for rate or delta.
- **Gauge**: Current value that can go up or down; use for current rule count, mode, uptime, etc.
- **Histogram**: Bucketed observations (e.g. latency); supports quantiles and rate.
- **Summary**: Quantiles computed in the application and exposed; use for distribution metrics like latency.

---

## Exposure

- **Endpoint**: Same port as the policy server, default `GET http://<addr>/metrics` (e.g. `http://127.0.0.1:18080/metrics`).
- **Format**: Prometheus text format (`text/plain; charset=utf-8`).
- **Collection**: Because the sidecar lifecycle is short, use short-interval scrape from the same Pod or push on exit/periodically (e.g. Pushgateway, OTLP). See [README](README.md) and observability notes.
- **Instance separation**: Metrics from different container instances are separated by the labels defined in "Instance identification" (e.g. `instance_id`) or by scrape target identity; see the "Instance identification" section above.
