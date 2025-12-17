# OpenSandbox

English | [中文](docs/README_zh.md)

OpenSandbox is a **universal sandbox platform** for AI application scenarios, providing **multi-language SDKs, unified sandbox protocols, and sandbox runtimes** for LLM-related capabilities (command execution, file operations, code execution, browser operations, Agent execution, etc.).

## Core Features

- **Multi-language SDKs**: Provides client SDKs in Python, Java, TypeScript (TODO), and other languages (`sdks/`).
- **Sandbox Protocol**: All sandboxes follow the same OAS interface specification (`specs/`). You can extend your own sandbox runtime through this unified protocol.
- **Sandbox Runtime**: Supports local execution (`server/`) and Kubernetes cluster execution (TODO).
- **Multiple Sandbox Implementations**: Built-in implementations for Command, Filesystem (`components/execd/`), Code Interpreter (`sandboxes/code-interpreter/`), Browser Use, Coding Agent (Claude Code), and more, with examples (`examples/`).

## Usage Examples

### Basic Sandbox Operations

Requirements:

- Docker (required for local execution)
- Python 3.10+ (recommended for examples and local runtime)

#### 1. Clone the Repository

```bash
git clone https://github.com/alibaba/OpenSandbox.git
cd OpenSandbox
```

#### 2. Start the Sandbox Server

```bash
cd server
uv sync
cp example.config.toml ~/.sandbox.toml # Copy configuration file
uv run python -m src.main # Start the service
```

#### 3. Create a Code Interpreter and Execute Commands

Install the Code Interpreter SDK

```bash
uv pip install opensandbox-code-interpreter
```

Create a sandbox and execute commands

```python
import asyncio
from datetime import timedelta

from code_interpreter import CodeInterpreter, SupportedLanguage, CodeContext
from opensandbox import Sandbox
from opensandbox.models import WriteEntry

async def main() -> None:
    # 1. Create a sandbox
    sandbox = await Sandbox.create(
        "opensandbox/code-interpreter:latest",
        entrypoint=["/opt/opensandbox/code-interpreter.sh"],
        env={"PYTHON_VERSION": "3.11"},
        timeout=timedelta(minutes=10),
    )

    async with sandbox:

        # 2. Execute a shell command
        execution = await sandbox.commands.run("echo 'Hello OpenSandbox!'")
        print(execution.logs.stdout[0].text)

        # 3. Write a file
        await sandbox.files.write_files([
            WriteEntry(path="/tmp/hello.txt", data="Hello World", mode=644)
        ])

        # 4. Read a file
        content = await sandbox.files.read_file("/tmp/hello.txt")
        print(f"Content: {content}") # Content: Hello World

        # 5. Create a code interpreter
        interpreter = await CodeInterpreter.create(sandbox)

        # 6. Execute Python code
        result = await interpreter.codes.run(
              """
                  import sys
                  print(sys.version)
                  result = 2 + 2
                  result
              """,
              context=CodeContext(language=SupportedLanguage.PYTHON)
        )

        print(result.result[0].text) # 4
        print(result.logs.stdout[0].text) # 3.11.14

    # 7. Cleanup the sandbox
    await sandbox.kill()

if __name__ == "__main__":
    asyncio.run(main())
```

### More Examples

OpenSandbox provides rich examples demonstrating sandbox usage in different scenarios. All example code is located in the `examples/` directory.

#### 🎯 Basic Examples

- **[code-interpreter](examples/code-interpreter/README.md)** - Complete Code Interpreter SDK example
  - Run commands and execute Python/Java/Go/TypeScript code inside a sandbox
  - Covers context creation, code execution, and result streaming
  - Supports custom language versions

- **[aio-sandbox](examples/aio-sandbox/README.md)** - All-in-One sandbox example
  - Uses OpenSandbox SDK to create an [agent-sandbox](https://github.com/agent-infra/sandbox) instance
  - Shows how to connect and use the full AIO sandbox capabilities

#### 🤖 Coding Agent Integrations

OpenSandbox integrates various Coding Agents, including Claude Code, Google Gemini, OpenAI Codex, and more.

- **[claude-code](examples/claude-code/README.md)** - Claude Code integration
- **[gemini-cli](examples/gemini-cli/README.md)** - Google Gemini CLI integration
- **[codex-cli](examples/codex-cli/README.md)** - OpenAI Codex CLI integration
- **[iflow-cli](examples/iflow-cli/README.md)** - iFLow CLI integration

#### 🌐 Browser and Desktop Environments

- **[chrome](examples/chrome/README.md)** - Chrome headless browser

  - Launches Chromium browser with remote debugging functionality
  - Provides VNC (port 5901) and DevTools (port 9222) access
  - Suitable for scenarios requiring browser automation or debugging

- **[playwright](examples/playwright/README.md)** - Playwright browser automation

  - Uses Playwright + Chromium in headless mode to scrape web content
  - Can extract web page titles, body text, and other information
  - Suitable for web crawling and automated testing

- **[desktop](examples/desktop/README.md)** - VNC desktop environment

  - Launches a complete desktop environment (Xvfb + x11vnc + fluxbox)
  - Remote access to sandbox desktop via VNC client
  - Supports custom VNC password

- **[vscode](examples/vscode/README.md)** - VS Code Web environment
  - Runs code-server (VS Code web version) in a sandbox
  - Access complete VS Code development environment through browser
  - Suitable for remote development and code editing scenarios

For more details, please refer to [examples](examples/README.md) and the README files in each example directory.

## Directory Structure

```bash
OpenSandbox/
├── sdks/                     # Multi-language SDKs
│   ├── code-interpreter/     # Code Interpreter SDK
│   └── sandbox/              # Sandbox base SDK
├── specs/                    # OpenAPI specifications
│   ├── execd-api.yaml        # Command execution and file operations API specification
│   └── sandbox-lifecycle.yml # Sandbox lifecycle API specification
├── server/                   # Sandbox server
├── components/               # Core components
│   └── execd/                # Command execution and file operations component (Go)
├── docs/                     # Documentation
├── examples/                 # Example integrations and use cases
├── sandboxes/                # Sandbox implementations
│   └── code-interpreter/     # Code Interpreter sandbox implementation
├── scripts/                  # Build and utility scripts
└── tests                     # End-to-end tests
```

## Documentation

- [docs/architecture.md](docs/architecture.md) – Overall architecture & design philosophy
- SDK
  - Sandbox base SDK ([Java\Kotlin SDK](sdks/sandbox/kotlin/README.md), [Python SDK](sdks/sandbox/python/README.md)) - includes sandbox lifecycle, command execution, file operations
  - Code Interpreter SDK ([Java\Kotlin SDK](sdks/code-interpreter/kotlin/README.md), [Python SDK](sdks/code-interpreter/python/README.md)) - code interpreter
- [specs/README.md](specs/README.md) - Contains OpenAPI definitions for sandbox lifecycle API and sandbox execution API
- [server/README.md](server/README.md) - Contains sandbox server startup and configuration, currently supports Docker Runtime, will support Kubernetes Runtime in the future

---

## License

This project is open source under the [Apache 2.0 License](LICENSE).

You can use OpenSandbox for personal or commercial projects in compliance with the license terms.

---

## Contact and Discussion

- Issues: Submit bugs, feature requests, or design discussions through GitHub Issues

We welcome everyone to help build OpenSandbox into a universal sandbox infrastructure for AI scenarios.
