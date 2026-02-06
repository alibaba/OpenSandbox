# OpenSandbox Architecture

OpenSandbox is a universal sandbox platform designed for AI application scenarios, providing a complete solution with multi-language SDKs, standardized sandbox protocols, and flexible runtime implementations. This document describes the overall architecture and design philosophy of OpenSandbox.

## Architecture Overview

![OpenSandbox Architecture](assets/architecture.svg)

The OpenSandbox architecture consists of four main layers:

1. **SDKs Layer** - Client libraries for interacting with sandboxes
2. **Specs Layer** - OpenAPI specifications defining the protocols
3. **Runtime Layer** - Server implementations managing sandbox lifecycle
4. **Sandbox Instances Layer** - Running sandbox containers with injected execution daemons

## 1. OpenSandbox SDKs

The SDK layer provides high-level abstractions for developers to interact with sandboxes. It handles communication with both the Sandbox Lifecycle API and the Sandbox Execution API.

### Core SDK Components

#### 1.1 Sandbox

The `Sandbox` class is the primary entry point for managing sandbox lifecycle:

- **Create**: Provision new sandbox instances from container images
- **Manage**: Monitor sandbox state, renew expiration, retrieve endpoints
- **Destroy**: Terminate sandbox instances when no longer needed

**Key Features:**
- Async/await support for non-blocking operations
- Automatic state polling for provisioning progress
- Resource quota management (CPU, memory, GPU)
- Metadata and environment variable injection
- TTL-based automatic expiration with renewal

#### 1.2 Filesystem

The `Filesystem` component provides comprehensive file operations within sandboxes:

- **CRUD Operations**: Create, read, update, and delete files and directories
- **Bulk Operations**: Upload/download multiple files efficiently
- **Search**: Glob-based file searching with pattern matching
- **Permissions**: Manage file ownership, group, and mode (chmod)
- **Metadata**: Retrieve file info including size, timestamps, permissions

**Use Cases:**
- Uploading code files and dependencies
- Downloading execution results and artifacts
- Managing workspace directories
- Searching for files by pattern

#### 1.3 Commands

The `Commands` component enables shell command execution within sandboxes:

- **Foreground Execution**: Run commands synchronously with real-time output streaming
- **Background Execution**: Launch long-running processes in detached mode
- **Stream Support**: Capture stdout/stderr via Server-Sent Events (SSE)
- **Process Control**: Interrupt running commands via context cancellation
- **Working Directory**: Specify custom working directory for command execution

**Use Cases:**
- Running build commands (e.g., `npm install`, `pip install`)
- Executing system utilities (e.g., `git`, `docker`)
- Starting web servers or services
- Running test suites

#### 1.4 CodeInterpreter

The `CodeInterpreter` component provides stateful code execution across multiple programming languages:

- **Multi-Language Support**: Python, Java, JavaScript, TypeScript, Go, Bash
- **Session Management**: Maintain execution state across multiple code blocks
- **Jupyter Integration**: Built on Jupyter kernel protocol for robust execution
- **Result Streaming**: Real-time output via SSE with execution counts
- **Error Handling**: Structured error responses with tracebacks

**Key Features:**
- Variable persistence across executions within same session
- Display data in multiple MIME types (text, HTML, images)
- Execution interruption support
- Execution timing and performance metrics

**Use Cases:**
- Interactive coding environments (e.g., Jupyter notebooks)
- AI code generation and execution
- Data analysis and visualization
- Educational coding platforms

### SDK Language Support

OpenSandbox provides SDKs in multiple languages:

- **Python SDK** (`sdks/sandbox/python`, `sdks/code-interpreter/python`)
- **Java/Kotlin SDK** (`sdks/sandbox/kotlin`, `sdks/code-interpreter/kotlin`)
- **TypeScript SDK** (Roadmap)

All SDKs follow the same design patterns and provide consistent APIs across languages.

## 2. OpenSandbox Specs

The Specs layer defines two core OpenAPI specifications that establish the contract between SDKs and runtime implementations.

### 2.1 Sandbox Lifecycle Spec

**File**: `specs/sandbox-lifecycle.yml`

The Lifecycle Spec defines the API for managing sandbox instances throughout their lifecycle.

#### Core Operations

