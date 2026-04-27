# OpenSandbox Console

A web UI for managing OpenSandbox containers, streaming Claude sessions, and viewing diagnostics.

## Features

- **Dashboard** — list all sandboxes, pause/resume/renew/delete, create new ones
- **Console** — interactive chat panel connected to `claude-agent-server` inside any sandbox via SSE
- **Pools** — CRUD for pre-warmed sandbox pools
- **Diagnostics** — live logs, Docker inspect, event timeline, and diagnostics summary per sandbox

## Quick Start (local dev)

```bash
cd console
npm install
npm run dev       # starts Vite dev server at http://localhost:5173
```

On first load, the Settings panel opens automatically. Enter your `opensandbox-server` URL (e.g. `http://localhost:8090`) and an optional Bearer token.

## Docker Compose

```bash
# From the repo root — builds and serves the console at http://localhost:8091
docker compose up console

# Change port:
CONSOLE_PORT=9000 docker compose up console
```

## Settings

Settings are stored in `localStorage` and never leave the browser. You can change them anytime via the gear icon in the top-right corner.

| Field | Description |
|---|---|
| Server URL | Full base URL of opensandbox-server, e.g. `http://localhost:8090` |
| Bearer Token | Optional API key (set `server.api_key` in `config.toml`) |

## CORS

The console calls `opensandbox-server` directly from the browser. The server already allows all origins in its default CORS config. For production, restrict to the console's origin in `config.toml`.

## Build

```bash
npm run build     # outputs to dist/
npm run check     # TypeScript type-check only
```

## Development

```
src/
  api/            API client functions and SSE streaming
  components/     Shared UI components (NavBar, SettingsPanel, modals…)
  features/       Feature-specific components (console, diagnostics, pools)
  hooks/          Reusable React hooks
  pages/          Top-level route components
```
