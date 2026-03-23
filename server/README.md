# OpenSandbox Server

English | [中文](README_zh.md)

A production-grade, FastAPI-based service for managing the lifecycle of containerized sandboxes. It acts as the control plane to create, run, monitor, and dispose isolated execution environments across container platforms.

## Features

### Core capabilities
- **Lifecycle APIs**: Standardized REST interfaces for create, start, pause, resume, delete
- **Pluggable runtimes**:
  - **Docker**: Production-ready
  - **Kubernetes**: Production-ready (see `kubernetes/` for deployment)
- **Lifecycle cleanup modes**: Configurable TTL with renewal, or manual cleanup with explicit delete
- **Access control**: API Key authentication (`OPEN-SANDBOX-API-KEY`); can be disabled for local/dev
- **Networking modes**:
  - Host: shared host network, performance first
  - Bridge: isolated network with built-in HTTP routing
- **Resource quotas**: CPU/memory limits with Kubernetes-style specs
- **Observability**: Unified status with transition tracking
- **Registry support**: Public and private images

### Extended capabilities
- **Async provisioning**: Background creation to reduce latency
- **Timer restoration**: Expiration timers restored after restart
- **Env/metadata injection**: Per-sandbox environment and metadata
- **Port resolution**: Dynamic endpoint generation
- **Structured errors**: Standard error codes and messages

Metadata keys under the reserved prefix `opensandbox.io/` are system-managed
and cannot be supplied by users.

## Requirements

