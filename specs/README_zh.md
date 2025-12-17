# OpenSandbox API 规范文档

中文 | [English](README.md)

本目录包含 OpenSandbox 项目的 OpenAPI 规范文档，定义了完整的 API 接口和数据模型。

## 规范文件

### 1. sandbox-lifecycle.yml

**沙箱生命周期管理 API**

定义了沙箱环境的创建、管理和销毁的完整生命周期接口。

**核心功能：**
- **沙箱管理**：创建、列表、查询、删除沙箱实例
- **状态控制**：暂停 (Pause)、恢复 (Resume) 沙箱执行
- **生命周期**：支持多种状态转换（Pending → Running → Pausing → Paused → Stopping → Terminated）
- **资源配置**：CPU、内存、GPU 等资源限制配置
- **镜像支持**：从容器镜像直接创建沙箱，支持公共和私有镜像仓库
- **超时管理**：自动过期和手动续期功能
- **端点访问**：获取沙箱内服务的公共访问端点

**主要端点：**
- `POST /v1/sandboxes` - 创建沙箱
- `GET /v1/sandboxes` - 列出沙箱（支持过滤和分页）
- `GET /v1/sandboxes/{sandboxId}` - 获取沙箱详情
- `DELETE /v1/sandboxes/{sandboxId}` - 删除沙箱
- `POST /v1/sandboxes/{sandboxId}/pause` - 暂停沙箱
- `POST /v1/sandboxes/{sandboxId}/resume` - 恢复沙箱
- `POST /v1/sandboxes/{sandboxId}/renew-expiration` - 续期沙箱
- `GET /v1/sandboxes/{sandboxId}/endpoints/{port}` - 获取访问端点

**认证方式：**
- HTTP Header: `OPEN-SANDBOX-API-KEY: your-api-key`
- 环境变量: `OPEN_SANDBOX_API_KEY`（SDK 客户端）

### 2. execd-api.yaml

**沙箱内代码执行 API**

定义了在沙箱环境内执行代码、命令和文件操作的接口，提供完整的代码解释器和文件系统管理能力。

**核心功能：**
- **代码执行**：支持 Python、JavaScript 等多语言的有状态代码执行
- **命令执行**：Shell 命令执行，支持前台/后台模式
- **文件操作**：完整的文件和目录 CRUD 操作（创建、读取、更新、删除）
- **实时流式输出**：基于 SSE (Server-Sent Events) 的实时输出流
- **系统监控**：CPU 和内存指标的实时监控
- **访问控制**：基于 Token 的 API 认证

**主要端点分类：**

**健康检查：**
- `GET /ping` - 服务健康检查

**代码解释器：**
- `POST /code/context` - 创建代码执行上下文
- `POST /code` - 在上下文中执行代码（流式输出）
- `DELETE /code` - 中断代码执行

**命令执行：**
- `POST /command` - 执行 Shell 命令（流式输出）
- `DELETE /command` - 中断命令执行

**文件系统：**
- `GET /files/info` - 获取文件元数据
- `DELETE /files` - 删除文件
- `POST /files/permissions` - 修改文件权限
- `POST /files/mv` - 移动/重命名文件
- `GET /files/search` - 搜索文件（支持 glob 模式）
- `POST /files/replace` - 批量替换文件内容
- `POST /files/upload` - 上传文件
- `GET /files/download` - 下载文件（支持断点续传）

**目录操作：**
- `POST /directories` - 创建目录
- `DELETE /directories` - 删除目录

**系统指标：**
- `GET /metrics` - 获取系统资源指标
- `GET /metrics/watch` - 实时监控系统指标（SSE 流）

## 技术特性

### 流式输出 (Server-Sent Events)

代码执行和命令执行接口使用 SSE 提供实时流式输出，支持以下事件类型：
- `init` - 初始化事件
- `status` - 状态更新
- `stdout` / `stderr` - 标准输出/错误流
- `result` - 执行结果
- `execution_complete` - 执行完成
- `execution_count` - 执行计数
- `error` - 错误信息

### 资源限制

支持灵活的资源配置（类似 Kubernetes）：
```json
{
  "cpu": "500m",
  "memory": "512Mi",
  "gpu": "1"
}
```

### 文件权限

支持 Unix 风格的文件权限管理：
- 所有者 (owner)
- 用户组 (group)
- 权限模式 (mode) - 八进制格式，如 755
