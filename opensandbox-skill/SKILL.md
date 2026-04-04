---
name: opensandbox
description: >-
  Use OpenSandbox to create isolated container sandboxes for running code, commands, and file operations.
  Covers sandbox lifecycle (create, execute, read/write files, cleanup) via Python SDK or REST API.
  Use this skill whenever the user wants to run code in a sandbox, create an isolated execution environment,
  execute untrusted code safely, set up a coding agent environment, or interact with OpenSandbox in any way.
  Also triggers for: "run this in a container", "isolated environment", "sandbox execution",
  "execute code safely", "set up a dev sandbox", "code interpreter", or any mention of OpenSandbox.
---

# OpenSandbox Skill

OpenSandbox is a general-purpose sandbox platform that manages isolated container environments.
You interact with it through a Python SDK (`opensandbox`) or REST API to create sandboxes,
run commands, manage files, and clean up — all inside Docker or Kubernetes containers.

This skill teaches you how to do that efficiently without needing to look up the docs each time.

## Step 0: Resolve Connection (MUST DO FIRST)

Before any OpenSandbox operation, you need a server address (`domain`) and optionally an API key.
These get resolved once and cached — you won't need to ask again.

### Resolution order:

1. **Check config file** `~/.opensandbox.json` — if it exists and has `domain`, use it
2. **Check environment variables** — `OPEN_SANDBOX_DOMAIN` and `OPEN_SANDBOX_API_KEY`
3. **Ask the user** — if neither source has the info, ask before proceeding

### How to resolve (run this logic before your first OpenSandbox call):

```python
import json, os
from pathlib import Path

def load_opensandbox_config() -> dict:
    """Load OpenSandbox connection config. Returns dict with 'domain', 'api_key', 'protocol'."""
    config_path = Path.home() / ".opensandbox.json"
    config = {}

    # 1. Try config file
    if config_path.exists():
        with open(config_path) as f:
            config = json.load(f)

    # 2. Env vars override (if set)
    if os.getenv("OPEN_SANDBOX_DOMAIN"):
        config["domain"] = os.environ["OPEN_SANDBOX_DOMAIN"]
    if os.getenv("OPEN_SANDBOX_API_KEY"):
        config["api_key"] = os.environ["OPEN_SANDBOX_API_KEY"]

    return config  # may be empty — caller should check and ask user
```

### If `domain` is missing, ask the user:

> "I need your OpenSandbox server address to proceed. This is the URL where your OpenSandbox
> server is running (e.g., `sandbox.mycompany.com` or `localhost:8080` for local dev).
> If you also have an API key, please share that too. I'll save these so you won't need
> to provide them again."

### Save for next time (after getting the info):

```python
def save_opensandbox_config(domain: str, api_key: str | None = None, protocol: str = "http") -> None:
    """Persist connection config to ~/.opensandbox.json."""
    config_path = Path.home() / ".opensandbox.json"
    config = {}
    if config_path.exists():
        with open(config_path) as f:
            config = json.load(f)
    config["domain"] = domain
    if api_key:
        config["api_key"] = api_key
    config["protocol"] = protocol
    config_path.write_text(json.dumps(config, indent=2))
```

### Then build your ConnectionConfig:

```python
from opensandbox.config import ConnectionConfig

cfg = load_opensandbox_config()
# If cfg is empty or missing 'domain', ask the user first, then save_opensandbox_config(...)

config = ConnectionConfig(
    domain=cfg["domain"],
    api_key=cfg.get("api_key"),
    protocol=cfg.get("protocol", "http"),
)
```

**Key points:**
- `localhost:8080` is only valid for local dev — never assume it silently for remote setups
- If the user says "use my sandbox", you still need to resolve where it is
- The config file persists across sessions — once saved, it's automatic
- `api_key` is optional for local dev (server may not require auth)

### Config file format (`~/.opensandbox.json`):

```json
{
  "domain": "sandbox.mycompany.com",
  "api_key": "sk-xxx",
  "protocol": "https"
}
```

## Core Workflow

Every interaction with OpenSandbox follows this pattern:

