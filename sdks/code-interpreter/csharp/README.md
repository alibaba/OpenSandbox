# OpenSandbox Code Interpreter SDK for C#

English | [中文](README_zh.md)

A C# SDK for code interpretation with OpenSandbox. Provides high-level APIs for executing code in multiple languages (Python, JavaScript, TypeScript, Go, Java, Bash) within secure sandbox environments.

## Installation

```bash
dotnet add package Alibaba.OpenSandbox.CodeInterpreter
```

## Quick Start

```csharp
using OpenSandbox;
using OpenSandbox.CodeInterpreter;
using OpenSandbox.CodeInterpreter.Models;

// Create a sandbox with a code interpreter image
await using var sandbox = await Sandbox.CreateAsync(new SandboxCreateOptions
{
    Image = "registry.example.com/code-interpreter:latest"
});

// Create a code interpreter from the sandbox
var interpreter = await CodeInterpreter.CreateAsync(sandbox);

// Run Python code
var execution = await interpreter.Codes.RunAsync(
    "print('Hello, World!')",
    new RunCodeOptions { Language = SupportedLanguage.Python });

// Print output
foreach (var msg in execution.Logs.Stdout)
{
    Console.Write(msg.Text);
}
```

## Features

### Supported Languages

- Python (`SupportedLanguage.Python`)
- JavaScript (`SupportedLanguage.JavaScript`)
- TypeScript (`SupportedLanguage.TypeScript`)
- Go (`SupportedLanguage.Go`)
- Java (`SupportedLanguage.Java`)
- Bash (`SupportedLanguage.Bash`)

### Context Management

Contexts allow you to maintain state between code executions:

```csharp
// Create a context for Python
var context = await interpreter.Codes.CreateContextAsync(SupportedLanguage.Python);

// Run code in the context - variables persist
await interpreter.Codes.RunAsync("x = 42", new RunCodeOptions { Context = context });
var result = await interpreter.Codes.RunAsync("print(x)", new RunCodeOptions { Context = context });
// Output: 42

// List all contexts
var contexts = await interpreter.Codes.ListContextsAsync();

// List contexts for a specific language
var pythonContexts = await interpreter.Codes.ListContextsAsync(SupportedLanguage.Python);

// Delete a specific context
await interpreter.Codes.DeleteContextAsync(context.Id!);

// Delete all contexts for a language
await interpreter.Codes.DeleteContextsAsync(SupportedLanguage.Python);
```

### Streaming Execution

For real-time output, use streaming:

```csharp
var request = new RunCodeRequest
{
    Code = "for i in range(5): print(i)",
    Context = new CodeContext { Language = SupportedLanguage.Python }
};

await foreach (var ev in interpreter.Codes.RunStreamAsync(request))
{
    switch (ev.Type)
    {
        case "stdout":
            Console.Write(ev.Text);
            break;
        case "stderr":
            Console.Error.Write(ev.Text);
            break;
        case "result":
            Console.WriteLine($"Result: {ev.Results}");
            break;
        case "error":
            Console.WriteLine($"Error: {ev.Error}");
            break;
    }
}
```

### Event Handlers

Use handlers for fine-grained control over execution events:

```csharp
var execution = await interpreter.Codes.RunAsync(
    "print('Hello')\nprint('World')",
    new RunCodeOptions
    {
        Language = SupportedLanguage.Python,
        Handlers = new ExecutionHandlers
        {
            OnStdout = async msg => Console.Write($"[OUT] {msg.Text}"),
            OnStderr = async msg => Console.Error.Write($"[ERR] {msg.Text}"),
            OnResult = async result => Console.WriteLine($"[RESULT] {result.Text}"),
            OnError = async error => Console.WriteLine($"[ERROR] {error.Name}: {error.Value}"),
            OnExecutionComplete = async complete => Console.WriteLine($"[DONE] Took {complete.ExecutionTimeMs}ms")
        }
    });
```

### Interrupt Execution

Stop a running code execution:

```csharp
var context = await interpreter.Codes.CreateContextAsync(SupportedLanguage.Python);

// Start a long-running task
var task = interpreter.Codes.RunAsync(
    "import time\nwhile True: time.sleep(1)",
    new RunCodeOptions { Context = context });

// Interrupt after some time
await Task.Delay(2000);
await interpreter.Codes.InterruptAsync(context.Id!);
```

### Access Sandbox Services

The code interpreter provides convenient access to underlying sandbox services:

```csharp
// File operations
await interpreter.Files.WriteAsync("/tmp/data.txt", "Hello, World!");
var content = await interpreter.Files.ReadAsync("/tmp/data.txt");

// Shell commands
var result = await interpreter.Commands.RunAsync("ls -la /tmp");

// Metrics
var metrics = await interpreter.Sandbox.GetMetricsAsync();
Console.WriteLine($"CPU: {metrics.CpuUsedPercentage}%, Memory: {metrics.MemoryUsedMiB}MiB");
```

## API Reference

### CodeInterpreter

| Method | Description |
|--------|-------------|
| `CreateAsync(sandbox, options?)` | Creates a code interpreter from a sandbox |

| Property | Description |
|----------|-------------|
| `Sandbox` | The underlying sandbox instance |
| `Codes` | The codes service for code execution |
| `Id` | The sandbox ID |
| `Files` | File system operations |
| `Commands` | Shell command execution |
| `Metrics` | Resource metrics |

### ICodes

| Method | Description |
|--------|-------------|
| `CreateContextAsync(language)` | Creates a new execution context |
| `GetContextAsync(contextId)` | Gets an existing context |
| `ListContextsAsync(language?)` | Lists contexts, optionally filtered by language |
| `DeleteContextAsync(contextId)` | Deletes a specific context |
| `DeleteContextsAsync(language)` | Deletes all contexts for a language |
| `RunAsync(code, options?)` | Executes code and returns the result |
| `RunStreamAsync(request)` | Executes code with streaming output |
| `InterruptAsync(contextId)` | Interrupts a running execution |

## Requirements

- .NET Standard 2.0+ / .NET 6.0+
- OpenSandbox Sandbox SDK (`Alibaba.OpenSandbox`)

## License

Apache License 2.0
