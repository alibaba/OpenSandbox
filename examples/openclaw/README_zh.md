# OpenClaw Gateway 示例

在 OpenSandbox 沙箱实例中启动 [OpenClaw](https://github.com/openclaw/openclaw) Gateway，并暴露 HTTP 访问端点。脚本会轮询 Gateway，直到返回 HTTP 200，然后打印可访问地址。

## 启动 OpenSandbox Server（本地）

最新 OpenClaw 镜像可在这里查看：[OpenClaw Container Registry](https://github.com/openclaw/openclaw/pkgs/container/openclaw)。

### 注意事项（Docker 运行时要求）

默认情况下，OpenSandbox Server 使用 `runtime.type = "docker"`，因此 **必须** 能访问可用的 Docker daemon。

- **Docker Desktop**：确保已启动，然后执行 `docker version` 验证。
- **Colima（macOS）**：先启动 (`colima start`)，再在启动 server 前导出 socket：

```shell
export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
```

预拉取 OpenClaw 镜像：

```shell
docker pull ghcr.io/openclaw/openclaw:latest
```

启动 OpenSandbox Server（日志会持续输出在当前终端）：

```shell
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker
opensandbox-server
```

如果出现 `docker/transport/unixconn.py` 的 `FileNotFoundError: [Errno 2] No such file or directory`，通常表示 Docker unix socket 不存在或 Docker 未启动。

## 创建并访问 OpenClaw Sandbox

该示例为快速体验预置了以下参数：

- OpenSandbox Server：`http://localhost:8080`
- 镜像：`ghcr.io/openclaw/openclaw:latest`
- Gateway 端口：`18789`
- 超时时间：`3600s`
- Token：`OPENCLAW_GATEWAY_TOKEN`（默认：`dummy-token-for-sandbox`）

在项目根目录安装依赖：

```shell
uv pip install opensandbox requests
```

运行示例（如需鉴权访问请设置真实 token）：

```shell
export OPENCLAW_GATEWAY_TOKEN="$(openssl rand -hex 32)"
uv run python examples/openclaw/main.py
```

预期输出类似：

```text
Creating openclaw sandbox with image=ghcr.io/openclaw/openclaw:latest on OpenSandbox server http://localhost:8080...
[check] sandbox ready after 7.1s
Openclaw started finished. Please refer to 127.0.0.1:56123
```

最后打印的地址（如 `127.0.0.1:56123`）就是沙箱中 OpenClaw Gateway 的可访问端点。

## 参考

- [OpenClaw](https://github.com/openclaw/openclaw)
- [OpenSandbox Python SDK](https://pypi.org/project/opensandbox/)