- **Python**: 3.10 or higher
- **Package Manager**: [uv](https://github.com/astral-sh/uv) (recommended) or pip
- **Runtime Backend**:
  - Docker Engine 20.10+ (for Docker runtime)
  - Kubernetes 1.21.1+ (for Kubernetes runtime)
- **Operating System**: Linux, macOS, or Windows with WSL2

## Quick Start

### Installation

1. **Install from PyPI**:
   > For source development or contributions, you can still clone the repo and run `uv sync` inside `server/`.
   ```bash
   uv pip install opensandbox-server
   ```

### Configuration

The server uses a TOML configuration file to select and configure the underlying runtime.

**Init configuration from simple example**:
```bash
# run opensandbox-server -h for help
opensandbox-server init-config ~/.sandbox.toml --example docker
```

**Create K8S configuration file**

The K8S version of the Sandbox Operator needs to be deployed in the cluster, refer to the Kubernetes directory.
```bash
# run opensandbox-server -h for help
opensandbox-server init-config ~/.sandbox.toml --example k8s
```

**[optional] Edit configuration for your environment**

- For quick e2e/demo (specify which one):
  ```bash
  opensandbox-server init-config ~/.sandbox.toml --example docker  # or docker-zh|k8s|k8s-zh
  # add --force to overwrite existing file
  ```
- Render the full schema-driven skeleton (no defaults, just placeholders) by omitting --example:
  ```bash
  opensandbox-server init-config ~/.sandbox.toml
  # add --force to overwrite existing file
  ```

**[optional] Edit `~/.sandbox.toml` for your environment**

Before you start the server, edit the configuration file to suit your environment. You could also generate a new empty configuration file by `opensandbox-server init-config ~/.sandbox.toml`.

**Docker runtime + host networking**
   ```toml
   [server]
   host = "0.0.0.0"
   port = 8080
   log_level = "INFO"
   api_key = "your-secret-api-key-change-this"
   max_sandbox_timeout_seconds = 86400  # Maximum TTL for requests that specify timeout

   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:v1.0.7"

   [docker]
   network_mode = "host"  # Containers share host network; only one sandbox instance at a time
   ```

**Docker runtime + bridge networking**
   ```toml
   [server]
   host = "0.0.0.0"
   port = 8080
   log_level = "INFO"
   api_key = "your-secret-api-key-change-this"
    max_sandbox_timeout_seconds = 86400  # Maximum TTL for requests that specify timeout

   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:v1.0.7"

   [docker]
   network_mode = "bridge"  # Isolated container networking
   ```

**Docker Compose deployment (server runs in a container)**

When `opensandbox-server` itself runs inside Docker Compose and manages sandboxes via
mounted `/var/run/docker.sock`, configure a reachable host value for bridge-mode endpoint
resolution:

```toml
[docker]
network_mode = "bridge"
host_ip = "host.docker.internal"  # or host LAN IP (for Linux: explicit host IP is recommended)
```

Why this matters:
- In bridge mode, sandbox containers get internal Docker IPs.
- External callers usually cannot reach those internal IPs directly.
- `host_ip` lets endpoint resolution return host-reachable addresses.

For SDK/API clients that cannot directly reach sandbox bridge addresses, request proxied
endpoints through the server:

```bash
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  "http://localhost:8080/v1/sandboxes/<sandbox-id>/endpoints/44772?use_server_proxy=true"
```

The returned endpoint is rewritten to the server proxy route:
- `<server-host>/sandboxes/<sandbox-id>/proxy/<port>`

Reference runtime compose file:
- `server/docker-compose.example.yaml`

For **experimental** lifecycle options (e.g. auto-renew on access), see [Experimental features](#experimental-features) (after [Configuration reference](#configuration-reference)).

**Sandbox TTL configuration**

- `timeout` requests must be at least 60 seconds.
- The maximum allowed TTL is controlled by `server.max_sandbox_timeout_seconds`.
- Omit `timeout` or set it to `null` in the create request to use manual cleanup mode instead of automatic expiration.

**Upgrade order for manual cleanup**

- Existing TTL-only clients can continue to work without changes as long as they do not encounter manual-cleanup sandboxes.
- Manual cleanup changes the lifecycle response contract: `expiresAt` may be `null`, and other nullable lifecycle fields may also be serialized explicitly as `null`.
- In practice this can include fields such as `metadata`, `status.reason`, `status.message`, and `status.lastTransitionAt`, depending on the sandbox state and the server response model.
- Before creating any manual-cleanup sandbox, upgrade every SDK/client that may call `create`, `get`, or `list` on the lifecycle API.
- Recommended rollout order:
  1. Upgrade SDKs/clients
  2. Upgrade the server
  3. Start creating sandboxes with `timeout` omitted or `null`
- Do not introduce manual-cleanup sandboxes into a shared environment while old SDKs are still actively reading lifecycle responses.

**Security hardening (applies to all Docker modes)**
   ```toml
   [docker]
   # Drop dangerous capabilities and block privilege escalation by default
   drop_capabilities = ["AUDIT_WRITE", "MKNOD", "NET_ADMIN", "NET_RAW", "SYS_ADMIN", "SYS_MODULE", "SYS_PTRACE", "SYS_TIME", "SYS_TTY_CONFIG"]
   no_new_privileges = true
   apparmor_profile = ""        # e.g. "docker-default" when AppArmor is available
   # Limit fork bombs and optionally enforce seccomp / read-only rootfs
   pids_limit = 512             # set to null to disable
   seccomp_profile = ""        # path or profile name; empty uses Docker default
   ```
   Further reading on Docker container security: https://docs.docker.com/engine/security/

For common issues and solutions, see [Troubleshooting](TROUBLESHOOTING.md).

**Secure container runtime (optional)**

OpenSandbox supports secure container runtimes for enhanced isolation:

```toml
[secure_runtime]
type = "gvisor"              # Options: "", "gvisor", "kata", "firecracker"
docker_runtime = "runsc"      # Docker OCI runtime name (for gVisor, Kata)
# k8s_runtime_class = "gvisor"  # Kubernetes RuntimeClass name (for K8s)
```

- `type=""` (default): No secure runtime, uses runc
- `type="gvisor"`: Uses gVisor (runsc) for user-space kernel isolation
- `type="kata"`: Uses Kata Containers for VM-level isolation
- `type="firecracker"`: Uses Firecracker microVM (Kubernetes only)

> **Detailed guide**: See [Secure Container Runtime Guide](../docs/secure-container.md) for complete installation instructions, system requirements, and troubleshooting.

**Docker daemon setup** for gVisor:
```json
{
  "runtimes": {
    "runsc": {
      "path": "/usr/bin/runsc"
    }
  }
}
```

**Kubernetes setup**: Create RuntimeClass before using:
```bash
kubectl create -f - <<EOF
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
EOF
```

**Ingress exposure (direct | gateway)**
   ```toml
   [ingress]
   mode = "direct"  # docker runtime only supports direct
   # gateway.address = "*.example.com"         # host only (domain or IP[:port]); scheme is not allowed
   # gateway.route.mode = "wildcard"            # wildcard | uri | header
   ```
   - `mode=direct`: default; required when `runtime.type=docker` (client ↔ sandbox direct reachability, no L7 gateway).
   - `mode=gateway`: configure external ingress.
     - `gateway.address`: wildcard domain required when `gateway.route.mode=wildcard`; otherwise must be domain, IP, or IP:port. Do not include scheme; clients decide http/https.
     - `gateway.route.mode`: `wildcard` (host-based wildcard), `uri` (path-prefix), `header` (header-based routing).
     - Response format examples:
       - `wildcard`: `<sandbox-id>-<port>.example.com/path/to/request`
       - `uri`: `10.0.0.1:8000/<sandbox-id>/<port>/path/to/request`
       - `header`: `gateway.example.com` with header `OpenSandbox-Ingress-To: <sandbox-id>-<port>`

**Kubernetes runtime**
   ```toml
   [runtime]
   type = "kubernetes"
   execd_image = "opensandbox/execd:v1.0.7"

   [kubernetes]
   kubeconfig_path = "~/.kube/config"
   namespace = "opensandbox"
   workload_provider = "batchsandbox"   # or "agent-sandbox"
   informer_enabled = true              # Beta: enable watch-based cache
   informer_resync_seconds = 300        # Beta: full list interval
   informer_watch_timeout_seconds = 60  # Beta: watch restart interval
   ```
   - Informer settings are **beta** and enabled by default to reduce API calls; set `informer_enabled = false` to turn off.
   - Resync and watch timeouts control how often the cache refreshes; tune for your cluster API limits.

### Egress configuration

The **`[egress]`** block configures the **egress sidecar** image and enforcement mode. The server only starts this sidecar when a sandbox is created **with** a `networkPolicy` (outbound allow/deny rules). If the create request omits `networkPolicy`, no egress sidecar is added and outbound traffic is not restricted by this mechanism.

#### Keys

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| `image` | string | — | **Yes** whenever `networkPolicy` is used in a create request | OCI image containing the egress binary. Pulled before the sidecar starts. |
| `mode` | `dns` or `dns+nft` | `dns` | No | How the sidecar enforces policy. Written to the sidecar as `OPENSANDBOX_EGRESS_MODE` (see below). |

#### `mode` values

- **`dns`**: DNS-based enforcement via the in-sidecar DNS proxy. No nftables layer-2 rules from this path. **CIDR and static IP targets in the policy are not enforced** (use domain-style rules only if you rely on `dns` mode).
- **`dns+nft`**: Same DNS path, plus nftables where available (see the [egress component README](../components/egress/README.md) for capabilities and fallbacks). **CIDR and static IP allow/deny rules are supported** via nftables when the table is applied successfully.

#### Per-request `networkPolicy`

- Rules are defined on **`CreateSandboxRequest.networkPolicy`** (default action and ordered egress rules: hostnames / patterns, and IP or CIDR entries when using **`dns+nft`**).
- The serialized policy is passed into the sidecar as **`OPENSANDBOX_EGRESS_RULES`** (JSON).
- An auth token may be attached for the egress HTTP API; see runtime behavior below.

#### Docker runtime

- **`egress.image` must be set** in config when clients send `networkPolicy`; otherwise the request is rejected.
- Outbound policy requires **`docker.network_mode = "bridge"`**. Requests with `networkPolicy` are rejected for `network_mode=host` or for user-defined Docker networks that are incompatible with the sidecar attachment model.
- The main sandbox container shares the sidecar’s network namespace, **drops `NET_ADMIN`**, and relies on the sidecar for policy; the sidecar **keeps `NET_ADMIN`**.
- **IPv6** is disabled in the shared namespace so allow/deny behavior stays consistent.

#### Kubernetes runtime

- When `networkPolicy` is present, the workload pod includes an **egress** sidecar built from `egress.image`, in addition to the main sandbox container.
- **`egress.image`** is required in the same way as for Docker.

#### Operational notes

- The sidecar image is pulled (or validated) before start; delete, expiry, and failure paths attempt to remove the sidecar.
- For deeper behavior (DNS proxy, nftables, limits), refer to the **egress** component documentation under `components/egress/`.

#### Example (`~/.sandbox.toml`)

```toml
[runtime]
type = "docker"
execd_image = "opensandbox/execd:v1.0.7"

[egress]
image = "opensandbox/egress:v1.0.3"
mode = "dns"
```

#### Example create request with `networkPolicy`

```json
{
  "image": {"uri": "python:3.11-slim"},
  "entrypoint": ["python", "-m", "http.server", "8000"],
  "timeout": 3600,
  "resourceLimits": {"cpu": "500m", "memory": "512Mi"},
  "networkPolicy": {
    "defaultAction": "deny",
    "egress": [
      {"action": "allow", "target": "pypi.org"},
      {"action": "allow", "target": "*.python.org"}
    ]
  }
}
```

### Run the server

Start the server using the installed CLI (reads `~/.sandbox.toml` by default):

```bash
opensandbox-server
```

The server will start at `http://0.0.0.0:8080` (or your configured host/port).

### Run the server (installed package)

After installing the package (wheel or PyPI), you can use the CLI entrypoint:

```bash
opensandbox-server --config ~/.sandbox.toml
```

**Health check**

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{"status": "healthy"}
```

## API documentation

Once the server is running, interactive API documentation is available:

- **Swagger UI**: [http://localhost:8080/docs](http://localhost:8080/docs)
- **ReDoc**: [http://localhost:8080/redoc](http://localhost:8080/redoc)

Further reading on Docker container security: https://docs.docker.com/engine/security/

### API authentication

Authentication is enforced only when `server.api_key` is set. If the value is empty or missing, the middleware skips API Key checks (intended for local/dev). For production, always set a non-empty `server.api_key` and send it via the `OPEN-SANDBOX-API-KEY` header.

All API endpoints (except `/health`, `/docs`, `/redoc`) require authentication via the `OPEN-SANDBOX-API-KEY` header when authentication is enabled:

```bash
curl http://localhost:8080/v1/sandboxes
```

### Example usage

**Create a Sandbox**

```bash
curl -X POST "http://localhost:8080/v1/sandboxes" \
  -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "image": {
      "uri": "python:3.11-slim"
    },
    "entrypoint": [
      "python",
      "-m",
      "http.server",
      "8000"
    ],
    "timeout": 3600,
    "resourceLimits": {
      "cpu": "500m",
      "memory": "512Mi"
    },
    "env": {
      "PYTHONUNBUFFERED": "1"
    },
    "metadata": {
      "team": "backend",
      "project": "api-testing"
    }
  }'
```

Response:
```json
{
  "id": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
  "status": {
    "state": "Pending",
    "reason": "CONTAINER_STARTING",
    "message": "Sandbox container is starting.",
    "lastTransitionAt": "2024-01-15T10:30:00Z"
  },
  "metadata": {
    "team": "backend",
    "project": "api-testing"
  },
  "expiresAt": "2024-01-15T11:30:00Z",
  "createdAt": "2024-01-15T10:30:00Z",
  "entrypoint": ["python", "-m", "http.server", "8000"]
}
```

**Get Sandbox Details**

```bash
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab
```

**Get Service Endpoint**

```bash
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/endpoints/8000

# execd (agent) endpoint
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/endpoints/44772
```

Response:
```json
{
  "endpoint": "sandbox.example.com/a1b2c3d4-5678-90ab-cdef-1234567890ab/8000"
}
```

**Renew Expiration**

```bash
curl -X POST "http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/renew-expiration" \
  -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "expiresAt": "2024-01-15T12:30:00Z"
  }'
