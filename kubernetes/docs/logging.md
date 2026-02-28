# 日志配置说明

## 功能特性

OpenSandbox Kubernetes Controller 支持灵活的日志配置，包括：

- ✅ **日志输出到控制台**（默认启用）
- ✅ **日志输出到文件**（可选）
- ✅ **自动日志轮转**（按文件大小）
- ✅ **自动压缩旧日志**（gzip）
- ✅ **自动清理过期日志**（按时间或数量）
- ✅ **支持 zap 所有标准选项**（日志级别、格式等）

## 命令行参数

### 日志文件相关参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--enable-file-log` | bool | false | 是否启用日志输出到文件 |
| `--log-file-path` | string | `/var/log/sandbox-controller/controller.log` | 日志文件路径 |
| `--log-max-size` | int | 100 | 日志文件最大大小（MB），超过后自动轮转 |
| `--log-max-backups` | int | 10 | 保留的旧日志文件最大数量 |
| `--log-max-age` | int | 30 | 保留旧日志文件的最大天数 |
| `--log-compress` | bool | true | 是否压缩轮转后的日志文件（gzip） |

### zap 标准参数（继承自 controller-runtime）

| 参数 | 说明 |
|------|------|
| `--zap-devel` | 启用开发模式（彩色输出、更详细的堆栈跟踪） |
| `--zap-encoder` | 日志编码格式：json 或 console |
| `--zap-log-level` | 日志级别：debug, info, error 等 |
| `--zap-stacktrace-level` | 打印堆栈跟踪的最低级别 |
| `--zap-time-encoding` | 时间编码格式：iso8601, millis, nano 等 |

## 使用示例

### 1. 仅输出到控制台（默认）

```bash
./controller
```

### 2. 同时输出到控制台和文件

```bash
./controller \
  --enable-file-log=true \
  --log-file-path=/var/log/sandbox-controller/controller.log
```

### 3. 自定义日志轮转配置

```bash
./controller \
  --enable-file-log=true \
  --log-file-path=/var/log/sandbox-controller/controller.log \
  --log-max-size=50 \
  --log-max-backups=5 \
  --log-max-age=7 \
  --log-compress=true
```

这将：
- 每个日志文件最大 50MB
- 最多保留 5 个旧日志文件
- 日志文件最多保留 7 天
- 压缩旧日志文件

### 4. 开发模式 + 文件输出

```bash
./controller \
  --zap-devel=true \
  --enable-file-log=true \
  --log-file-path=/tmp/controller-dev.log
```

### 5. JSON 格式 + 文件输出

```bash
./controller \
  --zap-encoder=json \
  --enable-file-log=true \
  --log-file-path=/var/log/sandbox-controller/controller.log
```

### 6. 调试级别 + 文件输出

```bash
./controller \
  --zap-log-level=debug \
  --enable-file-log=true \
  --log-file-path=/var/log/sandbox-controller/debug.log
```

## Kubernetes 部署配置

在 Kubernetes 中部署时，可以通过 Deployment 的 `args` 配置日志选项：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sandbox-controller
spec:
  template:
    spec:
      containers:
      - name: controller
        image: sandbox-controller:latest
        args:
        - --enable-file-log=true
        - --log-file-path=/var/log/controller/controller.log
        - --log-max-size=100
        - --log-max-backups=10
        - --log-max-age=30
        - --log-compress=true
        - --zap-encoder=json
        volumeMounts:
        - name: log-volume
          mountPath: /var/log/controller
      volumes:
      - name: log-volume
        emptyDir: {}
        # 或使用 PersistentVolumeClaim
        # persistentVolumeClaim:
        #   claimName: controller-logs
