# OpenSandbox CLI

A command-line interface for managing OpenSandbox environments from your terminal. Built on top of the [OpenSandbox Python SDK](../sdks/sandbox/python/README.md), the CLI provides intuitive commands for sandbox lifecycle management, file operations, command execution, and diagnostics.

## Installation

### pip

```bash
pip install opensandbox-cli
```

### uv

```bash
uv tool install opensandbox-cli
```

### pipx (recommended for global CLI usage)

```bash
pipx install opensandbox-cli
```

## Overview

```bash
osb --help
```

![CLI Help](assets/cli_help.png)

## Quick Start

### Step 0: Start the OpenSandbox Server

Before using the CLI, make sure the OpenSandbox server is running. See the root [README.md](../README.md) for startup instructions.

```bash
opensandbox-server
```

![Start OpenSandbox Server](assets/start_opensandbox_server.png)

### Step 1: Install the CLI

```bash
cd cli
uv sync
uv run osb --help
```

![Install CLI](assets/install_cli.png)

### Step 2: Initialize Configuration

```bash
osb config init
osb config set connection.domain localhost:8080
osb config set connection.protocol http
```

![Init CLI](assets/init_cli.png)

### Step 3: Create a Sandbox

```bash
osb sandbox create --image python:3.12
```

Or configure defaults once and omit repeated flags:

```bash
osb config set defaults.image python:3.12
osb config set defaults.timeout 15m
osb sandbox create
```

You can also load network policy and volume mounts from JSON files:

```bash
osb sandbox create \
  --image python:3.12 \
  --network-policy-file network-policy.json \
  --volumes-file volumes.json
```

![Create Sandbox](assets/cli_create_sandbox.png)

### Step 4: List Sandboxes

```bash
# Table output (default)
osb sandbox list

# JSON output for scripting
osb -o json sandbox list
```

![List Sandboxes](assets/cli_list_sandbox.png)

![List Sandboxes JSON](assets/cli_list_sandbox_json.png)

### Step 5: Execute Commands

```bash
osb exec <sandbox-id> -- echo "hello world"
osb exec <sandbox-id> -- python -c "print(1+1)"
```

![Execute Commands](assets/cli_sandbox_exec.png)

### Step 6: File Operations

```bash
# Write a file
osb file write <sandbox-id> /tmp/test.txt -c "hello"

# Read it back
osb file cat <sandbox-id> /tmp/test.txt
```

![File Operations](assets/cli_sandbox_file.png)

### Step 7: Cleanup

```bash
osb sandbox kill <sandbox-id>
osb sandbox list
```

![Kill Sandbox](assets/cli_kill_sandbox.png)

## Command Reference

### `osb sandbox` — Lifecycle Management

| Command    | Description                                 |
| ---------- | ------------------------------------------- |
| `create`   | Create a new sandbox                        |
| `list`     | List sandboxes (with optional filters)      |
| `get`      | Get sandbox details by ID                   |
| `kill`     | Terminate one or more sandboxes             |
| `pause`    | Pause a running sandbox                     |
| `resume`   | Resume a paused sandbox                     |
| `renew`    | Renew sandbox expiration                    |
| `endpoint` | Get public endpoint for a sandbox port      |
| `health`   | Check sandbox health                        |
| `metrics`  | Get sandbox resource metrics (CPU, memory)  |

```bash
# Point-in-time metrics snapshot
osb sandbox metrics <sandbox-id>

# Live metrics stream
osb sandbox metrics <sandbox-id> --watch

# Resolve a service endpoint
osb sandbox endpoint <sandbox-id> --port 8080
```

### `osb command` — Command Execution

| Command     | Description                               |
| ----------- | ----------------------------------------- |
| `run`       | Run a shell command in the sandbox        |
| `status`    | Get command execution status              |
| `logs`      | Get background command logs               |
| `interrupt` | Interrupt a running command               |
| `session`   | Manage persistent bash sessions           |

### `osb exec` — Quick Command Shortcut

```bash
osb exec <sandbox-id> -- <command>
```

Shortcut for `osb command run`. Everything after `--` is passed as the command.

Persistent shell sessions:

```bash
# Create a bash session
osb command session create <sandbox-id>

# Reuse that session for multiple commands
osb command session run <sandbox-id> <session-id> -- pwd
osb command session run <sandbox-id> <session-id> -- export FOO=bar
osb command session run <sandbox-id> <session-id> -- sh -c 'echo $FOO'

# Delete the session when done
osb command session delete <sandbox-id> <session-id>
```

### `osb file` — File Operations

| Command    | Description                                |
| ---------- | ------------------------------------------ |
| `cat`      | Read file contents                         |
| `write`    | Write content to a file                    |
| `upload`   | Upload a local file to the sandbox         |
| `download` | Download a file from the sandbox           |
| `rm`       | Delete files                               |
| `mv`       | Move or rename a file                      |
| `mkdir`    | Create directories                         |
| `rmdir`    | Remove directories                         |
| `search`   | Search for files by pattern                |
| `info`     | Get file/directory metadata                |
| `chmod`    | Set file permissions                       |
| `replace`  | Find and replace content in a file         |

### `osb egress` — Runtime Egress Policy

| Command | Description                              |
| ------- | ---------------------------------------- |
| `get`   | Get the current runtime egress policy    |
| `patch` | Patch runtime egress rules with merge semantics |

```bash
# Inspect current policy
osb egress get <sandbox-id>

# Allow PyPI and deny an internal domain
osb egress patch <sandbox-id> --rule allow=pypi.org --rule deny=internal.example.com
```

### `osb devops` — Experimental DevOps Diagnostics