```

**Delete a Sandbox**

```bash
curl -X DELETE \
  -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab
```

## Architecture

### Component responsibilities

- **API Layer** (`src/api/`): HTTP request handling, validation, and response formatting
- **Service Layer** (`src/services/`): Business logic for sandbox lifecycle operations
- **Middleware** (`src/middleware/`): Cross-cutting concerns (authentication, logging)
- **Configuration** (`src/config.py`): Centralized configuration management
- **Runtime Implementations**: Platform-specific sandbox orchestration

### Sandbox lifecycle states

```
       create()
          │
          ▼
     ┌─────────┐
     │ Pending │────────────────────┐
     └────┬────┘                    │
          │                         │
          │ (provisioning)          │
          ▼                         │
     ┌─────────┐    pause()         │
     │ Running │───────────────┐    │
     └────┬────┘               │    │
          │      resume()      │    │
          │   ┌────────────────┘    │
          │   │                     │
          │   ▼                     │
          │ ┌────────┐              │
          ├─│ Paused │              │
          │ └────────┘              │
          │                         │
          │ delete() or expire()    │
          ▼                         │
     ┌──────────┐                   │
     │ Stopping │                   │
     └────┬─────┘                   │
          │                         │
          ├────────────────┬────────┘
          │                │
          ▼                ▼
     ┌────────────┐   ┌────────┐
     │ Terminated │   │ Failed │
     └────────────┘   └────────┘