```

## 日志文件格式

### 开发模式（--zap-devel=true）

```
2026-02-12T10:30:45.123+0800	INFO	setup	starting manager
2026-02-12T10:30:45.456+0800	INFO	controller	Reconciling	{"namespace": "default", "name": "example"}
```

### 生产模式（JSON）

```json
{"level":"info","ts":"2026-02-12T10:30:45.123+0800","logger":"setup","msg":"starting manager"}
{"level":"info","ts":"2026-02-12T10:30:45.456+0800","logger":"controller","msg":"Reconciling","namespace":"default","name":"example"}
```

## 日志轮转机制

日志轮转由 [lumberjack](https://github.com/natefinch/lumberjack) 实现，支持：

1. **按大小轮转**：当日志文件达到 `--log-max-size` 指定的大小时，自动创建新文件
2. **文件命名**：轮转后的文件名格式为 `controller.log.2026-02-12T10-30-45.123`
3. **自动压缩**：如果启用 `--log-compress`，旧日志文件会被压缩为 `.gz` 格式
4. **自动清理**：
   - 根据 `--log-max-backups` 保留最新的 N 个文件
   - 根据 `--log-max-age` 删除超过指定天数的文件

## 目录权限

确保日志目录存在且有写入权限：

```bash
# 创建日志目录
mkdir -p /var/log/sandbox-controller

# 设置权限（根据实际运行用户调整）
chown controller:controller /var/log/sandbox-controller
chmod 755 /var/log/sandbox-controller
```

在 Kubernetes 中，可以使用 `initContainer` 或 `securityContext` 确保权限正确：

```yaml
spec:
  initContainers:
  - name: setup-log-dir
    image: busybox
    command: ['sh', '-c', 'mkdir -p /var/log/controller && chmod 755 /var/log/controller']
    volumeMounts:
    - name: log-volume
      mountPath: /var/log/controller
  containers:
  - name: controller
    securityContext:
      runAsUser: 1000
      runAsGroup: 1000
```

## 监控和查看日志

### 查看当前日志

```bash
tail -f /var/log/sandbox-controller/controller.log
```

### 查看压缩的日志

```bash
zcat /var/log/sandbox-controller/controller.log.2026-02-12T10-30-45.123.gz | less
```

### 搜索日志

```bash
# 搜索错误日志
grep -i error /var/log/sandbox-controller/controller.log

# 在所有日志文件中搜索（包括压缩文件）
zgrep -i error /var/log/sandbox-controller/*.log*
```

## 最佳实践

1. **生产环境建议**：
   ```bash
   --enable-file-log=true
   --log-file-path=/var/log/sandbox-controller/controller.log
   --log-max-size=100
   --log-max-backups=10
   --log-max-age=30
   --log-compress=true
   --zap-encoder=json
   ```

2. **开发环境建议**：
   ```bash
   --zap-devel=true
   --enable-file-log=true
   --log-file-path=/tmp/controller-dev.log
   --log-compress=false
   ```

3. **调试问题时**：
   ```bash
   --zap-log-level=debug
   --enable-file-log=true
   --log-max-size=500
   --log-compress=false
   ```

4. **磁盘空间有限时**：
   ```bash
   --enable-file-log=true
   --log-max-size=50
   --log-max-backups=3
   --log-max-age=7
   --log-compress=true
   ```

## 故障排查

### 日志文件未创建

1. 检查目录是否存在：`ls -la /var/log/sandbox-controller/`
2. 检查权限：`ls -ld /var/log/sandbox-controller/`
3. 检查进程是否有写入权限
4. 查看 controller 启动日志中是否有错误

### 日志文件不轮转

1. 确认 `--enable-file-log=true` 已设置
2. 检查文件大小是否达到 `--log-max-size` 限制
3. 确认 lumberjack 库已正确安装：`go list -m gopkg.in/natefinch/lumberjack.v2`

### 磁盘空间占用过大

1. 减小 `--log-max-size` 的值
2. 减少 `--log-max-backups` 的数量
3. 减小 `--log-max-age` 的天数
4. 确保 `--log-compress=true` 已启用
