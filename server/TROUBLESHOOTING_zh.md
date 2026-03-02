# 故障排查

[English](TROUBLESHOOTING.md) | 中文

## `exec /opt/opensandbox/bootstrap.sh: operation not permitted`

如果沙箱日志出现：

```text
exec /opt/opensandbox/bootstrap.sh: operation not permitted
```

建议先检查：

1. 确认脚本在沙箱容器内存在且可执行：
   ```bash
   docker exec -it <sandbox-container> ls -l /opt/opensandbox/bootstrap.sh
   ```
2. 检查运行时安全策略和挂载约束是否阻止执行（例如严格沙箱约束或 `noexec` 挂载行为）。
3. 如果使用 Snap 版本 Docker（如 Ubuntu Core 场景），生产环境建议优先使用 Docker CE 安装方式，因为部分严格约束环境会影响该 bootstrap 执行路径。
4. 升级并复现：使用最新 server / execd 镜像确认是否已包含修复。

如果仍可复现，建议附带以下信息提 issue：
- `docker info`
- `docker logs opensandbox-server`
- `docker logs <sandbox-container>`
- `config.toml`（注意脱敏）

## 沙箱健康检查超时（如阿里云 ECS）

若服务端部署在云主机（例如 [阿里云 ECS](https://github.com/alibaba/OpenSandbox/issues/297)），客户端创建沙箱时出现：

```text
opensandbox.exceptions.sandbox.SandboxReadyTimeoutException: Sandbox health check timed out after 30.0s (2 attempts). Health check returned false continuously
```

通常是因为服务端返回的 endpoint 地址（如 `127.0.0.1` 或内网 IP）对客户端不可达，客户端无法完成健康检查。

**解决办法：** 配置绑定的公网 IP，让服务端在返回 sandbox endpoint 时使用客户端可访问的地址。在配置文件（如 `~/.sandbox.toml`）的 `[server]` 下设置 `eip` 为云主机的公网 IP（或客户端访问该服务时使用的主机名）：

```toml
[server]
host = "0.0.0.0"
port = 8080
eip = "47.x.x.x"   # 你的 ECS 公网 IP，或客户端用来访问本机的主机名
```

重启服务后，获取 endpoint 的 API 会使用 `eip` 作为返回地址的 host 部分，客户端即可连通沙箱并通过健康检查。该行为针对 Docker 运行时；配置了 `eip` 后，服务端将不再根据 `host` 解析地址。
