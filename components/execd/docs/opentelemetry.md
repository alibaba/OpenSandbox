# OpenTelemetry Metrics (Current Execd Support)

This page lists the OpenTelemetry metrics currently implemented in execd.

## Meter

- `opensandbox/execd`

## Metrics

| Metric | Type | Unit | Attributes | Meaning |
|---|---|---|---|---|
| `execd.http.request.duration` | Histogram | `ms` | `http_method`, `http_route`, `http_status_code` | HTTP request latency. |
| `execd.execution.duration` | Histogram | `ms` | `operation`, `result` | Code/command execution duration. |
| `execd.filesystem.operations.duration` | Histogram | `ms` | `operation`, `result` | Filesystem operation duration. |
| `execd.system.process.count` | Observable Gauge | - | - | Current process count. |
| `execd.system.cpu.usage` | Observable Gauge | `%` | - | System CPU usage percent (gopsutil). |
| `execd.system.memory.usage_bytes` | Observable Gauge | `By` | - | System memory used bytes (gopsutil). |
| `execd.system.network.io.bytes` | Observable Counter | `By` | `direction` (`in`/`out`) | Cumulative network I/O bytes. |
| `execd.system.network.connections.active` | Observable Gauge | - | `protocol` (`tcp`/`udp`) | Current active network connections. |
| `execd.system.oom.kills` | Counter | `{kill}` | `level` (`local`/`descendant`, cgroup v2 only) | OOM kills detected in the sandbox cgroup (see below). |
| `execd.isolation.run.duration` | Histogram | `ms` | `result` | Isolated session run duration. |
| `execd.isolation.session.count` | Observable Gauge | - | - | Current number of active isolated sessions. |
| `execd.isolation.upper.usage_bytes` | Observable Gauge | `By` | - | Total bytes used by isolated session upper directories. |

## OOM Kill Observation

execd polls its own cgroup every 5 seconds and diffs the kernel OOM kill
counter: `memory.events` (`oom_kill`) on cgroup v2, `memory.oom_control` on
cgroup v1. Each increment is reported twice:

- Metric: `execd.system.oom.kills` increments with `level=local` for kills of
  processes in the sandbox cgroup itself and `level=descendant` for kills in
  nested cgroups (e.g. isolated sessions). The v2 counters are hierarchical,
  so the whole sandbox cgroup subtree is covered — including user/business
  processes, not only the sandbox main process. cgroup v1 reports a single
  increment without `level`.
- Log: one structured line per detection with the kill count, cgroup path,
  and troubleshooting context:
  - `memory=<used>/<limit> peak=<peak>` — where memory stood against the
    cgroup limit when the detection fired
  - `events=[oom=N oom_kill=N oom_group_kill=N]` — deltas of the
    memory.events sub-counters (cgroup v2 only)
  - `kill_cgroups=a,b` — first-level child cgroups whose subtree received
    the kills (any nested cgroup inside the sandbox: isolated sessions,
    user- or system-created cgroups)
  - `exited=[pid=N comm=x ppid=N rss=N age=Ns, ...]` — processes that
    disappeared from `/proc` between the polls surrounding the event
    (candidates for victims; bounded by the kill count), with details
    captured while they were alive, capped at 5 entries

Kernel logs (dmesg) are intentionally not used: the sandbox container has no
`CAP_SYSLOG` and the kernel ring buffer is not sandbox-scoped.

On warm sandboxes rebound by `POST /internal/init`, the counter interval that
crosses a binding handoff is dropped instead of being attributed to the new
sandbox identity. The operator-facing guide lives at
`docs/guides/component-telemetry.md` in the repository root.

## Shared Attributes

All execd metrics may include shared attributes:

- `sandbox_id` from `OPENSANDBOX_ID` (when set)
- extra key/value attributes from `OPENSANDBOX_EXECD_METRICS_EXTRA_ATTRS` (when set)

## OTEL Endpoint Configuration

Metric export is enabled only when at least one OTLP endpoint is set.

- `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` (preferred)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (fallback)

If both are unset, execd keeps metrics local (no OTLP export).

### Minimal Example

```bash
export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT="http://otel-collector:4318"
```

### Optional Service Name

```bash
export OTEL_SERVICE_NAME="opensandbox-execd"
```
