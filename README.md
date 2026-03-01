<div align="center">
  <img src="docs/assets/logo.svg" alt="OpenSandbox logo" width="150" />

  <h1>OpenSandbox</h1>

<p align="center">
  <a href="https://github.com/alibaba/OpenSandbox">
    <img src="https://img.shields.io/github/stars/alibaba/OpenSandbox.svg?style=social" alt="GitHub stars" />
  </a>
  <a href="https://deepwiki.com/alibaba/OpenSandbox">
    <img src="https://deepwiki.com/badge.svg" alt="Ask DeepWiki" />
  </a>
  <a href="https://www.apache.org/licenses/LICENSE-2.0.html">
    <img src="https://img.shields.io/github/license/alibaba/OpenSandbox.svg" alt="license" />
  </a>
  <a href="https://badge.fury.io/py/opensandbox">
    <img src="https://badge.fury.io/py/opensandbox.svg" alt="PyPI version" />
  </a>
  <a href="https://badge.fury.io/js/@alibaba-group%2Fopensandbox">
    <img src="https://badge.fury.io/js/@alibaba-group%2Fopensandbox.svg" alt="npm version" />
  </a>
  <a href="https://github.com/alibaba/OpenSandbox/actions">
    <img src="https://github.com/alibaba/OpenSandbox/actions/workflows/real-e2e.yml/badge.svg?branch=main" alt="E2E Status" />
  </a>
</p>

  <hr />
</div>

