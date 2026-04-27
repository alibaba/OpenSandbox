## ADDED Requirements

### Requirement: HTTP server is packaged inside the sandbox image
The sandbox Docker image SHALL include a production build of `claude-agent-server` (compiled TypeScript output in `dist/` and production `node_modules/`) without build-time devDependencies.

#### Scenario: Image builds successfully with server artifacts
- **WHEN** `docker build` runs against the Dockerfile
- **THEN** the final image layer contains `/app/dist/server.js` and `/app/node_modules/`

#### Scenario: Build toolchain is excluded from the final image
- **WHEN** the image is built
- **THEN** TypeScript compiler (`tsc`, `tsx`) and devDependencies are NOT present in the final image layer

### Requirement: HTTP server starts automatically on container boot
The container entrypoint SHALL start the HTTP server as a background process before proceeding to mount OFS storage and launch Jupyter.

#### Scenario: Server process is running after container starts
- **WHEN** a sandbox container boots
- **THEN** a `node dist/server.js` process is running inside the container

#### Scenario: Server log is available for diagnostics
- **WHEN** the container is running
- **THEN** server stdout/stderr is captured at `/tmp/claude-agent-server.log`

### Requirement: Container startup is gated on HTTP server health
The entrypoint SHALL poll `GET /health` on the HTTP server and SHALL NOT proceed to the Jupyter process until the server responds with HTTP 200, or SHALL log a diagnostic and continue after a timeout.

#### Scenario: Healthy server unblocks startup
- **WHEN** the HTTP server returns HTTP 200 on `/health` within the polling window
- **THEN** the entrypoint proceeds to the OFS mount and Jupyter launch

#### Scenario: Unhealthy server logs and continues
- **WHEN** the HTTP server does not return HTTP 200 within 10 seconds (10 × 1-second polls)
- **THEN** the entrypoint logs a warning with the last error and continues so the container does not hang indefinitely

### Requirement: HTTP server port is configurable via environment variable
The HTTP server inside the container SHALL listen on the port specified by the `PORT` environment variable, defaulting to `3000` if unset.

#### Scenario: Default port used when PORT is not set
- **WHEN** the container starts without a `PORT` env var
- **THEN** the HTTP server listens on port 3000

#### Scenario: Custom port used when PORT is set
- **WHEN** the container starts with `PORT=4000`
- **THEN** the HTTP server listens on port 4000

### Requirement: ANTHROPIC_API_KEY is required at runtime
The HTTP server SHALL exit with a non-zero code during startup if `ANTHROPIC_API_KEY` is not set, because the Zod config validation will fail.

#### Scenario: Missing API key causes visible failure
- **WHEN** the container starts without `ANTHROPIC_API_KEY` set
- **THEN** the HTTP server process exits immediately and logs a config validation error to `/tmp/claude-agent-server.log`

#### Scenario: Valid API key allows server to start
- **WHEN** the container starts with a non-empty `ANTHROPIC_API_KEY`
- **THEN** the HTTP server starts, passes health check, and accepts requests

### Requirement: HTTP server is reachable from outside the container
The sandbox container SHALL expose the HTTP server port so that the platform or clients can reach it at `http://<container-ip>:<PORT>/`.

#### Scenario: Health endpoint reachable from host
- **WHEN** a sandbox container is running with the HTTP server started
- **THEN** `curl http://<container-ip>:<PORT>/health` returns HTTP 200 from the host network

#### Scenario: Session create endpoint reachable from host
- **WHEN** a sandbox container is running
- **THEN** `POST http://<container-ip>:<PORT>/sessions` creates a new Claude Code session and returns the session ID
