# OpenSandbox Server（沙箱服务端）

中文 | [English](README.md)

基于 FastAPI 的生产级容器化沙箱生命周期管理服务。作为控制平面，协调在不同容器编排环境中的隔离运行时的创建、执行、监控与销毁。

## 功能特性

### 核心能力
- **生命周期管理**：标准化 REST API 覆盖创建、启动、暂停、恢复、删除
- **可插拔运行时**：
  - **Docker**：已支持生产部署
  - **Kubernetes**：配置占位，开发中
- **自动过期**：可配置 TTL，支持续期
- **访问控制**：API Key 认证（`OPEN-SANDBOX-API-KEY`），本地/开发可配置为空跳过
- **网络模式**：
  - Host：共享宿主网络，性能优先
  - Bridge：隔离网络，内置 HTTP 代理路由
- **资源配额**：CPU/内存限制，Kubernetes 风格规范
- **状态可观测性**：统一状态与转换跟踪
- **镜像仓库**：支持公共与私有镜像

### 扩展能力
- **异步供应**：后台创建，降低请求延迟
- **定时恢复**：重启后自动恢复过期定时器
- **环境与元数据注入**：按沙箱注入 env 与 metadata
- **端口解析**：动态生成访问端点
- **结构化错误**：标准错误码与消息，便于排障

## 环境要求

