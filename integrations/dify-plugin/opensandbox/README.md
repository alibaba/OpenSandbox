# OpenSandbox Dify Plugin (Tool)

This plugin lets Dify call a **self-hosted OpenSandbox server** to create, run, and
terminate sandboxes.

## Features (MVP)
- `sandbox_create`: create a sandbox and return its id
- `sandbox_run`: execute a command in an existing sandbox
- `sandbox_kill`: terminate a sandbox by id

## Requirements
- Python 3.12+
- Dify plugin runtime
- OpenSandbox server reachable by URL

## Local Testing (Dify docker-compose)

### 1) Start OpenSandbox Server
Run OpenSandbox locally with Docker runtime enabled and an API key.

Example config (adjust to your setup):
```toml
[server]
host = "0.0.0.0"
port = 8080
api_key = "your-open-sandbox-key"

[runtime]
type = "docker"
execd_image = "opensandbox/execd:v1.0.5"

[docker]
network_mode = "bridge"
```

### 2) Start Dify (official docker-compose)
Follow the official Dify self-hosted docker-compose guide to start a local Dify instance.

### 3) Enable Plugin Remote Debug in Dify UI
- Open Dify UI → **Plugins** → **Develop** (or **Debug**)
- Copy the **Remote Install URL** and **Remote Install Key**

Create `.env` in this plugin directory (do not commit it):
```bash
INSTALL_METHOD=remote
REMOTE_INSTALL_URL=debug.dify.ai:5003
REMOTE_INSTALL_KEY=your-debug-key
```

### 4) Run the Plugin
```bash
pip install -r requirements.txt
python -m main
```

### 5) Configure Provider Credentials in Dify
Set:
- **OpenSandbox base URL**: `http://localhost:8080`
- **OpenSandbox API Key**: `your-open-sandbox-key`

Then use the tools in a workflow:
1. `sandbox_create`
2. `sandbox_run`
3. `sandbox_kill`

## E2E Testing

Automated end-to-end tests are available in `tests/e2e/dify_plugin/`. These tests:
- Start OpenSandbox server and Dify
- Register the plugin via remote debugging
- Import and run a test workflow
- Verify sandbox operations work correctly

See `tests/e2e/dify_plugin/README.md` for details.

CI runs these tests automatically on changes to the plugin code. See `.github/workflows/dify-plugin-e2e.yml`.

## Notes
- The base URL should **not** include `/v1`.
- The plugin itself does **not** host OpenSandbox; it connects to your server.
