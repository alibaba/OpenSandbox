---
title: OpenTelemetry Metrics and Logs (execd, egress, and ingress)
authors:
  - "@Pangjiping"
  - "@ninan-nn"
creation-date: 2026-03-18
last-updated: 2026-03-31
status: draft
---

# OSEP-0010: OpenTelemetry Metrics and Logs (execd, egress, and ingress)

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Requirements](#requirements)
- [Proposal](#proposal)
  - [Notes/Constraints/Caveats](#notesconstraintscaveats)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [1. Metrics](#1-metrics)
    - [1.1 execd metrics](#11-execd-metrics)
    - [1.2 egress metrics](#12-egress-metrics)
    - [1.3 ingress metrics](#13-ingress-metrics)
  - [2. Logging](#2-logging)
    - [2.1 Egress: sandbox outbound access log (hostname / IP)](#21-egress-sandbox-outbound-access-log-hostname--ip)
    - [2.2 Egress: policy initialization and allow/deny rule changes](#22-egress-policy-initialization-and-allowdeny-rule-changes)
  - [3. Initialization and configuration](#3-initialization-and-configuration)
- [Test Plan](#test-plan)
- [Drawbacks](#drawbacks)
- [Infrastructure Needed](#infrastructure-needed)
- [Upgrade & Migration Strategy](#upgrade--migration-strategy)
<!-- /toc -->

## Summary

This proposal introduces unified **OpenTelemetry** instrumentation for OpenSandbox’s three Go components—**execd**, **egress**, and **ingress**—covering **Metrics** and **Logs** only (**no OpenTelemetry Tracing / distributed traces**). With OTLP export and environment-based configuration, operators and developers can observe request rates, latencies, resource usage, policy enforcement, **egress outbound hostname/IP access (via structured logs, not metrics)**, **egress policy load and allow/deny rule changes (structured logs)**, and ingress proxy traffic in production and integrate with existing observability stacks (e.g., Prometheus, Grafana Loki).

## Motivation

Today execd, egress, and ingress have partial observability (e.g., execd’s HTTP API and `GetMetrics`/`WatchMetrics`, zap/loggers in egress and ingress) but lack:

- **Standardized metrics**: No Prometheus/OpenTelemetry-style HTTP QPS, latency, status codes; no unified metrics for execd code execution and Jupyter sessions, egress DNS/policy, or ingress proxy requests and routing.
- **Unified export**: No OTLP endpoint or configuration for metrics and logs, so integration with a central observability platform is difficult.

Adopting OpenTelemetry for **metrics and logs** allows the three components to gain consistent signals without changing core business logic, with the ability to disable export via environment variables for deployments without observability backends.

### Goals

- Integrate the OpenTelemetry SDK (Go) into execd, egress, and ingress to emit **Metrics** and **Logs** via OTLP (and structured stdout logs where applicable).
- **Metrics**: Cover HTTP, code execution, Jupyter, filesystem operations, and system resources (execd); DNS, policy, nftables, and system resources (egress); HTTP/WebSocket proxy requests, routing resolution, status codes, and system resources (ingress).
- **Logging**: Use structured fields on the existing zap logger; for **egress**, **by default** emit **structured log records** for each outbound DNS attempt (hostname and resolved IPs when available), and for **initial egress policy load** and **runtime allow/deny rule updates** (POST/PATCH to the policy API or equivalent), all at **info** unless the deployment lowers the logger level.
- **Configuration**: Provide initialization and support for OTLP exporters and environment variables; default to no export so deployments without observability backends are unaffected.

### Non-Goals

- **OpenTelemetry Tracing** (distributed traces, `TracerProvider`, Jaeger-style request graphs) for execd, egress, or ingress — **out of scope** for this OSEP; correlation relies on **metrics dimensions**, **structured log fields** (e.g. `osbx.id`), and time—not on `trace_id`/`span_id`.
- Do not replace existing execd HTTP metric endpoints such as `GetMetrics`/`WatchMetrics`; they can coexist with OpenTelemetry metrics.
- Do not implement OpenTelemetry on the server (Python) in this proposal; scope is limited to the three Go components (execd, egress, ingress).
- Do not commit to vendor-specific backends (e.g., Datadog, New Relic); export is via the standard OTLP protocol only.
- Do not require a Collector; both direct OTLP and via-Collector export are supported.

## Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| R1 | execd/egress/ingress support exporting **Metrics** and **Logs** via OTLP (**not** Traces) | Must Have |
| R2 | Metrics cover all execd, egress, and ingress metric items listed in this proposal | Must Have |
| R3 | Structured logs include a filterable **sandbox identifier** where applicable (`osbx.id` or equivalent) | Must Have |
| R4 | Configuration via environment variables (endpoint, toggles) without code changes | Must Have |
| R5 | Default or unset OTLP config results in no export to avoid impacting deployments without observability | Should Have |
| R6 | Compatible with existing zap Logger interface; no breaking changes to Logger abstraction | Should Have |
| R7 | Egress can emit structured logs for per–sandbox outbound access (DNS question hostname and, when present, resolved IPs); **not** required as Prometheus-style metrics with hostname/IP as labels | Should Have |
| R8 | Egress emits structured logs when **initial policy** is applied and when **policy rules are added or changed** at runtime; logs include `osbx.id` and a **summary** of the effective rules (see [§2.2](#22-egress-policy-initialization-and-allowdeny-rule-changes)) | Should Have |

## Proposal

Introduce an **OpenTelemetry initialization module** in the main startup of execd, egress, and ingress that:

1. Creates and registers a **MeterProvider** and **MetricReader** (e.g., OTLP metric exporter).
2. Optionally creates a **LoggerProvider** and registers an OTLP log exporter; otherwise rely on zap JSON logs and optional **Logs Bridge** to OTLP.
3. Reads OTLP endpoint, service name, etc., from environment variables (or config files).

Application code records metrics on critical paths. **HTTP metrics** for execd/egress/ingress use **aggregated dimensions** (e.g. `http.route` as route template, `http.status_code`, `method`)—**without** OpenTelemetry spans: implement via **manual instrumentation** or thin middleware that only increments histograms/counters (no `TracerProvider`). Egress and ingress use `net/http`; execd uses Gin—same principle (metrics only, no trace spans).

### Notes/Constraints/Caveats

- OpenTelemetry Go SDK version and stability must match the project’s Go version; prefer the stable API (e.g., `go.opentelemetry.io/otel` v1).
- Metric names and attributes should follow [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/) where practical for compatibility with generic dashboards.
- egress may run as a sidecar in the same Pod as the workload; keep metric export batching configurable to limit sidecar CPU/memory.
- Log enhancements apply only to code paths using the shared Logger; code that uses the standard `log` package is out of scope for this proposal but can be migrated later.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| OTLP export failures or unreachable endpoint cause blocking or retry storms | Use async export, configurable timeouts and queue limits; on failure only log locally and do not affect the main flow |
| High metric cardinality (e.g., per sandbox_id or raw URL path) | Avoid high-cardinality dimensions: only use aggregated dimensions such as status_code, operation; **HTTP metrics must use the route template `http.route`** (e.g. `/code/contexts/:contextId`), not the raw request path, or execd routes with path parameters will produce high-cardinality series that are hard to operate |
| Divergence from existing metrics APIs | Leave existing HTTP metric endpoints unchanged; OpenTelemetry metrics are additive |

## Design Details

### 1. Metrics

#### 1.1 execd metrics

| Category | Metric name (suggested) | Type | Description |
|----------|-------------------------|------|-------------|
| **HTTP** | `execd.http.request.count` | Counter | Request count by method, **http.route (route template)**, status_code (QPS derivable) |
| | `execd.http.request.duration` | Histogram | Request latency (s or ms) by method, **http.route (route template)** |
| **Code execution** | `execd.execution.count` | Counter | Execution count by result (success/failure) |
| | `execd.execution.duration` | Histogram | Duration per execution |
| | `execd.execution.memory_bytes` | Histogram / Gauge | Memory usage during execution (if available) |
| **Jupyter sessions** | `execd.jupyter.sessions.active` | UpDownCounter / Gauge | Current active sessions |
| | `execd.jupyter.sessions.created_total` | Counter | Sessions created |
| | `execd.jupyter.sessions.deleted_total` | Counter | Sessions deleted |
| **Filesystem** | `execd.filesystem.operations.count` | Counter | Operation count by type (upload/download/list/delete, etc.) |
| | `execd.filesystem.operations.duration` | Histogram | Operation duration |
| **System** | `execd.system.cpu.usage` | Gauge | Process or host CPU usage (optional) |
| | `execd.system.memory.usage_bytes` | Gauge | Memory usage |
| | `execd.system.process.count` | Gauge | Current number of processes in the system |

All metrics are created via the OpenTelemetry Meter; units and attributes follow [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/).

**Execd HTTP dimensions:** Several execd routes embed identifiers in the URL (e.g. `/code/contexts/:contextId`, `/session/:sessionId/run`, `/command/status/:id` in `components/execd/pkg/web/router.go`). Using the raw request path as a metric dimension would create high-cardinality time series and make OTLP/Prometheus metrics hard to operate. Therefore **the route template must be used as the dimension**: `http.route` (e.g. `/code/contexts/:contextId`), not the actual request path (e.g. `/code/contexts/abc-123`). Record the matched route pattern from Gin (e.g. `c.FullPath()` or equivalent) in metric attributes—**without** OpenTelemetry tracing middleware.

#### 1.2 egress metrics

| Category | Metric name (suggested) | Type | Description |
|----------|-------------------------|------|-------------|
| **DNS** | `egress.dns.query.duration` | Histogram | Per-query latency |
| | `egress.dns.cache.hits_total` | Counter | Cache hits |
| | `egress.dns.cache.misses_total` | Counter | Cache misses (hit rate = hits / (hits + misses)) |
| **Policy** | `egress.policy.evaluations.count` | Counter | Evaluations by action (allow/deny) |
| | `egress.policy.denied_total` | Counter | Denials; block rate derivable with evaluations |
| **nftables** | `egress.nftables.rules.count` | Gauge | Current rule count |
| | `egress.nftables.updates.count` | Counter | Rule update count (update frequency observable) |
| **System** | `egress.system.cpu.usage` | Gauge | CPU usage |
| | `egress.system.memory.usage_bytes` | Gauge | Memory usage |

**Per–sandbox outbound hostname / IP (monitoring vs logs):** Operators often want a record of **which hostnames or IPs** a sandbox attempted to reach. **Do not** encode raw hostname or per-IP destination as **metric labels** or default metric dimensions: that creates extreme cardinality in Prometheus-style backends and conflicts with the cardinality controls elsewhere in this OSEP. Instead, treat **each DNS egress attempt** (and its outcome) as a **structured log event** (see [§2.1](#21-egress-sandbox-outbound-access-log-hostname--ip)). Aggregated **metrics** above remain suitable for rates, latency, allow/deny counts, and cache behavior without naming every destination.

**Egress `osbx.id` on metrics (and Resource):** For **egress** OpenTelemetry **metrics**, include an **`osbx.id`** **dimension** on exported series, and the same value on the **Resource** when appropriate, using the value from **`OPENSANDBOX_EGRESS_SANDBOX_ID`** when that env var is set. The sidecar is typically **one process per sandbox**; this is the **only** supported mechanism in this OSEP for per-sandbox **metric** aggregation labels—**do not** use a separate `OPENSANDBOX_OTEL_METRICS_EXTRA_ATTRIBUTES`-style hook. When **`OPENSANDBOX_EGRESS_SANDBOX_ID`** is unset, document whether the dimension is omitted or empty.

#### 1.3 ingress metrics

| Category | Metric name (suggested) | Type | Description |
|----------|-------------------------|------|-------------|
| **HTTP** | `ingress.http.request.count` | Counter | Request count by method, status_code, proxy_type (http/websocket) (QPS derivable) |
| | `ingress.http.request.duration` | Histogram | Request duration (including routing and proxy) by method, proxy_type |
| **Routing** | `ingress.routing.resolutions.count` | Counter | Resolutions by result (success/not_found/not_ready/error) |
| | `ingress.routing.resolution.duration` | Histogram | Time to resolve sandbox target (from cache or API) |
| **Proxy type** | `ingress.proxy.http.requests_total` | Counter | HTTP proxy request count |
| | `ingress.proxy.websocket.connections_total` | Counter | WebSocket connection count |
| **System** | `ingress.system.cpu.usage` | Gauge | CPU usage |
| | `ingress.system.memory.usage_bytes` | Gauge | Memory usage |

Note: Ingress typically returns 200 (success), 400 (bad request), 404 (sandbox not found), 502 (upstream error), 503 (sandbox not ready); aggregate by `http.status_code` for error-rate monitoring.

Metric namespaces are `execd.*`, `egress.*`, and `ingress.*` for easy filtering in a shared backend. **execd** and **ingress** obtain sandbox-related identifiers from the **request or routing context** where applicable (not from `OPENSANDBOX_EGRESS_SANDBOX_ID`).

### 2. Logging

**Egress structured log keys** use a short **`osbx.`** prefix (**OpenSandbox**). Shared across [§2.1](#21-egress-sandbox-outbound-access-log-hostname--ip) and [§2.2](#22-egress-policy-initialization-and-allowdeny-rule-changes); rule entries inside **`osbx.rules`** still use **`action`** and **`target`** per API shape.

- **Structured fields**: Use existing `Logger.With(Field{...})` for stable keys (`osbx.id`, component-specific attributes).
- **Context-aware**: Handlers that receive `context.Context` may attach request-scoped fields to logs where useful; **no** requirement for `trace_id`/`span_id` from OpenTelemetry (tracing out of scope).
- **Filter/query by `osbx.id`**: When a request or operation is associated with a sandbox (e.g. execd handling a request for that sandbox, ingress proxying to that sandbox), log records **should** include a filterable **`osbx.id`** so that log backends can filter and query by sandbox for per-sandbox debugging.
- **OTLP Logs**: If OTLP log export is enabled, log records carry the same structured fields for downstream systems (e.g. Loki via Collector).

The existing `Logger` interface (`Infof`, `With`, `Named`) stays unchanged.

**Egress log families (summary):** All egress structured logs use **zap** `With` fields plus a human-readable **`msg`**. **`osbx.id`** comes from **`OPENSANDBOX_EGRESS_SANDBOX_ID`** when set. Two event families:

| Family | `osbx.event` | Typical level | One line per |
|--------|---------------------|---------------|--------------|
| **Outbound access** | **`egress.outbound`** | `info` (default) | Each observed outbound attempt (DNS path and/or IP-only path per [§2.1](#21-egress-sandbox-outbound-access-log-hostname--ip)) |
| **Policy lifecycle** | **`egress.loaded`**, **`egress.updated`**, **`egress.update_failed`** | `info` / `warn` / `error` | Policy load, successful update, or failed update ([§2.2](#22-egress-policy-initialization-and-allowdeny-rule-changes)) |

#### 2.1 Egress: sandbox outbound access log (hostname / IP)

**Purpose:** Support **monitoring and audit** use cases that need “what did this sandbox try to reach?” with **hostname** (DNS question name) and, when applicable, **resolved IP addresses**—without turning every destination into a metric time series.

**Signal type:** **Logs** (structured fields on the existing zap logger, and optionally **OTLP Logs** when a log exporter is enabled). Identify records with **`osbx.event` = `egress.outbound`**; this is **not** a per-destination **Counter/Histogram metric** (per-destination “log metrics” in vendor UIs remain **log-derived**, e.g. Loki queries, not OpenTelemetry Metrics with hostname labels).

**Observation points:** (1) **DNS proxy** — each client DNS query is one “access attempt” to a name (after policy evaluation; after upstream resolution or on forward error). (2) **IP-only path** (e.g. direct connect to a literal IP, or enforcement observed without a DNS name) — log using **`osbx.peer`** instead of DNS-specific fields.

**Recommended fields** for **`osbx.event` = `egress.outbound`** (align with OpenTelemetry semantic conventions where possible):

| Field | Description |
|-------|-------------|
| `osbx.event` | Constant **`egress.outbound`** |
| `osbx.id` | Sandbox identifier from **`OPENSANDBOX_EGRESS_SANDBOX_ID`** when set |
| `osbx.result` | `allow` \| `deny` \| `error` (policy denied, or upstream DNS forward failed) |
| `osbx.host` | Normalized QNAME when the attempt is **name-based** (lowercase, trailing dot stripped). **Omit** when the attempt is **IP-only** (use `osbx.peer` instead). |
| `osbx.ips` | When `osbx.result` is `allow` and the DNS response includes A/AAAA: list of resolved IPs as strings. **Omit or empty** when not applicable (deny path, non-address RR types, or **IP-only** path). |
| `osbx.peer` | Destination **IP address** when the attempt is **IP-only** (no `osbx.host`), or when logging the peer IP is required without a DNS query name. **Omit** when only DNS name + `osbx.ips` describe the destination. |
| `osbx.err` | When `osbx.result` is `error`: short message (e.g. DNS forward failure) |

**Examples (logical JSON shape; zap may add `level`, `ts`, `logger`, `msg`):**

```json
{"osbx.event":"egress.outbound","osbx.id":"sb-abc","osbx.host":"pypi.org","osbx.result":"allow","osbx.ips":["151.101.0.223"]}
```

```json
{"osbx.event":"egress.outbound","osbx.id":"sb-abc","osbx.host":"blocked.example","osbx.result":"deny"}
```

```json
{"osbx.event":"egress.outbound","osbx.id":"sb-abc","osbx.peer":"198.51.100.7","osbx.result":"allow"}
```

**Trace correlation:** **Not applicable** under this OSEP (no distributed traces). **Correlation** uses `osbx.id`, destination fields (`osbx.host` and/or `osbx.peer`), and timestamps.

**Default behavior:** Emit one **info**-level structured record per handled DNS query on this path (**default on**). Under high DNS QPS this can produce **large log volume**; document clearly. Operators who need less verbosity may lower the component log level (e.g. `warn`) via existing egress log configuration—**no** separate env toggle for these events in this OSEP. This is separate from `OTEL_LOGS_EXPORTER`: local structured JSON logs may suffice without OTLP log export.

**Privacy / retention:** Document that full outbound access logs may be sensitive; operators should set retention and access controls in their log backend.

#### 2.2 Egress: policy initialization and allow/deny rule changes

**Purpose:** Provide an **audit trail** for **which egress policy** a sandbox sidecar is enforcing: first load at process start and every **successful** update when operators or the control plane add or change **allow/deny** rules. This is **low frequency** compared to per-DNS [§2.1](#21-egress-sandbox-outbound-access-log-hostname--ip) and uses default **info**-level structured logs (**default on**).

**Signal type:** **Logs** (structured fields; optional OTLP Logs). **Not** metrics with one time series per rule target (same cardinality rationale as §1.2).

**When to emit**

| Event | `osbx.event` (suggested) | When |
|-------|----------------------------------|------|
| **Initial policy load** | `egress.loaded` | Once the effective **initial** policy is known after startup: e.g. from **`OPENSANDBOX_EGRESS_POLICY_FILE`** (if valid), else **`OPENSANDBOX_EGRESS_RULES`**, else built-in default (e.g. deny-all). If the lifecycle server later **POSTs** policy after `/healthz`, emit **`egress.loaded`** when that first server-driven snapshot is applied, or **`egress.updated`** if implementation treats it strictly as an update—choose one convention and document it so operators do not see duplicate semantics. |
| **Runtime rule change** | `egress.updated` | After a **successful** application of new policy via the egress HTTP API (e.g. POST/PATCH **`/policy`** with auth), including additions or modifications of allow/deny rules. On **failure** (4xx/5xx, validation error), log at **warn** or **error** with `osbx.event` e.g. `egress.update_failed` and **no** effective policy change. |

**Recommended fields (common)**

| Field | Description |
|-------|-------------|
| `osbx.event` | `egress.loaded` \| `egress.updated` \| `egress.update_failed` |
| `osbx.id` | From `OPENSANDBOX_EGRESS_SANDBOX_ID` when set |
| `osbx.src` | **loaded**: `policy_file` \| `env` \| `default` \| `server_bootstrap` (or equivalent). **updated**: `http` (API). |
| `osbx.default` | Effective `defaultAction` after apply (e.g. `allow` / `deny`) |
| `osbx.rule_count` | Number of rules in `egress[]` after apply |
| `osbx.err` | On **`egress.update_failed`**: validation or transport message |

**Recommended fields (rule summary — not raw body)**

- Include a compact representation of **allow/deny rules** sufficient for audit: e.g. **`osbx.rules`** as an array of objects `{ "action": "allow|deny", "target": "<string>" }` in **stable order** (e.g. as stored after parse), **or** a **digest** (hash) of the canonical JSON plus **`osbx.rule_count`** if full rule listing is too large for log pipelines.
- Avoid logging **secrets** (policy JSON may embed sensitive hostnames in some deployments); document that **targets** are part of policy and may need redaction in highly regulated environments.

**Examples (logical JSON shape):**

```json
{"osbx.event":"egress.loaded","osbx.id":"sb-abc","osbx.src":"env","osbx.default":"deny","osbx.rule_count":2,"osbx.rules":[{"action":"allow","target":"pypi.org"},{"action":"allow","target":"*.github.com"}]}
```

```json
{"osbx.event":"egress.updated","osbx.id":"sb-abc","osbx.src":"http","osbx.default":"deny","osbx.rule_count":1,"osbx.rules":[{"action":"allow","target":"api.openai.com"}]}
```

```json
{"osbx.event":"egress.update_failed","osbx.id":"sb-abc","osbx.err":"validation failed: rule limit exceeded"}
```

**Trace correlation:** **Not applicable** under this OSEP. For **`egress.updated`**, correlation with the HTTP request is via **HTTP access logs** (e.g. method, path, status) and **time**; for **`egress.loaded`** at startup, resource attributes (`service.name`, pod name) identify the instance.

**Volume:** Policy events are **infrequent** compared to [§2.1](#21-egress-sandbox-outbound-access-log-hostname--ip). Emit **`egress.loaded`** and **`egress.updated`** at **info** when policy changes (**default on**). **No** separate env toggle in this OSEP; deployments that must not log policy content should use log level or pipeline-side filtering.

### 3. Initialization and configuration

- **Initialization**  
  Implement `InitOpenTelemetry(ctx context.Context, opts InitOptions) (shutdown func(), err error)` in main for execd, egress, and ingress (or in a shared `pkg/telemetry`):
  - Create `MeterProvider` and register an OTLP metric exporter (e.g., `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` or gRPC).
  - Optionally create `LoggerProvider` and register an OTLP log exporter; otherwise rely on zap and optional **Logs Bridge**.
  - Set global `otel.SetMeterProvider` (and logger provider if used), and return a `shutdown` function (Flush + ForceFlush) to call on process exit.
  - **Do not** set `TracerProvider` for production use; omit or use a no-op tracer provider.

- **OTLP exporter**  
  Support HTTP and gRPC OTLP endpoints via environment variables:
  - `OTEL_EXPORTER_OTLP_ENDPOINT` (or per-signal `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`, `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`).
  - If unset, do not export or use a Noop provider to avoid connection errors.

- **Environment variables**  
  Support at least (names follow OpenTelemetry conventions where applicable):
  - `OTEL_SERVICE_NAME`: service name (execd / egress / ingress).
  - `OTEL_EXPORTER_OTLP_ENDPOINT` (or per-signal endpoints for metrics/logs).
  - `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER` (e.g., `none` to disable).
  - `OTEL_RESOURCE_ATTRIBUTES`: key-value pairs for resource attributes (e.g., deployment.env);
  - **`OPENSANDBOX_EGRESS_SANDBOX_ID`** (egress): **source of truth** for the **`osbx.id`** value on **egress** OpenTelemetry **metric** dimensions and **Resource** attributes (see [§1.2](#12-egress-metrics)), and for **`osbx.id`** in structured logs ([§2.1](#21-egress-sandbox-outbound-access-log-hostname--ip), [§2.2](#22-egress-policy-initialization-and-allowdeny-rule-changes)). When unset, document omit-vs-empty behavior for metrics and logs.

Egress structured logs for outbound access ([§2.1](#21-egress-sandbox-outbound-access-log-hostname--ip)) and policy audit ([§2.2](#22-egress-policy-initialization-and-allowdeny-rule-changes)) are **default on** at **info**; they are **not** controlled by separate `OPENSANDBOX_*` env vars in this OSEP.

Optionally read some of these from existing config or flags and allow environment variables to override.

## Test Plan

- **Unit tests**
  - Metrics: Create a MeterProvider with an in-memory or mock exporter, run business logic, assert exported metric count and key attributes; for **egress**, assert **`osbx.id`** matches **`OPENSANDBOX_EGRESS_SANDBOX_ID`** when set.
  - Logging: Assert structured log records contain expected fields (e.g. `osbx.id` where applicable).
  - Egress outbound access: Issue DNS queries (allow/deny/error paths); assert **info**-level log records contain `osbx.host`, `osbx.result`, and `osbx.ips` when applicable.
  - Egress policy: Assert `egress.loaded` after startup with expected `policy.source` and rule summary; POST/PATCH policy and assert `egress.updated` (or `egress.update_failed` on invalid body).
- **Integration tests**
  - Start execd/egress/ingress with OTLP endpoint pointing at a test Collector or mock; send HTTP requests and trigger execution/DNS/policy/proxy; verify OTLP payloads contain expected **metrics** and **logs** (no trace spans required).
- **Configuration**
  - When `OTEL_EXPORTER_OTLP_*` is unset, no connection is made and no error is raised.
  - Environment variables override config file where applicable.

Acceptance: With OTLP enabled, Prometheus or the backend shows all execd, egress, and ingress **metrics** listed above; **logs** export or stdout contains structured fields; egress **default** outbound access logs include `osbx.host`, `osbx.result`, and `osbx.ips` when applicable, without adding hostname as a metric label. Egress emits `egress.loaded` for initial policy and `egress.updated` (or documented equivalent) after successful rule changes, with rule summary fields per [§2.2](#22-egress-policy-initialization-and-allowdeny-rule-changes). **No OpenTelemetry Tracing** is required for acceptance.

## Drawbacks

- Additional dependencies and binary size (OpenTelemetry SDK and OTLP exporters for metrics/logs).
- Under high QPS, metrics and log export add some CPU/memory cost; per-query egress outbound logs at **info** can be **high volume**—operators may lower log level or tune pipelines; aggregation dimensions control metric cardinality.

## Infrastructure Needed

- **Go dependencies**
  - `go.opentelemetry.io/otel`
  - `go.opentelemetry.io/otel/sdk`
  - `go.opentelemetry.io/otel/exporters/otlp/...` (metrics and logs, HTTP or gRPC as needed)
   - **No** `go.opentelemetry.io/contrib/.../otelhttp` or `otelgin` **required** for tracing; HTTP metrics may use manual instrumentation or minimal middleware that only records metrics.
- **Runtime**
  - For direct OTLP: a reachable OTLP endpoint (e.g., OpenTelemetry Collector, or an OTLP-capable backend for metrics/logs).
  - For “no export” mode: no extra infrastructure.

## Upgrade & Migration Strategy

- **Backward compatibility**: No changes to existing HTTP metric endpoints or Logger interface; only new initialization and optional env vars. With OpenTelemetry unconfigured, behavior is unchanged.
- **Rollout**
  1. Ship initialization and config code with OTLP endpoint unset (noop).
  2. Enable OTLP in test; verify metrics and logs.
  3. Add metric and log instrumentation in execd/egress/ingress handlers.
  4. Enable in production and tune endpoint as needed.
- **Rollback**: Unset or clear `OTEL_EXPORTER_OTLP_*` to stop export; no code change required.
