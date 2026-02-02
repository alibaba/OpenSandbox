# Development Guide

This guide provides comprehensive information for developers working on OpenSandbox Server, including environment setup, architecture deep-dive, testing strategies, and contribution workflows.

## 📋 Table of Contents

- [Development Environment Setup](#development-environment-setup)
- [Project Structure](#project-structure)
- [Architecture Deep Dive](#architecture-deep-dive)
- [Development Workflow](#development-workflow)
- [Testing Guide](#testing-guide)
- [Working with Docker Runtime](#working-with-docker-runtime)
- [Working with Kubernetes Runtime](#working-with-kubernetes-runtime)
- [Code Style and Standards](#code-style-and-standards)
- [Debugging](#debugging)
- [Performance Optimization](#performance-optimization)
- [Contributing](#contributing)

## Development Environment Setup

### Prerequisites

- **Python 3.10+**: Check version with `python --version`
- **uv**: Install from [https://github.com/astral-sh/uv](https://github.com/astral-sh/uv)
- **Docker**: For local development and testing
- **Git**: Version control
- **IDE**: VS Code, PyCharm, or Cursor (recommended for AI assistance)

### Initial Setup

1. **Clone and Navigate**
   ```bash
   git clone https://github.com/alibaba/OpenSandbox.git
   cd OpenSandbox/server
   ```

2. **Install Dependencies**
   ```bash
   uv sync
   ```

3. **Verify Installation**
   ```bash
   uv run python -c "import fastapi; print(fastapi.__version__)"
   ```

4. **Configure Development Environment**
   ```bash
   cp example.config.toml ~/.sandbox.toml
   ```

   Edit `~/.sandbox.toml` for local development:
   ```toml
   [server]
   host = "0.0.0.0"
   port = 8080
   log_level = "DEBUG"
   api_key = "your-secret-api-key-change-this"

   [runtime]
   type = "docker"
   execd_image = "opensandbox/execd:v1.0.5"

   [docker]
   network_mode = "host"
   ```

5. **Run Development Server**
   ```bash
   uv run python -m src.main
   ```

### IDE Configuration

#### VS Code / Cursor

Create `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Python: FastAPI",
            "type": "python",
            "request": "launch",
            "module": "src.main",
            "justMyCode": false,
            "env": {
                "SANDBOX_CONFIG_PATH": "${workspaceFolder}/.sandbox.toml"
            }
        }
    ]
}
```

#### PyCharm

1. Open project in PyCharm
2. Configure Python interpreter: **Settings → Project → Python Interpreter**
3. Select the virtual environment created by `uv sync`
4. Enable pytest: **Settings → Tools → Python Integrated Tools → Testing → pytest**

## Project Structure

```
server/
├── src/                          # Source code
│   ├── main.py                   # FastAPI application entry point
│   ├── config.py                 # Configuration management
│   ├── api/                      # API layer
│   │   ├── lifecycle.py          # Sandbox lifecycle routes
│   │   └── schema.py             # Pydantic models
│   ├── middleware/               # Middleware components
│   │   └── auth.py               # API Key authentication
│   └── services/                 # Business logic layer
│       ├── sandbox_service.py    # Abstract base class
│       ├── docker.py             # Docker implementation
│       └── factory.py            # Service factory
├── tests/                        # Test suite
├── scripts/                      # Utility scripts
├── pyproject.toml                # Project metadata and dependencies
└── example.config.toml           # Example configuration
```

## Architecture Deep Dive

### Layered Architecture

The server follows a clean layered architecture:

1. **HTTP Layer** (FastAPI routes) - Request validation and response serialization
2. **Middleware Layer** - Authentication and cross-cutting concerns
3. **Service Layer** - Business logic abstraction
4. **Runtime Implementation Layer** - Docker/Kubernetes specific code

### Request Flow

#### Create Sandbox (Async)

```
Client → POST /sandboxes
  ↓
Auth Middleware validates API key
  ↓
lifecycle.create_sandbox() receives CreateSandboxRequest
  ↓
sandbox_service.create_sandbox_async(request)
  ↓
Returns 202 Accepted with Pending status immediately
  ↓
Background thread provisions the sandbox
```

### Internal Systems

#### Expiration Timer System

Tracks sandbox timeouts using in-memory data structures:
- `_sandbox_expirations: Dict[str, datetime]` - Expiration times
- `_expiration_timers: Dict[str, Timer]` - Active timer threads
- `_expiration_lock: Lock` - Thread synchronization

#### Async Provisioning System

Avoids blocking API requests during slow operations by:
1. Storing sandboxes in pending state
2. Starting background provisioning thread
3. Returning 202 Accepted immediately
4. Transitioning to running state when ready

## Development Workflow

### Feature Development

```bash
git checkout -b feature/my-feature
# Implement feature
uv run pytest
git commit -m "feat: add my feature"
git push origin feature/my-feature
```

### Bug Fixes

```bash
git checkout -b fix/bug-description
# Write failing test
# Fix bug
uv run pytest
git commit -m "fix: resolve bug"
```

## Testing Guide

### Running Tests
> **Note**: A local Docker daemon is required to run the full test suite, as integration tests interact with the Docker Engine.

```bash
# All tests
uv run pytest

# Specific file
uv run pytest tests/test_docker_service.py

# With coverage
uv run pytest --cov=src --cov-report=html
```

### Writing Tests

Example unit test:

```python
@patch("src.services.docker.docker")
def test_create_sandbox_validates_entrypoint(mock_docker):
    service = DockerSandboxService(config=test_config())
    request = CreateSandboxRequest(
        image=ImageSpec(uri="python:3.11"),
        timeout=120,
        entrypoint=[]  # Invalid
    )
    with pytest.raises(HTTPException):
        service.create_sandbox(request)
```

## Working with Docker Runtime

### Local Development

```bash
# Use local Docker
export DOCKER_HOST="unix:///var/run/docker.sock"
uv run python -m src.main

# Use remote Docker
export DOCKER_HOST="ssh://user@remote-host"
uv run python -m src.main
```

### Network Modes

**Host Mode (Default):**
- Sandboxes share host network
- Direct port access
- Endpoint format: `http://{domain}/{sandbox_id}/{port}`

**Bridge Mode:**
- Isolated networks
- HTTP proxy required
- Endpoint format: `http://{server}/route/{sandbox_id}/{port}/path`

### Egress sidecar (bridge + `networkPolicy`)

- Config: set `[runtime].egress_image`; sidecar starts only when the request carries `networkPolicy`. Requires Docker `network_mode="bridge"`.
- Network & privileges: main container shares the sidecar netns (`network_mode=container:<sidecar>`); main container explicitly drops `NET_ADMIN`; sidecar keeps `NET_ADMIN` to manage iptables / DNS transparent redirect.
- Ports: host port bindings live on the sidecar; main container labels record the mapped ports for upstream endpoint resolution.
- Lifecycle: on create failure / delete / expiration / abnormal recovery, the sidecar is cleaned up; startup also removes orphaned sidecars.
- Injection: `OPENSANDBOX_EGRESS_RULES` env passes the `networkPolicy` JSON; sidecar image is pulled/ensured before start.

## Working with Kubernetes Runtime

> **Status:** Planned / Configuration Ready

Architecture will include:
- Pod management with execd init container
- Service/Ingress for networking
- CronJob or operator for expiration handling

## Code Style and Standards

Follow PEP 8 with Ruff enforcement:

```bash
uv run ruff check src tests
```

### Naming Conventions

- Functions: `snake_case`
- Classes: `PascalCase`
- Constants: `UPPER_SNAKE_CASE`
- Private: `_leading_underscore`

### Type Hints

Always use type hints:

```python
def get_sandbox(self, sandbox_id: str) -> Sandbox:
    pass
```

## Debugging

### Enable Debug Logging

```toml
[server]
log_level = "DEBUG"
```

### Interactive Debugging

Use VS Code/Cursor breakpoints or:

```python
breakpoint()  # Python 3.7+
```

### Docker Debugging

```python
import logging
logging.getLogger("docker").setLevel(logging.DEBUG)
```

## Performance Optimization

### Profiling

```bash
python -m cProfile -o profile.stats -m src.main
```

### Optimization Tips

1. **Async Operations**: Use async provisioning to avoid blocking
2. **Connection Pooling**: Reuse Docker client connections
3. **Caching**: Cache configuration and frequently accessed data
4. **Resource Limits**: Set appropriate container resource limits
5. **Monitoring**: Track container creation/deletion metrics

## Contributing

### Pull Request Process

1. Fork the repository
2. Create feature branch from `main`
3. Write tests for new functionality
4. Ensure all tests pass: `uv run pytest`
5. Run linter: `uv run ruff check`
6. Write clear commit messages
7. Submit PR with description

### Code Review Guidelines

- Focus on readability and maintainability
- Ensure test coverage for new code
- Check for proper error handling
- Verify documentation updates
- Test Docker and potential Kubernetes compatibility

### Commit Message Format

```
<type>: <description>

Types: feat, fix, docs, style, refactor, test, chore
```

Examples:
- `feat: add Kubernetes runtime support`
- `fix: resolve expiration timer memory leak`
- `docs: update API documentation`

---

For questions or support, please open an issue on the project repository.
