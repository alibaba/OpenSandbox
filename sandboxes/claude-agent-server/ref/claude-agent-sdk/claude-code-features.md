[Skip to main content](https://code.claude.com/docs/en/agent-sdk/claude-code-features#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Core concepts

Use Claude Code features in the SDK

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Control filesystem settings with settingSources](https://code.claude.com/docs/en/agent-sdk/claude-code-features#control-filesystem-settings-with-settingsources)
- [What settingSources does not control](https://code.claude.com/docs/en/agent-sdk/claude-code-features#what-settingsources-does-not-control)
- [Project instructions (CLAUDE.md and rules)](https://code.claude.com/docs/en/agent-sdk/claude-code-features#project-instructions-claude-md-and-rules)
- [CLAUDE.md load locations](https://code.claude.com/docs/en/agent-sdk/claude-code-features#claude-md-load-locations)
- [Skills](https://code.claude.com/docs/en/agent-sdk/claude-code-features#skills)
- [Hooks](https://code.claude.com/docs/en/agent-sdk/claude-code-features#hooks)
- [When to use which hook type](https://code.claude.com/docs/en/agent-sdk/claude-code-features#when-to-use-which-hook-type)
- [Choose the right feature](https://code.claude.com/docs/en/agent-sdk/claude-code-features#choose-the-right-feature)
- [Related resources](https://code.claude.com/docs/en/agent-sdk/claude-code-features#related-resources)

The Agent SDK is built on the same foundation as Claude Code, which means your SDK agents have access to the same filesystem-based features: project instructions (`CLAUDE.md` and rules), skills, hooks, and more.When you omit `settingSources`, `query()` reads the same filesystem settings as the Claude Code CLI: user, project, and local settings, CLAUDE.md files, and `.claude/` skills, agents, and commands. To run without these, pass `settingSources: []`, which limits the agent to what you configure programmatically. Managed policy settings and the global `~/.claude.json` config are read regardless of this option. See [What settingSources does not control](https://code.claude.com/docs/en/agent-sdk/claude-code-features#what-settingsources-does-not-control).For a conceptual overview of what each feature does and when to use it, see [Extend Claude Code](https://code.claude.com/docs/en/features-overview).

## [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#control-filesystem-settings-with-settingsources)  Control filesystem settings with settingSources

The setting sources option ( [`setting_sources`](https://code.claude.com/docs/en/agent-sdk/python#claude-agent-options) in Python, [`settingSources`](https://code.claude.com/docs/en/agent-sdk/typescript#setting-source) in TypeScript) controls which filesystem-based settings the SDK loads. Pass an explicit list to opt in to specific sources, or pass an empty array to disable user, project, and local settings.This example loads both user-level and project-level settings by setting `settingSources` to `["user", "project"]`:

Python

TypeScript

```
from claude_agent_sdk import query, ClaudeAgentOptions, AssistantMessage, ResultMessage

async for message in query(
    prompt="Help me refactor the auth module",
    options=ClaudeAgentOptions(
        # "user" loads from ~/.claude/, "project" loads from ./.claude/ in cwd.
        # Together they give the agent access to CLAUDE.md, skills, hooks, and
        # permissions from both locations.
        setting_sources=["user", "project"],
        allowed_tools=["Read", "Edit", "Bash"],
    ),
):
    if isinstance(message, AssistantMessage):
        for block in message.content:
            if hasattr(block, "text"):
                print(block.text)
    if isinstance(message, ResultMessage) and message.subtype == "success":
        print(f"\nResult: {message.result}")
```

Each source loads settings from a specific location, where `<cwd>` is the working directory you pass via the `cwd` option (or the process’s current directory if unset). For the full type definition, see [`SettingSource`](https://code.claude.com/docs/en/agent-sdk/typescript#setting-source) (TypeScript) or [`SettingSource`](https://code.claude.com/docs/en/agent-sdk/python#setting-source) (Python).

| Source | What it loads | Location |
| --- | --- | --- |
| `"project"` | Project CLAUDE.md, `.claude/rules/*.md`, project skills, project hooks, project `settings.json` | `<cwd>/.claude/` and each parent directory up to the filesystem root (stopping when a `.claude/` is found or no more parents exist) |
| `"user"` | User CLAUDE.md, `~/.claude/rules/*.md`, user skills, user settings | `~/.claude/` |
| `"local"` | CLAUDE.local.md (gitignored), `.claude/settings.local.json` | `<cwd>/` |

Omitting `settingSources` is equivalent to `["user", "project", "local"]`.The `cwd` option determines where the SDK looks for project settings. If neither `cwd` nor any of its parent directories contains a `.claude/` folder, project-level features won’t load.

### [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#what-settingsources-does-not-control)  What settingSources does not control

`settingSources` covers user, project, and local settings. A few inputs are read regardless of its value:

| Input | Behavior | To disable |
| --- | --- | --- |
| Managed policy settings | Always loaded when present on the host | Remove the managed settings file |
| `~/.claude.json` global config | Always read | Relocate with `CLAUDE_CONFIG_DIR` in `env` |
| Auto memory at `~/.claude/projects/<project>/memory/` | Loaded by default into the system prompt | Set `autoMemoryEnabled: false` in settings, or `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` in `env` |

Do not rely on default `query()` options for multi-tenant isolation. Because the inputs above are read regardless of `settingSources`, an SDK process can pick up host-level configuration and per-directory memory. For multi-tenant deployments, run each tenant in its own filesystem and set `settingSources: []` plus `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` in `env`. See [Secure deployment](https://code.claude.com/docs/en/agent-sdk/secure-deployment).

## [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#project-instructions-claude-md-and-rules)  Project instructions (CLAUDE.md and rules)

`CLAUDE.md` files and `.claude/rules/*.md` files give your agent persistent context about your project: coding conventions, build commands, architecture decisions, and instructions. When `settingSources` includes `"project"` (as in the example above), the SDK loads these files into context at session start. The agent then follows your project conventions without you repeating them in every prompt.

### [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#claude-md-load-locations)  CLAUDE.md load locations

| Level | Location | When loaded |
| --- | --- | --- |
| Project (root) | `<cwd>/CLAUDE.md` or `<cwd>/.claude/CLAUDE.md` | `settingSources` includes `"project"` |
| Project rules | `<cwd>/.claude/rules/*.md` | `settingSources` includes `"project"` |
| Project (parent dirs) | `CLAUDE.md` files in directories above `cwd` | `settingSources` includes `"project"`, loaded at session start |
| Project (child dirs) | `CLAUDE.md` files in subdirectories of `cwd` | `settingSources` includes `"project"`, loaded on demand when the agent reads a file in that subtree |
| Local (gitignored) | `<cwd>/CLAUDE.local.md` | `settingSources` includes `"local"` |
| User | `~/.claude/CLAUDE.md` | `settingSources` includes `"user"` |
| User rules | `~/.claude/rules/*.md` | `settingSources` includes `"user"` |

All levels are additive: if both project and user CLAUDE.md files exist, the agent sees both. There is no hard precedence rule between levels; if instructions conflict, the outcome depends on how Claude interprets them. Write non-conflicting rules, or state precedence explicitly in the more specific file (“These project instructions override any conflicting user-level defaults”).

You can also inject context directly via `systemPrompt` without using CLAUDE.md files. See [Modify system prompts](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts). Use CLAUDE.md when you want the same context shared between interactive Claude Code sessions and your SDK agents.

For how to structure and organize CLAUDE.md content, see [Manage Claude’s memory](https://code.claude.com/docs/en/memory).

## [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#skills)  Skills

Skills are markdown files that give your agent specialized knowledge and invocable workflows. Unlike `CLAUDE.md` (which loads every session), skills load on demand. The agent receives skill descriptions at startup and loads the full content when relevant.Skills are discovered from the filesystem through `settingSources`. With default options, user and project skills load automatically. The `Skill` tool is enabled by default when you don’t specify `allowedTools`. If you are using an `allowedTools` allowlist, include `"Skill"` explicitly.

Python

TypeScript

```
from claude_agent_sdk import query, ClaudeAgentOptions, ResultMessage

# Skills in .claude/skills/ are discovered automatically
# when settingSources includes "project"
async for message in query(
    prompt="Review this PR using our code review checklist",
    options=ClaudeAgentOptions(
        setting_sources=["user", "project"],
        allowed_tools=["Skill", "Read", "Grep", "Glob"],
    ),
):
    if isinstance(message, ResultMessage) and message.subtype == "success":
        print(message.result)
```

Skills must be created as filesystem artifacts (`.claude/skills/<name>/SKILL.md`). The SDK does not have a programmatic API for registering skills. See [Agent Skills in the SDK](https://code.claude.com/docs/en/agent-sdk/skills) for full details.

For more on creating and using skills, see [Agent Skills in the SDK](https://code.claude.com/docs/en/agent-sdk/skills).

## [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#hooks)  Hooks

The SDK supports two ways to define hooks, and they run side by side:

- **Filesystem hooks:** shell commands defined in `settings.json`, loaded when `settingSources` includes the relevant source. These are the same hooks you’d configure for [interactive Claude Code sessions](https://code.claude.com/docs/en/hooks-guide).
- **Programmatic hooks:** callback functions passed directly to `query()`. These run in your application process and can return structured decisions. See [Control execution with hooks](https://code.claude.com/docs/en/agent-sdk/hooks).

Both types execute during the same hook lifecycle. If you already have hooks in your project’s `.claude/settings.json` and you set `settingSources: ["project"]`, those hooks run automatically in the SDK with no extra configuration.Hook callbacks receive the tool input and return a decision dict. Returning `{}` (an empty dict) means allow the tool to proceed. Returning `{"decision": "block", "reason": "..."}` prevents execution and the reason is sent to Claude as the tool result. See the [hooks guide](https://code.claude.com/docs/en/agent-sdk/hooks) for the full callback signature and return types.

Python

TypeScript

```
from claude_agent_sdk import query, ClaudeAgentOptions, HookMatcher, ResultMessage

# PreToolUse hook callback. Positional args:
#   input_data: HookInput dict with tool_name, tool_input, hook_event_name
#   tool_use_id: str | None, the ID of the tool call being intercepted
#   context: HookContext, carries session metadata
async def audit_bash(input_data, tool_use_id, context):
    command = input_data.get("tool_input", {}).get("command", "")
    if "rm -rf" in command:
        return {"decision": "block", "reason": "Destructive command blocked"}
    return {}  # Empty dict: allow the tool to proceed

# Filesystem hooks from .claude/settings.json run automatically
# when settingSources loads them. You can also add programmatic hooks:
async for message in query(
    prompt="Refactor the auth module",
    options=ClaudeAgentOptions(
        setting_sources=["project"],  # Loads hooks from .claude/settings.json
        hooks={
            "PreToolUse": [\
                HookMatcher(matcher="Bash", hooks=[audit_bash]),\
            ]
        },
    ),
):
    if isinstance(message, ResultMessage) and message.subtype == "success":
        print(message.result)
```

### [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#when-to-use-which-hook-type)  When to use which hook type

| Hook type | Best for |
| --- | --- |
| **Filesystem** (`settings.json`) | Sharing hooks between CLI and SDK sessions. Supports `"command"` (shell scripts), `"http"` (POST to an endpoint), `"prompt"` (LLM evaluates a prompt), and `"agent"` (spawns a verifier agent). These fire in the main agent and any subagents it spawns. |
| **Programmatic** (callbacks in `query()`) | Application-specific logic; returning structured decisions; in-process integration. Scoped to the main session only. |

The TypeScript SDK supports additional hook events beyond Python, including `SessionStart`, `SessionEnd`, `TeammateIdle`, and `TaskCompleted`. See the [hooks guide](https://code.claude.com/docs/en/agent-sdk/hooks) for the full event compatibility table.

For full details on programmatic hooks, see [Control execution with hooks](https://code.claude.com/docs/en/agent-sdk/hooks). For filesystem hook syntax, see [Hooks](https://code.claude.com/docs/en/hooks).

## [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#choose-the-right-feature)  Choose the right feature

The Agent SDK gives you access to several ways to extend your agent’s behavior. If you’re unsure which to use, this table maps common goals to the right approach.

| You want to… | Use | SDK surface |
| --- | --- | --- |
| Set project conventions your agent always follows | [CLAUDE.md](https://code.claude.com/docs/en/memory) | `settingSources: ["project"]` loads it automatically |
| Give the agent reference material it loads when relevant | [Skills](https://code.claude.com/docs/en/agent-sdk/skills) | `settingSources` \+ `allowedTools: ["Skill"]` |
| Run a reusable workflow (deploy, review, release) | [User-invocable skills](https://code.claude.com/docs/en/agent-sdk/skills) | `settingSources` \+ `allowedTools: ["Skill"]` |
| Delegate an isolated subtask to a fresh context (research, review) | [Subagents](https://code.claude.com/docs/en/agent-sdk/subagents) | `agents` parameter + `allowedTools: ["Agent"]` |
| Coordinate multiple Claude Code instances with shared task lists and direct inter-agent messaging | [Agent teams](https://code.claude.com/docs/en/agent-teams) | Not directly configured via SDK options. Agent teams are a CLI feature where one session acts as the team lead, coordinating work across independent teammates |
| Run deterministic logic on tool calls (audit, block, transform) | [Hooks](https://code.claude.com/docs/en/agent-sdk/hooks) | `hooks` parameter with callbacks, or shell scripts loaded via `settingSources` |
| Give Claude structured tool access to an external service | [MCP](https://code.claude.com/docs/en/agent-sdk/mcp) | `mcpServers` parameter |

**Subagents versus agent teams:** Subagents are ephemeral and isolated: fresh conversation, one task, summary returned to parent. Agent teams coordinate multiple independent Claude Code instances that share a task list and message each other directly. Agent teams are a CLI feature. See [What subagents inherit](https://code.claude.com/docs/en/agent-sdk/subagents#what-subagents-inherit) and the [agent teams comparison](https://code.claude.com/docs/en/agent-teams#compare-with-subagents) for details.

Every feature you enable adds to your agent’s context window. For per-feature costs and how these features layer together, see [Extend Claude Code](https://code.claude.com/docs/en/features-overview#understand-context-costs).

## [​](https://code.claude.com/docs/en/agent-sdk/claude-code-features\#related-resources)  Related resources

- [Extend Claude Code](https://code.claude.com/docs/en/features-overview): Conceptual overview of all extension features, with comparison tables and context cost analysis
- [Skills in the SDK](https://code.claude.com/docs/en/agent-sdk/skills): Full guide to using skills programmatically
- [Subagents](https://code.claude.com/docs/en/agent-sdk/subagents): Define and invoke subagents for isolated subtasks
- [Hooks](https://code.claude.com/docs/en/agent-sdk/hooks): Intercept and control agent behavior at key execution points
- [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions): Control tool access with modes, rules, and callbacks
- [System prompts](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts): Inject context without CLAUDE.md files

Was this page helpful?

YesNo

[How the agent loop works](https://code.claude.com/docs/en/agent-sdk/agent-loop) [Work with sessions](https://code.claude.com/docs/en/agent-sdk/sessions)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.