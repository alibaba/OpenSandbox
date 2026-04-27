[Skip to main content](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Input and output

Streaming Input

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Overview](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#overview)
- [Streaming Input Mode (Recommended)](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#streaming-input-mode-recommended)
- [How It Works](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#how-it-works)
- [Benefits](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#benefits)
- [Implementation Example](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#implementation-example)
- [Single Message Input](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#single-message-input)
- [When to Use Single Message Input](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#when-to-use-single-message-input)
- [Limitations](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#limitations)
- [Implementation Example](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode#implementation-example-2)

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#overview)  Overview

The Claude Agent SDK supports two distinct input modes for interacting with agents:

- **Streaming Input Mode** (Default & Recommended) - A persistent, interactive session
- **Single Message Input** \- One-shot queries that use session state and resuming

This guide explains the differences, benefits, and use cases for each mode to help you choose the right approach for your application.

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#streaming-input-mode-recommended)  Streaming Input Mode (Recommended)

Streaming input mode is the **preferred** way to use the Claude Agent SDK. It provides full access to the agent’s capabilities and enables rich, interactive experiences.It allows the agent to operate as a long lived process that takes in user input, handles interruptions, surfaces permission requests, and handles session management.

### [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#how-it-works)  How It Works

Environment/File SystemTools/HooksClaude AgentYour ApplicationEnvironment/File SystemTools/HooksClaude AgentYour ApplicationSession stays alivePersistent file systemstate maintainedInitialize with AsyncGeneratorYield Message 1Execute toolsRead filesFile contentsWrite/Edit filesSuccess/ErrorStream partial responseStream more content...Complete Message 1Yield Message 2 + ImageProcess image & executeAccess filesystemOperation resultsStream response 2Queue Message 3Interrupt/CancelHandle interruption

### [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#benefits)  Benefits

## Image Uploads

Attach images directly to messages for visual analysis and understanding

## Queued Messages

Send multiple messages that process sequentially, with ability to interrupt

## Tool Integration

Full access to all tools and custom MCP servers during the session

## Hooks Support

Use lifecycle hooks to customize behavior at various points

## Real-time Feedback

See responses as they’re generated, not just final results

## Context Persistence

Maintain conversation context across multiple turns naturally

### [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#implementation-example)  Implementation Example

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";
import { readFile } from "fs/promises";

async function* generateMessages() {
  // First message
  yield {
    type: "user" as const,
    message: {
      role: "user" as const,
      content: "Analyze this codebase for security issues"
    }
  };

  // Wait for conditions or user input
  await new Promise((resolve) => setTimeout(resolve, 2000));

  // Follow-up with image
  yield {
    type: "user" as const,
    message: {
      role: "user" as const,
      content: [\
        {\
          type: "text",\
          text: "Review this architecture diagram"\
        },\
        {\
          type: "image",\
          source: {\
            type: "base64",\
            media_type: "image/png",\
            data: await readFile("diagram.png", "base64")\
          }\
        }\
      ]
    }
  };
}

// Process streaming responses
for await (const message of query({
  prompt: generateMessages(),
  options: {
    maxTurns: 10,
    allowedTools: ["Read", "Grep"]
  }
})) {
  if (message.type === "result") {
    console.log(message.result);
  }
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#single-message-input)  Single Message Input

Single message input is simpler but more limited.

### [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#when-to-use-single-message-input)  When to Use Single Message Input

Use single message input when:

- You need a one-shot response
- You do not need image attachments, hooks, etc.
- You need to operate in a stateless environment, such as a lambda function

### [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#limitations)  Limitations

Single message input mode does **not** support:

- Direct image attachments in messages
- Dynamic message queueing
- Real-time interruption
- Hook integration
- Natural multi-turn conversations

### [​](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode\#implementation-example-2)  Implementation Example

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

// Simple one-shot query
for await (const message of query({
  prompt: "Explain the authentication flow",
  options: {
    maxTurns: 1,
    allowedTools: ["Read", "Grep"]
  }
})) {
  if (message.type === "result") {
    console.log(message.result);
  }
}

// Continue conversation with session management
for await (const message of query({
  prompt: "Now explain the authorization process",
  options: {
    continue: true,
    maxTurns: 1
  }
})) {
  if (message.type === "result") {
    console.log(message.result);
  }
}
```

Was this page helpful?

YesNo

[Work with sessions](https://code.claude.com/docs/en/agent-sdk/sessions) [Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.