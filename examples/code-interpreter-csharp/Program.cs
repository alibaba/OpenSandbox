// Copyright 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

using OpenSandbox;
using OpenSandbox.CodeInterpreter;
using OpenSandbox.CodeInterpreter.Models;
using OpenSandbox.Config;
using OpenSandbox.Models;

// Read configuration from environment variables
var domain = Environment.GetEnvironmentVariable("SANDBOX_DOMAIN") ?? "localhost:8080";
var apiKey = Environment.GetEnvironmentVariable("SANDBOX_API_KEY");
var image = Environment.GetEnvironmentVariable("SANDBOX_IMAGE")
            ?? "opensandbox/code-interpreter:v1.0.1";

Console.WriteLine("=== OpenSandbox Code Interpreter C# Example ===\n");
Console.WriteLine($"Domain: {domain}");
Console.WriteLine($"Image: {image}\n");

// Create connection configuration
var config = new ConnectionConfig(new ConnectionConfigOptions
{
    Domain = domain,
    ApiKey = apiKey,
    RequestTimeoutSeconds = 160
});

try
{
    // Create sandbox with code interpreter image
    Console.WriteLine("Creating sandbox...");
    await using var sandbox = await Sandbox.CreateAsync(new SandboxCreateOptions
    {
        ConnectionConfig = config,
        Image = image,
        Entrypoint = new[] { "/opt/opensandbox/code-interpreter.sh" },
        TimeoutSeconds = 10 * 60,
        ReadyTimeoutSeconds = 10 * 60
    });

    Console.WriteLine($"Sandbox created: {sandbox.Id}\n");

    // Create code interpreter
    var interpreter = await CodeInterpreter.CreateAsync(sandbox);

    // === Python Example ===
    Console.WriteLine("=== Python Example ===");
    var pyExec = await interpreter.Codes.RunAsync(
        """
        import platform
        print('Hello from Python!')
        result = {'py': platform.python_version(), 'sum': 2 + 2}
        result
        """,
        new RunCodeOptions { Language = SupportedLanguage.Python });

    foreach (var msg in pyExec.Logs.Stdout)
    {
        Console.WriteLine($"[Python stdout] {msg.Text}");
    }

    foreach (var res in pyExec.Results)
    {
        Console.WriteLine($"[Python result] {res.Text}");
    }

    if (pyExec.Error != null)
    {
        Console.WriteLine($"[Python error] {pyExec.Error.Name}: {pyExec.Error.Value}");
    }

    // === Java Example ===
    Console.WriteLine("\n=== Java Example ===");
    var javaExec = await interpreter.Codes.RunAsync(
        """
        System.out.println("Hello from Java!");
        int result = 2 + 3;
        System.out.println("2 + 3 = " + result);
        result
        """,
        new RunCodeOptions { Language = SupportedLanguage.Java });

    foreach (var msg in javaExec.Logs.Stdout)
    {
        Console.WriteLine($"[Java stdout] {msg.Text}");
    }

    foreach (var res in javaExec.Results)
    {
        Console.WriteLine($"[Java result] {res.Text}");
    }

    if (javaExec.Error != null)
    {
        Console.WriteLine($"[Java error] {javaExec.Error.Name}: {javaExec.Error.Value}");
    }

    // === Go Example ===
    Console.WriteLine("\n=== Go Example ===");
    var goExec = await interpreter.Codes.RunAsync(
        """
        package main
        import "fmt"
        func main() {
            fmt.Println("Hello from Go!")
            sum := 3 + 4
            fmt.Println("3 + 4 =", sum)
        }
        """,
        new RunCodeOptions { Language = SupportedLanguage.Go });

    foreach (var msg in goExec.Logs.Stdout)
    {
        Console.WriteLine($"[Go stdout] {msg.Text}");
    }

    if (goExec.Error != null)
    {
        Console.WriteLine($"[Go error] {goExec.Error.Name}: {goExec.Error.Value}");
    }

    // === TypeScript Example ===
    Console.WriteLine("\n=== TypeScript Example ===");
    var tsExec = await interpreter.Codes.RunAsync(
        """
        console.log('Hello from TypeScript!');
        const nums: number[] = [1, 2, 3];
        console.log('sum =', nums.reduce((a, b) => a + b, 0));
        """,
        new RunCodeOptions { Language = SupportedLanguage.TypeScript });

    foreach (var msg in tsExec.Logs.Stdout)
    {
        Console.WriteLine($"[TypeScript stdout] {msg.Text}");
    }

    if (tsExec.Error != null)
    {
        Console.WriteLine($"[TypeScript error] {tsExec.Error.Name}: {tsExec.Error.Value}");
    }

    // === Context Management Example ===
    Console.WriteLine("\n=== Context Management Example ===");

    // Create a persistent context
    var ctx = await interpreter.Codes.CreateContextAsync(SupportedLanguage.Python);
    Console.WriteLine($"Created context: {ctx.Id}");

    // Run code that sets a variable
    await interpreter.Codes.RunAsync("x = 42", new RunCodeOptions { Context = ctx });
    Console.WriteLine("Set x = 42 in context");

    // Run code that uses the variable (state persists)
    var ctxExec = await interpreter.Codes.RunAsync("print(f'x = {x}')\nx * 2", new RunCodeOptions { Context = ctx });
    foreach (var msg in ctxExec.Logs.Stdout)
    {
        Console.WriteLine($"[Context stdout] {msg.Text}");
    }

    foreach (var res in ctxExec.Results)
    {
        Console.WriteLine($"[Context result] {res.Text}");
    }

    // List contexts
    var contexts = await interpreter.Codes.ListContextsAsync();
    Console.WriteLine($"Total contexts: {contexts.Count}");

    // Delete context
    await interpreter.Codes.DeleteContextAsync(ctx.Id!);
    Console.WriteLine($"Deleted context: {ctx.Id}");

    // === Streaming Example ===
    Console.WriteLine("\n=== Streaming Example ===");
    var streamExec = await interpreter.Codes.RunAsync(
        """
        import time
        for i in range(5):
            print(f'Count: {i}')
            time.sleep(0.1)
        print('Done!')
        """,
        new RunCodeOptions
        {
            Language = SupportedLanguage.Python,
            Handlers = new ExecutionHandlers
            {
                OnStdout = async msg => Console.Write($"[Stream] {msg.Text}"),
                OnExecutionComplete = async complete =>
                    Console.WriteLine($"[Stream] Completed in {complete.ExecutionTimeMs}ms")
            }
        });

    // === File Operations Example ===
    Console.WriteLine("\n=== File Operations Example ===");

    // Write a file
    await interpreter.Files.WriteFilesAsync(new[]
    {
        new WriteEntry { Path = "/tmp/hello.txt", Data = "Hello from C#!", Mode = 420 }
    });
    Console.WriteLine("Wrote /tmp/hello.txt");

    // Read the file back
    var content = await interpreter.Files.ReadFileAsync("/tmp/hello.txt");
    Console.WriteLine($"Read content: {content}");

    // Execute shell command
    var cmdExec = await interpreter.Commands.RunAsync("cat /tmp/hello.txt && echo ' - via shell'");
    foreach (var msg in cmdExec.Logs.Stdout)
    {
        Console.WriteLine($"[Shell] {msg.Text}");
    }

    // Cleanup
    Console.WriteLine("\n=== Cleanup ===");
    await sandbox.KillAsync();
    Console.WriteLine("Sandbox terminated.");
}
catch (Exception ex)
{
    Console.Error.WriteLine($"Error: {ex.Message}");
    if (ex.InnerException != null)
    {
        Console.Error.WriteLine($"Inner: {ex.InnerException.Message}");
    }

    Environment.Exit(1);
}

Console.WriteLine("\n=== Example completed successfully! ===");