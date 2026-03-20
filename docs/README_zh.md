<div align="center">
  <img src="assets/logo.svg" alt="OpenSandbox logo" width="150" />

  <h1>OpenSandbox</h1>

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
</p>

  <hr />
</div>

中文 | [English](../README.md)

OpenSandbox 是一个面向 AI 应用场景设计的「通用沙箱平台」，为LLM相关的能力（命令执行、文件操作、代码执行、浏览器操作、Agent 运行等）提供 **多语言 SDK、沙箱接口协议和沙箱运行时**。

OpenSandbox 已进入 [CNCF Landscape](https://landscape.cncf.io/?item=orchestration-management--scheduling-orchestration--opensandbox)。

## 核心特性

- **多语言 SDK**：提供 Python、Java/Kotlin、JavaScript/TypeScript、C#/.NET 等语言的客户端 SDK，Go SDK 仍在规划中。
- **沙箱协议**：定义了沙箱生命周期管理 API 和沙箱执行 API。你可以通过这些沙箱协议扩展自己的沙箱运行时。
- **沙箱运行时**：沙箱全生命周期管理，支持 Docker 和[自研高性能 Kubernetes 运行时](../kubernetes)，实现本地运行、企业级大规模分布式沙箱调度。
- **沙箱环境**：内置 Command、Filesystem、Code Interpreter 实现。并提供 Coding Agent（Claude Code 等）、浏览器自动化（Chrome、Playwright）和桌面环境（VNC、VS Code）等示例。
- **网络策略**：提供统一的 [Ingress Gateway](../components/ingress) 实现，并支持多种路由策略；提供单实例级别的沙箱[出口网络限制](../components/egress)。
- **强隔离安全**：支持 gVisor、Kata Containers 和 Firecracker 微虚拟机等安全容器运行时，为沙箱工作负载与宿主机之间提供增强的安全隔离。详见 [安全容器运行时指南](secure-container.md)。

## 使用示例

### 沙箱基础操作

环境要求：

- Docker（本地运行必需）
- Python 3.10+（本地 runtime 和快速开始）

#### 1. 安装并配置 Server

```bash
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker-zh
```

> 如果需要开发或使用源码编译，可通过clone仓库进行开发。
> 
> ```bash
> git clone https://github.com/alibaba/OpenSandbox.git
> cd OpenSandbox/server
> uv sync
> cp example.config.toml ~/.sandbox.toml # Copy configuration file
> uv run python -m src.main # Start the service
> ```

#### 2. 启动沙箱 Server

```bash
opensandbox-server

# Show help
opensandbox-server -h
```

#### 3. 创建代码解释器，并在沙箱中执行命令

安装 Code Interpreter SDK

```bash
uv pip install opensandbox-code-interpreter
```

创建沙箱并执行命令

```python
import asyncio
from datetime import timedelta

from code_interpreter import CodeInterpreter, SupportedLanguage
from opensandbox import Sandbox
from opensandbox.models import WriteEntry

async def main() -> None:
    # 1. Create a sandbox
    sandbox = await Sandbox.create(
        "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.2",
        entrypoint= ["/opt/opensandbox/code-interpreter.sh"],
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

        # 6. 执行 Python 代码（单次执行：直接传 language）
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

### 更多示例

OpenSandbox 提供了丰富的示例来演示不同场景下的沙箱使用方式。所有示例代码位于 `examples/` 目录下。

#### 🎯 基础示例

- **[code-interpreter](../examples/code-interpreter/README.md)** - Code Interpreter SDK 的端到端沙箱流程示例。
- **[aio-sandbox](../examples/aio-sandbox/README.md)** - 使用 OpenSandbox SDK 与 agent-sandbox 的一体化沙箱示例。
- **[agent-sandbox](../examples/agent-sandbox/README.md)** - 通过 kubernetes-sigs/agent-sandbox 在 Kubernetes 上运行 OpenSandbox。

#### 🤖 Coding Agent 集成

在 OpenSandbox 中，集成各类 Coding Agent，包括 Claude Code、Google Gemini、OpenAI Codex、Kimi CLI 等。

- **[claude-code](../examples/claude-code/README.md)** - 在 OpenSandbox 中运行 Claude Code。
- **[gemini-cli](../examples/gemini-cli/README.md)** - 在 OpenSandbox 中运行 Google Gemini CLI。
- **[codex-cli](../examples/codex-cli/README.md)** - 在 OpenSandbox 中运行 OpenAI Codex CLI。
- **[kimi-cli](../examples/kimi-cli/README.md)** - 在 OpenSandbox 中运行 [Kimi CLI](https://github.com/MoonshotAI/kimi-cli)（Moonshot AI）。
- **[iflow-cli](../examples/iflow-cli/README.md)** - 在 OpenSandbox 中运行 iFlow CLI。
- **[langgraph](../examples/langgraph/README.md)** - 基于 LangGraph 状态机编排沙箱任务与回退重试。
- **[google-adk](../examples/google-adk/README.md)** - 使用 Google ADK 通过 OpenSandbox 工具读写文件并执行命令。
- **[nullclaw](../examples/nullclaw/README.md)** - 在沙箱中启动 Nullclaw Gateway。
- **[openclaw](../examples/openclaw/README_zh.md)** - 在沙箱中启动 OpenClaw Gateway。

#### 🌐 浏览器与桌面环境

- **[chrome](../examples/chrome/README.md)** - 带 VNC 与 DevTools 的无头 Chromium，用于自动化/调试。
- **[playwright](../examples/playwright/README.md)** - Playwright + Chromium 无头抓取与测试示例。
- **[desktop](../examples/desktop/README.md)** - 通过 VNC 访问的完整桌面环境沙箱。
- **[vscode](../examples/vscode/README.md)** - 在沙箱中运行 code-server（VS Code Web）进行远程开发。

#### 🧠 机器学习与训练

- **[rl-training](../examples/rl-training/README.md)** - 在沙箱中运行 DQN CartPole 训练，输出 checkpoint 与训练汇总。

更多详细信息请参考 [examples](../examples/README.md) 和各示例目录下的 README 文件。

## 项目结构

| 目录 | 说明                                                |
|------|---------------------------------------------------|
| [`sdks/`](../sdks/) | 多语言 SDK（Python、Java/Kotlin、TypeScript/JavaScript、C#/.NET）      |
| [`specs/`](../specs/) | OpenAPI 与生命周期规范                                   |
| [`server/`](../server/README_zh.md) | Python FastAPI 沙箱生命周期服务，并集成多种运行时实现                |
| [`kubernetes/`](../kubernetes/README-ZH.md) | Kubernetes 部署与示例                                  |
| [`components/execd/`](../components/execd/README_zh.md) | 沙箱执行守护进程，负责命令和文件操作                                |
| [`components/ingress/`](../components/ingress/README.md) | 沙箱流量入口代理                                          |
| [`components/egress/`](../components/egress/README.md) | 沙箱网络 Egress 访问控制                                  |
| [`sandboxes/`](../sandboxes/) | 沙箱运行时实现与镜像（如 code-interpreter）                    |
| [`examples/`](../examples/README.md) | 集成示例和使用案例                                         |
| [`oseps/`](../oseps/README.md) | OpenSandbox Enhancement Proposals                 |
| [`docs/`](../docs/) | 架构和设计文档                                           |
| [`tests/`](../tests/) | 跨组件端到端测试                                          |
| [`scripts/`](../scripts/) | 开发和维护脚本                                           |

详细架构请参阅 [docs/architecture.md](architecture.md)。

## 文档

- [docs/architecture.md](architecture.md) – 整体架构 & 设计理念
- [oseps/README.md](../oseps/README.md) – OpenSandbox 增强提案 (OSEPs)
- SDK
  - Sandbox 基础 SDK（[Java\Kotlin SDK](../sdks/sandbox/kotlin/README_zh.md)、[Python SDK](../sdks/sandbox/python/README_zh.md)、[JavaScript/TypeScript SDK](../sdks/sandbox/javascript/README_zh.md)、[C#/.NET SDK](../sdks/sandbox/csharp/README_zh.md)）- 包含沙箱生命周期、命令执行、文件操作
  - Code Interpreter SDK（[Java\Kotlin SDK](../sdks/code-interpreter/kotlin/README_zh.md) 、[Python SDK](../sdks/code-interpreter/python/README_zh.md)、[JavaScript/TypeScript SDK](../sdks/code-interpreter/javascript/README_zh.md)、[C#/.NET SDK](../sdks/code-interpreter/csharp/README_zh.md)）- 代码解释器
- [specs/README.md](../specs/README_zh.md) - 包含沙箱生命周期 API 和沙箱执行 API 的 OpenAPI 定义
- [server/README.md](../server/README_zh.md) - 包含沙箱 Server 的启动和配置，支持 Docker 与 Kubernetes Runtime

## 许可证

本项目采用 [Apache 2.0 License](../LICENSE) 开源。

你可以在遵守许可条款的前提下，将 OpenSandbox 用于个人或商业项目。

## 路线图 [2026.03]

### SDK

- **沙箱客户端连接池** - 客户端沙箱连接池管理，提供预配置的沙箱实例，以毫秒级速度获取沙箱环境。
- **Go SDK** - Go 客户端 SDK，用于沙箱生命周期管理、命令执行和文件操作。

### Sandbox Runtime

- **持久化存储** - 沙箱的持久化存储挂载（参见 [Proposal 0003](../oseps/0003-volume-and-volumebinding-support.md)）。
- **本地轻量级沙箱** - 为运行在 PC 上的 AI 工具提供轻量级沙箱。
- **安全容器** - 为在容器内运行的 AI Agent 提供安全沙箱。

### Deployment

- **部署指南** - 自托管 Kubernetes 集群的部署指南。

## 联系与讨论

- Issue：通过 GitHub Issues 提交 bug、功能请求或设计讨论
- 钉钉群：加入 [OpenSandbox 技术交流群](https://qr.dingtalk.com/action/joingroup?code=v1,k1,A4Bgl5q1I1eNU/r33D18YFNrMY108aFF38V+r19RJOM=&_dt_no_comment=1&origin=11)

欢迎一起把 OpenSandbox 打造成 AI 场景下的通用沙箱基础设施。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=alibaba/OpenSandbox&type=date&legend=top-left)](https://www.star-history.com/#alibaba/OpenSandbox&type=date&legend=top-left)
