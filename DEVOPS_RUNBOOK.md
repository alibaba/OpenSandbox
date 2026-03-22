# SuperSandbox — DevOps Deployment Runbook

## Overview

SuperSandbox is a sandbox platform that runs isolated Linux containers on Kubernetes with gVisor for syscall-level security. It provides:
- Sandbox lifecycle (create / pause / resume / delete)
- Persistent `/workspace` volume per sandbox (survives pause/resume)
- Pod logs API
- Interactive WebSocket terminal

## Architecture

```
Client (API / WebSocket)
    ↓
SuperSandbox Server (FastAPI, port 8080)
    ↓
Kubernetes API
    ↓
agent-sandbox-controller (reconciles Sandbox CRs → Pods)
    ↓
Pods (gVisor runtime) + PVCs (workspace storage)
```

---

## Prerequisites

| Component | Version | Purpose |
|-----------|---------|---------|
| Kubernetes | 1.21+ | Cluster |
| containerd | 2.x | Container runtime (NOT Docker runtime) |
| gVisor (runsc) | latest | Sandbox isolation |
| agent-sandbox-controller | v0.2.1 | Sandbox CRD → Pod reconciliation |
| Python | 3.10+ | SuperSandbox server |
| uv | latest | Python package manager |

---

## Step 1: Kubernetes Cluster Requirements

The cluster MUST use **containerd** as the container runtime (not dockershim).

Verify:
```bash
kubectl get nodes -o jsonpath='{.items[*].status.nodeInfo.containerRuntimeVersion}'
# Should show: containerd://2.x.x
```

---

## Step 2: Install gVisor

gVisor provides syscall-level isolation. Each sandbox pod runs inside a gVisor sandbox.

### On each worker node:

```bash
# Install runsc binary (adjust arch: amd64 or aarch64)
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then ARCH="amd64"; fi
if [ "$ARCH" = "aarch64" ]; then ARCH="aarch64"; fi

curl -fsSL -o /usr/local/bin/runsc \
  https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}/runsc
chmod +x /usr/local/bin/runsc

curl -fsSL -o /usr/local/bin/containerd-shim-runsc-v1 \
  https://storage.googleapis.com/gvisor/releases/release/latest/${ARCH}/containerd-shim-runsc-v1
chmod +x /usr/local/bin/containerd-shim-runsc-v1
```

### Configure containerd to use runsc:

Add to `/etc/containerd/config.toml`:
```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runsc]
  runtime_type = "io.containerd.runsc.v1"
```

Then restart containerd:
```bash
systemctl restart containerd
```

### Create the RuntimeClass:

```bash
kubectl apply -f - <<EOF
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
EOF
```

### Verify:
```bash
kubectl get runtimeclass gvisor
# Should show: gvisor   runsc
```

---

## Step 3: Install agent-sandbox-controller

This controller watches `Sandbox` CRDs and creates/manages pods.

```bash
kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/download/v0.2.1/manifest.yaml
```

This creates:
- Namespace: `agent-sandbox-system`
- CRD: `sandboxes.agents.x-k8s.io`
- Controller deployment + RBAC

Verify:
```bash
kubectl get pods -n agent-sandbox-system
# Should show controller running
```

---

## Step 4: Create opensandbox namespace

```bash
kubectl create namespace opensandbox
```

---

## Step 5: Pre-pull images on worker nodes

To avoid pod creation timeouts:
```bash
# On each worker node, or via DaemonSet:
ctr -n k8s.io images pull docker.io/library/python:3.11-slim
ctr -n k8s.io images pull docker.io/opensandbox/execd:v1.0.6
```

---

## Step 6: Deploy SuperSandbox Server

### Clone and install:
```bash
git clone git@github.com:superagi/SuperSandbox.git
cd SuperSandbox/server
python -m venv .venv
source .venv/bin/activate
uv sync
uv pip install websockets  # Required for WebSocket terminal support
```

### Configure `~/.sandbox.toml`:

