# execd - OpenSandbox Execution Daemon

English | [中文](README_zh.md)

`execd` is the execution daemon for OpenSandbox. Built on Beego, it exposes a comprehensive HTTP API that turns external requests into runtime actions: managing Jupyter sessions, streaming code output via Server-Sent Events (SSE), executing shell commands, operating on the sandbox filesystem, and collecting host-side metrics.

## Table of Contents

- [Overview](#overview)
- [Core Features](#core-features)
- [Architecture](#architecture)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
  - [Linux `clone3` compatibility inside sandboxes](#linux-clone3-compatibility-inside-sandboxes)
- [API Reference](#api-reference)
- [Supported Languages](#supported-languages)
- [Development](#development)
- [Testing](#testing)
- [Observability](#observability)
- [Performance Benchmarks](#performance-benchmarks)
- [Contributing](#contributing)
- [License](#license)
- [Support](#support)

## Overview

`execd` provides a unified interface for:

- **Code execution**: Python, Java, JavaScript, TypeScript, Go, and Bash
- **Session management**: Long-lived Jupyter kernel sessions with state
- **Command execution**: Synchronous and background shell commands
- **File operations**: Full filesystem CRUD with chunked upload/download
- **Monitoring**: Real-time host metrics (CPU, memory, uptime)
- **Interactive PTY**: Long-lived Bash over WebSocket (TTY or pipe mode, replay on reconnect) — see [PTY.md](./PTY.md)

## Core Features

### Unified runtime management

- Translate REST calls into runtime requests handled by `pkg/runtime`
- Multiple execution backends: Jupyter, shell, etc.
- Automatic language detection and routing
- Pluggable Jupyter server configuration

### Jupyter integration

- Maintain kernel sessions via `pkg/jupyter`
- WebSocket-based real-time communication
- Stream execution events through SSE

### Command executor

- Foreground and background shell commands
- Proper signal forwarding with process groups
- Real-time stdout/stderr streaming
- Context-aware interruption
- **SSE resume (foreground `POST /command` and `POST /code`)**: stdout/stderr are copied to a bounded ring buffer while the primary SSE is active. If the client disconnects, **`GET /command/{id}/resume?after_eid=<eid>`** replays buffered events (`eid > after_eid`) and may attach as the sole live consumer if the command is still running. An active primary stream returns **`409 Conflict`**. See `specs/execd-api.yaml`.

### Filesystem

- CRUD helpers around the sandbox filesystem
- Glob-based file search
- Chunked upload/download with resume support
- Permission management

### Observability

- Lightweight metrics endpoint (CPU, memory, uptime)
- Structured streaming logs
- SSE-based real-time monitoring

## Architecture

### Directory structure

| Path                   | Purpose                                              |
|------------------------|------------------------------------------------------|
| `main.go`              | Entry point; initializes Beego, CLI flags, routers   |
| `pkg/flag/`            | CLI and environment configuration                    |
| `pkg/web/`             | HTTP layer (controllers, models, router, SSE helpers) |
| `pkg/web/controller/`  | Handlers for files, code, commands, metrics          |
| `pkg/web/model/`       | Request/response models and SSE event types          |
| `pkg/runtime/`         | Dispatcher to Jupyter and shell executors            |
| `pkg/jupyter/`         | Minimal Jupyter client (kernels/sessions/WebSocket)  |
| `pkg/jupyter/execute/` | Execution result types and stream parsers            |
| `pkg/jupyter/session/` | Session management and lifecycle                     |
| `pkg/util/`            | Utilities (safe goroutine helpers, glob helpers)     |
| `pkg/clone3compat/`    | Optional seccomp shim for `clone3` → `ENOSYS` on Linux |
| `tests/`               | Test scripts and tools                               |

## Getting Started

### Prerequisites

- **Go 1.24+** (as defined in `go.mod`)
- **Jupyter Server** (required for code execution)
- **Docker** (optional, for containerized builds)
- **Make** (optional, for convenience targets)

### Quick Start

#### 1. Clone and build

```bash
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/components/execd
go mod download
make build
```

#### 2. Start Jupyter Server

```bash
# Option 1: use the provided script
./tests/jupyter.sh

# Option 2: start manually
jupyter notebook --port=54321 --no-browser --ip=0.0.0.0 \
  --NotebookApp.token='your-jupyter-token'
```

#### 3. Run execd

```bash
./bin/execd \
  --jupyter-host=http://127.0.0.1:54321 \
  --jupyter-token=your-jupyter-token \
  --port=44772
```

#### 4. Verify

```bash
curl -v http://localhost:44772/ping
# Expect HTTP 200
```

### Image build

```bash
docker build -t opensandbox/execd:dev .

# Run container
docker run -d \
  -p 44772:44772 \
  -e JUPYTER_HOST=http://jupyter-server \
  -e JUPYTER_TOKEN=your-token \
  --name execd \
  opensandbox/execd:dev
```

## Configuration

### Command-line flags

| Flag                          | Type     | Default | Description                                   |
|-------------------------------|----------|---------|-----------------------------------------------|
| `--jupyter-host`              | string   | `""`    | Jupyter server URL (reachable by execd)       |
| `--jupyter-token`             | string   | `""`    | Jupyter HTTP/WebSocket token                  |
| `--port`                      | int      | `44772` | HTTP listen port                              |
| `--log-level`                 | int      | `6`     | Beego log level (0=Emergency, 7=Debug)        |
| `--access-token`              | string   | `""`    | Shared API secret (optional)                  |
| `--graceful-shutdown-timeout` | duration | `3s`    | Wait time before cutting off SSE on shutdown  |

### Environment variables

All flags can be set via environment variables:

```bash
export JUPYTER_HOST=http://127.0.0.1:8888
export JUPYTER_TOKEN=your-token
```

Environment variables override defaults but are superseded by explicit CLI flags.

| Variable | Description |
|----------|-------------|
| `EXECD_CLONE3_COMPAT` | Linux only. See [below](#linux-clone3-compatibility-inside-sandboxes). |

### Linux `clone3` compatibility inside sandboxes

Some sandbox images ship **glibc ≥ 2.34**, which prefers the `clone3(2)` syscall. On **older container engines** (for example Docker **before 20.10.10** and matching containerd CRI versions), or wherever `clone3` is not handled correctly, **process creation can fail**. That affects anything that forks or execs: **Go `os/exec`** inside execd, shell commands, package managers (`apt`, `dnf`), and language runtimes that start subprocesses.

Typical symptoms are errors mentioning **`clone3`**, **`Function not implemented`**, or **`Operation not permitted`** when running commands or code **inside the sandbox**—not necessarily when starting the OpenSandbox server on the host.

#### Enabling the built-in workaround

execd can install a **seccomp** rule so `clone3` returns **`ENOSYS`**, forcing libc and the Go runtime to fall back to `clone(2)`—the same idea as [AkihiroSuda/clone3-workaround](https://github.com/AkihiroSuda/clone3-workaround). Implementation: [`pkg/clone3compat/`](pkg/clone3compat/).

Set the variable **in the sandbox container environment** (where execd is PID 1 or otherwise started):

| Value | Behavior |
|-------|----------|
| `1`, `true`, `yes`, `on` | Load the filter at the beginning of `main` (after Go runtime `init`). |
| `reexec` | Load the filter, then `exec` the same binary again so subsequent `init` runs already under seccomp (closest to wrapping with the external workaround binary). |

Unset, empty, or `0` / `false` / `off` / `no` disables the feature.

#### SDK

Set `EXECD_CLONE3_COMPAT` in `Sandbox.create` `env` so execd sees it when the sandbox starts:

```python
sandbox = await Sandbox.create(
    "opensandbox/code-interpreter:v1.0.2",
    entrypoint=["/opt/opensandbox/code-interpreter.sh"],
    env={"EXECD_CLONE3_COMPAT": "1"},
)
```

The sandbox must allow **seccomp** and **`PR_SET_NO_NEW_PRIVS`** for the filter to load. Upgrading the host container engine/kernel is the long-term fix when possible.

## API Reference

[API Spec](../../specs/execd-api.yaml).

## Supported Languages

### Jupyter-based

| Language   | Kernel      | Highlights                  |
|------------|-------------|-----------------------------|
| Python     | IPython     | Full Jupyter protocol       |
| Java       | IJava       | JShell-based execution      |
| JavaScript | IJavaScript | Node.js runtime             |
| TypeScript | ITypeScript | TS compilation + Node exec  |
| Go         | gophernotes | Go interpreter              |
| Bash       | Bash kernel | Shell scripts               |

### Native executors

| Mode/Language        | Backend | Highlights                   |
|----------------------|---------|------------------------------|
| `command`            | OS exec | Synchronous shell commands   |
| `background-command` | OS exec | Detached background process  |

## Development

See [DEVELOPMENT.md](./DEVELOPMENT.md) for detailed guidelines.

## Testing

### Unit tests

```bash
make test
```

### Integration tests

Integration tests requiring a real Jupyter Server are skipped by default:

```bash
export JUPYTER_URL=http://localhost:8888
export JUPYTER_TOKEN=your-token
go test -v ./pkg/jupyter/...
```

### Manual testing workflow

1. Start Jupyter: `./tests/jupyter.sh`
2. Start execd: `./bin/execd --jupyter-host=http://localhost:54321 --jupyter-token=opensandboxexecdlocaltest`
3. Execute code:

```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{"language":"python","code":"print(\"test\")"}' \
  http://localhost:44772/code
```

## Configuration

### API graceful shutdown window

- Env: `EXECD_API_GRACE_SHUTDOWN` (e.g. `500ms`, `2s`, `1m`)
- Flag: `--graceful-shutdown-timeout`
- Default: `1s`

This controls how long execd keeps SSE responses (code/command runs) alive after sending the final chunk, so clients can drain tail output before the connection closes. Set to `0s` to disable the grace period.

## Observability

### Logging

Beego leveled logger:

```go
logs.Info("message")   // info
logs.Warning("message") // warning
logs.Error("message")   // error
logs.Debug("message")   // debug
```

- Env: `EXECD_LOG_FILE` writes execd logs to the given file path; when unset, logs are sent to stdout.

Log levels (0-7):

- 0: Emergency
- 1: Alert
- 2: Critical
- 3: Error
- 4: Warning
- 5: Notice
- 6: Info (default)
- 7: Debug

### Metrics

`/metrics` exposes:

- CPU usage percent
- Memory total/used (GB)
- Memory usage percent
- Process uptime
- Current timestamp

For real-time monitoring, use `/metrics/watch` (SSE, 1s cadence).

## Performance Benchmarks

### Typical latency (localhost)

| Operation           | Latency  |
|---------------------|----------|
| `/ping`             | < 1ms    |
| `/files/info`       | < 5ms    |
| Code execution (Py) | 50-200ms |
| File upload (1MB)   | 10-50ms  |
| Metrics snapshot    | < 10ms   |

### Resource usage (idle)

- Memory: ~50MB
- CPU: < 1%
- Goroutines: ~15

### Scalability

- 100+ concurrent SSE connections
- File operations scale linearly with file size
- Jupyter sessions are stateful and need dedicated resources

## Contributing

1. Fork the repository
2. Create a feature branch
3. Follow coding conventions (see DEVELOPMENT.md)
4. Add tests for new functionality
5. Run `make fmt` and `make test`
6. Submit a pull request

## License

`execd` is part of the OpenSandbox project. See [LICENSE](../../LICENSE) in the repository root.

## Support

- Issues: [GitHub Issues](https://github.com/alibaba/OpenSandbox/issues)
- Documentation: [OpenSandbox Docs](https://github.com/alibaba/OpenSandbox/wiki)
- Community: [Discussions](https://github.com/alibaba/OpenSandbox/discussions)
