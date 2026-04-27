[Skip to main content](https://code.claude.com/docs/en/agent-sdk/python#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

SDK references

Agent SDK reference - Python

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Installation](https://code.claude.com/docs/en/agent-sdk/python#installation)
- [Choosing between query() and ClaudeSDKClient](https://code.claude.com/docs/en/agent-sdk/python#choosing-between-query-and-claudesdkclient)
- [Quick comparison](https://code.claude.com/docs/en/agent-sdk/python#quick-comparison)
- [When to use query() (new session each time)](https://code.claude.com/docs/en/agent-sdk/python#when-to-use-query-new-session-each-time)
- [When to use ClaudeSDKClient (continuous conversation)](https://code.claude.com/docs/en/agent-sdk/python#when-to-use-claudesdkclient-continuous-conversation)
- [Functions](https://code.claude.com/docs/en/agent-sdk/python#functions)
- [query()](https://code.claude.com/docs/en/agent-sdk/python#query)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters)
- [Returns](https://code.claude.com/docs/en/agent-sdk/python#returns)
- [Example - With options](https://code.claude.com/docs/en/agent-sdk/python#example-with-options)
- [tool()](https://code.claude.com/docs/en/agent-sdk/python#tool)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters-2)
- [Input schema options](https://code.claude.com/docs/en/agent-sdk/python#input-schema-options)
- [Returns](https://code.claude.com/docs/en/agent-sdk/python#returns-2)
- [Example](https://code.claude.com/docs/en/agent-sdk/python#example)
- [ToolAnnotations](https://code.claude.com/docs/en/agent-sdk/python#toolannotations)
- [create\_sdk\_mcp\_server()](https://code.claude.com/docs/en/agent-sdk/python#create_sdk_mcp_server)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters-3)
- [Returns](https://code.claude.com/docs/en/agent-sdk/python#returns-3)
- [Example](https://code.claude.com/docs/en/agent-sdk/python#example-2)
- [list\_sessions()](https://code.claude.com/docs/en/agent-sdk/python#list_sessions)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters-4)
- [Return type: SDKSessionInfo](https://code.claude.com/docs/en/agent-sdk/python#return-type-sdksessioninfo)
- [Example](https://code.claude.com/docs/en/agent-sdk/python#example-3)
- [get\_session\_messages()](https://code.claude.com/docs/en/agent-sdk/python#get_session_messages)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters-5)
- [Return type: SessionMessage](https://code.claude.com/docs/en/agent-sdk/python#return-type-sessionmessage)
- [Example](https://code.claude.com/docs/en/agent-sdk/python#example-4)
- [get\_session\_info()](https://code.claude.com/docs/en/agent-sdk/python#get_session_info)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters-6)
- [Example](https://code.claude.com/docs/en/agent-sdk/python#example-5)
- [rename\_session()](https://code.claude.com/docs/en/agent-sdk/python#rename_session)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters-7)
- [Example](https://code.claude.com/docs/en/agent-sdk/python#example-6)
- [tag\_session()](https://code.claude.com/docs/en/agent-sdk/python#tag_session)
- [Parameters](https://code.claude.com/docs/en/agent-sdk/python#parameters-8)
- [Example](https://code.claude.com/docs/en/agent-sdk/python#example-7)
- [Classes](https://code.claude.com/docs/en/agent-sdk/python#classes)
- [ClaudeSDKClient](https://code.claude.com/docs/en/agent-sdk/python#claudesdkclient)
- [Key Features](https://code.claude.com/docs/en/agent-sdk/python#key-features)
- [Methods](https://code.claude.com/docs/en/agent-sdk/python#methods)
- [Context Manager Support](https://code.claude.com/docs/en/agent-sdk/python#context-manager-support)
- [Example - Continuing a conversation](https://code.claude.com/docs/en/agent-sdk/python#example-continuing-a-conversation)
- [Example - Streaming input with ClaudeSDKClient](https://code.claude.com/docs/en/agent-sdk/python#example-streaming-input-with-claudesdkclient)
- [Example - Using interrupts](https://code.claude.com/docs/en/agent-sdk/python#example-using-interrupts)
- [Example - Advanced permission control](https://code.claude.com/docs/en/agent-sdk/python#example-advanced-permission-control)
- [Types](https://code.claude.com/docs/en/agent-sdk/python#types)
- [SdkMcpTool](https://code.claude.com/docs/en/agent-sdk/python#sdkmcptool)
- [Transport](https://code.claude.com/docs/en/agent-sdk/python#transport)
- [ClaudeAgentOptions](https://code.claude.com/docs/en/agent-sdk/python#claudeagentoptions)
- [OutputFormat](https://code.claude.com/docs/en/agent-sdk/python#outputformat)
- [SystemPromptPreset](https://code.claude.com/docs/en/agent-sdk/python#systempromptpreset)
- [SettingSource](https://code.claude.com/docs/en/agent-sdk/python#settingsource)
- [Default behavior](https://code.claude.com/docs/en/agent-sdk/python#default-behavior)
- [Why use setting\_sources](https://code.claude.com/docs/en/agent-sdk/python#why-use-setting_sources)
- [Settings precedence](https://code.claude.com/docs/en/agent-sdk/python#settings-precedence)
- [AgentDefinition](https://code.claude.com/docs/en/agent-sdk/python#agentdefinition)
- [PermissionMode](https://code.claude.com/docs/en/agent-sdk/python#permissionmode)
- [CanUseTool](https://code.claude.com/docs/en/agent-sdk/python#canusetool)
- [ToolPermissionContext](https://code.claude.com/docs/en/agent-sdk/python#toolpermissioncontext)
- [PermissionResult](https://code.claude.com/docs/en/agent-sdk/python#permissionresult)
- [PermissionResultAllow](https://code.claude.com/docs/en/agent-sdk/python#permissionresultallow)
- [PermissionResultDeny](https://code.claude.com/docs/en/agent-sdk/python#permissionresultdeny)
- [PermissionUpdate](https://code.claude.com/docs/en/agent-sdk/python#permissionupdate)
- [PermissionRuleValue](https://code.claude.com/docs/en/agent-sdk/python#permissionrulevalue)
- [ToolsPreset](https://code.claude.com/docs/en/agent-sdk/python#toolspreset)
- [ThinkingConfig](https://code.claude.com/docs/en/agent-sdk/python#thinkingconfig)
- [SdkBeta](https://code.claude.com/docs/en/agent-sdk/python#sdkbeta)
- [McpSdkServerConfig](https://code.claude.com/docs/en/agent-sdk/python#mcpsdkserverconfig)
- [McpServerConfig](https://code.claude.com/docs/en/agent-sdk/python#mcpserverconfig)
- [McpStdioServerConfig](https://code.claude.com/docs/en/agent-sdk/python#mcpstdioserverconfig)
- [McpSSEServerConfig](https://code.claude.com/docs/en/agent-sdk/python#mcpsseserverconfig)
- [McpHttpServerConfig](https://code.claude.com/docs/en/agent-sdk/python#mcphttpserverconfig)
- [McpServerStatusConfig](https://code.claude.com/docs/en/agent-sdk/python#mcpserverstatusconfig)
- [McpStatusResponse](https://code.claude.com/docs/en/agent-sdk/python#mcpstatusresponse)
- [McpServerStatus](https://code.claude.com/docs/en/agent-sdk/python#mcpserverstatus)
- [SdkPluginConfig](https://code.claude.com/docs/en/agent-sdk/python#sdkpluginconfig)
- [Message Types](https://code.claude.com/docs/en/agent-sdk/python#message-types)
- [Message](https://code.claude.com/docs/en/agent-sdk/python#message)
- [UserMessage](https://code.claude.com/docs/en/agent-sdk/python#usermessage)
- [AssistantMessage](https://code.claude.com/docs/en/agent-sdk/python#assistantmessage)
- [AssistantMessageError](https://code.claude.com/docs/en/agent-sdk/python#assistantmessageerror)
- [SystemMessage](https://code.claude.com/docs/en/agent-sdk/python#systemmessage)
- [ResultMessage](https://code.claude.com/docs/en/agent-sdk/python#resultmessage)
- [StreamEvent](https://code.claude.com/docs/en/agent-sdk/python#streamevent)
- [RateLimitEvent](https://code.claude.com/docs/en/agent-sdk/python#ratelimitevent)
- [RateLimitInfo](https://code.claude.com/docs/en/agent-sdk/python#ratelimitinfo)
- [TaskStartedMessage](https://code.claude.com/docs/en/agent-sdk/python#taskstartedmessage)
- [TaskUsage](https://code.claude.com/docs/en/agent-sdk/python#taskusage)
- [TaskProgressMessage](https://code.claude.com/docs/en/agent-sdk/python#taskprogressmessage)
- [TaskNotificationMessage](https://code.claude.com/docs/en/agent-sdk/python#tasknotificationmessage)
- [Content Block Types](https://code.claude.com/docs/en/agent-sdk/python#content-block-types)
- [ContentBlock](https://code.claude.com/docs/en/agent-sdk/python#contentblock)
- [TextBlock](https://code.claude.com/docs/en/agent-sdk/python#textblock)
- [ThinkingBlock](https://code.claude.com/docs/en/agent-sdk/python#thinkingblock)
- [ToolUseBlock](https://code.claude.com/docs/en/agent-sdk/python#tooluseblock)
- [ToolResultBlock](https://code.claude.com/docs/en/agent-sdk/python#toolresultblock)
- [Error Types](https://code.claude.com/docs/en/agent-sdk/python#error-types)
- [ClaudeSDKError](https://code.claude.com/docs/en/agent-sdk/python#claudesdkerror)
- [CLINotFoundError](https://code.claude.com/docs/en/agent-sdk/python#clinotfounderror)
- [CLIConnectionError](https://code.claude.com/docs/en/agent-sdk/python#cliconnectionerror)
- [ProcessError](https://code.claude.com/docs/en/agent-sdk/python#processerror)
- [CLIJSONDecodeError](https://code.claude.com/docs/en/agent-sdk/python#clijsondecodeerror)
- [Hook Types](https://code.claude.com/docs/en/agent-sdk/python#hook-types)
- [HookEvent](https://code.claude.com/docs/en/agent-sdk/python#hookevent)
- [HookCallback](https://code.claude.com/docs/en/agent-sdk/python#hookcallback)
- [HookContext](https://code.claude.com/docs/en/agent-sdk/python#hookcontext)
- [HookMatcher](https://code.claude.com/docs/en/agent-sdk/python#hookmatcher)
- [HookInput](https://code.claude.com/docs/en/agent-sdk/python#hookinput)
- [BaseHookInput](https://code.claude.com/docs/en/agent-sdk/python#basehookinput)
- [PreToolUseHookInput](https://code.claude.com/docs/en/agent-sdk/python#pretoolusehookinput)
- [PostToolUseHookInput](https://code.claude.com/docs/en/agent-sdk/python#posttoolusehookinput)
- [PostToolUseFailureHookInput](https://code.claude.com/docs/en/agent-sdk/python#posttoolusefailurehookinput)
- [UserPromptSubmitHookInput](https://code.claude.com/docs/en/agent-sdk/python#userpromptsubmithookinput)
- [StopHookInput](https://code.claude.com/docs/en/agent-sdk/python#stophookinput)
- [SubagentStopHookInput](https://code.claude.com/docs/en/agent-sdk/python#subagentstophookinput)
- [PreCompactHookInput](https://code.claude.com/docs/en/agent-sdk/python#precompacthookinput)
- [NotificationHookInput](https://code.claude.com/docs/en/agent-sdk/python#notificationhookinput)
- [SubagentStartHookInput](https://code.claude.com/docs/en/agent-sdk/python#subagentstarthookinput)
- [PermissionRequestHookInput](https://code.claude.com/docs/en/agent-sdk/python#permissionrequesthookinput)
- [HookJSONOutput](https://code.claude.com/docs/en/agent-sdk/python#hookjsonoutput)
- [SyncHookJSONOutput](https://code.claude.com/docs/en/agent-sdk/python#synchookjsonoutput)
- [HookSpecificOutput](https://code.claude.com/docs/en/agent-sdk/python#hookspecificoutput)
- [AsyncHookJSONOutput](https://code.claude.com/docs/en/agent-sdk/python#asynchookjsonoutput)
- [Hook Usage Example](https://code.claude.com/docs/en/agent-sdk/python#hook-usage-example)
- [Tool Input/Output Types](https://code.claude.com/docs/en/agent-sdk/python#tool-input%2Foutput-types)
- [Agent](https://code.claude.com/docs/en/agent-sdk/python#agent)
- [AskUserQuestion](https://code.claude.com/docs/en/agent-sdk/python#askuserquestion)
- [Bash](https://code.claude.com/docs/en/agent-sdk/python#bash)
- [Monitor](https://code.claude.com/docs/en/agent-sdk/python#monitor)
- [Edit](https://code.claude.com/docs/en/agent-sdk/python#edit)
- [Read](https://code.claude.com/docs/en/agent-sdk/python#read)
- [Write](https://code.claude.com/docs/en/agent-sdk/python#write)
- [Glob](https://code.claude.com/docs/en/agent-sdk/python#glob)
- [Grep](https://code.claude.com/docs/en/agent-sdk/python#grep)
- [NotebookEdit](https://code.claude.com/docs/en/agent-sdk/python#notebookedit)
- [WebFetch](https://code.claude.com/docs/en/agent-sdk/python#webfetch)
- [WebSearch](https://code.claude.com/docs/en/agent-sdk/python#websearch)
- [TodoWrite](https://code.claude.com/docs/en/agent-sdk/python#todowrite)
- [BashOutput](https://code.claude.com/docs/en/agent-sdk/python#bashoutput)
- [KillBash](https://code.claude.com/docs/en/agent-sdk/python#killbash)
- [ExitPlanMode](https://code.claude.com/docs/en/agent-sdk/python#exitplanmode)
- [ListMcpResources](https://code.claude.com/docs/en/agent-sdk/python#listmcpresources)
- [ReadMcpResource](https://code.claude.com/docs/en/agent-sdk/python#readmcpresource)
- [Advanced Features with ClaudeSDKClient](https://code.claude.com/docs/en/agent-sdk/python#advanced-features-with-claudesdkclient)
- [Building a Continuous Conversation Interface](https://code.claude.com/docs/en/agent-sdk/python#building-a-continuous-conversation-interface)
- [Using Hooks for Behavior Modification](https://code.claude.com/docs/en/agent-sdk/python#using-hooks-for-behavior-modification)
- [Real-time Progress Monitoring](https://code.claude.com/docs/en/agent-sdk/python#real-time-progress-monitoring)
- [Example Usage](https://code.claude.com/docs/en/agent-sdk/python#example-usage)
- [Basic file operations (using query)](https://code.claude.com/docs/en/agent-sdk/python#basic-file-operations-using-query)
- [Error handling](https://code.claude.com/docs/en/agent-sdk/python#error-handling)
- [Streaming mode with client](https://code.claude.com/docs/en/agent-sdk/python#streaming-mode-with-client)
- [Using custom tools with ClaudeSDKClient](https://code.claude.com/docs/en/agent-sdk/python#using-custom-tools-with-claudesdkclient)
- [Sandbox Configuration](https://code.claude.com/docs/en/agent-sdk/python#sandbox-configuration)
- [SandboxSettings](https://code.claude.com/docs/en/agent-sdk/python#sandboxsettings)
- [Example usage](https://code.claude.com/docs/en/agent-sdk/python#example-usage-2)
- [SandboxNetworkConfig](https://code.claude.com/docs/en/agent-sdk/python#sandboxnetworkconfig)
- [SandboxIgnoreViolations](https://code.claude.com/docs/en/agent-sdk/python#sandboxignoreviolations)
- [Permissions Fallback for Unsandboxed Commands](https://code.claude.com/docs/en/agent-sdk/python#permissions-fallback-for-unsandboxed-commands)
- [See also](https://code.claude.com/docs/en/agent-sdk/python#see-also)

## [​](https://code.claude.com/docs/en/agent-sdk/python\#installation)  Installation

```
pip install claude-agent-sdk
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#choosing-between-query-and-claudesdkclient)  Choosing between `query()` and `ClaudeSDKClient`

The Python SDK provides two ways to interact with Claude Code:

### [​](https://code.claude.com/docs/en/agent-sdk/python\#quick-comparison)  Quick comparison

| Feature | `query()` | `ClaudeSDKClient` |
| --- | --- | --- |
| **Session** | Creates new session each time | Reuses same session |
| **Conversation** | Single exchange | Multiple exchanges in same context |
| **Connection** | Managed automatically | Manual control |
| **Streaming Input** | ✅ Supported | ✅ Supported |
| **Interrupts** | ❌ Not supported | ✅ Supported |
| **Hooks** | ✅ Supported | ✅ Supported |
| **Custom Tools** | ✅ Supported | ✅ Supported |
| **Continue Chat** | ❌ New session each time | ✅ Maintains conversation |
| **Use Case** | One-off tasks | Continuous conversations |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#when-to-use-query-new-session-each-time)  When to use `query()` (new session each time)

**Best for:**

- One-off questions where you don’t need conversation history
- Independent tasks that don’t require context from previous exchanges
- Simple automation scripts
- When you want a fresh start each time

### [​](https://code.claude.com/docs/en/agent-sdk/python\#when-to-use-claudesdkclient-continuous-conversation)  When to use `ClaudeSDKClient` (continuous conversation)

**Best for:**

- **Continuing conversations** \- When you need Claude to remember context
- **Follow-up questions** \- Building on previous responses
- **Interactive applications** \- Chat interfaces, REPLs
- **Response-driven logic** \- When next action depends on Claude’s response
- **Session control** \- Managing conversation lifecycle explicitly

## [​](https://code.claude.com/docs/en/agent-sdk/python\#functions)  Functions

### [​](https://code.claude.com/docs/en/agent-sdk/python\#query)  `query()`

Creates a new session for each interaction with Claude Code. Returns an async iterator that yields messages as they arrive. Each call to `query()` starts fresh with no memory of previous interactions.

```
async def query(
    *,
    prompt: str | AsyncIterable[dict[str, Any]],
    options: ClaudeAgentOptions | None = None,
    transport: Transport | None = None
) -> AsyncIterator[Message]
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters)  Parameters

| Parameter | Type | Description |
| --- | --- | --- |
| `prompt` | `str | AsyncIterable[dict]` | The input prompt as a string or async iterable for streaming mode |
| `options` | `ClaudeAgentOptions | None` | Optional configuration object (defaults to `ClaudeAgentOptions()` if None) |
| `transport` | `Transport | None` | Optional custom transport for communicating with the CLI process |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#returns)  Returns

Returns an `AsyncIterator[Message]` that yields messages from the conversation.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-with-options)  Example - With options

```
import asyncio
from claude_agent_sdk import query, ClaudeAgentOptions

async def main():
    options = ClaudeAgentOptions(
        system_prompt="You are an expert Python developer",
        permission_mode="acceptEdits",
        cwd="/home/user/project",
    )

    async for message in query(prompt="Create a Python web server", options=options):
        print(message)

asyncio.run(main())
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#tool)  `tool()`

Decorator for defining MCP tools with type safety.

```
def tool(
    name: str,
    description: str,
    input_schema: type | dict[str, Any],
    annotations: ToolAnnotations | None = None
) -> Callable[[Callable[[Any], Awaitable[dict[str, Any]]]], SdkMcpTool[Any]]
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters-2)  Parameters

| Parameter | Type | Description |
| --- | --- | --- |
| `name` | `str` | Unique identifier for the tool |
| `description` | `str` | Human-readable description of what the tool does |
| `input_schema` | `type | dict[str, Any]` | Schema defining the tool’s input parameters (see below) |
| `annotations` | [`ToolAnnotations`](https://code.claude.com/docs/en/agent-sdk/python#tool-annotations)` | None` | Optional MCP tool annotations providing behavioral hints to clients |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#input-schema-options)  Input schema options

1. **Simple type mapping** (recommended):






















```
{"text": str, "count": int, "enabled": bool}
```

2. **JSON Schema format** (for complex validation):






















```
{
       "type": "object",
       "properties": {
           "text": {"type": "string"},
           "count": {"type": "integer", "minimum": 0},
       },
       "required": ["text"],
}
```


#### [​](https://code.claude.com/docs/en/agent-sdk/python\#returns-2)  Returns

A decorator function that wraps the tool implementation and returns an `SdkMcpTool` instance.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example)  Example

```
from claude_agent_sdk import tool
from typing import Any

@tool("greet", "Greet a user", {"name": str})
async def greet(args: dict[str, Any]) -> dict[str, Any]:
    return {"content": [{"type": "text", "text": f"Hello, {args['name']}!"}]}
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#toolannotations)  `ToolAnnotations`

Re-exported from `mcp.types` (also available as `from claude_agent_sdk import ToolAnnotations`). All fields are optional hints; clients should not rely on them for security decisions.

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `title` | `str | None` | `None` | Human-readable title for the tool |
| `readOnlyHint` | `bool | None` | `False` | If `True`, the tool does not modify its environment |
| `destructiveHint` | `bool | None` | `True` | If `True`, the tool may perform destructive updates (only meaningful when `readOnlyHint` is `False`) |
| `idempotentHint` | `bool | None` | `False` | If `True`, repeated calls with the same arguments have no additional effect (only meaningful when `readOnlyHint` is `False`) |
| `openWorldHint` | `bool | None` | `True` | If `True`, the tool interacts with external entities (for example, web search). If `False`, the tool’s domain is closed (for example, a memory tool) |

```
from claude_agent_sdk import tool, ToolAnnotations
from typing import Any

@tool(
    "search",
    "Search the web",
    {"query": str},
    annotations=ToolAnnotations(readOnlyHint=True, openWorldHint=True),
)
async def search(args: dict[str, Any]) -> dict[str, Any]:
    return {"content": [{"type": "text", "text": f"Results for: {args['query']}"}]}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#create_sdk_mcp_server)  `create_sdk_mcp_server()`

Create an in-process MCP server that runs within your Python application.

```
def create_sdk_mcp_server(
    name: str,
    version: str = "1.0.0",
    tools: list[SdkMcpTool[Any]] | None = None
) -> McpSdkServerConfig
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters-3)  Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `name` | `str` | - | Unique identifier for the server |
| `version` | `str` | `"1.0.0"` | Server version string |
| `tools` | `list[SdkMcpTool[Any]] | None` | `None` | List of tool functions created with `@tool` decorator |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#returns-3)  Returns

Returns an `McpSdkServerConfig` object that can be passed to `ClaudeAgentOptions.mcp_servers`.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-2)  Example

```
from claude_agent_sdk import tool, create_sdk_mcp_server

@tool("add", "Add two numbers", {"a": float, "b": float})
async def add(args):
    return {"content": [{"type": "text", "text": f"Sum: {args['a'] + args['b']}"}]}

@tool("multiply", "Multiply two numbers", {"a": float, "b": float})
async def multiply(args):
    return {"content": [{"type": "text", "text": f"Product: {args['a'] * args['b']}"}]}

calculator = create_sdk_mcp_server(
    name="calculator",
    version="2.0.0",
    tools=[add, multiply],  # Pass decorated functions
)

# Use with Claude
options = ClaudeAgentOptions(
    mcp_servers={"calc": calculator},
    allowed_tools=["mcp__calc__add", "mcp__calc__multiply"],
)
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#list_sessions)  `list_sessions()`

Lists past sessions with metadata. Filter by project directory or list sessions across all projects. Synchronous; returns immediately.

```
def list_sessions(
    directory: str | None = None,
    limit: int | None = None,
    include_worktrees: bool = True
) -> list[SDKSessionInfo]
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters-4)  Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `directory` | `str | None` | `None` | Directory to list sessions for. When omitted, returns sessions across all projects |
| `limit` | `int | None` | `None` | Maximum number of sessions to return |
| `include_worktrees` | `bool` | `True` | When `directory` is inside a git repository, include sessions from all worktree paths |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#return-type-sdksessioninfo)  Return type: `SDKSessionInfo`

| Property | Type | Description |
| --- | --- | --- |
| `session_id` | `str` | Unique session identifier |
| `summary` | `str` | Display title: custom title, auto-generated summary, or first prompt |
| `last_modified` | `int` | Last modified time in milliseconds since epoch |
| `file_size` | `int | None` | Session file size in bytes (`None` for remote storage backends) |
| `custom_title` | `str | None` | User-set session title |
| `first_prompt` | `str | None` | First meaningful user prompt in the session |
| `git_branch` | `str | None` | Git branch at the end of the session |
| `cwd` | `str | None` | Working directory for the session |
| `tag` | `str | None` | User-set session tag (see [`tag_session()`](https://code.claude.com/docs/en/agent-sdk/python#tag-session)) |
| `created_at` | `int | None` | Session creation time in milliseconds since epoch |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-3)  Example

Print the 10 most recent sessions for a project. Results are sorted by `last_modified` descending, so the first item is the newest. Omit `directory` to search across all projects.

```
from claude_agent_sdk import list_sessions

for session in list_sessions(directory="/path/to/project", limit=10):
    print(f"{session.summary} ({session.session_id})")
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#get_session_messages)  `get_session_messages()`

Retrieves messages from a past session. Synchronous; returns immediately.

```
def get_session_messages(
    session_id: str,
    directory: str | None = None,
    limit: int | None = None,
    offset: int = 0
) -> list[SessionMessage]
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters-5)  Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `session_id` | `str` | required | The session ID to retrieve messages for |
| `directory` | `str | None` | `None` | Project directory to look in. When omitted, searches all projects |
| `limit` | `int | None` | `None` | Maximum number of messages to return |
| `offset` | `int` | `0` | Number of messages to skip from the start |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#return-type-sessionmessage)  Return type: `SessionMessage`

| Property | Type | Description |
| --- | --- | --- |
| `type` | `Literal["user", "assistant"]` | Message role |
| `uuid` | `str` | Unique message identifier |
| `session_id` | `str` | Session identifier |
| `message` | `Any` | Raw message content |
| `parent_tool_use_id` | `None` | Reserved for future use |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-4)  Example

```
from claude_agent_sdk import list_sessions, get_session_messages

sessions = list_sessions(limit=1)
if sessions:
    messages = get_session_messages(sessions[0].session_id)
    for msg in messages:
        print(f"[{msg.type}] {msg.uuid}")
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#get_session_info)  `get_session_info()`

Reads metadata for a single session by ID without scanning the full project directory. Synchronous; returns immediately.

```
def get_session_info(
    session_id: str,
    directory: str | None = None,
) -> SDKSessionInfo | None
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters-6)  Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `session_id` | `str` | required | UUID of the session to look up |
| `directory` | `str | None` | `None` | Project directory path. When omitted, searches all project directories |

Returns [`SDKSessionInfo`](https://code.claude.com/docs/en/agent-sdk/python#return-type-sdk-session-info), or `None` if the session is not found.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-5)  Example

Look up a single session’s metadata without scanning the project directory. Useful when you already have a session ID from a previous run.

```
from claude_agent_sdk import get_session_info

info = get_session_info("550e8400-e29b-41d4-a716-446655440000")
if info:
    print(f"{info.summary} (branch: {info.git_branch}, tag: {info.tag})")
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#rename_session)  `rename_session()`

Renames a session by appending a custom-title entry. Repeated calls are safe; the most recent title wins. Synchronous.

```
def rename_session(
    session_id: str,
    title: str,
    directory: str | None = None,
) -> None
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters-7)  Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `session_id` | `str` | required | UUID of the session to rename |
| `title` | `str` | required | New title. Must be non-empty after stripping whitespace |
| `directory` | `str | None` | `None` | Project directory path. When omitted, searches all project directories |

Raises `ValueError` if `session_id` is not a valid UUID or `title` is empty; `FileNotFoundError` if the session cannot be found.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-6)  Example

Rename the most recent session so it’s easier to find later. The new title appears in [`SDKSessionInfo.custom_title`](https://code.claude.com/docs/en/agent-sdk/python#return-type-sdk-session-info) on subsequent reads.

```
from claude_agent_sdk import list_sessions, rename_session

sessions = list_sessions(directory="/path/to/project", limit=1)
if sessions:
    rename_session(sessions[0].session_id, "Refactor auth module")
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#tag_session)  `tag_session()`

Tags a session. Pass `None` to clear the tag. Repeated calls are safe; the most recent tag wins. Synchronous.

```
def tag_session(
    session_id: str,
    tag: str | None,
    directory: str | None = None,
) -> None
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#parameters-8)  Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `session_id` | `str` | required | UUID of the session to tag |
| `tag` | `str | None` | required | Tag string, or `None` to clear. Unicode-sanitized before storing |
| `directory` | `str | None` | `None` | Project directory path. When omitted, searches all project directories |

Raises `ValueError` if `session_id` is not a valid UUID or `tag` is empty after sanitization; `FileNotFoundError` if the session cannot be found.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-7)  Example

Tag a session, then filter by that tag on a later read. Pass `None` to clear an existing tag.

```
from claude_agent_sdk import list_sessions, tag_session

# Tag a session
tag_session("550e8400-e29b-41d4-a716-446655440000", "needs-review")

# Later: find all sessions with that tag
for session in list_sessions(directory="/path/to/project"):
    if session.tag == "needs-review":
        print(session.summary)
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#classes)  Classes

### [​](https://code.claude.com/docs/en/agent-sdk/python\#claudesdkclient)  `ClaudeSDKClient`

**Maintains a conversation session across multiple exchanges.** This is the Python equivalent of how the TypeScript SDK’s `query()` function works internally - it creates a client object that can continue conversations.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#key-features)  Key Features

- **Session continuity**: Maintains conversation context across multiple `query()` calls
- **Same conversation**: The session retains previous messages
- **Interrupt support**: Can stop execution mid-task
- **Explicit lifecycle**: You control when the session starts and ends
- **Response-driven flow**: Can react to responses and send follow-ups
- **Custom tools and hooks**: Supports custom tools (created with `@tool` decorator) and hooks

```
class ClaudeSDKClient:
    def __init__(self, options: ClaudeAgentOptions | None = None, transport: Transport | None = None)
    async def connect(self, prompt: str | AsyncIterable[dict] | None = None) -> None
    async def query(self, prompt: str | AsyncIterable[dict], session_id: str = "default") -> None
    async def receive_messages(self) -> AsyncIterator[Message]
    async def receive_response(self) -> AsyncIterator[Message]
    async def interrupt(self) -> None
    async def set_permission_mode(self, mode: str) -> None
    async def set_model(self, model: str | None = None) -> None
    async def rewind_files(self, user_message_id: str) -> None
    async def get_mcp_status(self) -> McpStatusResponse
    async def reconnect_mcp_server(self, server_name: str) -> None
    async def toggle_mcp_server(self, server_name: str, enabled: bool) -> None
    async def stop_task(self, task_id: str) -> None
    async def get_server_info(self) -> dict[str, Any] | None
    async def disconnect(self) -> None
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#methods)  Methods

| Method | Description |
| --- | --- |
| `__init__(options)` | Initialize the client with optional configuration |
| `connect(prompt)` | Connect to Claude with an optional initial prompt or message stream |
| `query(prompt, session_id)` | Send a new request in streaming mode |
| `receive_messages()` | Receive all messages from Claude as an async iterator |
| `receive_response()` | Receive messages until and including a ResultMessage |
| `interrupt()` | Send interrupt signal (only works in streaming mode) |
| `set_permission_mode(mode)` | Change the permission mode for the current session |
| `set_model(model)` | Change the model for the current session. Pass `None` to reset to default |
| `rewind_files(user_message_id)` | Restore files to their state at the specified user message. Requires `enable_file_checkpointing=True`. See [File checkpointing](https://code.claude.com/docs/en/agent-sdk/file-checkpointing) |
| `get_mcp_status()` | Get the status of all configured MCP servers. Returns [`McpStatusResponse`](https://code.claude.com/docs/en/agent-sdk/python#mcp-status-response) |
| `reconnect_mcp_server(server_name)` | Retry connecting to an MCP server that failed or was disconnected |
| `toggle_mcp_server(server_name, enabled)` | Enable or disable an MCP server mid-session. Disabling removes its tools |
| `stop_task(task_id)` | Stop a running background task. A [`TaskNotificationMessage`](https://code.claude.com/docs/en/agent-sdk/python#task-notification-message) with status `"stopped"` follows in the message stream |
| `get_server_info()` | Get server information including session ID and capabilities |
| `disconnect()` | Disconnect from Claude |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#context-manager-support)  Context Manager Support

The client can be used as an async context manager for automatic connection management:

```
async with ClaudeSDKClient() as client:
    await client.query("Hello Claude")
    async for message in client.receive_response():
        print(message)
```

> **Important:** When iterating over messages, avoid using `break` to exit early as this can cause asyncio cleanup issues. Instead, let the iteration complete naturally or use flags to track when you’ve found what you need.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-continuing-a-conversation)  Example - Continuing a conversation

```
import asyncio
from claude_agent_sdk import ClaudeSDKClient, AssistantMessage, TextBlock, ResultMessage

async def main():
    async with ClaudeSDKClient() as client:
        # First question
        await client.query("What's the capital of France?")

        # Process response
        async for message in client.receive_response():
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        print(f"Claude: {block.text}")

        # Follow-up question - the session retains the previous context
        await client.query("What's the population of that city?")

        async for message in client.receive_response():
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        print(f"Claude: {block.text}")

        # Another follow-up - still in the same conversation
        await client.query("What are some famous landmarks there?")

        async for message in client.receive_response():
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        print(f"Claude: {block.text}")

asyncio.run(main())
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-streaming-input-with-claudesdkclient)  Example - Streaming input with ClaudeSDKClient

```
import asyncio
from claude_agent_sdk import ClaudeSDKClient

async def message_stream():
    """Generate messages dynamically."""
    yield {
        "type": "user",
        "message": {"role": "user", "content": "Analyze the following data:"},
    }
    await asyncio.sleep(0.5)
    yield {
        "type": "user",
        "message": {"role": "user", "content": "Temperature: 25°C, Humidity: 60%"},
    }
    await asyncio.sleep(0.5)
    yield {
        "type": "user",
        "message": {"role": "user", "content": "What patterns do you see?"},
    }

async def main():
    async with ClaudeSDKClient() as client:
        # Stream input to Claude
        await client.query(message_stream())

        # Process response
        async for message in client.receive_response():
            print(message)

        # Follow-up in same session
        await client.query("Should we be concerned about these readings?")

        async for message in client.receive_response():
            print(message)

asyncio.run(main())
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-using-interrupts)  Example - Using interrupts

```
import asyncio
from claude_agent_sdk import ClaudeSDKClient, ClaudeAgentOptions, ResultMessage

async def interruptible_task():
    options = ClaudeAgentOptions(allowed_tools=["Bash"], permission_mode="acceptEdits")

    async with ClaudeSDKClient(options=options) as client:
        # Start a long-running task
        await client.query("Count from 1 to 100 slowly, using the bash sleep command")

        # Let it run for a bit
        await asyncio.sleep(2)

        # Interrupt the task
        await client.interrupt()
        print("Task interrupted!")

        # Drain the interrupted task's messages (including its ResultMessage)
        async for message in client.receive_response():
            if isinstance(message, ResultMessage):
                print(f"Interrupted task finished with subtype={message.subtype!r}")
                # subtype is "error_during_execution" for interrupted tasks

        # Send a new command
        await client.query("Just say hello instead")

        # Now receive the new response
        async for message in client.receive_response():
            if isinstance(message, ResultMessage) and message.subtype == "success":
                print(f"New result: {message.result}")

asyncio.run(interruptible_task())
```

**Buffer behavior after interrupt:**`interrupt()` sends a stop signal but does not clear the message buffer. Messages already produced by the interrupted task, including its `ResultMessage` (with `subtype="error_during_execution"`), remain in the stream. You must drain them with `receive_response()` before reading the response to a new query. If you send a new query immediately after `interrupt()` and call `receive_response()` only once, you’ll receive the interrupted task’s messages, not the new query’s response.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-advanced-permission-control)  Example - Advanced permission control

```
from claude_agent_sdk import ClaudeSDKClient, ClaudeAgentOptions
from claude_agent_sdk.types import (
    PermissionResultAllow,
    PermissionResultDeny,
    ToolPermissionContext,
)

async def custom_permission_handler(
    tool_name: str, input_data: dict, context: ToolPermissionContext
) -> PermissionResultAllow | PermissionResultDeny:
    """Custom logic for tool permissions."""

    # Block writes to system directories
    if tool_name == "Write" and input_data.get("file_path", "").startswith("/system/"):
        return PermissionResultDeny(
            message="System directory write not allowed", interrupt=True
        )

    # Redirect sensitive file operations
    if tool_name in ["Write", "Edit"] and "config" in input_data.get("file_path", ""):
        safe_path = f"./sandbox/{input_data['file_path']}"
        return PermissionResultAllow(
            updated_input={**input_data, "file_path": safe_path}
        )

    # Allow everything else
    return PermissionResultAllow(updated_input=input_data)

async def main():
    options = ClaudeAgentOptions(
        can_use_tool=custom_permission_handler, allowed_tools=["Read", "Write", "Edit"]
    )

    async with ClaudeSDKClient(options=options) as client:
        await client.query("Update the system config file")

        async for message in client.receive_response():
            # Will use sandbox path instead
            print(message)

asyncio.run(main())
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#types)  Types

**`@dataclass` vs `TypedDict`:** This SDK uses two kinds of types. Classes decorated with `@dataclass` (such as `ResultMessage`, `AgentDefinition`, `TextBlock`) are object instances at runtime and support attribute access: `msg.result`. Classes defined with `TypedDict` (such as `ThinkingConfigEnabled`, `McpStdioServerConfig`, `SyncHookJSONOutput`) are **plain dicts at runtime** and require key access: `config["budget_tokens"]`, not `config.budget_tokens`. The `ClassName(field=value)` call syntax works for both, but only dataclasses produce objects with attributes.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#sdkmcptool)  `SdkMcpTool`

Definition for an SDK MCP tool created with the `@tool` decorator.

```
@dataclass
class SdkMcpTool(Generic[T]):
    name: str
    description: str
    input_schema: type[T] | dict[str, Any]
    handler: Callable[[T], Awaitable[dict[str, Any]]]
    annotations: ToolAnnotations | None = None
```

| Property | Type | Description |
| --- | --- | --- |
| `name` | `str` | Unique identifier for the tool |
| `description` | `str` | Human-readable description |
| `input_schema` | `type[T] | dict[str, Any]` | Schema for input validation |
| `handler` | `Callable[[T], Awaitable[dict[str, Any]]]` | Async function that handles tool execution |
| `annotations` | `ToolAnnotations | None` | Optional MCP tool annotations (e.g., `readOnlyHint`, `destructiveHint`, `openWorldHint`). From `mcp.types` |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#transport)  `Transport`

Abstract base class for custom transport implementations. Use this to communicate with the Claude process over a custom channel (for example, a remote connection instead of a local subprocess).

This is a low-level internal API. The interface may change in future releases. Custom implementations must be updated to match any interface changes.

```
from abc import ABC, abstractmethod
from collections.abc import AsyncIterator
from typing import Any

class Transport(ABC):
    @abstractmethod
    async def connect(self) -> None: ...

    @abstractmethod
    async def write(self, data: str) -> None: ...

    @abstractmethod
    def read_messages(self) -> AsyncIterator[dict[str, Any]]: ...

    @abstractmethod
    async def close(self) -> None: ...

    @abstractmethod
    def is_ready(self) -> bool: ...

    @abstractmethod
    async def end_input(self) -> None: ...
```

| Method | Description |
| --- | --- |
| `connect()` | Connect the transport and prepare for communication |
| `write(data)` | Write raw data (JSON + newline) to the transport |
| `read_messages()` | Async iterator that yields parsed JSON messages |
| `close()` | Close the connection and clean up resources |
| `is_ready()` | Returns `True` if the transport can send and receive |
| `end_input()` | Close the input stream (for example, close stdin for subprocess transports) |

Import: `from claude_agent_sdk import Transport`

### [​](https://code.claude.com/docs/en/agent-sdk/python\#claudeagentoptions)  `ClaudeAgentOptions`

Configuration dataclass for Claude Code queries.

```
@dataclass
class ClaudeAgentOptions:
    tools: list[str] | ToolsPreset | None = None
    allowed_tools: list[str] = field(default_factory=list)
    system_prompt: str | SystemPromptPreset | None = None
    mcp_servers: dict[str, McpServerConfig] | str | Path = field(default_factory=dict)
    permission_mode: PermissionMode | None = None
    continue_conversation: bool = False
    resume: str | None = None
    max_turns: int | None = None
    max_budget_usd: float | None = None
    disallowed_tools: list[str] = field(default_factory=list)
    model: str | None = None
    fallback_model: str | None = None
    betas: list[SdkBeta] = field(default_factory=list)
    output_format: dict[str, Any] | None = None
    permission_prompt_tool_name: str | None = None
    cwd: str | Path | None = None
    cli_path: str | Path | None = None
    settings: str | None = None
    add_dirs: list[str | Path] = field(default_factory=list)
    env: dict[str, str] = field(default_factory=dict)
    extra_args: dict[str, str | None] = field(default_factory=dict)
    max_buffer_size: int | None = None
    debug_stderr: Any = sys.stderr  # Deprecated
    stderr: Callable[[str], None] | None = None
    can_use_tool: CanUseTool | None = None
    hooks: dict[HookEvent, list[HookMatcher]] | None = None
    user: str | None = None
    include_partial_messages: bool = False
    fork_session: bool = False
    agents: dict[str, AgentDefinition] | None = None
    setting_sources: list[SettingSource] | None = None
    sandbox: SandboxSettings | None = None
    plugins: list[SdkPluginConfig] = field(default_factory=list)
    max_thinking_tokens: int | None = None  # Deprecated: use thinking instead
    thinking: ThinkingConfig | None = None
    effort: Literal["low", "medium", "high", "xhigh", "max"] | None = None
    enable_file_checkpointing: bool = False
```

| Property | Type | Default | Description |
| --- | --- | --- | --- |
| `tools` | `list[str] | ToolsPreset | None` | `None` | Tools configuration. Use `{"type": "preset", "preset": "claude_code"}` for Claude Code’s default tools |
| `allowed_tools` | `list[str]` | `[]` | Tools to auto-approve without prompting. This does not restrict Claude to only these tools; unlisted tools fall through to `permission_mode` and `can_use_tool`. Use `disallowed_tools` to block tools. See [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions#allow-and-deny-rules) |
| `system_prompt` | `str | SystemPromptPreset | None` | `None` | System prompt configuration. Pass a string for custom prompt, or use `{"type": "preset", "preset": "claude_code"}` for Claude Code’s system prompt. Add `"append"` to extend the preset |
| `mcp_servers` | `dict[str, McpServerConfig] | str | Path` | `{}` | MCP server configurations or path to config file |
| `permission_mode` | `PermissionMode | None` | `None` | Permission mode for tool usage |
| `continue_conversation` | `bool` | `False` | Continue the most recent conversation |
| `resume` | `str | None` | `None` | Session ID to resume |
| `max_turns` | `int | None` | `None` | Maximum agentic turns (tool-use round trips) |
| `max_budget_usd` | `float | None` | `None` | Stop the query when the client-side cost estimate reaches this USD value. Compared against the same estimate as `total_cost_usd`; see [Track cost and usage](https://code.claude.com/docs/en/agent-sdk/cost-tracking) for accuracy caveats |
| `disallowed_tools` | `list[str]` | `[]` | Tools to always deny. Deny rules are checked first and override `allowed_tools` and `permission_mode` (including `bypassPermissions`) |
| `enable_file_checkpointing` | `bool` | `False` | Enable file change tracking for rewinding. See [File checkpointing](https://code.claude.com/docs/en/agent-sdk/file-checkpointing) |
| `model` | `str | None` | `None` | Claude model to use |
| `fallback_model` | `str | None` | `None` | Fallback model to use if the primary model fails |
| `betas` | `list[SdkBeta]` | `[]` | Beta features to enable. See [`SdkBeta`](https://code.claude.com/docs/en/agent-sdk/python#sdk-beta) for available options |
| `output_format` | `dict[str, Any] | None` | `None` | Output format for structured responses (e.g., `{"type": "json_schema", "schema": {...}}`). See [Structured outputs](https://code.claude.com/docs/en/agent-sdk/structured-outputs) for details |
| `permission_prompt_tool_name` | `str | None` | `None` | MCP tool name for permission prompts |
| `cwd` | `str | Path | None` | `None` | Current working directory |
| `cli_path` | `str | Path | None` | `None` | Custom path to the Claude Code CLI executable |
| `settings` | `str | None` | `None` | Path to settings file |
| `add_dirs` | `list[str | Path]` | `[]` | Additional directories Claude can access |
| `env` | `dict[str, str]` | `{}` | Environment variables |
| `extra_args` | `dict[str, str | None]` | `{}` | Additional CLI arguments to pass directly to the CLI |
| `max_buffer_size` | `int | None` | `None` | Maximum bytes when buffering CLI stdout |
| `debug_stderr` | `Any` | `sys.stderr` | _Deprecated_ \- File-like object for debug output. Use `stderr` callback instead |
| `stderr` | `Callable[[str], None] | None` | `None` | Callback function for stderr output from CLI |
| `can_use_tool` | [`CanUseTool`](https://code.claude.com/docs/en/agent-sdk/python#can-use-tool)` | None` | `None` | Tool permission callback function. See [Permission types](https://code.claude.com/docs/en/agent-sdk/python#can-use-tool) for details |
| `hooks` | `dict[HookEvent, list[HookMatcher]] | None` | `None` | Hook configurations for intercepting events |
| `user` | `str | None` | `None` | User identifier |
| `include_partial_messages` | `bool` | `False` | Include partial message streaming events. When enabled, [`StreamEvent`](https://code.claude.com/docs/en/agent-sdk/python#stream-event) messages are yielded |
| `fork_session` | `bool` | `False` | When resuming with `resume`, fork to a new session ID instead of continuing the original session |
| `agents` | `dict[str, AgentDefinition] | None` | `None` | Programmatically defined subagents |
| `plugins` | `list[SdkPluginConfig]` | `[]` | Load custom plugins from local paths. See [Plugins](https://code.claude.com/docs/en/agent-sdk/plugins) for details |
| `sandbox` | [`SandboxSettings`](https://code.claude.com/docs/en/agent-sdk/python#sandbox-settings)` | None` | `None` | Configure sandbox behavior programmatically. See [Sandbox settings](https://code.claude.com/docs/en/agent-sdk/python#sandbox-settings) for details |
| `setting_sources` | `list[SettingSource] | None` | `None` (CLI defaults: all sources) | Control which filesystem settings to load. Pass `[]` to disable user, project, and local settings. Managed policy settings load regardless. See [Use Claude Code features](https://code.claude.com/docs/en/agent-sdk/claude-code-features#what-settingsources-does-not-control) |
| `max_thinking_tokens` | `int | None` | `None` | _Deprecated_ \- Maximum tokens for thinking blocks. Use `thinking` instead |
| `thinking` | [`ThinkingConfig`](https://code.claude.com/docs/en/agent-sdk/python#thinking-config)` | None` | `None` | Controls extended thinking behavior. Takes precedence over `max_thinking_tokens` |
| `effort` | `Literal["low", "medium", "high", "xhigh", "max"] | None` | `None` | Effort level for thinking depth |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#outputformat)  `OutputFormat`

Configuration for structured output validation. Pass this as a `dict` to the `output_format` field on `ClaudeAgentOptions`:

```
# Expected dict shape for output_format
{
    "type": "json_schema",
    "schema": {...},  # Your JSON Schema definition
}
```

| Field | Required | Description |
| --- | --- | --- |
| `type` | Yes | Must be `"json_schema"` for JSON Schema validation |
| `schema` | Yes | JSON Schema definition for output validation |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#systempromptpreset)  `SystemPromptPreset`

Configuration for using Claude Code’s preset system prompt with optional additions.

```
class SystemPromptPreset(TypedDict):
    type: Literal["preset"]
    preset: Literal["claude_code"]
    append: NotRequired[str]
    exclude_dynamic_sections: NotRequired[bool]
```

| Field | Required | Description |
| --- | --- | --- |
| `type` | Yes | Must be `"preset"` to use a preset system prompt |
| `preset` | Yes | Must be `"claude_code"` to use Claude Code’s system prompt |
| `append` | No | Additional instructions to append to the preset system prompt |
| `exclude_dynamic_sections` | No | Move per-session context such as working directory, git status, and memory paths from the system prompt into the first user message. Improves prompt-cache reuse across users and machines. See [Modify system prompts](https://code.claude.com/docs/en/agent-sdk/modifying-system-prompts#improve-prompt-caching-across-users-and-machines) |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#settingsource)  `SettingSource`

Controls which filesystem-based configuration sources the SDK loads settings from.

```
SettingSource = Literal["user", "project", "local"]
```

| Value | Description | Location |
| --- | --- | --- |
| `"user"` | Global user settings | `~/.claude/settings.json` |
| `"project"` | Shared project settings (version controlled) | `.claude/settings.json` |
| `"local"` | Local project settings (gitignored) | `.claude/settings.local.json` |

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#default-behavior)  Default behavior

When `setting_sources` is omitted or `None`, `query()` loads the same filesystem settings as the Claude Code CLI: user, project, and local. Managed policy settings are loaded in all cases. See [What settingSources does not control](https://code.claude.com/docs/en/agent-sdk/claude-code-features#what-settingsources-does-not-control) for inputs that are read regardless of this option, and how to disable them.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#why-use-setting_sources)  Why use setting\_sources

**Disable filesystem settings:**

```
# Do not load user, project, or local settings from disk
from claude_agent_sdk import query, ClaudeAgentOptions

async for message in query(
    prompt="Analyze this code",
    options=ClaudeAgentOptions(
        setting_sources=[]
    ),
):
    print(message)
```

In Python SDK 0.1.59 and earlier, an empty list was treated the same as omitting the option, so `setting_sources=[]` did not disable filesystem settings. Upgrade to a newer release if you need an empty list to take effect. The TypeScript SDK is not affected.

**Load all filesystem settings explicitly:**

```
from claude_agent_sdk import query, ClaudeAgentOptions

async for message in query(
    prompt="Analyze this code",
    options=ClaudeAgentOptions(
        setting_sources=["user", "project", "local"]
    ),
):
    print(message)
```

**Load only specific setting sources:**

```
# Load only project settings, ignore user and local
async for message in query(
    prompt="Run CI checks",
    options=ClaudeAgentOptions(
        setting_sources=["project"]  # Only .claude/settings.json
    ),
):
    print(message)
```

**Testing and CI environments:**

```
# Ensure consistent behavior in CI by excluding local settings
async for message in query(
    prompt="Run tests",
    options=ClaudeAgentOptions(
        setting_sources=["project"],  # Only team-shared settings
        permission_mode="bypassPermissions",
    ),
):
    print(message)
```

**SDK-only applications:**

```
# Define everything programmatically.
# Pass [] to opt out of filesystem setting sources.
async for message in query(
    prompt="Review this PR",
    options=ClaudeAgentOptions(
        setting_sources=[],
        agents={...},
        mcp_servers={...},
        allowed_tools=["Read", "Grep", "Glob"],
    ),
):
    print(message)
```

**Loading CLAUDE.md project instructions:**

```
# Load project settings to include CLAUDE.md files
async for message in query(
    prompt="Add a new feature following project conventions",
    options=ClaudeAgentOptions(
        system_prompt={
            "type": "preset",
            "preset": "claude_code",  # Use Claude Code's system prompt
        },
        setting_sources=["project"],  # Loads CLAUDE.md from project
        allowed_tools=["Read", "Write", "Edit"],
    ),
):
    print(message)
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#settings-precedence)  Settings precedence

When multiple sources are loaded, settings are merged with this precedence (highest to lowest):

1. Local settings (`.claude/settings.local.json`)
2. Project settings (`.claude/settings.json`)
3. User settings (`~/.claude/settings.json`)

Programmatic options such as `agents` and `allowed_tools` override user, project, and local filesystem settings. Managed policy settings take precedence over programmatic options.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#agentdefinition)  `AgentDefinition`

Configuration for a subagent defined programmatically.

```
@dataclass
class AgentDefinition:
    description: str
    prompt: str
    tools: list[str] | None = None
    model: Literal["sonnet", "opus", "haiku", "inherit"] | None = None
    skills: list[str] | None = None
    memory: Literal["user", "project", "local"] | None = None
    mcpServers: list[str | dict[str, Any]] | None = None
```

| Field | Required | Description |
| --- | --- | --- |
| `description` | Yes | Natural language description of when to use this agent |
| `prompt` | Yes | The agent’s system prompt |
| `tools` | No | Array of allowed tool names. If omitted, inherits all tools |
| `model` | No | Model override for this agent. If omitted, uses the main model |
| `skills` | No | List of skill names available to this agent |
| `memory` | No | Memory source for this agent: `"user"`, `"project"`, or `"local"` |
| `mcpServers` | No | MCP servers available to this agent. Each entry is a server name or an inline `{name: config}` dict |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissionmode)  `PermissionMode`

Permission modes for controlling tool execution.

```
PermissionMode = Literal[\
    "default",  # Standard permission behavior\
    "acceptEdits",  # Auto-accept file edits\
    "plan",  # Planning mode - no execution\
    "dontAsk",  # Deny anything not pre-approved instead of prompting\
    "bypassPermissions",  # Bypass all permission checks (use with caution)\
]
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#canusetool)  `CanUseTool`

Type alias for tool permission callback functions.

```
CanUseTool = Callable[\
    [str, dict[str, Any], ToolPermissionContext], Awaitable[PermissionResult]\
]
```

The callback receives:

- `tool_name`: Name of the tool being called
- `input_data`: The tool’s input parameters
- `context`: A `ToolPermissionContext` with additional information

Returns a `PermissionResult` (either `PermissionResultAllow` or `PermissionResultDeny`).

### [​](https://code.claude.com/docs/en/agent-sdk/python\#toolpermissioncontext)  `ToolPermissionContext`

Context information passed to tool permission callbacks.

```
@dataclass
class ToolPermissionContext:
    signal: Any | None = None  # Future: abort signal support
    suggestions: list[PermissionUpdate] = field(default_factory=list)
```

| Field | Type | Description |
| --- | --- | --- |
| `signal` | `Any | None` | Reserved for future abort signal support |
| `suggestions` | `list[PermissionUpdate]` | Permission update suggestions from the CLI |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissionresult)  `PermissionResult`

Union type for permission callback results.

```
PermissionResult = PermissionResultAllow | PermissionResultDeny
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissionresultallow)  `PermissionResultAllow`

Result indicating the tool call should be allowed.

```
@dataclass
class PermissionResultAllow:
    behavior: Literal["allow"] = "allow"
    updated_input: dict[str, Any] | None = None
    updated_permissions: list[PermissionUpdate] | None = None
```

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `behavior` | `Literal["allow"]` | `"allow"` | Must be “allow” |
| `updated_input` | `dict[str, Any] | None` | `None` | Modified input to use instead of original |
| `updated_permissions` | `list[PermissionUpdate] | None` | `None` | Permission updates to apply |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissionresultdeny)  `PermissionResultDeny`

Result indicating the tool call should be denied.

```
@dataclass
class PermissionResultDeny:
    behavior: Literal["deny"] = "deny"
    message: str = ""
    interrupt: bool = False
```

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `behavior` | `Literal["deny"]` | `"deny"` | Must be “deny” |
| `message` | `str` | `""` | Message explaining why the tool was denied |
| `interrupt` | `bool` | `False` | Whether to interrupt the current execution |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissionupdate)  `PermissionUpdate`

Configuration for updating permissions programmatically.

```
@dataclass
class PermissionUpdate:
    type: Literal[\
        "addRules",\
        "replaceRules",\
        "removeRules",\
        "setMode",\
        "addDirectories",\
        "removeDirectories",\
    ]
    rules: list[PermissionRuleValue] | None = None
    behavior: Literal["allow", "deny", "ask"] | None = None
    mode: PermissionMode | None = None
    directories: list[str] | None = None
    destination: (
        Literal["userSettings", "projectSettings", "localSettings", "session"] | None
    ) = None
```

| Field | Type | Description |
| --- | --- | --- |
| `type` | `Literal[...]` | The type of permission update operation |
| `rules` | `list[PermissionRuleValue] | None` | Rules for add/replace/remove operations |
| `behavior` | `Literal["allow", "deny", "ask"] | None` | Behavior for rule-based operations |
| `mode` | `PermissionMode | None` | Mode for setMode operation |
| `directories` | `list[str] | None` | Directories for add/remove directory operations |
| `destination` | `Literal[...] | None` | Where to apply the permission update |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissionrulevalue)  `PermissionRuleValue`

A rule to add, replace, or remove in a permission update.

```
@dataclass
class PermissionRuleValue:
    tool_name: str
    rule_content: str | None = None
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#toolspreset)  `ToolsPreset`

Preset tools configuration for using Claude Code’s default tool set.

```
class ToolsPreset(TypedDict):
    type: Literal["preset"]
    preset: Literal["claude_code"]
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#thinkingconfig)  `ThinkingConfig`

Controls extended thinking behavior. A union of three configurations:

```
class ThinkingConfigAdaptive(TypedDict):
    type: Literal["adaptive"]

class ThinkingConfigEnabled(TypedDict):
    type: Literal["enabled"]
    budget_tokens: int

class ThinkingConfigDisabled(TypedDict):
    type: Literal["disabled"]

ThinkingConfig = ThinkingConfigAdaptive | ThinkingConfigEnabled | ThinkingConfigDisabled
```

| Variant | Fields | Description |
| --- | --- | --- |
| `adaptive` | `type` | Claude adaptively decides when to think |
| `enabled` | `type`, `budget_tokens` | Enable thinking with a specific token budget |
| `disabled` | `type` | Disable thinking |

Because these are `TypedDict` classes, they’re plain dicts at runtime. Either construct them as dict literals or call the class like a constructor; both produce a `dict`. Access fields with `config["budget_tokens"]`, not `config.budget_tokens`:

```
from claude_agent_sdk import ClaudeAgentOptions, ThinkingConfigEnabled

# Option 1: dict literal (recommended, no import needed)
options = ClaudeAgentOptions(thinking={"type": "enabled", "budget_tokens": 20000})

# Option 2: constructor-style (returns a plain dict)
config = ThinkingConfigEnabled(type="enabled", budget_tokens=20000)
print(config["budget_tokens"])  # 20000
# config.budget_tokens would raise AttributeError
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#sdkbeta)  `SdkBeta`

Literal type for SDK beta features.

```
SdkBeta = Literal["context-1m-2025-08-07"]
```

Use with the `betas` field in `ClaudeAgentOptions` to enable beta features.

The `context-1m-2025-08-07` beta is retired as of April 30, 2026. Passing this header with Claude Sonnet 4.5 or Sonnet 4 has no effect, and requests that exceed the standard 200k-token context window return an error. To use a 1M-token context window, migrate to [Claude Sonnet 4.6, Claude Opus 4.6, or Claude Opus 4.7](https://platform.claude.com/docs/en/about-claude/models/overview), which include 1M context at standard pricing with no beta header required.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcpsdkserverconfig)  `McpSdkServerConfig`

Configuration for SDK MCP servers created with `create_sdk_mcp_server()`.

```
class McpSdkServerConfig(TypedDict):
    type: Literal["sdk"]
    name: str
    instance: Any  # MCP Server instance
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcpserverconfig)  `McpServerConfig`

Union type for MCP server configurations.

```
McpServerConfig = (
    McpStdioServerConfig | McpSSEServerConfig | McpHttpServerConfig | McpSdkServerConfig
)
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcpstdioserverconfig)  `McpStdioServerConfig`

```
class McpStdioServerConfig(TypedDict):
    type: NotRequired[Literal["stdio"]]  # Optional for backwards compatibility
    command: str
    args: NotRequired[list[str]]
    env: NotRequired[dict[str, str]]
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcpsseserverconfig)  `McpSSEServerConfig`

```
class McpSSEServerConfig(TypedDict):
    type: Literal["sse"]
    url: str
    headers: NotRequired[dict[str, str]]
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcphttpserverconfig)  `McpHttpServerConfig`

```
class McpHttpServerConfig(TypedDict):
    type: Literal["http"]
    url: str
    headers: NotRequired[dict[str, str]]
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcpserverstatusconfig)  `McpServerStatusConfig`

The configuration of an MCP server as reported by [`get_mcp_status()`](https://code.claude.com/docs/en/agent-sdk/python#methods). This is the union of all [`McpServerConfig`](https://code.claude.com/docs/en/agent-sdk/python#mcp-server-config) transport variants plus an output-only `claudeai-proxy` variant for servers proxied through claude.ai.

```
McpServerStatusConfig = (
    McpStdioServerConfig
    | McpSSEServerConfig
    | McpHttpServerConfig
    | McpSdkServerConfigStatus
    | McpClaudeAIProxyServerConfig
)
```

`McpSdkServerConfigStatus` is the serializable form of [`McpSdkServerConfig`](https://code.claude.com/docs/en/agent-sdk/python#mcp-sdk-server-config) with only `type` (`"sdk"`) and `name` (`str`) fields; the in-process `instance` is omitted. `McpClaudeAIProxyServerConfig` has `type` (`"claudeai-proxy"`), `url` (`str`), and `id` (`str`) fields.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcpstatusresponse)  `McpStatusResponse`

Response from [`ClaudeSDKClient.get_mcp_status()`](https://code.claude.com/docs/en/agent-sdk/python#methods). Wraps the list of server statuses under the `mcpServers` key.

```
class McpStatusResponse(TypedDict):
    mcpServers: list[McpServerStatus]
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#mcpserverstatus)  `McpServerStatus`

Status of a connected MCP server, contained in [`McpStatusResponse`](https://code.claude.com/docs/en/agent-sdk/python#mcp-status-response).

```
class McpServerStatus(TypedDict):
    name: str
    status: McpServerConnectionStatus  # "connected" | "failed" | "needs-auth" | "pending" | "disabled"
    serverInfo: NotRequired[McpServerInfo]
    error: NotRequired[str]
    config: NotRequired[McpServerStatusConfig]
    scope: NotRequired[str]
    tools: NotRequired[list[McpToolInfo]]
```

| Field | Type | Description |
| --- | --- | --- |
| `name` | `str` | Server name |
| `status` | `str` | One of `"connected"`, `"failed"`, `"needs-auth"`, `"pending"`, or `"disabled"` |
| `serverInfo` | `dict` (optional) | Server name and version (`{"name": str, "version": str}`) |
| `error` | `str` (optional) | Error message if the server failed to connect |
| `config` | [`McpServerStatusConfig`](https://code.claude.com/docs/en/agent-sdk/python#mcp-server-status-config) (optional) | Server configuration. Same shape as [`McpServerConfig`](https://code.claude.com/docs/en/agent-sdk/python#mcp-server-config) (stdio, SSE, HTTP, or SDK), plus a `claudeai-proxy` variant for servers connected through claude.ai |
| `scope` | `str` (optional) | Configuration scope |
| `tools` | `list` (optional) | Tools provided by this server, each with `name`, `description`, and `annotations` fields |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#sdkpluginconfig)  `SdkPluginConfig`

Configuration for loading plugins in the SDK.

```
class SdkPluginConfig(TypedDict):
    type: Literal["local"]
    path: str
```

| Field | Type | Description |
| --- | --- | --- |
| `type` | `Literal["local"]` | Must be `"local"` (only local plugins currently supported) |
| `path` | `str` | Absolute or relative path to the plugin directory |

**Example:**

```
plugins = [\
    {"type": "local", "path": "./my-plugin"},\
    {"type": "local", "path": "/absolute/path/to/plugin"},\
]
```

For complete information on creating and using plugins, see [Plugins](https://code.claude.com/docs/en/agent-sdk/plugins).

## [​](https://code.claude.com/docs/en/agent-sdk/python\#message-types)  Message Types

### [​](https://code.claude.com/docs/en/agent-sdk/python\#message)  `Message`

Union type of all possible messages.

```
Message = (
    UserMessage
    | AssistantMessage
    | SystemMessage
    | ResultMessage
    | StreamEvent
    | RateLimitEvent
)
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#usermessage)  `UserMessage`

User input message.

```
@dataclass
class UserMessage:
    content: str | list[ContentBlock]
    uuid: str | None = None
    parent_tool_use_id: str | None = None
    tool_use_result: dict[str, Any] | None = None
```

| Field | Type | Description |
| --- | --- | --- |
| `content` | `str | list[ContentBlock]` | Message content as text or content blocks |
| `uuid` | `str | None` | Unique message identifier |
| `parent_tool_use_id` | `str | None` | Tool use ID if this message is a tool result response |
| `tool_use_result` | `dict[str, Any] | None` | Tool result data if applicable |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#assistantmessage)  `AssistantMessage`

Assistant response message with content blocks.

```
@dataclass
class AssistantMessage:
    content: list[ContentBlock]
    model: str
    parent_tool_use_id: str | None = None
    error: AssistantMessageError | None = None
    usage: dict[str, Any] | None = None
    message_id: str | None = None
```

| Field | Type | Description |
| --- | --- | --- |
| `content` | `list[ContentBlock]` | List of content blocks in the response |
| `model` | `str` | Model that generated the response |
| `parent_tool_use_id` | `str | None` | Tool use ID if this is a nested response |
| `error` | [`AssistantMessageError`](https://code.claude.com/docs/en/agent-sdk/python#assistant-message-error)` | None` | Error type if the response encountered an error |
| `usage` | `dict[str, Any] | None` | Per-message token usage (same keys as [`ResultMessage.usage`](https://code.claude.com/docs/en/agent-sdk/python#result-message)) |
| `message_id` | `str | None` | API message ID. Multiple messages from one turn share the same ID |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#assistantmessageerror)  `AssistantMessageError`

Possible error types for assistant messages.

```
AssistantMessageError = Literal[\
    "authentication_failed",\
    "billing_error",\
    "rate_limit",\
    "invalid_request",\
    "server_error",\
    "max_output_tokens",\
    "unknown",\
]
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#systemmessage)  `SystemMessage`

System message with metadata.

```
@dataclass
class SystemMessage:
    subtype: str
    data: dict[str, Any]
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#resultmessage)  `ResultMessage`

Final result message with cost and usage information.

```
@dataclass
class ResultMessage:
    subtype: str
    duration_ms: int
    duration_api_ms: int
    is_error: bool
    num_turns: int
    session_id: str
    total_cost_usd: float | None = None
    usage: dict[str, Any] | None = None
    result: str | None = None
    stop_reason: str | None = None
    structured_output: Any = None
    model_usage: dict[str, Any] | None = None
```

The `usage` dict contains the following keys when present:

| Key | Type | Description |
| --- | --- | --- |
| `input_tokens` | `int` | Total input tokens consumed. |
| `output_tokens` | `int` | Total output tokens generated. |
| `cache_creation_input_tokens` | `int` | Tokens used to create new cache entries. |
| `cache_read_input_tokens` | `int` | Tokens read from existing cache entries. |

The `model_usage` dict maps model names to per-model usage. The inner dict keys use camelCase because the value is passed through unmodified from the underlying CLI process, matching the TypeScript [`ModelUsage`](https://code.claude.com/docs/en/agent-sdk/typescript#model-usage) type:

| Key | Type | Description |
| --- | --- | --- |
| `inputTokens` | `int` | Input tokens for this model. |
| `outputTokens` | `int` | Output tokens for this model. |
| `cacheReadInputTokens` | `int` | Cache read tokens for this model. |
| `cacheCreationInputTokens` | `int` | Cache creation tokens for this model. |
| `webSearchRequests` | `int` | Web search requests made by this model. |
| `costUSD` | `float` | Estimated cost in USD for this model, computed client-side. See [Track cost and usage](https://code.claude.com/docs/en/agent-sdk/cost-tracking) for billing caveats. |
| `contextWindow` | `int` | Context window size for this model. |
| `maxOutputTokens` | `int` | Maximum output token limit for this model. |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#streamevent)  `StreamEvent`

Stream event for partial message updates during streaming. Only received when `include_partial_messages=True` in `ClaudeAgentOptions`. Import via `from claude_agent_sdk.types import StreamEvent`.

```
@dataclass
class StreamEvent:
    uuid: str
    session_id: str
    event: dict[str, Any]  # The raw Claude API stream event
    parent_tool_use_id: str | None = None
```

| Field | Type | Description |
| --- | --- | --- |
| `uuid` | `str` | Unique identifier for this event |
| `session_id` | `str` | Session identifier |
| `event` | `dict[str, Any]` | The raw Claude API stream event data |
| `parent_tool_use_id` | `str | None` | Parent tool use ID if this event is from a subagent |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#ratelimitevent)  `RateLimitEvent`

Emitted when rate limit status changes (for example, from `"allowed"` to `"allowed_warning"`). Use this to warn users before they hit a hard limit, or to back off when status is `"rejected"`.

```
@dataclass
class RateLimitEvent:
    rate_limit_info: RateLimitInfo
    uuid: str
    session_id: str
```

| Field | Type | Description |
| --- | --- | --- |
| `rate_limit_info` | [`RateLimitInfo`](https://code.claude.com/docs/en/agent-sdk/python#rate-limit-info) | Current rate limit state |
| `uuid` | `str` | Unique event identifier |
| `session_id` | `str` | Session identifier |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#ratelimitinfo)  `RateLimitInfo`

Rate limit state carried by [`RateLimitEvent`](https://code.claude.com/docs/en/agent-sdk/python#rate-limit-event).

```
RateLimitStatus = Literal["allowed", "allowed_warning", "rejected"]
RateLimitType = Literal[\
    "five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet", "overage"\
]

@dataclass
class RateLimitInfo:
    status: RateLimitStatus
    resets_at: int | None = None
    rate_limit_type: RateLimitType | None = None
    utilization: float | None = None
    overage_status: RateLimitStatus | None = None
    overage_resets_at: int | None = None
    overage_disabled_reason: str | None = None
    raw: dict[str, Any] = field(default_factory=dict)
```

| Field | Type | Description |
| --- | --- | --- |
| `status` | `RateLimitStatus` | Current status. `"allowed_warning"` means approaching the limit; `"rejected"` means the limit was hit |
| `resets_at` | `int | None` | Unix timestamp when the rate limit window resets |
| `rate_limit_type` | `RateLimitType | None` | Which rate limit window applies |
| `utilization` | `float | None` | Fraction of the rate limit consumed (0.0 to 1.0) |
| `overage_status` | `RateLimitStatus | None` | Status of pay-as-you-go overage usage, if applicable |
| `overage_resets_at` | `int | None` | Unix timestamp when the overage window resets |
| `overage_disabled_reason` | `str | None` | Why overage is unavailable, if status is `"rejected"` |
| `raw` | `dict[str, Any]` | Full raw dict from the CLI, including fields not modeled above |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#taskstartedmessage)  `TaskStartedMessage`

Emitted when a background task starts. A background task is anything tracked outside the main turn: a backgrounded Bash command, a [Monitor](https://code.claude.com/docs/en/agent-sdk/python#monitor) watch, a subagent spawned via the Agent tool, or a remote agent. The `task_type` field tells you which. This naming is unrelated to the `Task`-to-`Agent` tool rename.

```
@dataclass
class TaskStartedMessage(SystemMessage):
    task_id: str
    description: str
    uuid: str
    session_id: str
    tool_use_id: str | None = None
    task_type: str | None = None
```

| Field | Type | Description |
| --- | --- | --- |
| `task_id` | `str` | Unique identifier for the task |
| `description` | `str` | Description of the task |
| `uuid` | `str` | Unique message identifier |
| `session_id` | `str` | Session identifier |
| `tool_use_id` | `str | None` | Associated tool use ID |
| `task_type` | `str | None` | Which kind of background task: `"local_bash"` for background Bash and Monitor watches, `"local_agent"`, or `"remote_agent"` |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#taskusage)  `TaskUsage`

Token and timing data for a background task.

```
class TaskUsage(TypedDict):
    total_tokens: int
    tool_uses: int
    duration_ms: int
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#taskprogressmessage)  `TaskProgressMessage`

Emitted periodically with progress updates for a running background task.

```
@dataclass
class TaskProgressMessage(SystemMessage):
    task_id: str
    description: str
    usage: TaskUsage
    uuid: str
    session_id: str
    tool_use_id: str | None = None
    last_tool_name: str | None = None
```

| Field | Type | Description |
| --- | --- | --- |
| `task_id` | `str` | Unique identifier for the task |
| `description` | `str` | Current status description |
| `usage` | `TaskUsage` | Token usage for this task so far |
| `uuid` | `str` | Unique message identifier |
| `session_id` | `str` | Session identifier |
| `tool_use_id` | `str | None` | Associated tool use ID |
| `last_tool_name` | `str | None` | Name of the last tool the task used |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#tasknotificationmessage)  `TaskNotificationMessage`

Emitted when a background task completes, fails, or is stopped. Background tasks include `run_in_background` Bash commands, Monitor watches, and background subagents.

```
@dataclass
class TaskNotificationMessage(SystemMessage):
    task_id: str
    status: TaskNotificationStatus  # "completed" | "failed" | "stopped"
    output_file: str
    summary: str
    uuid: str
    session_id: str
    tool_use_id: str | None = None
    usage: TaskUsage | None = None
```

| Field | Type | Description |
| --- | --- | --- |
| `task_id` | `str` | Unique identifier for the task |
| `status` | `TaskNotificationStatus` | One of `"completed"`, `"failed"`, or `"stopped"` |
| `output_file` | `str` | Path to the task output file |
| `summary` | `str` | Summary of the task result |
| `uuid` | `str` | Unique message identifier |
| `session_id` | `str` | Session identifier |
| `tool_use_id` | `str | None` | Associated tool use ID |
| `usage` | `TaskUsage | None` | Final token usage for the task |

## [​](https://code.claude.com/docs/en/agent-sdk/python\#content-block-types)  Content Block Types

### [​](https://code.claude.com/docs/en/agent-sdk/python\#contentblock)  `ContentBlock`

Union type of all content blocks.

```
ContentBlock = TextBlock | ThinkingBlock | ToolUseBlock | ToolResultBlock
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#textblock)  `TextBlock`

Text content block.

```
@dataclass
class TextBlock:
    text: str
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#thinkingblock)  `ThinkingBlock`

Thinking content block (for models with thinking capability).

```
@dataclass
class ThinkingBlock:
    thinking: str
    signature: str
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#tooluseblock)  `ToolUseBlock`

Tool use request block.

```
@dataclass
class ToolUseBlock:
    id: str
    name: str
    input: dict[str, Any]
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#toolresultblock)  `ToolResultBlock`

Tool execution result block.

```
@dataclass
class ToolResultBlock:
    tool_use_id: str
    content: str | list[dict[str, Any]] | None = None
    is_error: bool | None = None
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#error-types)  Error Types

### [​](https://code.claude.com/docs/en/agent-sdk/python\#claudesdkerror)  `ClaudeSDKError`

Base exception class for all SDK errors.

```
class ClaudeSDKError(Exception):
    """Base error for Claude SDK."""
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#clinotfounderror)  `CLINotFoundError`

Raised when Claude Code CLI is not installed or not found.

```
class CLINotFoundError(CLIConnectionError):
    def __init__(
        self, message: str = "Claude Code not found", cli_path: str | None = None
    ):
        """
        Args:
            message: Error message (default: "Claude Code not found")
            cli_path: Optional path to the CLI that was not found
        """
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#cliconnectionerror)  `CLIConnectionError`

Raised when connection to Claude Code fails.

```
class CLIConnectionError(ClaudeSDKError):
    """Failed to connect to Claude Code."""
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#processerror)  `ProcessError`

Raised when the Claude Code process fails.

```
class ProcessError(ClaudeSDKError):
    def __init__(
        self, message: str, exit_code: int | None = None, stderr: str | None = None
    ):
        self.exit_code = exit_code
        self.stderr = stderr
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#clijsondecodeerror)  `CLIJSONDecodeError`

Raised when JSON parsing fails.

```
class CLIJSONDecodeError(ClaudeSDKError):
    def __init__(self, line: str, original_error: Exception):
        """
        Args:
            line: The line that failed to parse
            original_error: The original JSON decode exception
        """
        self.line = line
        self.original_error = original_error
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#hook-types)  Hook Types

For a comprehensive guide on using hooks with examples and common patterns, see the [Hooks guide](https://code.claude.com/docs/en/agent-sdk/hooks).

### [​](https://code.claude.com/docs/en/agent-sdk/python\#hookevent)  `HookEvent`

Supported hook event types.

```
HookEvent = Literal[\
    "PreToolUse",  # Called before tool execution\
    "PostToolUse",  # Called after tool execution\
    "PostToolUseFailure",  # Called when a tool execution fails\
    "UserPromptSubmit",  # Called when user submits a prompt\
    "Stop",  # Called when stopping execution\
    "SubagentStop",  # Called when a subagent stops\
    "PreCompact",  # Called before message compaction\
    "Notification",  # Called for notification events\
    "SubagentStart",  # Called when a subagent starts\
    "PermissionRequest",  # Called when a permission decision is needed\
]
```

The TypeScript SDK supports additional hook events not yet available in Python: `SessionStart`, `SessionEnd`, `Setup`, `TeammateIdle`, `TaskCompleted`, `ConfigChange`, `WorktreeCreate`, and `WorktreeRemove`.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#hookcallback)  `HookCallback`

Type definition for hook callback functions.

```
HookCallback = Callable[[HookInput, str | None, HookContext], Awaitable[HookJSONOutput]]
```

Parameters:

- `input`: Strongly-typed hook input with discriminated unions based on `hook_event_name` (see [`HookInput`](https://code.claude.com/docs/en/agent-sdk/python#hook-input))
- `tool_use_id`: Optional tool use identifier (for tool-related hooks)
- `context`: Hook context with additional information

Returns a [`HookJSONOutput`](https://code.claude.com/docs/en/agent-sdk/python#hook-json-output) that may contain:

- `decision`: `"block"` to block the action
- `systemMessage`: System message to add to the transcript
- `hookSpecificOutput`: Hook-specific output data

### [​](https://code.claude.com/docs/en/agent-sdk/python\#hookcontext)  `HookContext`

Context information passed to hook callbacks.

```
class HookContext(TypedDict):
    signal: Any | None  # Future: abort signal support
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#hookmatcher)  `HookMatcher`

Configuration for matching hooks to specific events or tools.

```
@dataclass
class HookMatcher:
    matcher: str | None = (
        None  # Tool name or pattern to match (e.g., "Bash", "Write|Edit")
    )
    hooks: list[HookCallback] = field(
        default_factory=list
    )  # List of callbacks to execute
    timeout: float | None = (
        None  # Timeout in seconds for all hooks in this matcher (default: 60)
    )
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#hookinput)  `HookInput`

Union type of all hook input types. The actual type depends on the `hook_event_name` field.

```
HookInput = (
    PreToolUseHookInput
    | PostToolUseHookInput
    | PostToolUseFailureHookInput
    | UserPromptSubmitHookInput
    | StopHookInput
    | SubagentStopHookInput
    | PreCompactHookInput
    | NotificationHookInput
    | SubagentStartHookInput
    | PermissionRequestHookInput
)
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#basehookinput)  `BaseHookInput`

Base fields present in all hook input types.

```
class BaseHookInput(TypedDict):
    session_id: str
    transcript_path: str
    cwd: str
    permission_mode: NotRequired[str]
```

| Field | Type | Description |
| --- | --- | --- |
| `session_id` | `str` | Current session identifier |
| `transcript_path` | `str` | Path to the session transcript file |
| `cwd` | `str` | Current working directory |
| `permission_mode` | `str` (optional) | Current permission mode |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#pretoolusehookinput)  `PreToolUseHookInput`

Input data for `PreToolUse` hook events.

```
class PreToolUseHookInput(BaseHookInput):
    hook_event_name: Literal["PreToolUse"]
    tool_name: str
    tool_input: dict[str, Any]
    tool_use_id: str
    agent_id: NotRequired[str]
    agent_type: NotRequired[str]
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["PreToolUse"]` | Always “PreToolUse” |
| `tool_name` | `str` | Name of the tool about to be executed |
| `tool_input` | `dict[str, Any]` | Input parameters for the tool |
| `tool_use_id` | `str` | Unique identifier for this tool use |
| `agent_id` | `str` (optional) | Subagent identifier, present when the hook fires inside a subagent |
| `agent_type` | `str` (optional) | Subagent type, present when the hook fires inside a subagent |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#posttoolusehookinput)  `PostToolUseHookInput`

Input data for `PostToolUse` hook events.

```
class PostToolUseHookInput(BaseHookInput):
    hook_event_name: Literal["PostToolUse"]
    tool_name: str
    tool_input: dict[str, Any]
    tool_response: Any
    tool_use_id: str
    agent_id: NotRequired[str]
    agent_type: NotRequired[str]
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["PostToolUse"]` | Always “PostToolUse” |
| `tool_name` | `str` | Name of the tool that was executed |
| `tool_input` | `dict[str, Any]` | Input parameters that were used |
| `tool_response` | `Any` | Response from the tool execution |
| `tool_use_id` | `str` | Unique identifier for this tool use |
| `agent_id` | `str` (optional) | Subagent identifier, present when the hook fires inside a subagent |
| `agent_type` | `str` (optional) | Subagent type, present when the hook fires inside a subagent |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#posttoolusefailurehookinput)  `PostToolUseFailureHookInput`

Input data for `PostToolUseFailure` hook events. Called when a tool execution fails.

```
class PostToolUseFailureHookInput(BaseHookInput):
    hook_event_name: Literal["PostToolUseFailure"]
    tool_name: str
    tool_input: dict[str, Any]
    tool_use_id: str
    error: str
    is_interrupt: NotRequired[bool]
    agent_id: NotRequired[str]
    agent_type: NotRequired[str]
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["PostToolUseFailure"]` | Always “PostToolUseFailure” |
| `tool_name` | `str` | Name of the tool that failed |
| `tool_input` | `dict[str, Any]` | Input parameters that were used |
| `tool_use_id` | `str` | Unique identifier for this tool use |
| `error` | `str` | Error message from the failed execution |
| `is_interrupt` | `bool` (optional) | Whether the failure was caused by an interrupt |
| `agent_id` | `str` (optional) | Subagent identifier, present when the hook fires inside a subagent |
| `agent_type` | `str` (optional) | Subagent type, present when the hook fires inside a subagent |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#userpromptsubmithookinput)  `UserPromptSubmitHookInput`

Input data for `UserPromptSubmit` hook events.

```
class UserPromptSubmitHookInput(BaseHookInput):
    hook_event_name: Literal["UserPromptSubmit"]
    prompt: str
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["UserPromptSubmit"]` | Always “UserPromptSubmit” |
| `prompt` | `str` | The user’s submitted prompt |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#stophookinput)  `StopHookInput`

Input data for `Stop` hook events.

```
class StopHookInput(BaseHookInput):
    hook_event_name: Literal["Stop"]
    stop_hook_active: bool
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["Stop"]` | Always “Stop” |
| `stop_hook_active` | `bool` | Whether the stop hook is active |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#subagentstophookinput)  `SubagentStopHookInput`

Input data for `SubagentStop` hook events.

```
class SubagentStopHookInput(BaseHookInput):
    hook_event_name: Literal["SubagentStop"]
    stop_hook_active: bool
    agent_id: str
    agent_transcript_path: str
    agent_type: str
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["SubagentStop"]` | Always “SubagentStop” |
| `stop_hook_active` | `bool` | Whether the stop hook is active |
| `agent_id` | `str` | Unique identifier for the subagent |
| `agent_transcript_path` | `str` | Path to the subagent’s transcript file |
| `agent_type` | `str` | Type of the subagent |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#precompacthookinput)  `PreCompactHookInput`

Input data for `PreCompact` hook events.

```
class PreCompactHookInput(BaseHookInput):
    hook_event_name: Literal["PreCompact"]
    trigger: Literal["manual", "auto"]
    custom_instructions: str | None
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["PreCompact"]` | Always “PreCompact” |
| `trigger` | `Literal["manual", "auto"]` | What triggered the compaction |
| `custom_instructions` | `str | None` | Custom instructions for compaction |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#notificationhookinput)  `NotificationHookInput`

Input data for `Notification` hook events.

```
class NotificationHookInput(BaseHookInput):
    hook_event_name: Literal["Notification"]
    message: str
    title: NotRequired[str]
    notification_type: str
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["Notification"]` | Always “Notification” |
| `message` | `str` | Notification message content |
| `title` | `str` (optional) | Notification title |
| `notification_type` | `str` | Type of notification |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#subagentstarthookinput)  `SubagentStartHookInput`

Input data for `SubagentStart` hook events.

```
class SubagentStartHookInput(BaseHookInput):
    hook_event_name: Literal["SubagentStart"]
    agent_id: str
    agent_type: str
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["SubagentStart"]` | Always “SubagentStart” |
| `agent_id` | `str` | Unique identifier for the subagent |
| `agent_type` | `str` | Type of the subagent |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissionrequesthookinput)  `PermissionRequestHookInput`

Input data for `PermissionRequest` hook events. Allows hooks to handle permission decisions programmatically.

```
class PermissionRequestHookInput(BaseHookInput):
    hook_event_name: Literal["PermissionRequest"]
    tool_name: str
    tool_input: dict[str, Any]
    permission_suggestions: NotRequired[list[Any]]
```

| Field | Type | Description |
| --- | --- | --- |
| `hook_event_name` | `Literal["PermissionRequest"]` | Always “PermissionRequest” |
| `tool_name` | `str` | Name of the tool requesting permission |
| `tool_input` | `dict[str, Any]` | Input parameters for the tool |
| `permission_suggestions` | `list[Any]` (optional) | Suggested permission updates from the CLI |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#hookjsonoutput)  `HookJSONOutput`

Union type for hook callback return values.

```
HookJSONOutput = AsyncHookJSONOutput | SyncHookJSONOutput
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#synchookjsonoutput)  `SyncHookJSONOutput`

Synchronous hook output with control and decision fields.

```
class SyncHookJSONOutput(TypedDict):
    # Control fields
    continue_: NotRequired[bool]  # Whether to proceed (default: True)
    suppressOutput: NotRequired[bool]  # Hide stdout from transcript
    stopReason: NotRequired[str]  # Message when continue is False

    # Decision fields
    decision: NotRequired[Literal["block"]]
    systemMessage: NotRequired[str]  # Warning message for user
    reason: NotRequired[str]  # Feedback for Claude

    # Hook-specific output
    hookSpecificOutput: NotRequired[HookSpecificOutput]
```

Use `continue_` (with underscore) in Python code. It is automatically converted to `continue` when sent to the CLI.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#hookspecificoutput)  `HookSpecificOutput`

A `TypedDict` containing the hook event name and event-specific fields. The shape depends on the `hookEventName` value. For full details on available fields per hook event, see [Control execution with hooks](https://code.claude.com/docs/en/agent-sdk/hooks#outputs).A discriminated union of event-specific output types. The `hookEventName` field determines which fields are valid.

```
class PreToolUseHookSpecificOutput(TypedDict):
    hookEventName: Literal["PreToolUse"]
    permissionDecision: NotRequired[Literal["allow", "deny", "ask"]]
    permissionDecisionReason: NotRequired[str]
    updatedInput: NotRequired[dict[str, Any]]
    additionalContext: NotRequired[str]

class PostToolUseHookSpecificOutput(TypedDict):
    hookEventName: Literal["PostToolUse"]
    additionalContext: NotRequired[str]
    updatedMCPToolOutput: NotRequired[Any]

class PostToolUseFailureHookSpecificOutput(TypedDict):
    hookEventName: Literal["PostToolUseFailure"]
    additionalContext: NotRequired[str]

class UserPromptSubmitHookSpecificOutput(TypedDict):
    hookEventName: Literal["UserPromptSubmit"]
    additionalContext: NotRequired[str]

class NotificationHookSpecificOutput(TypedDict):
    hookEventName: Literal["Notification"]
    additionalContext: NotRequired[str]

class SubagentStartHookSpecificOutput(TypedDict):
    hookEventName: Literal["SubagentStart"]
    additionalContext: NotRequired[str]

class PermissionRequestHookSpecificOutput(TypedDict):
    hookEventName: Literal["PermissionRequest"]
    decision: dict[str, Any]

HookSpecificOutput = (
    PreToolUseHookSpecificOutput
    | PostToolUseHookSpecificOutput
    | PostToolUseFailureHookSpecificOutput
    | UserPromptSubmitHookSpecificOutput
    | NotificationHookSpecificOutput
    | SubagentStartHookSpecificOutput
    | PermissionRequestHookSpecificOutput
)
```

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#asynchookjsonoutput)  `AsyncHookJSONOutput`

Async hook output that defers hook execution.

```
class AsyncHookJSONOutput(TypedDict):
    async_: Literal[True]  # Set to True to defer execution
    asyncTimeout: NotRequired[int]  # Timeout in milliseconds
```

Use `async_` (with underscore) in Python code. It is automatically converted to `async` when sent to the CLI.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#hook-usage-example)  Hook Usage Example

This example registers two hooks: one that blocks dangerous bash commands like `rm -rf /`, and another that logs all tool usage for auditing. The security hook only runs on Bash commands (via the `matcher`), while the logging hook runs on all tools.

```
from claude_agent_sdk import query, ClaudeAgentOptions, HookMatcher, HookContext
from typing import Any

async def validate_bash_command(
    input_data: dict[str, Any], tool_use_id: str | None, context: HookContext
) -> dict[str, Any]:
    """Validate and potentially block dangerous bash commands."""
    if input_data["tool_name"] == "Bash":
        command = input_data["tool_input"].get("command", "")
        if "rm -rf /" in command:
            return {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "deny",
                    "permissionDecisionReason": "Dangerous command blocked",
                }
            }
    return {}

async def log_tool_use(
    input_data: dict[str, Any], tool_use_id: str | None, context: HookContext
) -> dict[str, Any]:
    """Log all tool usage for auditing."""
    print(f"Tool used: {input_data.get('tool_name')}")
    return {}

options = ClaudeAgentOptions(
    hooks={
        "PreToolUse": [\
            HookMatcher(\
                matcher="Bash", hooks=[validate_bash_command], timeout=120\
            ),  # 2 min for validation\
            HookMatcher(\
                hooks=[log_tool_use]\
            ),  # Applies to all tools (default 60s timeout)\
        ],
        "PostToolUse": [HookMatcher(hooks=[log_tool_use])],
    }
)

async for message in query(prompt="Analyze this codebase", options=options):
    print(message)
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#tool-input/output-types)  Tool Input/Output Types

Documentation of input/output schemas for all built-in Claude Code tools. While the Python SDK doesn’t export these as types, they represent the structure of tool inputs and outputs in messages.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#agent)  Agent

**Tool name:**`Agent` (previously `Task`, which is still accepted as an alias)**Input:**

```
{
    "description": str,  # A short (3-5 word) description of the task
    "prompt": str,  # The task for the agent to perform
    "subagent_type": str,  # The type of specialized agent to use
}
```

**Output:**

```
{
    "result": str,  # Final result from the subagent
    "usage": dict | None,  # Token usage statistics
    "total_cost_usd": float | None,  # Estimated total cost in USD
    "duration_ms": int | None,  # Execution duration in milliseconds
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#askuserquestion)  AskUserQuestion

**Tool name:**`AskUserQuestion`Asks the user clarifying questions during execution. See [Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input#handle-clarifying-questions) for usage details.**Input:**

```
{
    "questions": [  # Questions to ask the user (1-4 questions)\
        {\
            "question": str,  # The complete question to ask the user\
            "header": str,  # Very short label displayed as a chip/tag (max 12 chars)\
            "options": [  # The available choices (2-4 options)\
                {\
                    "label": str,  # Display text for this option (1-5 words)\
                    "description": str,  # Explanation of what this option means\
                }\
            ],\
            "multiSelect": bool,  # Set to true to allow multiple selections\
        }\
    ],
    "answers": dict | None,  # User answers populated by the permission system
}
```

**Output:**

```
{
    "questions": [  # The questions that were asked\
        {\
            "question": str,\
            "header": str,\
            "options": [{"label": str, "description": str}],\
            "multiSelect": bool,\
        }\
    ],
    "answers": dict[str, str],  # Maps question text to answer string
    # Multi-select answers are comma-separated
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#bash)  Bash

**Tool name:**`Bash`**Input:**

```
{
    "command": str,  # The command to execute
    "timeout": int | None,  # Optional timeout in milliseconds (max 600000)
    "description": str | None,  # Clear, concise description (5-10 words)
    "run_in_background": bool | None,  # Set to true to run in background
}
```

**Output:**

```
{
    "output": str,  # Combined stdout and stderr output
    "exitCode": int,  # Exit code of the command
    "killed": bool | None,  # Whether command was killed due to timeout
    "shellId": str | None,  # Shell ID for background processes
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#monitor)  Monitor

**Tool name:**`Monitor`Runs a background script and delivers each stdout line to Claude as an event so it can react without polling. Monitor follows the same permission rules as Bash. See the [Monitor tool reference](https://code.claude.com/docs/en/tools-reference#monitor-tool) for behavior and provider availability.**Input:**

```
{
    "command": str,  # Shell script; each stdout line is an event, exit ends the watch
    "description": str,  # Short description shown in notifications
    "timeout_ms": int | None,  # Kill after this deadline (default 300000, max 3600000)
    "persistent": bool | None,  # Run for the lifetime of the session; stop with TaskStop
}
```

**Output:**

```
{
    "taskId": str,  # ID of the background monitor task
    "timeoutMs": int,  # Timeout deadline in milliseconds (0 when persistent)
    "persistent": bool | None,  # True when running until TaskStop or session end
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#edit)  Edit

**Tool name:**`Edit`**Input:**

```
{
    "file_path": str,  # The absolute path to the file to modify
    "old_string": str,  # The text to replace
    "new_string": str,  # The text to replace it with
    "replace_all": bool | None,  # Replace all occurrences (default False)
}
```

**Output:**

```
{
    "message": str,  # Confirmation message
    "replacements": int,  # Number of replacements made
    "file_path": str,  # File path that was edited
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#read)  Read

**Tool name:**`Read`**Input:**

```
{
    "file_path": str,  # The absolute path to the file to read
    "offset": int | None,  # The line number to start reading from
    "limit": int | None,  # The number of lines to read
}
```

**Output (Text files):**

```
{
    "content": str,  # File contents with line numbers
    "total_lines": int,  # Total number of lines in file
    "lines_returned": int,  # Lines actually returned
}
```

**Output (Images):**

```
{
    "image": str,  # Base64 encoded image data
    "mime_type": str,  # Image MIME type
    "file_size": int,  # File size in bytes
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#write)  Write

**Tool name:**`Write`**Input:**

```
{
    "file_path": str,  # The absolute path to the file to write
    "content": str,  # The content to write to the file
}
```

**Output:**

```
{
    "message": str,  # Success message
    "bytes_written": int,  # Number of bytes written
    "file_path": str,  # File path that was written
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#glob)  Glob

**Tool name:**`Glob`**Input:**

```
{
    "pattern": str,  # The glob pattern to match files against
    "path": str | None,  # The directory to search in (defaults to cwd)
}
```

**Output:**

```
{
    "matches": list[str],  # Array of matching file paths
    "count": int,  # Number of matches found
    "search_path": str,  # Search directory used
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#grep)  Grep

**Tool name:**`Grep`**Input:**

```
{
    "pattern": str,  # The regular expression pattern
    "path": str | None,  # File or directory to search in
    "glob": str | None,  # Glob pattern to filter files
    "type": str | None,  # File type to search
    "output_mode": str | None,  # "content", "files_with_matches", or "count"
    "-i": bool | None,  # Case insensitive search
    "-n": bool | None,  # Show line numbers
    "-B": int | None,  # Lines to show before each match
    "-A": int | None,  # Lines to show after each match
    "-C": int | None,  # Lines to show before and after
    "head_limit": int | None,  # Limit output to first N lines/entries
    "multiline": bool | None,  # Enable multiline mode
}
```

**Output (content mode):**

```
{
    "matches": [\
        {\
            "file": str,\
            "line_number": int | None,\
            "line": str,\
            "before_context": list[str] | None,\
            "after_context": list[str] | None,\
        }\
    ],
    "total_matches": int,
}
```

**Output (files\_with\_matches mode):**

```
{
    "files": list[str],  # Files containing matches
    "count": int,  # Number of files with matches
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#notebookedit)  NotebookEdit

**Tool name:**`NotebookEdit`**Input:**

```
{
    "notebook_path": str,  # Absolute path to the Jupyter notebook
    "cell_id": str | None,  # The ID of the cell to edit
    "new_source": str,  # The new source for the cell
    "cell_type": "code" | "markdown" | None,  # The type of the cell
    "edit_mode": "replace" | "insert" | "delete" | None,  # Edit operation type
}
```

**Output:**

```
{
    "message": str,  # Success message
    "edit_type": "replaced" | "inserted" | "deleted",  # Type of edit performed
    "cell_id": str | None,  # Cell ID that was affected
    "total_cells": int,  # Total cells in notebook after edit
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#webfetch)  WebFetch

**Tool name:**`WebFetch`**Input:**

```
{
    "url": str,  # The URL to fetch content from
    "prompt": str,  # The prompt to run on the fetched content
}
```

**Output:**

```
{
    "response": str,  # AI model's response to the prompt
    "url": str,  # URL that was fetched
    "final_url": str | None,  # Final URL after redirects
    "status_code": int | None,  # HTTP status code
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#websearch)  WebSearch

**Tool name:**`WebSearch`**Input:**

```
{
    "query": str,  # The search query to use
    "allowed_domains": list[str] | None,  # Only include results from these domains
    "blocked_domains": list[str] | None,  # Never include results from these domains
}
```

**Output:**

```
{
    "results": [{"title": str, "url": str, "snippet": str, "metadata": dict | None}],
    "total_results": int,
    "query": str,
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#todowrite)  TodoWrite

**Tool name:**`TodoWrite`**Input:**

```
{
    "todos": [\
        {\
            "content": str,  # The task description\
            "status": "pending" | "in_progress" | "completed",  # Task status\
            "activeForm": str,  # Active form of the description\
        }\
    ]
}
```

**Output:**

```
{
    "message": str,  # Success message
    "stats": {"total": int, "pending": int, "in_progress": int, "completed": int},
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#bashoutput)  BashOutput

**Tool name:**`BashOutput`**Input:**

```
{
    "bash_id": str,  # The ID of the background shell
    "filter": str | None,  # Optional regex to filter output lines
}
```

**Output:**

```
{
    "output": str,  # New output since last check
    "status": "running" | "completed" | "failed",  # Current shell status
    "exitCode": int | None,  # Exit code when completed
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#killbash)  KillBash

**Tool name:**`KillBash`**Input:**

```
{
    "shell_id": str  # The ID of the background shell to kill
}
```

**Output:**

```
{
    "message": str,  # Success message
    "shell_id": str,  # ID of the killed shell
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#exitplanmode)  ExitPlanMode

**Tool name:**`ExitPlanMode`**Input:**

```
{
    "plan": str  # The plan to run by the user for approval
}
```

**Output:**

```
{
    "message": str,  # Confirmation message
    "approved": bool | None,  # Whether user approved the plan
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#listmcpresources)  ListMcpResources

**Tool name:**`ListMcpResources`**Input:**

```
{
    "server": str | None  # Optional server name to filter resources by
}
```

**Output:**

```
{
    "resources": [\
        {\
            "uri": str,\
            "name": str,\
            "description": str | None,\
            "mimeType": str | None,\
            "server": str,\
        }\
    ],
    "total": int,
}
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#readmcpresource)  ReadMcpResource

**Tool name:**`ReadMcpResource`**Input:**

```
{
    "server": str,  # The MCP server name
    "uri": str,  # The resource URI to read
}
```

**Output:**

```
{
    "contents": [\
        {"uri": str, "mimeType": str | None, "text": str | None, "blob": str | None}\
    ],
    "server": str,
}
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#advanced-features-with-claudesdkclient)  Advanced Features with ClaudeSDKClient

### [​](https://code.claude.com/docs/en/agent-sdk/python\#building-a-continuous-conversation-interface)  Building a Continuous Conversation Interface

```
from claude_agent_sdk import (
    ClaudeSDKClient,
    ClaudeAgentOptions,
    AssistantMessage,
    TextBlock,
)
import asyncio

class ConversationSession:
    """Maintains a single conversation session with Claude."""

    def __init__(self, options: ClaudeAgentOptions | None = None):
        self.client = ClaudeSDKClient(options)
        self.turn_count = 0

    async def start(self):
        await self.client.connect()
        print("Starting conversation session. Claude will remember context.")
        print(
            "Commands: 'exit' to quit, 'interrupt' to stop current task, 'new' for new session"
        )

        while True:
            user_input = input(f"\n[Turn {self.turn_count + 1}] You: ")

            if user_input.lower() == "exit":
                break
            elif user_input.lower() == "interrupt":
                await self.client.interrupt()
                print("Task interrupted!")
                continue
            elif user_input.lower() == "new":
                # Disconnect and reconnect for a fresh session
                await self.client.disconnect()
                await self.client.connect()
                self.turn_count = 0
                print("Started new conversation session (previous context cleared)")
                continue

            # Send message - the session retains all previous messages
            await self.client.query(user_input)
            self.turn_count += 1

            # Process response
            print(f"[Turn {self.turn_count}] Claude: ", end="")
            async for message in self.client.receive_response():
                if isinstance(message, AssistantMessage):
                    for block in message.content:
                        if isinstance(block, TextBlock):
                            print(block.text, end="")
            print()  # New line after response

        await self.client.disconnect()
        print(f"Conversation ended after {self.turn_count} turns.")

async def main():
    options = ClaudeAgentOptions(
        allowed_tools=["Read", "Write", "Bash"], permission_mode="acceptEdits"
    )
    session = ConversationSession(options)
    await session.start()

# Example conversation:
# Turn 1 - You: "Create a file called hello.py"
# Turn 1 - Claude: "I'll create a hello.py file for you..."
# Turn 2 - You: "What's in that file?"
# Turn 2 - Claude: "The hello.py file I just created contains..." (remembers!)
# Turn 3 - You: "Add a main function to it"
# Turn 3 - Claude: "I'll add a main function to hello.py..." (knows which file!)

asyncio.run(main())
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#using-hooks-for-behavior-modification)  Using Hooks for Behavior Modification

```
from claude_agent_sdk import (
    ClaudeSDKClient,
    ClaudeAgentOptions,
    HookMatcher,
    HookContext,
)
import asyncio
from typing import Any

async def pre_tool_logger(
    input_data: dict[str, Any], tool_use_id: str | None, context: HookContext
) -> dict[str, Any]:
    """Log all tool usage before execution."""
    tool_name = input_data.get("tool_name", "unknown")
    print(f"[PRE-TOOL] About to use: {tool_name}")

    # You can modify or block the tool execution here
    if tool_name == "Bash" and "rm -rf" in str(input_data.get("tool_input", {})):
        return {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": "Dangerous command blocked",
            }
        }
    return {}

async def post_tool_logger(
    input_data: dict[str, Any], tool_use_id: str | None, context: HookContext
) -> dict[str, Any]:
    """Log results after tool execution."""
    tool_name = input_data.get("tool_name", "unknown")
    print(f"[POST-TOOL] Completed: {tool_name}")
    return {}

async def user_prompt_modifier(
    input_data: dict[str, Any], tool_use_id: str | None, context: HookContext
) -> dict[str, Any]:
    """Add context to user prompts."""
    original_prompt = input_data.get("prompt", "")

    # Add a timestamp as additional context for Claude to see
    from datetime import datetime

    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    return {
        "hookSpecificOutput": {
            "hookEventName": "UserPromptSubmit",
            "additionalContext": f"[Submitted at {timestamp}] Original prompt: {original_prompt}",
        }
    }

async def main():
    options = ClaudeAgentOptions(
        hooks={
            "PreToolUse": [\
                HookMatcher(hooks=[pre_tool_logger]),\
                HookMatcher(matcher="Bash", hooks=[pre_tool_logger]),\
            ],
            "PostToolUse": [HookMatcher(hooks=[post_tool_logger])],
            "UserPromptSubmit": [HookMatcher(hooks=[user_prompt_modifier])],
        },
        allowed_tools=["Read", "Write", "Bash"],
    )

    async with ClaudeSDKClient(options=options) as client:
        await client.query("List files in current directory")

        async for message in client.receive_response():
            # Hooks will automatically log tool usage
            pass

asyncio.run(main())
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#real-time-progress-monitoring)  Real-time Progress Monitoring

```
from claude_agent_sdk import (
    ClaudeSDKClient,
    ClaudeAgentOptions,
    AssistantMessage,
    ToolUseBlock,
    ToolResultBlock,
    TextBlock,
)
import asyncio

async def monitor_progress():
    options = ClaudeAgentOptions(
        allowed_tools=["Write", "Bash"], permission_mode="acceptEdits"
    )

    async with ClaudeSDKClient(options=options) as client:
        await client.query("Create 5 Python files with different sorting algorithms")

        # Monitor progress in real-time
        async for message in client.receive_response():
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, ToolUseBlock):
                        if block.name == "Write":
                            file_path = block.input.get("file_path", "")
                            print(f"Creating: {file_path}")
                    elif isinstance(block, ToolResultBlock):
                        print("Completed tool execution")
                    elif isinstance(block, TextBlock):
                        print(f"Claude says: {block.text[:100]}...")

        print("Task completed!")

asyncio.run(monitor_progress())
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#example-usage)  Example Usage

### [​](https://code.claude.com/docs/en/agent-sdk/python\#basic-file-operations-using-query)  Basic file operations (using query)

```
from claude_agent_sdk import query, ClaudeAgentOptions, AssistantMessage, ToolUseBlock
import asyncio

async def create_project():
    options = ClaudeAgentOptions(
        allowed_tools=["Read", "Write", "Bash"],
        permission_mode="acceptEdits",
        cwd="/home/user/project",
    )

    async for message in query(
        prompt="Create a Python project structure with setup.py", options=options
    ):
        if isinstance(message, AssistantMessage):
            for block in message.content:
                if isinstance(block, ToolUseBlock):
                    print(f"Using tool: {block.name}")

asyncio.run(create_project())
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#error-handling)  Error handling

```
from claude_agent_sdk import query, CLINotFoundError, ProcessError, CLIJSONDecodeError

try:
    async for message in query(prompt="Hello"):
        print(message)
except CLINotFoundError:
    print(
        "Claude Code CLI not found. Try reinstalling: pip install --force-reinstall claude-agent-sdk"
    )
except ProcessError as e:
    print(f"Process failed with exit code: {e.exit_code}")
except CLIJSONDecodeError as e:
    print(f"Failed to parse response: {e}")
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#streaming-mode-with-client)  Streaming mode with client

```
from claude_agent_sdk import ClaudeSDKClient
import asyncio

async def interactive_session():
    async with ClaudeSDKClient() as client:
        # Send initial message
        await client.query("What's the weather like?")

        # Process responses
        async for msg in client.receive_response():
            print(msg)

        # Send follow-up
        await client.query("Tell me more about that")

        # Process follow-up response
        async for msg in client.receive_response():
            print(msg)

asyncio.run(interactive_session())
```

### [​](https://code.claude.com/docs/en/agent-sdk/python\#using-custom-tools-with-claudesdkclient)  Using custom tools with ClaudeSDKClient

```
from claude_agent_sdk import (
    ClaudeSDKClient,
    ClaudeAgentOptions,
    tool,
    create_sdk_mcp_server,
    AssistantMessage,
    TextBlock,
)
import asyncio
from typing import Any

# Define custom tools with @tool decorator
@tool("calculate", "Perform mathematical calculations", {"expression": str})
async def calculate(args: dict[str, Any]) -> dict[str, Any]:
    try:
        result = eval(args["expression"], {"__builtins__": {}})
        return {"content": [{"type": "text", "text": f"Result: {result}"}]}
    except Exception as e:
        return {
            "content": [{"type": "text", "text": f"Error: {str(e)}"}],
            "is_error": True,
        }

@tool("get_time", "Get current time", {})
async def get_time(args: dict[str, Any]) -> dict[str, Any]:
    from datetime import datetime

    current_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    return {"content": [{"type": "text", "text": f"Current time: {current_time}"}]}

async def main():
    # Create SDK MCP server with custom tools
    my_server = create_sdk_mcp_server(
        name="utilities", version="1.0.0", tools=[calculate, get_time]
    )

    # Configure options with the server
    options = ClaudeAgentOptions(
        mcp_servers={"utils": my_server},
        allowed_tools=["mcp__utils__calculate", "mcp__utils__get_time"],
    )

    # Use ClaudeSDKClient for interactive tool usage
    async with ClaudeSDKClient(options=options) as client:
        await client.query("What's 123 * 456?")

        # Process calculation response
        async for message in client.receive_response():
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        print(f"Calculation: {block.text}")

        # Follow up with time query
        await client.query("What time is it now?")

        async for message in client.receive_response():
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, TextBlock):
                        print(f"Time: {block.text}")

asyncio.run(main())
```

## [​](https://code.claude.com/docs/en/agent-sdk/python\#sandbox-configuration)  Sandbox Configuration

### [​](https://code.claude.com/docs/en/agent-sdk/python\#sandboxsettings)  `SandboxSettings`

Configuration for sandbox behavior. Use this to enable command sandboxing and configure network restrictions programmatically.

```
class SandboxSettings(TypedDict, total=False):
    enabled: bool
    autoAllowBashIfSandboxed: bool
    excludedCommands: list[str]
    allowUnsandboxedCommands: bool
    network: SandboxNetworkConfig
    ignoreViolations: SandboxIgnoreViolations
    enableWeakerNestedSandbox: bool
```

| Property | Type | Default | Description |
| --- | --- | --- | --- |
| `enabled` | `bool` | `False` | Enable sandbox mode for command execution |
| `autoAllowBashIfSandboxed` | `bool` | `True` | Auto-approve bash commands when sandbox is enabled |
| `excludedCommands` | `list[str]` | `[]` | Commands that always bypass sandbox restrictions (e.g., `["docker"]`). These run unsandboxed automatically without model involvement |
| `allowUnsandboxedCommands` | `bool` | `True` | Allow the model to request running commands outside the sandbox. When `True`, the model can set `dangerouslyDisableSandbox` in tool input, which falls back to the [permissions system](https://code.claude.com/docs/en/agent-sdk/python#permissions-fallback-for-unsandboxed-commands) |
| `network` | [`SandboxNetworkConfig`](https://code.claude.com/docs/en/agent-sdk/python#sandbox-network-config) | `None` | Network-specific sandbox configuration |
| `ignoreViolations` | [`SandboxIgnoreViolations`](https://code.claude.com/docs/en/agent-sdk/python#sandbox-ignore-violations) | `None` | Configure which sandbox violations to ignore |
| `enableWeakerNestedSandbox` | `bool` | `False` | Enable a weaker nested sandbox for compatibility |

**Filesystem and network access restrictions** are NOT configured via sandbox settings. Instead, they are derived from [permission rules](https://code.claude.com/docs/en/settings#permission-settings):

- **Filesystem read restrictions**: Read deny rules
- **Filesystem write restrictions**: Edit allow/deny rules
- **Network restrictions**: WebFetch allow/deny rules

Use sandbox settings for command execution sandboxing, and permission rules for filesystem and network access control.

#### [​](https://code.claude.com/docs/en/agent-sdk/python\#example-usage-2)  Example usage

```
from claude_agent_sdk import query, ClaudeAgentOptions, SandboxSettings

sandbox_settings: SandboxSettings = {
    "enabled": True,
    "autoAllowBashIfSandboxed": True,
    "network": {"allowLocalBinding": True},
}

async for message in query(
    prompt="Build and test my project",
    options=ClaudeAgentOptions(sandbox=sandbox_settings),
):
    print(message)
```

**Unix socket security**: The `allowUnixSockets` option can grant access to powerful system services. For example, allowing `/var/run/docker.sock` effectively grants full host system access through the Docker API, bypassing sandbox isolation. Only allow Unix sockets that are strictly necessary and understand the security implications of each.

### [​](https://code.claude.com/docs/en/agent-sdk/python\#sandboxnetworkconfig)  `SandboxNetworkConfig`

Network-specific configuration for sandbox mode.

```
class SandboxNetworkConfig(TypedDict, total=False):
    allowLocalBinding: bool
    allowUnixSockets: list[str]
    allowAllUnixSockets: bool
    httpProxyPort: int
    socksProxyPort: int
```

| Property | Type | Default | Description |
| --- | --- | --- | --- |
| `allowLocalBinding` | `bool` | `False` | Allow processes to bind to local ports (e.g., for dev servers) |
| `allowUnixSockets` | `list[str]` | `[]` | Unix socket paths that processes can access (e.g., Docker socket) |
| `allowAllUnixSockets` | `bool` | `False` | Allow access to all Unix sockets |
| `httpProxyPort` | `int` | `None` | HTTP proxy port for network requests |
| `socksProxyPort` | `int` | `None` | SOCKS proxy port for network requests |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#sandboxignoreviolations)  `SandboxIgnoreViolations`

Configuration for ignoring specific sandbox violations.

```
class SandboxIgnoreViolations(TypedDict, total=False):
    file: list[str]
    network: list[str]
```

| Property | Type | Default | Description |
| --- | --- | --- | --- |
| `file` | `list[str]` | `[]` | File path patterns to ignore violations for |
| `network` | `list[str]` | `[]` | Network patterns to ignore violations for |

### [​](https://code.claude.com/docs/en/agent-sdk/python\#permissions-fallback-for-unsandboxed-commands)  Permissions Fallback for Unsandboxed Commands

When `allowUnsandboxedCommands` is enabled, the model can request to run commands outside the sandbox by setting `dangerouslyDisableSandbox: True` in the tool input. These requests fall back to the existing permissions system, meaning your `can_use_tool` handler will be invoked, allowing you to implement custom authorization logic.

**`excludedCommands` vs `allowUnsandboxedCommands`:**

- `excludedCommands`: A static list of commands that always bypass the sandbox automatically (e.g., `["docker"]`). The model has no control over this.
- `allowUnsandboxedCommands`: Lets the model decide at runtime whether to request unsandboxed execution by setting `dangerouslyDisableSandbox: True` in the tool input.

```
from claude_agent_sdk import (
    query,
    ClaudeAgentOptions,
    HookMatcher,
    PermissionResultAllow,
    PermissionResultDeny,
    ToolPermissionContext,
)

async def can_use_tool(
    tool: str, input: dict, context: ToolPermissionContext
) -> PermissionResultAllow | PermissionResultDeny:
    # Check if the model is requesting to bypass the sandbox
    if tool == "Bash" and input.get("dangerouslyDisableSandbox"):
        # The model is requesting to run this command outside the sandbox
        print(f"Unsandboxed command requested: {input.get('command')}")

        if is_command_authorized(input.get("command")):
            return PermissionResultAllow()
        return PermissionResultDeny(
            message="Command not authorized for unsandboxed execution"
        )
    return PermissionResultAllow()

# Required: dummy hook keeps the stream open for can_use_tool
async def dummy_hook(input_data, tool_use_id, context):
    return {"continue_": True}

async def prompt_stream():
    yield {
        "type": "user",
        "message": {"role": "user", "content": "Deploy my application"},
    }

async def main():
    async for message in query(
        prompt=prompt_stream(),
        options=ClaudeAgentOptions(
            sandbox={
                "enabled": True,
                "allowUnsandboxedCommands": True,  # Model can request unsandboxed execution
            },
            permission_mode="default",
            can_use_tool=can_use_tool,
            hooks={"PreToolUse": [HookMatcher(matcher=None, hooks=[dummy_hook])]},
        ),
    ):
        print(message)
```

This pattern enables you to:

- **Audit model requests**: Log when the model requests unsandboxed execution
- **Implement allowlists**: Only permit specific commands to run unsandboxed
- **Add approval workflows**: Require explicit authorization for privileged operations

Commands running with `dangerouslyDisableSandbox: True` have full system access. Ensure your `can_use_tool` handler validates these requests carefully.If `permission_mode` is set to `bypassPermissions` and `allow_unsandboxed_commands` is enabled, the model can autonomously execute commands outside the sandbox without any approval prompts. This combination effectively allows the model to escape sandbox isolation silently.

## [​](https://code.claude.com/docs/en/agent-sdk/python\#see-also)  See also

- [SDK overview](https://code.claude.com/docs/en/agent-sdk/overview) \- General SDK concepts
- [TypeScript SDK reference](https://code.claude.com/docs/en/agent-sdk/typescript) \- TypeScript SDK documentation
- [CLI reference](https://code.claude.com/docs/en/cli-reference) \- Command-line interface
- [Common workflows](https://code.claude.com/docs/en/common-workflows) \- Step-by-step guides

Was this page helpful?

YesNo

[TypeScript V2 (preview)](https://code.claude.com/docs/en/agent-sdk/typescript-v2-preview) [Migration Guide](https://code.claude.com/docs/en/agent-sdk/migration-guide)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.