# Egress observability (OpenTelemetry metrics & structured logs)

Egress can send **metrics** to an OTLP HTTP endpoint. **Logs** are structured JSON written by zap (typically **stdout**); they are **not** sent over OTLP. **Distributed tracing** is not implemented.

Design background: [OSEP-0010](../../../oseps/0010-opentelemetry-instrumentation.md).

---

## Configuration

### When metrics are exported

Metrics export is **on** when either of these is non-empty (after trimming spaces):

| Condition |
|-----------|
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` (used if the metrics-specific variable is unset) |

If **neither** is set, no OTLP client is started; DNS/policy behavior is unchanged.

### Environment variables

| Variable | Role |
|----------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP base URL for metrics when `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` is unset (e.g. `http://otel-collector:4318`). |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | If set, used for metrics instead of the shared endpoint. |
| `OPENSANDBOX_EGRESS_SANDBOX_ID` | Optional. Adds **`sandbox_id`** to OTLP resource, **every metric** sample, and the default fields on **all** log lines (same value). |
| `OPENSANDBOX_EGRESS_METRICS_EXTRA_ATTRS` | Optional. Comma-separated **`key=value`** pairs appended to **every metric** and merged into the **root logger** (low-cardinality only). First `=` splits key and value per segment. |
| `OPENSANDBOX_EGRESS_LOG_LEVEL` | Log level for zap (e.g. `info`, `warn`). |
| `OPENSANDBOX_LOG_OUTPUT` | Where zap writes (see shared logger); default is stdout JSON. |

Other `OTEL_EXPORTER_OTLP_*` options (headers, timeout, compression, etc.) follow the [OpenTelemetry SDK environment variable](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/) conventions for the Go HTTP exporter.

**Not** used by this binary for resource naming: `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`. **`service.name`** on the OTLP resource is set to **`opensandbox-egress-<version>`**.

### OTLP misconfiguration

If OTLP initialization fails (e.g. invalid endpoint), egress **continues** to run; metrics export is disabled and a warning is logged.

---

## Resource (OTLP metrics)

| Attribute | Meaning |
|-----------|---------|
| `service.name` | `opensandbox-egress-<version>` |
| `sandbox_id` | Present when `OPENSANDBOX_EGRESS_SANDBOX_ID` is set |

---

## Metrics

**Meter:** `opensandbox/egress`. All instruments below inherit **`sandbox_id`** and **`OPENSANDBOX_EGRESS_METRICS_EXTRA_ATTRS`** when configured.

| Name | Type | Unit | Meaning |
|------|------|------|--------|
| `egress.dns.query.duration` | Histogram | `s` | Time spent on **upstream DNS forward** after policy **allow**. Recorded for success and forward-error paths. **Not** recorded when policy **denies** (no forward). |
| `egress.policy.denied_total` | Counter | — | +1 per DNS query **denied** by policy. |
| `egress.nftables.updates.count` | Counter | — | +1 per successful **nftables** static apply (including retry path) or successful **dynamic resolved-IP** update. |
| `egress.nftables.rules.count` | Observable gauge | `{element}` | **Approximate** rule/set size after the last successful static apply (policy rows + static allow/deny set members). **0** in dns-only mode or before any apply. |
| `egress.system.memory.usage_bytes` | Observable gauge | `By` | Host RAM in use (Linux: from `/proc/meminfo`; **0** on non-Linux). |
| `egress.system.cpu.utilization` | Observable gauge | `1` | CPU busy ratio **0–1** between scrapes (Linux `/proc/stat`). First scrape after start **0**; **0** on non-Linux. |

---

## Structured logs (JSON)

Logs are **not** exported via OTLP. Use **`opensandbox.event`** to filter by event family.

**Always on the root logger when set:** `sandbox_id` and any keys from **`OPENSANDBOX_EGRESS_METRICS_EXTRA_ATTRS`** (aligned with metric dimensions).

### Outbound DNS (`opensandbox.event` = `egress.outbound`)

Emitted on **allow** path (with resolution or forward error). Policy **deny** does not emit this event (see `egress.policy.denied_total`).

| Field | Meaning |
|-------|---------|
| `target.host` | Query name (normalized: lowercase, no trailing dot). |
| `target.ips` | Resolved A/AAAA addresses when present. |
| `peer` | Destination IP when the path is **IP-only** (no hostname). |
| `error` | Short message if upstream DNS forward failed. |

### Policy lifecycle

| `opensandbox.event` | Level | Meaning |
|---------------------|-------|---------|
| `egress.loaded` | info | Initial effective policy is loaded (startup). |
| `egress.updated` | info | Policy API applied successfully; **`rules`** reflects **this request only** (PATCH body, POST/PUT body, or reset). |
| `egress.update_failed` | warn / error | Validation, persistence, or apply failure; includes **`error`**. |

**Additional fields (policy events):**

| Field | Meaning |
|-------|---------|
| `egress.default` | Effective default action after apply (`allow` / `deny`). |
| `rules` | For **`egress.loaded`**: full policy snapshot. For **`egress.updated`**: delta for this request only. |
