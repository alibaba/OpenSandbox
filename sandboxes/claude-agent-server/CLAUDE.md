# claude-agent-server — TypeScript HTTP Wrapper

> Navigation: [Root](../CLAUDE.md)

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
npm run dev          # Start with hot-reload (tsx watch)
npm run build        # Compile TypeScript → dist/
npm run start        # Run compiled output
npm run check        # Type-check only (no emit)
npx vitest run       # Run all tests once
npx vitest run tests/lib/http/errors.test.ts   # Run a single test file
npx vitest           # Run tests in watch mode
```

## Architecture

This is a thin HTTP wrapper around `@anthropic-ai/claude-agent-sdk` that exposes Claude Code sessions over a REST + SSE API.

**Entry → App → Routes → Service → SDK**

```
src/server.ts          binds port, starts Express
src/app.ts             factory: JSON body-parser, pino-http logging, optional Bearer auth, route mounting
src/routes/
  sessions.ts          all /sessions/* endpoints
  health.ts            GET /health
  docs.ts              Swagger UI
src/lib/
  config.ts            env var validation (Zod); single source of runtime config
  logger.ts            Pino logger singleton
  http/
    errors.ts          HttpError class, asyncHandler wrapper, Express errorHandler middleware
    sse.ts             SSE helpers (open/write/close) + requestAbortSignal
  claude/
    sdk-schemas.ts     Zod enums/schemas mirroring SDK types — dependency leaf, imported by both config.ts and session-service.ts
    session-service.ts core logic: wraps SDK query(), listSessions(), getSessionInfo(), fork, rename, tag, rewindFiles, introspection
    runtime-registry.ts  in-memory Map<sessionId, ActiveRun> tracking live Query handles; enables abort and active-session introspection
    message-normalizer.ts  maps raw SDKMessage types → NormalizedEvent {event, data} shapes used by both batch and SSE responses
```

**Key design points:**

- `runtimeRegistry` is the authority on whether a session has an active run. Endpoints that need a live Query handle (abort, rewind, commands, models, agents, context) call `requireActiveQuery()` which returns 404 (unknown session) or 409 (idle session) as appropriate.
- Streaming responses use SSE: `openSse` sets headers, `writeSseEvent` emits named events, `requestAbortSignal` fires on client disconnect (watches `res` close, not `req` close, to avoid false positives from body-parser).
- `permissionMode=bypassPermissions` is explicitly rejected at the service layer regardless of what the SDK accepts.
- All route handlers use `asyncHandler` to forward async errors to Express's error handler.
- `sdk-schemas.ts` is intentionally a dependency leaf — no local imports — so it can be safely imported by both `config.ts` (which runs at module load) and `session-service.ts`.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `3000` | Listen port |
| `HOST` | `0.0.0.0` | Listen host |
| `CLAUDE_WRAPPER_DEFAULT_MODEL` | — | Default model for all queries |
| `CLAUDE_WRAPPER_DEFAULT_PERMISSION_MODE` | `default` | Default permission mode |
| `CLAUDE_WRAPPER_DEFAULT_SETTING_SOURCES` | `project,user,local` | Comma-separated setting sources |
| `CLAUDE_WRAPPER_REQUIRE_AUTH_TOKEN` | — | When set, enforces `Authorization: Bearer <token>` |
| `LOG_LEVEL` | `info` | Pino log level |

## Local Guides

- [`src/lib/CLAUDE.md`](src/lib/CLAUDE.md) — config singleton, logger, and pointers to sub-areas
- [`src/lib/claude/CLAUDE.md`](src/lib/claude/CLAUDE.md) — SDK wrapper: execute flow, runtime registry, message normalizer, schemas
- [`src/lib/http/CLAUDE.md`](src/lib/http/CLAUDE.md) — error handling, SSE streaming, abort signal
- [`src/routes/CLAUDE.md`](src/routes/CLAUDE.md) — endpoint map, batch vs SSE flow, route conventions

## TypeScript config notes

- `"module": "NodeNext"` + `"moduleResolution": "NodeNext"` — all local imports **must** use `.js` extensions (e.g. `'./lib/config.js'`).
- `exactOptionalPropertyTypes: true` — optional properties must be explicitly `undefined`, not just omitted.
- `noUncheckedIndexedAccess: true` — array/map index access returns `T | undefined`.
- Tests live in `tests/` but are not included in the TypeScript project (`tsconfig.json` only covers `src/`). Vitest handles test compilation independently.

## Scan Snapshot

- Date: 2026-04-26
- Scope: src/ tree — server.ts, app.ts, all routes, all lib sub-areas
