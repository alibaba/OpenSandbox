# OpenSandbox SDK for Python

中文 | [English](README.md)

用于与 OpenSandbox 进行底层交互的 Python SDK。它提供了创建、管理和与安全沙箱环境交互的能力，包括执行 Shell 命令、管理文件和监控资源。

## 安装指南

### pip

```bash
pip install opensandbox
```

### uv

```bash
uv add opensandbox
```

## 快速开始

以下示例展示了如何创建一个沙箱并执行 Shell 命令。

> **注意**: 在运行此示例之前，请确保 OpenSandbox 服务已启动。服务启动请参考根目录的 [README_zh.md](../../../docs/README_zh.md)。

```python
import asyncio
from opensandbox.sandbox import Sandbox
from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import SandboxException

async def main():
    # 1. 配置连接信息
    config = ConnectionConfig(
        domain="api.opensandbox.io",
        api_key="your-api-key"
    )

    # 2. 创建 Sandbox
    try:
        sandbox = await Sandbox.create(
            "ubuntu",
            connection_config=config
        )
        async with sandbox:

            # 3. 执行 Shell 命令
            execution = await sandbox.commands.run("echo 'Hello Sandbox!'")

            # 4. 打印输出
            print(execution.logs.stdout[0].text)

            # 5. 清理资源 (自动调用 sandbox.close())
            # 注意: 如果希望立即终止远程沙箱实例，仍需显式调用 kill()
            await sandbox.kill()

    except SandboxException as e:
        # 处理 Sandbox 特定异常
        print(f"沙箱错误: [{e.error.code}] {e.error.message}")
    except Exception as e:
        print(f"错误: {e}")

if __name__ == "__main__":
    asyncio.run(main())
```

### 同步版本快速开始

如果你更偏好同步 API，可以使用 `SandboxSync` / `SandboxManagerSync` 与 `ConnectionConfigSync`：

```python
from datetime import timedelta

import httpx
from opensandbox import SandboxSync
from opensandbox.config import ConnectionConfigSync

config = ConnectionConfigSync(
    domain="api.opensandbox.io",
    api_key="your-api-key",
    request_timeout=timedelta(seconds=30),
    transport=httpx.HTTPTransport(limits=httpx.Limits(max_connections=20)),
)

sandbox = SandboxSync.create("ubuntu", connection_config=config)
with sandbox:
    execution = sandbox.commands.run("echo 'Hello Sandbox!'")
    print(execution.logs.stdout[0].text)
    sandbox.kill()
```

## 核心功能示例

### 1. 生命周期管理

管理沙箱的生命周期，包括续期、暂停、恢复和状态查询。

```python
from datetime import timedelta

# 续期沙箱
# 此操作将沙箱的过期时间重置为 (当前时间 + duration)
await sandbox.renew(timedelta(minutes=30))

# 暂停执行 (挂起所有进程)
await sandbox.pause()

# 恢复执行
sandbox = await Sandbox.resume(
    sandbox_id=sandbox.id,
    connection_config=config,
)

# 获取当前状态
info = await sandbox.get_info()
print(f"当前状态: {info.status.state}")
```

### 2. 自定义健康检查

定义自定义逻辑来判断沙箱是否健康。这可以覆盖默认的 Ping 检查。

```python
async def custom_health_check(sbx: Sandbox) -> bool:
    try:
        # 1. 获取沙箱 80 端口映射的外部访问地址
        endpoint = await sbx.get_endpoint(80)

        # 2. 执行你的连接检查逻辑 (例如 HTTP 请求、Socket 连接等)
        # return await check_connection(endpoint.endpoint)
        return True
    except Exception:
        return False

sandbox = await Sandbox.create(
    "nginx:latest",
    connection_config=config,
    health_check=custom_health_check  # 自定义检查：等待 80 端口可访问
)
```

### 3. 命令执行与流式响应

执行命令并实时处理输出流。

```python
from opensandbox.models.execd import ExecutionHandlers, RunCommandOpts

# 定义异步处理器用于流式输出
async def handle_stdout(msg):
    print(f"STDOUT: {msg.text}")

async def handle_stderr(msg):
    print(f"STDERR: {msg.text}")

async def handle_complete(complete):
    print(f"命令执行耗时: {complete.execution_time_in_millis}ms")

# 创建流式输出处理器 (所有处理器必须是异步函数)
handlers = ExecutionHandlers(
    on_stdout=handle_stdout,
    on_stderr=handle_stderr,
    on_execution_complete=handle_complete
)

# 带处理器的命令执行
result = await sandbox.commands.run(
    "for i in {1..5}; do echo \"Count $i\"; sleep 0.5; done"
    handlers=handlers,
)
```

### 4. 全面的文件操作

管理文件和目录，包括读写、列表、删除和搜索。

