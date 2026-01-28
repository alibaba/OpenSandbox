# execd - OpenSandbox 执行守护进程

中文 | [English](README.md)

`execd` 是 OpenSandbox 的执行守护进程，基于 Beego 框架提供全面的 HTTP API。它将外部请求转化为实际的运行时动作：管理 Jupyter
会话、以 SSE（Server-Sent Events）流式返回代码输出、执行 shell 命令、操作沙箱文件系统，并采集主机侧指标。

## 目录

- [概述](#概述)
- [核心特性](#核心特性)
- [架构设计](#架构设计)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [API 参考](#api-参考)
- [支持的语言](#支持的语言)
- [开发指南](#开发指南)
- [测试](#测试)
- [可观测性](#可观测性)
- [许可证](#许可证)

## 概述

`execd` 作为 OpenSandbox 的运行时守护进程，提供统一的接口用于：

- **代码执行**：Python、Java、JavaScript、TypeScript、Go 和 Bash
- **会话管理**：带状态保持的长连接 Jupyter kernel 会话
- **命令执行**：同步执行和异步执行 shell 命令
- **文件操作**：完整的文件系统 CRUD，支持分块上传/下载
- **监控**：实时系统指标（CPU、内存、运行时间）

## 核心特性

### 统一运行时管理

- 将 REST 调用转化为由 `pkg/runtime` 控制器处理的运行时请求
- 支持多种执行后端：Jupyter、Shell、等等
- 自动语言检测和路由
- 可插拔 Jupyter server 配置

### Jupyter 集成

- 通过 `pkg/jupyter` 维护 kernel 会话
- 基于 WebSocket 的实时通信
- 通过 Server-Sent Events (SSE) 流式推送执行事件

### 命令执行器

- 前台、后台 shell 命令
- 通过进程组管理正确转发信号
- 实时 stdout/stderr 流式输出
- 支持上下文感知的中断

### 文件系统

- 围绕沙箱文件系统的 CRUD 辅助工具
- Glob 模式匹配文件搜索
- 支持断点续传的分块上传/下载
- 权限管理

### 可观测性

- 轻量级指标端点（CPU、内存、运行时间）
- 结构化流式日志
- 基于 SSE 的实时监控

## 架构设计

### 目录结构

| 路径                     | 说明                                         |
|------------------------|--------------------------------------------|
| `main.go`              | 程序入口，初始化 Beego、CLI 标志和路由                   |
| `pkg/flag/`            | 命令行与环境变量配置                                 |
| `pkg/web/`             | HTTP 层（控制器、模型、路由、SSE 辅助）                   |
| `pkg/web/controller/`  | 文件、代码、命令、指标的请求处理器                          |
| `pkg/web/model/`       | 请求/响应模型与 SSE 事件类型                          |
| `pkg/runtime/`         | 运行时控制器，调度到 Jupyter、Shell执行器                |
| `pkg/jupyter/`         | 精简 Jupyter 客户端（kernels/sessions/WebSocket） |
| `pkg/jupyter/execute/` | 执行结果类型与流解析器                                |
| `pkg/jupyter/session/` | 会话管理与生命周期                                  |
| `pkg/util/`            | 通用工具（安全 goroutine、glob 辅助）                 |
| `tests/`               | 测试脚本和工具                                    |

## 快速开始

### 环境要求

- **Go 1.24+**（在 `go.mod` 中定义）
- **Jupyter Server**（代码执行上下文所需）
- **Docker**（可选，用于容器化构建）
- **Make**（可选，用于便捷命令）

### 快速启动

#### 1. 克隆并构建

```bash
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/components/execd
go mod download
make build
```

#### 2. 启动 Jupyter Server

```bash
# 方式 1：使用提供的脚本
./tests/jupyter.sh

# 方式 2：手动启动
jupyter notebook --port=54321 --no-browser --ip=0.0.0.0 \
  --NotebookApp.token='your-jupyter-token'
```

#### 3. 运行 execd

```bash
./bin/execd \
  --jupyter-host=http://127.0.0.1:54321 \
  --jupyter-token=your-jupyter-token \
  --port=44772
```

#### 4. 验证安装

```bash
curl -v http://localhost:44772/ping
# 期望200状态码
```

### 镜像构建

```bash
docker build -t opensandbox/execd:dev .

# 运行容器
docker run -d \
  -p 44772:44772 \
  -e JUPYTER_HOST=http://jupyter-server \
  -e JUPYTER_TOKEN=your-token \
  --name execd \
  opensandbox/execd:dev
```

## 配置说明

### 命令行标志

| 标志                            | 类型       | 默认值     | 说明                                  |
|-------------------------------|----------|---------|-------------------------------------|
| `--jupyter-host`              | string   | `""`    | 后端 Jupyter server 地址，要求execd进程可访问即可 |
| `--jupyter-token`             | string   | `""`    | Jupyter HTTP/WebSocket 令牌           |
| `--port`                      | int      | `44772` | HTTP 监听端口                           |
| `--log-level`                 | int      | `6`     | Beego 日志级别（0=紧急，7=调试）               |
| `--access-token`              | string   | `""`    | API 共享密钥（可选）                        |
| `--graceful-shutdown-timeout` | duration | `3s`    | 关闭前等待 SSE 的时间                       |

### 环境变量

所有标志都可以通过环境变量设置：

```bash
export JUPYTER_HOST=http://127.0.0.1:8888
export JUPYTER_TOKEN=your-token
```

环境变量优先于默认值，但会被显式的 CLI 标志覆盖。

## API 参考

[API Spec](../../specs/execd-api.yaml)。

## 支持的语言

### 基于 Jupyter 的语言

| 语言         | Kernel      | 特性              |
|------------|-------------|-----------------|
| Python     | IPython     | 完整 Jupyter 协议支持 |
| Java       | IJava       | 基于 JShell 的执行   |
| JavaScript | IJavaScript | Node.js 运行时     |
| TypeScript | ITypeScript | TS 编译 + Node 执行 |
| Go         | gophernotes | Go 解释器          |
| Bash       | Bash kernel | Shell 脚本执行      |

### 原生执行器

| 模式/语言                | 后端      | 特性          |
|----------------------|---------|-------------|
| `command`            | OS exec | 同步 shell 命令 |
| `background-command` | OS exec | 分离的后台进程     |

## 开发指南

开发指南请参见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

## 测试

### 单元测试

```bash
make test
```

### 集成测试

需要真实 Jupyter Server 的集成测试默认跳过：

```bash
export JUPYTER_URL=http://localhost:8888
export JUPYTER_TOKEN=your-token
go test -v ./pkg/jupyter/...
```

### 手动测试工作流

1. 启动 Jupyter：`./tests/jupyter.sh`
2. 启动 execd：`./bin/execd --jupyter-host=http://localhost:54321 --jupyter-token=opensandboxexecdlocaltest`

3. 执行代码：

```bash
curl -X POST -H "Content-Type: application/json" \
  -d '{"language":"python","code":"print(\"test\")"}' \
  http://localhost:44772/code
```

## 配置

### SSE API 优雅结束时间窗口

- 环境变量：`EXECD_API_GRACE_SHUTDOWN`（如 `500ms`、`2s`、`1m`）
- 命令行参数：`--graceful-shutdown-timeout`
- 默认值：`1s`

作用：控制 SSE 响应（代码/命令执行）在发送最后一块数据后，保持连接的宽限时间，方便客户端完全读到尾部输出再关闭。如果设置为 `0s` 则关闭这一等待。

## 可观测性

### 日志记录

全程使用 Beego 的分级日志器：

```go
logs.Info("message") // 常规信息
logs.Warning("message") // 警告条件
logs.Error("message")   // 错误条件
logs.Debug("message") // 调试级别消息
```

- 环境变量：`EXECD_LOG_FILE` 指定日志输出文件；未设置时日志输出到标准输出（stdout）。

日志级别（0-7）：

- 0：紧急
- 1：警报
- 2：严重
- 3：错误
- 4：警告
- 5：注意
- 6：信息（默认）
- 7：调试

### 指标采集

`/metrics` 端点提供：

- CPU 使用百分比
- 内存总量/已用（GB）
- 内存使用百分比
- 进程运行时间
- 当前时间戳

对于实时监控，使用 `/metrics/watch`，每秒通过 SSE 流式推送更新。

## 性能基准

### 典型延迟（localhost）

| 操作            | 延迟       |
|---------------|----------|
| `/ping`       | < 1ms    |
| `/files/info` | < 5ms    |
| 代码执行（Python）  | 50-200ms |
| 文件上传（1MB）     | 10-50ms  |
| 指标快照          | < 10ms   |

### 资源使用（空闲）

- 内存：~50MB
- CPU：< 1%
- Goroutines：~15

### 可扩展性

- 支持 100+ 并发 SSE 连接
- 文件操作随文件大小线性扩展
- Jupyter 会话是有状态的，需要专用资源

## 贡献

1. Fork 仓库
2. 创建特性分支
3. 遵循编码规范（见 DEVELOPMENT.md）
4. 为新功能添加测试
5. 运行 `make fmt` 和 `make test`
6. 提交 pull request

## 许可证

`execd` 是 OpenSandbox 项目的一部分。详见仓库根目录的 [LICENSE](../../LICENSE)。

## 支持

- 问题：[GitHub Issues](https://github.com/alibaba/OpenSandbox/issues)
- 文档：[OpenSandbox Docs](https://github.com/alibaba/OpenSandbox/wiki)
- 社区：[Discussions](https://github.com/alibaba/OpenSandbox/discussions)
