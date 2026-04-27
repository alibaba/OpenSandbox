# console/ — Web Management UI

> Navigation: [Root](../CLAUDE.md)

## Purpose

Owns the browser-based management console for OpenSandbox. Provides sandbox CRUD, pre-warmed pool management, an interactive chat console (SSE streaming to the in-sandbox Claude agent), and a diagnostics viewer. Does not own server logic, SDK generation, or API contract definitions.

## Entry Points

- `src/main.tsx` — React DOM mount; wraps `<App />` in `<StrictMode>`
- `src/App.tsx` — Router setup (`/` → Dashboard, `/pools` → Pools), React Query config, global settings modal and toast state
- `index.html` — Single-page app shell

## Directory Map

```text
console/
├── src/
│   ├── api/                  # HTTP + SSE client layer
│   │   ├── client.ts         # apiFetch(), apiFetchSSE(), settings, connection status
│   │   ├── types.ts          # TypeScript types mirroring server schema
│   │   ├── sandboxes.ts      # Sandbox CRUD and lifecycle actions
│   │   ├── pools.ts          # Pool management endpoints
│   │   ├── sse.ts            # Streaming session via eventsource-parser
│   │   └── devops.ts         # Diagnostics: logs, inspect, events, summary
│   ├── components/           # Shared UI
│   │   ├── Layout.tsx        # NavBar + Outlet wrapper
│   │   ├── NavBar.tsx        # Nav links + connection indicator + settings button
│   │   ├── SettingsPanel.tsx # Modal for serverUrl / authToken (localStorage)
│   │   ├── CreateSandboxModal.tsx  # Sandbox creation form (image, resources, env, volumes)
│   │   ├── CreatePoolModal.tsx     # Pool creation form
│   │   ├── ConfirmDialog.tsx       # Reusable confirmation dialog
│   │   ├── StatusBadge.tsx         # Sandbox state indicator
│   │   ├── ToastContainer.tsx      # 4s auto-dismiss toasts
│   │   └── Skeleton.tsx            # Loading placeholder tables
│   ├── features/
│   │   ├── console/          # SSE chat interface
│   │   │   ├── ConsolePanelDrawer.tsx  # Streaming session, tool-use handling
│   │   │   ├── MessageList.tsx         # Message rendering
│   │   │   └── PromptInput.tsx         # User prompt input
│   │   ├── diagnostics/
│   │   │   └── DiagnosticsDrawer.tsx   # Logs / inspect / events / summary tabs
│   │   └── pools/
│   │       └── PoolDetailDrawer.tsx    # Pool detail view
│   ├── hooks/
│   │   ├── useSettings.ts             # Settings state
│   │   ├── useToast.ts                # Toast notification logic
│   │   └── useConnectionStatus.ts     # Server reachability indicator
│   └── pages/
│       ├── DashboardPage.tsx          # Sandbox table, actions, inline drawers
│       └── PoolsPage.tsx              # Pool list, resize, detail drawer
├── vite.config.ts            # Vite + React plugin; path alias @/ → ./src/
├── tailwind.config.js        # JetBrains Mono font; dark neutral-950 theme
├── tsconfig.app.json         # Strict ES2020, JSX react-jsx, path alias @/*
├── Dockerfile                # Multi-stage Node 22 Alpine build
└── index.html
```

## Key Flows

### 1. Sandbox Lifecycle Action (e.g. Pause)

1. User clicks action in `DashboardPage` action menu.
2. `useMutation()` calls `pauseSandbox(id)` in `src/api/sandboxes.ts`.
3. `apiFetch()` in `client.ts` attaches `OPEN-SANDBOX-API-KEY` header from settings.
4. On success, React Query invalidates the sandbox list query → table re-fetches.
5. `useToast` shows success or `ApiRequestError` message.

### 2. SSE Console Session

1. User opens `ConsolePanelDrawer` for a sandbox.
2. `getSandboxEndpoint()` resolves the proxy URL for the target port.
3. `streamSession()` in `sse.ts` POSTs the prompt and reads the response body as a stream.
4. `eventsource-parser` parses SSE events; each delta appends to the `ChatMessage` in state.
5. Tool-use and tool-result events are handled inline; `isStreaming` flag cleared on `done`.
6. AbortController cancels the stream if the drawer closes mid-stream.

### 3. Connection Status

1. Every `apiFetch()` call updates connection status via listener callbacks.
2. `NavBar` subscribes via `useConnectionStatus` to show a green/red/gray indicator dot.
3. Settings panel at `SettingsPanel.tsx` lets the user change `serverUrl` and `authToken`; changes saved to `localStorage` under key `opensandbox_settings`.

## Interfaces and Dependencies

- **Auth header**: `OPEN-SANDBOX-API-KEY: <token>` on every API request.
- **Settings**: Persisted in `localStorage` (`opensandbox_settings`). Loaded by `client.ts:loadSettings()`.
- **Sandbox types**: `src/api/types.ts` mirrors the server schema. When the server schema changes, update here first.
- **Path alias**: `@/` resolves to `./src/`. Use this for all internal imports.
- **React Query stale time**: 5 s; 1 retry on failure.

## Tests

No automated test suite is present in this directory. Verification is currently manual:

1. `npm run check` — TypeScript type-check (no emit).
2. `npm run build` — Full type-check + Vite production build; must succeed before shipping.
3. Manual smoke test against a running OpenSandbox server.

Known gap: no unit or integration tests for API functions or React components.

## Build and Run

```bash
# Dev server (http://localhost:5173)
npm run dev

# Production build → dist/
npm run build

# Preview production build
npm run preview

# Type-check only
npm run check

# Via Docker Compose (http://localhost:8091 by default)
docker compose up console
```

## Working Notes

- `CreateSandboxModal.tsx` is ~968 lines — the largest single file. Keep additions to named sub-sections; avoid growing it further without refactoring.
- `ConsolePanelDrawer.tsx` manages streaming state manually. If the drawer unmounts while a stream is active, the AbortController must be called — double-check any refactor that moves mount lifecycle.
- `apiFetch()` in `client.ts` is the single choke point for connection status and auth. All API modules must go through it; do not use raw `fetch`.
- `agentation` in `main.tsx` is an analytics/tracking library. Leave it in place unless explicitly removing.
- The `dist/` directory is gitignored build output — do not hand-edit it.

## Scan Snapshot

- Date: 2026-04-27
- Scope: full directory scan
- Files reviewed: package.json, vite.config.ts, tsconfig.*, tailwind.config.js, src/main.tsx, src/App.tsx, src/api/*, src/components/*, src/features/*, src/hooks/*, src/pages/*
