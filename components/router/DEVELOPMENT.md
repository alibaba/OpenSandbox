# Development Guide (Quick)

## Prerequisites
- Go 1.24+
- Docker (optional, for image build)
- Access to a Kubernetes cluster if you want to exercise proxy behavior.

## Install deps
```bash
cd components/router
go mod tidy && go mod vendor
```

## Build & Run
```bash
make build          # binary at bin/router with ldflags version info
./bin/router \
  --namespace <ns> \
  --ingress-label-key <label-key> \
  --port 28888 \
  --log-level info
```

## Tests & Lint
```bash
make test           # go test ./pkg/...
go vet ./...        # included in make build
```

## Docker (with build args)
```bash
docker build \
  --build-arg VERSION=$(git describe --tags --always --dirty) \
  --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t opensandbox/router:dev .
```

## Key Paths
- `main.go` — entrypoint, HTTP routes.
- `pkg/proxy/` — HTTP/WebSocket proxy logic and pod watching.
- `version/` — build metadata (ldflags).

## Tips
- Health check: `/status.ok`
- Env overrides: `VERSION/GIT_COMMIT/BUILD_TIME` usable via Makefile and build.sh.

