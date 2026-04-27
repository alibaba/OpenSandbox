[Skip to main content](https://code.claude.com/docs/en/agent-sdk/permissions#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Control and observability

Configure permissions

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [How permissions are evaluated](https://code.claude.com/docs/en/agent-sdk/permissions#how-permissions-are-evaluated)
- [Allow and deny rules](https://code.claude.com/docs/en/agent-sdk/permissions#allow-and-deny-rules)
- [Permission modes](https://code.claude.com/docs/en/agent-sdk/permissions#permission-modes)
- [Available modes](https://code.claude.com/docs/en/agent-sdk/permissions#available-modes)
- [Set permission mode](https://code.claude.com/docs/en/agent-sdk/permissions#set-permission-mode)
- [Mode details](https://code.claude.com/docs/en/agent-sdk/permissions#mode-details)
- [Accept edits mode (acceptEdits)](https://code.claude.com/docs/en/agent-sdk/permissions#accept-edits-mode-acceptedits)
- [Don’t ask mode (dontAsk)](https://code.claude.com/docs/en/agent-sdk/permissions#don%E2%80%99t-ask-mode-dontask)
- [Bypass permissions mode (bypassPermissions)](https://code.claude.com/docs/en/agent-sdk/permissions#bypass-permissions-mode-bypasspermissions)
- [Plan mode (plan)](https://code.claude.com/docs/en/agent-sdk/permissions#plan-mode-plan)
- [Related resources](https://code.claude.com/docs/en/agent-sdk/permissions#related-resources)

The Claude Agent SDK provides permission controls to manage how Claude uses tools. Use permission modes and rules to define what’s allowed automatically, and the [`canUseTool` callback](https://code.claude.com/docs/en/agent-sdk/user-input) to handle everything else at runtime.

This page covers permission modes and rules. To build interactive approval flows where users approve or deny tool requests at runtime, see [Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input).

## [​](https://code.claude.com/docs/en/agent-sdk/permissions\#how-permissions-are-evaluated)  How permissions are evaluated

When Claude requests a tool, the SDK checks permissions in this order:

1

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/permissions#)

Hooks

Run [hooks](https://code.claude.com/docs/en/agent-sdk/hooks) first, which can allow, deny, or continue to the next step

2

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/permissions#)

Deny rules

Check `deny` rules (from `disallowed_tools` and [settings.json](https://code.claude.com/docs/en/settings#permission-settings)). If a deny rule matches, the tool is blocked, even in `bypassPermissions` mode.

3

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/permissions#)

Permission mode

Apply the active [permission mode](https://code.claude.com/docs/en/agent-sdk/permissions#permission-modes). `bypassPermissions` approves everything that reaches this step. `acceptEdits` approves file operations. Other modes fall through.

4

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/permissions#)

Allow rules

Check `allow` rules (from `allowed_tools` and settings.json). If a rule matches, the tool is approved.

5

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/permissions#)

canUseTool callback

If not resolved by any of the above, call your [`canUseTool` callback](https://code.claude.com/docs/en/agent-sdk/user-input) for a decision. In `dontAsk` mode, this step is skipped and the tool is denied.

![Permission evaluation flow diagram](https://mintcdn.com/claude-code/gvy2DIUELtNA8qD3/images/agent-sdk/permissions-flow.svg?fit=max&auto=format&n=gvy2DIUELtNA8qD3&q=85&s=0ccd63043a9ffc2a34d863602e043f72)This page focuses on **allow and deny rules** and **permission modes**. For the other steps:

- **Hooks:** run custom code to allow, deny, or modify tool requests. See [Control execution with hooks](https://code.claude.com/docs/en/agent-sdk/hooks).
- **canUseTool callback:** prompt users for approval at runtime. See [Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input).

## [​](https://code.claude.com/docs/en/agent-sdk/permissions\#allow-and-deny-rules)  Allow and deny rules

`allowed_tools` and `disallowed_tools` (TypeScript: `allowedTools` / `disallowedTools`) add entries to the allow and deny rule lists in the evaluation flow above. They control whether a tool call is approved, not whether the tool is available to Claude.

| Option | Effect |
| --- | --- |
| `allowed_tools=["Read", "Grep"]` | `Read` and `Grep` are auto-approved. Tools not listed here still exist and fall through to the permission mode and `canUseTool`. |
| `disallowed_tools=["Bash"]` | `Bash` is always denied. Deny rules are checked first and hold in every permission mode, including `bypassPermissions`. |

For a locked-down agent, pair `allowedTools` with `permissionMode: "dontAsk"`. Listed tools are approved; anything else is denied outright instead of prompting:

```
const options = {
  allowedTools: ["Read", "Glob", "Grep"],
  permissionMode: "dontAsk"
};
```

**`allowed_tools` does not constrain `bypassPermissions`.**`allowed_tools` only pre-approves the tools you list. Unlisted tools are not matched by any allow rule and fall through to the permission mode, where `bypassPermissions` approves them. Setting `allowed_tools=["Read"]` alongside `permission_mode="bypassPermissions"` still approves every tool, including `Bash`, `Write`, and `Edit`. If you need `bypassPermissions` but want specific tools blocked, use `disallowed_tools`.

You can also configure allow, deny, and ask rules declaratively in `.claude/settings.json`. These rules are read when the `project` setting source is enabled, which it is for default `query()` options. If you set `setting_sources` (TypeScript: `settingSources`) explicitly, include `"project"` for them to apply. See [Permission settings](https://code.claude.com/docs/en/settings#permission-settings) for the rule syntax.

## [​](https://code.claude.com/docs/en/agent-sdk/permissions\#permission-modes)  Permission modes

Permission modes provide global control over how Claude uses tools. You can set the permission mode when calling `query()` or change it dynamically during streaming sessions.

### [​](https://code.claude.com/docs/en/agent-sdk/permissions\#available-modes)  Available modes

The SDK supports these permission modes:

| Mode | Description | Tool behavior |
| --- | --- | --- |
| `default` | Standard permission behavior | No auto-approvals; unmatched tools trigger your `canUseTool` callback |
| `dontAsk` | Deny instead of prompting | Anything not pre-approved by `allowed_tools` or rules is denied; `canUseTool` is never called |
| `acceptEdits` | Auto-accept file edits | File edits and [filesystem operations](https://code.claude.com/docs/en/agent-sdk/permissions#accept-edits-mode-acceptedits) (`mkdir`, `rm`, `mv`, etc.) are automatically approved |
| `bypassPermissions` | Bypass all permission checks | All tools run without permission prompts (use with caution) |
| `plan` | Planning mode | No tool execution; Claude plans without making changes |
| `auto` (TypeScript only) | Model-classified approvals | A model classifier approves or denies each tool call. See [Auto mode](https://code.claude.com/docs/en/permission-modes#eliminate-prompts-with-auto-mode) for availability |

**Subagent inheritance:** When the parent uses `bypassPermissions`, `acceptEdits`, or `auto`, all subagents inherit that mode and it cannot be overridden per subagent. Subagents may have different system prompts and less constrained behavior than your main agent, so inheriting `bypassPermissions` grants them full, autonomous system access without any approval prompts.

### [​](https://code.claude.com/docs/en/agent-sdk/permissions\#set-permission-mode)  Set permission mode

You can set the permission mode once when starting a query, or change it dynamically while the session is active.

- At query time

- During streaming


Pass `permission_mode` (Python) or `permissionMode` (TypeScript) when creating a query. This mode applies for the entire session unless changed dynamically.

Python

TypeScript

```
import asyncio
from claude_agent_sdk import query, ClaudeAgentOptions

async def main():
    async for message in query(
        prompt="Help me refactor this code",
        options=ClaudeAgentOptions(
            permission_mode="default",  # Set the mode here
        ),
    ):
        if hasattr(message, "result"):
            print(message.result)

asyncio.run(main())
```

Call `set_permission_mode()` (Python) or `setPermissionMode()` (TypeScript) to change the mode mid-session. The new mode takes effect immediately for all subsequent tool requests. This lets you start restrictive and loosen permissions as trust builds, for example switching to `acceptEdits` after reviewing Claude’s initial approach.

Python

TypeScript

```
import asyncio
from claude_agent_sdk import query, ClaudeAgentOptions

async def main():
    q = query(
        prompt="Help me refactor this code",
        options=ClaudeAgentOptions(
            permission_mode="default",  # Start in default mode
        ),
    )

    # Change mode dynamically mid-session
    await q.set_permission_mode("acceptEdits")

    # Process messages with the new permission mode
    async for message in q:
        if hasattr(message, "result"):
            print(message.result)

asyncio.run(main())
```

### [​](https://code.claude.com/docs/en/agent-sdk/permissions\#mode-details)  Mode details

#### [​](https://code.claude.com/docs/en/agent-sdk/permissions\#accept-edits-mode-acceptedits)  Accept edits mode (`acceptEdits`)

Auto-approves file operations so Claude can edit code without prompting. Other tools (like Bash commands that aren’t filesystem operations) still require normal permissions.**Auto-approved operations:**

- File edits (Edit, Write tools)
- Filesystem commands: `mkdir`, `touch`, `rm`, `rmdir`, `mv`, `cp`, `sed`

Both apply only to paths inside the working directory or `additionalDirectories`. Paths outside that scope and writes to protected paths still prompt.**Use when:** you trust Claude’s edits and want faster iteration, such as during prototyping or when working in an isolated directory.

#### [​](https://code.claude.com/docs/en/agent-sdk/permissions\#don%E2%80%99t-ask-mode-dontask)  Don’t ask mode (`dontAsk`)

Converts any permission prompt into a denial. Tools pre-approved by `allowed_tools`, `settings.json` allow rules, or a hook run as normal. Everything else is denied without calling `canUseTool`.**Use when:** you want a fixed, explicit tool surface for a headless agent and prefer a hard deny over silent reliance on `canUseTool` being absent.

#### [​](https://code.claude.com/docs/en/agent-sdk/permissions\#bypass-permissions-mode-bypasspermissions)  Bypass permissions mode (`bypassPermissions`)

Auto-approves all tool uses without prompts. Hooks still execute and can block operations if needed.

Use with extreme caution. Claude has full system access in this mode. Only use in controlled environments where you trust all possible operations.`allowed_tools` does not constrain this mode. Every tool is approved, not just the ones you listed. Deny rules (`disallowed_tools`), explicit `ask` rules, and hooks are evaluated before the mode check and can still block a tool.

#### [​](https://code.claude.com/docs/en/agent-sdk/permissions\#plan-mode-plan)  Plan mode (`plan`)

Prevents tool execution entirely. Claude can analyze code and create plans but cannot make changes. Claude may use `AskUserQuestion` to clarify requirements before finalizing the plan. See [Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input#handle-clarifying-questions) for handling these prompts.**Use when:** you want Claude to propose changes without executing them, such as during code review or when you need to approve changes before they’re made.

## [​](https://code.claude.com/docs/en/agent-sdk/permissions\#related-resources)  Related resources

For the other steps in the permission evaluation flow:

- [Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input): interactive approval prompts and clarifying questions
- [Hooks guide](https://code.claude.com/docs/en/agent-sdk/hooks): run custom code at key points in the agent lifecycle
- [Permission rules](https://code.claude.com/docs/en/settings#permission-settings): declarative allow/deny rules in `settings.json`

Was this page helpful?

YesNo

[Plugins in the SDK](https://code.claude.com/docs/en/agent-sdk/plugins) [Intercept and control agent behavior with hooks](https://code.claude.com/docs/en/agent-sdk/hooks)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.

![Permission evaluation flow diagram](https://mintcdn.com/claude-code/gvy2DIUELtNA8qD3/images/agent-sdk/permissions-flow.svg?w=1100&fit=max&auto=format&n=gvy2DIUELtNA8qD3&q=85&s=c18408ea483175a1f8856baf1cd9c7d4)