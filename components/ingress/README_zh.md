# OpenSandbox Ingress

## 功能概览
- 基于 Kubernetes BatchSandbox CR 的 HTTP / WebSocket 反向代理，按 `OPEN-SANDBOX-INGRESS` 或 Host 解析目标沙箱。
- 自动监听目标 Namespace 内的 BatchSandbox 资源，从 annotation 中获取端点 IP。
- 提供 `/status.ok` 健康探针，启动时打印编译版本、时间、提交、Go/平台信息。

## 启动与参数
```bash
go run main.go \
  --namespace <目标命名空间> \
  --port 28888 \
  --log-level info
```
- `--namespace`：监听的 Kubernetes 命名空间。
- `--port`：监听端口（默认 28888）。
- `--log-level`：日志级别，遵循 zap 定义。

入口：`/` 走代理，`/status.ok` 健康检查。

## 构建与发布
### 本地二进制
```bash
cd components/ingress
make build
# 可覆盖版本信息
VERSION=1.2.3 GIT_COMMIT=$(git rev-parse HEAD) BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") make build
```

### Docker 镜像
Dockerfile 支持编译期注入：
```bash
docker build \
  --build-arg VERSION=$(git describe --tags --always --dirty) \
  --build-arg GIT_COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t opensandbox/ingress:local .
```

### 多架构推送脚本
`build.sh` 使用 buildx 构建并推送 amd64/arm64，多标签支持，传入同名环境变量即可覆盖：
```bash
cd components/ingress
TAG=local VERSION=1.2.3 GIT_COMMIT=abc BUILD_TIME=2025-01-01T00:00:00Z bash build.sh
```

## 运行时依赖
- 可访问的 Kubernetes API（集群内或 KUBECONFIG）。
- 目标命名空间中的 BatchSandbox CR 必须包含 `sandbox.opensandbox.io/endpoints` annotation。

## 开发与测试
```bash
cd components/ingress
go test ./...
```
主要代码位置：
- `main.go`：入口与 HTTP 路由注册。
- `pkg/proxy/`：HTTP/WebSocket 代理逻辑、沙箱端点解析。
- `pkg/sandbox/`：沙箱 Provider 抽象和 BatchSandbox 实现。
- `version/`：版本信息输出（ldflags 注入）。

## 常见行为说明
- Header 优先：`OPEN-SANDBOX-INGRESS`，否则回退 Host 解析 `<sandbox-name>-<port>.*`。
- 从请求中提取沙箱名称，查询 BatchSandbox CR 获取端点 IP。
- 错误处理：
  - `ErrSandboxNotFound`（沙箱资源不存在）→ HTTP 404
  - `ErrSandboxNotReady`（副本数不足、缺少端点、配置无效）→ HTTP 503
  - 其他错误（K8s API 错误等）→ HTTP 502
- WebSocket 保留关键头并透传 X-Forwarded-*，HTTP 会移除 `OPEN-SANDBOX-INGRESS` 后再转发。

