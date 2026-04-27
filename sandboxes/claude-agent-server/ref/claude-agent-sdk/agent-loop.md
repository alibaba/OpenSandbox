[Skip to main content](https://code.claude.com/docs/en/agent-sdk/agent-loop#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Core concepts

How the agent loop works

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [The loop at a glance](https://code.claude.com/docs/en/agent-sdk/agent-loop#the-loop-at-a-glance)
- [Turns and messages](https://code.claude.com/docs/en/agent-sdk/agent-loop#turns-and-messages)
- [Message types](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types)
- [Handle messages](https://code.claude.com/docs/en/agent-sdk/agent-loop#handle-messages)
- [Tool execution](https://code.claude.com/docs/en/agent-sdk/agent-loop#tool-execution)
- [Built-in tools](https://code.claude.com/docs/en/agent-sdk/agent-loop#built-in-tools)
- [Tool permissions](https://code.claude.com/docs/en/agent-sdk/agent-loop#tool-permissions)
- [Parallel tool execution](https://code.claude.com/docs/en/agent-sdk/agent-loop#parallel-tool-execution)
- [Control how the loop runs](https://code.claude.com/docs/en/agent-sdk/agent-loop#control-how-the-loop-runs)
- [Turns and budget](https://code.claude.com/docs/en/agent-sdk/agent-loop#turns-and-budget)
- [Effort level](https://code.claude.com/docs/en/agent-sdk/agent-loop#effort-level)
- [Permission mode](https://code.claude.com/docs/en/agent-sdk/agent-loop#permission-mode)
- [Model](https://code.claude.com/docs/en/agent-sdk/agent-loop#model)
- [The context window](https://code.claude.com/docs/en/agent-sdk/agent-loop#the-context-window)
- [What consumes context](https://code.claude.com/docs/en/agent-sdk/agent-loop#what-consumes-context)
- [Automatic compaction](https://code.claude.com/docs/en/agent-sdk/agent-loop#automatic-compaction)
- [Keep context efficient](https://code.claude.com/docs/en/agent-sdk/agent-loop#keep-context-efficient)
- [Sessions and continuity](https://code.claude.com/docs/en/agent-sdk/agent-loop#sessions-and-continuity)
- [Handle the result](https://code.claude.com/docs/en/agent-sdk/agent-loop#handle-the-result)
- [Hooks](https://code.claude.com/docs/en/agent-sdk/agent-loop#hooks)
- [Put it all together](https://code.claude.com/docs/en/agent-sdk/agent-loop#put-it-all-together)
- [Next steps](https://code.claude.com/docs/en/agent-sdk/agent-loop#next-steps)

The Agent SDK lets you embed Claude Code’s autonomous agent loop in your own applications. The SDK is a standalone package that gives you programmatic control over tools, permissions, cost limits, and output. You don’t need the Claude Code CLI installed to use it.When you start an agent, the SDK runs the same [execution loop that powers Claude Code](https://code.claude.com/docs/en/how-claude-code-works#the-agentic-loop): Claude evaluates your prompt, calls tools to take action, receives the results, and repeats until the task is complete. This page explains what happens inside that loop so you can build, debug, and optimize your agents effectively.

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#the-loop-at-a-glance)  The loop at a glance

Every agent session follows the same cycle:![Agent loop: prompt enters, Claude evaluates, branches to tool calls or final answer](https://mintcdn.com/claude-code/gvy2DIUELtNA8qD3/images/agent-loop-diagram.svg?fit=max&auto=format&n=gvy2DIUELtNA8qD3&q=85&s=192e1bd6c8a2950a16e5ee0b94e27e26)

1. **Receive prompt.** Claude receives your prompt, along with the system prompt, tool definitions, and conversation history. The SDK yields a [`SystemMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) with subtype `"init"` containing session metadata.
2. **Evaluate and respond.** Claude evaluates the current state and determines how to proceed. It may respond with text, request one or more tool calls, or both. The SDK yields an [`AssistantMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) containing the text and any tool call requests.
3. **Execute tools.** The SDK runs each requested tool and collects the results. Each set of tool results feeds back to Claude for the next decision. You can use [hooks](https://code.claude.com/docs/en/agent-sdk/hooks) to intercept, modify, or block tool calls before they run.
4. **Repeat.** Steps 2 and 3 repeat as a cycle. Each full cycle is one turn. Claude continues calling tools and processing results until it produces a response with no tool calls.
5. **Return result.** The SDK yields a final [`AssistantMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) with the text response (no tool calls), followed by a [`ResultMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) with the final text, token usage, cost, and session ID.

A quick question (“what files are here?”) might take one or two turns of calling `Glob` and responding with the results. A complex task (“refactor the auth module and update the tests”) can chain dozens of tool calls across many turns, reading files, editing code, and running tests, with Claude adjusting its approach based on each result.

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#turns-and-messages)  Turns and messages

A turn is one round trip inside the loop: Claude produces output that includes tool calls, the SDK executes those tools, and the results feed back to Claude automatically. This happens without yielding control back to your code. Turns continue until Claude produces output with no tool calls, at which point the loop ends and the final result is delivered.Consider what a full session might look like for the prompt “Fix the failing tests in auth.ts”.First, the SDK sends your prompt to Claude and yields a [`SystemMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) with the session metadata. Then the loop begins:

1. **Turn 1:** Claude calls `Bash` to run `npm test`. The SDK yields an [`AssistantMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) with the tool call, executes the command, then yields a [`UserMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) with the output (three failures).
2. **Turn 2:** Claude calls `Read` on `auth.ts` and `auth.test.ts`. The SDK returns the file contents and yields an `AssistantMessage`.
3. **Turn 3:** Claude calls `Edit` to fix `auth.ts`, then calls `Bash` to re-run `npm test`. All three tests pass. The SDK yields an `AssistantMessage`.
4. **Final turn:** Claude produces a text-only response with no tool calls: “Fixed the auth bug, all three tests pass now.” The SDK yields a final `AssistantMessage` with this text, then a [`ResultMessage`](https://code.claude.com/docs/en/agent-sdk/agent-loop#message-types) with the same text plus cost and usage.

That was four turns: three with tool calls, one final text-only response.You can cap the loop with `max_turns` / `maxTurns`, which counts tool-use turns only. For example, `max_turns=2` in the loop above would have stopped before the edit step. You can also use `max_budget_usd` / `maxBudgetUsd` to cap turns based on a spend threshold.Without limits, the loop runs until Claude finishes on its own, which is fine for well-scoped tasks but can run long on open-ended prompts (“improve this codebase”). Setting a budget is a good default for production agents. See [Turns and budget](https://code.claude.com/docs/en/agent-sdk/agent-loop#turns-and-budget) below for the option reference.

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#message-types)  Message types

As the loop runs, the SDK yields a stream of messages. Each message carries a type that tells you what stage of the loop it came from. The five core types are:

- **`SystemMessage`:** session lifecycle events. The `subtype` field distinguishes them: `"init"` is the first message (session metadata), and `"compact_boundary"` fires after [compaction](https://code.claude.com/docs/en/agent-sdk/agent-loop#automatic-compaction). In TypeScript, the compact boundary is its own [`SDKCompactBoundaryMessage`](https://code.claude.com/docs/en/agent-sdk/typescript#sdk-compact-boundary-message) type rather than a subtype of `SDKSystemMessage`.
- **`AssistantMessage`:** emitted after each Claude response, including the final text-only one. Contains text content blocks and tool call blocks from that turn.
- **`UserMessage`:** emitted after each tool execution with the tool result content sent back to Claude. Also emitted for any user inputs you stream mid-loop.
- **`StreamEvent`:** only emitted when partial messages are enabled. Contains raw API streaming events (text deltas, tool input chunks). See [Stream responses](https://code.claude.com/docs/en/agent-sdk/streaming-output).
- **`ResultMessage`:** marks the end of the agent loop. Contains the final text result, token usage, cost, and session ID. Check the `subtype` field to determine whether the task succeeded or hit a limit. A small number of trailing system events, such as `prompt_suggestion`, can arrive after it, so iterate the stream to completion rather than breaking on the result. See [Handle the result](https://code.claude.com/docs/en/agent-sdk/agent-loop#handle-the-result).

These five types cover the full agent loop lifecycle in both SDKs. The TypeScript SDK also yields additional observability events (hook events, tool progress, rate limits, task notifications) that provide extra detail but are not required to drive the loop. See the [Python message types reference](https://code.claude.com/docs/en/agent-sdk/python#message-types) and [TypeScript message types reference](https://code.claude.com/docs/en/agent-sdk/typescript#message-types) for the complete lists.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#handle-messages)  Handle messages

Which messages you handle depends on what you’re building:

- **Final results only:** handle `ResultMessage` to get the output, cost, and whether the task succeeded or hit a limit.
- **Progress updates:** handle `AssistantMessage` to see what Claude is doing each turn, including which tools it called.
- **Live streaming:** enable partial messages (`include_partial_messages` in Python, `includePartialMessages` in TypeScript) to get `StreamEvent` messages in real time. See [Stream responses in real-time](https://code.claude.com/docs/en/agent-sdk/streaming-output).

How you check message types depends on the SDK:

- **Python:** check message types with `isinstance()` against classes imported from `claude_agent_sdk` (for example, `isinstance(message, ResultMessage)`).
- **TypeScript:** check the `type` string field (for example, `message.type === "result"`). `AssistantMessage` and `UserMessage` wrap the raw API message in a `.message` field, so content blocks are at `message.message.content`, not `message.content`.

Example: Check message types and handle results

Python

TypeScript

```
from claude_agent_sdk import query, AssistantMessage, ResultMessage

async for message in query(prompt="Summarize this project"):
    if isinstance(message, AssistantMessage):
        print(f"Turn completed: {len(message.content)} content blocks")
    if isinstance(message, ResultMessage):
        if message.subtype == "success":
            print(message.result)
        else:
            print(f"Stopped: {message.subtype}")
```

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#tool-execution)  Tool execution

Tools give your agent the ability to take action. Without tools, Claude can only respond with text. With tools, Claude can read files, run commands, search code, and interact with external services.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#built-in-tools)  Built-in tools

The SDK includes the same tools that power Claude Code:

| Category | Tools | What they do |
| --- | --- | --- |
| **File operations** | `Read`, `Edit`, `Write` | Read, modify, and create files |
| **Search** | `Glob`, `Grep` | Find files by pattern, search content with regex |
| **Execution** | `Bash` | Run shell commands, scripts, git operations |
| **Web** | `WebSearch`, `WebFetch` | Search the web, fetch and parse pages |
| **Discovery** | `ToolSearch` | Dynamically find and load tools on-demand instead of preloading all of them |
| **Orchestration** | `Agent`, `Skill`, `AskUserQuestion`, `TodoWrite` | Spawn subagents, invoke skills, ask the user, track tasks |

Beyond built-in tools, you can:

- **Connect external services** with [MCP servers](https://code.claude.com/docs/en/agent-sdk/mcp) (databases, browsers, APIs)
- **Define custom tools** with [custom tool handlers](https://code.claude.com/docs/en/agent-sdk/custom-tools)
- **Load project skills** via [setting sources](https://code.claude.com/docs/en/agent-sdk/claude-code-features) for reusable workflows

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#tool-permissions)  Tool permissions

Claude determines which tools to call based on the task, but you control whether those calls are allowed to execute. You can auto-approve specific tools, block others entirely, or require approval for everything. Three options work together to determine what runs:

- **`allowed_tools` / `allowedTools`** auto-approves listed tools. A read-only agent with `["Read", "Glob", "Grep"]` in its allowed tools list runs those tools without prompting. Tools not listed are still available but require permission.
- **`disallowed_tools` / `disallowedTools`** blocks listed tools, regardless of other settings. See [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions) for the order that rules are checked before a tool runs.
- **`permission_mode` / `permissionMode`** controls what happens to tools that aren’t covered by allow or deny rules. See [Permission mode](https://code.claude.com/docs/en/agent-sdk/agent-loop#permission-mode) for available modes.

You can also scope individual tools with rules like `"Bash(npm *)"` to allow only specific commands. See [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions) for the full rule syntax.When a tool is denied, Claude receives a rejection message as the tool result and typically attempts a different approach or reports that it couldn’t proceed.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#parallel-tool-execution)  Parallel tool execution

When Claude requests multiple tool calls in a single turn, both SDKs can run them concurrently or sequentially depending on the tool. Read-only tools (like `Read`, `Glob`, `Grep`, and MCP tools marked as read-only) can run concurrently. Tools that modify state (like `Edit`, `Write`, and `Bash`) run sequentially to avoid conflicts.Custom tools default to sequential execution. To enable parallel execution for a custom tool, mark it as read-only in its annotations: `readOnly` in [TypeScript](https://code.claude.com/docs/en/agent-sdk/typescript#tool) or `readOnlyHint` in [Python](https://code.claude.com/docs/en/agent-sdk/python#tool).

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#control-how-the-loop-runs)  Control how the loop runs

You can limit how many turns the loop takes, how much it costs, how deeply Claude reasons, and whether tools require approval before running. All of these are fields on [`ClaudeAgentOptions`](https://code.claude.com/docs/en/agent-sdk/python#claude-agent-options) (Python) / [`Options`](https://code.claude.com/docs/en/agent-sdk/typescript#options) (TypeScript).

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#turns-and-budget)  Turns and budget

| Option | What it controls | Default |
| --- | --- | --- |
| Max turns (`max_turns` / `maxTurns`) | Maximum tool-use round trips | No limit |
| Max budget (`max_budget_usd` / `maxBudgetUsd`) | Maximum cost before stopping | No limit |

When either limit is hit, the SDK returns a `ResultMessage` with a corresponding error subtype (`error_max_turns` or `error_max_budget_usd`). See [Handle the result](https://code.claude.com/docs/en/agent-sdk/agent-loop#handle-the-result) for how to check these subtypes and [`ClaudeAgentOptions`](https://code.claude.com/docs/en/agent-sdk/python#claude-agent-options) / [`Options`](https://code.claude.com/docs/en/agent-sdk/typescript#options) for syntax.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#effort-level)  Effort level

The `effort` option controls how much reasoning Claude applies. Lower effort levels use fewer tokens per turn and reduce cost. Not all models support the effort parameter. See [Effort](https://platform.claude.com/docs/en/build-with-claude/effort) for which models support it.

| Level | Behavior | Good for |
| --- | --- | --- |
| `"low"` | Minimal reasoning, fast responses | File lookups, listing directories |
| `"medium"` | Balanced reasoning | Routine edits, standard tasks |
| `"high"` | Thorough analysis | Refactors, debugging |
| `"xhigh"` | Extended reasoning depth | Coding and agentic tasks; recommended on Opus 4.7 |
| `"max"` | Maximum reasoning depth | Multi-step problems requiring deep analysis |

If you don’t set `effort`, the Python SDK leaves the parameter unset and defers to the model’s default behavior. The TypeScript SDK defaults to `"high"`.

`effort` trades latency and token cost for reasoning depth within each response. [Extended thinking](https://platform.claude.com/docs/en/build-with-claude/extended-thinking) is a separate feature that produces visible chain-of-thought blocks in the output. They are independent: you can set `effort: "low"` with extended thinking enabled, or `effort: "max"` without it.

Use lower effort for agents doing simple, well-scoped tasks (like listing files or running a single grep) to reduce cost and latency. `effort` is set at the top-level `query()` options, not per-subagent.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#permission-mode)  Permission mode

The permission mode option (`permission_mode` in Python, `permissionMode` in TypeScript) controls whether the agent asks for approval before using tools:

| Mode | Behavior |
| --- | --- |
| `"default"` | Tools not covered by allow rules trigger your approval callback; no callback means deny |
| `"acceptEdits"` | Auto-approves file edits and common filesystem commands (`mkdir`, `touch`, `mv`, `cp`, etc.); other Bash commands follow default rules |
| `"plan"` | No tool execution; Claude produces a plan for review |
| `"dontAsk"` | Never prompts. Tools pre-approved by [permission rules](https://code.claude.com/docs/en/settings#permission-settings) run, everything else is denied |
| `"auto"` (TypeScript only) | Uses a model classifier to approve or deny each tool call. See [Auto mode](https://code.claude.com/docs/en/permission-modes#eliminate-prompts-with-auto-mode) for availability and behavior |
| `"bypassPermissions"` | Runs all allowed tools without asking. Cannot be used when running as root on Unix. Use only in isolated environments where the agent’s actions cannot affect systems you care about |

For interactive applications, use `"default"` with a tool approval callback to surface approval prompts. For autonomous agents on a dev machine, `"acceptEdits"` auto-approves file edits and common filesystem commands (`mkdir`, `touch`, `mv`, `cp`, etc.) while still gating other `Bash` commands behind allow rules. Reserve `"bypassPermissions"` for CI, containers, or other isolated environments. See [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions) for full details.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#model)  Model

If you don’t set `model`, the SDK uses Claude Code’s default, which depends on your authentication method and subscription. Set it explicitly (for example, `model="claude-sonnet-4-6"`) to pin a specific model or to use a smaller model for faster, cheaper agents. See [models](https://platform.claude.com/docs/en/about-claude/models) for available IDs.

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#the-context-window)  The context window

The context window is the total amount of information available to Claude during a session. It does not reset between turns within a session. Everything accumulates: the system prompt, tool definitions, conversation history, tool inputs, and tool outputs. Content that stays the same across turns (system prompt, tool definitions, CLAUDE.md) is automatically [prompt cached](https://platform.claude.com/docs/en/build-with-claude/prompt-caching), which reduces cost and latency for repeated prefixes.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#what-consumes-context)  What consumes context

Here’s how each component affects context in the SDK:

| Source | When it loads | Impact |
| --- | --- | --- |
| **System prompt** | Every request | Small fixed cost, always present |
| **CLAUDE.md files** | Session start, via [`settingSources`](https://code.claude.com/docs/en/agent-sdk/claude-code-features) | Full content in every request (but prompt-cached, so only the first request pays full cost) |
| **Tool definitions** | Every request | Each tool adds its schema; use [MCP tool search](https://code.claude.com/docs/en/agent-sdk/mcp#mcp-tool-search) to load tools on-demand instead of all at once |
| **Conversation history** | Accumulates over turns | Grows with each turn: prompts, responses, tool inputs, tool outputs |
| **Skill descriptions** | Session start, via setting sources | Short summaries; full content loads only when invoked |

Large tool outputs consume significant context. Reading a big file or running a command with verbose output can use thousands of tokens in a single turn. Context accumulates across turns, so longer sessions with many tool calls build up significantly more context than short ones.

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#automatic-compaction)  Automatic compaction

When the context window approaches its limit, the SDK automatically compacts the conversation: it summarizes older history to free space, keeping your most recent exchanges and key decisions intact. The SDK emits a message with `type: "system"` and `subtype: "compact_boundary"` in the stream when this happens (in Python this is a `SystemMessage`; in TypeScript it is a separate `SDKCompactBoundaryMessage` type).Compaction replaces older messages with a summary, so specific instructions from early in the conversation may not be preserved. Persistent rules belong in CLAUDE.md (loaded via [`settingSources`](https://code.claude.com/docs/en/agent-sdk/claude-code-features)) rather than in the initial prompt, because CLAUDE.md content is re-injected on every request.You can customize compaction behavior in several ways:

- **Summarization instructions in CLAUDE.md:** The compactor reads your CLAUDE.md like any other context, so you can include a section telling it what to preserve when summarizing. The section header is free-form (not a magic string); the compactor matches on intent.
- **`PreCompact` hook:** Run custom logic before compaction occurs, for example to archive the full transcript. The hook receives a `trigger` field (`manual` or `auto`). See [hooks](https://code.claude.com/docs/en/agent-sdk/hooks).
- **Manual compaction:** Send `/compact` as a prompt string to trigger compaction on demand. (Slash commands sent this way are SDK inputs, not CLI-only shortcuts. See [slash commands in the SDK](https://code.claude.com/docs/en/agent-sdk/slash-commands).)

Example: Summarization instructions in CLAUDE.md

Add a section to your project’s CLAUDE.md telling the compactor what to preserve. The header name isn’t special; use any clear label.

CLAUDE.md

```
# Summary instructions

When summarizing this conversation, always preserve:
- The current task objective and acceptance criteria
- File paths that have been read or modified
- Test results and error messages
- Decisions made and the reasoning behind them
```

### [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#keep-context-efficient)  Keep context efficient

A few strategies for long-running agents:

- **Use subagents for subtasks.** Each subagent starts with a fresh conversation (no prior message history, though it does load its own system prompt and project-level context like CLAUDE.md). It does not see the parent’s turns, and only its final response returns to the parent as a tool result. The main agent’s context grows by that summary, not by the full subtask transcript. See [What subagents inherit](https://code.claude.com/docs/en/agent-sdk/subagents#what-subagents-inherit) for details.
- **Be selective with tools.** Every tool definition takes context space. Use the `tools` field on [`AgentDefinition`](https://code.claude.com/docs/en/agent-sdk/subagents#agent-definition-configuration) to scope subagents to the minimum set they need, and use [MCP tool search](https://code.claude.com/docs/en/agent-sdk/mcp#mcp-tool-search) to load tools on demand instead of preloading all of them.
- **Watch MCP server costs.** Each MCP server adds all its tool schemas to every request. A few servers with many tools can consume significant context before the agent does any work. The `ToolSearch` tool can help by loading tools on-demand instead of preloading all of them. See [MCP tool search](https://code.claude.com/docs/en/agent-sdk/mcp#mcp-tool-search) for configuration.
- **Use lower effort for routine tasks.** Set [effort](https://code.claude.com/docs/en/agent-sdk/agent-loop#effort-level) to `"low"` for agents that only need to read files or list directories. This reduces token usage and cost.

For a detailed breakdown of per-feature context costs, see [Understand context costs](https://code.claude.com/docs/en/features-overview#understand-context-costs).

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#sessions-and-continuity)  Sessions and continuity

Each interaction with the SDK creates or continues a session. Capture the session ID from `ResultMessage.session_id` (available in both SDKs) to resume later. The TypeScript SDK also exposes it as a direct field on the init `SystemMessage`; in Python it’s nested in `SystemMessage.data`.When you resume, the full context from previous turns is restored: files that were read, analysis that was performed, and actions that were taken. You can also fork a session to branch into a different approach without modifying the original.See [Session management](https://code.claude.com/docs/en/agent-sdk/sessions) for the full guide on resume, continue, and fork patterns.

In Python, `ClaudeSDKClient` handles session IDs automatically across multiple calls. See the [Python SDK reference](https://code.claude.com/docs/en/agent-sdk/python#choosing-between-query-and-claude-sdk-client) for details.

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#handle-the-result)  Handle the result

When the loop ends, the `ResultMessage` tells you what happened and gives you the output. The `subtype` field (available in both SDKs) is the primary way to check termination state.

| Result subtype | What happened | `result` field available? |
| --- | --- | --- |
| `success` | Claude finished the task normally | Yes |
| `error_max_turns` | Hit the `maxTurns` limit before finishing | No |
| `error_max_budget_usd` | Hit the `maxBudgetUsd` limit before finishing | No |
| `error_during_execution` | An error interrupted the loop (for example, an API failure or cancelled request) | No |
| `error_max_structured_output_retries` | Structured output validation failed after the configured retry limit | No |

The `result` field (the final text output) is only present on the `success` variant, so always check the subtype before reading it. All result subtypes carry `total_cost_usd`, `usage`, `num_turns`, and `session_id` so you can track cost and resume even after errors. In Python, `total_cost_usd` and `usage` are typed as optional and may be `None` on some error paths, so guard before formatting them. See [Tracking costs and usage](https://code.claude.com/docs/en/agent-sdk/cost-tracking) for details on interpreting the `usage` fields.The result also includes a `stop_reason` field (`string | null` in TypeScript, `str | None` in Python) indicating why the model stopped generating on its final turn. Common values are `end_turn` (model finished normally), `max_tokens` (hit the output token limit), and `refusal` (the model declined the request). On error result subtypes, `stop_reason` carries the value from the last assistant response before the loop ended. To detect refusals, check `stop_reason === "refusal"` (TypeScript) or `stop_reason == "refusal"` (Python). See [`SDKResultMessage`](https://code.claude.com/docs/en/agent-sdk/typescript#sdk-result-message) (TypeScript) or [`ResultMessage`](https://code.claude.com/docs/en/agent-sdk/python#result-message) (Python) for the full type.

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#hooks)  Hooks

[Hooks](https://code.claude.com/docs/en/agent-sdk/hooks) are callbacks that fire at specific points in the loop: before a tool runs, after it returns, when the agent finishes, and so on. Some commonly used hooks are:

| Hook | When it fires | Common uses |
| --- | --- | --- |
| `PreToolUse` | Before a tool executes | Validate inputs, block dangerous commands |
| `PostToolUse` | After a tool returns | Audit outputs, trigger side effects |
| `UserPromptSubmit` | When a prompt is sent | Inject additional context into prompts |
| `Stop` | When the agent finishes | Validate the result, save session state |
| `SubagentStart` / `SubagentStop` | When a subagent spawns or completes | Track and aggregate parallel task results |
| `PreCompact` | Before context compaction | Archive full transcript before summarizing |

Hooks run in your application process, not inside the agent’s context window, so they don’t consume context. Hooks can also short-circuit the loop: a `PreToolUse` hook that rejects a tool call prevents it from executing, and Claude receives the rejection message instead.Both SDKs support all the events above. The TypeScript SDK includes additional events that Python does not yet support. See [Control execution with hooks](https://code.claude.com/docs/en/agent-sdk/hooks) for the complete event list, per-SDK availability, and the full callback API.

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#put-it-all-together)  Put it all together

This example combines the key concepts from this page into a single agent that fixes failing tests. It configures the agent with allowed tools (auto-approved so the agent runs autonomously), project settings, and safety limits on turns and reasoning effort. As the loop runs, it captures the session ID for potential resumption, handles the final result, and prints the total cost.

Python

TypeScript

```
import asyncio
from claude_agent_sdk import query, ClaudeAgentOptions, ResultMessage

async def run_agent():
    session_id = None

    async for message in query(
        prompt="Find and fix the bug causing test failures in the auth module",
        options=ClaudeAgentOptions(
            allowed_tools=[\
                "Read",\
                "Edit",\
                "Bash",\
                "Glob",\
                "Grep",\
            ],  # Listing tools here auto-approves them (no prompting)
            setting_sources=[\
                "project"\
            ],  # Load CLAUDE.md, skills, hooks from current directory
            max_turns=30,  # Prevent runaway sessions
            effort="high",  # Thorough reasoning for complex debugging
        ),
    ):
        # Handle the final result
        if isinstance(message, ResultMessage):
            session_id = message.session_id  # Save for potential resumption

            if message.subtype == "success":
                print(f"Done: {message.result}")
            elif message.subtype == "error_max_turns":
                # Agent ran out of turns. Resume with a higher limit.
                print(f"Hit turn limit. Resume session {session_id} to continue.")
            elif message.subtype == "error_max_budget_usd":
                print("Hit budget limit.")
            else:
                print(f"Stopped: {message.subtype}")
            if message.total_cost_usd is not None:
                print(f"Cost: ${message.total_cost_usd:.4f}")

asyncio.run(run_agent())
```

## [​](https://code.claude.com/docs/en/agent-sdk/agent-loop\#next-steps)  Next steps

Now that you understand the loop, here’s where to go depending on what you’re building:

- **Haven’t run an agent yet?** Start with the [quickstart](https://code.claude.com/docs/en/agent-sdk/quickstart) to get the SDK installed and see a full example running end to end.
- **Ready to hook into your project?** [Load CLAUDE.md, skills, and filesystem hooks](https://code.claude.com/docs/en/agent-sdk/claude-code-features) so the agent follows your project conventions automatically.
- **Building an interactive UI?** Enable [streaming](https://code.claude.com/docs/en/agent-sdk/streaming-output) to show live text and tool calls as the loop runs.
- **Need tighter control over what the agent can do?** Lock down tool access with [permissions](https://code.claude.com/docs/en/agent-sdk/permissions), and use [hooks](https://code.claude.com/docs/en/agent-sdk/hooks) to audit, block, or transform tool calls before they execute.
- **Running long or expensive tasks?** Offload isolated work to [subagents](https://code.claude.com/docs/en/agent-sdk/subagents) to keep your main context lean.

For the broader conceptual picture of the agentic loop (not SDK-specific), see [How Claude Code works](https://code.claude.com/docs/en/how-claude-code-works).

Was this page helpful?

YesNo

[Quickstart](https://code.claude.com/docs/en/agent-sdk/quickstart) [Use Claude Code features](https://code.claude.com/docs/en/agent-sdk/claude-code-features)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.

![Agent loop: prompt enters, Claude evaluates, branches to tool calls or final answer](https://mintcdn.com/claude-code/gvy2DIUELtNA8qD3/images/agent-loop-diagram.svg?w=1100&fit=max&auto=format&n=gvy2DIUELtNA8qD3&q=85&s=9a364a7e1c9b7b35951258e312833487)