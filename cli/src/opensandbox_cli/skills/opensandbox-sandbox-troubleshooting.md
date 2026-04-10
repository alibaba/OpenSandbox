---
name: sandbox-troubleshooting
description: Use OpenSandbox CLI state, health, summary, inspect, events, and logs to investigate failed, unhealthy, or unreachable sandboxes. Trigger when users report startup failures, crashes, OOM, image pull problems, pending sandboxes, network issues, or an unresponsive sandbox and want root cause plus next actions.
---

# OpenSandbox Sandbox Troubleshooting

Investigate the reported sandbox before proposing a fix. Prefer evidence from OpenSandbox state, health checks, summary output, and diagnostics streams over speculation.

## Inputs To Collect

Capture these from the user request or surrounding context before running commands:

- sandbox ID or an unambiguous short ID
- whether `osb` or `opensandbox` CLI is available locally
- any reported symptom: pending forever, crash, OOM, unreachable service, bad image, failed exec, etc.

If the sandbox ID is missing, ask for it first.

## Configuration Resolution

Before troubleshooting a sandbox, resolve the active OpenSandbox connection configuration:

```bash
osb config show -o json
```

Check the resolved values for:

- `domain`
- `api_key`
- `protocol`
- `use_server_proxy` when endpoint routing or proxy behavior may matter

`osb config show` redacts the API key. Use it to confirm the effective target server and protocol before troubleshooting.

If `domain` is missing, stop and set it first:

```bash
osb config set connection.domain <host:port> -o json
osb config show -o json
```

If auth is required and `api_key` is missing, stop and set it first:

```bash
osb config set connection.api_key <api-key> -o json
osb config show -o json
```

Use raw HTTP only after domain, protocol, and API key expectations are explicit.

## Operating Rules

- start with the highest-signal commands first: sandbox state, sandbox health, then diagnostics summary
- use CLI commands when `osb` is available because they are shorter and usually already authenticated
- use HTTP only when the CLI is unavailable or the user is clearly working from raw API access
- distinguish observed facts from inference and quote the field, event, or log line that supports the diagnosis
- separate sandbox/runtime failures from workload/application failures before suggesting a fix
- do not paper over readiness problems with `--skip-health-check`
- end with a likely root cause and 1-3 concrete remediation steps

## Triage Order

Use this order by default:

```bash
osb sandbox get <sandbox-id> -o json
osb sandbox health <sandbox-id> -o json
osb devops summary <sandbox-id> -o raw
```

Then drill down only where the summary points:

```bash
osb devops inspect <sandbox-id> -o raw
osb devops events <sandbox-id> --limit 100 -o raw
osb devops logs <sandbox-id> --tail 500 -o raw
osb devops logs <sandbox-id> --since 30m -o raw
```

## Diagnostics Streams

Important properties of the diagnostics commands:

- `summary` is the broadest starting point because it combines inspect output, event history, and recent logs
- `inspect`, `events`, and `logs` are detailed streams used after the broad summary
- diagnostics commands return plain-text output, not structured SDK model objects
- quote concrete lines from diagnostics output instead of summarizing vaguely

Use:

- `inspect` for container state, exit code, restart count, resources, ports, and runtime metadata
- `events` for scheduling failures, image pull issues, restarts, OOM kills, and probe transitions
- `logs` for application errors, missing binaries, bad entrypoints, startup hangs, and health-check failures

## Symptom To Command Mapping

Use the first command that best matches the reported symptom:

| Symptom | First command | What to confirm next |
| --- | --- | --- |
| pending forever or stuck creating | `osb devops events <sandbox-id> --limit 100 -o raw` | image pull errors, scheduling failures, admission errors |
| image pull failure | `osb devops events <sandbox-id> --limit 100 -o raw` | image name, tag, registry auth |
| crash loop or repeated restarts | `osb devops logs <sandbox-id> --tail 200 -o raw` | `osb devops inspect <sandbox-id> -o raw` for exit code and restart count |
| suspected OOM or exit code issue | `osb devops inspect <sandbox-id> -o raw` | `OOMKilled`, exit code, resource limits |
| endpoint unreachable or connection refused | `osb sandbox health <sandbox-id> -o json` | `osb sandbox endpoint <sandbox-id> --port <port> -o json` and then `osb devops logs <sandbox-id> --tail 200 -o raw` |
| outbound network access failure | `osb sandbox health <sandbox-id> -o json` | check service behavior, then switch to `network-egress` if the issue is egress policy related |

## Diagnosis Playbooks

### Image Pull Failure

- first evidence: `events` shows `ImagePullBackOff`, `ErrImagePull`, or auth failures
- confirming evidence: sandbox stays `Pending` or never reaches healthy state
- likely cause: bad image reference or missing registry credentials
- next actions: verify image URI and tag, fix registry auth, recreate the sandbox

### OOM Kill

- first evidence: `inspect` shows `OOMKilled: True` or exit code `137`
- confirming evidence: events mention container killed due to out-of-memory
- likely cause: memory limit too low for the workload
- next actions: increase memory, rerun the workload, compare peak workload memory with the configured limit

### Crash Loop Or Bad Entrypoint

- first evidence: `logs` show startup exceptions, missing binaries, or permission errors
- confirming evidence: `inspect` shows repeated restarts, exit code `126`, or exit code `127`
- likely cause: bad entrypoint, missing executable, or application crash on boot
- next actions: fix the command or image contents, correct file permissions, redeploy or recreate

### Endpoint Or Service Unreachable

- first evidence: sandbox is `Running` but client requests fail or connection is refused
- confirming evidence: `sandbox endpoint <id> --port <port>` is missing, wrong, or points to a service that is not listening
- likely cause: wrong exposed port, service not bound, or server endpoint host misconfiguration
- next actions: verify the port, inspect service logs, and if the endpoint host is unreachable from the client environment check the server endpoint configuration

## Minimal Closed Loops

CLI-first troubleshooting:

```bash
osb sandbox get <sandbox-id> -o json
osb sandbox health <sandbox-id> -o json
osb devops summary <sandbox-id> -o raw
osb devops events <sandbox-id> --limit 100 -o raw
```

Crash-focused investigation:

```bash
osb devops summary <sandbox-id> -o raw
osb devops logs <sandbox-id> --tail 500 -o raw
osb devops inspect <sandbox-id> -o raw
```

Endpoint troubleshooting:

```bash
osb sandbox get <sandbox-id> -o json
osb sandbox health <sandbox-id> -o json
osb sandbox endpoint <sandbox-id> --port <port> -o json
osb devops logs <sandbox-id> --tail 200 -o raw
```

## Response Format

Structure the answer in this order:

1. current state: what the sandbox is doing now
2. evidence: the command output that matters
3. root cause: the most likely diagnosis, stated as confidence not certainty when needed
4. next actions: specific fixes or follow-up checks

Keep the conclusion compact.
