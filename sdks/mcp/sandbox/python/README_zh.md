# OpenSandbox MCP 沙箱服务（Python）

## 1. 简介

OpenSandbox MCP Server 将 OpenSandbox Python SDK 以 MCP 工具形式暴露给
Claude Code、Cursor 等客户端，提供沙箱生命周期、命令执行与文件操作能力。

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

### Server

- `server_healthcheck`: 服务健康检查
- `server_sdk_version`: OpenSandbox SDK 版本

### Sandbox 生命周期

- `sandbox_create`: 创建沙箱并注册到本地会话
- `sandbox_connect`: 连接已有沙箱并注册到本地会话
- `sandbox_resume`: 恢复暂停沙箱并注册到本地会话
- `sandbox_pause`: 暂停沙箱
- `sandbox_kill`: 终止沙箱
- `sandbox_close`: 释放本地资源（不终止远端沙箱）
- `sandbox_get_info`: 获取沙箱信息
- `sandbox_list`: 使用 `filter` 列出沙箱
- `sandbox_renew`: 续期
- `sandbox_healthcheck`: 沙箱健康检查
- `sandbox_get_metrics`: 资源指标
- `sandbox_get_endpoint`: 获取指定端口的访问地址

### 命令执行

- `command_run`: 在沙箱内执行命令
- `command_run_stream`: 执行命令并流式输出日志
- `command_interrupt`: 中断命令

### 文件系统

- `file_read_text`: 读取文本文件
- `file_read_bytes`: 读取二进制文件（base64）
- `file_write_text`: 写文本文件
- `file_write_bytes`: 写二进制文件（base64）
- `file_write_files`: 批量写文件
- `file_delete`: 删除文件
- `file_get_info`: 获取文件元信息
- `file_search`: 按 glob 搜索
- `file_create_directories`: 创建目录
- `file_delete_directories`: 删除目录
- `file_move`: 移动/重命名
- `file_set_permissions`: 设置权限/所有者
- `file_replace_contents`: 替换文件内容

## 5. 使用案例

下面是一些你可以让 LLM 完成的指令示例：

- "创建一个 Python 沙箱，抓取一个简单网页并总结重点。"
- "下载一个 GitHub 仓库，安装依赖并运行测试。"
- "生成一份销售数据 CSV，并快速做个统计/可视化。"
- "启动一个 8000 端口的 Web 服务并返回公网链接。"
- "搭一个最小 REST API（hello + health）并对外暴露。"
- "搜索 /app 里的 TODO 并返回文件列表。"
- "批量缩放 /data 的图片输出到 /out。"
- "运行一段 Python 脚本打印前 20 个质数。"
- "把 /app 打包成 tar.gz 并报告文件大小。"
- "清理本次会话创建的所有沙箱。"
