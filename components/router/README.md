# OpenSandbox Router

English | [中文](README_zh.md)

## Overview
- HTTP/WebSocket reverse proxy that routes to sandbox Pods by `OPEN-SANDBOX-INGRESS` header or Host.
- Watches Pods in a target Namespace filtered by an ingress label, and routes only when exactly one ready Pod matches.
- Exposes `/status.ok` health check; prints build metadata (version, commit, time, Go/platform) at startup.

## Quick Start
```bash
go run main.go \
  --namespace <target-namespace> \
  --ingress-label-key <label-key> \
  --port 28888 \
  --log-level info
```
Endpoints: `/` (proxy), `/status.ok` (health).

## Build
```bash
cd components/router
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
  -t opensandbox/router:local .
```

## Multi-arch Publish Script
`build.sh` uses buildx to build/push linux/amd64 and linux/arm64:
```bash
cd components/router
TAG=local VERSION=1.2.3 GIT_COMMIT=abc BUILD_TIME=2025-01-01T00:00:00Z bash build.sh
```

## Runtime Requirements
- Access to Kubernetes API (in-cluster or via KUBECONFIG).
- Pods in the specified Namespace labeled with the configured ingress label; Pod IPs must be reachable.

## Behavior Notes
- Routing key priority: `OPEN-SANDBOX-INGRESS` header first, otherwise Host parsing `<ingress>-<port>.*`.
- Multiple matching Pods → HTTP 409; no matching Pod → HTTP 404.
- WebSocket path forwards essential headers and X-Forwarded-*; HTTP path strips `OPEN-SANDBOX-INGRESS` before proxying.

## Development & Tests
```bash
cd components/router
go test ./...
```
Key code:
- `main.go`: entrypoint and handlers.
- `pkg/proxy/`: HTTP/WebSocket proxy logic, pod watching, health check.
- `version/`: build metadata output (populated via ldflags).

