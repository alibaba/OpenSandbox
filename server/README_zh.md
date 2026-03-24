# OpenSandbox Server（沙箱服务端）

中文 | [English](README.md)

基于 FastAPI 的生产级容器化沙箱生命周期管理服务。作为控制平面，协调在不同容器编排环境中的隔离运行时的创建、执行、监控与销毁。

## 功能特性

### 核心能力
- **生命周期管理**：标准化 REST API 覆盖创建、启动、暂停、恢复、删除
- **可插拔运行时**：
  - **Docker**：已支持生产部署
  - **Kubernetes**：已支持生产部署
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
  - Kubernetes 1.21.1+（使用 Kubernetes 运行时）
- **操作系统**：Linux、macOS 或带 WSL2 的 Windows

## 快速开始

### 安装步骤

1. **通过 PyPI 安装**（无需克隆仓库）：

```bash
uv pip install opensandbox-server
```
> 如需源码开发或贡献，可仍然克隆仓库并在 `server/` 下执行 `uv sync`。

### 配置指南

服务端使用 TOML 配置文件来选择和配置底层运行时。

**从简单示例初始化配置**：
```bash
# 运行 opensandbox-server -h 查看帮助
opensandbox-server init-config ~/.sandbox.toml --example docker-zh
```

**创建 K8S 配置文件**

需要在集群中部署 K8S 版本的 Sandbox Operator，参考 Kubernetes 目录。
```bash
# 运行 opensandbox-server -h 查看帮助
opensandbox-server init-config ~/.sandbox.toml --example k8s-zh
```

**[可选] 编辑配置以适配您的环境**

- 用于快速 e2e/demo：
  ```bash
  opensandbox-server init-config ~/.sandbox.toml --example docker-zh  # 或 docker-zh|k8s|k8s-zh
  # 已有文件需覆盖时加 --force
  ```
- 省略 `--example` 时生成“配置框架”（无默认值，只有占位符）：
  ```bash
  opensandbox-server init-config ~/.sandbox.toml
  # 已有文件需覆盖时加 --force
  ```

**[可选] 编辑 `~/.sandbox.toml`** 适配您的环境

在启动服务器前，编辑配置文件以适配您的环境。您也可以通过 `opensandbox-server init-config ~/.sandbox.toml` 生成一个新的完整配置模板。

**Docker 运行时 + Host 网络模式**
   ```toml
   [server]
   host = "0.0.0.0"
   port = 8080
   log_level = "INFO"
   api_key = "your-secret-api-key-change-this"

   [runtime]
   type = "docker"
   execd_image = "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/execd:v1.0.7"

   [docker]
   network_mode = "host"  # 容器共享宿主机网络，只能创建一个sandbox实例
   ```

**Docker 运行时 + Bridge 网络模式**
   ```toml
   [server]
   host = "0.0.0.0"
   port = 8080
   log_level = "INFO"
   api_key = "your-secret-api-key-change-this"

   [runtime]
   type = "docker"
   execd_image = "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/execd:v1.0.7"

   [docker]
   network_mode = "bridge"  # 容器隔离网络
   ```

**Docker Compose 部署（server 本身运行在容器中）**

当 `opensandbox-server` 运行在 Docker Compose 容器内，并通过挂载
`/var/run/docker.sock` 管理沙箱时，需要为 bridge 模式端点解析配置一个可达的宿主地址：

```toml
[docker]
network_mode = "bridge"
host_ip = "host.docker.internal"  # 或宿主机 LAN IP（Linux 建议显式填写）
```

原因：
- bridge 模式下沙箱容器会分配 Docker 内部 IP。
- 外部调用方通常无法直接访问这些内部 IP。
- `host_ip` 会让端点解析返回对调用方可达的宿主地址。

对于无法直连 sandbox bridge 地址的 SDK/API 调用方，可通过 server 代理获取端点：

```bash
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  "http://localhost:8080/v1/sandboxes/<sandbox-id>/endpoints/44772?use_server_proxy=true"
```

