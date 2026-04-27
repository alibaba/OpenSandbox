[Skip to main content](https://code.claude.com/docs/en/agent-sdk/streaming-output#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Input and output

Stream responses in real-time

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Enable streaming output](https://code.claude.com/docs/en/agent-sdk/streaming-output#enable-streaming-output)
- [StreamEvent reference](https://code.claude.com/docs/en/agent-sdk/streaming-output#streamevent-reference)
- [Message flow](https://code.claude.com/docs/en/agent-sdk/streaming-output#message-flow)
- [Stream text responses](https://code.claude.com/docs/en/agent-sdk/streaming-output#stream-text-responses)
- [Stream tool calls](https://code.claude.com/docs/en/agent-sdk/streaming-output#stream-tool-calls)
- [Build a streaming UI](https://code.claude.com/docs/en/agent-sdk/streaming-output#build-a-streaming-ui)
- [Known limitations](https://code.claude.com/docs/en/agent-sdk/streaming-output#known-limitations)
- [Next steps](https://code.claude.com/docs/en/agent-sdk/streaming-output#next-steps)

By default, the Agent SDK yields complete `AssistantMessage` objects after Claude finishes generating each response. To receive incremental updates as text and tool calls are generated, enable partial message streaming by setting `include_partial_messages` (Python) or `includePartialMessages` (TypeScript) to `true` in your options.

This page covers output streaming (receiving tokens in real-time). For input modes (how you send messages), see [Send messages to agents](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode). You can also [stream responses using the Agent SDK via the CLI](https://code.claude.com/docs/en/headless).

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#enable-streaming-output)  Enable streaming output

To enable streaming, set `include_partial_messages` (Python) or `includePartialMessages` (TypeScript) to `true` in your options. This causes the SDK to yield `StreamEvent` messages containing raw API events as they arrive, in addition to the usual `AssistantMessage` and `ResultMessage`.Your code then needs to:

1. Check each message’s type to distinguish `StreamEvent` from other message types
2. For `StreamEvent`, extract the `event` field and check its `type`
3. Look for `content_block_delta` events where `delta.type` is `text_delta`, which contain the actual text chunks

The example below enables streaming and prints text chunks as they arrive. Notice the nested type checks: first for `StreamEvent`, then for `content_block_delta`, then for `text_delta`:

Python

TypeScript

```
from claude_agent_sdk import query, ClaudeAgentOptions
from claude_agent_sdk.types import StreamEvent
import asyncio

async def stream_response():
    options = ClaudeAgentOptions(
        include_partial_messages=True,
        allowed_tools=["Bash", "Read"],
    )

    async for message in query(prompt="List the files in my project", options=options):
        if isinstance(message, StreamEvent):
            event = message.event
            if event.get("type") == "content_block_delta":
                delta = event.get("delta", {})
                if delta.get("type") == "text_delta":
                    print(delta.get("text", ""), end="", flush=True)

asyncio.run(stream_response())
```

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#streamevent-reference)  StreamEvent reference

When partial messages are enabled, you receive raw Claude API streaming events wrapped in an object. The type has different names in each SDK:

- **Python**: `StreamEvent` (import from `claude_agent_sdk.types`)
- **TypeScript**: `SDKPartialAssistantMessage` with `type: 'stream_event'`

Both contain raw Claude API events, not accumulated text. You need to extract and accumulate text deltas yourself. Here’s the structure of each type:

Python

TypeScript

```
@dataclass
class StreamEvent:
    uuid: str  # Unique identifier for this event
    session_id: str  # Session identifier
    event: dict[str, Any]  # The raw Claude API stream event
    parent_tool_use_id: str | None  # Parent tool ID if from a subagent
```

The `event` field contains the raw streaming event from the [Claude API](https://platform.claude.com/docs/en/build-with-claude/streaming#event-types). Common event types include:

| Event Type | Description |
| --- | --- |
| `message_start` | Start of a new message |
| `content_block_start` | Start of a new content block (text or tool use) |
| `content_block_delta` | Incremental update to content |
| `content_block_stop` | End of a content block |
| `message_delta` | Message-level updates (stop reason, usage) |
| `message_stop` | End of the message |

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#message-flow)  Message flow

With partial messages enabled, you receive messages in this order:

```
StreamEvent (message_start)
StreamEvent (content_block_start) - text block
StreamEvent (content_block_delta) - text chunks...
StreamEvent (content_block_stop)
StreamEvent (content_block_start) - tool_use block
StreamEvent (content_block_delta) - tool input chunks...
StreamEvent (content_block_stop)
StreamEvent (message_delta)
StreamEvent (message_stop)
AssistantMessage - complete message with all content
... tool executes ...
... more streaming events for next turn ...
ResultMessage - final result
```

Without partial messages enabled (`include_partial_messages` in Python, `includePartialMessages` in TypeScript), you receive all message types except `StreamEvent`. Common types include `SystemMessage` (session initialization), `AssistantMessage` (complete responses), `ResultMessage` (final result), and a compact boundary message indicating when conversation history was compacted (`SDKCompactBoundaryMessage` in TypeScript; `SystemMessage` with subtype `"compact_boundary"` in Python).

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#stream-text-responses)  Stream text responses

To display text as it’s generated, look for `content_block_delta` events where `delta.type` is `text_delta`. These contain the incremental text chunks. The example below prints each chunk as it arrives:

Python

TypeScript

```
from claude_agent_sdk import query, ClaudeAgentOptions
from claude_agent_sdk.types import StreamEvent
import asyncio

async def stream_text():
    options = ClaudeAgentOptions(include_partial_messages=True)

    async for message in query(prompt="Explain how databases work", options=options):
        if isinstance(message, StreamEvent):
            event = message.event
            if event.get("type") == "content_block_delta":
                delta = event.get("delta", {})
                if delta.get("type") == "text_delta":
                    # Print each text chunk as it arrives
                    print(delta.get("text", ""), end="", flush=True)

    print()  # Final newline

asyncio.run(stream_text())
```

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#stream-tool-calls)  Stream tool calls

Tool calls also stream incrementally. You can track when tools start, receive their input as it’s generated, and see when they complete. The example below tracks the current tool being called and accumulates the JSON input as it streams in. It uses three event types:

- `content_block_start`: tool begins
- `content_block_delta` with `input_json_delta`: input chunks arrive
- `content_block_stop`: tool call complete

Python

TypeScript

```
from claude_agent_sdk import query, ClaudeAgentOptions
from claude_agent_sdk.types import StreamEvent
import asyncio

async def stream_tool_calls():
    options = ClaudeAgentOptions(
        include_partial_messages=True,
        allowed_tools=["Read", "Bash"],
    )

    # Track the current tool and accumulate its input JSON
    current_tool = None
    tool_input = ""

    async for message in query(prompt="Read the README.md file", options=options):
        if isinstance(message, StreamEvent):
            event = message.event
            event_type = event.get("type")

            if event_type == "content_block_start":
                # New tool call is starting
                content_block = event.get("content_block", {})
                if content_block.get("type") == "tool_use":
                    current_tool = content_block.get("name")
                    tool_input = ""
                    print(f"Starting tool: {current_tool}")

            elif event_type == "content_block_delta":
                delta = event.get("delta", {})
                if delta.get("type") == "input_json_delta":
                    # Accumulate JSON input as it streams in
                    chunk = delta.get("partial_json", "")
                    tool_input += chunk
                    print(f"  Input chunk: {chunk}")

            elif event_type == "content_block_stop":
                # Tool call complete - show final input
                if current_tool:
                    print(f"Tool {current_tool} called with: {tool_input}")
                    current_tool = None

asyncio.run(stream_tool_calls())
```

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#build-a-streaming-ui)  Build a streaming UI

This example combines text and tool streaming into a cohesive UI. It tracks whether the agent is currently executing a tool (using an `in_tool` flag) to show status indicators like `[Using Read...]` while tools run. Text streams normally when not in a tool, and tool completion triggers a “done” message. This pattern is useful for chat interfaces that need to show progress during multi-step agent tasks.

Python

TypeScript

```
from claude_agent_sdk import query, ClaudeAgentOptions, ResultMessage
from claude_agent_sdk.types import StreamEvent
import asyncio
import sys

async def streaming_ui():
    options = ClaudeAgentOptions(
        include_partial_messages=True,
        allowed_tools=["Read", "Bash", "Grep"],
    )

    # Track whether we're currently in a tool call
    in_tool = False

    async for message in query(
        prompt="Find all TODO comments in the codebase", options=options
    ):
        if isinstance(message, StreamEvent):
            event = message.event
            event_type = event.get("type")

            if event_type == "content_block_start":
                content_block = event.get("content_block", {})
                if content_block.get("type") == "tool_use":
                    # Tool call is starting - show status indicator
                    tool_name = content_block.get("name")
                    print(f"\n[Using {tool_name}...]", end="", flush=True)
                    in_tool = True

            elif event_type == "content_block_delta":
                delta = event.get("delta", {})
                # Only stream text when not executing a tool
                if delta.get("type") == "text_delta" and not in_tool:
                    sys.stdout.write(delta.get("text", ""))
                    sys.stdout.flush()

            elif event_type == "content_block_stop":
                if in_tool:
                    # Tool call finished
                    print(" done", flush=True)
                    in_tool = False

        elif isinstance(message, ResultMessage):
            # Agent finished all work
            print(f"\n\n--- Complete ---")

asyncio.run(streaming_ui())
```

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#known-limitations)  Known limitations

Some SDK features are incompatible with streaming:

- **Extended thinking**: when you explicitly set `max_thinking_tokens` (Python) or `maxThinkingTokens` (TypeScript), `StreamEvent` messages are not emitted. You’ll only receive complete messages after each turn. Note that thinking is disabled by default in the SDK, so streaming works unless you enable it.
- **Structured output**: the JSON result appears only in the final `ResultMessage.structured_output`, not as streaming deltas. See [structured outputs](https://code.claude.com/docs/en/agent-sdk/structured-outputs) for details.

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-output\#next-steps)  Next steps

Now that you can stream text and tool calls in real-time, explore these related topics:

- [Interactive vs one-shot queries](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode): choose between input modes for your use case
- [Structured outputs](https://code.claude.com/docs/en/agent-sdk/structured-outputs): get typed JSON responses from the agent
- [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions): control which tools the agent can use

Was this page helpful?

YesNo

[Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input) [Get structured output from agents](https://code.claude.com/docs/en/agent-sdk/structured-outputs)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.