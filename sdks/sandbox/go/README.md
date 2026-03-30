# OpenSandbox Go SDK

Go client library for the [OpenSandbox](https://github.com/alibaba/OpenSandbox) API.

Covers all three OpenAPI specs:
- **Lifecycle** — Create, manage, and destroy sandbox instances
- **Execd** — Execute commands, manage files, monitor metrics inside sandboxes
- **Egress** — Inspect and mutate sandbox network policy at runtime

## Installation

```bash
go get github.com/alibaba/OpenSandbox/sdks/sandbox/go
```

## Quick Start

### Create and manage a sandbox

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func main() {
    ctx := context.Background()

    // Create a lifecycle client
    lc := opensandbox.NewLifecycleClient("http://localhost:8080/v1", "your-api-key")

    // Create a sandbox
    sbx, err := lc.CreateSandbox(ctx, opensandbox.CreateSandboxRequest{
        Image:      opensandbox.ImageSpec{URI: "python:3.12"},
        Entrypoint: []string{"/bin/sh"},
        ResourceLimits: opensandbox.ResourceLimits{
            "cpu":    "500m",
            "memory": "512Mi",
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created sandbox: %s (state: %s)\n", sbx.ID, sbx.Status.State)

    // Get sandbox details
    sbx, err = lc.GetSandbox(ctx, sbx.ID)
    if err != nil {
        log.Fatal(err)
    }

    // List all running sandboxes
    list, err := lc.ListSandboxes(ctx, opensandbox.ListOptions{
        States:   []opensandbox.SandboxState{opensandbox.StateRunning},
        PageSize: 10,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Running sandboxes: %d\n", list.Pagination.TotalItems)

    // Pause and resume
    _ = lc.PauseSandbox(ctx, sbx.ID)
    _ = lc.ResumeSandbox(ctx, sbx.ID)

    // Clean up
    _ = lc.DeleteSandbox(ctx, sbx.ID)
}
```

### Run a command with streaming output

```go
exec := opensandbox.NewExecdClient("http://localhost:9090", "your-execd-token")

err := exec.RunCommand(ctx, opensandbox.RunCommandRequest{
    Command: "echo 'Hello from sandbox!'",
    Timeout: 30000,
}, func(event opensandbox.StreamEvent) error {
    // The execd server emits NDJSON (raw JSON blobs separated by blank
    // lines), so event.Event is always empty. Parse the "type" field
    // from the JSON payload in event.Data instead.
    var msg struct {
        Type string `json:"type"`
        Data string `json:"data"`
    }
    if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
        return fmt.Errorf("parse stream event: %w", err)
    }
    switch msg.Type {
    case "stdout":
        fmt.Print(msg.Data)
    case "stderr":
        fmt.Fprintf(os.Stderr, "%s", msg.Data)
    case "result":
        fmt.Printf("\n[done] %s\n", msg.Data)
    }
    return nil
})
```

### Check egress policy

```go
egress := opensandbox.NewEgressClient("http://localhost:18080", "your-egress-token")

// Get current policy
policy, err := egress.GetPolicy(ctx)
fmt.Printf("Mode: %s, Default: %s\n", policy.Mode, policy.Policy.DefaultAction)

// Add a rule
updated, err := egress.PatchPolicy(ctx, []opensandbox.NetworkRule{
    {Action: "allow", Target: "api.example.com"},
})
```

## API Reference

### LifecycleClient

Created with `NewLifecycleClient(baseURL, apiKey string, opts ...Option)`.

| Method | Description |
|--------|-------------|
| `CreateSandbox(ctx, req)` | Create a new sandbox from a container image |
| `GetSandbox(ctx, id)` | Get sandbox details by ID |
| `ListSandboxes(ctx, opts)` | List sandboxes with filtering and pagination |
| `DeleteSandbox(ctx, id)` | Delete a sandbox |
| `PauseSandbox(ctx, id)` | Pause a running sandbox |
| `ResumeSandbox(ctx, id)` | Resume a paused sandbox |
| `RenewExpiration(ctx, id, duration)` | Extend sandbox expiration time |
| `GetEndpoint(ctx, sandboxID, port)` | Get public endpoint for a sandbox port |

### ExecdClient

Created with `NewExecdClient(baseURL, accessToken string, opts ...Option)`.

**Health:**
| Method | Description |
|--------|-------------|
| `Ping(ctx)` | Check server health |

**Code Execution:**
| Method | Description |
|--------|-------------|
| `ListContexts(ctx, language)` | List active code execution contexts |
| `CreateContext(ctx, req)` | Create a code execution context |
| `GetContext(ctx, contextID)` | Get context details |
| `DeleteContext(ctx, contextID)` | Delete a context |
| `DeleteContextsByLanguage(ctx, language)` | Delete all contexts for a language |
| `ExecuteCode(ctx, req, handler)` | Execute code with SSE streaming |
| `InterruptCode(ctx, sessionID)` | Interrupt running code |

**Command Execution:**
| Method | Description |
|--------|-------------|
| `CreateSession(ctx)` | Create a bash session |
| `RunInSession(ctx, sessionID, req, handler)` | Run command in session with SSE |
| `DeleteSession(ctx, sessionID)` | Delete a bash session |
| `RunCommand(ctx, req, handler)` | Run a command with SSE streaming |
| `InterruptCommand(ctx, sessionID)` | Interrupt running command |
| `GetCommandStatus(ctx, commandID)` | Get command execution status |
| `GetCommandLogs(ctx, commandID, cursor)` | Get command stdout/stderr |

**File Operations:**
| Method | Description |
|--------|-------------|
| `GetFileInfo(ctx, path)` | Get file metadata |
| `DeleteFiles(ctx, paths)` | Delete files |
| `SetPermissions(ctx, req)` | Change file permissions |
| `MoveFiles(ctx, req)` | Move/rename files |
| `SearchFiles(ctx, dir, pattern)` | Search files by glob pattern |
| `ReplaceInFiles(ctx, req)` | Text replacement in files |
| `UploadFile(ctx, localPath, remotePath)` | Upload a file to the sandbox |
| `DownloadFile(ctx, remotePath, rangeHeader)` | Download a file from the sandbox |

**Directory Operations:**
| Method | Description |
|--------|-------------|
| `CreateDirectory(ctx, path, mode)` | Create a directory (mkdir -p) |
| `DeleteDirectory(ctx, path, recursive)` | Delete a directory |

**Metrics:**
| Method | Description |
|--------|-------------|
| `GetMetrics(ctx)` | Get system resource metrics |
| `WatchMetrics(ctx, handler)` | Stream metrics via SSE |

### EgressClient

Created with `NewEgressClient(baseURL, authToken string, opts ...Option)`.

| Method | Description |
|--------|-------------|
| `GetPolicy(ctx)` | Get current egress policy |
| `PatchPolicy(ctx, rules)` | Merge rules into current policy |

## SSE Streaming

Methods that stream output (`RunCommand`, `ExecuteCode`, `RunInSession`, `WatchMetrics`) accept an `EventHandler` callback:

```go
type EventHandler func(event StreamEvent) error
```

Each `StreamEvent` contains:
- `Event` — the SSE event type, if provided. **Note:** the execd server emits NDJSON (raw JSON blobs separated by blank lines) rather than standard SSE `event:` fields, so `Event` will be empty for execd streams.
- `Data` — the event payload. For execd streams, this is a JSON object with a `type` field (`"stdout"`, `"stderr"`, `"result"`) and a `data` field containing the actual content.
- `ID` — optional event identifier

For execd streams, parse `event.Data` as JSON and switch on the `type` field (see the [quick start example](#run-a-command-with-streaming-output) above).

Return a non-nil error from the handler to stop processing the stream early.

## Client Options

All client constructors accept optional `Option` functions:

```go
// Use a custom http.Client
client := opensandbox.NewLifecycleClient(url, key,
    opensandbox.WithHTTPClient(myHTTPClient),
)

// Set a custom timeout
client := opensandbox.NewExecdClient(url, token,
    opensandbox.WithTimeout(60 * time.Second),
)
```

## Error Handling

Non-2xx responses are returned as `*opensandbox.APIError`:

```go
_, err := lc.GetSandbox(ctx, "nonexistent")
if apiErr, ok := err.(*opensandbox.APIError); ok {
    fmt.Printf("HTTP %d: %s — %s\n", apiErr.StatusCode, apiErr.Response.Code, apiErr.Response.Message)
}
```

## License

Apache 2.0