| Operation | Endpoint | Description |
|-----------|----------|-------------|
| **Create** | `POST /sandboxes` | Create a new sandbox from a container image |
| **List** | `GET /sandboxes` | List sandboxes with filtering and pagination |
| **Get** | `GET /sandboxes/{id}` | Retrieve sandbox details and status |
| **Delete** | `DELETE /sandboxes/{id}` | Terminate a sandbox |
| **Pause** | `POST /sandboxes/{id}/pause` | Pause a running sandbox |
| **Resume** | `POST /sandboxes/{id}/resume` | Resume a paused sandbox |
| **Renew** | `POST /sandboxes/{id}/renew-expiration` | Extend sandbox TTL |
| **Endpoint** | `GET /sandboxes/{id}/endpoints/{port}` | Get public URL for a port |

### 2.2 Sandbox Execution Spec

**File**: `specs/execd-api.yaml`

The Execution Spec defines the API for interacting with running sandbox instances. This API is implemented by the `execd` daemon injected into each sandbox.

#### API Categories

**Health**
- `GET /ping` - Health check

**Code Interpreting**
- `POST /code/context` - Create execution context
- `POST /code` - Execute code with streaming output
- `DELETE /code` - Interrupt code execution

**Command Execution**
- `POST /command` - Execute shell command
- `DELETE /command` - Interrupt command

**Filesystem**
- `GET /files/info` - Get file metadata
- `DELETE /files` - Remove files
- `POST /files/permissions` - Change permissions
- `POST /files/mv` - Rename/move files
- `GET /files/search` - Search files by glob pattern
- `POST /files/replace` - Replace file content
- `POST /files/upload` - Upload files
- `GET /files/download` - Download files
- `POST /directories` - Create directories
- `DELETE /directories` - Remove directories

**Metrics**
- `GET /metrics` - Get system metrics snapshot
- `GET /metrics/watch` - Stream metrics via SSE

## 3. OpenSandbox Runtime

The Runtime layer implements the Sandbox Lifecycle Spec and manages the orchestration of sandbox containers.

### 3.1 Server Architecture

**Location**: `server/`

The OpenSandbox server is a FastAPI-based service providing:

- **Lifecycle Management**: Create, monitor, pause, resume, and terminate sandboxes
- **Pluggable Runtimes**: Docker (production-ready), Kubernetes (roadmap)
- **Async Provisioning**: Background creation to reduce latency
- **Automatic Expiration**: Configurable TTL with renewal support
- **Access Control**: API key authentication
- **Observability**: Unified status tracking with transition logging

### 3.2 Runtime Implementations

#### Docker Runtime (Ready)

**Features:**
- Direct Docker API integration
- Two networking modes:
  - **Host Mode**: Containers share host network (single instance)
  - **Bridge Mode**: Isolated networking with HTTP routing
- Container lifecycle management
- Resource quota enforcement
- Private registry authentication
- Volume mounting for execd injection
- Automatic cleanup on expiration

**Key Responsibilities:**
1. Pull container images (with auth support)
2. Create containers with resource limits
3. Inject execd binary and start script
4. Monitor container state
5. Handle pause/resume operations
6. Clean up terminated containers

#### Kubernetes Runtime (Roadmap)

**Planned Features:**
- Pod-based sandbox instances
- Native Kubernetes resource management
- Multi-tenancy support
- Horizontal scaling
- Network policies
- Persistent volume claims
- Service mesh integration

#### Custom Runtime

The pluggable architecture allows implementing custom runtimes by:
1. Implementing the Lifecycle Spec APIs
2. Managing sandbox provisioning and cleanup
3. Injecting execd into sandbox instances
4. Reporting sandbox state transitions

### 3.3 Networking and Routing

#### Sandbox Router

**Purpose**: Provides HTTP/HTTPS load balancing to sandbox instance ports.

**Features:**
- Dynamic endpoint generation based on sandbox ID and port
- Supports both domain-based and wildcard routing
- Reverse proxy to sandbox container ports
- Automatic cleanup when sandbox terminates

**Endpoint Format**: `{domain}/sandboxes/{sandboxId}/port/{port}`

**Use Cases:**
- Accessing web applications running in sandboxes
- Connecting to development servers (e.g., VS Code Server)
- Exposing APIs and services
- VNC and remote desktop access