```
1. CREATE sandbox  →  get sandbox_id
2. EXECUTE commands / WRITE files  →  do your work
3. READ results  →  collect output
4. KILL sandbox  →  clean up
```

---

## 1. Create a Sandbox

### Python SDK (recommended)

```python
import asyncio
from datetime import timedelta
from opensandbox import Sandbox

# config = ConnectionConfig(...) from Step 0 above

sandbox = await Sandbox.create(
    "opensandbox/code-interpreter:v1.0.2",  # Docker image
    connection_config=config,
    timeout=timedelta(minutes=30),           # auto-cleanup TTL
    entrypoint=["/opt/opensandbox/code-interpreter.sh"],
    env={"MY_VAR": "value"},                 # injected as container env vars
    resource={"cpu": "1", "memory": "2Gi"},  # resource limits
)
# sandbox.id is now your sandbox_id
```

### REST API (curl)

```bash
# Use $DOMAIN and $API_KEY from ~/.opensandbox.json or ask the user
curl -X POST "http://$DOMAIN/v1/sandboxes" \
  -H "OPEN-SANDBOX-API-KEY: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "image": {"uri": "opensandbox/code-interpreter:v1.0.2"},
    "entrypoint": ["/opt/opensandbox/code-interpreter.sh"],
    "timeout": 1800,
    "resourceLimits": {"cpu": "1", "memory": "2Gi"},
    "env": {"MY_VAR": "value"}
  }'
# Response includes "id": "sandbox-uuid"
```

### Common Images

| Image | What's inside | Good for |
|-------|--------------|----------|
| `opensandbox/code-interpreter:v1.0.2` | Python 3.10-3.14, Node.js 18/20/22, Java 8/11/17/21, Go 1.23-1.25 | General code execution, agent CLI tools |
| `python:3.11-slim` | Just Python | Lightweight Python scripts |
| `ubuntu` | Bare Ubuntu | Custom setups |
| `node:20` | Just Node.js | JavaScript/TypeScript tasks |

The code-interpreter image is the most versatile — it has Node.js (for npm-based agent CLIs like Claude Code, Codex, Gemini CLI) and multi-language support.

### Creation Parameters

| Parameter | Type | Default | Notes |
|-----------|------|---------|-------|
| `image` | str or ImageSpec | required | Docker image URI |
| `timeout` | timedelta | 10 min | TTL before auto-kill. `None` = manual cleanup |
| `entrypoint` | list[str] | `["tail", "-f", "/dev/null"]` | Container entrypoint |
| `env` | dict | `{}` | Environment variables (auth tokens, config) |
| `resource` | dict | `{"cpu": "1", "memory": "2Gi"}` | CPU/memory limits |
| `metadata` | dict | `{}` | Custom tags for filtering |
| `network_policy` | NetworkPolicy | None | Egress allow/deny rules |

---

## 2. Execute Commands

### Run a command and get output

```python
execution = await sandbox.commands.run("echo 'Hello World'")

# Read stdout
for msg in execution.logs.stdout:
    print(msg.text)

# Read stderr
for msg in execution.logs.stderr:
    print(msg.text)

# Check for errors
if execution.error:
    print(f"Error: {execution.error.name}: {execution.error.value}")
```

### Run with working directory

```python
from opensandbox.models.execd import RunCommandOpts

execution = await sandbox.commands.run(
    "python main.py",
    opts=RunCommandOpts(working_directory="/app"),
)
```

### Stream output in real-time

```python
from opensandbox.models.execd import ExecutionHandlers

async def on_stdout(msg):
    print(f"→ {msg.text}")

async def on_stderr(msg):
    print(f"⚠ {msg.text}")

execution = await sandbox.commands.run(
    "pip install requests && python fetch.py",
    handlers=ExecutionHandlers(on_stdout=on_stdout, on_stderr=on_stderr),
)
```

### Install tools at runtime

Agent CLIs are not pre-installed — install them after sandbox creation:

