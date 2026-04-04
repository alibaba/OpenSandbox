---
name: troubleshoot-sandbox
description: Use OpenSandbox diagnostics to investigate failed, unhealthy, or unreachable sandboxes. Trigger when users report startup failures, crashes, OOM, image pull problems, pending sandboxes, network issues, or an unresponsive sandbox and want root cause plus next actions.
---

# OpenSandbox Troubleshooting

Investigate the reported sandbox before proposing a fix. Prefer evidence from OpenSandbox state, diagnostics, and logs over speculation.

## Inputs To Collect

Capture these from the user request or surrounding context before running commands:

- sandbox ID or an unambiguous short ID
- whether `osb` or `opensandbox` CLI is available locally
- any reported symptom: pending forever, crash, OOM, unreachable service, bad image, failed exec, etc.

If the sandbox ID is missing, ask for it first.

## Configuration Resolution

Before troubleshooting a sandbox, resolve the active OpenSandbox connection configuration:

```bash
osb config show
```

Check the resolved values for:

- `domain`
- `api_key`
- `protocol`
- `use_server_proxy` when endpoint routing or proxy behavior may matter

Do not reduce this step to checking environment variables only. Use the resolved configuration that the CLI will actually apply after combining CLI flags, environment variables, config file values, and defaults.

After reading the configuration:

- tell the user which server and protocol the CLI is currently pointed at
- confirm whether they want to keep the current configuration, temporarily override it, or persist new values
- if they want a persistent change, use `osb config set connection.<field> ...`
- if they only want a one-off override, use CLI flags such as `--domain`, `--api-key`, or `--protocol`
- use raw HTTP only after domain, protocol, and API key expectations are explicit

## Operating Rules

- Start with the highest-signal commands first: sandbox state, sandbox health, then diagnostics summary.
- Use CLI commands when `osb` is available because they are shorter and usually already authenticated.
- Use HTTP only when the CLI is unavailable or the user is clearly working from raw API access.
- Do not ask the user to manually inspect logs before you do the diagnostics yourself.
- Distinguish observed facts from inference. Quote the specific field, event, or log line that supports the diagnosis.
- Separate sandbox/runtime failures from workload/application failures before suggesting a fix.
- Do not recommend `--skip-health-check` as a first response to startup failures. Diagnose why readiness is failing first.
- End with a likely root cause and 1-3 concrete remediation steps.

## Triage Model

Use this skill to answer one question at a time:

1. Does the sandbox exist and what state is it in now
2. Is the sandbox healthy and reachable
3. Is the failure in scheduling/runtime/image startup or in the workload running inside the sandbox
4. What evidence supports the likely root cause

Treat these evidence buckets differently:

- sandbox state and health for object existence, lifecycle state, and basic reachability
- diagnostics events for scheduling, image pull, restart, and probe failures
- diagnostics inspect for exit code, OOM status, resources, ports, and runtime details
- diagnostics logs for application startup errors, stack traces, missing binaries, and bad entrypoints

## Golden Path

### 1. Confirm sandbox state

CLI:

```bash
osb sandbox get <sandbox-id>
```

HTTP:

```bash
curl http://<server-domain>/v1/sandboxes/<sandbox-id>
```

If the server requires an API key:

```bash
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  http://<server-domain>/v1/sandboxes/<sandbox-id>
```

Check whether the sandbox is `Pending`, `Running`, `Paused`, `Failed`, or missing entirely. If it is missing, say that clearly instead of continuing with generic troubleshooting.

### 2. Check sandbox health

CLI:

```bash
osb sandbox health <sandbox-id>
```

Use health before deep diagnostics when the sandbox exists but the user reports startup failures, unresponsive exec, or an unreachable service. A sandbox can exist while still failing readiness.

### 3. Pull the diagnostics summary

CLI:

```bash
osb devops summary <sandbox-id>
```

HTTP:

```bash
curl http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/summary
```

With API key:

```bash
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/summary
```

Use the summary as the default first diagnostic because it combines inspect output, events, and recent logs in one response.

### 4. Drill down only where the summary points

CLI:

```bash
osb devops inspect <sandbox-id>
osb devops events <sandbox-id> --limit 100
osb devops logs <sandbox-id> --tail 500
osb devops logs <sandbox-id> --since 30m
```

HTTP:

```bash
curl http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/inspect
curl "http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/events?limit=100"
curl "http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/logs?tail=500"
curl "http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/logs?since=30m"
```

With API key:

```bash
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/inspect
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  "http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/events?limit=100"
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  "http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/logs?tail=500"
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  "http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/logs?since=30m"
```

Use:

- `inspect` for container state, exit code, restart count, resources, ports, and pod-level details
- `events` for scheduler failures, image pull issues, OOM kills, restarts, and probe failures
- `logs` for application errors, missing binaries, bad entrypoints, startup hangs, and health-check failures

## Symptom To Command Mapping

Use the first command that best matches the reported symptom:

| Symptom | First command | What to confirm next |
| --- | --- | --- |
| pending forever or stuck creating | `osb devops events <sandbox-id> --limit 100` | image pull errors, scheduling failures, admission errors |
| image pull failure | `osb devops events <sandbox-id> --limit 100` | image name, tag, registry auth |
| crash loop or repeated restarts | `osb devops logs <sandbox-id> --tail 200` | `osb devops inspect <sandbox-id>` for exit code and restart count |
| suspected OOM or exit code issue | `osb devops inspect <sandbox-id>` | `OOMKilled`, exit code, resource limits |
| endpoint unreachable or connection refused | `osb sandbox health <sandbox-id>` | `osb sandbox endpoint <sandbox-id> --port <port>` and then `osb devops logs <sandbox-id> --tail 200` |
| outbound network access failure | `osb sandbox health <sandbox-id>` | check service behavior, then switch to `network-egress` if the issue is egress policy related |

