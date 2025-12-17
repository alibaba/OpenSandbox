# OpenSandbox API Specifications

English | [中文](README_zh.md)

This directory contains OpenAPI specification documents for the OpenSandbox project, defining the complete API interfaces and data models.

## Specification Files

### 1. sandbox-lifecycle.yml

**Sandbox Lifecycle Management API**

Defines the complete lifecycle interfaces for creating, managing, and destroying sandbox environments.

**Core Features:**
- **Sandbox Management**: Create, list, query, and delete sandbox instances
- **State Control**: Pause and resume sandbox execution
- **Lifecycle States**: Support multiple state transitions (Pending → Running → Pausing → Paused → Stopping → Terminated)
- **Resource Configuration**: CPU, memory, GPU, and other resource limit configurations
- **Image Support**: Create sandboxes directly from container images, supporting both public and private registries
- **Timeout Management**: Automatic expiration and manual renewal capabilities
- **Endpoint Access**: Retrieve public access endpoints for services running inside sandboxes

**Main Endpoints:**
- `POST /v1/sandboxes` - Create sandbox
- `GET /v1/sandboxes` - List sandboxes (with filtering and pagination)
- `GET /v1/sandboxes/{sandboxId}` - Get sandbox details
- `DELETE /v1/sandboxes/{sandboxId}` - Delete sandbox
- `POST /v1/sandboxes/{sandboxId}/pause` - Pause sandbox
- `POST /v1/sandboxes/{sandboxId}/resume` - Resume sandbox
- `POST /v1/sandboxes/{sandboxId}/renew-expiration` - Renew sandbox expiration
- `GET /v1/sandboxes/{sandboxId}/endpoints/{port}` - Get access endpoint

**Authentication:**
- HTTP Header: `OPEN-SANDBOX-API-KEY: your-api-key`
- Environment Variable: `OPEN_SANDBOX_API_KEY` (for SDK clients)

### 2. execd-api.yaml

**Code Execution API Inside Sandbox**

Defines interfaces for executing code, commands, and file operations within sandbox environments, providing complete code interpreter and filesystem management capabilities.

**Core Features:**
- **Code Execution**: Stateful code execution supporting Python, JavaScript, and other languages
- **Command Execution**: Shell command execution with foreground/background modes
- **File Operations**: Complete CRUD operations for files and directories
- **Real-time Streaming**: Real-time output streaming via SSE (Server-Sent Events)
- **System Monitoring**: Real-time monitoring of CPU and memory metrics
- **Access Control**: Token-based API authentication

**Main Endpoint Categories:**

**Health Check:**
- `GET /ping` - Service health check

**Code Interpreter:**
- `POST /code/context` - Create code execution context
- `POST /code` - Execute code in context (streaming output)
- `DELETE /code` - Interrupt code execution

**Command Execution:**
- `POST /command` - Execute shell command (streaming output)
- `DELETE /command` - Interrupt command execution

**Filesystem:**
- `GET /files/info` - Get file metadata
- `DELETE /files` - Delete files
- `POST /files/permissions` - Change file permissions
- `POST /files/mv` - Move/rename files
- `GET /files/search` - Search files (supports glob patterns)
- `POST /files/replace` - Batch replace file content
- `POST /files/upload` - Upload files
- `GET /files/download` - Download files (supports range requests)

**Directory Operations:**
- `POST /directories` - Create directories
- `DELETE /directories` - Delete directories

**System Metrics:**
- `GET /metrics` - Get system resource metrics
- `GET /metrics/watch` - Watch system metrics in real-time (SSE stream)

## Technical Features

### Streaming Output (Server-Sent Events)

Code execution and command execution interfaces use SSE for real-time streaming output, supporting the following event types:
- `init` - Initialization event
- `status` - Status update
- `stdout` / `stderr` - Standard output/error streams
- `result` - Execution result
- `execution_complete` - Execution completed
- `execution_count` - Execution count
- `error` - Error information

### Resource Limits

Supports flexible resource configuration (similar to Kubernetes):
```json
{
  "cpu": "500m",
  "memory": "512Mi",
  "gpu": "1"
}
```

### File Permissions

Supports Unix-style file permission management:
- Owner
- Group
- Permission mode (octal format, e.g., 755)
