# src/routes — Express Route Handlers

> Navigation: [Root](../../../CLAUDE.md) | [src/lib/claude](../lib/claude/CLAUDE.md)

## Purpose

Thin HTTP boundary layer. Routes validate request inputs, call service functions, and format responses. Business logic lives in `src/lib/claude/session-service.ts` — keep it there.

## Directory Map

```
src/routes/
  sessions.ts   All /sessions/* endpoints
  health.ts     GET /health
  docs.ts       GET /docs (Swagger UI)
```

## Endpoint Map

### `sessions.ts` — sessionsRouter

| Method | Path | Description |
|---|---|---|
| GET | `/sessions` | List stored sessions |
| POST | `/sessions` | Create session + run prompt (batch or SSE) |
| GET | `/sessions/:sessionId` | Get session info + runtime status |
| PATCH | `/sessions/:sessionId` | Rename or tag a session |
| GET | `/sessions/:sessionId/messages` | Get stored messages |
| POST | `/sessions/:sessionId/messages` | Send message to existing session (batch or SSE) |
| POST | `/sessions/:sessionId/abort` | Abort an active run |
| POST | `/sessions/:sessionId/fork` | Fork a session |
| POST | `/sessions/:sessionId/rewind` | Roll back file changes (requires checkpointing) |
| GET | `/sessions/:sessionId/commands` | List slash commands (requires active run) |
| GET | `/sessions/:sessionId/models` | List available models (requires active run) |
| GET | `/sessions/:sessionId/agents` | List available agents (requires active run) |
| GET | `/sessions/:sessionId/context` | Get token usage (requires active run) |
| GET | `/sessions/:sessionId/subagents/:agentId/messages` | Get messages from a subagent session (agentId is the subagent's session ID) |
| PATCH | `/sessions/:sessionId/model` | Hot-swap model on active run (streaming input mode only) |
| PATCH | `/sessions/:sessionId/permission-mode` | Change permission mode on active run (streaming input mode only) |

Endpoints that return `501 Not Implemented`: `GET /sessions/:sessionId/subagents` (no SDK API exists to enumerate spawned subagents for a parent session).

### `health.ts` — healthRouter

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Returns `{ healthy, service, host, port, timestamp }` |

## Key Flows

### Batch response (stream: false or omitted)

1. Parse and validate request body/query with the appropriate Zod schema from `session-service.ts`
2. Call the service function (e.g. `execute(promptInput(...))`)
3. Return JSON directly: `res.json(...)` or `res.status(201).json(...)`

### SSE streaming (stream: true)

1. Call `openSse(res)` to set headers and flush
2. Pass `requestAbortSignal(req, res)` so client disconnects abort the SDK query
3. Call `execute(input, (event) => writeSseEvent(res, event))` — each normalized event is emitted in real time
4. On completion: emit a `session.completed` event with `{ sessionId, subtype }`
5. On error: emit via `writeSseError(res, error)`
6. Always: call `closeSse(res)` in `finally`

## Working Notes

- `sessionIdParam()` extracts and validates `:sessionId` from `req.params`. It throws (not returns null) so callers don't need a null check.
- `promptInput()` assembles `ExecutePromptInput` from the parsed body, applying only defined optional fields to avoid violating `exactOptionalPropertyTypes`.
- `session.completed` is the terminal SSE event for streaming prompts. Consumers should watch for it to know when the stream is done.
- Introspection endpoints (`/commands`, `/models`, `/agents`, `/context`) all require an active SDK run. They return `409` if the session is idle and `404` if it does not exist — this comes from `requireActiveQuery()` in `session-service.ts`.
- All handlers use `asyncHandler` from `src/lib/http/errors.ts` — do not use raw async route handlers.

## Scan Snapshot

- Date: 2026-04-21
- Files reviewed: sessions.ts, health.ts, docs.ts