```python
# Claude Code
await sandbox.commands.run("npm i -g @anthropic-ai/claude-code@latest")

# OpenAI Codex
await sandbox.commands.run("npm install -g @openai/codex@latest")

# Gemini CLI
await sandbox.commands.run("npm install -g @google/gemini-cli@latest")

# Python packages
await sandbox.commands.run("pip install pandas numpy")
```

Auth for these CLIs comes from env vars injected at sandbox creation time
(e.g., `ANTHROPIC_AUTH_TOKEN`, `OPENAI_API_KEY`, `GEMINI_API_KEY`).

---

## 3. File Operations

### Write a file

```python
await sandbox.files.write_file("/app/script.py", "print('hello')", mode=644)
```

### Write multiple files

```python
from opensandbox.models.filesystem import WriteEntry

await sandbox.files.write_files([
    WriteEntry(path="/app/main.py", data="import sys; print(sys.version)", mode=644),
    WriteEntry(path="/app/config.json", data='{"debug": true}', mode=644),
])
```

### Read a file

```python
content = await sandbox.files.read_file("/app/output.txt")
```

### Create directories

```python
from opensandbox.models.filesystem import WriteEntry

await sandbox.files.create_directories([
    WriteEntry(path="/app/src", mode=755),
    WriteEntry(path="/app/data", mode=755),
])
```

### Search files

```python
from opensandbox.models.filesystem import SearchEntry

files = await sandbox.files.search(SearchEntry(path="/app", pattern="*.py"))
for f in files:
    print(f"{f.path} ({f.size} bytes)")
```

### Delete files

```python
await sandbox.files.delete_files(["/app/temp.txt", "/app/old.log"])
```

---

## 4. Clean Up

```python
# Kill the remote sandbox (terminates container)
await sandbox.kill()

# Close local HTTP client resources
await sandbox.close()
```

### Using context manager (recommended)

```python
async with sandbox:
    await sandbox.commands.run("python script.py")
    # close() called automatically on exit

# But you still need to kill explicitly:
await sandbox.kill()
```

### Full lifecycle pattern

```python
sandbox = await Sandbox.create("python:3.11", connection_config=config)
try:
    async with sandbox:
        await sandbox.files.write_file("/app/run.py", code)
        result = await sandbox.commands.run("python /app/run.py")
        output = "\n".join(m.text for m in result.logs.stdout)
finally:
    await sandbox.kill()
```

---

## 5. Other Useful Operations

### Get sandbox info

```python
info = await sandbox.get_info()
print(f"State: {info.status.state}")  # Running, Paused, Terminated, etc.
print(f"Expires: {info.expires_at}")
```

### Renew sandbox TTL

```python
await sandbox.renew(timedelta(minutes=30))
```

### Pause and resume

```python
await sandbox.pause()
# ... later ...
sandbox = await Sandbox.resume(sandbox.id, connection_config=config)
```

### Connect to existing sandbox

```python
sandbox = await Sandbox.connect("sandbox-uuid", connection_config=config)
```

### Get network endpoint (for exposed ports)

```python
endpoint = await sandbox.get_endpoint(8000)
print(f"Access at: http://{endpoint.endpoint}")
```

### Get resource metrics

```python
metrics = await sandbox.get_metrics()
print(f"CPU: {metrics.cpu_used_in_percent}%, Memory: {metrics.memory_used_in_mib}MB")
```

---

## Complete Example: Run Python Code in a Sandbox

```python
import asyncio
from datetime import timedelta
from opensandbox import Sandbox
from opensandbox.config import ConnectionConfig

async def main():
    cfg = load_opensandbox_config()
    # If cfg is empty, ask user for domain/api_key, then save_opensandbox_config(...)
    config = ConnectionConfig(
        domain=cfg["domain"],
        api_key=cfg.get("api_key"),
        protocol=cfg.get("protocol", "http"),
    )

    sandbox = await Sandbox.create(
        "opensandbox/code-interpreter:v1.0.2",
        connection_config=config,
        entrypoint=["/opt/opensandbox/code-interpreter.sh"],
        timeout=timedelta(minutes=10),
    )

    try:
        async with sandbox:
            # Write a script
            await sandbox.files.write_file("/tmp/analyze.py", """
import json
data = [1, 2, 3, 4, 5]
result = {"sum": sum(data), "mean": sum(data)/len(data), "count": len(data)}
print(json.dumps(result))
""")

            # Run it
            execution = await sandbox.commands.run("python /tmp/analyze.py")
            print(execution.logs.stdout[0].text)
            # {"sum": 15, "mean": 3.0, "count": 5}
    finally:
        await sandbox.kill()

asyncio.run(main())
```