## 4. Sandbox Instances

Sandbox instances are running containers that host user workloads with an injected execution daemon.

### 4.1 Container Structure

Each sandbox instance consists of:

1. **Base Container**: User-specified image (e.g., `ubuntu:22.04`, `python:3.11`)
2. **execd Daemon**: Injected execution agent implementing the Execution Spec
3. **Entrypoint Process**: User-defined main process

### 4.2 execd - Execution Daemon

**Location**: `components/execd/`

execd is a Go-based HTTP daemon built on the Beego framework.

#### Core Responsibilities

1. **Code Execution**: Manage Jupyter kernel sessions for multi-language code execution
2. **Command Execution**: Run shell commands with output streaming
3. **File Operations**: Provide filesystem API for remote file management
4. **Metrics Collection**: Monitor and report CPU, memory usage

#### Architecture

**Technology Stack:**
- **Language**: Go 1.24+
- **Web Framework**: Beego
- **Jupyter Integration**: WebSocket-based Jupyter protocol client
- **Streaming**: Server-Sent Events (SSE)

**Package Structure:**
- `pkg/flag/` - Configuration and CLI flags
- `pkg/web/` - HTTP layer (controllers, models, router)
- `pkg/runtime/` - Execution dispatcher
- `pkg/jupyter/` - Jupyter kernel client
- `pkg/util/` - Utilities and helpers

#### Jupyter Integration

execd integrates with Jupyter Server running inside the container:

1. **Session Management**: Create and maintain kernel sessions
2. **WebSocket Communication**: Real-time bidirectional communication
3. **Message Protocol**: Jupyter message spec implementation
4. **Stream Parsing**: Parse execution results, outputs, errors

**Supported Kernels:**
- Python (IPython)
- Java (IJava)
- JavaScript (IJavaScript)
- TypeScript (ITypeScript)
- Go (gophernotes)
- Bash

### 4.3 Injection Mechanism

The execd daemon is injected into sandbox containers during creation:

**Docker Runtime Injection Process:**

1. **Pull execd Image**: Retrieve the execd container image
2. **Extract Binary**: Copy execd binary from image to temporary location
3. **Volume Mount**: Mount execd binary and startup script into target container
4. **Entrypoint Override**: Modify container entrypoint to start execd first
5. **User Process Launch**: execd forks and executes the user's entrypoint

**Startup Sequence:**

```bash
# Container starts with modified entrypoint
/opt/opensandbox/start.sh
  ↓
# Start Jupyter Server
jupyter notebook --port=54321 --no-browser --ip=0.0.0.0
  ↓
# Start execd daemon
/opt/opensandbox/execd --jupyter-host=http://127.0.0.1:54321 --port=44772
  ↓
# Execute user entrypoint
exec "${USER_ENTRYPOINT[@]}"
```

**Benefits:**
- Transparent to user code
- No image modification required
- Dynamic injection at runtime
- Works with any base image

## 5. Communication Flow

### 5.1 Sandbox Creation Flow

```
User/SDK
   │
   │ 1. POST /sandboxes (image, entrypoint, resources)
   ▼
Server (Lifecycle API)
   │
   │ 2. Pull container image
   │ 3. Inject execd binary
   │ 4. Create container with entrypoint override
   │ 5. Start container
   ▼
Sandbox Instance
   │
   │ 6. Start execd daemon
   │ 7. Start Jupyter Server
   │ 8. Execute user entrypoint
   ▼
Running (State)
```

### 5.2 Code Execution Flow

```
User/SDK
   │
   │ 1. Create sandbox
   │ 2. Get execd endpoint
   ▼
CodeInterpreter SDK
   │
   │ 3. POST /code/context (create session)
   │ 4. POST /code (execute code)
   ▼
execd (Execution API)
   │
   │ 5. Route to Jupyter runtime
   ▼
Jupyter Runtime
   │
   │ 6. WebSocket to Jupyter Server
   │ 7. Send execute_request
   ▼
Jupyter Kernel (Python/Java/etc.)
   │
   │ 8. Execute code
   │ 9. Stream output events
   ▼
execd
   │
   │ 10. Convert to SSE events
   │ 11. Stream to client
   ▼
CodeInterpreter SDK
   │
   │ 12. Parse events
   │ 13. Return result to user
   ▼
User/Application
```