```python
from opensandbox.models.filesystem import WriteEntry, SearchEntry

# 1. 写入文件
await sandbox.files.write_files([
    WriteEntry(
        path="/tmp/hello.txt",
        data="Hello World",
        mode=0o644
    )
])

# 2. 读取文件
content = await sandbox.files.read_file("/tmp/hello.txt")
print(f"文件内容: {content}")

# 3. 搜索/列表文件
files = await sandbox.files.search(
    SearchEntry(
        path="/tmp",
        pattern="*.txt"
    )
)
for f in files:
    print(f"找到文件: {f.path}")

# 4. 删除文件
await sandbox.files.delete_files(["/tmp/hello.txt"])
```

### 5. 沙箱管理 (Sandbox Manager)

使用 `SandboxManager` 进行管理操作，如查询现有沙箱列表。

```python
from opensandbox.manager import SandboxManager
from opensandbox.models.sandboxes import SandboxFilter

# 使用异步上下文管理器创建管理器
async with await SandboxManager.create(connection_config=config) as manager:

    # 列出运行中的沙箱
    sandboxes = await manager.list_sandbox_infos(
        SandboxFilter(
            states=["RUNNING"],
            page_size=10
        )
    )

    for info in sandboxes.sandbox_infos:
        print(f"找到沙箱: {info.id}")
        # 执行管理操作
        await manager.kill_sandbox(info.id)
```

## 配置说明

### 1. 连接配置 (Connection Configuration)

`ConnectionConfig` 类管理与 API 服务器的连接设置。

| 参数              | 描述                                     | 默认值                   | 环境变量               |
| ----------------- | ---------------------------------------- | ------------------------ | ---------------------- |
| `api_key`         | 用于认证的 API Key                       | 必填                     | `OPEN_SANDBOX_API_KEY` |
| `domain`          | 沙箱服务的端点域名                       | 必填 (或 localhost:8080) | `OPEN_SANDBOX_DOMAIN`  |
| `protocol`        | HTTP 协议 (http/https)                   | `http`                   | -                      |
| `request_timeout` | API 请求超时时间                         | 30 秒                    | -                      |
| `debug`           | 是否开启 HTTP 请求的调试日志             | `False`                  | -                      |
| `headers`         | 自定义 HTTP 请求头                       | 空                       | -                      |
| `transport`       | 共享 httpx transport（连接池/代理/重试） | SDK 每实例创建           | -                      |
| `use_server_proxy` | 是否通过沙箱服务代理访问 execd/endpoint（适用于客户端无法直连沙箱的场景） | `False` | -                      |

```python
from datetime import timedelta

# 1. 基础配置
config = ConnectionConfig(
    api_key="your-key",
    domain="api.opensandbox.io",
    request_timeout=timedelta(seconds=60)
)

# 2. 进阶配置：自定义请求头和 transport
# 如果你需要创建大量 Sandbox 实例，建议配置共享 transport 以优化资源使用。
# SDK 默认连接保活时间为 30 秒。
import httpx

config = ConnectionConfig(
    api_key="your-key",
    domain="api.opensandbox.io",
    headers={"X-Custom-Header": "value"},
    transport=httpx.AsyncHTTPTransport(
        limits=httpx.Limits(
            max_connections=100,
            max_keepalive_connections=50,
        keepalive_expiry=30.0,
        )
    ),
)

# 如果你传入自定义 transport，需要你自己负责关闭：
# await config.transport.aclose()
```

### 2. 沙箱创建配置 (Sandbox Creation Configuration)

`Sandbox.create()` 用于配置沙箱环境。

| 参数            | 描述                 | 默认值                          |
| --------------- | -------------------- | ------------------------------- |
| `image`    | Docker 镜像        | 必填                            |
| `timeout`       | 自动终止的超时时间     | 10 分钟                         |
| `entrypoint`    | 容器启动入口命令       | `["tail", "-f", "/dev/null"]`   |
| `resource`      | CPU 和内存限制        | `{"cpu": "1", "memory": "2Gi"}` |
| `env`           | 环境变量             | 空                              |
| `metadata`      | 自定义元数据标签       | 空                              |
| `network_policy` | 可选的出站网络策略（egress） | -                         |
| `ready_timeout` | 等待沙箱就绪的最大时间 | 30 秒                           |

```python
from datetime import timedelta

from opensandbox.models.sandboxes import NetworkPolicy, NetworkRule

sandbox = await Sandbox.create(
    "python:3.11",
    connection_config=config,
    timeout=timedelta(minutes=30),
    resource={"cpu": "2", "memory": "4Gi"},
    env={"PYTHONPATH": "/app"},
    metadata={"project": "demo"},
    network_policy=NetworkPolicy(
        defaultAction="deny",
        egress=[NetworkRule(action="allow", target="pypi.org")],
    ),
)
```
