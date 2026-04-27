## 1. Dockerfile — multi-stage build

- [x] 1.1 Add a `builder` stage (FROM node:20) that copies the repo, runs `npm ci`, and runs `npm run build`
- [x] 1.2 In the final stage, copy `/app/dist/` and `/app/node_modules/` from the builder into `/app/` in the sandbox image
- [x] 1.3 Verify the final image does NOT contain TypeScript compiler or devDependencies (inspect with `docker run --rm <image> ls /app/node_modules/.bin/tsc` — should fail)

## 2. entrypoint.sh — start HTTP server

- [x] 2.1 Before the OFS mount block, add `nohup node /app/dist/server.js > /tmp/claude-agent-server.log 2>&1 &` to start the server
- [x] 2.2 Add a polling loop (up to 10 × 1-second retries) that curls `http://localhost:${PORT:-3000}/health` and exits the loop on HTTP 200
- [x] 2.3 If the loop exhausts retries, log a warning to stderr and continue (do not block the container)

## 3. Environment variable wiring

- [x] 3.1 Document required env vars (`ANTHROPIC_API_KEY`, `PORT`, `HOST`) in `docker/README.md`
- [x] 3.2 Add an example sandbox-creation call to `docker/README.md` showing env injection (Python SDK or curl)

## 4. Port exposure

- [x] 4.1 Add `EXPOSE 3000` to the Dockerfile (documents the default port; actual binding is done by the opensandbox platform at creation time)

## 5. Smoke test

- [x] 5.1 Build the image locally: `docker build -t my-sandbox:test docker/docker/`
- [x] 5.2 Run a container with `ANTHROPIC_API_KEY`, `SESSION_ID`, `USERNAME` set and verify `/health` returns 200 from the host
- [x] 5.3 Confirm `POST /sessions` creates a session and returns a session ID
