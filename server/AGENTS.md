# Server AGENTS

You are working on the OpenSandbox lifecycle server. Keep the route layer thin and put behavior in services, validators, or runtime helpers.

## Scope

- `opensandbox_server/**`
- `tests/**`

If the task changes lifecycle API contracts in `../specs/sandbox-lifecycle.yml`, also read `../specs/AGENTS.md`.

## Key Paths

- `opensandbox_server/main.py`: app entry point and startup wiring
- `opensandbox_server/api/`: FastAPI routes and request/response schemas
- `opensandbox_server/services/`: business logic and runtime integration
- `opensandbox_server/integrations/`: optional external integrations
- `tests/`: unit, integration, smoke, and Kubernetes-focused tests

## Commands

Setup and focused checks:

```bash
cd server
uv sync --all-groups
uv run ruff check
uv run pytest tests/test_docker_service.py
uv run pytest tests/test_schema.py
```

Typed or broader validation:

```bash
cd server
uv run pyright
uv run pytest
```

Local startup:

```bash
cp server/opensandbox_server/examples/example.config.toml ~/.sandbox.toml
cd server
uv run python -m opensandbox_server.main
```

Smoke path when Docker is available:

```bash
cd server
chmod +x tests/smoke.sh
./tests/smoke.sh
```

## Guardrails

Always:

- Keep FastAPI routes thin and delegate behavior to services, validators, or runtime helpers.
- Extend existing fixtures and helpers before adding parallel abstractions.
- Add focused regression tests with every bug fix or behavior change.

Ask first:

- Removing or renaming public endpoints
- Changing config shape or defaults in a user-visible way
- Introducing new external service dependencies
- Large reorganizations across `api/`, `services/`, and `tests/`

Never:

- Put business logic directly in route handlers.
- Change public server behavior without tests.
- Assume Docker-only behavior is harmless for Kubernetes paths.
