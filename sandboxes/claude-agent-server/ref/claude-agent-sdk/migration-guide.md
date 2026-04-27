[Skip to main content](https://code.claude.com/docs/en/agent-sdk/migration-guide#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

SDK references

Migrate to Claude Agent SDK

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Overview](https://code.claude.com/docs/en/agent-sdk/migration-guide#overview)
- [What’s Changed](https://code.claude.com/docs/en/agent-sdk/migration-guide#what%E2%80%99s-changed)
- [Migration Steps](https://code.claude.com/docs/en/agent-sdk/migration-guide#migration-steps)
- [For TypeScript/JavaScript Projects](https://code.claude.com/docs/en/agent-sdk/migration-guide#for-typescript%2Fjavascript-projects)
- [For Python Projects](https://code.claude.com/docs/en/agent-sdk/migration-guide#for-python-projects)
- [Breaking changes](https://code.claude.com/docs/en/agent-sdk/migration-guide#breaking-changes)
- [Python: ClaudeCodeOptions renamed to ClaudeAgentOptions](https://code.claude.com/docs/en/agent-sdk/migration-guide#python-claudecodeoptions-renamed-to-claudeagentoptions)
- [System prompt no longer default](https://code.claude.com/docs/en/agent-sdk/migration-guide#system-prompt-no-longer-default)
- [Settings Sources No Longer Loaded by Default](https://code.claude.com/docs/en/agent-sdk/migration-guide#settings-sources-no-longer-loaded-by-default)
- [Why the Rename?](https://code.claude.com/docs/en/agent-sdk/migration-guide#why-the-rename)
- [Getting Help](https://code.claude.com/docs/en/agent-sdk/migration-guide#getting-help)
- [Next Steps](https://code.claude.com/docs/en/agent-sdk/migration-guide#next-steps)

## [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#overview)  Overview

The Claude Code SDK has been renamed to the **Claude Agent SDK** and its documentation has been reorganized. This change reflects the SDK’s broader capabilities for building AI agents beyond just coding tasks.

## [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#what%E2%80%99s-changed)  What’s Changed

| Aspect | Old | New |
| --- | --- | --- |
| **Package Name (TS/JS)** | `@anthropic-ai/claude-code` | `@anthropic-ai/claude-agent-sdk` |
| **Python Package** | `claude-code-sdk` | `claude-agent-sdk` |
| **Documentation Location** | Claude Code docs | API Guide → Agent SDK section |

**Documentation Changes:** The Agent SDK documentation has moved from the Claude Code docs to the API Guide under a dedicated [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) section. The Claude Code docs now focus on the CLI tool and automation features.

## [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#migration-steps)  Migration Steps

### [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#for-typescript/javascript-projects)  For TypeScript/JavaScript Projects

**1\. Uninstall the old package:**

```
npm uninstall @anthropic-ai/claude-code
```

**2\. Install the new package:**

```
npm install @anthropic-ai/claude-agent-sdk
```

**3\. Update your imports:**Change all imports from `@anthropic-ai/claude-code` to `@anthropic-ai/claude-agent-sdk`:

```
// Before
import { query, tool, createSdkMcpServer } from "@anthropic-ai/claude-code";

// After
import { query, tool, createSdkMcpServer } from "@anthropic-ai/claude-agent-sdk";
```

**4\. Update package.json dependencies:**If you have the package listed in your `package.json`, update it:Before:

```
{
  "dependencies": {
    "@anthropic-ai/claude-code": "^0.0.42"
  }
}
```

After:

```
{
  "dependencies": {
    "@anthropic-ai/claude-agent-sdk": "^0.2.0"
  }
}
```

That’s it! No other code changes are required.

### [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#for-python-projects)  For Python Projects

**1\. Uninstall the old package:**

```
pip uninstall claude-code-sdk
```

**2\. Install the new package:**

```
pip install claude-agent-sdk
```

**3\. Update your imports:**Change all imports from `claude_code_sdk` to `claude_agent_sdk`:

```
# Before
from claude_code_sdk import query, ClaudeCodeOptions

# After
from claude_agent_sdk import query, ClaudeAgentOptions
```

**4\. Update type names:**Change `ClaudeCodeOptions` to `ClaudeAgentOptions`:

```
# Before
from claude_code_sdk import query, ClaudeCodeOptions

options = ClaudeCodeOptions(model="claude-opus-4-7")

# After
from claude_agent_sdk import query, ClaudeAgentOptions

options = ClaudeAgentOptions(model="claude-opus-4-7")
```

**5\. Review [breaking changes](https://code.claude.com/docs/en/agent-sdk/migration-guide#breaking-changes)**Make any code changes needed to complete the migration.

## [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#breaking-changes)  Breaking changes

To improve isolation and explicit configuration, Claude Agent SDK v0.1.0 introduces breaking changes for users migrating from Claude Code SDK. Review this section carefully before migrating.

### [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#python-claudecodeoptions-renamed-to-claudeagentoptions)  Python: ClaudeCodeOptions renamed to ClaudeAgentOptions

**What changed:** The Python SDK type `ClaudeCodeOptions` has been renamed to `ClaudeAgentOptions`.**Migration:**

```
# BEFORE (claude-code-sdk)
from claude_code_sdk import query, ClaudeCodeOptions

options = ClaudeCodeOptions(model="claude-opus-4-7", permission_mode="acceptEdits")

# AFTER (claude-agent-sdk)
from claude_agent_sdk import query, ClaudeAgentOptions

options = ClaudeAgentOptions(model="claude-opus-4-7", permission_mode="acceptEdits")
```

**Why this changed:** The type name now matches the “Claude Agent SDK” branding and provides consistency across the SDK’s naming conventions.

### [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#system-prompt-no-longer-default)  System prompt no longer default

**What changed:** The SDK no longer uses Claude Code’s system prompt by default.**Migration:**

TypeScript

Python

```
// BEFORE (v0.0.x) - Used Claude Code's system prompt by default
const result = query({ prompt: "Hello" });

// AFTER (v0.1.0) - Uses minimal system prompt by default
// To get the old behavior, explicitly request Claude Code's preset:
const result = query({
  prompt: "Hello",
  options: {
    systemPrompt: { type: "preset", preset: "claude_code" }
  }
});

// Or use a custom system prompt:
const result = query({
  prompt: "Hello",
  options: {
    systemPrompt: "You are a helpful coding assistant"
  }
});
```

**Why this changed:** Provides better control and isolation for SDK applications. You can now build agents with custom behavior without inheriting Claude Code’s CLI-focused instructions.

### [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#settings-sources-no-longer-loaded-by-default)  Settings Sources No Longer Loaded by Default

**What changed:** The SDK no longer reads from filesystem settings (CLAUDE.md, settings.json, slash commands, etc.) by default.**Migration:**

TypeScript

Python

```
// BEFORE (v0.0.x) - Loaded all settings automatically
const result = query({ prompt: "Hello" });
// Would read from:
// - ~/.claude/settings.json (user)
// - .claude/settings.json (project)
// - .claude/settings.local.json (local)
// - CLAUDE.md files
// - Custom slash commands

// AFTER (v0.1.0) - No settings loaded by default
// To get the old behavior:
const result = query({
  prompt: "Hello",
  options: {
    settingSources: ["user", "project", "local"]
  }
});

// Or load only specific sources:
const result = query({
  prompt: "Hello",
  options: {
    settingSources: ["project"] // Only project settings
  }
});
```

**Why this changed:** Ensures SDK applications have predictable behavior independent of local filesystem configurations. This is especially important for:

- **CI/CD environments** \- Consistent behavior without local customizations
- **Deployed applications** \- No dependency on filesystem settings
- **Testing** \- Isolated test environments
- **Multi-tenant systems** \- Prevent settings leakage between users

Current SDK releases have reverted this default for `query()`: omitting the option once again loads user, project, and local settings, matching the CLI. Pass `settingSources: []` in TypeScript or `setting_sources=[]` in Python if your application depends on the isolated behavior described above. Python SDK 0.1.59 and earlier treated an empty list the same as omitting the option, so upgrade before relying on `setting_sources=[]`. See [What settingSources does not control](https://code.claude.com/docs/en/agent-sdk/claude-code-features#what-settingsources-does-not-control) for inputs that are read even when `settingSources` is `[]`.

## [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#why-the-rename)  Why the Rename?

The Claude Code SDK was originally designed for coding tasks, but it has evolved into a powerful framework for building all types of AI agents. The new name “Claude Agent SDK” better reflects its capabilities:

- Building business agents (legal assistants, finance advisors, customer support)
- Creating specialized coding agents (SRE bots, security reviewers, code review agents)
- Developing custom agents for any domain with tool use, MCP integration, and more

## [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#getting-help)  Getting Help

If you encounter any issues during migration:**For TypeScript/JavaScript:**

1. Check that all imports are updated to use `@anthropic-ai/claude-agent-sdk`
2. Verify your package.json has the new package name
3. Run `npm install` to ensure dependencies are updated

**For Python:**

1. Check that all imports are updated to use `claude_agent_sdk`
2. Verify your requirements.txt or pyproject.toml has the new package name
3. Run `pip install claude-agent-sdk` to ensure the package is installed

## [​](https://code.claude.com/docs/en/agent-sdk/migration-guide\#next-steps)  Next Steps

- Explore the [Agent SDK Overview](https://code.claude.com/docs/en/agent-sdk/overview) to learn about available features
- Check out the [TypeScript SDK Reference](https://code.claude.com/docs/en/agent-sdk/typescript) for detailed API documentation
- Review the [Python SDK Reference](https://code.claude.com/docs/en/agent-sdk/python) for Python-specific documentation
- Learn about [Custom Tools](https://code.claude.com/docs/en/agent-sdk/custom-tools) and [MCP Integration](https://code.claude.com/docs/en/agent-sdk/mcp)

Was this page helpful?

YesNo

[Python SDK](https://code.claude.com/docs/en/agent-sdk/python)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.