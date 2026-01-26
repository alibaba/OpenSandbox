<div align="center">
  <img src="docs/assets/logo.svg" alt="OpenSandbox logo" width="150" />

  <h1>OpenSandbox</h1>

[![GitHub stars](https://img.shields.io/github/stars/alibaba/OpenSandbox.svg?style=social)](https://github.com/alibaba/OpenSandbox)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/alibaba/OpenSandbox)
[![license](https://img.shields.io/github/license/alibaba/OpenSandbox.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![PyPI version](https://badge.fury.io/py/opensandbox.svg)](https://badge.fury.io/py/opensandbox)
[![npm version](https://badge.fury.io/js/@alibaba-group%2Fopensandbox.svg)](https://badge.fury.io/js/@alibaba-group%2Fopensandbox)
[![E2E Status](https://github.com/alibaba/OpenSandbox/actions/workflows/real-e2e.yml/badge.svg?branch=main)](https://github.com/alibaba/OpenSandbox/actions)

  <hr />
</div>

English | [中文](docs/README_zh.md)

OpenSandbox is a **universal sandbox platform** for AI application scenarios, providing **multi-language SDKs, unified sandbox protocols, and sandbox runtimes** for LLM-related capabilities (command execution, file operations, code execution, browser operations, Agent execution, etc.).

## Features

- **Multi-language SDKs**: Provides sandbox SDKs in Python, Java/Kotlin, JavaScript/TypeScript, Go (Roadmap), and more.
- **Sandbox Protocol**: Defines sandbox lifecycle management API and sandbox execution API. You can extend your own sandbox runtime through these sandbox protocols.
- **Sandbox Runtime**: Implements sandbox lifecycle management by default, supports Docker and Kubernetes runtimes, enabling large-scale distributed sandbox scheduling.
- **Sandbox Environments**: Built-in implementations for Command, Filesystem, Code Interpreter. And provides examples for Coding Agents (Claude Code, etc.), Browser automation (Chrome, Playwright), and Desktop environments (VNC, VS Code).

## Examples

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

from code_interpreter import CodeInterpreter, SupportedLanguage
from opensandbox import Sandbox
from opensandbox.models import WriteEntry

async def main() -> None:
    # 1. Create a sandbox
    sandbox = await Sandbox.create(
        "opensandbox/code-interpreter:v1.0.1",
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

        # 6. Execute Python code (single-run, pass language directly)
        result = await interpreter.codes.run(
              """
                  import sys
                  print(sys.version)
                  result = 2 + 2
                  result
              """,
              language=SupportedLanguage.PYTHON,
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
- **[langgraph](examples/langgraph/README.md)** - LangGraph integration
- **[google-adk](examples/google-adk/README.md)** - Google ADK integration

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

## Project Structure

| Directory | Description                                                      |
|-----------|------------------------------------------------------------------|
| [`server/`](server/README.md) | Python FastAPI sandbox lifecycle server                          |
| [`components/execd/`](components/execd/README.md) | Sandbox execution daemon (commands and file operations)          |
| [`components/ingress/`](components/ingress/README.md) | Sandbox traffic ingress proxy                                    |
| [`components/egress/`](components/egress/README.md) | Sandbox network egress control                                   |
| [`sdks/`](sdks/) | Multi-language SDKs (Python, Java/Kotlin, Typescript/Javascript) |
| [`sandboxes/`](sandboxes/) | Sandbox runtime images (e.g., code-interpreter)                  |
| [`kubernetes/`](kubernetes/README.md) | Kubernetes operator and batch sandbox support                    |
| [`specs/`](specs/README.md) | OpenAPI specifications                                           |
| [`examples/`](examples/README.md) | Integration examples and use cases                               |
| [`oseps/`](oseps/README.md) | OpenSandbox Enhancement Proposals                                |
| [`docs/`](docs/) | Architecture and design documentation                            |
| [`tests/`](tests/) | Cross-component E2E tests                                        |
| [`scripts/`](scripts/) | Development and maintenance scripts                              |

For detailed architecture, see [docs/architecture.md](docs/architecture.md).

## Documentation

- [docs/architecture.md](docs/architecture.md) – Overall architecture & design philosophy
- SDK
  - Sandbox base SDK ([Java\Kotlin SDK](sdks/sandbox/kotlin/README.md), [Python SDK](sdks/sandbox/python/README.md), [JavaScript/TypeScript SDK](sdks/sandbox/javascript/README.md)) - includes sandbox lifecycle, command execution, file operations
  - Code Interpreter SDK ([Java\Kotlin SDK](sdks/code-interpreter/kotlin/README.md), [Python SDK](sdks/code-interpreter/python/README.md), [JavaScript/TypeScript SDK](sdks/code-interpreter/javascript/README.md)) - code interpreter
- [specs/README.md](specs/README.md) - Contains OpenAPI definitions for sandbox lifecycle API and sandbox execution API
- [server/README.md](server/README.md) - Contains sandbox server startup and configuration, currently supports Docker Runtime, will support Kubernetes Runtime in the future

## License

This project is open source under the [Apache 2.0 License](LICENSE).

You can use OpenSandbox for personal or commercial projects in compliance with the license terms.

## Roadmap

### SDK

- [ ] **Go SDK** - Go client SDK for sandbox lifecycle management and command execution, file operations.

### Server Runtime

- [x] **OpenSandbox Kubernetes Runtime** - High-performance sandbox scheduling implementation (see [`kubernetes/`](kubernetes/README.md))
- [ ] **kubernetes-sigs/agent-sandbox Support** - Integration with [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox)
- [ ] **Declarative Network Isolation** - Network egress control with allow/deny rules for specific domains (see [OSEP-0001](oseps/0001-fqdn-based-egress-control.md))
  - [x] DNS-based egress control (Layer 1)
  - [ ] Network-based egress control (Layer 2)

## Contact and Discussion

- Issues: Submit bugs, feature requests, or design discussions through GitHub Issues

We welcome everyone to help build OpenSandbox into a universal sandbox infrastructure for AI scenarios.
