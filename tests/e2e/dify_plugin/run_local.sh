#!/usr/bin/env bash
# Copyright 2025 Alibaba Group Holding Ltd.
# SPDX-License-Identifier: Apache-2.0
#
# Local E2E test runner for Dify plugin.
# Prerequisites:
#   - Docker and Docker Compose installed
#   - Python 3.12+ available
#   - Ports 5001 (Dify) and 8080 (OpenSandbox) available

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
DIFY_COMPOSE_DIR="${DIFY_COMPOSE_DIR:-$SCRIPT_DIR/.dify}"
PLUGIN_DIR="$ROOT_DIR/integrations/dify-plugin/opensandbox"

# Configuration
export DIFY_PORT="${DIFY_PORT:-5001}"
# Use latest stable release tag (check https://github.com/langgenius/dify/releases)
export DIFY_REF="${DIFY_REF:-1.11.4}"
export USE_DOCKER_MIRROR="${USE_DOCKER_MIRROR:-true}"
export SKIP_DIFY_START="${SKIP_DIFY_START:-false}"  # Set to true if Dify is already running
export DIFY_CONSOLE_API_URL="${DIFY_CONSOLE_API_URL:-http://localhost:$DIFY_PORT}"
export DIFY_ADMIN_EMAIL="${DIFY_ADMIN_EMAIL:-admin@example.com}"
export DIFY_ADMIN_PASSWORD="${DIFY_ADMIN_PASSWORD:-ChangeMe123!}"
export OPEN_SANDBOX_BASE_URL="${OPEN_SANDBOX_BASE_URL:-http://localhost:8080}"
export OPEN_SANDBOX_API_KEY="${OPEN_SANDBOX_API_KEY:-opensandbox-e2e-key}"

echo "Configuration:"
echo "  DIFY_PORT: $DIFY_PORT"
echo "  DIFY_REF: $DIFY_REF"
echo "  USE_DOCKER_MIRROR: $USE_DOCKER_MIRROR"
echo "  SKIP_DIFY_START: $SKIP_DIFY_START"
echo ""

OPENSANDBOX_PID=""
DIFY_STARTED=""

cleanup() {
    echo "==> Cleaning up..."
    
    # Stop OpenSandbox server
    if [[ -n "$OPENSANDBOX_PID" ]] && kill -0 "$OPENSANDBOX_PID" 2>/dev/null; then
        echo "    Stopping OpenSandbox server (PID: $OPENSANDBOX_PID)"
        kill "$OPENSANDBOX_PID" 2>/dev/null || true
        wait "$OPENSANDBOX_PID" 2>/dev/null || true
    fi
    
    # Stop Dify
    if [[ -n "$DIFY_STARTED" ]] && [[ -d "$DIFY_COMPOSE_DIR" ]]; then
        echo "    Stopping Dify..."
        cd "$DIFY_COMPOSE_DIR"
        docker compose down --volumes --remove-orphans 2>/dev/null || true
    fi
    
    echo "==> Cleanup complete"
}

trap cleanup EXIT

echo "==> Step 1: Prepare Dify docker-compose files"
cd "$SCRIPT_DIR"
python3 prepare_dify_compose.py
echo "    Dify compose files ready at: $DIFY_COMPOSE_DIR"

echo "==> Step 2: Start Dify"
if [[ "$SKIP_DIFY_START" == "true" ]]; then
    echo "    SKIP_DIFY_START=true, assuming Dify is already running"
