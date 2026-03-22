<div align="center">
  <img src="docs/assets/logo.svg" alt="OpenSandbox logo" width="150" />

  <h1>OpenSandbox</h1>

  <p align="center">
    <a href="https://trendshift.io/repositories/21828" target="_blank">
      <img src="https://trendshift.io/api/badge/repositories/21828" alt="alibaba%2FOpenSandbox | Trendshift" style="width: 320px; height: 70px;" width="320" height="70" />
    </a>
  </p>

<p align="center">
  <a href="https://github.com/alibaba/OpenSandbox">
    <img src="https://img.shields.io/github/stars/alibaba/OpenSandbox.svg?style=social" alt="GitHub stars" />
  </a>
  <a href="https://deepwiki.com/alibaba/OpenSandbox">
    <img src="https://deepwiki.com/badge.svg" alt="Ask DeepWiki" />
  </a>
  <a href="https://www.apache.org/licenses/LICENSE-2.0.html">
    <img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="license" />
  </a>
  <a href="https://badge.fury.io/py/opensandbox">
    <img src="https://badge.fury.io/py/opensandbox.svg" alt="PyPI version" />
  </a>
  <a href="https://badge.fury.io/js/@alibaba-group%2Fopensandbox">
    <img src="https://badge.fury.io/js/@alibaba-group%2Fopensandbox.svg" alt="npm version" />
  </a>
  <a href="https://landscape.cncf.io/?item=orchestration-management--scheduling-orchestration--opensandbox">
    <img src="https://img.shields.io/badge/CNCF-Landscape-0C66E4" alt="CNCF Landscape" />
  </a>
  <a href="https://qr.dingtalk.com/action/joingroup?code=v1,k1,A4Bgl5q1I1eNU/r33D18YFNrMY108aFF38V+r19RJOM=&_dt_no_comment=1&origin=11">
    <img src="https://img.shields.io/badge/DingTalk-Join-0089FF?logo=dingtalk&logoColor=white" alt="DingTalk" />
  </a>
  <a href="https://github.com/alibaba/OpenSandbox/actions">
    <img src="https://github.com/alibaba/OpenSandbox/actions/workflows/real-e2e.yml/badge.svg?branch=main" alt="E2E Status" />
  </a>
  <a href="https://github.com/alibaba/OpenSandbox/actions">
    <img src="https://github.com/alibaba/OpenSandbox/actions/workflows/real-k8s-e2e.yml/badge.svg?branch=main" alt="E2E Status" />
  </a>
</p>

  <hr />
</div>

