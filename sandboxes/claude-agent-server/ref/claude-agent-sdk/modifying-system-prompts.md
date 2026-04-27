[Skip to main content](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Customize behavior

Modifying system prompts

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Understanding system prompts](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#understanding-system-prompts)
- [Methods of modification](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#methods-of-modification)
- [Method 1: CLAUDE.md files (project-level instructions)](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#method-1-claude-md-files-project-level-instructions)
- [How CLAUDE.md works with the SDK](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#how-claude-md-works-with-the-sdk)
- [Example CLAUDE.md](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#example-claude-md)
- [Using CLAUDE.md with the SDK](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#using-claude-md-with-the-sdk)
- [When to use CLAUDE.md](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#when-to-use-claude-md)
- [Method 2: Output styles (persistent configurations)](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#method-2-output-styles-persistent-configurations)
- [Creating an output style](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#creating-an-output-style)
- [Using output styles](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#using-output-styles)
- [Method 3: Using systemPrompt with append](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#method-3-using-systemprompt-with-append)
- [Improve prompt caching across users and machines](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#improve-prompt-caching-across-users-and-machines)
- [Method 4: Custom system prompts](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#method-4-custom-system-prompts)
- [Comparison of all four approaches](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#comparison-of-all-four-approaches)
- [Use cases and best practices](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#use-cases-and-best-practices)
- [When to use CLAUDE.md](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#when-to-use-claude-md-2)
- [When to use output styles](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#when-to-use-output-styles)
- [When to use systemPrompt with append](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#when-to-use-systemprompt-with-append)
- [When to use custom systemPrompt](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#when-to-use-custom-systemprompt)
- [Combining approaches](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#combining-approaches)
- [Example: Output style with session-specific additions](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#example-output-style-with-session-specific-additions)
- [See also](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#see-also)

System prompts define Claude’s behavior, capabilities, and response style. The Claude Agent SDK provides three ways to customize system prompts: using output styles (persistent, file-based configurations), appending to Claude Code’s prompt, or using a fully custom prompt.

## [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#understanding-system-prompts)  Understanding system prompts

A system prompt is the initial instruction set that shapes how Claude behaves throughout a conversation.

**Default behavior:** The Agent SDK uses a **minimal system prompt** by default. It contains only essential tool instructions but omits Claude Code’s coding guidelines, response style, and project context. To include the full Claude Code system prompt, specify `systemPrompt: { type: "preset", preset: "claude_code" }` in TypeScript or `system_prompt={"type": "preset", "preset": "claude_code"}` in Python.

Claude Code’s system prompt includes:

- Tool usage instructions and available tools
- Code style and formatting guidelines
- Response tone and verbosity settings
- Security and safety instructions
- Context about the current working directory and environment

## [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#methods-of-modification)  Methods of modification

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#method-1-claude-md-files-project-level-instructions)  Method 1: CLAUDE.md files (project-level instructions)

CLAUDE.md files provide project-specific context and instructions that are automatically read by the Agent SDK when it runs in a directory. They serve as persistent “memory” for your project.

#### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#how-claude-md-works-with-the-sdk)  How CLAUDE.md works with the SDK

**Location and discovery:**

- **Project-level:**`CLAUDE.md` or `.claude/CLAUDE.md` in your working directory
- **User-level:**`~/.claude/CLAUDE.md` for global instructions across all projects

CLAUDE.md files are read when the corresponding setting source is enabled: `'project'` for project-level CLAUDE.md and `'user'` for `~/.claude/CLAUDE.md`. With default `query()` options both sources are enabled, so CLAUDE.md loads automatically. If you set `settingSources` (TypeScript) or `setting_sources` (Python) explicitly, include the sources you need. CLAUDE.md loading is controlled by setting sources, not by the `claude_code` preset.**Content format:**
CLAUDE.md files use plain markdown and can contain:

- Coding guidelines and standards
- Project-specific context
- Common commands or workflows
- API conventions
- Testing requirements

#### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#example-claude-md)  Example CLAUDE.md

```
# Project Guidelines

## Code Style

- Use TypeScript strict mode
- Prefer functional components in React
- Always include JSDoc comments for public APIs

## Testing

- Run `npm test` before committing
- Maintain >80% code coverage
- Use jest for unit tests, playwright for E2E

## Commands

- Build: `npm run build`
- Dev server: `npm run dev`
- Type check: `npm run typecheck`
```

#### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#using-claude-md-with-the-sdk)  Using CLAUDE.md with the SDK

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

const messages = [];

for await (const message of query({
  prompt: "Add a new React component for user profiles",
  options: {
    systemPrompt: {
      type: "preset",
      preset: "claude_code" // Use Claude Code's system prompt
    },
    settingSources: ["project"] // Loads CLAUDE.md from project
  }
})) {
  messages.push(message);
}

// Now Claude has access to your project guidelines from CLAUDE.md
```

#### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#when-to-use-claude-md)  When to use CLAUDE.md

**Best for:**

- **Team-shared context** \- Guidelines everyone should follow
- **Project conventions** \- Coding standards, file structure, naming patterns
- **Common commands** \- Build, test, deploy commands specific to your project
- **Long-term memory** \- Context that should persist across all sessions
- **Version-controlled instructions** \- Commit to git so the team stays in sync

**Key characteristics:**

- ✅ Persistent across all sessions in a project
- ✅ Shared with team via git
- ✅ Automatic discovery (no code changes needed)
- ⚠️ Not loaded if you pass `settingSources: []`

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#method-2-output-styles-persistent-configurations)  Method 2: Output styles (persistent configurations)

Output styles are saved configurations that modify Claude’s system prompt. They’re stored as markdown files and can be reused across sessions and projects.

#### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#creating-an-output-style)  Creating an output style

TypeScript

Python

```
import { writeFile, mkdir } from "fs/promises";
import { join } from "path";
import { homedir } from "os";

async function createOutputStyle(name: string, description: string, prompt: string) {
  // User-level: ~/.claude/output-styles
  // Project-level: .claude/output-styles
  const outputStylesDir = join(homedir(), ".claude", "output-styles");

  await mkdir(outputStylesDir, { recursive: true });

  const content = `---
name: ${name}
description: ${description}
---

${prompt}`;

  const filePath = join(outputStylesDir, `${name.toLowerCase().replace(/\s+/g, "-")}.md`);
  await writeFile(filePath, content, "utf-8");
}

// Example: Create a code review specialist
await createOutputStyle(
  "Code Reviewer",
  "Thorough code review assistant",
  `You are an expert code reviewer.

For every code submission:
1. Check for bugs and security issues
2. Evaluate performance
3. Suggest improvements
4. Rate code quality (1-10)`
);
```

#### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#using-output-styles)  Using output styles

Once created, activate output styles via:

- **CLI**: `/output-style [style-name]`
- **Settings**: `.claude/settings.local.json`
- **Create new**: `/output-style:new [description]`

**Note for SDK users:** Output styles are loaded when you include `settingSources: ['user']` or `settingSources: ['project']` (TypeScript) / `setting_sources=["user"]` or `setting_sources=["project"]` (Python) in your options.

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#method-3-using-systemprompt-with-append)  Method 3: Using `systemPrompt` with append

You can use the Claude Code preset with an `append` property to add your custom instructions while preserving all built-in functionality.

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

const messages = [];

for await (const message of query({
  prompt: "Help me write a Python function to calculate fibonacci numbers",
  options: {
    systemPrompt: {
      type: "preset",
      preset: "claude_code",
      append: "Always include detailed docstrings and type hints in Python code."
    }
  }
})) {
  messages.push(message);
  if (message.type === "assistant") {
    console.log(message.message.content);
  }
}
```

#### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#improve-prompt-caching-across-users-and-machines)  Improve prompt caching across users and machines

By default, two sessions that use the same `claude_code` preset and `append` text still cannot share a prompt cache entry if they run from different working directories. This is because the preset embeds per-session context in the system prompt ahead of your `append` text: the working directory, platform and OS version, current date, git status, and auto-memory paths. Any difference in that context produces a different system prompt and a cache miss.To make the system prompt identical across sessions, set `excludeDynamicSections: true` in TypeScript or `"exclude_dynamic_sections": True` in Python. The per-session context moves into the first user message, leaving only the static preset and your `append` text in the system prompt so identical configurations share a cache entry across users and machines.

`excludeDynamicSections` requires `@anthropic-ai/claude-agent-sdk` v0.2.98 or later, or `claude-agent-sdk` v0.1.58 or later for Python. It applies only to the preset object form and has no effect when `systemPrompt` is a string.

The following example pairs a shared `append` block with `excludeDynamicSections` so a fleet of agents running from different directories can reuse the same cached system prompt:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

for await (const message of query({
  prompt: "Triage the open issues in this repo",
  options: {
    systemPrompt: {
      type: "preset",
      preset: "claude_code",
      append: "You operate Acme's internal triage workflow. Label issues by component and severity.",
      excludeDynamicSections: true
    }
  }
})) {
  // ...
}
```

**Tradeoffs:** the working directory, git status, and memory location still reach Claude, but as part of the first user message rather than the system prompt. Instructions in the user message carry marginally less weight than the same text in the system prompt, so Claude may rely on them less strongly when reasoning about the current directory or auto-memory paths. Enable this option when cross-session cache reuse matters more than maximally authoritative environment context.For the equivalent flag in non-interactive CLI mode, see [`--exclude-dynamic-system-prompt-sections`](https://code.claude.com/docs/en/cli-reference).

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#method-4-custom-system-prompts)  Method 4: Custom system prompts

You can provide a custom string as `systemPrompt` to replace the default entirely with your own instructions.

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

const customPrompt = `You are a Python coding specialist.
Follow these guidelines:
- Write clean, well-documented code
- Use type hints for all functions
- Include comprehensive docstrings
- Prefer functional programming patterns when appropriate
- Always explain your code choices`;

const messages = [];

for await (const message of query({
  prompt: "Create a data processing pipeline",
  options: {
    systemPrompt: customPrompt
  }
})) {
  messages.push(message);
  if (message.type === "assistant") {
    console.log(message.message.content);
  }
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#comparison-of-all-four-approaches)  Comparison of all four approaches

| Feature | CLAUDE.md | Output Styles | `systemPrompt` with append | Custom `systemPrompt` |
| --- | --- | --- | --- | --- |
| **Persistence** | Per-project file | Saved as files | Session only | Session only |
| **Reusability** | Per-project | Across projects | Code duplication | Code duplication |
| **Management** | On filesystem | CLI + files | In code | In code |
| **Default tools** | Preserved | Preserved | Preserved | Lost (unless included) |
| **Built-in safety** | Maintained | Maintained | Maintained | Must be added |
| **Environment context** | Automatic | Automatic | Automatic | Must be provided |
| **Customization level** | Additions only | Replace default | Additions only | Complete control |
| **Version control** | With project | Yes | With code | With code |
| **Scope** | Project-specific | User or project | Code session | Code session |

**Note:** “With append” means using `systemPrompt: { type: "preset", preset: "claude_code", append: "..." }` in TypeScript or `system_prompt={"type": "preset", "preset": "claude_code", "append": "..."}` in Python.

## [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#use-cases-and-best-practices)  Use cases and best practices

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#when-to-use-claude-md-2)  When to use CLAUDE.md

**Best for:**

- Project-specific coding standards and conventions
- Documenting project structure and architecture
- Listing common commands (build, test, deploy)
- Team-shared context that should be version controlled
- Instructions that apply to all SDK usage in a project

**Examples:**

- “All API endpoints should use async/await patterns”
- “Run `npm run lint:fix` before committing”
- “Database migrations are in the `migrations/` directory”

CLAUDE.md files load when the `project` setting source is enabled, which it is for default `query()` options. If you set `settingSources` (TypeScript) or `setting_sources` (Python) explicitly, include `'project'` to keep loading project-level CLAUDE.md.

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#when-to-use-output-styles)  When to use output styles

**Best for:**

- Persistent behavior changes across sessions
- Team-shared configurations
- Specialized assistants (code reviewer, data scientist, DevOps)
- Complex prompt modifications that need versioning

**Examples:**

- Creating a dedicated SQL optimization assistant
- Building a security-focused code reviewer
- Developing a teaching assistant with specific pedagogy

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#when-to-use-systemprompt-with-append)  When to use `systemPrompt` with append

**Best for:**

- Adding specific coding standards or preferences
- Customizing output formatting
- Adding domain-specific knowledge
- Modifying response verbosity
- Enhancing Claude Code’s default behavior without losing tool instructions

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#when-to-use-custom-systemprompt)  When to use custom `systemPrompt`

**Best for:**

- Complete control over Claude’s behavior
- Specialized single-session tasks
- Testing new prompt strategies
- Situations where default tools aren’t needed
- Building specialized agents with unique behavior

## [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#combining-approaches)  Combining approaches

You can combine these methods for maximum flexibility:

### [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#example-output-style-with-session-specific-additions)  Example: Output style with session-specific additions

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

// Assuming "Code Reviewer" output style is active (via /output-style)
// Add session-specific focus areas
const messages = [];

for await (const message of query({
  prompt: "Review this authentication module",
  options: {
    systemPrompt: {
      type: "preset",
      preset: "claude_code",
      append: `
        For this review, prioritize:
        - OAuth 2.0 compliance
        - Token storage security
        - Session management
      `
    }
  }
})) {
  messages.push(message);
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts\#see-also)  See also

- [Output styles](https://code.claude.com/docs/en/output-styles) \- Complete output styles documentation
- [TypeScript SDK guide](https://code.claude.com/docs/en/agent-sdk/typescript) \- Complete SDK usage guide
- [Configuration guide](https://code.claude.com/docs/en/settings) \- General configuration options

Was this page helpful?

YesNo

[Subagents in the SDK](https://code.claude.com/docs/en/agent-sdk/subagents) [Slash Commands in the SDK](https://code.claude.com/docs/en/agent-sdk/slash-commands)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.