else
    cd "$DIFY_COMPOSE_DIR"

    # Pull images with retry
    echo "    Pulling Dify images (this may take a while)..."
    echo "    TIP: If pull fails, configure Docker mirror or set SKIP_DIFY_START=true"
    for i in 1 2 3; do
        if docker compose pull 2>&1; then
            break
        fi
        if [[ $i -eq 3 ]]; then
            echo ""
            echo "    ERROR: Failed to pull Dify images after 3 attempts"
            echo "    This is likely a network issue (Docker Hub not accessible)."
            echo ""
            echo "    Solutions:"
            echo "    1. Configure Docker mirror in ~/.docker/daemon.json:"
            echo '       {"registry-mirrors": ["https://mirror.ccs.tencentyun.com"]}'
            echo "    2. Or start Dify manually and run with SKIP_DIFY_START=true"
            echo ""
            exit 1
        fi
        echo "    Pull attempt $i failed, retrying..."
        sleep 5
    done

    docker compose up -d
    DIFY_STARTED="1"
    echo "    Dify starting on port $DIFY_PORT..."
fi

# Wait for Dify API to be ready
echo "    Waiting for Dify API to be ready..."
for i in {1..60}; do
    if curl -s "http://localhost:$DIFY_PORT/console/api/ping" 2>/dev/null | grep -q pong; then
        echo "    Dify API is ready"
        break
    fi
    if [[ $i -eq 60 ]]; then
        echo "    ERROR: Dify API not responding within timeout"
        if [[ -n "$DIFY_STARTED" ]]; then
            echo "    Container status:"
            docker compose -f "$DIFY_COMPOSE_DIR/docker-compose.yaml" ps -a
            echo "    Container logs (last 50 lines):"
            docker compose -f "$DIFY_COMPOSE_DIR/docker-compose.yaml" logs --tail=50
        fi
        exit 1
    fi
    echo "    Waiting for Dify API... ($i/60)"
    sleep 5
done

echo "==> Step 3: Configure and start OpenSandbox server"

# Detect architecture and choose appropriate image
ARCH="$(uname -m)"
if [[ "$ARCH" == "arm64" || "$ARCH" == "aarch64" ]]; then
    # On ARM64, use official image (has multi-platform support) or build locally
    EXECD_IMAGE="${EXECD_IMAGE:-opensandbox/execd:v1.0.5}"
    echo "    Detected ARM64, using image: $EXECD_IMAGE"
else
    # On amd64, can use mirror
    EXECD_IMAGE="${EXECD_IMAGE:-swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/opensandbox/execd:v1.0.5}"
    echo "    Detected amd64, using image: $EXECD_IMAGE"
fi

# Create config file
cat > "$ROOT_DIR/server/.sandbox.e2e.toml" <<EOF
[server]
host = "0.0.0.0"
port = 8080
log_level = "INFO"
api_key = "$OPEN_SANDBOX_API_KEY"

[runtime]
type = "docker"
execd_image = "$EXECD_IMAGE"

[docker]
network_mode = "bridge"
EOF

cd "$ROOT_DIR/server"

# Install dependencies if needed
if [[ ! -d ".venv" ]]; then
    echo "    Installing server dependencies..."
    uv sync
fi

# Start server in background
echo "    Starting OpenSandbox server..."
SANDBOX_CONFIG_PATH="$ROOT_DIR/server/.sandbox.e2e.toml" \
    uv run python -m src.main > "$ROOT_DIR/server/server-e2e.log" 2>&1 &
OPENSANDBOX_PID=$!
echo "    OpenSandbox server started (PID: $OPENSANDBOX_PID)"

echo "==> Step 4: Install plugin dependencies"
cd "$PLUGIN_DIR"
if [[ ! -d ".venv" ]]; then
    python3 -m venv .venv
fi
source .venv/bin/activate
pip install -q -r requirements.txt
deactivate

echo "==> Step 5: Install e2e test dependencies"
cd "$SCRIPT_DIR"
if [[ ! -d ".venv" ]]; then
    python3 -m venv .venv
fi
source .venv/bin/activate
pip install -q -r requirements.txt

echo "==> Step 6: Run E2E test"
cd "$SCRIPT_DIR"
python3 run_e2e.py
deactivate

echo ""
echo "========================================="
echo "  E2E TEST PASSED"
echo "========================================="