### 5.3 File Operations Flow

```
User/SDK
   │
   │ 1. Upload files
   ▼
Filesystem SDK
   │
   │ 2. POST /files/upload (multipart)
   ▼
execd (Execution API)
   │
   │ 3. Write to filesystem
   │ 4. Set permissions
   ▼
Sandbox Container Filesystem
```

## 6. Design Principles

### 6.1 Protocol-First Design

- All interactions defined by OpenAPI specifications
- Clear contracts between components
- Enables polyglot implementations
- Supports custom runtime implementations

### 6.2 Separation of Concerns

- **SDK**: Client-side abstraction and convenience
- **Specs**: Protocol definition and documentation
- **Runtime**: Sandbox orchestration and lifecycle
- **execd**: In-sandbox execution and operations

### 6.3 Extensibility

- Pluggable runtime implementations
- Custom sandbox images
- Multiple SDK languages
- Additional Jupyter kernels

### 6.4 Security

- API key authentication for lifecycle operations
- Token-based authentication for execution operations
- Isolated sandbox environments
- Resource quota enforcement
- Network isolation options

### 6.5 Observability

- Structured state transitions
- Real-time metrics streaming
- Comprehensive logging
- Health check endpoints

## 7. Use Cases

### 7.1 AI Code Generation and Execution

AI models (like Claude, GPT-4, Gemini) generate code that needs to be executed safely:

- **Isolation**: Run untrusted AI-generated code in sandboxes
- **Multi-Language**: Support various programming languages
- **Iteration**: Maintain state across multiple code generations
- **Feedback**: Capture execution results and errors for AI refinement

**Examples**: [claude-code](../examples/claude-code/), [gemini-cli](../examples/gemini-cli/), [codex-cli](../examples/codex-cli/)

### 7.2 Interactive Coding Environments

Build web-based coding platforms and notebooks:

- **Code Execution**: Run code in isolated environments
- **File Management**: Upload/download project files
- **Terminal Access**: Execute shell commands
- **Collaboration**: Share sandbox instances

**Examples**: [code-interpreter](../examples/code-interpreter/)

### 7.3 Browser Automation and Testing

Automate web browsers for testing and scraping:

- **Headless Browsers**: Chrome, Playwright
- **Remote Debugging**: DevTools protocol
- **VNC Access**: Visual debugging
- **Network Isolation**: Controlled environment

**Examples**: [chrome](../examples/chrome/), [playwright](../examples/playwright/)

### 7.4 Remote Development Environments

Provide cloud-based development workspaces:

- **VS Code Server**: Full IDE in browser
- **Desktop Environments**: VNC-based desktops
- **Tool Pre-installation**: Language runtimes, build tools
- **Port Forwarding**: Access development servers

**Examples**: [vscode](../examples/vscode/), [desktop](../examples/desktop/)

### 7.5 Continuous Integration and Testing

Run build and test pipelines in isolated environments:

- **Reproducible Builds**: Consistent container images
- **Parallel Execution**: Multiple sandbox instances
- **Artifact Collection**: Download build outputs
- **Resource Limits**: Prevent resource exhaustion

## 8. Conclusion

OpenSandbox provides a complete, production-ready platform for building AI-powered applications that require safe code execution, file management, and command execution in isolated environments. The architecture is designed to be:

- **Universal**: Works with any container image
- **Extensible**: Pluggable runtimes and custom implementations
- **Developer-Friendly**: Multi-language SDKs with consistent APIs
- **Production-Ready**: Robust lifecycle management and observability
- **Secure**: Isolated environments with access control

The protocol-first design ensures that all components can evolve independently while maintaining compatibility. Whether you're building AI coding assistants, interactive notebooks, or remote development environments, OpenSandbox provides the foundation you need.

## 9. References

- [Contributing Guide](contributing.md)
- [Sandbox Lifecycle Spec](../specs/sandbox-lifecycle.yml)
- [Sandbox Execution Spec](../specs/execd-api.yaml)
- [Server Documentation](../server/README.md)
- [execd Documentation](../components/execd/README.md)
- [Python SDK](../sdks/sandbox/python/README.md)
- [Java/Kotlin SDK](../sdks/sandbox/kotlin/README.md)
- [Examples](../examples/README.md)
