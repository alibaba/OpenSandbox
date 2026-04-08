# Kubernetes Operator

## Overview

Kubernetes operator managing sandbox environments via custom resources. Provides BatchSandbox (O(1) batch delivery), Pool (resource pooling for fast provisioning), and optional task orchestration. Built with controller-runtime (Kubebuilder).

## Structure

```
kubernetes/
├── apis/sandbox/v1alpha1/   # CRD type definitions
│   ├── batchsandbox_types.go # BatchSandbox spec + status
│   ├── pool_types.go        # Pool spec + status
│   └── sandboxsnapshot_types.go
├── cmd/
│   ├── controller/main.go      # Controller manager entry point
│   ├── image-committer/main.go # Image committer binary (runs as commit Job)
│   └── task-executor/main.go   # Task executor binary (runs as sidecar)
├── internal/
│   ├── controller/          # Reconciliation loops
│   ├── scheduler/           # Pool allocation logic (bufferMin/Max, poolMax)
│   └── utils/               # Utility functions
├── config/
│   ├── crd/bases/           # Generated CRD YAML manifests
│   ├── rbac/                # ClusterRole, ClusterRoleBinding
│   ├── manager/             # Controller deployment manifest
│   └── samples/             # Example CRD instances
├── charts/                  # Helm charts (opensandbox-controller, opensandbox-server, opensandbox)
├── test/e2e/                # End-to-end tests + testdata
└── Dockerfile               # Controller image build
    Dockerfile.image-committer # Image-committer image build
```

## Where to Look

| Task | File | Notes |
|------|------|-------|
| Add CRD field | `apis/sandbox/v1alpha1/*_types.go` | Run `make install` to update CRDs |
| Controller logic | `internal/controller/` | BatchSandbox + Pool reconciliation |
| Pool allocation | `internal/scheduler/` | Buffer management, sandbox→pool assignment |
| Task execution | `cmd/task-executor/`, `internal/task-executor/` | Process-based tasks in sandboxes |
| Helm values | `charts/opensandbox-controller/values.yaml` | Controller + task-executor image refs |
| RBAC permissions | `config/rbac/` | ClusterRole rules |
| E2E tests | `test/e2e/` | Ginkgo/Gomega test framework |

## Conventions

- **Framework**: Kubebuilder with `controller-runtime` v0.21.
- **Go version**: 1.24. Own `go.mod` (`github.com/alibaba/opensandbox/sandbox-k8s`).
- **Concurrency**: BatchSandbox controller concurrency=32, Pool controller concurrency=1.
- **CRD version**: `v1alpha1` under group `sandbox.opensandbox.io`.
- **Helm charts**: Umbrella chart (`opensandbox`) wraps controller + server subcharts.
- **Logging**: `klog/v2` + `zap`. Log level configurable via `--zap-log-level` flag.

## Anti-Patterns

- `pause`/`resume` lifecycle uses SandboxSnapshot CRD + image-committer Job to snapshot and restore containers.
- BatchSandbox deletion waits for running tasks to terminate before removing the resource.
- Task-executor requires `shareProcessNamespace: true` and `SYS_PTRACE` capability in pod spec.
- Pool template changes do not affect already-allocated sandboxes.

## Commands

```bash
make install                       # install CRDs into cluster
make deploy CONTROLLER_IMG=... TASK_EXECUTOR_IMG=...  # deploy controller
make docker-build                  # build controller image
make docker-build-task-executor    # build task-executor image
make docker-build-image-committer  # build image-committer image
make test                          # run tests
```
