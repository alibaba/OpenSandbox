# OpenSandbox Server

English | [中文](README_zh.md)

A production-grade, FastAPI-based service for managing the lifecycle of containerized sandboxes. It acts as the control plane to create, run, monitor, and dispose isolated execution environments across container platforms.

## Features

### Core capabilities
- **Lifecycle APIs**: Standardized REST interfaces for create, start, pause, resume, delete
- **Pluggable runtimes**:
  - **Docker**: Production-ready
  - **Kubernetes**: Supported (see `kubernetes/` for deployment)
- **Automatic expiration**: Configurable TTL with renewal
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

## Requirements

- **Python**: 3.10 or higher
- **Package Manager**: [uv](https://github.com/astral-sh/uv) (recommended) or pip
- **Runtime Backend**:
  - Docker Engine 20.10+ (for Docker runtime)
  - Kubernetes 1.21+ (for Kubernetes runtime)
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

   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:v1.0.6"

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

   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:v1.0.6"

   [docker]
   network_mode = "bridge"  # Isolated container networking
   ```

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
   execd_image = "opensandbox/execd:v1.0.5"

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

### Egress sidecar for `networkPolicy`

- **Required when using `networkPolicy`**: Configure the sidecar image. The `egress.image` setting is mandatory when requests include `networkPolicy`:
   ```toml
   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:v1.0.6"
   
   [egress]
   image = "opensandbox/egress:v1.0.0"
   ```
- Supported only in Docker bridge mode; requests with `networkPolicy` are rejected when `network_mode=host` or when `egress.image` is not configured.
- Main container shares the sidecar netns and explicitly drops `NET_ADMIN`; the sidecar keeps `NET_ADMIN` to manage iptables.
- IPv6 is disabled in the shared namespace when the egress sidecar is injected to keep policy enforcement consistent.
- Sidecar image is pulled before start; delete/expire/failure paths attempt to clean up the sidecar as well.
- Request example (`CreateSandboxRequest` with `networkPolicy`):
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
- When `networkPolicy` is empty or omitted, no sidecar is injected (allow-all at start).

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

### Runtime configuration

| Key                    | Type   | Required | Description                                           |
|------------------------|--------|----------|-------------------------------------------------------|
| `runtime.type`         | string | Yes      | Runtime implementation (`"docker"` or `"kubernetes"`) |
| `runtime.execd_image`  | string | Yes      | Container image with execd binary                     |

### Egress configuration

| Key           | Type   | Required | Description                    |
|---------------|--------|----------|--------------------------------|
| `egress.image` | string | **Required when using `networkPolicy`** | Container image with egress binary. Must be configured when `networkPolicy` is provided in sandbox creation requests. |

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
| `DOCKER_API_TIMEOUT` | Docker client timeout in seconds (default: 180) |
| `PENDING_FAILURE_TTL` | TTL for failed pending sandboxes in seconds (default: 3600) |

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
