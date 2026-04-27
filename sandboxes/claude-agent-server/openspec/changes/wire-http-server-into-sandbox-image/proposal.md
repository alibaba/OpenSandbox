## Why

The `claude-agent-server` HTTP wrapper exposes Claude Code sessions over REST + SSE, but today it only runs as a standalone process on the host. To give each sandbox container its own isolated Claude Code service — reachable by the platform that created it — the server must be embedded in the sandbox image and start automatically when the container boots.

## What Changes

- **Dockerfile** gains a build stage that compiles the TypeScript server and installs its production Node.js dependencies, then copies the compiled output into the sandbox image.
- **`entrypoint.sh`** is extended to launch the HTTP server as a background process (on a dedicated port) before handing off to the existing OFS-mount + code-interpreter flow.
- **`docker-compose.yaml` / `config.toml`** document the new per-sandbox HTTP port so the orchestration layer knows how to reach it.
- A health-check hook is added to the entrypoint so container startup is gated on the server being ready.

## Capabilities

### New Capabilities

- `sandbox-http-server`: Embedding and lifecycle management of the claude-agent-server HTTP process inside each sandbox container — covering image packaging, startup ordering, port exposure, environment wiring, and health-gating.

### Modified Capabilities

<!-- No existing spec-level requirements are changing. -->

## Impact

- **`docker/docker/Dockerfile`** — adds a multi-stage build; Node.js must be available in the final image (the base `code-interpreter` image already includes it).
- **`docker/docker/entrypoint.sh`** — starts the HTTP server background process and performs a readiness check before continuing.
- **`docker/config.toml`** or sandbox creation call — must expose the HTTP server port from the container.
- **No changes to `src/`** — the server code is consumed as-is; only its packaging and startup wiring changes.
- **New env vars** injected at sandbox creation: `PORT`, `HOST`, `CLAUDE_WRAPPER_REQUIRE_AUTH_TOKEN` (optional), and any model defaults needed by the Claude Code SDK.
