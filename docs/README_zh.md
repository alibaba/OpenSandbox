# OpenSandbox

中文 | [English](../README.md)

OpenSandbox 是一个面向 AI 应用场景设计的「通用沙箱平台」，为大模型相关的能力（命令执行、文件操作、代码执行、浏览器操作、Agent 运行等）提供 **多语言 SDK、沙箱接口协议和沙箱运行时**。

## 核心特性

- **多语言 SDK**：提供 Python / Java / TypeScript（TODO） 等语言的客户端 SDK（`sdks/`）。
- **沙箱协议**：所有沙箱都遵循同一套 OAS 接口规范（`specs/`）。你可以通过统一协议扩展自己的沙箱运行时。
- **沙箱运行时**：支持本地运行（`server/`）和 Kubernetes 集群运行（TODO）。
- **多种沙箱实现**：内置 Command、Filesystem（`components/execd/`）、Code Interpreter（`sandboxes/code-interpreter/`）、Browser Use、Coding Agent（Claude Code）等多种沙箱实现和例子（`examples/`）。

## 使用示例

### 沙箱基础操作

环境要求：

- Docker（本地运行必需）
- Python 3.10+（本地 runtime 和快速开始）

#### 1. 克隆仓库

```bash
git clone https://github.com/alibaba/OpenSandbox.git
cd OpenSandbox
```

#### 2. 启动沙箱 Server

```bash
cd server
uv sync
cp example.config.zh.toml ~/.sandbox.toml # 复制配置文件
uv run python -m src.main # 启动服务
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

from code_interpreter import CodeInterpreter, SupportedLanguage, CodeContext
from opensandbox import Sandbox
from opensandbox.models import WriteEntry

async def main() -> None:
    # 1. Create a sandbox
    sandbox = await Sandbox.create(
        "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:latest",
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

        # 6. Execute a Python code
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

### 更多示例

OpenSandbox 提供了丰富的示例来演示不同场景下的沙箱使用方式。所有示例代码位于 `examples/` 目录下。

#### 🎯 基础示例

- **[code-interpreter](../examples/code-interpreter/README.md)** - Code Interpreter SDK 完整示例

  - 展示如何使用 Code Interpreter SDK 在沙箱中运行命令，执行 Python/Java/Go/TS 代码
  - 包含上下文创建、代码执行、结果输出等完整流程
  - 支持自定义语言版本

- **[aio-sandbox](../examples/aio-sandbox/README.md)** - All-in-One 沙箱示例
  - 使用 OpenSandbox SDK 创建 [agent-sandbox](https://github.com/agent-infra/sandbox) 沙箱
  - 演示如何连接并使用 AIO 沙箱提供的完整功能

#### 🤖 Coding Agent 集成

在 OpenSandbox 中，集成各类 Coding Agent，包括 Claude Code、Google Gemini、OpenAI Codex 等。

- **[claude-code](../examples/claude-code/README.md)** - Claude Code 集成
- **[gemini-cli](../examples/gemini-cli/README.md)** - Google Gemini CLI 集成
- **[codex-cli](../examples/codex-cli/README.md)** - OpenAI Codex CLI 集成
- **[iflow-cli](../examples/iflow-cli/README.md)** - iFLow CLI 集成

#### 🌐 浏览器与桌面环境

- **[chrome](../examples/chrome/README.md)** - Chrome 无头浏览器

  - 启动带有远程调试功能的 Chromium 浏览器
  - 提供 VNC (端口 5901) 和 DevTools (端口 9222) 访问
  - 适合需要浏览器自动化或调试的场景

- **[playwright](../examples/playwright/README.md)** - Playwright 浏览器自动化

  - 使用 Playwright + Chromium 在无头模式下抓取网页内容
  - 可提取网页标题、正文等信息
  - 适合网页爬虫和自动化测试

- **[desktop](../examples/desktop/README.md)** - VNC 桌面环境

  - 启动完整的桌面环境(Xvfb + x11vnc + fluxbox)
  - 通过 VNC 客户端远程访问沙箱桌面
  - 支持自定义 VNC 密码

- **[vscode](../examples/vscode/README.md)** - VS Code Web 环境
  - 在沙箱中运行 code-server (VS Code Web 版本)
  - 通过浏览器访问完整的 VS Code 开发环境
  - 适合远程开发和代码编辑场景

更多详细信息请参考 [examples](../examples/README.md) 和各示例目录下的 README 文件。

## 目录结构

```bash
OpenSandbox/
├── sdks/                     # 多语言 SDK
│   ├── code-interpreter/     # Code Interpreter SDK
│   └── sandbox/              # Sandbox 基础 SDK
├── specs/                    # OpenAPI 规范
│   ├── execd-api.yaml        # 命令执行和文件操作 API 规范
│   └── sandbox-lifecycle.yml # 沙箱生命周期 API 规范
├── server/                   # 沙箱服务端
├── components/               # 核心组件
│   └── execd/                # 命令执行和文件操作组件(Go)
├── docs/                     # 文档
├── examples/                 # 示例集成和使用案例
├── sandboxes/                # 沙箱实现
│   └── code-interpreter/     # Code Interpreter 沙箱实现
├── scripts/                  # 构建和工具脚本
└── e2e/                      # 端到端测试
```

## 文档

- [docs/architecture.md](architecture.md) – 整体架构 & 设计理念
- SDK
  - Sandbox 基础 SDK（[Java\Kotlin SDK](../sdks/sandbox/kotlin/README_zh.md)、[Python SDK](../sdks/sandbox/python/README_zh.md)）-包含沙箱生命周期、命令执行、文件操作
  - Code Interpreter SDK（[Java\Kotlin SDK](../sdks/code-interpreter/kotlin/README_zh.md) 、[Python SDK](../sdks/code-interpreter/python/README_zh.md)）- 代码解释器
- [specs/README.md](../specs/README_zh.md) - 包含沙箱生命周期 API 和沙箱执行 API 的 OpenAPI 定义
- [server/README.md](../server/README_zh.md) - 包含沙箱 Server 的启动和配置，目前支持 Docker Runtime，后续将支持 Kubernetes Runtime

---

## 许可证

本项目采用 [Apache 2.0 License](../LICENSE) 开源。

你可以在遵守许可条款的前提下，将 OpenSandbox 用于个人或商业项目。

---

## 联系与讨论

- Issue：通过 GitHub Issues 提交 bug、功能请求或设计讨论

欢迎一起把 OpenSandbox 打造成 AI 场景下的通用沙箱基础设施。
