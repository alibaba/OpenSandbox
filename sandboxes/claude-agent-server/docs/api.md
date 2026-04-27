# claude-agent-server — API reference

Thin HTTP wrapper around the `@anthropic-ai/claude-agent-sdk` stable API.  
Claude Code / the SDK remain the source of truth for session history and behavior.

---

## Base URL

```
http://<HOST>:<PORT>        default: http://0.0.0.0:3000
```

---

## Authentication

Optional. Set `CLAUDE_WRAPPER_REQUIRE_AUTH_TOKEN=<token>` at startup.  
When set, every request must include:

```
Authorization: Bearer <token>
```

Requests without a valid token receive `401 Unauthorized`.

---

## Common error shape

All errors return JSON:

```json
{
  "error": {
    "message": "Human-readable description",
    "details": null
  }
}
```

Validation errors (`400`) return structured details:

```json
{
  "error": {
    "message": "Validation error",
    "details": [
      { "path": "options.permissionMode", "message": "Invalid enum value" }
    ]
  }
}
```

---

## Endpoints

### `GET /health`

Liveness check. No auth required.

**Response `200`**

```json
{
  "healthy": true,
  "service": "claude-agent-server",
  "host": "0.0.0.0",
  "port": 3000,
  "timestamp": "2026-04-17T12:00:00.000Z"
}
```

---

### `GET /sessions`

List persisted Claude Code sessions from the SDK store.

**Query parameters**

| Name | Type | Description |
|---|---|---|
| `dir` | string | Claude data directory override |
| `limit` | integer | Max sessions to return |
| `offset` | integer | Pagination offset |
| `includeWorktrees` | boolean | Include worktree sessions |

**Response `200`**

```json
{
  "sessions": [
    {
      "sessionId": "abc123",
      "summary": "...",
      "lastModified": "2026-04-17T12:00:00.000Z",
      "fileSize": 4096,
      "customTitle": null,
      "firstPrompt": "Hello",
      "gitBranch": "main",
      "cwd": "/home/user/project",
      "tag": null,
      "createdAt": "2026-04-17T11:00:00.000Z"
    }
  ]
}
```

---

### `POST /sessions`

Start a new Claude Code session by sending the first prompt. The session ID is
assigned by the SDK and returned in the response.

**Request body**

