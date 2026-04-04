---
name: devops-diagnostics
description: Use OpenSandbox devops commands to collect inspect output, events, logs, and one-shot summaries for deep runtime diagnostics. Trigger when users want raw diagnostics data, lower-level runtime inspection, or detailed analysis of a sandbox issue.
---

# OpenSandbox DevOps Diagnostics

Use the `osb devops` command group when the user needs raw runtime diagnostics. Treat this skill as the evidence-collection layer, not the final troubleshooting conclusion.

## When To Use

- the user wants raw logs, inspect output, events, or a combined summary
- the user wants lower-level evidence before a diagnosis
- the user needs deeper runtime detail than `sandbox get` provides
- the user is investigating crashes, restarts, probe failures, image issues, scheduling problems, or networking symptoms

## Configuration Resolution

Before running diagnostics, resolve the active OpenSandbox connection configuration:

```bash
osb config show
```

Check the resolved values for:

- `domain`
- `api_key`
- `protocol`

Use the resolved configuration as the source of truth instead of checking only environment variables. If the user wants to change where diagnostics run, confirm whether the change should be temporary or persistent before rewriting settings.

## Diagnostics Model

Important properties of this command group:

- the diagnostics commands return plain-text output, not structured SDK model objects
- `summary` is the broadest starting point because it combines inspect output, event history, and recent logs
- `inspect`, `events`, and `logs` are detailed streams used after the broad summary
- this is an experimental diagnostics surface, so treat it as a powerful troubleshooting tool rather than a stable configuration workflow

## Golden Paths

Broad diagnostic sweep:

```bash
osb devops summary <sandbox-id>
osb devops inspect <sandbox-id>
osb devops events <sandbox-id> --limit 100
```

Log-focused investigation:

```bash
osb devops summary <sandbox-id>
osb devops logs <sandbox-id> --since 30m
osb devops logs <sandbox-id> --tail 500
```

Start with `summary` unless the user explicitly asks for a specific stream.

## Command Selection By Symptom

Use these defaults:

- suspected OOM or exit code issue
  start with `summary`, then `inspect`
- suspected application crash
  start with `summary`, then `logs`
- sandbox stuck in pending or scheduling trouble
  start with `summary`, then `events`
- suspected networking or port issue
  start with `summary`, then `inspect`, then `logs`

## Detailed Streams

One-shot summary:

```bash
osb devops summary <sandbox-id>
osb devops summary <sandbox-id> --tail 100 --event-limit 50
```

Detailed inspection:

```bash
osb devops inspect <sandbox-id>
```

Use `inspect` for container state, exit code, restart count, resource settings, and runtime metadata.

Event history:

```bash
osb devops events <sandbox-id> --limit 100
```

Use `events` for scheduling failures, image pull issues, restarts, and probe or lifecycle transitions.

Logs:

```bash
osb devops logs <sandbox-id> --tail 500
osb devops logs <sandbox-id> --since 30m
```

Rules:

- use `--tail` when the user wants recent output volume
- use `--since` when the relevant question is time-based
- prefer reading and quoting key lines from the text output instead of summarizing vaguely

## Relationship To Troubleshooting

- use this skill to gather raw runtime evidence
- use `troubleshoot-sandbox` when the user wants root cause analysis and concrete remediation
- do not jump straight to conclusions before collecting evidence with `summary`, `inspect`, `events`, or `logs`

## Response Pattern

Structure the answer as:

1. exact `osb devops` command or command sequence
2. what evidence each command is expected to reveal
3. the next diagnostic stream to inspect if the result is inconclusive

Keep command examples concrete and ready to paste.

## Minimal Closed Loops

Broad sweep:

```bash
osb devops summary <sandbox-id>
osb devops inspect <sandbox-id>
osb devops events <sandbox-id> --limit 100
```

Crash-focused investigation:

```bash
osb devops summary <sandbox-id>
osb devops logs <sandbox-id> --tail 500
```

Pending or scheduling investigation:

```bash
osb devops summary <sandbox-id>
osb devops events <sandbox-id> --limit 100
```

Recent-time-window investigation:

```bash
osb devops summary <sandbox-id>
osb devops logs <sandbox-id> --since 30m
```

## Best Practices

- resolve the active connection configuration before collecting diagnostics from a server
- prefer `osb config show` over checking individual environment variables in isolation
- start with `summary` unless a narrower stream is explicitly requested
- use `inspect` plus `events` for platform-level failures
- use `logs` for application-level failures
- quote concrete lines from the text output when forming a conclusion
- switch to `troubleshoot-sandbox` when the user wants diagnosis and remediation instead of raw evidence
