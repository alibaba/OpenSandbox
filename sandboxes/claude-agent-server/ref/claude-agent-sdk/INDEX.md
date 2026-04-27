# Claude Agent SDK — Reference Documentation

Fetched from https://code.claude.com/docs/en/agent-sdk on 2026-04-20.  
29 pages crawled. Each file contains the full page markdown as-scraped.

## Pages

| File | Topic | Size |
|------|-------|------|
| [overview.md](overview.md) | SDK overview, capabilities, comparison to Anthropic API | 17 KB |
| [quickstart.md](quickstart.md) | Getting started guide — install, first agent, run | 14 KB |
| [agent-loop.md](agent-loop.md) | How the autonomous agent loop works internally | 35 KB |
| [sessions.md](sessions.md) | Session state, conversation history, resume | 18 KB |
| [streaming-vs-single-mode.md](streaming-vs-single-mode.md) | Streaming input vs single-turn input modes | 7 KB |
| [streaming-output.md](streaming-output.md) | Real-time streaming of agent responses | 14 KB |
| [user-input.md](user-input.md) | Handling approvals and user input during agent runs | 28 KB |
| [custom-tools.md](custom-tools.md) | Define custom tools via in-process MCP server | 28 KB |
| [mcp.md](mcp.md) | Connect to external tools with MCP servers | 21 KB |
| [subagents.md](subagents.md) | Define and invoke subagents, parallel tasks | 25 KB |
| [skills.md](skills.md) | Agent Skills — SKILL.md-based capability packages | 13 KB |
| [plugins.md](plugins.md) | Programmatically load plugins (commands, agents, MCP) | 12 KB |
| [hooks.md](hooks.md) | Intercept and control agent behavior with hooks | 33 KB |
| [permissions.md](permissions.md) | Permission modes and rules for tool access | 14 KB |
| [structured-outputs.md](structured-outputs.md) | Get typed/structured data back from agents | 14 KB |
| [tool-search.md](tool-search.md) | Scale to hundreds of tools with dynamic tool search | 8 KB |
| [slash-commands.md](slash-commands.md) | Available slash commands (/compact, /clear, etc.) | 15 KB |
| [modifying-system-prompts.md](modifying-system-prompts.md) | Override or extend the default system prompt | 20 KB |
| [file-checkpointing.md](file-checkpointing.md) | Rewind file changes — checkpoint and restore | 23 KB |
| [todo-tracking.md](todo-tracking.md) | Built-in todo list for complex multi-step workflows | 6 KB |
| [cost-tracking.md](cost-tracking.md) | Track token usage, cost, and prompt caching | 16 KB |
| [observability.md](observability.md) | OpenTelemetry tracing, metrics, log events | 13 KB |
| [hosting.md](hosting.md) | Deploying stateful agents — infra considerations | 12 KB |
| [secure-deployment.md](secure-deployment.md) | Security hardening for production agent deployments | 27 KB |
| [claude-code-features.md](claude-code-features.md) | CLAUDE.md, memory, project config inside SDK | 18 KB |
| [typescript.md](typescript.md) | TypeScript SDK API reference (complete) | 133 KB |
| [typescript-v2-preview.md](typescript-v2-preview.md) | TypeScript SDK V2 preview — session-based API | 12 KB |
| [python.md](python.md) | Python SDK API reference (complete) | 133 KB |
| [migration-guide.md](migration-guide.md) | Migrate from old Claude Code SDK to Agent SDK | 12 KB |

## Key concepts

- **Agent loop** — autonomous tool-calling loop; see `agent-loop.md`
- **Sessions** — stateful conversation history; see `sessions.md`
- **Custom tools** — in-process MCP server pattern; see `custom-tools.md`
- **Subagents** — parallel / isolated task execution; see `subagents.md`
- **Hooks** — `PreToolCall`, `PostToolCall`, `Stop` lifecycle hooks; see `hooks.md`
- **Permissions** — `bypassPermissions`, `allowedTools`, rules; see `permissions.md`
- **Streaming** — `query()` vs streaming input mode; see `streaming-vs-single-mode.md`

## Source

Scraped via `firecrawl crawl` from `https://code.claude.com/docs/en/agent-sdk`.  
Raw crawl JSON cached at `.firecrawl/crawl-agent-sdk.json`.
