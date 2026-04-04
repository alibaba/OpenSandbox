---
name: sandbox-lifecycle
description: Use OpenSandbox CLI lifecycle commands to create, inspect, verify, renew, pause, resume, expose, and terminate sandboxes. Trigger when users want help provisioning a sandbox, choosing create flags, checking runtime state, retrieving endpoints, or safely cleaning up sandboxes.
---

# OpenSandbox Sandbox Lifecycle

Use OpenSandbox lifecycle commands directly instead of giving generic container advice. Prefer a verified lifecycle flow over isolated commands.

## When To Use

- the user wants to create a sandbox for a task or workflow
- the user needs to inspect sandbox state or health
- the user wants to expose a service port through an OpenSandbox endpoint
- the user wants to renew, pause, resume, or terminate a sandbox
- the user is unsure which create flags or file-based inputs to use

## Configuration Resolution

Before giving lifecycle commands, resolve the active OpenSandbox connection configuration:

```bash
osb config show
```

Check the resolved values for:

- `domain`
- `api_key`
- `protocol`
- `use_server_proxy` when endpoint routing matters

Do not focus only on environment variables. `osb config show` already reflects the effective configuration after applying CLI flags, environment variables, config file values, and defaults.

After reading the resolved configuration:

- tell the user what connection settings are currently active
- ask whether they want to keep using the current configuration, temporarily override it for one command, or persist new values
- if they want a persistent change, use `osb config set connection.<field> ...`
- if they only want a one-off override, use CLI flags such as `--domain`, `--api-key`, or `--protocol`

## Golden Path

Use this as the default lifecycle flow unless the user asks for a narrower action:

```bash
osb config show
osb sandbox create --image python:3.12 --timeout 30m
osb sandbox get <sandbox-id>
osb sandbox health <sandbox-id>
```

If the sandbox is intended to serve traffic on a known port, continue with:

```bash
osb sandbox endpoint <sandbox-id> --port 8080
```

This sequence is safer than stopping at `sandbox get`, because `get` confirms object state while `health` confirms the sandbox is actually reachable through the execd health path.

## Create Options

Start with the narrowest create command that matches the request:

```bash
osb sandbox create --image python:3.12
osb sandbox create --image node:20 --timeout 30m
osb sandbox create --image python:3.12 --entrypoint python --entrypoint -m --entrypoint http.server
osb sandbox create --image python:3.12 --ready-timeout 90s
osb sandbox create --image python:3.12 --network-policy-file network-policy.json
osb sandbox create --image python:3.12 --volumes-file volumes.json
```

Use these options deliberately:

- `--image`: required unless the CLI already has `defaults.image` configured
- `--timeout`: recommended for most temporary workloads so sandboxes do not linger indefinitely
- `--entrypoint`: repeat the flag once per argv item; do not use JSON or shell-wrapped command strings
- `--ready-timeout`: increase this when the image or workload needs more startup time
- `--skip-health-check`: use only when the user explicitly wants object creation without waiting for readiness; do not use it to mask startup problems

If the user does not specify an image, recommend one that matches the runtime they need instead of guessing silently.

## JSON Shapes

When recommending `--network-policy-file` or `--volumes-file`, always show the JSON shape instead of assuming the user knows it.

Example `network-policy.json`:

```json
{
  "defaultAction": "deny",
  "egress": [
    {
      "action": "allow",
      "target": "pypi.org"
    },
    {
      "action": "allow",
      "target": "files.pythonhosted.org"
    }
  ]
}
```

Example `volumes-host.json`:

```json
[
  {
    "name": "workspace-data",
    "host": {
      "path": "/tmp/opensandbox-data"
    },
    "mountPath": "/workspace/data",
    "readOnly": false
  }
]
```

Example `volumes-pvc.json`:

```json
[
  {
    "name": "shared-models",
    "pvc": {
      "claimName": "shared-models-pvc"
    },
    "mountPath": "/workspace/models",
    "readOnly": true
  }
]
```

Prefer `pvc` when the environment supports it and the user needs a more portable storage definition. Use `host` when the user explicitly needs a host-path bind mount and the server has been configured to allow that path.

## Verification

Use verification commands in this order:

```bash
osb sandbox get <sandbox-id>
osb sandbox health <sandbox-id>
osb sandbox metrics <sandbox-id>
osb sandbox metrics <sandbox-id> --watch
```

Use:

- `sandbox get` to inspect the current lifecycle state and metadata
- `sandbox health` to confirm the sandbox is usable
- `sandbox metrics` for a point-in-time resource snapshot
- `sandbox metrics --watch` when the user wants live CPU and memory updates while diagnosing load or pressure

If the user needs a public or routed port, verify it explicitly:

```bash
osb sandbox endpoint <sandbox-id> --port 8080
```

## Lifecycle Actions

Use these commands for ongoing lifecycle management:

```bash
osb sandbox list
osb sandbox renew <sandbox-id> --timeout 30m
osb sandbox pause <sandbox-id>
osb sandbox resume <sandbox-id>
osb sandbox kill <sandbox-id>
```

Rules:

- use `sandbox list` for discovery or filtering, not single-sandbox verification
- use `renew` before long-running work instead of waiting for expiry
- use `pause` only when the workload can tolerate suspension
- use `kill` when cleanup is the real goal; do not leave orphaned sandboxes behind

## Runtime Notes

- `renew` resets the expiration to approximately `now + timeout`; treat it as a fresh TTL, not a simple additive extension to the old timestamp
- `pause` and `resume` may depend on the underlying runtime; if the runtime does not support them, avoid promising they will work
- host-path volumes depend on server-side allowed host path configuration
- if creation fails or the sandbox never becomes healthy, switch to `troubleshoot-sandbox` instead of adding more create flags blindly

## Response Pattern

Structure the answer as:

1. exact command to run
2. what state change or verification result to expect
3. the next lifecycle command if the workflow continues

Keep command examples concrete and ready to paste.

## Minimal Closed Loops

Create and verify readiness:

```bash
osb sandbox create --image python:3.12 --timeout 30m
osb sandbox get <sandbox-id>
osb sandbox health <sandbox-id>
```

Create a service sandbox and retrieve its endpoint:

```bash
osb sandbox create --image python:3.12 --timeout 30m
osb sandbox health <sandbox-id>
osb sandbox endpoint <sandbox-id> --port 8080
```

Create with network policy:

```bash
osb sandbox create --image python:3.12 --network-policy-file network-policy.json
osb sandbox get <sandbox-id>
osb sandbox health <sandbox-id>
```

Renew before long work:

```bash
osb sandbox renew <sandbox-id> --timeout 30m
osb sandbox get <sandbox-id>
```

Pause and confirm state:

```bash
osb sandbox pause <sandbox-id>
osb sandbox get <sandbox-id>
```

Resume and verify health:

```bash
osb sandbox resume <sandbox-id>
osb sandbox health <sandbox-id>
```

## Best Practices

- resolve the active connection configuration before assuming which server the command will hit
- prefer `osb config show` over checking individual environment variables in isolation
- confirm whether the user wants to keep, override, or persist connection settings before changing them

- Prefer explicit `--image` and `--timeout` when demonstrating commands
- Prefer `health` over assuming readiness from `create` output alone
- Prefer `endpoint` over telling the user to guess host/port mappings
- Prefer `pvc` over `host` when portability matters
- Prefer troubleshooting over `--skip-health-check` when startup is failing
