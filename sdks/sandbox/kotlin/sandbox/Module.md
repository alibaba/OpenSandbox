# Module sandbox
The Open Sandbox SDK provides a comprehensive interface for creating and managing secure, isolated execution environments. Built with Kotlin and designed for both Kotlin and Java applications, it offers high-level abstractions for container-based sandboxing with advanced features like file system operations, command execution, and lifecycle management.

## Features

- **🔒 Secure Isolation**: Complete Linux OS access in isolated containers
- **📁 File System Operations**: Create, read, update, delete files and directories
- **⚡ Multi-language Execution**: Support for Python, Java, Bash, and other languages
- **🎛️ Real-time Command Execution**: Streaming output with timeout handling
- **📊 Resource Management**: CPU, memory, and storage constraints
- **🔄 Lifecycle Management**: Create, pause, resume, terminate operations
- **💚 Health Monitoring**: Automatic readiness detection and status tracking
- **🏗️ Fluent API**: Type-safe builder pattern with DSL support

## Quick Start

### Basic Usage

```kotlin
// Create a simple Python sandbox
val sandbox = Sandbox.builder()
    .image("python:3.11")
    .build()

// Write and execute code
sandbox.filesystem.writeFile("hello.py", "print('Hello, World!')")
val result = sandbox.commands.execute("python hello.py")
println(result.stdout) // Output: Hello, World!

// Clean up
sandbox.terminate()
```

### Advanced Configuration

```kotlin
val sandbox = Sandbox.builder()
    .image("myregistry.com/app:latest")
    .imageAuth("username", "password")
    .resource {
        put("cpu", "1000m")      // 1 CPU core
        put("memory", "2Gi")     // 2 GB RAM
        put("gpu", "1")          // 1 GPU device
    }
    .environment {
        put("DEBUG", "true")
        put("LOG_LEVEL", "info")
    }
    .metadata {
        put("project", "my-project")
        put("team", "backend")
    }
    .timeout(Duration.ofMinutes(30))
    .readyTimeout(Duration.ofSeconds(120))
    .build()
```

### File System Operations

```kotlin
// File operations
sandbox.filesystem.writeFile("config.json", """{"debug": true}""")
val content = sandbox.filesystem.readFile("config.json")
val exists = sandbox.filesystem.exists("config.json")

// Directory operations
sandbox.filesystem.createDirectory("workspace")
val files = sandbox.filesystem.listDirectory("workspace")

// Advanced operations
sandbox.filesystem.copy("source.txt", "backup.txt")
sandbox.filesystem.move("old.txt", "new.txt")
sandbox.filesystem.setPermissions("script.sh", "755")
```

### Command Execution

```kotlin
// Synchronous execution
val result = sandbox.commands.execute("ls -la")
println("Exit code: ${result.exitCode}")
println("Output: ${result.stdout}")

// With environment and working directory
val result = sandbox.commands.execute(
    command = "npm install",
    workingDirectory = "/app",
    environment = mapOf("NODE_ENV" to "production"),
    timeout = Duration.ofMinutes(5)
)

// Streaming execution
sandbox.commands.executeStreaming("long-running-task").collect { event ->
    when (event) {
        is StreamEvent.Stdout -> print(event.data)
        is StreamEvent.Stderr -> System.err.print(event.data)
        is StreamEvent.Completed -> println("Exit code: ${event.exitCode}")
        is StreamEvent.Error -> println("Error: ${event.message}")
    }
}
```

## Key Components

### Sandbox
The primary interface for interacting with sandbox environments. Provides methods for:
- Creating new sandbox instances with fluent configuration
- Connecting to existing sandboxes by ID
- Managing sandbox lifecycle (pause, resume, terminate)
- Accessing file system and command execution capabilities
- Health monitoring and status checking

### SandboxBuilder
A fluent builder for configuring sandbox creation with:
- Container image specification with authentication
- Resource limits (CPU, memory, GPU)
- Environment variables and metadata
- Timeout and readiness configuration
- API client configuration

### Operations Interfaces

#### FileSystemOperations
- **File Operations**: Read, write, copy, move, delete files
- **Directory Operations**: Create, list, navigate directories
- **Metadata Operations**: Get file info, set permissions, check existence
- **Batch Operations**: Replace multiple files atomically

#### CommandOperations
- **Synchronous Execution**: Run commands and wait for completion
- **Streaming Execution**: Real-time output streaming with Flow API
- **Background Execution**: Non-blocking command execution
- **Shell Scripts**: Execute multi-line shell scripts
- **Command Utilities**: Check command availability, get versions

### Domain Models
- **SandboxState**: Lifecycle states (PROVISIONING, RUNNING, PAUSED, etc.)
- **ExecutionResult**: Command execution output with exit code and timing
- **FileInfo**: File system entry information with permissions and metadata
- **Resource Maps**: Kubernetes-style resource specifications as key-value pairs
- **StreamEvent**: Real-time command output events

### Infrastructure Layer
- **ApiClientAdapter**: HTTP client management with authentication and retry logic
- **SandboxConfig**: Centralized configuration with environment variable support
- **ModelAdapter**: Translation between OpenAPI models and domain types
- **Exception Hierarchy**: Specific exceptions for different error scenarios

## Architecture

The SDK follows a clean architecture with clear separation of concerns:

```
┌─────────────────────────────────────────┐
│              Public API                 │
│         (Sandbox, SandboxBuilder)       │
├─────────────────────────────────────────┤
│            Operations Layer             │
│     (FileSystem, Command, Lifecycle)    │
├─────────────────────────────────────────┤
│           Infrastructure Layer          │
│      (API Clients, Configuration)       │
├─────────────────────────────────────────┤
│             Domain Layer                │
│        (Types, Exceptions, Models)      │
└─────────────────────────────────────────┘
```

## Java Interoperability

The SDK is fully compatible with Java applications:

```java
// Java usage example
Sandbox sandbox = Sandbox.builder()
    .image("openjdk:11")
    .resource(Map.of(
        "cpu", "1000m",
        "memory", "2Gi"
    ))
    .build();

ExecutionResult result = sandbox.getCommands().execute("java -version");
System.out.println("Java version: " + result.getStdout());

sandbox.terminate();
```

## Best Practices

### Resource Management
Always use try-with-resources or explicit cleanup:

```kotlin
// Using AutoCloseable
Sandbox.builder()
    .image("python:3.11")
    .build()
    .use { sandbox ->
        // Use sandbox - automatically terminated when exiting
        sandbox.filesystem.writeFile("script.py", "print('Hello')")
        sandbox.commands.execute("python script.py")
    }
```

### Error Handling
Handle specific exception types:

```kotlin
try {
    val sandbox = Sandbox.builder().image("python:3.11").build()
} catch (e: AuthenticationException) {
    // Handle auth errors
} catch (e: TimeoutException) {
    // Handle timeouts
} catch (e: SandboxException) {
    // Handle general sandbox errors
}
```

## Usage Examples

See the [samples](../../samples/) directory for comprehensive usage examples including:
- Basic sandbox creation and usage
- Advanced configuration scenarios
- File system operations
- Command execution patterns
- Error handling strategies