返回端点会被重写为 server 代理路径：
- `<server-host>/sandboxes/<sandbox-id>/proxy/<port>`

可参考 Compose 运行示例：
- `server/docker-compose.example.yaml`

**实验性**生命周期能力（例如按访问自动续期）见文末 [实验性功能](#实验性功能) 一节（位于 [配置参考](#配置参考) 之后）。

**安全加固（适用于所有 Docker 模式）**
   ```toml
   [docker]
   # 默认关闭危险能力、防止提权
   drop_capabilities = ["AUDIT_WRITE", "MKNOD", "NET_ADMIN", "NET_RAW", "SYS_ADMIN", "SYS_MODULE", "SYS_PTRACE", "SYS_TIME", "SYS_TTY_CONFIG"]
   no_new_privileges = true
   apparmor_profile = ""        # 例如当 AppArmor 可用时使用 "docker-default"
   # 限制进程数量
   pids_limit = 512             # 设为 null 可关闭
   seccomp_profile = ""        # 配置文件路径或名称；为空使用 Docker 默认
   ```
   更多 Docker 容器安全参考：https://docs.docker.com/engine/security/

常见问题及解决方案请参阅 [故障排查](TROUBLESHOOTING_zh.md)。

**安全容器运行时（可选）**

OpenSandbox 支持安全容器运行时以增强隔离性：

```toml
[secure_runtime]
type = "gvisor"              # 选项: "", "gvisor", "kata", "firecracker"
docker_runtime = "runsc"      # Docker OCI 运行时名称（用于 gVisor、Kata）
# k8s_runtime_class = "gvisor"  # Kubernetes RuntimeClass 名称（用于 K8s）
```

- `type=""`（默认）：不使用安全运行时，使用 runc
- `type="gvisor"`：使用 gVisor (runsc) 实现用户态内核隔离
- `type="kata"`：使用 Kata Containers 实现 VM 级隔离
- `type="firecracker"`：使用 Firecracker 微虚拟机（仅 Kubernetes）

> **详细指南**：参阅 [安全容器运行时指南](../docs/secure-container.md) 获取完整的安装说明、系统要求和故障排除。

**Docker daemon 配置** gVisor 示例：
```json
{
  "runtimes": {
    "runsc": {
      "path": "/usr/bin/runsc"
    }
  }
}
```

**Kubernetes 配置**：使用前需先创建 RuntimeClass：
```bash
kubectl create -f - <<EOF
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
EOF
```

**Ingress 暴露（direct | gateway）**
```toml
[ingress]
mode = "direct"  # Docker 运行时仅支持 direct（直连，无 L7 网关）
# gateway.address = "*.example.com"  # 仅主机（域名/IP 或 IP:port），不允许带 scheme
# gateway.route.mode = "wildcard"            # wildcard | uri | header
```
- `mode=direct`：默认；当 `runtime.type=docker` 时必须使用（客户端与 sandbox 直连，不经过网关）。
- `mode=gateway`：配置外部入口。
  - `gateway.address`：当 `gateway.route.mode=wildcard` 时必须是泛域名；其他模式需为域名/IP 或 IP:port。不允许携带 scheme，客户端自行选择 http/https。
  - `gateway.route.mode`：`wildcard`（域名泛匹配）、`uri`（基于路径前缀）、`header`（基于请求头路由）。
  - 返回示例：
    - `wildcard`：`<sandbox-id>-<port>.example.com/path/to/request`
    - `uri`：`10.0.0.1:8000/<sandbox-id>/<port>/path/to/request`
    - `header`：`gateway.example.com`，请求头 `OpenSandbox-Ingress-To: <sandbox-id>-<port>`

**Kubernetes 运行时**
   ```toml
   [runtime]
   type = "kubernetes"
   execd_image = "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/execd:v1.0.7"

   [kubernetes]
   kubeconfig_path = "~/.kube/config"
   namespace = "opensandbox"
   workload_provider = "batchsandbox"        # 或 "agent-sandbox"
   informer_enabled = true                   # Beta：启用 watch 缓存
   informer_resync_seconds = 300             # Beta：全量刷新间隔
   informer_watch_timeout_seconds = 60       # Beta：watch 超时重连间隔
   ```
   - Informer 配置为 **Beta**，默认开启以减少 API 压力；若需关闭设置 `informer_enabled = false`。
   - resync / watch 超时用于控制缓存刷新频率，可根据集群 API 限流调优。

### Egress 配置（`[egress]` 配置块）

**`[egress]`** 用于配置 **egress 侧车** 的镜像与执行模式。仅当创建沙箱的请求中带有 **`networkPolicy`**（出站允许/拒绝规则）时，服务器才会注入该侧车；若请求未带 `networkPolicy`，不会添加 egress 侧车，也不会通过该机制限制出站流量。

#### 配置项

| 键 | 类型 | 默认值 | 何时必填 | 说明 |
|----|------|--------|----------|------|
| `image` | string | — | 任意一次创建请求携带 `networkPolicy` 时 **必填** | 包含 egress 可执行文件的容器镜像；侧车启动前会拉取或校验镜像。 |
| `mode` | `dns` 或 `dns+nft` | `dns` | 否 | 侧车如何执行策略，写入环境变量 `OPENSANDBOX_EGRESS_MODE`（见下）。 |

#### `mode` 取值

- **`dns`**：通过侧车内 DNS 代理做基于域名的策略；不依赖本路径下的 nftables 二层规则。**策略中的 CIDR、静态 IP 类目标不会被强制执行**（若只用 `dns` 模式，请使用域名类规则）。
- **`dns+nft`**：在 `dns` 的基础上启用 nftables（能力与回退行为见 [egress 组件说明](../components/egress/README.md)）。**支持 CIDR 与静态 IP 的放行/拒绝规则**（nftables 表成功下发时生效）。

#### 请求体中的 `networkPolicy`

- 规则在 **`CreateSandboxRequest.networkPolicy`** 中声明（默认动作与有序的 egress 规则：域名/通配符；在使用 **`dns+nft`** 时还可包含 IP 或 CIDR 条目）。
- 序列化后的策略以 JSON 形式注入侧车环境变量 **`OPENSANDBOX_EGRESS_RULES`**。
- 可能同时下发用于 egress HTTP API 的鉴权信息（与运行时行为一致）。

#### Docker 运行时

- 客户端传入 `networkPolicy` 时，配置中必须设置 **`egress.image`**，否则请求会被拒绝。
- 出站策略要求 **`docker.network_mode = "bridge"`**；`network_mode=host` 或与侧车挂载模型不兼容的用户自定义网络下，携带 `networkPolicy` 的请求会被拒绝。
- 主沙箱容器与侧车 **共享网络命名空间**，主容器 **drop `NET_ADMIN`**，由侧车保留 **`NET_ADMIN`** 完成策略相关操作。
- 共享 netns 内会 **禁用 IPv6**，以保证放行/拒绝行为一致。

#### Kubernetes 运行时

- 当请求带有 `networkPolicy` 时，工作负载 Pod 中除主容器外，还会增加基于 **`egress.image`** 的 **egress** 侧车。
- **`egress.image`** 的必填规则与 Docker 相同。

#### 运维说明

- 侧车镜像在启动前拉取或校验；删除、过期、失败等路径会尽量清理侧车。
- DNS 代理、nftables、能力边界等详见仓库内 **`components/egress/`** 文档。

#### 配置示例（`~/.sandbox.toml`）

```toml
[runtime]
type = "docker"
execd_image = "opensandbox/execd:v1.0.7"

[egress]
image = "opensandbox/egress:v1.0.3"
mode = "dns"
```

#### 带 `networkPolicy` 的创建请求示例

```json
{
  "image": {"uri": "python:3.11-slim"},
  "entrypoint": ["python", "-m", "http.server", "8000"],
  "timeout": 3600,
  "resourceLimits": {"cpu": "500m", "memory": "512Mi"},
  "networkPolicy": {
    "defaultAction": "deny",
    "egress": [
      {"action": "allow", "target": "pypi.org"},
      {"action": "allow", "target": "*.python.org"}
    ]
  }
}
```

### 启动服务

使用安装后的 CLI 启动（默认读取 `~/.sandbox.toml`）：

```bash
opensandbox-server
```

服务将在 `http://0.0.0.0:8080`（或您配置的主机/端口）启动。

### 启动服务（安装包方式）

安装为 Python 包后，可直接使用 CLI 启动：

```bash
opensandbox-server --config ~/.sandbox.toml
```

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

仅当 `server.api_key` 设置为非空值时才启用鉴权；当该值为空或缺省时，中间件会跳过 API Key 校验（适合本地/开发调试）。生产环境请务必设置非空的 `server.api_key`，并通过 `OPEN-SANDBOX-API-KEY` 请求头发送。

当鉴权开启时，除 `/health`、`/docs`、`/redoc` 外的 API 端点均需要通过 `OPEN-SANDBOX-API-KEY` 请求头进行认证：

```bash
curl http://localhost:8080/v1/sandboxes
```

### 使用示例

**创建沙箱**

```bash
curl -X POST "http://localhost:8080/v1/sandboxes" \
  -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
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
  "id": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
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
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab
```

**获取服务端点**

```bash
# 获取自定义服务端点
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/endpoints/8000

# 获取OpenSandbox守护进程（execd）端点
curl -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/endpoints/44772
```

响应：
```json
{
  "endpoint": "sandbox.example.com/a1b2c3d4-5678-90ab-cdef-1234567890ab/8000"
}
```

**续期沙箱**

```bash
curl -X POST "http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab/renew-expiration" \
  -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "expiresAt": "2024-01-15T12:30:00Z"
  }'
```

**删除沙箱**

```bash
curl -X DELETE \
  -H "OPEN-SANDBOX-API-KEY: your-secret-api-key" \
  http://localhost:8080/v1/sandboxes/a1b2c3d4-5678-90ab-cdef-1234567890ab
```

## 系统架构

### 组件职责

- **API 层**（`opensandbox_server/api/`）：HTTP 请求处理、验证和响应格式化
- **服务层**（`opensandbox_server/services/`）：沙箱生命周期操作的业务逻辑
- **中间件**（`opensandbox_server/middleware/`）：横切关注点（认证、日志）
- **配置**（`opensandbox_server/config.py`）：集中式配置管理
- **运行时实现**：平台特定的沙箱编排

### 沙箱生命周期状态

```
       create()
          │
          ▼
     ┌─────────┐
     │ Pending │────────────────────┐
     └────┬────┘                    │
          │                         │
          │ (provisioning)          │
          ▼                         │
     ┌─────────┐    pause()         │
     │ Running │───────────────┐    │
     └────┬────┘               │    │
          │      resume()      │    │
          │   ┌────────────────┘    │
          │   │                     │
          │   ▼                     │
          │ ┌────────┐              │
          ├─│ Paused │              │
          │ └────────┘              │
          │                         │
          │ delete() or expire()    │
          ▼                         │
     ┌──────────┐                   │
     │ Stopping │                   │
     └────┬─────┘                   │
          │                         │
          ├────────────────┬────────┘
          │                │
          ▼                ▼
     ┌────────────┐   ┌────────┐
     │ Terminated │   │ Failed │
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
| `server.eip` | string | `null` | 绑定的公网 IP；配置后，返回 sandbox endpoint 时作为地址的 host 部分（Docker 运行时） |

### 运行时配置

| 键                      | 类型     | 必需 | 描述                                 |
|------------------------|--------|----|------------------------------------|
| `runtime.type`         | string | 是  | 运行时实现（`"docker"` 或 `"kubernetes"`） |
| `runtime.execd_image`  | string | 是  | 包含 execd 二进制文件的容器镜像                |

### Egress 配置

| 键 | 类型 | 默认值 | 使用 `networkPolicy` 时是否必填 | 说明 |
|----|------|--------|--------------------------------|------|
| `egress.image` | string | — | 是 | Egress 侧车镜像（OCI 引用）。 |
| `egress.mode` | `dns` \| `dns+nft` | `dns` | 否 | `OPENSANDBOX_EGRESS_MODE`。CIDR/IP 类规则需 `dns+nft`；`dns` 仅面向域名类策略。 |

### Docker 配置

| 键 | 类型 | 默认值 | 描述 |
|----|------|--------|------|
| `docker.network_mode` | string | `"host"` | 网络模式（`"host"` 或 `"bridge"`）|

### Agent-sandbox 配置

| 键 | 类型 | 默认值 | 描述 |
|----|------|--------|------|
| `agent_sandbox.template_file` | string | `null` | agent-sandbox 的 Sandbox CR YAML 模板路径（仅在 `kubernetes.workload_provider = "agent-sandbox"` 时使用） |
| `agent_sandbox.shutdown_policy` | string | `"Delete"` | 过期时的关停策略（`"Delete"` 或 `"Retain"`） |
| `agent_sandbox.ingress_enabled` | boolean | `true` | 是否启用 ingress 路由 |

### 环境变量

| 变量 | 描述 |
|------|------|
| `SANDBOX_CONFIG_PATH` | 覆盖配置文件位置 |
| `DOCKER_HOST` | Docker 守护进程 URL（例如 `unix:///var/run/docker.sock`）|
| `PENDING_FAILURE_TTL` | 失败的待处理沙箱的 TTL（秒，默认：3600）|

## 实验性功能

以下为**可选**的 **🧪 实验性**能力；在 `server/example.config.toml` 与各 `example.config.*.toml` 中**默认关闭**。生产启用前请阅读 **[OSEP-0009](../oseps/0009-auto-renew-sandbox-on-ingress-access.md)** 与发版说明。

### 按访问自动续期

在观测到访问时延长沙箱 TTL（经 Lifecycle **服务端代理** 和/或 **Ingress**）。设计、数据流与调参见 **[OSEP-0009](../oseps/0009-auto-renew-sandbox-on-ingress-access.md)**。

**服务端开关**

| 目的 | 操作 |
|------|------|
| **关闭（默认）** | `~/.sandbox.toml` 中保持 `[renew_intent] enabled = false`（见 `example.config.zh.toml`）。 |
| **开启** | 设置 `[renew_intent] enabled = true`。若使用 **Ingress + Redis** 模式，在同一 `[renew_intent]` 表中设置 `redis.enabled = true` 与 `redis.dsn`（见 OSEP）。 |
| **其它配置项** | `min_interval_seconds`、`queue_key`、`consumer_concurrency` 等见 OSEP 与 `example.config.zh.toml` 的 `[renew_intent]`。 |

**按沙箱接入**

**创建**沙箱时在 `extensions` 中设置 `access.renew.extend.seconds`，值为 **300～86400** 的**字符串**整数（秒）。不设该键（或未开 renew_intent）则该沙箱不按访问续期。

**客户端（SDK / HTTP）**

- **走 Lifecycle 服务端代理**，使请求经过 `/v1/sandboxes/{id}/proxy/{port}/...`：
  - **REST**：获取端点时加 `use_server_proxy=true`，例如 `GET /v1/sandboxes/{id}/endpoints/{port}?use_server_proxy=true`。
  - **SDK**：`ConnectionConfig(use_server_proxy=True)` 或 `ConnectionConfigSync(use_server_proxy=True)`（详见 SDK 文档中的 `use_server_proxy`）。
- **Ingress / 网关** 模式：按 OSEP 部署网关与路由，客户端按网关方式访问即可。

**延伸阅读**：[OSEP-0009](../oseps/0009-auto-renew-sandbox-on-ingress-access.md)；配置样例见 `server/example.config.zh.toml` → `[renew_intent]`。

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
uv run pytest --cov=opensandbox_server --cov-report=html
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
