# Pause and Resume Guide

This guide explains how to use the pause and resume features for Kubernetes-backed sandboxes in OpenSandbox. Pause commits the sandbox's root filesystem as an OCI image and releases cluster resources. Resume restores the sandbox from that image.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Server Configuration](#server-configuration)
- [Registry and Secret Setup](#registry-and-secret-setup)
- [Usage Guide](#usage-guide)
- [Administrator Guide](#administrator-guide)
- [SandboxSnapshot Reference](#sandboxsnapshot-reference)
- [Troubleshooting](#troubleshooting)

---

## Overview

### What Pause and Resume Does

| | Behavior |
|--|---------|
| **Pause** | Commits the running container's root filesystem as an OCI image, pushes it to a registry, then deletes the `BatchSandbox` and its Pod to release cluster resources |
| **Resume** | Creates a new `BatchSandbox` using the snapshot image, restoring the filesystem state |
| **sandboxId** | Stable across pause/resume cycles — callers use the same ID throughout the sandbox lifetime |

### Key Design Principle

**Server-level configuration**: Push/pull secrets and registry URL are configured once in `~/.sandbox.toml`. SDK users and API callers require **no code changes** to use pause/resume — they just call `pause` and `resume` on the existing sandbox ID.

### Lifecycle

```text
Time ---------------------------------------------------------------->

Sandbox lifecycle:   [Running]--[Pausing]--[Paused]--[Resuming]--[Running]
                         |                     |
                  commit rootfs          create new BatchSandbox
                  push to registry       from snapshot image
                  delete BatchSandbox
```

### What Is Preserved

| | Preserved? |
|--|-----------|
| Root filesystem contents | ✅ Yes — committed as OCI image |
| Environment variables | ✅ Yes — from BatchSandbox template |
| Running processes / memory | ❌ No — process state is not checkpointed |
| Explicit volume mounts | Depends on volume type |

---

## Architecture

```
API caller
    │ POST /v1/sandboxes/{id}/pause
    ▼
OpenSandbox Server
    │ reads [pause] config from ~/.sandbox.toml
    │ creates/updates SandboxSnapshot CR (spec.action = Pause)
    ▼
SandboxSnapshot Controller (Kubernetes)
    │ resolves running Pod
    │ creates commit Job on the same node
    ▼
commit Job Pod (image-committer)
    │ ctr/crictl: commit container rootfs → OCI image
    │ push to registry
    ▼
SandboxSnapshot.status.phase = Ready
    │ controller deletes source BatchSandbox
    ▼
Cluster resources released

--- Later: resume ---

API caller
    │ POST /v1/sandboxes/{id}/resume
    ▼
OpenSandbox Server
    │ sets SandboxSnapshot.spec.action = Resume
    ▼
SandboxSnapshot Controller
    │ creates new BatchSandbox from snapshot image
    ▼
Sandbox running again with restored filesystem
```

---

## Prerequisites

1. **Kubernetes cluster** with the OpenSandbox controller deployed
2. **OCI-compatible container registry** accessible from cluster nodes (push) and the Kubernetes API (pull)
3. **Kubernetes Secrets** of type `kubernetes.io/dockerconfigjson` for registry authentication
4. **Server** running with `[pause]` configured in `~/.sandbox.toml`

---

## Server Configuration

Add a `[pause]` section to `~/.sandbox.toml`:

```toml
[runtime]
type = "kubernetes"
execd_image = "opensandbox/execd:latest"

[pause]
snapshot_registry = "registry.example.com/sandboxes"
snapshot_push_secret = "registry-push-secret"
resume_pull_secret = "registry-pull-secret"
```

### Configuration Reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `snapshot_registry` | string | `""` | **Required.** OCI registry prefix. Images are stored as `<registry>/<sandboxId>-<container>:snapshot-v<N>`. |
| `snapshot_push_secret` | string | `""` | Kubernetes Secret name for pushing snapshots. Must be `kubernetes.io/dockerconfigjson` type. |
| `resume_pull_secret` | string | `""` | Kubernetes Secret name for pulling snapshot images on resume. Can be the same as push secret. |
| `snapshot_type` | string | `"Rootfs"` | Snapshot type. Only `"Rootfs"` is supported. |

> **Full reference**: [`server/configuration.md`](../server/configuration.md#pause--kubernetes-only)

### Startup behavior

The server does **not** validate `[pause]` at startup — the section can be omitted if pause/resume is not needed. Validation happens at request time: if `snapshot_registry` is empty when a pause request arrives, the server returns `400 Bad Request` with code `PAUSE_POLICY_NOT_CONFIGURED`.

---

## Registry and Secret Setup

### Step 1: Prepare your registry

Any OCI-compatible registry works (Docker Hub, GitHub Container Registry, Harbor, a private `registry:2` instance, etc.). The registry must be:

- **Reachable from cluster nodes** (for the commit Job to push)
- **Reachable from the Kubernetes API server / kubelet** (for image pull on resume)

### Step 2: Create the push secret

```bash
kubectl create secret docker-registry registry-push-secret \
  --docker-server=registry.example.com \
  --docker-username=<username> \
  --docker-password=<password-or-token> \
  --namespace=<sandbox-namespace>
```

### Step 3: Create the pull secret

The pull secret is used by the resumed `BatchSandbox` Pod to pull the snapshot image. It can be the same secret as the push secret if your credentials have both read and write access:

```bash
kubectl create secret docker-registry registry-pull-secret \
  --docker-server=registry.example.com \
  --docker-username=<username> \
  --docker-password=<password-or-token> \
  --namespace=<sandbox-namespace>
```

### Using a private `registry:2` (development)

For development with a cluster-internal `registry:2` deployment:

```bash
# Create a registry deployment
kubectl create deployment docker-registry \
  --image=registry:2 --port=5000

kubectl expose deployment docker-registry --port=5000

# No authentication needed for internal registry
# Leave snapshot_push_secret and resume_pull_secret empty in config
```

---

## Usage Guide

Once the server is configured, pause/resume works through the standard Lifecycle API. No SDK changes are needed.

### Pause a sandbox

```bash
curl -X POST http://localhost:8080/v1/sandboxes/{sandbox_id}/pause \
  -H "Content-Type: application/json"
```

**Response:**

```json
{
  "id": "my-sandbox-id",
  "status": "pausing"
}
```

The pause is asynchronous. The sandbox transitions through:
`running` → `pausing` → `paused`

### Check pause status

```bash
curl http://localhost:8080/v1/sandboxes/{sandbox_id}
```

When `status` is `paused`, the filesystem has been committed and cluster resources have been released.

### Resume a sandbox

```bash
curl -X POST http://localhost:8080/v1/sandboxes/{sandbox_id}/resume \
  -H "Content-Type: application/json"
```

The sandbox transitions through:
`paused` → `resuming` → `running`

### Multiple pause/resume cycles

Pause and resume can be repeated. Each pause cycle produces a new snapshot version (`snapshot-v1`, `snapshot-v2`, ...). The latest snapshot is always used for the next resume.

---

## Administrator Guide

### Controller RBAC

The OpenSandbox controller requires the following RBAC permissions for pause/resume (included in the Helm chart and `make manifests` output):

| Resource | Verbs | Purpose |
|----------|-------|---------|
| `sandboxsnapshots` | get, list, watch, create, update, patch, delete | Manage SandboxSnapshot CRs |
| `jobs` / `jobs/status` | full | Create/monitor commit Jobs |
| `secrets` | get | Validate push secret exists before creating commit Job |
| `pods` | get, list, watch | Find running Pod for commit |

### Snapshot image naming

Snapshot images are named:
```
<snapshot_registry>/<sandboxId>-<containerName>:snapshot-v<N>
```

For example, with `snapshot_registry = "registry.example.com/sandboxes"`, sandbox ID `my-sandbox`, container `sandbox`, first pause:
```
registry.example.com/sandboxes/my-sandbox-sandbox:snapshot-v1
```

### Commit Job

The controller creates a short-lived Kubernetes `Job` for each pause:

- **Job name**: `<snapshotName>-commit-v<N>`
- **Node affinity**: Runs on the **same node** as the source Pod (containerd socket access required)
- **Timeout**: 10 minutes (`ActiveDeadlineSeconds`)
- **TTL**: 5 minutes after completion (`TTLSecondsAfterFinished`)
- **Image**: `image-committer` (configurable via controller `--image-committer-image` flag)

### Monitoring

Check SandboxSnapshot status:

```bash
kubectl get sandboxsnapshot -n <namespace>
# NAME          PHASE       SANDBOX_ID     AGE
# my-snapshot   Ready       my-sandbox     5m

kubectl describe sandboxsnapshot my-snapshot -n <namespace>
```

Key fields to watch:

- `status.phase`: `Pending` → `Committing` → `Ready` / `Failed`
- `status.message`: Human-readable status or error message
- `status.containerSnapshots`: Image URIs for each committed container
- `status.history`: Audit log of last 10 pause/resume events

---

## SandboxSnapshot Reference

### Spec fields (set by Server)

| Field | Type | Description |
|-------|------|-------------|
| `sandboxId` | string | Stable sandbox identifier |
| `sourceBatchSandboxName` | string | BatchSandbox to snapshot |
| `action` | string | `Pause` or `Resume` |
| `snapshotRegistry` | string | OCI registry prefix |
| `snapshotPushSecret` | string | Secret name for push |
| `resumeImagePullSecret` | string | Secret name for pull on resume |
| `snapshotType` | string | `Rootfs` |

### Status fields (set by Controller)

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | `Pending` / `Committing` / `Ready` / `Failed` |
| `message` | string | Status message or error detail |
| `sourcePodName` | string | Pod name used for commit |
| `sourceNodeName` | string | Node where commit Job runs |
| `containerSnapshots` | list | `{containerName, imageUri}` per container |
| `resumeTemplate` | object | BatchSandbox spec template for resume |
| `pauseVersion` | int | Monotonically increasing pause counter |
| `resumeVersion` | int | Monotonically increasing resume counter |
| `lastPauseAt` | time | Timestamp of most recent pause request |
| `lastResumeAt` | time | Timestamp of most recent resume |
| `readyAt` | time | Timestamp when snapshot became Ready |
| `history` | list | Last 10 pause/resume records |
| `observedGeneration` | int | Last processed spec generation |

---

## Troubleshooting

### 1. Snapshot stuck in `Failed` — `snapshotPushSecret "xxx" not found`

**Cause**: The `snapshotPushSecret` specified in `~/.sandbox.toml` does not exist in the sandbox namespace.

**Solution**:
```bash
kubectl get secret registry-push-secret -n <namespace>
# If missing:
kubectl create secret docker-registry registry-push-secret \
  --docker-server=<registry> \
  --docker-username=<user> \
  --docker-password=<token> \
  -n <namespace>
```

The controller validates secret existence **before** creating the commit Job (fail-fast). Once the secret is created, trigger a new pause cycle.

---

### 2. Snapshot stuck in `Committing` for a long time

**Check the commit Job and its Pod:**

```bash
kubectl get job -n <namespace> -l sandbox.opensandbox.io/snapshot=<snapshotName>
kubectl describe pod <commit-pod-name> -n <namespace>
```

**Common causes:**

| Symptom | Cause | Solution |
|---------|-------|---------|
| `ContainerCreating` for >30s | Secret missing or wrong type | Re-create secret as `kubernetes.io/dockerconfigjson` |
| `FailedMount` event | Secret not found | See issue #1 above |
| Pod running but job never completes | Registry unreachable from node | Check network connectivity from node to registry |
| `unauthorized` in Pod logs | Wrong credentials in secret | Verify secret content with `kubectl get secret ... -o yaml` |

---

### 3. Wrong secret type

Docker registry secrets **must** be type `kubernetes.io/dockerconfigjson`. Generic secrets (`Opaque`) will cause a `FailedMount` error.

```bash
# Check secret type
kubectl get secret registry-push-secret -o jsonpath='{.type}'
# Expected: kubernetes.io/dockerconfigjson

# If wrong type, delete and recreate:
kubectl delete secret registry-push-secret
kubectl create secret docker-registry registry-push-secret \
  --docker-server=<registry> \
  --docker-username=<user> \
  --docker-password=<token>
```

---

### 4. Registry unreachable (`Committing` → `Failed` after timeout)

**Symptoms**: Commit Job Pod starts, runs for a while, then fails with a push error.

**Check:**

```bash
# Inspect commit Pod logs
kubectl logs <commit-pod-name> -n <namespace>

# Test registry connectivity from a node
kubectl run registry-test --rm -it --image=alpine -- \
  wget -O- https://<registry>/v2/ --timeout=5
```

**Common causes:**
- Registry behind a firewall not accessible from cluster nodes
- Self-signed TLS certificate not trusted by containerd
- Wrong registry URL (http vs https)

---

### 5. Resume creates sandbox but Pod fails to start

**Cause**: The snapshot image cannot be pulled.

```bash
kubectl describe pod <resumed-pod-name> -n <namespace>
# Look for: ErrImagePull or ImagePullBackOff
```

**Check:**
- `resume_pull_secret` is correctly configured and exists
- The registry is accessible from the node pulling the image
- The snapshot image was successfully pushed during pause (check `status.containerSnapshots`)

---

### 6. SandboxSnapshot not being processed (no status)

**Cause**: The OpenSandbox controller is not running.

```bash
kubectl get pods -n opensandbox-system
kubectl logs -n opensandbox-system deployment/opensandbox-controller-manager
```

---

## Getting Help

- **Documentation**: [OpenSandbox GitHub](https://github.com/alibaba/OpenSandbox)
- **Issues**: [GitHub Issues](https://github.com/alibaba/OpenSandbox/issues)
- **Design Document**: [OSEP-0008](../oseps/0008-pause-resume-rootfs-snapshot.md)
- **Server configuration reference**: [`server/configuration.md`](../server/configuration.md#pause--kubernetes-only)
- **Kubernetes controller**: [`kubernetes/README.md`](../kubernetes/README.md#pause-and-resume-rootfs-snapshot)
