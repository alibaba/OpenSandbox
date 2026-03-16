# OpenSandbox CLI

A command-line interface for managing OpenSandbox environments from your terminal. Built on top of the [OpenSandbox Python SDK](../sdks/sandbox/python/README.md), the CLI provides intuitive commands for sandbox lifecycle management, file operations, command execution, and code interpretation.

## Installation

### pip

```bash
pip install opensandbox-cli
```

### uv

```bash
uv add opensandbox-cli
```

### pipx (recommended for global CLI usage)

```bash
pipx install opensandbox-cli
```

## Quick Start

> **Note**: Before running these commands, ensure the OpenSandbox service is running. See the root [README.md](../README.md) for startup instructions.

### 1. Initialize Configuration

```bash
osb config init
```

This creates `~/.opensandbox/config.toml` with default settings. Edit it to set your API key and server domain:

```toml
[connection]
api_key = "your-api-key"
domain = "localhost:8080"
protocol = "http"
```

### 2. Create a Sandbox

```bash
osb sandbox create --image python:3.11
```

### 3. Run Commands

```bash
# Quick shortcut
osb exec <sandbox-id> -- echo "Hello Sandbox!"

# Or use the full command group
osb command run <sandbox-id> -- ls -la /workspace
```

### 4. Manage Files

```bash
# Read a file
osb file cat <sandbox-id> /etc/hostname

# Upload a local file
osb file upload <sandbox-id> ./script.py /workspace/script.py

# Download a file
osb file download <sandbox-id> /workspace/output.txt ./output.txt
```

### 5. Execute Code (Code Interpreter)

```bash
osb code run <sandbox-id> --language python -c "print('Hello from sandbox!')"
```

### 6. Cleanup

```bash
osb sandbox kill <sandbox-id>
```

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

### `osb command` — Command Execution

| Command     | Description                               |
| ----------- | ----------------------------------------- |
| `run`       | Run a shell command in the sandbox        |
| `status`    | Get command execution status              |
| `logs`      | Get background command logs               |
| `interrupt` | Interrupt a running command               |

### `osb exec` — Quick Command Shortcut

```bash
osb exec <sandbox-id> -- <command>
```

Shortcut for `osb command run`. Everything after `--` is passed as the command.

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

### `osb code` — Code Interpreter

| Command     | Description                               |
| ----------- | ----------------------------------------- |
| `run`       | Execute code in a sandbox                 |
| `context`   | Manage code execution contexts            |
| `interrupt` | Interrupt a running code execution        |

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
