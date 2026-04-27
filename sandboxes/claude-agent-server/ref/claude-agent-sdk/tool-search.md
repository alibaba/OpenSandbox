[Skip to main content](https://code.claude.com/docs/en/agent-sdk/tool-search#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Extend with tools

Scale to many tools with tool search

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [How tool search works](https://code.claude.com/docs/en/agent-sdk/tool-search#how-tool-search-works)
- [Configure tool search](https://code.claude.com/docs/en/agent-sdk/tool-search#configure-tool-search)
- [Optimize tool discovery](https://code.claude.com/docs/en/agent-sdk/tool-search#optimize-tool-discovery)
- [Limits](https://code.claude.com/docs/en/agent-sdk/tool-search#limits)
- [Related documentation](https://code.claude.com/docs/en/agent-sdk/tool-search#related-documentation)

Tool search enables your agent to work with hundreds or thousands of tools by dynamically discovering and loading them on demand. Instead of loading all tool definitions into the context window upfront, the agent searches your tool catalog and loads only the tools it needs.This approach solves two challenges as tool libraries scale:

- **Context efficiency:** Tool definitions can consume large portions of the context window (50 tools can use 10-20K tokens), leaving less room for actual work.
- **Tool selection accuracy:** Tool selection accuracy degrades with more than 30-50 tools loaded at once.

Tool search is enabled by default. This page covers [how it works](https://code.claude.com/docs/en/agent-sdk/tool-search#how-tool-search-works), how to [configure it](https://code.claude.com/docs/en/agent-sdk/tool-search#configure-tool-search), and how to [optimize tool discovery](https://code.claude.com/docs/en/agent-sdk/tool-search#optimize-tool-discovery).

## [​](https://code.claude.com/docs/en/agent-sdk/tool-search\#how-tool-search-works)  How tool search works

When tool search is active, tool definitions are withheld from the context window. The agent receives a summary of available tools and searches for relevant ones when the task requires a capability not already loaded. The 3-5 most relevant tools are loaded into context, where they stay available for subsequent turns. If the conversation is long enough that the SDK compacts earlier messages to free space, previously discovered tools may be removed, and the agent searches again as needed.Tool search adds one extra round-trip the first time Claude discovers a tool (the search step), but for large tool sets this is offset by smaller context on every turn. With fewer than ~10 tools, loading everything upfront is typically faster.For details on the underlying API mechanism, see [Tool search in the API](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool).

Tool search requires Claude Sonnet 4 or later, or Claude Opus 4 or later. Haiku models do not support tool search.

## [​](https://code.claude.com/docs/en/agent-sdk/tool-search\#configure-tool-search)  Configure tool search

By default, tool search is always on. You can change this with the `ENABLE_TOOL_SEARCH` environment variable:

| Value | Behavior |
| --- | --- |
| (unset) | Tool search is always on. Tool definitions are never loaded into context. This is the default. |
| `true` | Same as unset. |
| `auto` | Checks the combined token count of all tool definitions against the model’s context window. If they exceed 10%, tool search activates. If they’re under 10%, all tools are loaded into context normally. |
| `auto:N` | Same as `auto` with a custom percentage. `auto:5` activates when tool definitions exceed 5% of the context window. Lower values activate sooner. |
| `false` | Tool search is off. All tool definitions are loaded into context on every turn. |

Tool search applies to all registered tools, whether they come from remote MCP servers or [custom SDK MCP servers](https://code.claude.com/docs/en/agent-sdk/custom-tools). When using `auto`, the threshold is based on the combined size of all tool definitions across all servers.Set the value in the `env` option on `query()`. This example connects to a remote MCP server that exposes many tools, pre-approves all of them with a wildcard, and uses `auto:5` so tool search activates when their definitions exceed 5% of the context window:

TypeScript

Python

```
import { query } from "@anthropic-ai/claude-agent-sdk";

for await (const message of query({
  prompt: "Find and run the appropriate database query",
  options: {
    mcpServers: {
      "enterprise-tools": {
        // Connect to a remote MCP server
        type: "http",
        url: "https://tools.example.com/mcp"
      }
    },
    allowedTools: ["mcp__enterprise-tools__*"], // Wildcard pre-approves all tools from this server
    env: {
      ENABLE_TOOL_SEARCH: "auto:5" // Activate tool search when tools exceed 5% of context
    }
  }
})) {
  if (message.type === "result" && message.subtype === "success") {
    console.log(message.result);
  }
}
```

Setting `ENABLE_TOOL_SEARCH` to `"false"` disables tool search and loads all tool definitions into context on every turn. This removes the search round-trip, which can be faster when the tool set is small (fewer than ~10 tools) and the definitions fit comfortably in the context window.

## [​](https://code.claude.com/docs/en/agent-sdk/tool-search\#optimize-tool-discovery)  Optimize tool discovery

The search mechanism matches queries against tool names and descriptions. Names like `search_slack_messages` surface for a wider range of requests than `query_slack`. Descriptions with specific keywords (“Search Slack messages by keyword, channel, or date range”) match more queries than generic ones (“Query Slack”).You can also add a system prompt section listing available tool categories. This gives the agent context about what kinds of tools are available to search for:

```
You can search for tools to interact with Slack, GitHub, and Jira.
```

## [​](https://code.claude.com/docs/en/agent-sdk/tool-search\#limits)  Limits

- **Maximum tools:** 10,000 tools in your catalog
- **Search results:** Returns 3-5 most relevant tools per search
- **Model support:** Claude Sonnet 4 and later, Claude Opus 4 and later (no Haiku)

## [​](https://code.claude.com/docs/en/agent-sdk/tool-search\#related-documentation)  Related documentation

- [Tool search in the API](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool): Full API documentation for tool search, including custom implementations
- [Connect MCP servers](https://code.claude.com/docs/en/agent-sdk/mcp): Connect to external tools via MCP servers
- [Custom tools](https://code.claude.com/docs/en/agent-sdk/custom-tools): Build your own tools with SDK MCP servers
- [TypeScript SDK reference](https://code.claude.com/docs/en/agent-sdk/typescript): Full API reference
- [Python SDK reference](https://code.claude.com/docs/en/agent-sdk/python): Full API reference

Was this page helpful?

YesNo

[Connect to external tools with MCP](https://code.claude.com/docs/en/agent-sdk/mcp) [Subagents in the SDK](https://code.claude.com/docs/en/agent-sdk/subagents)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.