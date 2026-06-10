# Credential Vault

Credential Vault 是 OpenSandbox 为沙箱内 Agent 和开发工具提供的出站凭证代理能力。真实凭证由宿主侧 SDK 写入 egress sidecar，沙箱进程只拿到假的或空的凭证值。当 Claude Code、Git、curl、包管理器或模型 API 客户端等工具发起被允许的 HTTPS 出站请求时，sidecar 会根据 Credential Vault binding 匹配请求，并在出站链路上注入所需的认证 header。这样工具可以保持原有使用方式，同时真实密钥不会进入沙箱环境变量、命令行、文件系统或日志，从而降低 prompt injection 或不可信代码导致的凭证外泄风险。

## 原理

Credential Vault 由 egress sidecar 提供。通过 Python SDK 创建沙箱时需要同时设置 `network_policy` 和 `credential_proxy.enabled = true`；对应的 lifecycle API 字段名是 `credentialProxy.enabled`。

整体流程如下：

1. lifecycle server 为沙箱注入 egress sidecar。
2. SDK 将凭证和绑定规则写入 sidecar 的 Credential Vault API。
3. 沙箱进程只拿到假的或空的凭证环境变量。
4. 沙箱发起 HTTPS 出站请求时，sidecar 中的透明 MITM 会检查请求元信息。
5. 如果某条绑定唯一匹配请求的 scheme、host、port、method 和 path，sidecar 会注入配置的认证 header。
6. Vault 响应和 response headers 中会对密钥值做 redaction。

MITM 进程读取的 active vault 只通过 sidecar 内部的 Unix domain socket 暴露。沙箱工作负载不能通过普通 server proxy 路径读取 active vault。

绑定规则应该尽量精确。推荐使用默认拒绝的 egress 策略，并把路径限制到最小范围，例如 Anthropic API 使用 `/v1/*`。

## Auth 类型

每条 binding 都通过 `auth` 规则描述如何把引用的 credential 注入到出站请求中：

- `bearer`：注入 `Authorization: Bearer <credential>`。
- `basic`：注入 `Authorization: Basic <credential>`。credential 值需要提前编码成 base64 格式的 `username:password`。
- `apiKey`：把 credential 值注入到配置的 header name 中。
- `customHeaders`：一次注入多个配置 header，每个 header 可以引用自己的 credential。

简单示例：

```python
auth={"type": "bearer", "credential": "github-token"}
```

```http
Authorization: Bearer <github-token>
```

```python
auth={"type": "basic", "credential": "registry-basic"}
```

```http
Authorization: Basic <base64(username:password)>
```

```python
auth={"type": "apiKey", "name": "x-api-key", "credential": "anthropic-api-key"}
```

```http
x-api-key: <anthropic-api-key>
```

```python
auth={
    "type": "customHeaders",
    "headers": [
        {"name": "X-Client-Id", "credential": "client-id"},
        {"name": "X-Client-Secret", "credential": "client-secret"},
    ],
}
```

```http
X-Client-Id: <client-id>
X-Client-Secret: <client-secret>
```

## 前置条件

- Server 配置中设置了 `[egress].image`。
- 创建沙箱时传入 `network_policy`。
- 创建沙箱时设置 `credential_proxy=CredentialProxyConfig(enabled=True)`。
- 沙箱镜像包含要运行的工具。运行 Claude Code 时，可以使用包含 Node.js 和 npm 的 OpenSandbox code-interpreter 镜像。

## Claude Code 调用 Anthropic 官方 API

下面的示例会在沙箱中安装 Claude Code，并访问 Anthropic 官方 API 地址。真实 API key 从宿主机环境变量读取并写入 Credential Vault；沙箱中只放一个假的 `ANTHROPIC_API_KEY`。

运行前准备：

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
# 可选：export ANTHROPIC_MODEL="<Claude Code 支持的 Anthropic 模型>"
```

示例代码：

```python
import os
from datetime import timedelta

from opensandbox import SandboxSync
from opensandbox.models.sandboxes import (
    Credential,
    CredentialBinding,
    CredentialProxyConfig,
    NetworkPolicy,
    NetworkRule,
    SandboxImageSpec,
)


ANTHROPIC_HOST = "api.anthropic.com"
ANTHROPIC_BASE_URL = "https://api.anthropic.com"
REAL_API_KEY = os.environ["ANTHROPIC_API_KEY"]


sandbox_env = {
    "ANTHROPIC_BASE_URL": ANTHROPIC_BASE_URL,
    "ANTHROPIC_API_KEY": "fake-key-inside-sandbox",
}
if os.getenv("ANTHROPIC_MODEL"):
    sandbox_env["ANTHROPIC_MODEL"] = os.environ["ANTHROPIC_MODEL"]


sandbox = SandboxSync.create(
    image=SandboxImageSpec(
        os.getenv("SANDBOX_IMAGE", "opensandbox/code-interpreter:latest")
    ),
    timeout=timedelta(minutes=15),
    env=sandbox_env,
    network_policy=NetworkPolicy(
        defaultAction="deny",
        egress=[
            NetworkRule(action="allow", target=ANTHROPIC_HOST),
            NetworkRule(action="allow", target="registry.npmjs.org"),
        ],
    ),
    credential_proxy=CredentialProxyConfig(enabled=True),
)

try:
    sandbox.credential_vault.create(
        credentials=[
            Credential(
                name="anthropic-api-key",
                source={"value": REAL_API_KEY},
            )
        ],
        bindings=[
            CredentialBinding(
                name="anthropic-api",
                match={
                    "schemes": ["https"],
                    "ports": [443],
                    "hosts": [ANTHROPIC_HOST],
                    "methods": ["GET", "POST"],
                    "paths": ["/v1/*"],
                },
                auth={
                    "type": "apiKey",
                    "name": "x-api-key",
                    "credential": "anthropic-api-key",
                },
            )
        ],
    )

    sandbox.commands.run(
        "npm install -g @anthropic-ai/claude-code --no-audit --no-fund"
    )
    result = sandbox.commands.run("claude -p '1+1'")
    output = "".join(part.text for part in result.logs.stdout)
    print(output)
finally:
    sandbox.kill()
    sandbox.close()
```

Claude Code 进程读取到的是假的 `ANTHROPIC_API_KEY`，但访问 `api.anthropic.com/v1/*` 时，Credential Vault 会在出站 HTTPS 请求上注入真实的 `x-api-key` header。如果你的环境使用 npm 私有镜像，需要把 network policy 和 `npm install` 命令中的 `registry.npmjs.org` 替换成对应镜像域名。

## 使用建议

- 使用 `defaultAction="deny"`，只 allow 工具实际需要访问的服务域名。
- 尽量按 path 缩小绑定范围，例如 `/v1/*`。
- 避免同优先级绑定重叠；匹配不唯一会被拒绝。
- 不要把真实密钥放进沙箱 `env`、命令参数、文件或 metadata。
- 如果 CLI 没有 key 不启动，可以放 fake env；真正认证依赖 Vault 注入的 header。