```json
{
  "prompt": "Explain this codebase",
  "stream": false,
  "includePartialMessages": false,
  "options": { }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `prompt` | string | yes | First user message |
| `stream` | boolean | no | Return SSE stream instead of a batch response |
| `includePartialMessages` | boolean | no | Include in-progress assistant messages in the stream |
| `options` | object | no | Per-request SDK options (see [Query options](#query-options)) |

**Response `201`** (non-stream)

```json
{
  "sessionId": "abc123",
  "result": { "subtype": "success", ... },
  "events": [ ... ]
}
```

**Response `200`** (stream, `"stream": true`)  
Content-Type: `text/event-stream`  
See [SSE events](#sse-events).

---

### `GET /sessions/:sessionId`

Fetch metadata for a single session.

**Query parameters**

| Name | Type | Description |
|---|---|---|
| `dir` | string | Claude data directory override |

**Response `200`**

```json
{
  "session": {
    "sessionId": "abc123",
    "summary": "...",
    "lastModified": "2026-04-17T12:00:00.000Z",
    "fileSize": 4096,
    "customTitle": null,
    "firstPrompt": "Explain this codebase",
    "gitBranch": "main",
    "cwd": "/home/user/project",
    "tag": null,
    "createdAt": "2026-04-17T11:00:00.000Z",
    "runtime": null
  }
}
```

`runtime` is `null` when the session is idle, or:

```json
{
  "sessionId": "abc123",
  "status": "running",
  "startedAt": 1713355200000
}
```

**Errors:** `404` if the session does not exist.

---

### `PATCH /sessions/:sessionId`

Rename a session and/or set its tag. At least one of `title` or `tag` is required.

**Request body**

```json
{
  "title": "My project refactor",
  "tag": "important",
  "dir": "/optional/override"
}
```

| Field | Type | Description |
|---|---|---|
| `title` | string | New custom title (non-empty) |
| `tag` | string \| null | Tag string, or `null` to clear |
| `dir` | string | Claude data directory override |

**Response `200`** — same shape as `GET /sessions/:sessionId`.

**Errors:** `400` if neither `title` nor `tag` is present; `404` if not found.

---

### `GET /sessions/:sessionId/messages`

Fetch the raw message log for a session from the SDK store.

**Query parameters**

| Name | Type | Description |
|---|---|---|
| `dir` | string | Claude data directory override |
| `limit` | integer | Max messages to return |
| `offset` | integer | Pagination offset |
| `includeSystemMessages` | boolean | Include system messages |

**Response `200`**

```json
{
  "messages": [
    {
      "type": "human",
      "uuid": "...",
      "sessionId": "abc123",
      "message": { ... },
      "parentToolUseId": null
    }
  ]
}
```

---

### `POST /sessions/:sessionId/messages`

Send a follow-up prompt to an existing session.

**Request body**

```json
{
  "prompt": "Now add tests",
  "stream": false,
  "includePartialMessages": false,
  "forkSession": false,
  "options": { }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `prompt` | string | yes | Follow-up user message |
| `stream` | boolean | no | Return SSE stream |
| `includePartialMessages` | boolean | no | Include partial assistant messages in stream |
| `forkSession` | boolean | no | Fork the session before sending (creates a new session ID) |
| `options` | object | no | Per-request SDK options (see [Query options](#query-options)) |

**Response `200`** (non-stream)

```json
{
  "sessionId": "abc123",
  "result": { "subtype": "success", ... },
  "events": [ ... ]
}
```

**Response `200`** (stream)  
Content-Type: `text/event-stream`  
See [SSE events](#sse-events).

**Errors:**  
`404` if the session does not exist;  
`409` if the session already has an active run (and `forkSession` is not set).

---

### `POST /sessions/:sessionId/abort`

Interrupt the active run for a session via `Query.interrupt()`.

**Response `200`**

```json
{
  "ok": true,
  "sessionId": "abc123",
  "previousStatus": "running"
}
```

**Errors:**  
`404` if the session does not exist at all;  
`409` if the session exists but has no active run.

---

### `POST /sessions/:sessionId/fork`

Fork a session into a new independent session via the SDK `forkSession` API.  
The original session is not modified.

**Request body**

```json
{
  "title": "New branch",
  "upToMessageId": "optional-message-uuid",
  "dir": "/optional/override"
}
```

| Field | Type | Description |
|---|---|---|
| `title` | string | Custom title for the new session |
| `upToMessageId` | string | Fork only up to (and including) this message |
| `dir` | string | Claude data directory override |

**Response `201`** — same shape as `GET /sessions/:sessionId`.

---

## Query options

Accepted in `options` on `POST /sessions` and `POST /sessions/:sessionId/messages`:

| Field | Type | Default | Description |
|---|---|---|---|
| `cwd` | string | SDK default | Working directory for Claude Code |
| `model` | string | `CLAUDE_WRAPPER_DEFAULT_MODEL` env | Model identifier |
| `permissionMode` | enum | `CLAUDE_WRAPPER_DEFAULT_PERMISSION_MODE` env (`default`) | `default`, `acceptEdits`, `plan`, `dontAsk`, `auto` — **`bypassPermissions` is rejected** |
| `settingSources` | `("user"\|"project"\|"local")[]` | `["project","user","local"]` | Which Claude Code config files to load |
| `systemPrompt` | string | claude_code preset | Override the full system prompt |
| `appendSystemPrompt` | string | — | Append to the claude_code preset system prompt |
| `allowedTools` | string[] | — | Tool allow-list |
| `disallowedTools` | string[] | — | Tool deny-list |
| `additionalDirectories` | string[] | — | Extra directories Claude Code may access |
| `tools` | string[] \| `{type:"preset",preset:"claude_code"}` | claude_code preset | Custom tool set or preset |
| `maxTurns` | integer | SDK default | Maximum agentic turns before stopping |
| `enableFileCheckpointing` | boolean | `false` | Snapshot files before each tool execution, enabling `POST /sessions/:sessionId/rewind` |

---

### `POST /sessions/:sessionId/rewind`

Roll back file changes to the state at a prior user message turn.

**Requires:** the session must have been started (or resumed) with `options.enableFileCheckpointing: true`, and the session must currently have an active run.

**Request body**

```json
{
  "userMessageId": "<uuid-of-user-message>"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `userMessageId` | string | yes | UUID of the user message to rewind to |
| `dryRun` | boolean | no | If `true`, report what would change without modifying files |

**Response `200`**

```json
{
  "canRewind": true,
  "filesChanged": ["src/app.ts", "src/lib/config.ts"],
  "insertions": 12,
  "deletions": 8
}
```

| Field | Type | Description |
|---|---|---|
| `canRewind` | boolean | Whether the rewind was (or would be) possible |
| `filesChanged` | string[] | Paths of files that were (or would be) reverted |
| `insertions` | integer | Lines re-added |
| `deletions` | integer | Lines removed |
| `error` | string | Present when `canRewind` is `false` |

**Errors:**  
`400` if the request body is invalid;  
`404` if the session does not exist;  
`409` if the session has no active run.

---

### `GET /sessions/:sessionId/commands`

List the slash commands supported by the active Claude Code session.

**Requires:** an active run on the session.

**Response `200`**

```json
{
  "commands": [
    {
      "name": "compact",
      "description": "Summarize the conversation to free up context",
      "argumentHint": ""
    }
  ]
}
```

**Errors:** `404` if not found; `409` if no active run.

---

### `GET /sessions/:sessionId/models`

List the models available in the active session.

**Requires:** an active run on the session.

**Response `200`**

```json
{
  "models": [
    {
      "value": "claude-opus-4-5",
      "displayName": "Claude Opus 4.5",
      "description": "...",
      "supportsEffort": true,
      "supportedEffortLevels": ["low", "medium", "high", "xhigh", "max"]
    }
  ]
}
```

**Errors:**  
`404` if the session does not exist;  
`409` if the session has no active run.

---

### `GET /sessions/:sessionId/agents`

List the agents available in the active session.

**Requires:** an active run on the session.

**Response `200`**

```json
{
  "agents": [
    {
      "name": "default",
      "description": "Main Claude Code agent"
    }
  ]
}
```

**Errors:**  
`404` if the session does not exist;  
`409` if the session has no active run.

---

### `GET /sessions/:sessionId/context`

Return the context-window (token) usage breakdown for the active session.

**Requires:** an active run on the session.

**Response `200`**

```json
{
  "categories": [
    { "name": "System prompt", "tokens": 8192, "color": "#4a90d9" }
  ],
  "totalTokens": 24000,
  "maxTokens": 200000,
  "rawMaxTokens": 200000,
  "percentage": 12,
  "model": "claude-opus-4-5",
  "memoryFiles": []
}
```

**Errors:**  
`404` if the session does not exist;  
`409` if the session has no active run.

---

### `GET /sessions/:sessionId/subagents` *(not yet implemented)*

List subagent IDs that ran during the session. Returns `501 Not Implemented`.

---

### `GET /sessions/:sessionId/subagents/:agentId/messages` *(not yet implemented)*

Retrieve a subagent's message transcript. Returns `501 Not Implemented`.

---

### `PATCH /sessions/:sessionId/model` *(not yet implemented)*

Hot-swap the model on a running session. Returns `501 Not Implemented`.

---

### `PATCH /sessions/:sessionId/permission-mode` *(not yet implemented)*

Change the permission mode on a running session. Returns `501 Not Implemented`.

---

## SSE events

When `"stream": true`, the response is a `text/event-stream`.  
Each event has the format:

```
event: <event-name>
data: <JSON object>

```

### Event sequence

A typical successful prompt run emits events in roughly this order:

1. `session.init` — session opened, tools and model confirmed
2. `session.status` — status change (e.g. `running`)
3. `message.delta` *(repeated, only if `includePartialMessages: true`)* — streaming assistant token chunk
4. `task.started` *(optional)* — a sub-task (tool use) has started
5. `task.progress` *(repeated, optional)* — sub-task progress update
6. `task.notification` *(optional)* — sub-task completed
7. `message.assistant` — complete assistant message
8. `result` — final result
9. `session.completed` — stream is finishing (added by the server, not the SDK)

Unrecognized SDK messages are forwarded as `message.raw`.

### `session.init`

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "cwd": "/home/user/project",
  "model": "claude-opus-4-5",
  "tools": [ ... ],
  "permissionMode": "default",
  "slashCommands": [ ... ],
  "skills": [ ... ],
  "mcpServers": [ ... ],
  "claudeCodeVersion": "1.x.x"
}
```

### `session.status`

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "status": "running",
  "permissionMode": null,
  "compactResult": null,
  "compactError": null
}
```

### `message.assistant`

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "text": "Here is the explanation…",
  "message": { "role": "assistant", "content": [ ... ] },
  "parentToolUseId": null,
  "error": null
}
```

### `message.delta`

Emitted only when `includePartialMessages: true`.

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "event": { ... },
  "parentToolUseId": null,
  "ttftMs": 312
}
```

### `task.started`

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "taskId": "task-1",
  "description": "Reading file src/app.ts",
  "taskType": "tool_use",
  "toolUseId": "toolu_abc"
}
```

### `task.progress`

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "taskId": "task-1",
  "description": "Reading file src/app.ts",
  "toolUseId": "toolu_abc",
  "usage": { ... },
  "lastToolName": "Read",
  "summary": null
}
```

### `task.notification`

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "taskId": "task-1",
  "toolUseId": "toolu_abc",
  "status": "completed",
  "outputFile": null,
  "summary": "Read 120 lines",
  "usage": { ... }
}
```

### `result`

```json
{
  "sessionId": "abc123",
  "uuid": "...",
  "subtype": "success",
  "isError": false,
  "result": "Final text output from Claude",
  "errors": null,
  "stopReason": "end_turn",
  "terminalReason": null,
  "durationMs": 4200,
  "durationApiMs": 3800,
  "numTurns": 3,
  "totalCostUsd": 0.0042
}
```

### `session.completed`

Server-injected terminal event.

```json
{
  "sessionId": "abc123",
  "subtype": "success"
}
```

### `error`

Emitted if the server encounters an error during streaming.  
The stream closes immediately after.

```json
{
  "message": "Session abc123 already has an active run",
  "code": 409
}
```

---

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `3000` | TCP port to listen on |
| `HOST` | `0.0.0.0` | Bind address |
| `CLAUDE_WRAPPER_DEFAULT_MODEL` | *(SDK default)* | Default model for all prompts |
| `CLAUDE_WRAPPER_DEFAULT_PERMISSION_MODE` | `default` | Default permission mode (`default`, `acceptEdits`, `plan`, `dontAsk`, `auto`) |
| `CLAUDE_WRAPPER_DEFAULT_SETTING_SOURCES` | `project,user,local` | Comma-separated list of Claude config sources to load |
| `CLAUDE_WRAPPER_REQUIRE_AUTH_TOKEN` | *(unset)* | When set, require `Authorization: Bearer <token>` on all requests |

---

## What's next

The following are not yet implemented:

### Dockerfile / container setup
The server is designed to run inside a container alongside Claude Code.  
A `Dockerfile` and `docker-compose.yml` are needed to:
- install Claude Code and the Node runtime
- copy and build the server
- expose the port
- set a default `ANTHROPIC_API_KEY` mount or env

### Test suite
No automated tests exist yet. Recommended:
- **Unit tests** for `message-normalizer.ts` (pure function, easy to test)
- **Integration tests** for each route using a mocked SDK `query`
- Test framework: `vitest` (ESM-native, no config overhead)

### Dedicated SSE-only event stream endpoint
Currently SSE is opt-in per prompt (`"stream": true`).  
A `GET /sessions/:sessionId/events` endpoint could let clients subscribe to a running session's events independently of the request that started it.

### OpenAPI spec polish
The draft spec at `spec/openapi/claude-code-wrapper.openapi.json` needs:
- full request/response schemas aligned to the current implementation
- error response schemas for each status code
- concrete examples
- `components/schemas` reuse to avoid duplication

### `startup()` prewarming
The SDK exposes a `startup()` function to prewarm the Claude Code process.  
It has not been used yet. Calling it on server start could reduce first-prompt latency.

### Structured logging
All server output is currently `console.log`.  
A structured logger (e.g. `pino`) would help with filtering and observability in production.
