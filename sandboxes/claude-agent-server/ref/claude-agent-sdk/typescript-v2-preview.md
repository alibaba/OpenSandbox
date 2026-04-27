[Skip to main content](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

SDK references

TypeScript SDK V2 interface (preview)

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Installation](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#installation)
- [Quick start](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#quick-start)
- [One-shot prompt](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#one-shot-prompt)
- [Basic session](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#basic-session)
- [Multi-turn conversation](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#multi-turn-conversation)
- [Session resume](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#session-resume)
- [Cleanup](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#cleanup)
- [API reference](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#api-reference)
- [unstable\_v2\_createSession()](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#unstable_v2_createsession)
- [unstable\_v2\_resumeSession()](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#unstable_v2_resumesession)
- [unstable\_v2\_prompt()](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#unstable_v2_prompt)
- [SDKSession interface](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#sdksession-interface)
- [Feature availability](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#feature-availability)
- [Feedback](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#feedback)
- [See also](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview#see-also)

The V2 interface is an **unstable preview**. APIs may change based on feedback before becoming stable. Some features like session forking are only available in the [V1 SDK](https://code.claude.com/docs/en/agent-sdk/typescript).

The V2 Claude Agent TypeScript SDK removes the need for async generators and yield coordination. This makes multi-turn conversations simpler, instead of managing generator state across turns, each turn is a separate `send()`/`stream()` cycle. The API surface reduces to three concepts:

- `createSession()` / `resumeSession()`: Start or continue a conversation
- `session.send()`: Send a message
- `session.stream()`: Get the response

## [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#installation)  Installation

The V2 interface is included in the existing SDK package:

```
npm install @anthropic-ai/claude-agent-sdk
```

The SDK bundles a native Claude Code binary for your platform as an optional dependency, so you don’t need to install Claude Code separately.

## [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#quick-start)  Quick start

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#one-shot-prompt)  One-shot prompt

For simple single-turn queries where you don’t need to maintain a session, use `unstable_v2_prompt()`. This example sends a math question and logs the answer:

```
import { unstable_v2_prompt } from "@anthropic-ai/claude-agent-sdk";

const result = await unstable_v2_prompt("What is 2 + 2?", {
  model: "claude-opus-4-7"
});
if (result.subtype === "success") {
  console.log(result.result);
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#basic-session)  Basic session

For interactions beyond a single prompt, create a session. V2 separates sending and streaming into distinct steps:

- `send()` dispatches your message
- `stream()` streams back the response

This explicit separation makes it easier to add logic between turns (like processing responses before sending follow-ups).The example below creates a session, sends “Hello!” to Claude, and prints the text response. It uses [`await using`](https://www.typescriptlang.org/docs/handbook/release-notes/typescript-5-2.html#using-declarations-and-explicit-resource-management) (TypeScript 5.2+) to automatically close the session when the block exits. You can also call `session.close()` manually.

```
import { unstable_v2_createSession } from "@anthropic-ai/claude-agent-sdk";

await using session = unstable_v2_createSession({
  model: "claude-opus-4-7"
});

await session.send("Hello!");
for await (const msg of session.stream()) {
  // Filter for assistant messages to get human-readable output
  if (msg.type === "assistant") {
    const text = msg.message.content
      .filter((block) => block.type === "text")
      .map((block) => block.text)
      .join("");
    console.log(text);
  }
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#multi-turn-conversation)  Multi-turn conversation

Sessions persist context across multiple exchanges. To continue a conversation, call `send()` again on the same session. Claude remembers the previous turns.This example asks a math question, then asks a follow-up that references the previous answer:

```
import { unstable_v2_createSession } from "@anthropic-ai/claude-agent-sdk";

await using session = unstable_v2_createSession({
  model: "claude-opus-4-7"
});

// Turn 1
await session.send("What is 5 + 3?");
for await (const msg of session.stream()) {
  // Filter for assistant messages to get human-readable output
  if (msg.type === "assistant") {
    const text = msg.message.content
      .filter((block) => block.type === "text")
      .map((block) => block.text)
      .join("");
    console.log(text);
  }
}

// Turn 2
await session.send("Multiply that by 2");
for await (const msg of session.stream()) {
  if (msg.type === "assistant") {
    const text = msg.message.content
      .filter((block) => block.type === "text")
      .map((block) => block.text)
      .join("");
    console.log(text);
  }
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#session-resume)  Session resume

If you have a session ID from a previous interaction, you can resume it later. This is useful for long-running workflows or when you need to persist conversations across application restarts.This example creates a session, stores its ID, closes it, then resumes the conversation:

```
import {
  unstable_v2_createSession,
  unstable_v2_resumeSession,
  type SDKMessage
} from "@anthropic-ai/claude-agent-sdk";

// Helper to extract text from assistant messages
function getAssistantText(msg: SDKMessage): string | null {
  if (msg.type !== "assistant") return null;
  return msg.message.content
    .filter((block) => block.type === "text")
    .map((block) => block.text)
    .join("");
}

// Create initial session and have a conversation
const session = unstable_v2_createSession({
  model: "claude-opus-4-7"
});

await session.send("Remember this number: 42");

// Get the session ID from any received message
let sessionId: string | undefined;
for await (const msg of session.stream()) {
  sessionId = msg.session_id;
  const text = getAssistantText(msg);
  if (text) console.log("Initial response:", text);
}

console.log("Session ID:", sessionId);
session.close();

// Later: resume the session using the stored ID
await using resumedSession = unstable_v2_resumeSession(sessionId!, {
  model: "claude-opus-4-7"
});

await resumedSession.send("What number did I ask you to remember?");
for await (const msg of resumedSession.stream()) {
  const text = getAssistantText(msg);
  if (text) console.log("Resumed response:", text);
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#cleanup)  Cleanup

Sessions can be closed manually or automatically using [`await using`](https://www.typescriptlang.org/docs/handbook/release-notes/typescript-5-2.html#using-declarations-and-explicit-resource-management), a TypeScript 5.2+ feature for automatic resource cleanup. If you’re using an older TypeScript version or encounter compatibility issues, use manual cleanup instead.**Automatic cleanup (TypeScript 5.2+):**

```
import { unstable_v2_createSession } from "@anthropic-ai/claude-agent-sdk";

await using session = unstable_v2_createSession({
  model: "claude-opus-4-7"
});
// Session closes automatically when the block exits
```

**Manual cleanup:**

```
import { unstable_v2_createSession } from "@anthropic-ai/claude-agent-sdk";

const session = unstable_v2_createSession({
  model: "claude-opus-4-7"
});
// ... use the session ...
session.close();
```

## [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#api-reference)  API reference

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#unstable_v2_createsession)  `unstable_v2_createSession()`

Creates a new session for multi-turn conversations.

```
function unstable_v2_createSession(options: {
  model: string;
  // Additional options supported
}): SDKSession;
```

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#unstable_v2_resumesession)  `unstable_v2_resumeSession()`

Resumes an existing session by ID.

```
function unstable_v2_resumeSession(
  sessionId: string,
  options: {
    model: string;
    // Additional options supported
  }
): SDKSession;
```

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#unstable_v2_prompt)  `unstable_v2_prompt()`

One-shot convenience function for single-turn queries.

```
function unstable_v2_prompt(
  prompt: string,
  options: {
    model: string;
    // Additional options supported
  }
): Promise<SDKResultMessage>;
```

### [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#sdksession-interface)  SDKSession interface

```
interface SDKSession {
  readonly sessionId: string;
  send(message: string | SDKUserMessage): Promise<void>;
  stream(): AsyncGenerator<SDKMessage, void>;
  close(): void;
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#feature-availability)  Feature availability

Not all V1 features are available in V2 yet. The following require using the [V1 SDK](https://code.claude.com/docs/en/agent-sdk/typescript):

- Session forking (`forkSession` option)
- Some advanced streaming input patterns

## [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#feedback)  Feedback

Share your feedback on the V2 interface before it becomes stable. Report issues and suggestions through [GitHub Issues](https://github.com/anthropics/claude-code/issues).

## [​](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview\#see-also)  See also

- [TypeScript SDK reference (V1)](https://code.claude.com/docs/en/agent-sdk/typescript) \- Full V1 SDK documentation
- [SDK overview](https://code.claude.com/docs/en/agent-sdk/overview) \- General SDK concepts
- [V2 examples on GitHub](https://github.com/anthropics/claude-agent-sdk-demos/tree/main/hello-world-v2) \- Working code examples

Was this page helpful?

YesNo

[TypeScript SDK](https://code.claude.com/docs/en/agent-sdk/typescript) [Python SDK](https://code.claude.com/docs/en/agent-sdk/python)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.