[Document](https://open-sandbox.ai/) | [文档](https://open-sandbox.ai/zh/)

OpenSandbox is a **general-purpose sandbox platform** for AI applications, offering multi-language SDKs, unified sandbox APIs, and Docker/Kubernetes runtimes for scenarios like Coding Agents, GUI Agents, Agent Evaluation, AI Code Execution, and RL Training.

## Features

- **Multi-language SDKs**: Provides sandbox SDKs in Python, Java/Kotlin, JavaScript/TypeScript, C#/.NET, Go (Roadmap), and more.
- **Sandbox Protocol**: Defines sandbox lifecycle management APIs and sandbox execution APIs so you can extend custom sandbox runtimes.
- **Sandbox Runtime**: Built-in lifecycle management supporting Docker and [high-performance Kubernetes runtime](./kubernetes), enabling both local runs and large-scale distributed scheduling.
- **Sandbox Environments**: Built-in Command, Filesystem, and Code Interpreter implementations. Examples cover Coding Agents (e.g., Claude Code), browser automation (Chrome, Playwright), and desktop environments (VNC, VS Code).
- **Network Policy**: Unified [Ingress Gateway](components/ingress) with multiple routing strategies plus per-sandbox [egress controls](components/egress).

## Examples

### Basic Sandbox Operations

Requirements:

- Docker (required for local execution)
- Python 3.10+ (recommended for examples and local runtime)

#### 1. Install and Configure the Sandbox Server

```bash
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker
```

> If you prefer working from source, you can still clone the repo for development, but server startup no longer requires it.
>
> ```bash
> git clone https://github.com/alibaba/OpenSandbox.git
> cd OpenSandbox/server
> uv sync
> cp example.config.toml ~/.sandbox.toml # Copy configuration file
> uv run python -m src.main # Start the service
> ```

#### 2. Start the Sandbox Server

```bash
opensandbox-server

# Show help
opensandbox-server -h
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

- **[code-interpreter](examples/code-interpreter/README.md)** - End-to-end Code Interpreter SDK workflow in a sandbox.
- **[aio-sandbox](examples/aio-sandbox/README.md)** - All-in-One sandbox setup using the OpenSandbox SDK.
- **[agent-sandbox](examples/agent-sandbox/README.md)** - Run OpenSandbox on Kubernetes via [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox).

#### 🤖 Coding Agent Integrations

- **[claude-code](examples/claude-code/README.md)** - Run Claude Code inside OpenSandbox.
- **[gemini-cli](examples/gemini-cli/README.md)** - Run Google Gemini CLI inside OpenSandbox.
- **[codex-cli](examples/codex-cli/README.md)** - Run OpenAI Codex CLI inside OpenSandbox.
- **[iflow-cli](examples/iflow-cli/README.md)** - Run iFLow CLI inside OpenSandbox.
- **[langgraph](examples/langgraph/README.md)** - LangGraph state-machine workflow that creates/runs a sandbox job with fallback retry.
- **[google-adk](examples/google-adk/README.md)** - Google ADK agent using OpenSandbox tools to write/read files and run commands.
- **[openclaw](examples/openclaw/README.md)** - Launch an OpenClaw Gateway inside a sandbox.

#### 🌐 Browser and Desktop Environments

- **[chrome](examples/chrome/README.md)** - Headless Chromium with VNC and DevTools access for automation/debugging.
- **[playwright](examples/playwright/README.md)** - Playwright + Chromium headless scraping and testing example.
- **[desktop](examples/desktop/README.md)** - Full desktop environment in a sandbox with VNC access.
- **[vscode](examples/vscode/README.md)** - code-server (VS Code Web) running inside a sandbox for remote dev.

#### 🧠 ML and Training

- **[rl-training](examples/rl-training/README.md)** - DQN CartPole training in a sandbox with checkpoints and summary output.

For more details, please refer to [examples](examples/README.md) and the README files in each example directory.

## Project Structure

| Directory | Description                                                      |
|-----------|------------------------------------------------------------------|
| [`sdks/`](sdks/) | Multi-language SDKs (Python, Java/Kotlin, TypeScript/JavaScript, C#/.NET) |
| [`specs/`](specs/README.md) | OpenAPI specs and lifecycle specifications                      |
| [`server/`](server/README.md) | Python FastAPI sandbox lifecycle server                          |
| [`kubernetes/`](kubernetes/README.md) | Kubernetes deployment and examples                               |
| [`components/execd/`](components/execd/README.md) | Sandbox execution daemon (commands and file operations)          |
| [`components/ingress/`](components/ingress/README.md) | Sandbox traffic ingress proxy                                    |
| [`components/egress/`](components/egress/README.md) | Sandbox network egress control                                   |
| [`sandboxes/`](sandboxes/) | Runtime sandbox implementations                                   |
| [`examples/`](examples/README.md) | Integration examples and use cases                               |
| [`oseps/`](oseps/README.md) | OpenSandbox Enhancement Proposals                                |
| [`docs/`](docs/) | Architecture and design documentation                            |
| [`tests/`](tests/) | Cross-component E2E tests                                        |
| [`scripts/`](scripts/) | Development and maintenance scripts                              |

For detailed architecture, see [docs/architecture.md](docs/architecture.md).

## Documentation

- [docs/architecture.md](docs/architecture.md) – Overall architecture & design philosophy
- SDK
  - Sandbox base SDK ([Java\Kotlin SDK](sdks/sandbox/kotlin/README.md), [Python SDK](sdks/sandbox/python/README.md), [JavaScript/TypeScript SDK](sdks/sandbox/javascript/README.md), [C#/.NET SDK](sdks/sandbox/csharp/README.md)) - includes sandbox lifecycle, command execution, file operations
  - Code Interpreter SDK ([Java\Kotlin SDK](sdks/code-interpreter/kotlin/README.md), [Python SDK](sdks/code-interpreter/python/README.md), [JavaScript/TypeScript SDK](sdks/code-interpreter/javascript/README.md), [C#/.NET SDK](sdks/code-interpreter/csharp/README.md)) - code interpreter
- [specs/README.md](specs/README.md) - OpenAPI definitions for sandbox lifecycle API and sandbox execution API
- [server/README.md](server/README.md) - Sandbox server startup and configuration; supports Docker and Kubernetes runtimes

## License

This project is open source under the [Apache 2.0 License](LICENSE).

## Roadmap

### SDK

- [ ] **Go SDK** - Go client SDK for sandbox lifecycle management, command execution, and file operations.

### Sandbox Runtime

- [ ] **Persistent storage** - Mountable persistent storage for sandboxes (see [Proposal 0003](oseps/0003-volume-and-volumebinding-support.md)).
- [ ] **Ingress multi-network strategies** - Multi-Kubernetes provisioning and multi-network modes for the Ingress Gateway.
- [ ] **Local lightweight sandbox** - Lightweight sandbox for AI tools running directly on PCs.

### Deployment

- [ ] **Kubernetes Helm** - Helm charts to deploy all components.

## Contact and Discussion

- Issues: Submit bugs, feature requests, or design discussions through GitHub Issues
## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=alibaba/OpenSandbox&type=date&legend=top-left)](https://www.star-history.com/#alibaba/OpenSandbox&type=date&legend=top-left)