| Command   | Description                                          |
| --------- | ---------------------------------------------------- |
| `logs`    | Retrieve container/pod logs                          |
| `inspect` | Retrieve detailed container/pod inspection info      |
| `events`  | Retrieve events related to a sandbox                 |
| `summary` | One-shot diagnostics: inspect + events + logs combined |

These diagnostics commands are currently experimental. They are implemented by the server and exposed by the CLI, but are not yet part of the public `specs/` API contract and may change before being formalized.

```bash
# Quick diagnostics summary
osb devops summary <sandbox-id>

# Get last 500 log lines
osb devops logs <sandbox-id> --tail 500

# Get logs from the last 30 minutes
osb devops logs <sandbox-id> --since 30m

# Detailed container/pod inspection
osb devops inspect <sandbox-id>

# View events (up to 100)
osb devops events <sandbox-id> --limit 100
```

All devops commands return plain text output, making them ideal for both human reading and AI agent consumption.

![DevOps Summary 1](assets/cli_devops_summary_1.png)

![DevOps Summary 2](assets/cli_devops_summary_2.png)

### `osb skills` — AI Coding Skills

| Command     | Description                                          |
| ----------- | ---------------------------------------------------- |
| `install`   | Install one or more bundled OpenSandbox skills       |
| `show`      | Show the content and trigger hint for a bundled skill |
| `list`      | List bundled skills, targets, and install status     |
| `uninstall` | Remove an installed OpenSandbox skill                |

The CLI ships with a small built-in skill set for OpenSandbox-aware AI tooling, including:

- `troubleshoot-sandbox`
- `sandbox-lifecycle`
- `command-execution`
- `file-operations`
- `network-egress`
- `devops-diagnostics`

These skills cover common OpenSandbox flows such as troubleshooting, lifecycle control, command execution, file manipulation, network policy updates, and lower-level diagnostics. Supported targets:

| Target    | AI Tool          | Install Location |
| --------- | ---------------- | ---------------- |
| `claude`  | Claude Code      | `project: ./.claude/skills/`, `global: ~/.claude/skills/` |
| `cursor`  | Cursor           | `project: ./.cursor/rules/`, `global: ~/.cursor/rules/` |
| `codex`   | Codex            | `project: ./.codex/skills/<name>/SKILL.md`, `global: ~/.codex/skills/<name>/SKILL.md` |
| `copilot` | GitHub Copilot   | `project: ./.github/copilot-instructions.md`, `global: ~/.github/copilot-instructions.md` |
| `windsurf`| Windsurf         | `project: ./.windsurfrules`, `global: ~/.windsurfrules` |
| `cline`   | Cline            | `project: ./.clinerules`, `global: ~/.clinerules` |
| `opencode`| OpenCode         | `project: ./.agents/skills/<name>/SKILL.md`, `global: ~/.agents/skills/<name>/SKILL.md` |

```bash
# Show bundled skills and install status
osb skills list

# Show one bundled skill in full
osb skills show sandbox-lifecycle

# If you omit skill/target, the CLI prints install guidance
osb skills install

# Install a named skill for a specific tool and scope
osb skills install troubleshoot-sandbox --target cursor --scope project

# Install every bundled skill for a specific tool
osb skills install --all-builtins --target codex --scope global

# Install a single skill for OpenCode in the current project
osb skills install network-egress --target opencode --scope project

# Install for all supported tools
osb skills install troubleshoot-sandbox --target all --scope project

# Uninstall
osb skills uninstall troubleshoot-sandbox --target claude --scope global
```

### `osb config` — Configuration

| Command | Description                                |
| ------- | ------------------------------------------ |
| `init`  | Create a default config file               |
| `show`  | Show resolved configuration                |

## Configuration

The CLI resolves configuration from multiple sources with the following priority (highest to lowest):

1. **CLI flags** — `--api-key`, `--domain`, `--protocol`, `--timeout`
2. **Environment variables** — `OPEN_SANDBOX_API_KEY`, `OPEN_SANDBOX_DOMAIN`, `OPEN_SANDBOX_PROTOCOL`, `OPEN_SANDBOX_REQUEST_TIMEOUT`, `OPEN_SANDBOX_OUTPUT`
3. **Config file** — `~/.opensandbox/config.toml` (or path specified via `--config`)
4. **SDK defaults**

## Development

For local CLI development in this monorepo, prefer `uv sync` from the `cli/` directory. That workflow honors the local `[tool.uv.sources]` override for `opensandbox`, so the CLI resolves against the checked-out SDK instead of the published package.

```bash
cd cli
uv sync
uv run osb --help
```

If you specifically need an editable install into another environment, install the SDK dependencies from their local paths first, then install the CLI.

### Config File Format

```toml
[connection]
api_key = "your-api-key"
domain = "localhost:8080"
protocol = "http"
request_timeout = 30

[output]
format = "table"    # table | json | yaml
color = true

[defaults]
image = "python:3.11"
timeout = "10m"
```

## Global Options

| Option                        | Description                      |
| ----------------------------- | -------------------------------- |
| `--api-key TEXT`              | API key for authentication       |
| `--domain TEXT`               | API server domain                |
| `--protocol [http\|https]`    | Protocol                         |
| `--timeout INTEGER`           | Request timeout in seconds       |
| `-o, --output [table\|json\|yaml]` | Output format              |
| `--config PATH`               | Config file path                 |
| `-v, --verbose`               | Enable debug output              |
| `--no-color`                  | Disable colored output           |
| `--version`                   | Show version                     |

## Output Formats

The CLI supports three output formats via the `-o` / `--output` flag:

- **`table`** (default) — Human-friendly tables powered by [Rich](https://github.com/Textualize/rich)
- **`json`** — Machine-readable JSON
- **`yaml`** — YAML output

```bash
# Table (default)
osb sandbox list

# JSON for scripting
osb -o json sandbox list

# YAML
osb -o yaml sandbox list
```
