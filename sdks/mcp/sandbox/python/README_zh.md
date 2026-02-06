# OpenSandbox MCP 沙箱服务（Python）

## 1. 简介

OpenSandbox MCP Server 将 OpenSandbox Python SDK 以 MCP 工具形式暴露给
Claude Code、Cursor 等客户端，提供精简的沙箱生命周期、命令执行与文本文件操作能力。

## 2. 安装和启动

### 源码方式（本地开发）

```bash
uv sync
uv run opensandbox-mcp
```

### 下载包方式

```bash
pip install opensandbox-mcp
opensandbox-mcp
```

### 配置

环境变量：

- `OPEN_SANDBOX_API_KEY`
- `OPEN_SANDBOX_DOMAIN`

CLI 覆盖：

```bash
opensandbox-mcp --api-key ... --domain ... --protocol https
```

配置项说明：

- `api_key`：OpenSandbox API Key（鉴权）。
- `domain`：OpenSandbox API 域名（如 `api.opensandbox.io`）。
- `protocol`：`http` 或 `https`。
- `request_timeout_seconds`：HTTP 请求超时（秒）。
- `transport`：`stdio`（默认）或 `streamable-http`。

### Streamable HTTP

```bash
opensandbox-mcp \
  --transport streamable-http
```

## 3. 集成案例

### Claude Code stdio

```bash
claude mcp add opensandbox-sandbox --transport stdio -- \
  opensandbox-mcp --api-key "$OPEN_SANDBOX_API_KEY" --domain "$OPEN_SANDBOX_DOMAIN"
```

### Claude Code http

```bash
claude mcp add opensandbox-sandbox --transport http http://localhost:8000/mcp
```

### Cursor stdio

```json
{
  "mcpServers": {
    "opensandbox-sandbox": {
      "command": "opensandbox-mcp",
      "args": [
        "--api-key",
        "${OPEN_SANDBOX_API_KEY}",
        "--domain",
        "${OPEN_SANDBOX_DOMAIN}"
      ]
    }
  }
}
```

### Cursor http

```json
{
  "mcpServers": {
    "opensandbox-sandbox": {
      "url": "http://localhost:8000/mcp"
    }
  }
}
```

## 4. 工具描述

说明：

- 所有工具均使用 `sandbox_create` / `sandbox_connect` 返回的 `sandbox_id`。
- `file_read` / `file_write` 仅支持文本文件；大文件可用 `encoding` 和 `range_header`。

### Sandbox 生命周期

- `sandbox_create`: 创建沙箱并注册到本地会话
- `sandbox_connect`: 连接已有沙箱并注册到本地会话
- `sandbox_get_info`: 获取沙箱信息
- `sandbox_list`: 使用 `filter` 列出沙箱
- `sandbox_renew`: 续期
- `sandbox_get_metrics`: 资源指标
- `sandbox_healthcheck`: 沙箱健康检查
- `sandbox_kill`: 终止沙箱
- `sandbox_get_endpoint`: 获取指定端口的访问地址

### 命令执行

- `command_run`: 在沙箱内执行命令
- `command_interrupt`: 中断命令

### 文件系统

- `file_read`: 读取文本文件
- `file_write`: 写文本文件
- `file_delete`: 删除文件
- `file_search`: 按 glob 搜索
- `file_create_directories`: 创建目录
- `file_delete_directories`: 删除目录
- `file_move`: 移动/重命名
- `file_replace_contents`: 替换文件内容

## 5. 最小流程

1. `sandbox_create` -> 记录 `sandbox_id`。
2. `file_write` 写入代码或资源。
3. `command_run` 执行、安装依赖或启动服务。
4. 对外暴露端口时使用 `sandbox_get_endpoint`。
5. 完成后 `sandbox_kill`。

## 6. 使用案例

下面是一些你可以让 LLM 完成的指令示例：

- "创建一个 Python 沙箱并执行健康检查命令。"
- "把一段 Python 脚本写入沙箱并执行。"
- "下载一个 GitHub 仓库，安装依赖并运行测试。"
- "生成一份销售数据 CSV，并运行简单统计脚本。"
- "启动一个 8000 端口的 Web 服务并返回公网链接。"
- "搭一个最小 REST API（hello + health）并对外暴露。"
- "把 /app 打包成 tar.gz 并报告文件大小。"
- "实现一个贪吃蛇小游戏，并且返回可访问的web链接"
