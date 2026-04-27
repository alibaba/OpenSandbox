## Context

The `claude-agent-server` is a TypeScript/Express app that wraps `@anthropic-ai/claude-agent-sdk` and exposes sessions over HTTP (REST + SSE). The sandbox image (`docker/docker/Dockerfile`) is built on top of `opensandbox/code-interpreter`, which already ships Node.js (needed for JavaScript/TypeScript kernel support). The container lifecycle is:

1. `entrypoint.sh` mounts OFS storage at `/workspace/$USERNAME/$SESSION_ID`
2. It then `exec`s `code-interpreter.sh`, which starts Jupyter on port 44771

The HTTP server needs to run as a parallel process, not replacing any existing behaviour.

## Goals / Non-Goals

**Goals:**
- Embed a production build of `claude-agent-server` inside the sandbox image
- Start the HTTP server automatically when the container boots, before handing off to Jupyter
- Gate startup on the server passing its own `/health` check
- Expose the server port so the platform can reach it

**Non-Goals:**
- Process supervision / automatic restart on crash (the server lifecycle is tied to the container lifecycle)
- Running the HTTP server outside of a sandbox container (it stays a host-level concern when used standalone)
- Changing any `src/` code — the server is consumed as-is

## Decisions

### 1. Multi-stage Docker build

**Decision:** Add a `builder` stage to the Dockerfile that runs `npm ci && npm run build`, then copy `dist/` and `node_modules/` into the final image.

**Rationale:** Keeps build tools (`typescript`, `tsx`, dev deps) out of the final image. The alternative — pre-building on the host and `COPY`-ing artifacts — breaks reproducibility and requires the host to have matching Node version.

**Alternatives considered:**
- `COPY . .` + build inside final image — includes devDependencies and build toolchain in the final layer (~2× larger image).
- Install from npm registry — would require publishing the server; adds a deploy step.

### 2. Startup via `nohup` background process in `entrypoint.sh`

**Decision:** Start the HTTP server with `nohup node dist/server.js > /tmp/claude-agent-server.log 2>&1 &` and poll `/health` before proceeding.

**Rationale:** The existing entrypoint already uses `nohup ... &` for orangefs. Matching the pattern keeps the entrypoint consistent. A full process supervisor (e.g. `supervisord`) would add a new dependency, configuration file, and complicate log routing.

**Alternatives considered:**
- `supervisord` — more reliable restart, but heavyweight for a single auxiliary service.
- Separate init system (tini + `CMD`) — would require restructuring the multi-process model.

### 3. Port allocation

**Decision:** HTTP server listens on port **3000** (the framework default, controlled by `PORT` env var). Jupyter keeps port 44771. No conflict.

**Rationale:** 3000 is already the default in the server's config and documentation; no reason to diverge. The opensandbox platform exposes arbitrary ports via its sandbox creation API — callers pass `exposed_ports` at creation time.

### 4. Environment variable injection

**Decision:** The sandbox creation call (or opensandbox config) injects `PORT=3000`, `HOST=0.0.0.0`, and `ANTHROPIC_API_KEY` as container env vars. `CLAUDE_WRAPPER_REQUIRE_AUTH_TOKEN` is optional.

**Rationale:** The server's `config.ts` already reads all configuration from env vars (Zod-validated). No config file format needs to be added. Secrets (API key, auth token) must not be baked into the image — they must be injected at runtime.

## Risks / Trade-offs

- **OFS mount takes ~2 seconds** before the entrypoint proceeds; the HTTP server starts in parallel, so total startup overhead is negligible.
- **`ANTHROPIC_API_KEY` must be injected at runtime** — if it is missing the server exits during Zod validation. Sandbox creation must include it; failure mode is a container that starts but has no HTTP server. Mitigation: the health-check loop will time out and the entrypoint will log clearly.
- **Node.js version mismatch** — the builder stage uses whatever Node is on the build host; the final image uses the Node bundled in the base image. Mitigation: pin the builder to `node:20` (matching the base image) and validate in CI.
- **Log rotation** — the server writes to `/tmp/claude-agent-server.log` with no rotation. For long-lived containers this could grow. Mitigation: acceptable for sandbox workloads which are ephemeral; pino's structured JSON makes it easy to tail/grep.

## Migration Plan

1. Build the updated image locally: `docker build -t my-sandbox:latest docker/docker/`
2. Update sandbox creation calls to include `PORT=3000` and `ANTHROPIC_API_KEY` in env.
3. After container starts, reach the server at `http://<container-ip>:3000/health`.
4. No rollback required — the old image is unchanged; switching back means using the old tag.

## Open Questions

- Should the HTTP server port be configurable per-sandbox, or is a fixed `3000` sufficient?
- Does the opensandbox platform need the port pre-declared in `config.toml`, or can it be specified per-sandbox-creation call?
