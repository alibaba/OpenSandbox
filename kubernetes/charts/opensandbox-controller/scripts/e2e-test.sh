#!/bin/bash
# OpenSandbox Helm Chart End-to-End Test Script

set -e

# Get the parent directory of the script's directory (chart root directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$(dirname "$SCRIPT_DIR")"
NAMESPACE="opensandbox"
RELEASE_NAME="opensandbox-e2e-test"
VALUES_FILE="${1:-values-e2e.yaml}"
PORT_FORWARD_PID=""
PF_LOG="/tmp/opensandbox-pf-$$.log"
SERVER_LOCAL_PORT=18088
SERVER_URL="http://localhost:${SERVER_LOCAL_PORT}"

# Cleanup function: ensure port-forward and temporary files are cleaned up
cleanup() {
    if [ -n "$PORT_FORWARD_PID" ]; then
        # Kill setsid and its child processes
        if [ "$USE_SUDO" = true ]; then
            sudo pkill -P $PORT_FORWARD_PID 2>/dev/null || true
            sudo kill $PORT_FORWARD_PID 2>/dev/null || true
        else
            pkill -P $PORT_FORWARD_PID 2>/dev/null || true
            kill $PORT_FORWARD_PID 2>/dev/null || true
        fi
    fi
    rm -f "$PF_LOG" 2>/dev/null || true
}

# Register cleanup function to ensure execution on script exit
trap cleanup EXIT INT TERM

# Check if sudo is required
USE_SUDO=false
if ! kubectl get nodes &> /dev/null 2>&1; then
    if sudo kubectl get nodes &> /dev/null 2>&1; then
        echo "Detected sudo privileges required, will use sudo for commands"
        USE_SUDO=true
    else
        echo "Error: Unable to access Kubernetes cluster"
        exit 1
    fi
fi

# Define command functions
kubectl_cmd() {
    if [ "$USE_SUDO" = true ]; then
        sudo kubectl "$@"
    else
        kubectl "$@"
    fi
}