[Documentation](https://open-sandbox.ai/) | [中文文档](https://open-sandbox.ai/zh/)

OpenSandbox is a **general-purpose sandbox platform** for AI applications, offering multi-language SDKs, unified sandbox APIs, and Docker/Kubernetes runtimes for scenarios like Coding Agents, GUI Agents, Agent Evaluation, AI Code Execution, and RL Training.

OpenSandbox is now listed in the [CNCF Landscape](https://landscape.cncf.io/?item=orchestration-management--scheduling-orchestration--opensandbox).

## Features

- **Multi-language SDKs**: Provides sandbox SDKs in Python, Java/Kotlin, JavaScript/TypeScript, C#/.NET, Go (Roadmap), and more.
- **Sandbox Protocol**: Defines sandbox lifecycle management APIs and sandbox execution APIs so you can extend custom sandbox runtimes.
- **Sandbox Runtime**: Built-in lifecycle management supporting Docker and [high-performance Kubernetes runtime](./kubernetes), enabling both local runs and large-scale distributed scheduling.
- **Sandbox Environments**: Built-in Command, Filesystem, and Code Interpreter implementations. Examples cover Coding Agents (e.g., Claude Code), browser automation (Chrome, Playwright), and desktop environments (VNC, VS Code).
- **Network Policy**: Unified [Ingress Gateway](components/ingress) with multiple routing strategies plus per-sandbox [egress controls](components/egress).
- **Strong Isolation**: Supports secure container runtimes like gVisor, Kata Containers, and Firecracker microVM for enhanced isolation between sandbox workloads and the host. See [Secure Container Runtime Guide](docs/secure-container.md) for details.

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

> If you prefer working from source, you can still clone the repo for development, but you no longer need to clone this repository just to start the server.
> You'll also require an instance of docker running.
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
        "opensandbox/code-interpreter:v1.0.2",
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

OpenSandbox provides examples covering SDK usage, agent integrations, browser automation, and training workloads. All example code is located in the `examples/` directory.

#### 🎯 Basic Examples

- **[code-interpreter](examples/code-interpreter/README.md)** - End-to-end Code Interpreter SDK workflow in a sandbox.
- **[aio-sandbox](examples/aio-sandbox/README.md)** - All-in-One sandbox setup using the OpenSandbox SDK.
- **[agent-sandbox](examples/agent-sandbox/README.md)** - Example integration for running OpenSandbox workloads on Kubernetes with [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox).

#### 🤖 Coding Agent Integrations

- **[claude-code](examples/claude-code/README.md)** - Run Claude Code inside OpenSandbox.
- **[gemini-cli](examples/gemini-cli/README.md)** - Run Google Gemini CLI inside OpenSandbox.
- **[codex-cli](examples/codex-cli/README.md)** - Run OpenAI Codex CLI inside OpenSandbox.
- **[qwen-code](examples/qwen-code/README.md)** - Run Qwen Code inside OpenSandbox.
- **[kimi-cli](examples/kimi-cli/README.md)** - Run [Kimi CLI](https://github.com/MoonshotAI/kimi-cli) (Moonshot AI) inside OpenSandbox.
- **[langgraph](examples/langgraph/README.md)** - LangGraph state-machine workflow that creates/runs a sandbox job with fallback retry.
- **[google-adk](examples/google-adk/README.md)** - Google ADK agent using OpenSandbox tools to write/read files and run commands.
- **[nullclaw](examples/nullclaw/README.md)** - Launch a [Nullclaw](https://github.com/nullclaw/nullclaw) Gateway inside a sandbox.
- **[openclaw](examples/openclaw/README.md)** - Launch an OpenClaw Gateway inside a sandbox.

#### 🌐 Browser and Desktop Environments

- **[chrome](examples/chrome/README.md)** - Chromium sandbox with VNC and DevTools access for automation and debugging.
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
- [oseps/README.md](oseps/README.md) – OpenSandbox Enhancement Proposals
- SDK
  - Sandbox base SDK ([Java/Kotlin SDK](sdks/sandbox/kotlin/README.md), [Python SDK](sdks/sandbox/python/README.md), [JavaScript/TypeScript SDK](sdks/sandbox/javascript/README.md), [C#/.NET SDK](sdks/sandbox/csharp/README.md)) - includes sandbox lifecycle, command execution, file operations
  - Code Interpreter SDK ([Java/Kotlin SDK](sdks/code-interpreter/kotlin/README.md), [Python SDK](sdks/code-interpreter/python/README.md), [JavaScript/TypeScript SDK](sdks/code-interpreter/javascript/README.md), [C#/.NET SDK](sdks/code-interpreter/csharp/README.md)) - code interpreter
- [specs/README.md](specs/README.md) - OpenAPI definitions for sandbox lifecycle API and sandbox execution API
- [server/README.md](server/README.md) - Sandbox server startup and configuration; supports Docker and Kubernetes runtimes

## License

This project is open source under the [Apache 2.0 License](LICENSE).

## Roadmap [2026.03]

### SDK

- **Sandbox client connection pool** - Client-side sandbox connection pool management, providing pre-provisioned sandboxes to obtain an environment at X ms.
- **Go SDK** - Go client SDK for sandbox lifecycle management, command execution, and file operations.

### Sandbox Runtime

- **Persistent volumes** - Mountable persistent volumes for sandboxes (see [Proposal 0003](oseps/0003-volume-and-volumebinding-support.md)).
- **Local lightweight sandbox** - Lightweight sandbox for AI tools running directly on PCs.
- **Secure Container** - Secure sandbox for AI Agents running inside container.

### Deployment

- **Guide** - Deployment guide for self-hosted Kubernetes cluster.

## Contact and Discussion

- Issues: Submit bugs, feature requests, or design discussions through GitHub Issues
- DingTalk: Join the [OpenSandbox technical discussion group](https://qr.dingtalk.com/action/joingroup?code=v1,k1,A4Bgl5q1I1eNU/r33D18YFNrMY108aFF38V+r19RJOM=&_dt_no_comment=1&origin=11)
## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=alibaba/OpenSandbox&type=date&legend=top-left)](https://www.star-history.com/#alibaba/OpenSandbox&type=date&legend=top-left)