## Evidence Buckets

Map evidence to the most likely category:

| Evidence | Likely cause | Next step |
| --- | --- | --- |
| `ImagePullBackOff`, `ErrImagePull`, auth errors in events | image name or registry credential problem | verify image reference and registry access |
| `OOMKilled=true`, exit code `137`, memory kill events | memory limit too low | increase memory allocation and inspect workload peak |
| exit code `126` or `127` | command missing or not executable | fix entrypoint path, permissions, or image contents |
| repeated restarts or `CrashLoopBackOff` | app crashes on boot | inspect logs around startup and fix app/config |
| sandbox `Running` but endpoint refused | service not listening or wrong port | confirm process startup and exposed port |
| pending with scheduling warnings | cluster capacity or scheduling constraint | inspect node resources, selectors, tolerations |
| probe failures or execd errors | sidecar/daemon or health-check issue | inspect logs and configuration for execd/probes |

## Diagnosis Playbooks

### Image pull failure

- First evidence: `events` shows `ImagePullBackOff`, `ErrImagePull`, or auth failures
- Confirming evidence: sandbox stays `Pending` or never reaches healthy state
- Likely cause: bad image reference or missing registry credentials
- Next actions: verify image URI and tag, fix registry auth, recreate the sandbox

### OOM kill

- First evidence: `inspect` shows `OOMKilled: True` or exit code `137`
- Confirming evidence: events mention container killed due to out-of-memory
- Likely cause: memory limit too low for the workload
- Next actions: increase memory, rerun the workload, compare peak workload memory with the configured limit

### Crash loop or bad entrypoint

- First evidence: `logs` show startup exceptions, missing binaries, or permission errors
- Confirming evidence: `inspect` shows repeated restarts, exit code `126`, or exit code `127`
- Likely cause: bad entrypoint, missing executable, or application crash on boot
- Next actions: fix the command or image contents, correct file permissions, redeploy or recreate

### Startup health-check failure

- First evidence: `sandbox health` fails while the sandbox object exists
- Confirming evidence: logs show the app never binds the expected port or readiness never turns healthy
- Likely cause: service startup problem, wrong bind address, or endpoint reachability issue
- Next actions: verify the service is listening on the expected port, confirm endpoint host and port, do not paper over the issue with `--skip-health-check`

### Endpoint or service unreachable

- First evidence: sandbox is `Running` but client requests fail or connection is refused
- Confirming evidence: `sandbox endpoint <id> --port <port>` is missing, wrong, or points to a service that is not listening
- Likely cause: wrong exposed port, service not bound, or server endpoint host misconfiguration
- Next actions: verify the port, inspect service logs, and if the endpoint host is unreachable from the client environment check the server endpoint configuration

### Scheduling or pending failure

- First evidence: sandbox remains `Pending`
- Confirming evidence: events show scheduler capacity issues, affinity or selector mismatch, or admission failures
- Likely cause: infrastructure or scheduling constraints rather than application code
- Next actions: resolve the scheduling constraint, then recreate or resume normal startup flow

## Response Format

Structure the answer in this order:

1. Current state: what the sandbox is doing now
2. Evidence: the command output that matters
3. Root cause: the most likely diagnosis, stated as confidence not certainty when needed
4. Next actions: specific fixes or follow-up checks

Keep the conclusion compact. Example:

> The sandbox is failing during image startup. `events` shows `ImagePullBackOff` for `registry.example/foo:latest`, so the most likely cause is an invalid image reference or missing registry credentials. Verify the image tag and registry secret, then recreate the sandbox.

## Best Practices

- resolve the active connection configuration before troubleshooting so you know which server the commands target
- prefer `osb config show` over checking environment variables in isolation
- Prefer CLI-first triage because it is shorter and usually already authenticated.
- Start with `get`, `health`, and `summary` before jumping into narrow commands.
- Treat state, events, inspect, and logs as different evidence buckets instead of one mixed signal.
- Verify endpoint and health before blaming application code for connection failures.
- Use `network-egress` when the sandbox is healthy but outbound access appears policy-restricted.
- If the server returns an unreachable endpoint host during readiness checks, inspect server endpoint configuration rather than weakening health checks.

## Minimal Closed Loops

CLI-first troubleshooting:

```bash
osb sandbox get <sandbox-id>
osb sandbox health <sandbox-id>
osb devops summary <sandbox-id>
osb devops events <sandbox-id> --limit 100
```

HTTP troubleshooting with authentication:

```bash
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  http://<server-domain>/v1/sandboxes/<sandbox-id>
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/summary
curl -H "OPEN-SANDBOX-API-KEY: <api-key>" \
  "http://<server-domain>/v1/sandboxes/<sandbox-id>/diagnostics/logs?tail=500"
```

Endpoint troubleshooting:

```bash
osb sandbox get <sandbox-id>
osb sandbox health <sandbox-id>
osb sandbox endpoint <sandbox-id> --port <port>
osb devops logs <sandbox-id> --tail 200
```

## API Notes

Diagnostics endpoints are plain text:

- `GET /v1/sandboxes/{id}/diagnostics/summary`
- `GET /v1/sandboxes/{id}/diagnostics/logs`
- `GET /v1/sandboxes/{id}/diagnostics/inspect`
- `GET /v1/sandboxes/{id}/diagnostics/events`

Common query params:

- `tail` for log line count
- `since` for time windows such as `10m` or `1h`
- `limit` for event count
