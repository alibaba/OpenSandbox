[Skip to main content](https://code.claude.com/docs/en/agent-sdk/quickstart#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Agent SDK

Quickstart

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Prerequisites](https://code.claude.com/docs/en/agent-sdk/quickstart#prerequisites)
- [Setup](https://code.claude.com/docs/en/agent-sdk/quickstart#setup)
- [Create a buggy file](https://code.claude.com/docs/en/agent-sdk/quickstart#create-a-buggy-file)
- [Build an agent that finds and fixes bugs](https://code.claude.com/docs/en/agent-sdk/quickstart#build-an-agent-that-finds-and-fixes-bugs)
- [Run your agent](https://code.claude.com/docs/en/agent-sdk/quickstart#run-your-agent)
- [Try other prompts](https://code.claude.com/docs/en/agent-sdk/quickstart#try-other-prompts)
- [Customize your agent](https://code.claude.com/docs/en/agent-sdk/quickstart#customize-your-agent)
- [Key concepts](https://code.claude.com/docs/en/agent-sdk/quickstart#key-concepts)
- [Troubleshooting](https://code.claude.com/docs/en/agent-sdk/quickstart#troubleshooting)
- [API error thinking.type.enabled is not supported for this model](https://code.claude.com/docs/en/agent-sdk/quickstart#api-error-thinking-type-enabled-is-not-supported-for-this-model)
- [Next steps](https://code.claude.com/docs/en/agent-sdk/quickstart#next-steps)

Use the Agent SDK to build an AI agent that reads your code, finds bugs, and fixes them, all without manual intervention.**What you’ll do:**

1. Set up a project with the Agent SDK
2. Create a file with some buggy code
3. Run an agent that finds and fixes the bugs automatically

## [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#prerequisites)  Prerequisites

- **Node.js 18+** or **Python 3.10+**
- An **Anthropic account** ( [sign up here](https://platform.claude.com/))

## [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#setup)  Setup

1

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/quickstart#)

Create a project folder

Create a new directory for this quickstart:

```
mkdir my-agent && cd my-agent
```

For your own projects, you can run the SDK from any folder; it will have access to files in that directory and its subdirectories by default.

2

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/quickstart#)

Install the SDK

Install the Agent SDK package for your language:

- TypeScript

- Python (uv)

- Python (pip)


```
npm install @anthropic-ai/claude-agent-sdk
```

[uv Python package manager](https://docs.astral.sh/uv/) is a fast Python package manager that handles virtual environments automatically:

```
uv init && uv add claude-agent-sdk
```

Create a virtual environment first, then install:

```
python3 -m venv .venv && source .venv/bin/activate
pip3 install claude-agent-sdk
```

The TypeScript SDK bundles a native Claude Code binary for your platform as an optional dependency, so you don’t need to install Claude Code separately.

3

[Navigate to header](https://code.claude.com/docs/en/agent-sdk/quickstart#)

Set your API key

Get an API key from the [Claude Console](https://platform.claude.com/), then create a `.env` file in your project directory:

```
ANTHROPIC_API_KEY=your-api-key
```

The SDK also supports authentication via third-party API providers:

- **Amazon Bedrock**: set `CLAUDE_CODE_USE_BEDROCK=1` environment variable and configure AWS credentials
- **Google Vertex AI**: set `CLAUDE_CODE_USE_VERTEX=1` environment variable and configure Google Cloud credentials
- **Microsoft Azure**: set `CLAUDE_CODE_USE_FOUNDRY=1` environment variable and configure Azure credentials

See the setup guides for [Bedrock](https://code.claude.com/docs/en/amazon-bedrock), [Vertex AI](https://code.claude.com/docs/en/google-vertex-ai), or [Azure AI Foundry](https://code.claude.com/docs/en/microsoft-foundry) for details.

Unless previously approved, Anthropic does not allow third party developers to offer claude.ai login or rate limits for their products, including agents built on the Claude Agent SDK. Please use the API key authentication methods described in this document instead.

## [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#create-a-buggy-file)  Create a buggy file

This quickstart walks you through building an agent that can find and fix bugs in code. First, you need a file with some intentional bugs for the agent to fix. Create `utils.py` in the `my-agent` directory and paste the following code:

```
def calculate_average(numbers):
    total = 0
    for num in numbers:
        total += num
    return total / len(numbers)

def get_user_name(user):
    return user["name"].upper()
```

This code has two bugs:

1. `calculate_average([])` crashes with division by zero
2. `get_user_name(None)` crashes with a TypeError

## [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#build-an-agent-that-finds-and-fixes-bugs)  Build an agent that finds and fixes bugs

Create `agent.py` if you’re using the Python SDK, or `agent.ts` for TypeScript:

Python

TypeScript

```
import asyncio
from claude_agent_sdk import query, ClaudeAgentOptions, AssistantMessage, ResultMessage

async def main():
    # Agentic loop: streams messages as Claude works
    async for message in query(
        prompt="Review utils.py for bugs that would cause crashes. Fix any issues you find.",
        options=ClaudeAgentOptions(
            allowed_tools=["Read", "Edit", "Glob"],  # Tools Claude can use
            permission_mode="acceptEdits",  # Auto-approve file edits
        ),
    ):
        # Print human-readable output
        if isinstance(message, AssistantMessage):
            for block in message.content:
                if hasattr(block, "text"):
                    print(block.text)  # Claude's reasoning
                elif hasattr(block, "name"):
                    print(f"Tool: {block.name}")  # Tool being called
        elif isinstance(message, ResultMessage):
            print(f"Done: {message.subtype}")  # Final result

asyncio.run(main())
```

This code has three main parts:

1. **`query`**: the main entry point that creates the agentic loop. It returns an async iterator, so you use `async for` to stream messages as Claude works. See the full API in the [Python](https://code.claude.com/docs/en/agent-sdk/python#query) or [TypeScript](https://code.claude.com/docs/en/agent-sdk/typescript#query) SDK reference.
2. **`prompt`**: what you want Claude to do. Claude figures out which tools to use based on the task.
3. **`options`**: configuration for the agent. This example uses `allowedTools` to pre-approve `Read`, `Edit`, and `Glob`, and `permissionMode: "acceptEdits"` to auto-approve file changes. Other options include `systemPrompt`, `mcpServers`, and more. See all options for [Python](https://code.claude.com/docs/en/agent-sdk/python#claude-agent-options) or [TypeScript](https://code.claude.com/docs/en/agent-sdk/typescript#options).

The `async for` loop keeps running as Claude thinks, calls tools, observes results, and decides what to do next. Each iteration yields a message: Claude’s reasoning, a tool call, a tool result, or the final outcome. The SDK handles the orchestration (tool execution, context management, retries) so you just consume the stream. The loop ends when Claude finishes the task or hits an error.The message handling inside the loop filters for human-readable output. Without filtering, you’d see raw message objects including system initialization and internal state, which is useful for debugging but noisy otherwise.

This example uses streaming to show progress in real-time. If you don’t need live output (e.g., for background jobs or CI pipelines), you can collect all messages at once. See [Streaming vs. single-turn mode](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode) for details.

### [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#run-your-agent)  Run your agent

Your agent is ready. Run it with the following command:

- Python

- TypeScript


```
python3 agent.py
```

```
npx tsx agent.ts
```

After running, check `utils.py`. You’ll see defensive code handling empty lists and null users. Your agent autonomously:

1. **Read**`utils.py` to understand the code
2. **Analyzed** the logic and identified edge cases that would crash
3. **Edited** the file to add proper error handling

This is what makes the Agent SDK different: Claude executes tools directly instead of asking you to implement them.

If you see “API key not found”, make sure you’ve set the `ANTHROPIC_API_KEY` environment variable in your `.env` file or shell environment. See the [full troubleshooting guide](https://code.claude.com/docs/en/troubleshooting) for more help.

### [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#try-other-prompts)  Try other prompts

Now that your agent is set up, try some different prompts:

- `"Add docstrings to all functions in utils.py"`
- `"Add type hints to all functions in utils.py"`
- `"Create a README.md documenting the functions in utils.py"`

### [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#customize-your-agent)  Customize your agent

You can modify your agent’s behavior by changing the options. Here are a few examples:**Add web search capability:**

Python

TypeScript

```
options = ClaudeAgentOptions(
    allowed_tools=["Read", "Edit", "Glob", "WebSearch"], permission_mode="acceptEdits"
)
```

**Give Claude a custom system prompt:**

Python

TypeScript

```
options = ClaudeAgentOptions(
    allowed_tools=["Read", "Edit", "Glob"],
    permission_mode="acceptEdits",
    system_prompt="You are a senior Python developer. Always follow PEP 8 style guidelines.",
)
```

**Run commands in the terminal:**

Python

TypeScript

```
options = ClaudeAgentOptions(
    allowed_tools=["Read", "Edit", "Glob", "Bash"], permission_mode="acceptEdits"
)
```

With `Bash` enabled, try: `"Write unit tests for utils.py, run them, and fix any failures"`

## [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#key-concepts)  Key concepts

**Tools** control what your agent can do:

| Tools | What the agent can do |
| --- | --- |
| `Read`, `Glob`, `Grep` | Read-only analysis |
| `Read`, `Edit`, `Glob` | Analyze and modify code |
| `Read`, `Edit`, `Bash`, `Glob`, `Grep` | Full automation |

**Permission modes** control how much human oversight you want:

| Mode | Behavior | Use case |
| --- | --- | --- |
| `acceptEdits` | Auto-approves file edits and common filesystem commands, asks for other actions | Trusted development workflows |
| `dontAsk` | Denies anything not in `allowedTools` | Locked-down headless agents |
| `auto` (TypeScript only) | A model classifier approves or denies each tool call | Autonomous agents with safety guardrails |
| `bypassPermissions` | Runs every tool without prompts | Sandboxed CI, fully trusted environments |
| `default` | Requires a `canUseTool` callback to handle approval | Custom approval flows |

The example above uses `acceptEdits` mode, which auto-approves file operations so the agent can run without interactive prompts. If you want to prompt users for approval, use `default` mode and provide a [`canUseTool` callback](https://code.claude.com/docs/en/agent-sdk/user-input) that collects user input. For more control, see [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions).

## [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#troubleshooting)  Troubleshooting

### [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#api-error-thinking-type-enabled-is-not-supported-for-this-model)  API error `thinking.type.enabled` is not supported for this model

Claude Opus 4.7 replaces `thinking.type.enabled` with `thinking.type.adaptive`. Older Agent SDK versions fail with the following API error when you select `claude-opus-4-7`:

```
API Error: 400 {"type":"invalid_request_error","message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\" to control thinking behavior."}
```

Upgrade to Agent SDK v0.2.111 or later to use Opus 4.7.

## [​](https://code.claude.com/docs/en/agent-sdk/quickstart\#next-steps)  Next steps

Now that you’ve created your first agent, learn how to extend its capabilities and tailor it to your use case:

- **[Permissions](https://code.claude.com/docs/en/agent-sdk/permissions)**: control what your agent can do and when it needs approval
- **[Hooks](https://code.claude.com/docs/en/agent-sdk/hooks)**: run custom code before or after tool calls
- **[Sessions](https://code.claude.com/docs/en/agent-sdk/sessions)**: build multi-turn agents that maintain context
- **[MCP servers](https://code.claude.com/docs/en/agent-sdk/mcp)**: connect to databases, browsers, APIs, and other external systems
- **[Hosting](https://code.claude.com/docs/en/agent-sdk/hosting)**: deploy agents to Docker, cloud, and CI/CD
- **[Example agents](https://github.com/anthropics/claude-agent-sdk-demos)**: see complete examples: email assistant, research agent, and more

Was this page helpful?

YesNo

[Overview](https://code.claude.com/docs/en/agent-sdk/overview) [How the agent loop works](https://code.claude.com/docs/en/agent-sdk/agent-loop)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.