# Repository Guidelines

## Project Structure & Module Organization
- `server/`: Python FastAPI service, configs, and tests.
- `components/execd/`: Go execution daemon and related tests.
- `sdks/`: Multi-language SDKs (`sdks/sandbox/*`, `sdks/code-interpreter/*`).
- `sandboxes/`: Runtime sandbox implementations (e.g., `sandboxes/code-interpreter/`).
- `specs/`: OpenAPI specs (`specs/execd-api.yaml`, `specs/sandbox-lifecycle.yml`).
- `examples/`: End-to-end usage examples and integrations.
- `tests/`: Cross-component/E2E tests (`tests/python/`, `tests/java/`).
- `docs/`, `oseps/`, `scripts/`: Docs, proposals, and automation scripts.

## Build, Test, and Development Commands
- Server (Python):
  - `cd server && uv sync` installs deps.
  - `cp server/opensandbox_server/examples/example.config.toml ~/.sandbox.toml` sets local config.
  - `cd server && uv run python -m opensandbox_server.main` runs the API server.
- execd (Go):
  - `cd components/execd && go build -o bin/execd .` builds the daemon.
  - `cd components/execd && make fmt` formats Go sources.
- SDKs:
  - Python: `cd sdks/sandbox/python && uv sync && uv run pytest`.
  - Kotlin: `cd sdks/sandbox/kotlin && ./gradlew build`.
- Specs: `node scripts/spec-doc/generate-spec.js` regenerates spec docs.

## Coding Style & Naming Conventions
- Python: PEP 8, `ruff` for lint/format, type hints on public APIs.
- Go: `gofmt`, explicit error handling, standard import grouping.
- Kotlin: Kotlin Coding Conventions, `ktlint` where configured.
- Naming: classes `PascalCase`, functions `snake_case` (Python) / `camelCase` (Go/Kotlin), constants `UPPER_SNAKE_CASE`.

## SDK API Implementation Conventions
- Keep a clear split between generated API transport code and handwritten SDK business/adaptor code.
- In adapter/infrastructure layers, default to integrating through generated API clients instead of handcrafted request wiring.
- Prefer generated OpenAPI clients for standard request/response endpoints; use handwritten transport only for streaming or protocol-specific paths (for example SSE).
- Do not manually edit generated client files. When specs change, regenerate first, then adapt handwritten layers.
- For handwritten streaming paths, keep wire contracts aligned with OpenAPI field names/models and cover behavior with focused tests (especially parsing and error mapping).

## Testing Guidelines
- Python tests use `pytest` (async tests common).
- Go tests use `go test` under `components/execd/pkg/...`.
- Kotlin tests use Gradle (`./gradlew test`).
- Coverage targets (from CONTRIBUTING): core packages >80%, API layer >70%.

## Commit & Pull Request Guidelines
- Commit messages follow Conventional Commits, e.g. `feat(server): add runtime`.
- Use feature branches (e.g., `feature/...`, `fix/...`) and keep PRs focused.
- PRs should include summary, testing status, and linked issues; follow the template in `CONTRIBUTING.md`.
- For major API or architectural changes, submit an OSEP (`oseps/`).

## Security & Configuration Tips
- Local server config lives in `~/.sandbox.toml` (copied from `server/example.config.toml`).
- Docker is required for local sandbox execution; keep images and keys out of commits.