- **Python**：3.10 或更高版本
- **包管理器**：[uv](https://github.com/astral-sh/uv)（推荐）或 pip
- **运行时后端**：
  - Docker Engine 20.10+（使用 Docker 运行时）
  - Kubernetes 1.21+（使用 Kubernetes 运行时，开发中）
- **操作系统**：Linux、macOS 或带 WSL2 的 Windows

## 快速开始

### 安装步骤

1. **克隆仓库**并进入 server 目录：

```bash
cd server
```

2. **安装依赖**（使用 `uv`）：

```bash
uv sync
```

### 配置指南

服务端使用 TOML 配置文件来选择和配置底层运行时。

**复制配置文件**：

```bash
cp example.config.zh.toml ~/.sandbox.toml
```
**[可选] 复制K8S版本配置文件：
需要在集群中部署 K8S版本的Sandbox Operator，参考Kubernetes目录。
```bash
cp example.config.k8s.zh.toml ~/.sandbox.toml
cp example.batchsandbox-template.yaml ~/batchsandbox-template.yaml
```

**[可选] 编辑 `~/.sandbox.toml`** 适配您的环境：


**选项 A：Docker 运行时 + Host 网络模式**
```toml
[server]
host = "0.0.0.0"
port = 8080
log_level = "INFO"
api_key = "your-secret-api-key-change-this"

[runtime]
type = "docker"
execd_image = "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/execd:v1.0.3"

[docker]
network_mode = "host"  # 容器共享宿主机网络，只能创建一个sandbox实例
```

**选项 B：Docker 运行时 + Bridge 网络模式**
```toml
[server]
host = "0.0.0.0"
port = 8080
log_level = "INFO"
api_key = "your-secret-api-key-change-this"

[runtime]
type = "docker"
execd_image = "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/execd:v1.0.3"

[docker]
network_mode = "bridge"  # 容器隔离网络
```

**安全加固（适用于所有 Docker 模式）**
```toml
[docker]
# 默认关闭危险能力、防止提权
drop_capabilities = ["AUDIT_WRITE", "MKNOD", "NET_ADMIN", "NET_RAW", "SYS_ADMIN", "SYS_MODULE", "SYS_PTRACE", "SYS_TIME", "SYS_TTY_CONFIG"]
no_new_privileges = true
# 当宿主机启用了 AppArmor 时，可指定策略名称（如 "docker-default"）；否则留空
apparmor_profile = ""
# 限制进程数量，可选的 seccomp/只读根文件系统
pids_limit = 512             # 设为 null 可关闭
seccomp_profile = ""        # 配置文件路径或名称；为空使用 Docker 默认
```
更多 Docker 安全参考：https://docs.docker.com/engine/security/

### 启动服务

使用 `uv` 启动服务：

```bash
uv run python -m src.main
```

服务将在 `http://0.0.0.0:8080`（或您配置的主机/端口）启动。

**健康检查**

```bash
curl http://localhost:8080/health
```

预期响应：
```json
{"status": "healthy"}
```

## API 文档

服务启动后，可访问交互式 API 文档：

- **Swagger UI**：[http://localhost:8080/docs](http://localhost:8080/docs)
- **ReDoc**：[http://localhost:8080/redoc](http://localhost:8080/redoc)

### API 认证

仅当 `server.api_key` 设置为非空值时才启用鉴权；当该值为空或缺省时，中间件会跳过 API Key 校验（适合本地/开发调试）。生产环境请务必设置强随机的 `server.api_key`，并在请求头 `OPEN-SANDBOX-API-KEY` 中携带。

当鉴权开启时，除 `/health`、`/docs`、`/redoc` 外的 API 端点均需要通过 `OPEN-SANDBOX-API-KEY` 请求头进行认证：

```bash
curl http://localhost:8080/v1/sandboxes
```

### 使用示例

**创建沙箱**

```bash
curl -X POST "http://localhost:8080/v1/sandboxes" \
  -H "Content-Type: application/json" \
  -d '{
    "image": {
      "uri": "python:3.11-slim"
    },
    "entrypoint": [
      "python",
      "-m",
      "http.server",
      "8000"
    ],
    "timeout": 3600,
    "resourceLimits": {
      "cpu": "500m",
      "memory": "512Mi"
    },
    "env": {
      "PYTHONUNBUFFERED": "1"
    },
    "metadata": {
      "team": "backend",
      "project": "api-testing"
    }
  }'
```

响应：
```json
{
  "id": "<sandbox-id>",
  "status": {
    "state": "Pending",
    "reason": "CONTAINER_STARTING",
    "message": "Sandbox container is starting.",
    "lastTransitionAt": "2024-01-15T10:30:00Z"
  },
  "metadata": {
    "team": "backend",
    "project": "api-testing"
  },
  "expiresAt": "2024-01-15T11:30:00Z",
  "createdAt": "2024-01-15T10:30:00Z",
  "entrypoint": ["python", "-m", "http.server", "8000"]
}
```

**获取沙箱详情**

```bash
curl http://localhost:8080/v1/sandboxes/<sandbox-id>
```

**获取服务端点**

```bash
# 获取自定义服务端点
curl http://localhost:8080/v1/sandboxes/<sandbox-id>/endpoints/8000

# 获取OpenSandbox守护进程（execd）端点
curl http://localhost:8080/v1/sandboxes/<sandbox-id>/endpoints/44772
```

响应：
```json
{
  "endpoint": "sandbox.example.com/<sandbox-id>/8000"
}
```

**续期沙箱**

```bash
curl -X POST "http://localhost:8080/v1/sandboxes/<sandbox-id>/renew-expiration" \
  -H "Content-Type: application/json" \
  -d '{
    "expiresAt": "2024-01-15T12:30:00Z"
  }'
```

**删除沙箱**

```bash
curl -X DELETE http://localhost:8080/v1/sandboxes/<sandbox-id>
```

## 系统架构

### 组件职责

- **API 层**（`src/api/`）：HTTP 请求处理、验证和响应格式化
- **服务层**（`src/services/`）：沙箱生命周期操作的业务逻辑
- **中间件**（`src/middleware/`）：横切关注点（认证、日志）
- **配置**（`src/config.py`）：集中式配置管理
- **运行时实现**：平台特定的沙箱编排

### 沙箱生命周期状态

```
       create()
          │
          ▼
     ┌─────────┐
     │ Pending │────────────────────┐
     │ 待处理   │                    │
     └────┬────┘                    │
          │                         │
          │ (provisioning 供应中)    │
          ▼                         │
     ┌─────────┐    pause()         │
     │ Running │───────────────┐    │
     │ 运行中   │               │    │
     └────┬────┘               │    │
          │      resume()      │    │
          │   ┌────────────────┘    │
          │   │                     │
          │   ▼                     │
          │ ┌────────┐              │
          ├─│ Paused │              │
          │ │ 已暂停  │              │
          │ └────────┘              │
          │                         │
          │ delete() or expire()    │
          ▼                         │
     ┌──────────┐                   │
     │ Stopping │                   │
     │ 停止中    │                   │
     └────┬─────┘                   │
          │                         │
          ├────────────────┬────────┘
          │                │
          ▼                ▼
     ┌────────────┐   ┌────────┐
     │ Terminated │   │ Failed │
     │  已终止     │   │  失败   │
     └────────────┘   └────────┘
```

## 配置参考

### 服务器配置

| 键 | 类型 | 默认值 | 描述 |
|----|------|--------|------|
| `server.host` | string | `"0.0.0.0"` | 绑定的网络接口 |
| `server.port` | integer | `8080` | 监听端口 |
| `server.log_level` | string | `"INFO"` | Python 日志级别 |
| `server.api_key` | string | `null` | API 认证密钥 |

### 运行时配置

| 键 | 类型 | 必需 | 描述 |
|----|------|------|------|
| `runtime.type` | string | 是 | 运行时实现（`"docker"` 或 `"kubernetes"`）|
| `runtime.execd_image` | string | 是 | 包含 execd 二进制文件的容器镜像 |

### Docker 配置

| 键 | 类型 | 默认值 | 描述 |
|----|------|--------|------|
| `docker.network_mode` | string | `"host"` | 网络模式（`"host"` 或 `"bridge"`）|

### 环境变量

| 变量 | 描述 |
|------|------|
| `SANDBOX_CONFIG_PATH` | 覆盖配置文件位置 |
| `DOCKER_HOST` | Docker 守护进程 URL（例如 `unix:///var/run/docker.sock`）|
| `DOCKER_API_TIMEOUT` | Docker 客户端超时时间（秒，默认：180）|
| `PENDING_FAILURE_TTL` | 失败的待处理沙箱的 TTL（秒，默认：3600）|

## 开发

### 代码质量

**运行代码检查**：
```bash
uv run ruff check
```

**自动修复问题**：
```bash
uv run ruff check --fix
```

**格式化代码**：
```bash
uv run ruff format
```

### 测试

**运行所有测试**：
```bash
uv run pytest
```

**带覆盖率运行**：
```bash
uv run pytest --cov=src --cov-report=html
```

**运行特定测试**：
```bash
uv run pytest tests/test_docker_service.py::test_create_sandbox_requires_entrypoint
```

## 许可证

本项目遵循仓库根目录下的 LICENSE 文件条款。

## 贡献

欢迎提交改进，建议遵循以下流程：

1. Fork 仓库
2. 创建特性分支（`git checkout -b feature/amazing-feature`）
3. 为新功能编写测试
4. 确保所有测试通过（`uv run pytest`）
5. 运行代码检查（`uv run ruff check`）
6. 使用清晰的消息提交
7. 推送到您的 fork
8. 打开 Pull Request

## 支持

- 文档：参阅 `DEVELOPMENT.md` 获取开发指南
- 问题报告：通过 GitHub Issues 报告缺陷
- 讨论：在 GitHub Discussions 进行答疑与交流