```toml
[server]
host = "0.0.0.0"         # Bind to all interfaces for production
port = 8080
log_level = "INFO"
# api_key = "your-secret-key"  # Uncomment for production

[runtime]
type = "kubernetes"
execd_image = "opensandbox/execd:v1.0.6"

[storage]
allowed_host_paths = []
default_workspace_volume_size = "1Gi"    # PVC size per sandbox

[kubernetes]
kubeconfig_path = ""                      # Empty = in-cluster config
namespace = "opensandbox"
informer_enabled = true
informer_resync_seconds = 300
informer_watch_timeout_seconds = 60
pod_ready_timeout_seconds = 300
pod_ready_poll_interval_seconds = 2
workload_provider = "agent-sandbox"

[secure_runtime]
type = "gvisor"
k8s_runtime_class = "gvisor"

[ingress]
mode = "direct"
```

> **Note**: For in-cluster deployment, leave `kubeconfig_path = ""`. The server will use the pod's service account. Ensure the service account has permissions to manage Sandbox CRDs, PVCs, and Pods in the `opensandbox` namespace.

### RBAC for in-cluster deployment:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: supersandbox-server
  namespace: opensandbox
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: supersandbox-server
rules:
  - apiGroups: ["agents.x-k8s.io"]
    resources: ["sandboxes"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["pods", "pods/log", "pods/exec"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "create", "delete"]
  - apiGroups: ["node.k8s.io"]
    resources: ["runtimeclasses"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["services", "secrets"]
    verbs: ["get", "list", "create", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: supersandbox-server
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: supersandbox-server
subjects:
  - kind: ServiceAccount
    name: supersandbox-server
    namespace: opensandbox
```

### Start the server:
```bash
opensandbox-server --config ~/.sandbox.toml
```

Or via Docker/K8s deployment (containerize the server directory).

---

## Step 7: Verify

```bash
# 1. Health check
curl http://<server-host>:8080/health

# 2. Create a sandbox
curl -X POST http://<server-host>:8080/sandboxes \
  -H "Content-Type: application/json" \
  -d '{
    "image": {"uri": "python:3.11-slim"},
    "timeout": 3600,
    "resourceLimits": {"cpu": "500m", "memory": "512Mi"},
    "entrypoint": ["python", "-c", "import time; time.sleep(3600)"]
  }'

# 3. Check pod is running with gVisor
kubectl get pods -n opensandbox -o jsonpath='{.items[*].spec.runtimeClassName}'
# Should show: gvisor

# 4. Check PVC created
kubectl get pvc -n opensandbox

# 5. Test logs
curl "http://<server-host>:8080/sandboxes/<id>/logs?tail=10"

# 6. Test pause/resume
curl -X POST http://<server-host>:8080/sandboxes/<id>/pause
kubectl get pods -n opensandbox    # Should be empty
kubectl get pvc -n opensandbox     # PVC still exists

curl -X POST http://<server-host>:8080/sandboxes/<id>/resume
kubectl get pods -n opensandbox    # Pod recreated

# 7. Delete
curl -X DELETE http://<server-host>:8080/sandboxes/<id>
```

---

## API Quick Reference

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/sandboxes` | POST | Create sandbox |
| `/sandboxes` | GET | List sandboxes |
| `/sandboxes/{id}` | GET | Get sandbox status |
| `/sandboxes/{id}` | DELETE | Delete sandbox + PVC |
| `/sandboxes/{id}/pause` | POST | Pause (scale to 0, keep PVC) |
| `/sandboxes/{id}/resume` | POST | Resume (scale to 1, remount PVC) |
| `/sandboxes/{id}/logs?tail=100&follow=false` | GET | Pod logs |
| `/sandboxes/{id}/terminal` | WebSocket | Interactive bash terminal |
| `/sandboxes/{id}/endpoints/{port}` | GET | Get pod IP:port |

---

## Production Considerations

- **Storage class**: Default uses `standard`. For production, configure a proper StorageClass (e.g., `gp3` on AWS, `pd-ssd` on GCP)
- **API key**: Set `api_key` in `[server]` config section
- **Resource quotas**: Set ResourceQuota on the `opensandbox` namespace to limit total sandbox resources
- **Network policies**: Consider adding NetworkPolicies to isolate sandbox pods
- **Monitoring**: The server exposes `/health` for liveness probes
- **Volume size**: Adjust `default_workspace_volume_size` in config based on your use case
