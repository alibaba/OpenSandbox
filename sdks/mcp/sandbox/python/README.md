# OpenSandbox MCP Sandbox Server

## 1. Overview

OpenSandbox MCP Server exposes the OpenSandbox Python SDK as MCP tools for
Claude Code, Cursor, and other MCP-capable clients. It provides sandbox
lifecycle management, command execution, and filesystem operations.

## 2. Installation & Startup

### Source

```bash
uv sync
uv run opensandbox-mcp
```

### Package

```bash
pip install opensandbox-mcp
opensandbox-mcp
```

### Configuration

Environment variables:

- `OPEN_SANDBOX_API_KEY`
- `OPEN_SANDBOX_DOMAIN`

CLI overrides:

```bash
opensandbox-mcp --api-key ... --domain ... --protocol https
```

Config fields:

- `api_key`: OpenSandbox API key for authentication.
- `domain`: OpenSandbox API domain, for example `api.opensandbox.io`.
- `protocol`: `http` or `https` for API requests.
- `request_timeout_seconds`: HTTP request timeout in seconds.
- `transport`: `stdio` by default, or `streamable-http`.

### Streamable HTTP

```bash
opensandbox-mcp \
  --transport streamable-http
```

## 3. Integrations

### Claude Code stdio

```bash
claude mcp add opensandbox-sandbox --transport stdio -- \
  opensandbox-mcp --api-key "$OPEN_SANDBOX_API_KEY" --domain "$OPEN_SANDBOX_DOMAIN"
```

### Claude Code http

```bash
claude mcp add opensandbox-sandbox --transport http http://localhost:8000/mcp
```

### Cursor stdio

```json
{
  "mcpServers": {
    "opensandbox-sandbox": {
      "command": "opensandbox-mcp",
      "args": [
        "--api-key",
        "${OPEN_SANDBOX_API_KEY}",
        "--domain",
        "${OPEN_SANDBOX_DOMAIN}"
      ]
    }
  }
}
```

### Cursor http

```json
{
  "mcpServers": {
    "opensandbox-sandbox": {
      "url": "http://localhost:8000/mcp"
    }
  }
}
```

## 4. Tools

### Server

- `server_healthcheck`: basic service health probe
- `server_sdk_version`: OpenSandbox SDK version

### Sandbox Lifecycle

- `sandbox_create`: create a new sandbox and register it locally
- `sandbox_connect`: attach to an existing sandbox and register it locally
- `sandbox_resume`: resume a paused sandbox and register it locally
- `sandbox_pause`: pause a sandbox by ID
- `sandbox_kill`: terminate a sandbox by ID
- `sandbox_close`: close local resources for a sandbox, does not terminate it
- `sandbox_get_info`: fetch sandbox info by ID
- `sandbox_list`: list sandboxes with optional `filter` object
- `sandbox_renew`: extend sandbox expiration
- `sandbox_healthcheck`: check if sandbox is healthy
- `sandbox_get_metrics`: get resource metrics
- `sandbox_get_endpoint`: get network endpoint for a port

### Command Execution

- `command_run`: run a command inside a sandbox
- `command_run_stream`: run a command and stream logs via MCP notifications
- `command_interrupt`: interrupt a running command

### Filesystem

- `file_read_text`: read a text file
- `file_read_bytes`: read a binary file as base64
- `file_write_text`: write a text file
- `file_write_bytes`: write a binary file from base64
- `file_write_files`: batch write files
- `file_delete`: delete files
- `file_get_info`: get file metadata
- `file_search`: search for files by glob
- `file_create_directories`: create directories
- `file_delete_directories`: delete directories
- `file_move`: move/rename files or directories
- `file_set_permissions`: set permissions/ownership
- `file_replace_contents`: replace file content by pattern

## 5. Usage Examples

Here are some examples of what you can ask an LLM to do:

- "Create a Python sandbox, scrape a simple HTML page, and summarize the key points."
- "Download a GitHub repo, install dependencies, and run its tests."
- "Generate a CSV file with fake sales data and plot a quick summary."
- "Start a tiny web server on port 8000 and return the public URL."
- "Build a minimal REST API with hello and health, then expose it on port 8000."
- "Search /app for TODOs and return the matching file paths."
- "Batch resize images in /data into /out."
- "Run a short Python script that prints the first 20 primes."
- "Create a tar.gz of /app and report the file size."
- "Clean up all sandboxes you created in this session."
