[Skip to main content](https://code.claude.com/docs/en/agent-sdk/slash-commands#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Customize behavior

Slash Commands in the SDK

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Discovering Available Slash Commands](https://code.claude.com/docs/en/agent-sdk/slash-commands#discovering-available-slash-commands)
- [Sending Slash Commands](https://code.claude.com/docs/en/agent-sdk/slash-commands#sending-slash-commands)
- [Common Slash Commands](https://code.claude.com/docs/en/agent-sdk/slash-commands#common-slash-commands)
- [/compact - Compact Conversation History](https://code.claude.com/docs/en/agent-sdk/slash-commands#%2Fcompact-compact-conversation-history)
- [Clearing the conversation](https://code.claude.com/docs/en/agent-sdk/slash-commands#clearing-the-conversation)
- [Creating Custom Slash Commands](https://code.claude.com/docs/en/agent-sdk/slash-commands#creating-custom-slash-commands)
- [File Locations](https://code.claude.com/docs/en/agent-sdk/slash-commands#file-locations)
- [File Format](https://code.claude.com/docs/en/agent-sdk/slash-commands#file-format)
- [Basic Example](https://code.claude.com/docs/en/agent-sdk/slash-commands#basic-example)
- [With Frontmatter](https://code.claude.com/docs/en/agent-sdk/slash-commands#with-frontmatter)
- [Using Custom Commands in the SDK](https://code.claude.com/docs/en/agent-sdk/slash-commands#using-custom-commands-in-the-sdk)
- [Advanced Features](https://code.claude.com/docs/en/agent-sdk/slash-commands#advanced-features)
- [Arguments and Placeholders](https://code.claude.com/docs/en/agent-sdk/slash-commands#arguments-and-placeholders)
- [Bash Command Execution](https://code.claude.com/docs/en/agent-sdk/slash-commands#bash-command-execution)
- [File References](https://code.claude.com/docs/en/agent-sdk/slash-commands#file-references)
- [Organization with Namespacing](https://code.claude.com/docs/en/agent-sdk/slash-commands#organization-with-namespacing)
- [Practical Examples](https://code.claude.com/docs/en/agent-sdk/slash-commands#practical-examples)
- [Code Review Command](https://code.claude.com/docs/en/agent-sdk/slash-commands#code-review-command)
- [Test Runner Command](https://code.claude.com/docs/en/agent-sdk/slash-commands#test-runner-command)
- [See Also](https://code.claude.com/docs/en/agent-sdk/slash-commands#see-also)

Slash commands provide a way to control Claude Code sessions with special commands that start with `/`. These commands can be sent through the SDK to perform actions like compacting context, listing context usage, or invoking custom commands. Only commands that work without an interactive terminal are dispatchable through the SDK; the `system/init` message lists the ones available in your session.

## [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#discovering-available-slash-commands)  Discovering Available Slash Commands

The Claude Agent SDK provides information about available slash commands in the system initialization message. Access this information when your session starts:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

for await (const message of query({
  prompt: "Hello Claude",
  options: { maxTurns: 1 }
})) {
  if (message.type === "system" && message.subtype === "init") {
    console.log("Available slash commands:", message.slash_commands);
    // Example output: ["/compact", "/context", "/cost"]
  }
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#sending-slash-commands)  Sending Slash Commands

Send slash commands by including them in your prompt string, just like regular text:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

// Send a slash command
for await (const message of query({
  prompt: "/compact",
  options: { maxTurns: 1 }
})) {
  if (message.type === "result") {
    console.log("Command executed:", message.result);
  }
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#common-slash-commands)  Common Slash Commands

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#/compact-compact-conversation-history)  `/compact` \- Compact Conversation History

The `/compact` command reduces the size of your conversation history by summarizing older messages while preserving important context:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

for await (const message of query({
  prompt: "/compact",
  options: { maxTurns: 1 }
})) {
  if (message.type === "system" && message.subtype === "compact_boundary") {
    console.log("Compaction completed");
    console.log("Pre-compaction tokens:", message.compact_metadata.pre_tokens);
    console.log("Trigger:", message.compact_metadata.trigger);
  }
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#clearing-the-conversation)  Clearing the conversation

The interactive `/clear` command is not available in the SDK. Each `query()` call already starts a fresh conversation, so to clear context, end the current `query()` and start a new one. The previous conversation stays on disk and can be returned to by passing its session ID to the [`resume` option](https://code.claude.com/docs/en/agent-sdk/sessions#resume-by-id).

## [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#creating-custom-slash-commands)  Creating Custom Slash Commands

In addition to using built-in slash commands, you can create your own custom commands that are available through the SDK. Custom commands are defined as markdown files in specific directories, similar to how subagents are configured.

The `.claude/commands/` directory is the legacy format. The recommended format is `.claude/skills/<name>/SKILL.md`, which supports the same slash-command invocation (`/name`) plus autonomous invocation by Claude. See [Skills](https://code.claude.com/docs/en/agent-sdk/skills) for the current format. The CLI continues to support both formats, and the examples below remain accurate for `.claude/commands/`.

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#file-locations)  File Locations

Custom slash commands are stored in designated directories based on their scope:

- **Project commands**: `.claude/commands/` \- Available only in the current project (legacy; prefer `.claude/skills/`)
- **Personal commands**: `~/.claude/commands/` \- Available across all your projects (legacy; prefer `~/.claude/skills/`)

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#file-format)  File Format

Each custom command is a markdown file where:

- The filename (without `.md` extension) becomes the command name
- The file content defines what the command does
- Optional YAML frontmatter provides configuration

#### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#basic-example)  Basic Example

Create `.claude/commands/refactor.md`:

```
Refactor the selected code to improve readability and maintainability.
Focus on clean code principles and best practices.
```

This creates the `/refactor` command that you can use through the SDK.

#### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#with-frontmatter)  With Frontmatter

Create `.claude/commands/security-check.md`:

```
---
allowed-tools: Read, Grep, Glob
description: Run security vulnerability scan
model: claude-opus-4-7
---

Analyze the codebase for security vulnerabilities including:
- SQL injection risks
- XSS vulnerabilities
- Exposed credentials
- Insecure configurations
```

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#using-custom-commands-in-the-sdk)  Using Custom Commands in the SDK

Once defined in the filesystem, custom commands are automatically available through the SDK:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

// Use a custom command
for await (const message of query({
  prompt: "/refactor src/auth/login.ts",
  options: { maxTurns: 3 }
})) {
  if (message.type === "assistant") {
    console.log("Refactoring suggestions:", message.message);
  }
}

// Custom commands appear in the slash_commands list
for await (const message of query({
  prompt: "Hello",
  options: { maxTurns: 1 }
})) {
  if (message.type === "system" && message.subtype === "init") {
    // Will include both built-in and custom commands
    console.log("Available commands:", message.slash_commands);
    // Example: ["/compact", "/context", "/cost", "/refactor", "/security-check"]
  }
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#advanced-features)  Advanced Features

#### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#arguments-and-placeholders)  Arguments and Placeholders

Custom commands support dynamic arguments using placeholders:Create `.claude/commands/fix-issue.md`:

```
---
argument-hint: [issue-number] [priority]
description: Fix a GitHub issue
---

Fix issue #$1 with priority $2.
Check the issue description and implement the necessary changes.
```

Use in SDK:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

// Pass arguments to custom command
for await (const message of query({
  prompt: "/fix-issue 123 high",
  options: { maxTurns: 5 }
})) {
  // Command will process with $1="123" and $2="high"
  if (message.type === "result") {
    console.log("Issue fixed:", message.result);
  }
}
```

#### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#bash-command-execution)  Bash Command Execution

Custom commands can execute bash commands and include their output:Create `.claude/commands/git-commit.md`:

```
---
allowed-tools: Bash(git add *), Bash(git status *), Bash(git commit *)
description: Create a git commit
---

## Context

- Current status: !`git status`
- Current diff: !`git diff HEAD`

## Task

Create a git commit with appropriate message based on the changes.
```

#### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#file-references)  File References

Include file contents using the `@` prefix:Create `.claude/commands/review-config.md`:

```
---
description: Review configuration files
---

Review the following configuration files for issues:
- Package config: @package.json
- TypeScript config: @tsconfig.json
- Environment config: @.env

Check for security issues, outdated dependencies, and misconfigurations.
```

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#organization-with-namespacing)  Organization with Namespacing

Organize commands in subdirectories for better structure:

```
.claude/commands/
├── frontend/
│   ├── component.md      # Creates /component (project:frontend)
│   └── style-check.md     # Creates /style-check (project:frontend)
├── backend/
│   ├── api-test.md        # Creates /api-test (project:backend)
│   └── db-migrate.md      # Creates /db-migrate (project:backend)
└── review.md              # Creates /review (project)
```

The subdirectory appears in the command description but doesn’t affect the command name itself.

### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#practical-examples)  Practical Examples

#### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#code-review-command)  Code Review Command

Create `.claude/commands/code-review.md`:

```
---
allowed-tools: Read, Grep, Glob, Bash(git diff *)
description: Comprehensive code review
---

## Changed Files
!`git diff --name-only HEAD~1`

## Detailed Changes
!`git diff HEAD~1`

## Review Checklist

Review the above changes for:
1. Code quality and readability
2. Security vulnerabilities
3. Performance implications
4. Test coverage
5. Documentation completeness

Provide specific, actionable feedback organized by priority.
```

#### [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#test-runner-command)  Test Runner Command

Create `.claude/commands/test.md`:

```
---
allowed-tools: Bash, Read, Edit
argument-hint: [test-pattern]
description: Run tests with optional pattern
---

Run tests matching pattern: $ARGUMENTS

1. Detect the test framework (Jest, pytest, etc.)
2. Run tests with the provided pattern
3. If tests fail, analyze and fix them
4. Re-run to verify fixes
```

Use these commands through the SDK:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

// Run code review
for await (const message of query({
  prompt: "/code-review",
  options: { maxTurns: 3 }
})) {
  // Process review feedback
}

// Run specific tests
for await (const message of query({
  prompt: "/test auth",
  options: { maxTurns: 5 }
})) {
  // Handle test results
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/slash-commands\#see-also)  See Also

- [Slash Commands](https://code.claude.com/docs/en/skills) \- Complete slash command documentation
- [Subagents in the SDK](https://code.claude.com/docs/en/agent-sdk/subagents) \- Similar filesystem-based configuration for subagents
- [TypeScript SDK reference](https://code.claude.com/docs/en/agent-sdk/typescript) \- Complete API documentation
- [SDK overview](https://code.claude.com/docs/en/agent-sdk/overview) \- General SDK concepts
- [CLI reference](https://code.claude.com/docs/en/cli-reference) \- Command-line interface

Was this page helpful?

YesNo

[Modifying system prompts](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts) [Agent Skills in the SDK](https://code.claude.com/docs/en/agent-sdk/skills)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.