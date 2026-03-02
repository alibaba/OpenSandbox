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