helm_cmd() {
    if [ "$USE_SUDO" = true ]; then
        sudo helm "$@"
    else
        helm "$@"
    fi
}

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}OpenSandbox Helm Chart E2E Validation${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""
echo "Test Coverage:"
echo "  1. Helm Install (using ${VALUES_FILE})"
echo "  2. Server Deployment Verification"
echo "  3. Pool Deployment Verification"
echo "  4. SDK Integration Verification"
echo "  5. Helm Uninstall"
echo ""
echo "Environment Info:"
echo "  Chart: ${CHART_DIR}"
echo "  Values: ${VALUES_FILE}"
echo "  Release: ${RELEASE_NAME}"
echo "  Namespace: ${NAMESPACE}"
echo "  Server: ${SERVER_URL} (port-forward)"
echo ""

# ==========================================
# Stage 1: Helm Install
# ==========================================
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}Stage 1: Helm Install${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""

echo -e "${YELLOW}[1.1] Checking for existing release...${NC}"
if helm_cmd list -n "$NAMESPACE" 2>/dev/null | grep -q "$RELEASE_NAME"; then
    echo "  Release already exists, uninstalling first..."
    helm_cmd uninstall "$RELEASE_NAME" -n "$NAMESPACE" 2>/dev/null || true
    sleep 5
fi
echo -e "${GREEN}✓ Check completed${NC}"
echo ""

echo -e "${YELLOW}[1.2] Installing Helm chart (using ${VALUES_FILE})...${NC}"
helm_cmd install "$RELEASE_NAME" "$CHART_DIR" \
    --values "$CHART_DIR/$VALUES_FILE" \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --wait \
    --timeout 3m 2>&1 | tail -5
echo -e "${GREEN}✓ Helm chart installed successfully${NC}"
echo ""

echo -e "${YELLOW}[1.3] Waiting for Controller to be ready...${NC}"
kubectl_cmd wait --for=condition=available \
    deployment/opensandbox-controller-manager \
    -n "$NAMESPACE" \
    --timeout=120s 2>/dev/null
echo -e "${GREEN}✓ Controller is ready${NC}"
echo ""

echo -e "${YELLOW}[1.4] Checking deployment status...${NC}"
kubectl_cmd get deployment -n "$NAMESPACE"
echo ""

echo -e "${GREEN}✅ Stage 1 Complete: Helm Install Successful${NC}"
echo ""

# ==========================================
# Stage 2: Server Deployment Verification
# ==========================================
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}Stage 2: Server Deployment Verification${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""

echo -e "${YELLOW}[2.1] Checking Server Service...${NC}"
SERVER_SERVICE_NAME=$(kubectl_cmd get svc -n "$NAMESPACE" -l app.kubernetes.io/component=server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$SERVER_SERVICE_NAME" ]; then
    echo -e "${RED}❌ Server Service does not exist${NC}"
    exit 1
fi
echo "  Server Service: ${SERVER_SERVICE_NAME}"
kubectl_cmd get svc "$SERVER_SERVICE_NAME" -n "$NAMESPACE"
echo ""

echo -e "${YELLOW}[2.2] Waiting for Server Pod to be ready...${NC}"
SERVER_DEPLOYMENT_NAME=$(kubectl_cmd get deployment -n "$NAMESPACE" -l app.kubernetes.io/component=server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$SERVER_DEPLOYMENT_NAME" ]; then
    echo -e "${RED}❌ Server Deployment does not exist${NC}"
    exit 1
fi
echo "  Server Deployment: ${SERVER_DEPLOYMENT_NAME}"
kubectl_cmd wait --for=condition=available \
    deployment/"$SERVER_DEPLOYMENT_NAME" \
    -n "$NAMESPACE" \
    --timeout=120s 2>/dev/null
echo -e "${GREEN}✓ Server Deployment is ready${NC}"
echo ""

echo -e "${YELLOW}[2.3] Checking Server Pod status...${NC}"
kubectl_cmd get pods -n "$NAMESPACE" -l app.kubernetes.io/component=server
echo ""

echo -e "${YELLOW}[2.4] Setting up Port Forward...${NC}"
# Use setsid to completely detach from TTY, preventing kubectl from outputting to terminal
if [ "$USE_SUDO" = true ]; then
    setsid sudo kubectl port-forward -n "$NAMESPACE" svc/"$SERVER_SERVICE_NAME" ${SERVER_LOCAL_PORT}:8080 </dev/null >"$PF_LOG" 2>&1 &
else
    setsid kubectl port-forward -n "$NAMESPACE" svc/"$SERVER_SERVICE_NAME" ${SERVER_LOCAL_PORT}:8080 </dev/null >"$PF_LOG" 2>&1 &
fi
PORT_FORWARD_PID=$!
echo "  Port-forward started (PID: $PORT_FORWARD_PID)"
echo ""

echo -e "${YELLOW}[2.5] Waiting for Port Forward to be ready...${NC}"
for i in $(seq 1 30); do
    if curl -s "${SERVER_URL}/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Port-forward is ready${NC}"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo -e "${RED}❌ Port-forward timed out${NC}"
        exit 1
    fi
    sleep 1
done
echo ""

echo -e "${YELLOW}[2.6] Testing Server API...${NC}"
HEALTH_RESPONSE=$(curl -s "${SERVER_URL}/health")
if [ -n "$HEALTH_RESPONSE" ]; then
    echo -e "${GREEN}✓ Server API responding normally: $HEALTH_RESPONSE${NC}"
else
    echo -e "${RED}❌ Server API not responding${NC}"
    exit 1
fi
echo ""

echo -e "${GREEN}✅ Stage 2 Complete: Server Deployment Verified${NC}"
echo ""

# ==========================================
# Stage 3: Pool Deployment Verification
# ==========================================
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}Stage 3: Pool Deployment Verification${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""

echo -e "${YELLOW}[3.1] Checking Pool resources...${NC}"
POOL_COUNT=$(kubectl_cmd get pool -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l)
echo "  Pool count: ${POOL_COUNT}"
if [ "$POOL_COUNT" -eq 0 ]; then
    echo -e "${RED}❌ No Pool resources found${NC}"
    exit 1
fi
kubectl_cmd get pool -n "$NAMESPACE"
echo ""

echo -e "${YELLOW}[3.2] Checking agent-pool status...${NC}"
if ! kubectl_cmd get pool agent-pool -n "$NAMESPACE" --no-headers 2>/dev/null | grep -q agent-pool; then
    echo -e "${RED}❌ agent-pool does not exist${NC}"
    exit 1
fi
echo -e "${GREEN}✓ agent-pool exists${NC}"
echo ""

echo -e "${YELLOW}[3.3] Viewing Pool detailed status...${NC}"
kubectl_cmd get pool agent-pool -n "$NAMESPACE" -o jsonpath='{.status}' 2>/dev/null | jq '.' 2>/dev/null || echo "  (jq not installed, skipping JSON formatting)"
echo ""

echo -e "${YELLOW}[3.4] Waiting for Pool Pods to be ready (up to 180 seconds)...${NC}"
TIMEOUT=180
ELAPSED=0
READY=false
while [ $ELAPSED -lt $TIMEOUT ]; do
    AVAILABLE=$(kubectl_cmd get pool agent-pool -n "$NAMESPACE" -o jsonpath='{.status.available}' 2>/dev/null || echo "")
    if [ -n "$AVAILABLE" ] && [ "$AVAILABLE" -gt 0 ]; then
        echo -e "${GREEN}✓ Pool has ${AVAILABLE} available Pods${NC}"
        READY=true
        break
    fi
    echo "  Waiting... (${ELAPSED}s/${TIMEOUT}s)"
    sleep 5
    ELAPSED=$((ELAPSED + 5))
done
if [ "$READY" = false ]; then
    echo -e "${RED}❌ Pool Pods not ready, timed out${NC}"
    echo ""
    echo "Viewing Pool events:"
    kubectl_cmd describe pool agent-pool -n "$NAMESPACE" | tail -20
    exit 1
fi
echo ""

echo -e "${YELLOW}[3.5] Viewing Pool Pods...${NC}"
kubectl_cmd get pods -l pool=agent-pool -n "$NAMESPACE"
echo ""

echo -e "${YELLOW}[3.6] Checking execd process in Pod...${NC}"
POD_NAME=$(kubectl_cmd get pods -l pool=agent-pool -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -n "$POD_NAME" ]; then
    echo "  Checking Pod: ${POD_NAME}"
    sleep 3
    if kubectl_cmd exec -n "$NAMESPACE" "$POD_NAME" -c sandbox-container -- pgrep -f execd > /dev/null 2>&1; then
        EXECD_PID=$(kubectl_cmd exec -n "$NAMESPACE" "$POD_NAME" -c sandbox-container -- pgrep -f execd 2>/dev/null)
        echo -e "${GREEN}✓ execd process is running (PID: ${EXECD_PID})${NC}"
    else
        echo -e "${YELLOW}⚠️  execd process not found, checking container logs...${NC}"
        kubectl_cmd logs -n "$NAMESPACE" "$POD_NAME" -c sandbox-container --tail=20 2>/dev/null || true
    fi
else
    echo -e "${YELLOW}⚠️  No agent-pool Pod found${NC}"
fi
echo ""

echo -e "${GREEN}✅ Stage 3 Complete: Pool Deployment Verified${NC}"
echo ""

# ==========================================
# Stage 4: SDK Integration Verification
# ==========================================
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}Stage 4: SDK Integration Verification${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""

echo -e "${YELLOW}[4.1] Creating SDK test script...${NC}"
SDK_DOMAIN="localhost:${SERVER_LOCAL_PORT}"

rm -f /tmp/e2e_sdk_test.py
cat > /tmp/e2e_sdk_test.py <<'EOF'
import asyncio
from datetime import timedelta
from opensandbox import Sandbox
from opensandbox.config import ConnectionConfig

async def main():
    print("=" * 60)
    print("SDK End-to-End Test")
    print("=" * 60)

    config = ConnectionConfig(domain="SDK_DOMAIN_PLACEHOLDER")

    print("\n[Test 1] Creating sandbox (using agent-pool)...")
    try:
        sandbox = await Sandbox.create(
            "nginx:latest",
            entrypoint=["/bin/sh", "-c", "sleep infinity"],
            env={"TEST": "e2e"},
            timeout=timedelta(minutes=10),
            ready_timeout=timedelta(minutes=5),
            connection_config=config,
            extensions={"poolRef": "agent-pool"}
        )
        print(f"✅ Sandbox created successfully: {sandbox.id}")
    except Exception as e:
        print(f"❌ Sandbox creation failed: {e}")
        import traceback
        traceback.print_exc()
        return False

    try:
        print("\n[Test 2] Executing command...")
        execution = await sandbox.commands.run("echo 'Hello from E2E test'")
        if execution.logs.stdout:
            print(f"✅ Command executed successfully: {execution.logs.stdout[0].text}")
        else:
            print("⚠️  Command executed successfully but no output")

        print("\n[Test 3] File operations...")
        from opensandbox.models import WriteEntry
        await sandbox.files.write_files([
            WriteEntry(path="/tmp/e2e.txt", data="E2E Test", mode=644)
        ])
        print("✅ File written successfully")

        content = await sandbox.files.read_file("/tmp/e2e.txt")
        print(f"✅ File read successfully: {content}")

        print("\n[Test 4] Cleaning up sandbox...")
        await sandbox.kill()
        print("✅ Sandbox cleaned up successfully")

        print("\n" + "=" * 60)
        print("✅ All SDK end-to-end tests passed!")
        print("=" * 60)
        return True

    except Exception as e:
        print(f"❌ Test failed: {e}")
        import traceback
        traceback.print_exc()
        try:
            await sandbox.kill()
        except:
            pass
        return False

if __name__ == "__main__":
    success = asyncio.run(main())
    exit(0 if success else 1)
EOF

sed -i "s/SDK_DOMAIN_PLACEHOLDER/${SDK_DOMAIN}/g" /tmp/e2e_sdk_test.py
echo -e "${GREEN}✓ SDK test script created${NC}"
echo ""

echo -e "${YELLOW}[4.2] Running SDK test...${NC}"

# Detect SDK test environment
if [ -z "$SDK_TEST_DIR" ]; then
    for dir in "$HOME/sandbox-test" "/data/home/cz/sandbox-test" "$(pwd)/sandbox-test"; do
        if [ -d "$dir" ] && [ -f "$dir/pyproject.toml" ]; then
            SDK_TEST_DIR="$dir"
            break
        fi
    done
    if [ -z "$SDK_TEST_DIR" ]; then
        echo -e "${RED}❌ SDK test directory not found${NC}"
        echo "  Hint: Please set environment variable SDK_TEST_DIR to point to a directory containing pyproject.toml"
        echo "  Example: export SDK_TEST_DIR=\$HOME/sandbox-test"
        exit 1
    fi
fi

# Detect UV command
if [ -z "$UV_CMD" ]; then
    for uv_path in "uv" "/data/miniconda3/bin/uv" "$HOME/.cargo/bin/uv" "$HOME/.local/bin/uv"; do
        if command -v "$uv_path" &> /dev/null; then
            UV_CMD="$uv_path"
            break
        fi
    done
    if [ -z "$UV_CMD" ]; then
        echo -e "${RED}❌ uv command not found${NC}"
        echo "  Hint: Please set environment variable UV_CMD or install uv"
        echo "  Installation: curl -LsSf https://astral.sh/uv/install.sh | sh"
        exit 1
    fi
fi

echo "  SDK test directory: $SDK_TEST_DIR"
echo "  UV command: $UV_CMD"
echo ""

cd "$SDK_TEST_DIR"
if $UV_CMD run /tmp/e2e_sdk_test.py; then
    echo ""
    echo -e "${GREEN}✅ Stage 4 Complete: SDK Integration Verified${NC}"
else
    echo ""
    echo -e "${RED}❌ Stage 4 Failed: SDK Integration Failed${NC}"
    echo ""
    echo -e "${YELLOW}Diagnostic Information:${NC}"
    SERVER_POD=$(kubectl_cmd get pods -n "$NAMESPACE" -l app.kubernetes.io/component=server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -n "$SERVER_POD" ]; then
        echo "Server Pod: $SERVER_POD"
        echo ""
        echo "Server logs (last 50 lines):"
        kubectl_cmd logs -n "$NAMESPACE" "$SERVER_POD" --tail=50 2>/dev/null || true
    else
        echo -e "${RED}❌ Server Pod not found${NC}"
    fi
    exit 1
fi
echo ""

# ==========================================
# Stage 5: Helm Uninstall
# ==========================================
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}Stage 5: Helm Uninstall${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""

echo -e "${YELLOW}[5.1] Stopping Port Forward...${NC}"
if [ -n "$PORT_FORWARD_PID" ]; then
    if [ "$USE_SUDO" = true ]; then
        sudo pkill -P $PORT_FORWARD_PID 2>/dev/null || true
        sudo kill $PORT_FORWARD_PID 2>/dev/null || true
    else
        pkill -P $PORT_FORWARD_PID 2>/dev/null || true
        kill $PORT_FORWARD_PID 2>/dev/null || true
    fi
    PORT_FORWARD_PID=""
fi
# Clean up port-forward log file
rm -f "$PF_LOG" 2>/dev/null || true
echo -e "${GREEN}✓ Port-forward stopped${NC}"
echo ""

echo -e "${YELLOW}[5.2] Uninstalling Helm release...${NC}"
helm_cmd uninstall "$RELEASE_NAME" -n "$NAMESPACE"
echo -e "${GREEN}✓ Helm release uninstalled${NC}"
echo ""

echo "Waiting for resource cleanup..."
sleep 10

echo -e "${YELLOW}[5.3] Verifying resources are cleaned up...${NC}"
REMAINING_PODS=$(kubectl_cmd get pods -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l)
REMAINING_POOLS=$(kubectl_cmd get pools -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l)
echo "  Remaining Pods: ${REMAINING_PODS}"
echo "  Remaining Pools: ${REMAINING_POOLS}"

if [ "$REMAINING_PODS" -eq 0 ] && [ "$REMAINING_POOLS" -eq 0 ]; then
    echo -e "${GREEN}✓ All resources cleaned up${NC}"
else
    echo -e "${YELLOW}⚠️  Resources still remaining (Terminating)${NC}"
    if [ "$REMAINING_PODS" -gt 0 ]; then
        kubectl_cmd get pods -n "$NAMESPACE" 2>/dev/null || true
    fi
fi
echo ""

echo -e "${GREEN}✅ Stage 5 Complete: Helm Uninstall Successful${NC}"
echo ""

# ==========================================
# Test Summary
# ==========================================
echo -e "${GREEN}==========================================${NC}"
echo -e "${GREEN}End-to-End Test Complete!${NC}"
echo -e "${GREEN}==========================================${NC}"
echo ""
echo "Test Results:"
echo -e "  ${GREEN}✅ Stage 1: Helm Install - Success${NC}"
echo -e "  ${GREEN}✅ Stage 2: Server Deployment Verification - Success${NC}"
echo -e "  ${GREEN}✅ Stage 3: Pool Deployment Verification - Success${NC}"
echo -e "  ${GREEN}✅ Stage 4: SDK Integration Verification - Success${NC}"
echo -e "  ${GREEN}✅ Stage 5: Helm Uninstall - Success${NC}"
echo ""
echo -e "${GREEN}🎉 All tests passed!${NC}"
echo ""