```

## Configuration reference

### Server configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `server.host` | string | `"0.0.0.0"` | Interface to bind |
| `server.port` | integer | `8080` | Port to listen on |
| `server.log_level` | string | `"INFO"` | Python logging level |
| `server.api_key` | string | `null` | API key for authentication |
| `server.eip` | string | `null` | Bound public IP; when set, used as the host part when returning sandbox endpoints (Docker runtime) |

### Runtime configuration

| Key                    | Type   | Required | Description                                           |
|------------------------|--------|----------|-------------------------------------------------------|
| `runtime.type`         | string | Yes      | Runtime implementation (`"docker"` or `"kubernetes"`) |
| `runtime.execd_image`  | string | Yes      | Container image with execd binary                     |

### Egress configuration

| Key | Type | Default | Required if using `networkPolicy` | Description |
|-----|------|---------|-----------------------------------|-------------|
| `egress.image` | string | — | Yes | Egress sidecar image (OCI reference). |
| `egress.mode` | `dns` \| `dns+nft` | `dns` | No | `OPENSANDBOX_EGRESS_MODE`. CIDR/IP rules need `dns+nft`; `dns` is domain-oriented only. |

### Docker configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `docker.network_mode` | string | `"host"` | Network mode (`"host"` or `"bridge"`) |

### Agent-sandbox configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `agent_sandbox.template_file` | string | `null` | Sandbox CR YAML template for agent-sandbox (used when `kubernetes.workload_provider = "agent-sandbox"`) |
| `agent_sandbox.shutdown_policy` | string | `"Delete"` | Shutdown policy on expiry (`"Delete"` or `"Retain"`) |
| `agent_sandbox.ingress_enabled` | boolean | `true` | Whether ingress routing is expected to be enabled |

### Environment variables

| Variable | Description |
|----------|-------------|
| `SANDBOX_CONFIG_PATH` | Override config file location |
| `DOCKER_HOST` | Docker daemon URL (e.g., `unix:///var/run/docker.sock`) |
| `PENDING_FAILURE_TTL` | TTL for failed pending sandboxes in seconds (default: 3600) |

