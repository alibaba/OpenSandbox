---
name: command-execution
description: Use OpenSandbox command execution commands to run foreground or background processes, inspect tracked execution status and logs, interrupt work, and manage persistent shell sessions inside a sandbox. Trigger when users want to execute commands in a sandbox and need exact OpenSandbox CLI flows.
---

# OpenSandbox Command Execution

Run commands with `osb command` or the `osb exec` shortcut. Choose the execution mode first, then use the matching follow-up commands. Do not mix foreground, background, and session workflows casually.

## When To Use

- the user wants to run a one-off command in a sandbox
- the user needs a tracked background command with later status/log inspection
- the user wants to stop a running command
- the user wants a persistent shell session that keeps working directory or environment state across runs

## Execution Modes

Treat these as three distinct execution paths:

- `osb exec` or `osb command run` without `--background`
  Use for foreground one-shot commands when the result should stream directly to the terminal
- `osb command run --background`
  Use when the user needs an execution ID, later status checks, or log retrieval
- `osb command session ...`
  Use when shell state must persist across commands, such as exported variables or a working directory

## Golden Paths

Foreground one-shot command:

```bash
osb exec <sandbox-id> -- python -c "print(1 + 1)"
```

Tracked background command:

```bash
osb command run <sandbox-id> --background -- sh -c "sleep 10; echo done"
osb command status <sandbox-id> <execution-id>
osb command logs <sandbox-id> <execution-id>
```

Persistent session:

```bash
osb command session create <sandbox-id> --workdir /workspace
osb command session run <sandbox-id> <session-id> -- pwd
osb command session run <sandbox-id> <session-id> -- export FOO=bar
osb command session run <sandbox-id> <session-id> -- sh -c 'echo $FOO'
osb command session delete <sandbox-id> <session-id>
```

## Foreground Commands

For simple one-off execution, use:

```bash
osb exec <sandbox-id> -- <command>
osb command run <sandbox-id> -- <command>
osb command run <sandbox-id> --workdir /workspace -- <command>
osb command run <sandbox-id> --timeout 30s -- <command>
```

Use foreground mode when the user wants immediate output and does not need a tracked execution ID.

## Background Commands

Use background mode when the user will need follow-up inspection:

```bash
osb command run <sandbox-id> --background -- <command>
osb command run <sandbox-id> --background --workdir /workspace -- <command>
osb command run <sandbox-id> --background --timeout 5m -- <command>
```

Then inspect the tracked execution:

```bash
osb command status <sandbox-id> <execution-id>
osb command logs <sandbox-id> <execution-id>
osb command logs <sandbox-id> <execution-id> --cursor 0
```

Use `status` for lifecycle state and exit information. Use `logs` for tracked background output. Do not suggest `command logs` for foreground commands that already streamed to the terminal.

## Persistent Sessions

Use sessions when commands must share shell state:

```bash
osb command session create <sandbox-id> --workdir /workspace
osb command session run <sandbox-id> <session-id> -- pwd
osb command session run <sandbox-id> <session-id> -- export FOO=bar
osb command session run <sandbox-id> <session-id> -- sh -c 'echo $FOO'
osb command session run <sandbox-id> <session-id> --workdir /var -- pwd
osb command session delete <sandbox-id> <session-id>
```

Rules:

- `session create --workdir` sets the initial working directory for the session
- `session run --workdir` overrides the working directory for that single run only
- exported variables and shell state persist across `session run` calls in the same session
- delete the session when the user is done; do not leave idle sessions around

## Interrupting Work

Interrupt only tracked executions:

```bash
osb command interrupt <sandbox-id> <execution-id>
```

Only suggest interruption when the user explicitly wants to stop work or the process is clearly stuck.

## Failure Semantics

- foreground `osb exec` and foreground `osb command run` stream output directly and exit non-zero on execution error
- `session run` also exits non-zero on execution error
- tracked background commands should be checked with `status` and `logs`
- if the command failure is caused by an unhealthy sandbox rather than the command itself, switch to `troubleshoot-sandbox`

## Response Pattern

Structure the answer as:

1. exact command to run
2. which execution mode it uses
3. the next inspection or cleanup command if the workflow continues

Keep command examples concrete and ready to paste.

## Minimal Closed Loops

Foreground command with timeout:

```bash
osb command run <sandbox-id> --timeout 30s -- python -c "print(1 + 1)"
```

Tracked background execution:

```bash
osb command run <sandbox-id> --background -- sh -c "sleep 10; echo done"
osb command status <sandbox-id> <execution-id>
osb command logs <sandbox-id> <execution-id>
```

Background execution with interrupt:

```bash
osb command run <sandbox-id> --background -- sh -c "sleep 300"
osb command interrupt <sandbox-id> <execution-id>
osb command status <sandbox-id> <execution-id>
```

Persistent session with shared shell state:

```bash
osb command session create <sandbox-id> --workdir /workspace
osb command session run <sandbox-id> <session-id> -- export FOO=bar
osb command session run <sandbox-id> <session-id> -- sh -c 'echo $FOO'
osb command session delete <sandbox-id> <session-id>
```

Per-run working directory override inside a session:

```bash
osb command session create <sandbox-id> --workdir /tmp
osb command session run <sandbox-id> <session-id> -- pwd
osb command session run <sandbox-id> <session-id> --workdir /var -- pwd
osb command session delete <sandbox-id> <session-id>
```

## Best Practices

- use `osb exec` for quick foreground commands only
- use `osb command run --background` when the user will need execution tracking
- use sessions only when state persistence is actually needed
- use `--workdir` explicitly when directory context matters
- use `--timeout` when the command should not run indefinitely
- prefer `status` before guessing whether a background command is still running or already failed
