# Dify Plugin E2E Tests

End-to-end tests for the OpenSandbox Dify plugin integration.

## Overview

These tests verify the complete workflow:
1. Start OpenSandbox server
2. Start Dify (via docker-compose)
3. Connect plugin to Dify via remote debugging
4. Configure OpenSandbox credentials in Dify
5. Import and run a test workflow that:
   - Creates a sandbox
   - Runs a command (`echo opensandbox-e2e`)
   - Kills the sandbox
6. Verify the output contains expected text

## Files

- `run_e2e.py` - Main test script
- `workflow_template.yml` - Dify workflow DSL template
- `opensandbox.config.toml` - OpenSandbox server config for testing
- `prepare_dify_compose.py` - Downloads Dify docker-compose files
- `requirements.txt` - Python dependencies for e2e test
- `run_local.sh` - Local test runner (requires Docker)

## Running in CI

The tests run automatically via GitHub Actions when changes are made to:
- `integrations/dify-plugin/**`
- `tests/e2e/dify_plugin/**`
- `sdks/sandbox/python/**`
- `server/**`

See `.github/workflows/dify-plugin-e2e.yml` for the CI configuration.

## Running Locally

### Prerequisites

- Docker and Docker Compose
- Python 3.12+
- Network access to pull Dify images from Docker Hub

### Steps

```bash
# 1. Prepare Dify docker-compose files
cd tests/e2e/dify_plugin
python prepare_dify_compose.py

# 2. Start Dify
cd .dify
docker compose up -d
cd ..

# 3. Start OpenSandbox server (in another terminal)
cd server
cp ../tests/e2e/dify_plugin/opensandbox.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main

# 4. Install dependencies
pip install -r requirements.txt
cd ../integrations/dify-plugin/opensandbox
pip install -r requirements.txt
cd ../../../tests/e2e/dify_plugin

# 5. Run the test
python run_e2e.py
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DIFY_PORT` | `5001` | Dify console port |
| `DIFY_CONSOLE_API_URL` | `http://localhost:5001` | Dify console API URL |
| `DIFY_ADMIN_EMAIL` | `admin@example.com` | Admin email for Dify setup |
| `DIFY_ADMIN_PASSWORD` | `ChangeMe123!` | Admin password for Dify setup |
| `OPEN_SANDBOX_BASE_URL` | `http://localhost:8080` | OpenSandbox server URL |
| `OPEN_SANDBOX_API_KEY` | `opensandbox-e2e-key` | OpenSandbox API key |

## Troubleshooting

### Docker image pull failures

If you see errors like `Get "https://registry-1.docker.io/v2/": EOF`, this is a network issue (common in China). Solutions:

**Option 1: Configure Docker mirror**

Add registry mirrors to `~/.docker/daemon.json`:
```json
{
  "registry-mirrors": [
    "https://mirror.ccs.tencentyun.com",
    "https://docker.mirrors.ustc.edu.cn"
  ]
}
```
Then restart Docker.

**Option 2: Start Dify manually**

1. Clone Dify repo and start it manually:
```bash
git clone https://github.com/langgenius/dify.git
cd dify/docker
cp .env.example .env
docker compose up -d
```

2. Run the E2E test with `SKIP_DIFY_START=true`:
```bash
SKIP_DIFY_START=true ./run_local.sh
```

### Plugin not found

If the plugin doesn't appear in Dify:
1. Check that Dify's plugin daemon is running (`docker compose logs plugin_daemon`)
2. Verify the debugging key is valid
3. Check plugin process logs

### Workflow execution fails

If the workflow fails:
1. Check OpenSandbox server is healthy (`curl http://localhost:8080/health`)
2. Verify credentials are correctly configured in Dify
3. Check OpenSandbox server logs for errors
