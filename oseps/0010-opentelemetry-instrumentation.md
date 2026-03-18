---
title: OpenTelemetry Instrumentation (execd, egress, and ingress)
authors:
  - "@Pangjiping"
creation-date: 2026-03-18
last-updated: 2026-03-18
status: draft
---

# OSEP-0010: OpenTelemetry Instrumentation (execd, egress, and ingress)

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
  - [3. Tracing](#3-tracing)
  - [4. Initialization and configuration](#4-initialization-and-configuration)
- [Test Plan](#test-plan)
- [Drawbacks](#drawbacks)
- [Infrastructure Needed](#infrastructure-needed)
- [Upgrade & Migration Strategy](#upgrade--migration-strategy)
<!-- /toc -->

## Summary

This proposal introduces unified **OpenTelemetry** instrumentation for OpenSandbox’s three Go components—**execd**, **egress**, and **ingress**—covering **Metrics**, **Logs**, and **Distributed Traces**. With OTLP export, configurable sampling, and environment-based configuration, operators and developers can observe request flows, resource usage, policy enforcement, and ingress proxy traffic in production and integrate with existing observability stacks (e.g., Jaeger, Prometheus, Grafana Loki).

## Motivation

Today execd, egress, and ingress have partial observability (e.g., execd’s HTTP API and `GetMetrics`/`WatchMetrics`, zap/loggers in egress and ingress) but lack:

- **Standardized metrics**: No Prometheus/OpenTelemetry-style HTTP QPS, latency, status codes; no unified metrics for execd code execution and Jupyter sessions, egress DNS/policy, or ingress proxy requests and routing.
- **Distributed tracing**: No way to correlate requests, code execution, DNS lookups, policy evaluation, and ingress proxy forwarding in a single trace.
- **Log–trace correlation**: Logs do not include `trace_id`/`span_id`, making it hard to jump from logs to traces.
- **Unified export**: No OTLP endpoint or sampling configuration, so integration with a central observability platform is difficult.

Adopting OpenTelemetry allows the three components to gain consistent metrics, logs, and tracing without changing core logic, with the ability to disable or tune sampling via environment variables for production.

### Goals

- Integrate the OpenTelemetry SDK (Go) into execd, egress, and ingress to emit **Metrics**, **Logs**, and **Traces**.
- **Metrics**: Cover HTTP, code execution, Jupyter, filesystem operations, and system resources (execd); DNS, policy, nftables, and system resources (egress); HTTP/WebSocket proxy requests, routing resolution, status codes, and system resources (ingress).
- **Logging**: Extend the existing zap logger to automatically add `trace_id` and `span_id`, with context-aware logging.
- **Tracing**: Instrument key paths (HTTP requests, code execution, DNS lookups, policy evaluation, ingress proxy and routing) with spans.
- **Configuration**: Provide full initialization and support for OTLP exporters, sampling, and environment variables; default to no export or low sampling so deployments without observability backends are unaffected.

### Non-Goals

- Do not replace existing execd HTTP metric endpoints such as `GetMetrics`/`WatchMetrics`; they can coexist with OpenTelemetry metrics.
- Do not implement OpenTelemetry on the server (Python) in this proposal; scope is limited to the three Go components (execd, egress, ingress).
- Do not commit to vendor-specific backends (e.g., Datadog, New Relic); export is via the standard OTLP protocol only.
- Do not require a Collector; both direct OTLP and via-Collector export are supported.

## Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| R1 | execd/egress/ingress support exporting Metrics, Logs, and Traces via OTLP | Must Have |
| R2 | Metrics cover all execd, egress, and ingress metric items listed in this proposal | Must Have |
| R3 | Logs automatically include trace_id and span_id, with values taken from context | Must Have |
| R4 | Key paths (HTTP, code execution, DNS, policy, ingress proxy) have trace spans | Must Have |
| R5 | Configuration via environment variables (endpoint, sampling, toggles) without code changes | Must Have |
| R6 | Default or unset config results in no export or low sampling to avoid impacting deployments without observability | Should Have |
| R7 | Compatible with existing zap Logger interface; no breaking changes to Logger abstraction | Should Have |

## Proposal

Introduce an **OpenTelemetry initialization module** in the main startup of execd, egress, and ingress that:

1. Creates and registers a **MeterProvider** and **MetricReader** (e.g., OTLP exporter).
2. Creates a **TracerProvider** with a sampler such as **TraceIDRatioBased** and registers an OTLP trace exporter.
3. Optionally sets up a **LoggerProvider** or **zap enhancement** so that log fields include trace/span information.
4. Reads OTLP endpoint, sampling rate, service name, etc., from environment variables (or config files).

Application code records metrics and spans on critical paths and, when logging, extracts the current span’s trace_id/span_id from `context.Context` into zap fields. Metrics, logs, and traces then align semantically and can be exported to the same observability platform via OTLP. Egress and ingress both use the standard library `net/http` (egress for the policy API ServeMux, ingress for the proxy Handler); wrap the `Handler` or use middleware such as otelhttp to create a span and context per request. Execd uses Gin and can use the otelgin middleware.

### Notes/Constraints/Caveats

- OpenTelemetry Go SDK version and stability must match the project’s Go version; prefer the stable API (e.g., `go.opentelemetry.io/otel` v1).
- Metric and span names should follow OpenTelemetry semantic conventions (e.g., HTTP attributes, metric units) for compatibility with generic dashboards.
- egress may run as a sidecar in the same Pod as the workload; keep sampling and export batching configurable to limit sidecar CPU/memory.
- Log enhancements apply only to code paths using the shared Logger; code that uses the standard `log` package is out of scope for this proposal but can be migrated later.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| OTLP export failures or unreachable endpoint cause blocking or retry storms | Use async export, configurable timeouts and queue limits; on failure only log locally and do not affect the main flow |
| High sampling rate produces too much trace data | Default to low or no sampling; configure via environment; recommend ≤ 0.1 in production |
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

**Execd HTTP dimensions:** Several execd routes embed identifiers in the URL (e.g. `/code/contexts/:contextId`, `/session/:sessionId/run`, `/command/status/:id` in `components/execd/pkg/web/router.go`). Using the raw request path as a metric dimension would create high-cardinality time series and make OTLP/Prometheus metrics hard to operate. Therefore **the route template must be used as the dimension**: `http.route` (e.g. `/code/contexts/:contextId`), not the actual request path (e.g. `/code/contexts/abc-123`). Gin and middleware such as otelgin should be configured to record the matched route pattern as `http.route`.

#### 1.2 egress metrics

| Category | Metric name (suggested) | Type | Description |
|----------|-------------------------|------|-------------|
| **DNS** | `egress.dns.queries.count` | Counter | DNS query count (QPS derivable) |
| | `egress.dns.query.duration` | Histogram | Per-query latency |
| | `egress.dns.cache.hits_total` | Counter | Cache hits |
| | `egress.dns.cache.misses_total` | Counter | Cache misses (hit rate = hits / (hits + misses)) |
| **Policy** | `egress.policy.evaluations.count` | Counter | Evaluations by action (allow/deny) |
| | `egress.policy.denied_total` | Counter | Denials; block rate derivable with evaluations |
| **nftables** | `egress.nftables.rules.count` | Gauge | Current rule count |
| | `egress.nftables.updates.count` | Counter | Rule update count (update frequency observable) |
| **System** | `egress.system.cpu.usage` | Gauge | CPU usage |
| | `egress.system.memory.usage_bytes` | Gauge | Memory usage |

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

Metric namespaces are `execd.*`, `egress.*`, and `ingress.*` for easy filtering in a shared backend.

**Custom metric dimensions (env):** Provide an env-based hook so users can define **extra metric dimensions** (not limited to sandbox_id). For example, support **`OPENSANDBOX_OTEL_METRICS_EXTRA_ATTRIBUTES`** (or equivalent): a comma-separated list of attribute names (e.g. `sandbox_id`, `tenant_id`, or custom keys). When recording metrics, if the current context or request carries those attributes, they are reported as dimensions on that data point; when unset or empty, no extra dimensions are added. This lets users opt in to “aggregate by sandbox_id” or any custom dimension and accept the cardinality and cost. Implementations must document that this option increases cardinality and should be used only when entity count is bounded.

### 2. Logging

- **Zap enhancement**: In `components/internal/logger` (zap implementation), add the ability to read the current span’s `TraceID` and `SpanID` from `context.Context` and inject them as zap fields, e.g.:
  - Add `LoggerWithContext(ctx context.Context) Logger`, or at call sites use `logger.With(Field{Key: "trace_id", Value: trace.SpanFromContext(ctx).SpanContext().TraceID().String()})` (and similarly for span_id).
- **Context-aware**: Handlers and middleware that receive `context.Context` should use a logger that has trace/span injected so all logs for the same request share the same trace_id.
- **Filter/query by sandbox_id**: When a request or operation is associated with a sandbox (e.g. execd handling a request for that sandbox, ingress proxying to that sandbox), log records **must** include a filterable sandbox identifier (recommend a consistent attribute name such as `opensandbox.sandbox_id` or `sandbox_id`) so that log backends can filter and query by sandbox_id for per-sandbox debugging.
- **Correlation**: If OTLP Logs are used, log records can carry trace_id/span_id and link to the Traces backend for “click from log to trace” workflows.

Implementation options:

- A zap `Core` or `Hook` that reads span from `context.Context` and adds fields (requires middleware to propagate context with span through the request path).
- A `log.Ctx(ctx).Infof(...)`-style helper that gets span from ctx and calls zap.

The existing `Logger` interface (`Infof`, `With`, `Named`) stays unchanged; only context-based construction or trace-field helpers are added.

### 3. Tracing

- **HTTP (execd: Gin)**
  execd uses Gin (`components/execd/pkg/web/router.go`). Register OpenTelemetry HTTP middleware (e.g., `otelgin`) on its routes so each request gets a span with `http.method`, `http.route`, `http.status_code`, etc. When the request is associated with a sandbox (e.g. API call for that sandbox), the span **must** include `sandbox_id` (or a consistent name such as `opensandbox.sandbox_id`) so that traces can be filtered and queried by sandbox_id in Jaeger and similar backends. Pass the request’s context downstream so business logic and logging use the same trace.

- **HTTP (egress: net/http)**
  egress exposes its policy API from a net/http `ServeMux` (`components/egress/policy_server.go`), not Gin. Instrument egress’s HTTP entry points the same way as ingress: wrap the `http.Handler` or use net/http-compatible middleware (e.g., `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`) to create a span per request and pass context with span downstream, so that R1/R4 are met for egress HTTP.

- **ingress HTTP**  
  Ingress uses `net/http`. Wrap the `http.Handler` (or use middleware) to create a root span per request (e.g., `ingress.proxy`) with `http.method`, `http.route`, `http.status_code`, `ingress.mode` (header/uri). When the request is routed to a sandbox, the span **must** include `sandbox_id` (or `opensandbox.sandbox_id`) so that traces can be filtered and queried by sandbox_id. Pass context with span from `ServeHTTP` into the proxy for logs and child spans.

- **ingress proxy forwarding**  
  When forwarding to the target sandbox, create a child span (e.g., `ingress.forward`) with target host and proxy_type (http/websocket). Sandbox resolution (sandbox_id → backend address from sandbox provider) can be a separate child span (e.g., `ingress.resolve`) with attribute resolution_result (success/not_found/not_ready/error) to distinguish 404/503/502 in the trace.

- **Code execution**  
  At execd’s execution entry (e.g., `ExecuteCode`/run), create a child span such as `execution.run` with attributes like `execd.operation=execute` and result. If there are multiple steps (prepare, run, cleanup), add child spans per step.

- **DNS query**  
  In egress DNS proxy, create a span per query (e.g., `dns.query`) with domain, result (allow/deny), cache hit/miss.

- **Policy evaluation**  
  In egress policy evaluation, create a span (e.g., `policy.evaluate`) with target (domain/IP) and action (allow/deny).

All spans are children of the HTTP request span when entered via HTTP, so the full call tree is visible in UIs like Jaeger.

### 4. Initialization and configuration

- **Initialization**  
  Implement `InitOpenTelemetry(ctx context.Context, opts InitOptions) (shutdown func(), err error)` in main for execd, egress, and ingress (or in a shared `pkg/telemetry`):
  - Create `MeterProvider` and register an OTLP metric exporter (e.g., `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` or gRPC).
  - Create `TracerProvider` with a `TraceIDRatioBased` sampler and register an OTLP trace exporter.
  - Optionally create `LoggerProvider` and register an OTLP log exporter; otherwise rely on zap enhancement and the standard Logs Bridge.
  - Set global `otel.SetMeterProvider`, `otel.SetTracerProvider`, etc., and return a `shutdown` function (Flush + ForceFlush) to call on process exit.

- **OTLP exporter**  
  Support HTTP and gRPC OTLP endpoints via environment variables:
  - `OTEL_EXPORTER_OTLP_ENDPOINT` (or per-signal `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`, `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`).
  - If unset, do not export or use a Noop provider to avoid connection errors.

- **Sampling**  
  - Use `OTEL_TRACES_SAMPLER_ARG` (0.0–1.0 for ratio sampler).
  - Or `OTEL_TRACES_SAMPLER=parentbased_traceidratio` with `OTEL_TRACES_SAMPLER_ARG=0.1`.

- **Environment variables**  
  Support at least (names follow OpenTelemetry conventions):
  - `OTEL_SERVICE_NAME`: service name (execd / egress / ingress).
  - `OTEL_EXPORTER_OTLP_ENDPOINT` (or per-signal endpoints).
  - `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`.
  - `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER` (e.g., `none` to disable).
  - `OTEL_RESOURCE_ATTRIBUTES`: key-value pairs for resource attributes (e.g., deployment.env);
  - **`OPENSANDBOX_OTEL_METRICS_EXTRA_ATTRIBUTES`**: comma-separated list of **custom metric dimension** attribute names (e.g. `sandbox_id`, `tenant_id`, or custom keys). When recording metrics, if the context or request carries an attribute with that name, it is added as an extra dimension on the data point; when unset or empty, no extra dimensions are added. This allows opt-in “aggregate by sandbox_id” or other custom dimensions; users assume cardinality and cost. Document that this increases cardinality and is best when entity count is bounded.

Optionally read some of these from existing config or flags and allow environment variables to override.

## Test Plan

- **Unit tests**
  - Metrics: Create a MeterProvider with an in-memory or mock exporter, run business logic, assert exported metric count and key attributes.
  - Logging: Build context with a span, call LoggerWithContext and log, assert output contains trace_id and span_id.
  - Tracing: Use sdktrace.NewTracerProvider with a SpanRecorder or in-memory exporter, run one request flow, assert span names and parent-child relationships.
- **Integration tests**
  - Start execd/egress/ingress with OTLP endpoint pointing at a test Collector or mock; send HTTP requests and trigger execution/DNS/policy/proxy; verify OTLP payloads contain expected metrics and traces.
- **Configuration**
  - When `OTEL_EXPORTER_OTLP_*` is unset, no connection is made and no error is raised.
  - When sampling rate is 0, no spans are produced.
  - Environment variables override config file where applicable.

Acceptance: With OTLP enabled and sampling configured, Jaeger shows full traces (HTTP → execution/DNS/policy/ingress proxy); Prometheus or the backend shows all execd, egress, and ingress metrics listed above; log lines include trace_id/span_id that link to traces.

## Drawbacks

- Additional dependencies and binary size (OpenTelemetry SDK and OTLP exporters).
- Under high QPS, even with low sampling, tracing and metrics add some CPU/memory cost; control via sampling and aggregation dimensions.
- Correct log–trace correlation requires passing `context.Context` through the call chain; some legacy code may need small changes.

## Infrastructure Needed

- **Go dependencies**
  - `go.opentelemetry.io/otel`
  - `go.opentelemetry.io/otel/sdk`
  - `go.opentelemetry.io/otel/exporters/otlp/...` (metrics/traces/logs, HTTP or gRPC as needed)
  - Optional: `go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin` (execd only); egress and ingress use net/http and require wrapping the Handler with e.g. `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`.
- **Runtime**
  - For direct OTLP: an reachable OTLP endpoint (e.g., OpenTelemetry Collector, Jaeger, or an OTLP-capable backend).
  - For “no export” mode: no extra infrastructure.

## Upgrade & Migration Strategy

- **Backward compatibility**: No changes to existing HTTP metric endpoints or Logger interface; only new initialization and optional env vars. With OpenTelemetry unconfigured, behavior is unchanged.
- **Rollout**
  1. Ship initialization and config code with OTLP endpoint unset (noop).
  2. Enable OTLP and low sampling in test; verify metrics and traces.
  3. Add metric and span instrumentation in execd/egress/ingress handlers and zap trace injection.
  4. Enable in production and tune sampling and endpoint as needed.
- **Rollback**: Unset or clear `OTEL_EXPORTER_OTLP_*` to stop export; no code change required.
