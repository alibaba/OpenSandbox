[Skip to main content](https://code.claude.com/docs/en/agent-sdk/sessions#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Core concepts

Work with sessions

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Choose an approach](https://code.claude.com/docs/en/agent-sdk/sessions#choose-an-approach)
- [Continue, resume, and fork](https://code.claude.com/docs/en/agent-sdk/sessions#continue-resume-and-fork)
- [Automatic session management](https://code.claude.com/docs/en/agent-sdk/sessions#automatic-session-management)
- [Python: ClaudeSDKClient](https://code.claude.com/docs/en/agent-sdk/sessions#python-claudesdkclient)
- [TypeScript: continue: true](https://code.claude.com/docs/en/agent-sdk/sessions#typescript-continue-true)
- [Use session options with query()](https://code.claude.com/docs/en/agent-sdk/sessions#use-session-options-with-query)
- [Capture the session ID](https://code.claude.com/docs/en/agent-sdk/sessions#capture-the-session-id)
- [Resume by ID](https://code.claude.com/docs/en/agent-sdk/sessions#resume-by-id)
- [Fork to explore alternatives](https://code.claude.com/docs/en/agent-sdk/sessions#fork-to-explore-alternatives)
- [Resume across hosts](https://code.claude.com/docs/en/agent-sdk/sessions#resume-across-hosts)
- [Related resources](https://code.claude.com/docs/en/agent-sdk/sessions#related-resources)

A session is the conversation history the SDK accumulates while your agent works. It contains your prompt, every tool call the agent made, every tool result, and every response. The SDK writes it to disk automatically so you can return to it later.Returning to a session means the agent has full context from before: files it already read, analysis it already performed, decisions it already made. You can ask a follow-up question, recover from an interruption, or branch off to try a different approach.

Sessions persist the **conversation**, not the filesystem. To snapshot and revert file changes the agent made, use [file checkpointing](https://code.claude.com/docs/en/agent-sdk/file-checkpointing).

This guide covers how to pick the right approach for your app, the SDK interfaces that track sessions automatically, how to capture session IDs and use `resume` and `fork` manually, and what to know about resuming sessions across hosts.

## [​](https://code.claude.com/docs/en/agent-sdk/sessions\#choose-an-approach)  Choose an approach

How much session handling you need depends on your application’s shape. Session management comes into play when you send multiple prompts that should share context. Within a single `query()` call, the agent already takes as many turns as it needs, and permission prompts and `AskUserQuestion` are [handled in-loop](https://code.claude.com/docs/en/agent-sdk/user-input) (they don’t end the call).

| What you’re building | What to use |
| --- | --- |
| One-shot task: single prompt, no follow-up | Nothing extra. One `query()` call handles it. |
| Multi-turn chat in one process | [`ClaudeSDKClient` (Python) or `continue: true` (TypeScript)](https://code.claude.com/docs/en/agent-sdk/sessions#automatic-session-management). The SDK tracks the session for you with no ID handling. |
| Pick up where you left off after a process restart | `continue_conversation=True` (Python) / `continue: true` (TypeScript). Resumes the most recent session in the directory, no ID needed. |
| Resume a specific past session (not the most recent) | Capture the session ID and pass it to `resume`. |
| Try an alternative approach without losing the original | Fork the session. |
| Stateless task, don’t want anything written to disk (TypeScript only) | Set [`persistSession: false`](https://code.claude.com/docs/en/agent-sdk/typescript#options). The session exists only in memory for the duration of the call. Python always persists to disk. |

### [​](https://code.claude.com/docs/en/agent-sdk/sessions\#continue-resume-and-fork)  Continue, resume, and fork

Continue, resume, and fork are option fields you set on `query()` ( [`ClaudeAgentOptions`](https://code.claude.com/docs/en/agent-sdk/python#claude-agent-options) in Python, [`Options`](https://code.claude.com/docs/en/agent-sdk/typescript#options) in TypeScript).**Continue** and **resume** both pick up an existing session and add to it. The difference is how they find that session:

- **Continue** finds the most recent session in the current directory. You don’t track anything. Works well when your app runs one conversation at a time.
- **Resume** takes a specific session ID. You track the ID. Required when you have multiple sessions (for example, one per user in a multi-user app) or want to return to one that isn’t the most recent.

**Fork** is different: it creates a new session that starts with a copy of the original’s history. The original stays unchanged. Use fork to try a different direction while keeping the option to go back.

## [​](https://code.claude.com/docs/en/agent-sdk/sessions\#automatic-session-management)  Automatic session management

Both SDKs offer an interface that tracks session state for you across calls, so you don’t pass IDs around manually. Use these for multi-turn conversations within a single process.

### [​](https://code.claude.com/docs/en/agent-sdk/sessions\#python-claudesdkclient)  Python: `ClaudeSDKClient`

[`ClaudeSDKClient`](https://code.claude.com/docs/en/agent-sdk/python#claude-sdk-client) handles session IDs internally. Each call to `client.query()` automatically continues the same session. Call [`client.receive_response()`](https://code.claude.com/docs/en/agent-sdk/python#claude-sdk-client) to iterate over the messages for the current query. The client must be used as an async context manager.This example runs two queries against the same `client`. The first asks the agent to analyze a module; the second asks it to refactor that module. Because both calls go through the same client instance, the second query has full context from the first without any explicit `resume` or session ID:

Python

```
import asyncio
from claude_agent_sdk import (
    ClaudeSDKClient,
    ClaudeAgentOptions,
    AssistantMessage,
    ResultMessage,
    TextBlock,
)

def print_response(message):
    """Print only the human-readable parts of a message."""
    if isinstance(message, AssistantMessage):
        for block in message.content:
            if isinstance(block, TextBlock):
                print(block.text)
    elif isinstance(message, ResultMessage):
        cost = (
            f"${message.total_cost_usd:.4f}"
            if message.total_cost_usd is not None
            else "N/A"
        )
        print(f"[done: {message.subtype}, cost: {cost}]")

async def main():
    options = ClaudeAgentOptions(
        allowed_tools=["Read", "Edit", "Glob", "Grep"],
    )

    async with ClaudeSDKClient(options=options) as client:
        # First query: client captures the session ID internally
        await client.query("Analyze the auth module")
        async for message in client.receive_response():
            print_response(message)

        # Second query: automatically continues the same session
        await client.query("Now refactor it to use JWT")
        async for message in client.receive_response():
            print_response(message)

asyncio.run(main())
```

See the [Python SDK reference](https://code.claude.com/docs/en/agent-sdk/python#choosing-between-query-and-claude-sdk-client) for details on when to use `ClaudeSDKClient` vs the standalone `query()` function.

### [​](https://code.claude.com/docs/en/agent-sdk/sessions\#typescript-continue-true)  TypeScript: `continue: true`

The stable TypeScript SDK (the `query()` function used throughout these docs, sometimes called V1) doesn’t have a session-holding client object like Python’s `ClaudeSDKClient`. Instead, pass `continue: true` on each subsequent `query()` call and the SDK picks up the most recent session in the current directory. No ID tracking required.This example makes two separate `query()` calls. The first creates a fresh session; the second sets `continue: true`, which tells the SDK to find and resume the most recent session on disk. The agent has full context from the first call:

TypeScript

```
import { query } from "@anthropic-ai/claude-agent-sdk";

// First query: creates a new session
for await (const message of query({
  prompt: "Analyze the auth module",
  options: { allowedTools: ["Read", "Glob", "Grep"] }
})) {
  if (message.type === "result" && message.subtype === "success") {
    console.log(message.result);
  }
}

// Second query: continue: true resumes the most recent session
for await (const message of query({
  prompt: "Now refactor it to use JWT",
  options: {
    continue: true,
    allowedTools: ["Read", "Edit", "Write", "Glob", "Grep"]
  }
})) {
  if (message.type === "result" && message.subtype === "success") {
    console.log(message.result);
  }
}
```

There’s also a [V2 preview](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview) of the TypeScript SDK that provides `createSession()` with a `send` / `stream` pattern, closer to Python’s `ClaudeSDKClient` in feel. V2 is unstable and its APIs may change; the rest of this documentation uses the stable V1 `query()` function.

## [​](https://code.claude.com/docs/en/agent-sdk/sessions\#use-session-options-with-query)  Use session options with `query()`

### [​](https://code.claude.com/docs/en/agent-sdk/sessions\#capture-the-session-id)  Capture the session ID

Resume and fork require a session ID. Read it from the `session_id` field on the result message ( [`ResultMessage`](https://code.claude.com/docs/en/agent-sdk/python#result-message) in Python, [`SDKResultMessage`](https://code.claude.com/docs/en/agent-sdk/typescript#sdk-result-message) in TypeScript), which is present on every result regardless of success or error. In TypeScript the ID is also available earlier as a direct field on the init `SystemMessage`; in Python it’s nested inside `SystemMessage.data`.

Python

TypeScript

```
import asyncio
from claude_agent_sdk import query, ClaudeAgentOptions, ResultMessage

async def main():
    session_id = None

    async for message in query(
        prompt="Analyze the auth module and suggest improvements",
        options=ClaudeAgentOptions(
            allowed_tools=["Read", "Glob", "Grep"],
        ),
    ):
        if isinstance(message, ResultMessage):
            session_id = message.session_id
            if message.subtype == "success":
                print(message.result)

    print(f"Session ID: {session_id}")
    return session_id

session_id = asyncio.run(main())
```

### [​](https://code.claude.com/docs/en/agent-sdk/sessions\#resume-by-id)  Resume by ID

Pass a session ID to `resume` to return to that specific session. The agent picks up with full context from wherever the session left off. Common reasons to resume:

- **Follow up on a completed task.** The agent already analyzed something; now you want it to act on that analysis without re-reading files.
- **Recover from a limit.** The first run ended with `error_max_turns` or `error_max_budget_usd` (see [Handle the result](https://code.claude.com/docs/en/agent-sdk/agent-loop#handle-the-result)); resume with a higher limit.
- **Restart your process.** You captured the ID before shutdown and want to restore the conversation.

This example resumes the session from [Capture the session ID](https://code.claude.com/docs/en/agent-sdk/sessions#capture-the-session-id) with a follow-up prompt. Because you’re resuming, the agent already has the prior analysis in context:

Python

TypeScript

```
# Earlier session analyzed the code; now build on that analysis
async for message in query(
    prompt="Now implement the refactoring you suggested",
    options=ClaudeAgentOptions(
        resume=session_id,
        allowed_tools=["Read", "Edit", "Write", "Glob", "Grep"],
    ),
):
    if isinstance(message, ResultMessage) and message.subtype == "success":
        print(message.result)
```

If a `resume` call returns a fresh session instead of the expected history, the most common cause is a mismatched `cwd`. Sessions are stored under `~/.claude/projects/<encoded-cwd>/*.jsonl`, where `<encoded-cwd>` is the absolute working directory with every non-alphanumeric character replaced by `-` (so `/Users/me/proj` becomes `-Users-me-proj`). If your resume call runs from a different directory, the SDK looks in the wrong place. The session file also needs to exist on the current machine.

### [​](https://code.claude.com/docs/en/agent-sdk/sessions\#fork-to-explore-alternatives)  Fork to explore alternatives

Forking creates a new session that starts with a copy of the original’s history but diverges from that point. The fork gets its own session ID; the original’s ID and history stay unchanged. You end up with two independent sessions you can resume separately.

Forking branches the conversation history, not the filesystem. If a forked agent edits files, those changes are real and visible to any session working in the same directory. To branch and revert file changes, use [file checkpointing](https://code.claude.com/docs/en/agent-sdk/file-checkpointing).

This example builds on [Capture the session ID](https://code.claude.com/docs/en/agent-sdk/sessions#capture-the-session-id): you’ve already analyzed an auth module in `session_id` and want to explore OAuth2 without losing the JWT-focused thread. The first block forks the session and captures the fork’s ID (`forked_id`); the second block resumes the original `session_id` to continue down the JWT path. You now have two session IDs pointing at two separate histories:

Python

TypeScript

```
# Fork: branch from session_id into a new session
forked_id = None
async for message in query(
    prompt="Instead of JWT, implement OAuth2 for the auth module",
    options=ClaudeAgentOptions(
        resume=session_id,
        fork_session=True,
    ),
):
    if isinstance(message, ResultMessage):
        forked_id = message.session_id  # The fork's ID, distinct from session_id
        if message.subtype == "success":
            print(message.result)

print(f"Forked session: {forked_id}")

# Original session is untouched; resuming it continues the JWT thread
async for message in query(
    prompt="Continue with the JWT approach",
    options=ClaudeAgentOptions(resume=session_id),
):
    if isinstance(message, ResultMessage) and message.subtype == "success":
        print(message.result)
```

## [​](https://code.claude.com/docs/en/agent-sdk/sessions\#resume-across-hosts)  Resume across hosts

Session files are local to the machine that created them. To resume a session on a different host (CI workers, ephemeral containers, serverless), you have two options:

- **Move the session file.** Persist `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl` from the first run and restore it to the same path on the new host before calling `resume`. The `cwd` must match.
- **Don’t rely on session resume.** Capture the results you need (analysis output, decisions, file diffs) as application state and pass them into a fresh session’s prompt. This is often more robust than shipping transcript files around.

Both SDKs expose functions for enumerating sessions on disk and reading their messages: [`listSessions()`](https://code.claude.com/docs/en/agent-sdk/typescript#list-sessions) and [`getSessionMessages()`](https://code.claude.com/docs/en/agent-sdk/typescript#get-session-messages) in TypeScript, [`list_sessions()`](https://code.claude.com/docs/en/agent-sdk/python#list-sessions) and [`get_session_messages()`](https://code.claude.com/docs/en/agent-sdk/python#get-session-messages) in Python. Use them to build custom session pickers, cleanup logic, or transcript viewers.Both SDKs also expose functions for looking up and mutating individual sessions: [`get_session_info()`](https://code.claude.com/docs/en/agent-sdk/python#get-session-info), [`rename_session()`](https://code.claude.com/docs/en/agent-sdk/python#rename-session), and [`tag_session()`](https://code.claude.com/docs/en/agent-sdk/python#tag-session) in Python, and [`getSessionInfo()`](https://code.claude.com/docs/en/agent-sdk/typescript#get-session-info), [`renameSession()`](https://code.claude.com/docs/en/agent-sdk/typescript#rename-session), and [`tagSession()`](https://code.claude.com/docs/en/agent-sdk/typescript#tag-session) in TypeScript. Use them to organize sessions by tag or give them human-readable titles.

## [​](https://code.claude.com/docs/en/agent-sdk/sessions\#related-resources)  Related resources

- [How the agent loop works](https://code.claude.com/docs/en/agent-sdk/agent-loop): Understand turns, messages, and context accumulation within a session
- [File checkpointing](https://code.claude.com/docs/en/agent-sdk/file-checkpointing): Track and revert file changes across sessions
- [Python `ClaudeAgentOptions`](https://code.claude.com/docs/en/agent-sdk/python#claude-agent-options): Full session option reference for Python
- [TypeScript `Options`](https://code.claude.com/docs/en/agent-sdk/typescript#options): Full session option reference for TypeScript

Was this page helpful?

YesNo

[Use Claude Code features](https://code.claude.com/docs/en/agent-sdk/claude-code-features) [Streaming Input](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.