## Complete Example: Set Up Claude Code Agent

```python
import asyncio, os
from datetime import timedelta
from opensandbox import Sandbox
from opensandbox.config import ConnectionConfig

async def main():
    cfg = load_opensandbox_config()
    config = ConnectionConfig(
        domain=cfg["domain"],
        api_key=cfg.get("api_key"),
        protocol=cfg.get("protocol", "http"),
    )

    sandbox = await Sandbox.create(
        "opensandbox/code-interpreter:v1.0.2",
        connection_config=config,
        entrypoint=["/opt/opensandbox/code-interpreter.sh"],
        timeout=timedelta(minutes=30),
        env={
            "ANTHROPIC_AUTH_TOKEN": os.environ["ANTHROPIC_AUTH_TOKEN"],
            "ANTHROPIC_MODEL": "claude_sonnet4",
        },
    )

    try:
        async with sandbox:
            # Install Claude Code CLI
            await sandbox.commands.run("npm i -g @anthropic-ai/claude-code@latest")

            # Use it
            result = await sandbox.commands.run('claude "Write a Python fibonacci function"')
            for msg in result.logs.stdout:
                print(msg.text)
    finally:
        await sandbox.kill()

asyncio.run(main())
```

## REST API Quick Reference

All endpoints are under `/v1` prefix. Auth header: `OPEN-SANDBOX-API-KEY: <key>`.

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `POST` | `/v1/sandboxes` | Create sandbox |
| `GET` | `/v1/sandboxes` | List sandboxes (filter by `?state=Running`) |
| `GET` | `/v1/sandboxes/{id}` | Get sandbox details |
| `DELETE` | `/v1/sandboxes/{id}` | Delete (kill) sandbox |
| `POST` | `/v1/sandboxes/{id}/pause` | Pause sandbox |
| `POST` | `/v1/sandboxes/{id}/resume` | Resume sandbox |
| `POST` | `/v1/sandboxes/{id}/renew-expiration` | Extend TTL |
| `GET` | `/v1/sandboxes/{id}/endpoints/{port}` | Get network endpoint |

The execd daemon (port 44772 inside the sandbox) handles command execution and file
operations — the SDK wraps this for you. If you need direct access:

| Method | Endpoint (execd :44772) | Purpose |
|--------|------------------------|---------|
| `POST` | `/command` | Run command (SSE stream) |
| `DELETE` | `/command?id={execId}` | Interrupt command |
| `POST` | `/files/upload` | Upload file (multipart) |
| `GET` | `/files/download?path={path}` | Download file |
| `GET` | `/files/info?path[]={path}` | File metadata |
| `GET` | `/files/search?path={dir}&pattern={glob}` | Search files |
| `POST` | `/directories` | Create directories |

## Troubleshooting

**"Connection refused" on create** — Is the OpenSandbox server running? Check `curl http://<your-domain>/health`. Also verify `~/.opensandbox.json` has the correct domain.

**Sandbox stuck in "Pending"** — The Docker image might be pulling. Pre-pull with `docker pull <image>`.

**Command timeout** — Default SDK request timeout is 30s. For long commands, increase it:
```python
config = ConnectionConfig(domain="...", request_timeout=timedelta(minutes=5))
```

**"Sandbox not found" after restart** — Sandboxes are ephemeral. After server restart, old sandbox IDs are gone. Create a new one.

**Private registry auth** — Pass auth at creation time:
```python
from opensandbox.models.sandboxes import SandboxImageSpec, SandboxImageAuth

image = SandboxImageSpec(
    "my-registry.com/my-image:v1",
    auth=SandboxImageAuth(username="user", password="token"),
)
sandbox = await Sandbox.create(image, connection_config=config)
```
