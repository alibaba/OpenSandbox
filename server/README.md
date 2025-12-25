# OpenSandbox Server

English | [中文](README_zh.md)

A production-grade, FastAPI-based service for managing the lifecycle of containerized sandboxes. It acts as the control plane to create, run, monitor, and dispose isolated execution environments across container platforms.

## Features

### Core capabilities
- **Lifecycle APIs**: Standardized REST interfaces for create, start, pause, resume, delete
- **Pluggable runtimes**:
  - **Docker**: Production-ready
  - **Kubernetes**: Configuration placeholder, under development
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
  - Kubernetes 1.21+ (for Kubernetes runtime, when available)
- **Operating System**: Linux, macOS, or Windows with WSL2

## Quick Start

### Installation

1. **Clone the repository** and navigate to the server directory:
   ```bash
   cd server
   ```

2. **Install dependencies** using `uv`:
   ```bash
   uv sync
   ```

### Configuration

The server uses a TOML configuration file to select and configure the underlying runtime.

**Create configuration file**:
```bash
cp example.config.toml ~/.sandbox.toml
```
**[optional] Create K8S configuration file：
The K8S version of the Sandbox Operator needs to be deployed in the cluster, refer to the Kubernetes directory.
```bash
cp example.config.k8s.toml ~/.sandbox.toml
cp example.batchsandbox-template.yaml ~/batchsandbox-template.yaml
```

**[optional] Edit `~/.sandbox.toml`** for your environment:

**Option A: Docker runtime + host networking (default)**
   ```toml
   [server]
   host = "0.0.0.0"
   port = 8080
   log_level = "INFO"
   api_key = "your-secret-api-key-change-this"

   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:latest"

   [docker]
   network_mode = "host"  # Containers share host network; only one sandbox instance at a time
   ```

**Option B: Docker runtime + bridge networking**
   ```toml
   [server]
   host = "0.0.0.0"
   port = 8080
   log_level = "INFO"
   api_key = "your-secret-api-key-change-this"

   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:latest"

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

### Run the server

Start the server using `uv`:

```bash
uv run python -m src.main
```

The server will start at `http://0.0.0.0:8080` (or your configured host/port).

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
curl http://localhost:8080/sandboxes
```

### Example usage

**Create a Sandbox**

```bash
curl -X POST "http://localhost:8080/sandboxes" \
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
  http://localhost:8080/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab
```

**Get Service Endpoint**

```bash
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/endpoints/8000

# execd (agent) endpoint
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/endpoints/44772
```

Response:
```json
{
  "endpoint": "sandbox.example.com/a1b2c3d4-5678-90ab-cdef-1234567890ab/8000"
}
```

**Renew Expiration**

```bash
curl -X POST "http://localhost:8080/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/renew-expiration" \
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
  http://localhost:8080/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab
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

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `runtime.type` | string | Yes | Runtime implementation (`"docker"` or `"kubernetes"`) |
| `runtime.execd_image` | string | Yes | Container image with execd binary |

### Docker configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `docker.network_mode` | string | `"host"` | Network mode (`"host"` or `"bridge"`) |

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