## Experimental features

Optional **🧪 experimental** capabilities; **off by default** in `server/example.config.toml` and `example.config.*.toml`. Check release notes before production.

### Auto-renew on access

Extends sandbox TTL when access is observed (via the lifecycle **server proxy** and/or **ingress**). Architecture, data flow, and tuning are in **[OSEP-0009](../oseps/0009-auto-renew-sandbox-on-ingress-access.md)**.

**Server on/off**

| Goal | What to do |
|------|------------|
| **Off (default)** | Keep `[renew_intent] enabled = false` in `~/.sandbox.toml` (see `example.config.toml`). |
| **On** | Set `[renew_intent] enabled = true`. For **ingress + Redis** mode, set `redis.enabled = true` and `redis.dsn` in the same `[renew_intent]` table (see OSEP-0009). |
| **Other keys** | `min_interval_seconds`, `queue_key`, `consumer_concurrency` — see OSEP-0009 and `[renew_intent]` in `example.config.toml`. |

**Per sandbox**

On **create**, set `extensions["access.renew.extend.seconds"]` to a string integer between **300** and **86400** (seconds). Omit the key to opt that sandbox out of renew-on-access (or leave renew_intent disabled globally).

**Clients (SDK / HTTP)**

- **Use the lifecycle server as proxy** so traffic goes to `/v1/sandboxes/{id}/proxy/{port}/...`:
  - **REST**: request endpoints with `use_server_proxy=true`, e.g. `GET /v1/sandboxes/{id}/endpoints/{port}?use_server_proxy=true`.
  - **SDK**: `ConnectionConfig(use_server_proxy=True)` or `ConnectionConfigSync(use_server_proxy=True)` (see SDK docs for `use_server_proxy`).
- **Ingress / gateway** path: deploy and route per OSEP-0009; clients use the gateway as usual.

**Further reading**: [OSEP-0009](../oseps/0009-auto-renew-sandbox-on-ingress-access.md); sample keys under `[renew_intent]` in `server/example.config.toml`.

## Development

### Code quality

**Run linter**:
```bash
uv run ruff check
```

**Auto-fix issues**:
```bash
uv run ruff check --fix
```

**Format code**:
```bash
uv run ruff format
```

### Testing

**Run all tests**:
```bash
uv run pytest
```

**Run with coverage**:
```bash
uv run pytest --cov=src --cov-report=html
```

**Run specific test**:
```bash
uv run pytest tests/test_docker_service.py::test_create_sandbox_requires_entrypoint
```

## License

This project is licensed under the terms specified in the LICENSE file in the repository root.

## Contributing

Contributions are welcome. Suggested flow:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests for new functionality
4. Ensure all tests pass (`uv run pytest`)
5. Run linting (`uv run ruff check`)
6. Commit with clear messages
7. Push to your fork
8. Open a Pull Request

## Support

- Documentation: See `DEVELOPMENT.md` for development guidance
- Issues: Report defects via GitHub Issues
- Discussions: Use GitHub Discussions for Q&A and ideas
