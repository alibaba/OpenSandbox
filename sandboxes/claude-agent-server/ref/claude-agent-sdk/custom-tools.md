[Skip to main content](https://code.claude.com/docs/en/agent-sdk/custom-tools#content-area)

[Claude Code Docs home page![light logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/light.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=78fd01ff4f4340295a4f66e2ea54903c)![dark logo](https://mintcdn.com/claude-code/c5r9_6tjPMzFdDDT/logo/dark.svg?fit=max&auto=format&n=c5r9_6tjPMzFdDDT&q=85&s=1298a0c3b3a1da603b190d0de0e31712)](https://code.claude.com/docs/en/overview)

![US](https://d3gk2c5xim1je2.cloudfront.net/flags/US.svg)

English

Search...

Ctrl KAsk AI

Search...

Navigation

Extend with tools

Give Claude custom tools

[Getting started](https://code.claude.com/docs/en/overview) [Build with Claude Code](https://code.claude.com/docs/en/sub-agents) [Deployment](https://code.claude.com/docs/en/third-party-integrations) [Administration](https://code.claude.com/docs/en/setup) [Configuration](https://code.claude.com/docs/en/settings) [Reference](https://code.claude.com/docs/en/cli-reference) [Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview) [What's New](https://code.claude.com/docs/en/whats-new) [Resources](https://code.claude.com/docs/en/legal-and-compliance)

On this page

- [Quick reference](https://code.claude.com/docs/en/agent-sdk/custom-tools#quick-reference)
- [Create a custom tool](https://code.claude.com/docs/en/agent-sdk/custom-tools#create-a-custom-tool)
- [Weather tool example](https://code.claude.com/docs/en/agent-sdk/custom-tools#weather-tool-example)
- [Call a custom tool](https://code.claude.com/docs/en/agent-sdk/custom-tools#call-a-custom-tool)
- [Add more tools](https://code.claude.com/docs/en/agent-sdk/custom-tools#add-more-tools)
- [Add tool annotations](https://code.claude.com/docs/en/agent-sdk/custom-tools#add-tool-annotations)
- [Control tool access](https://code.claude.com/docs/en/agent-sdk/custom-tools#control-tool-access)
- [Tool name format](https://code.claude.com/docs/en/agent-sdk/custom-tools#tool-name-format)
- [Configure allowed tools](https://code.claude.com/docs/en/agent-sdk/custom-tools#configure-allowed-tools)
- [Handle errors](https://code.claude.com/docs/en/agent-sdk/custom-tools#handle-errors)
- [Return images and resources](https://code.claude.com/docs/en/agent-sdk/custom-tools#return-images-and-resources)
- [Images](https://code.claude.com/docs/en/agent-sdk/custom-tools#images)
- [Resources](https://code.claude.com/docs/en/agent-sdk/custom-tools#resources)
- [Example: unit converter](https://code.claude.com/docs/en/agent-sdk/custom-tools#example-unit-converter)
- [Next steps](https://code.claude.com/docs/en/agent-sdk/custom-tools#next-steps)
- [Related documentation](https://code.claude.com/docs/en/agent-sdk/custom-tools#related-documentation)

Custom tools extend the Agent SDK by letting you define your own functions that Claude can call during a conversation. Using the SDK’s in-process MCP server, you can give Claude access to databases, external APIs, domain-specific logic, or any other capability your application needs.This guide covers how to define tools with input schemas and handlers, bundle them into an MCP server, pass them to `query`, and control which tools Claude can access. It also covers error handling, tool annotations, and returning non-text content like images.

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#quick-reference)  Quick reference

| If you want to… | Do this |
| --- | --- |
| Define a tool | Use [`@tool`](https://code.claude.com/docs/en/agent-sdk/python#tool) (Python) or [`tool()`](https://code.claude.com/docs/en/agent-sdk/typescript#tool) (TypeScript) with a name, description, schema, and handler. See [Create a custom tool](https://code.claude.com/docs/en/agent-sdk/custom-tools#create-a-custom-tool). |
| Register a tool with Claude | Wrap in `create_sdk_mcp_server` / `createSdkMcpServer` and pass to `mcpServers` in `query()`. See [Call a custom tool](https://code.claude.com/docs/en/agent-sdk/custom-tools#call-a-custom-tool). |
| Pre-approve a tool | Add to your allowed tools. See [Configure allowed tools](https://code.claude.com/docs/en/agent-sdk/custom-tools#configure-allowed-tools). |
| Remove a built-in tool from Claude’s context | Pass a `tools` array listing only the built-ins you want. See [Configure allowed tools](https://code.claude.com/docs/en/agent-sdk/custom-tools#configure-allowed-tools). |
| Let Claude call tools in parallel | Set `readOnlyHint: true` on tools with no side effects. See [Add tool annotations](https://code.claude.com/docs/en/agent-sdk/custom-tools#add-tool-annotations). |
| Handle errors without stopping the loop | Return `isError: true` instead of throwing. See [Handle errors](https://code.claude.com/docs/en/agent-sdk/custom-tools#handle-errors). |
| Return images or files | Use `image` or `resource` blocks in the content array. See [Return images and resources](https://code.claude.com/docs/en/agent-sdk/custom-tools#return-images-and-resources). |
| Scale to many tools | Use [tool search](https://code.claude.com/docs/en/agent-sdk/tool-search) to load tools on demand. |

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#create-a-custom-tool)  Create a custom tool

A tool is defined by four parts, passed as arguments to the [`tool()`](https://code.claude.com/docs/en/agent-sdk/typescript#tool) helper in TypeScript or the [`@tool`](https://code.claude.com/docs/en/agent-sdk/python#tool) decorator in Python:

- **Name:** a unique identifier Claude uses to call the tool.
- **Description:** what the tool does. Claude reads this to decide when to call it.
- **Input schema:** the arguments Claude must provide. In TypeScript this is always a [Zod schema](https://zod.dev/), and the handler’s `args` are typed from it automatically. In Python this is a dict mapping names to types, like `{"latitude": float}`, which the SDK converts to JSON Schema for you. The Python decorator also accepts a full [JSON Schema](https://json-schema.org/understanding-json-schema/about) dict directly when you need enums, ranges, optional fields, or nested objects.
- **Handler:**the async function that runs when Claude calls the tool. It receives the validated arguments and must return an object with:

  - `content` (required): an array of result blocks, each with a `type` of `"text"`, `"image"`, or `"resource"`. See [Return images and resources](https://code.claude.com/docs/en/agent-sdk/custom-tools#return-images-and-resources) for non-text blocks.
  - `isError` (optional): set to `true` to signal a tool failure so Claude can react to it. See [Handle errors](https://code.claude.com/docs/en/agent-sdk/custom-tools#handle-errors).

After defining a tool, wrap it in a server with [`createSdkMcpServer`](https://code.claude.com/docs/en/agent-sdk/typescript#create-sdk-mcp-server) (TypeScript) or [`create_sdk_mcp_server`](https://code.claude.com/docs/en/agent-sdk/python#create-sdk-mcp-server) (Python). The server runs in-process inside your application, not as a separate process.

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#weather-tool-example)  Weather tool example

This example defines a `get_temperature` tool and wraps it in an MCP server. It only sets up the tool; to pass it to `query` and run it, see [Call a custom tool](https://code.claude.com/docs/en/agent-sdk/custom-tools#call-a-custom-tool) below.

Python

TypeScript

```
from typing import Any
import httpx
from claude_agent_sdk import tool, create_sdk_mcp_server

# Define a tool: name, description, input schema, handler
@tool(
    "get_temperature",
    "Get the current temperature at a location",
    {"latitude": float, "longitude": float},
)
async def get_temperature(args: dict[str, Any]) -> dict[str, Any]:
    async with httpx.AsyncClient() as client:
        response = await client.get(
            "https://api.open-meteo.com/v1/forecast",
            params={
                "latitude": args["latitude"],
                "longitude": args["longitude"],
                "current": "temperature_2m",
                "temperature_unit": "fahrenheit",
            },
        )
        data = response.json()

    # Return a content array - Claude sees this as the tool result
    return {
        "content": [\
            {\
                "type": "text",\
                "text": f"Temperature: {data['current']['temperature_2m']}°F",\
            }\
        ]
    }

# Wrap the tool in an in-process MCP server
weather_server = create_sdk_mcp_server(
    name="weather",
    version="1.0.0",
    tools=[get_temperature],
)
```

See the [`tool()`](https://code.claude.com/docs/en/agent-sdk/typescript#tool) TypeScript reference or the [`@tool`](https://code.claude.com/docs/en/agent-sdk/python#tool) Python reference for full parameter details, including JSON Schema input formats and return value structure.

To make a parameter optional: in TypeScript, add `.default()` to the Zod field. In Python, the dict schema treats every key as required, so leave the parameter out of the schema, mention it in the description string, and read it with `args.get()` in the handler. The [`get_precipitation_chance` tool below](https://code.claude.com/docs/en/agent-sdk/custom-tools#add-more-tools) shows both patterns.

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#call-a-custom-tool)  Call a custom tool

Pass the MCP server you created to `query` via the `mcpServers` option. The key in `mcpServers` becomes the `{server_name}` segment in each tool’s fully qualified name: `mcp__{server_name}__{tool_name}`. List that name in `allowedTools` so the tool runs without a permission prompt.These snippets reuse the `weatherServer` from the [example above](https://code.claude.com/docs/en/agent-sdk/custom-tools#weather-tool-example) to ask Claude what the weather is in a specific location.

Python

TypeScript

```
import asyncio
from claude_agent_sdk import query, ClaudeAgentOptions, ResultMessage

async def main():
    options = ClaudeAgentOptions(
        mcp_servers={"weather": weather_server},
        allowed_tools=["mcp__weather__get_temperature"],
    )

    async for message in query(
        prompt="What's the temperature in San Francisco?",
        options=options,
    ):
        # ResultMessage is the final message after all tool calls complete
        if isinstance(message, ResultMessage) and message.subtype == "success":
            print(message.result)

asyncio.run(main())
```

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#add-more-tools)  Add more tools

A server holds as many tools as you list in its `tools` array. With more than one tool on a server, you can list each one in `allowedTools` individually or use the wildcard `mcp__weather__*` to cover every tool the server exposes.The example below adds a second tool, `get_precipitation_chance`, to the `weatherServer` from the [weather tool example](https://code.claude.com/docs/en/agent-sdk/custom-tools#weather-tool-example) and rebuilds it with both tools in the array.

Python

TypeScript

```
# Define a second tool for the same server
@tool(
    "get_precipitation_chance",
    "Get the hourly precipitation probability for a location. "
    "Optionally pass 'hours' (1-24) to control how many hours to return.",
    {"latitude": float, "longitude": float},
)
async def get_precipitation_chance(args: dict[str, Any]) -> dict[str, Any]:
    # 'hours' isn't in the schema - read it with .get() to make it optional
    hours = args.get("hours", 12)
    async with httpx.AsyncClient() as client:
        response = await client.get(
            "https://api.open-meteo.com/v1/forecast",
            params={
                "latitude": args["latitude"],
                "longitude": args["longitude"],
                "hourly": "precipitation_probability",
                "forecast_days": 1,
            },
        )
        data = response.json()
    chances = data["hourly"]["precipitation_probability"][:hours]

    return {
        "content": [\
            {\
                "type": "text",\
                "text": f"Next {hours} hours: {'%, '.join(map(str, chances))}%",\
            }\
        ]
    }

# Rebuild the server with both tools in the array
weather_server = create_sdk_mcp_server(
    name="weather",
    version="1.0.0",
    tools=[get_temperature, get_precipitation_chance],
)
```

Every tool in this array consumes context window space on every turn. If you’re defining dozens of tools, see [tool search](https://code.claude.com/docs/en/agent-sdk/tool-search) to load them on demand instead.

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#add-tool-annotations)  Add tool annotations

[Tool annotations](https://modelcontextprotocol.io/docs/concepts/tools#tool-annotations) are optional metadata describing how a tool behaves. Pass them as the fifth argument to `tool()` helper in TypeScript or via the `annotations` keyword argument for the `@tool` decorator in Python. All hint fields are Booleans.

| Field | Default | Meaning |
| --- | --- | --- |
| `readOnlyHint` | `false` | Tool does not modify its environment. Controls whether the tool can be called in parallel with other read-only tools. |
| `destructiveHint` | `true` | Tool may perform destructive updates. Informational only. |
| `idempotentHint` | `false` | Repeated calls with the same arguments have no additional effect. Informational only. |
| `openWorldHint` | `true` | Tool reaches systems outside your process. Informational only. |

Annotations are metadata, not enforcement. A tool marked `readOnlyHint: true` can still write to disk if that’s what the handler does. Keep the annotation accurate to the handler.This example adds `readOnlyHint` to the `get_temperature` tool from the [weather tool example](https://code.claude.com/docs/en/agent-sdk/custom-tools#weather-tool-example).

Python

TypeScript

```
from claude_agent_sdk import tool, ToolAnnotations

@tool(
    "get_temperature",
    "Get the current temperature at a location",
    {"latitude": float, "longitude": float},
    annotations=ToolAnnotations(
        readOnlyHint=True
    ),  # Lets Claude batch this with other read-only calls
)
async def get_temperature(args):
    return {"content": [{"type": "text", "text": "..."}]}
```

See `ToolAnnotations` in the [TypeScript](https://code.claude.com/docs/en/agent-sdk/typescript#tool-annotations) or [Python](https://code.claude.com/docs/en/agent-sdk/python#tool-annotations) reference.

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#control-tool-access)  Control tool access

The [weather tool example](https://code.claude.com/docs/en/agent-sdk/custom-tools#weather-tool-example) registered a server and listed tools in `allowedTools`. This section covers how tool names are constructed and how to scope access when you have multiple tools or want to restrict built-ins.

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#tool-name-format)  Tool name format

When MCP tools are exposed to Claude, their names follow a specific format:

- Pattern: `mcp__{server_name}__{tool_name}`
- Example: A tool named `get_temperature` in server `weather` becomes `mcp__weather__get_temperature`

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#configure-allowed-tools)  Configure allowed tools

The `tools` option and the allowed/disallowed lists operate on separate layers. `tools` controls which built-in tools appear in Claude’s context. Allowed and disallowed tool lists control whether calls are approved or denied once Claude attempts them.

| Option | Layer | Effect |
| --- | --- | --- |
| `tools: ["Read", "Grep"]` | Availability | Only the listed built-ins are in Claude’s context. Unlisted built-ins are removed. MCP tools are unaffected. |
| `tools: []` | Availability | All built-ins are removed. Claude can only use your MCP tools. |
| allowed tools | Permission | Listed tools run without a permission prompt. Unlisted tools remain available; calls go through the [permission flow](https://code.claude.com/docs/en/agent-sdk/permissions). |
| disallowed tools | Permission | Every call to a listed tool is denied. The tool stays in Claude’s context, so Claude may still attempt it before the call is rejected. |

To limit which built-ins Claude can use, prefer `tools` over disallowed tools. Omitting a tool from `tools` removes it from context so Claude never attempts it; listing it in `disallowedTools` (Python: `disallowed_tools`) blocks the call but leaves the tool visible, so Claude may waste a turn trying it. See [Configure permissions](https://code.claude.com/docs/en/agent-sdk/permissions) for the full evaluation order.

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#handle-errors)  Handle errors

How your handler reports errors determines whether the agent loop continues or stops:

| What happens | Result |
| --- | --- |
| Handler throws an uncaught exception | Agent loop stops. Claude never sees the error, and the `query` call fails. |
| Handler catches the error and returns `isError: true` (TS) / `"is_error": True` (Python) | Agent loop continues. Claude sees the error as data and can retry, try a different tool, or explain the failure. |

The example below catches two kinds of failures inside the handler instead of letting them throw. A non-200 HTTP status is caught from the response and returned as an error result. A network error or invalid JSON is caught by the surrounding `try/except` (Python) or `try/catch` (TypeScript) and also returned as an error result. In both cases the handler returns normally and the agent loop continues.

Python

TypeScript

```
import json
import httpx
from typing import Any

@tool(
    "fetch_data",
    "Fetch data from an API",
    {"endpoint": str},  # Simple schema
)
async def fetch_data(args: dict[str, Any]) -> dict[str, Any]:
    try:
        async with httpx.AsyncClient() as client:
            response = await client.get(args["endpoint"])
            if response.status_code != 200:
                # Return the failure as a tool result so Claude can react to it.
                # is_error marks this as a failed call rather than odd-looking data.
                return {
                    "content": [\
                        {\
                            "type": "text",\
                            "text": f"API error: {response.status_code} {response.reason_phrase}",\
                        }\
                    ],
                    "is_error": True,
                }

            data = response.json()
            return {"content": [{"type": "text", "text": json.dumps(data, indent=2)}]}
    except Exception as e:
        # Catching here keeps the agent loop alive. An uncaught exception
        # would end the whole query() call.
        return {
            "content": [{"type": "text", "text": f"Failed to fetch data: {str(e)}"}],
            "is_error": True,
        }
```

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#return-images-and-resources)  Return images and resources

The `content` array in a tool result accepts `text`, `image`, and `resource` blocks. You can mix them in the same response.

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#images)  Images

An image block carries the image bytes inline, encoded as base64. There is no URL field. To return an image that lives at a URL, fetch it in the handler, read the response bytes, and base64-encode them before returning. The result is processed as visual input.

| Field | Type | Notes |
| --- | --- | --- |
| `type` | `"image"` |  |
| `data` | `string` | Base64-encoded bytes. Raw base64 only, no `data:image/...;base64,` prefix |
| `mimeType` | `string` | Required. For example `image/png`, `image/jpeg`, `image/webp`, `image/gif` |

Python

TypeScript

```
import base64
import httpx

# Define a tool that fetches an image from a URL and returns it to Claude
@tool("fetch_image", "Fetch an image from a URL and return it to Claude", {"url": str})
async def fetch_image(args):
    async with httpx.AsyncClient() as client:  # Fetch the image bytes
        response = await client.get(args["url"])

    return {
        "content": [\
            {\
                "type": "image",\
                "data": base64.b64encode(response.content).decode(\
                    "ascii"\
                ),  # Base64-encode the raw bytes\
                "mimeType": response.headers.get(\
                    "content-type", "image/png"\
                ),  # Read MIME type from the response\
            }\
        ]
    }
```

### [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#resources)  Resources

A resource block embeds a piece of content identified by a URI. The URI is a label for Claude to reference; the actual content rides in the block’s `text` or `blob` field. Use this when your tool produces something that makes sense to address by name later, such as a generated file or a record from an external system.

| Field | Type | Notes |
| --- | --- | --- |
| `type` | `"resource"` |  |
| `resource.uri` | `string` | Identifier for the content. Any URI scheme |
| `resource.text` | `string` | The content, if it’s text. Provide this or `blob`, not both |
| `resource.blob` | `string` | The content base64-encoded, if it’s binary |
| `resource.mimeType` | `string` | Optional |

This example shows a resource block returned from inside a tool handler. The URI `file:///tmp/report.md` is a label that Claude can reference later; the SDK does not read from that path.

TypeScript

Python

```
return {
  content: [\
    {\
      type: "resource",\
      resource: {\
        uri: "file:///tmp/report.md", // Label for Claude to reference, not a path the SDK reads\
        mimeType: "text/markdown",\
        text: "# Report\n..." // The actual content, inline\
      }\
    }\
  ]
};
```

These block shapes come from the MCP `CallToolResult` type. See the [MCP specification](https://modelcontextprotocol.io/specification/2025-06-18/server/tools#tool-result) for the full definition.

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#example-unit-converter)  Example: unit converter

This tool converts values between units of length, temperature, and weight. A user can ask “convert 100 kilometers to miles” or “what is 72°F in Celsius,” and Claude picks the right unit type and units from the request.It demonstrates two patterns:

- **Enum schemas:**`unit_type` is constrained to a fixed set of values. In TypeScript, use `z.enum()`. In Python, the dict schema doesn’t support enums, so the full JSON Schema dict is required.
- **Unsupported input handling:** when a conversion pair isn’t found, the handler returns `isError: true` so Claude can tell the user what went wrong rather than treating a failure as a normal result.

Python

TypeScript

```
from typing import Any
from claude_agent_sdk import tool, create_sdk_mcp_server

# z.enum() in TypeScript becomes an "enum" constraint in JSON Schema.
# The dict schema has no equivalent, so full JSON Schema is required.
@tool(
    "convert_units",
    "Convert a value from one unit to another",
    {
        "type": "object",
        "properties": {
            "unit_type": {
                "type": "string",
                "enum": ["length", "temperature", "weight"],
                "description": "Category of unit",
            },
            "from_unit": {
                "type": "string",
                "description": "Unit to convert from, e.g. kilometers, fahrenheit, pounds",
            },
            "to_unit": {"type": "string", "description": "Unit to convert to"},
            "value": {"type": "number", "description": "Value to convert"},
        },
        "required": ["unit_type", "from_unit", "to_unit", "value"],
    },
)
async def convert_units(args: dict[str, Any]) -> dict[str, Any]:
    conversions = {
        "length": {
            "kilometers_to_miles": lambda v: v * 0.621371,
            "miles_to_kilometers": lambda v: v * 1.60934,
            "meters_to_feet": lambda v: v * 3.28084,
            "feet_to_meters": lambda v: v * 0.3048,
        },
        "temperature": {
            "celsius_to_fahrenheit": lambda v: (v * 9) / 5 + 32,
            "fahrenheit_to_celsius": lambda v: (v - 32) * 5 / 9,
            "celsius_to_kelvin": lambda v: v + 273.15,
            "kelvin_to_celsius": lambda v: v - 273.15,
        },
        "weight": {
            "kilograms_to_pounds": lambda v: v * 2.20462,
            "pounds_to_kilograms": lambda v: v * 0.453592,
            "grams_to_ounces": lambda v: v * 0.035274,
            "ounces_to_grams": lambda v: v * 28.3495,
        },
    }

    key = f"{args['from_unit']}_to_{args['to_unit']}"
    fn = conversions.get(args["unit_type"], {}).get(key)

    if not fn:
        return {
            "content": [\
                {\
                    "type": "text",\
                    "text": f"Unsupported conversion: {args['from_unit']} to {args['to_unit']}",\
                }\
            ],
            "is_error": True,
        }

    result = fn(args["value"])
    return {
        "content": [\
            {\
                "type": "text",\
                "text": f"{args['value']} {args['from_unit']} = {result:.4f} {args['to_unit']}",\
            }\
        ]
    }

converter_server = create_sdk_mcp_server(
    name="converter",
    version="1.0.0",
    tools=[convert_units],
)
```

Once the server is defined, pass it to `query` the same way as the weather example. This example sends three different prompts in a loop to show the same tool handling different unit types. For each response, it inspects `AssistantMessage` objects (which contain the tool calls Claude made during that turn) and prints each `ToolUseBlock` before printing the final `ResultMessage` text. This lets you see when Claude is using the tool versus answering from its own knowledge.

Python

TypeScript

```
import asyncio
from claude_agent_sdk import (
    query,
    ClaudeAgentOptions,
    ResultMessage,
    AssistantMessage,
    ToolUseBlock,
)

async def main():
    options = ClaudeAgentOptions(
        mcp_servers={"converter": converter_server},
        allowed_tools=["mcp__converter__convert_units"],
    )

    prompts = [\
        "Convert 100 kilometers to miles.",\
        "What is 72°F in Celsius?",\
        "How many pounds is 5 kilograms?",\
    ]

    for prompt in prompts:
        async for message in query(prompt=prompt, options=options):
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    if isinstance(block, ToolUseBlock):
                        print(f"[tool call] {block.name}({block.input})")
            elif isinstance(message, ResultMessage) and message.subtype == "success":
                print(f"Q: {prompt}\nA: {message.result}\n")

asyncio.run(main())
```

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#next-steps)  Next steps

Custom tools wrap async functions in a standard interface. You can mix the patterns on this page in the same server: a single server can hold a database tool, an API gateway tool, and an image renderer alongside each other.From here:

- If your server grows to dozens of tools, see [tool search](https://code.claude.com/docs/en/agent-sdk/tool-search) to defer loading them until Claude needs them.
- To connect to external MCP servers (filesystem, GitHub, Slack) instead of building your own, see [Connect MCP servers](https://code.claude.com/docs/en/agent-sdk/mcp).
- To control which tools run automatically versus requiring approval, see [Configure permissions](https://code.claude.com/docs/en/agent-sdk/permissions).

## [​](https://code.claude.com/docs/en/agent-sdk/custom-tools\#related-documentation)  Related documentation

- [TypeScript SDK Reference](https://code.claude.com/docs/en/agent-sdk/typescript)
- [Python SDK Reference](https://code.claude.com/docs/en/agent-sdk/python)
- [MCP Documentation](https://modelcontextprotocol.io/)
- [SDK Overview](https://code.claude.com/docs/en/agent-sdk/overview)

Was this page helpful?

YesNo

[Get structured output from agents](https://code.claude.com/docs/en/agent-sdk/structured-outputs) [Connect to external tools with MCP](https://code.claude.com/docs/en/agent-sdk/mcp)

Ctrl+I

Assistant

Responses are generated using AI and may contain mistakes.