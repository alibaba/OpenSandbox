# Code Interpreter C# Example

English | [中文](README_zh.md)

Complete demonstration of running multi-language code using the OpenSandbox Code Interpreter C# SDK.

## Prerequisites

- .NET 8.0 SDK or later
- Docker (for local execution)
- OpenSandbox server running

## Getting Code Interpreter Image

Pull the prebuilt image from a registry:

```shell
docker pull opensandbox/code-interpreter:v1.0.1

# Or use Alibaba Cloud registry (China)
# docker pull sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.1
```

## Start OpenSandbox Server

Start the local OpenSandbox server:

```shell
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/server
cp example.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main
```

## Run the Example

```shell
cd examples/code-interpreter-csharp

# Run with default settings (localhost:8080)
dotnet run

# Or with custom environment variables
SANDBOX_DOMAIN=localhost:8080 SANDBOX_IMAGE=opensandbox/code-interpreter:v1.0.1 dotnet run
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SANDBOX_DOMAIN` | Sandbox service address | `localhost:8080` |
| `SANDBOX_API_KEY` | API key for authentication | (none) |
| `SANDBOX_IMAGE` | Docker image to use | `opensandbox/code-interpreter:v1.0.1` |

## What This Example Demonstrates

1. **Multi-language Code Execution**: Python, Java, Go, TypeScript
2. **Context Management**: Create, use, list, and delete execution contexts
3. **State Persistence**: Variables persist across runs within a context
4. **Streaming Output**: Real-time stdout/stderr handling with event handlers
5. **File Operations**: Write and read files in the sandbox
6. **Shell Commands**: Execute shell commands via the Commands service

## Example Output

```text
=== OpenSandbox Code Interpreter C# Example ===

Domain: localhost:8080
Image: opensandbox/code-interpreter:v1.0.1

Creating sandbox...
Sandbox created: sbx-abc123

=== Python Example ===
[Python stdout] Hello from Python!
[Python result] {'py': '3.11.0', 'sum': 4}

=== Java Example ===
[Java stdout] Hello from Java!
[Java stdout] 2 + 3 = 5
[Java result] 5

=== Go Example ===
[Go stdout] Hello from Go!
[Go stdout] 3 + 4 = 7

=== TypeScript Example ===
[TypeScript stdout] Hello from TypeScript!
[TypeScript stdout] sum = 6

=== Context Management Example ===
Created context: ctx-xyz789
Set x = 42 in context
[Context stdout] x = 42
[Context result] 84
Total contexts: 1
Deleted context: ctx-xyz789

=== Streaming Example ===
[Stream] Count: 0
[Stream] Count: 1
[Stream] Count: 2
[Stream] Count: 3
[Stream] Count: 4
[Stream] Done!
[Stream] Completed in 523ms

=== File Operations Example ===
Wrote /tmp/hello.txt
Read content: Hello from C#!
[Shell] Hello from C#! - via shell

=== Cleanup ===
Sandbox terminated.

=== Example completed successfully! ===
```

## Code Structure

```csharp
// 1. Create connection configuration
var config = new ConnectionConfig(new ConnectionConfigOptions
{
    Domain = "localhost:8080",
    ApiKey = "your-api-key"
});

// 2. Create sandbox with code interpreter image
await using var sandbox = await Sandbox.CreateAsync(new SandboxCreateOptions
{
    ConnectionConfig = config,
    Image = "opensandbox/code-interpreter:v1.0.1",
    Entrypoint = new[] { "/opt/opensandbox/code-interpreter.sh" }
});

// 3. Create code interpreter
var interpreter = await CodeInterpreter.CreateAsync(sandbox);

// 4. Run code in various languages
var result = await interpreter.Codes.RunAsync(
    "print('Hello!')",
    new RunCodeOptions { Language = SupportedLanguage.Python });

// 5. Access results
foreach (var msg in result.Logs.Stdout)
{
    Console.WriteLine(msg.Text);
}
```

## Related Documentation

- [OpenSandbox C# SDK](../../sdks/sandbox/csharp/README.md)
- [Code Interpreter C# SDK](../../sdks/code-interpreter/csharp/README.md)
- [Code Interpreter Python Example](../code-interpreter/README.md)
