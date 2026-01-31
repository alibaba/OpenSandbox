# OpenSandbox Ingress

English | [中文](README_zh.md)

## Overview
- HTTP/WebSocket reverse proxy that routes to sandbox instances by `OPEN-SANDBOX-INGRESS` header or Host.
- Watches BatchSandbox CRs in a target Namespace, and routes to sandbox endpoints from annotations.
- Exposes `/status.ok` health check; prints build metadata (version, commit, time, Go/platform) at startup.

## Quick Start
```bash
go run main.go \
  --namespace <target-namespace> \
  --port 28888 \
  --log-level info
```
Endpoints: `/` (proxy), `/status.ok` (health).

## Build
```bash
cd components/ingress
make build
# override build metadata if needed
VERSION=1.2.3 GIT_COMMIT=$(git rev-parse HEAD) BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") make build
```

## Docker Build
Dockerfile already wires ldflags via build args:
```bash
docker build \
  --build-arg VERSION=$(git describe --tags --always --dirty) \
  --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t opensandbox/ingress:local .
```

## Multi-arch Publish Script
`build.sh` uses buildx to build/push linux/amd64 and linux/arm64:
```bash
cd components/ingress
TAG=local VERSION=1.2.3 GIT_COMMIT=abc BUILD_TIME=2025-01-01T00:00:00Z bash build.sh
```

## Runtime Requirements
- Access to Kubernetes API (in-cluster or via KUBECONFIG).
- BatchSandbox CRs in the specified Namespace with `sandbox.opensandbox.io/endpoints` annotation containing Pod IPs.

## Behavior Notes
- Routing key priority: `OPEN-SANDBOX-INGRESS` header first, otherwise Host parsing `<sandbox-name>-<port>.*`.
- Sandbox name extracted from request is used to query BatchSandbox CR for endpoint IP.
- Error handling:
  - `ErrSandboxNotFound` (sandbox resource not exists) → HTTP 404
  - `ErrSandboxNotReady` (not enough replicas, missing endpoints, invalid config) → HTTP 503
  - Other errors (K8s API errors, etc.) → HTTP 502
- WebSocket path forwards essential headers and X-Forwarded-*; HTTP path strips `OPEN-SANDBOX-INGRESS` before proxying.

## Development & Tests
```bash
cd components/ingress
go test ./...
```
Key code:
- `main.go`: entrypoint and handlers.
- `pkg/proxy/`: HTTP/WebSocket proxy logic, sandbox endpoint resolution.
- `pkg/sandbox/`: Sandbox provider abstraction and BatchSandbox implementation.
- `version/`: build metadata output (populated via ldflags).

