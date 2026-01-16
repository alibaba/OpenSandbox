# Alibaba Sandbox SDK for Kotlin

中文 | [English](README.md)

用于与 OpenSandbox 进行底层交互的 Kotlin SDK。它提供了创建、管理和与安全沙箱环境交互的能力，包括执行 Shell 命令、管理文件和监控资源。

## 安装指南

### Gradle (Kotlin DSL)

```kotlin
dependencies {
    implementation("com.alibaba.opensandbox:sandbox:{latest_version}")
}
```

### Maven

```xml
<dependency>
    <groupId>com.alibaba.opensandbox</groupId>
    <artifactId>sandbox</artifactId>
    <version>{latest_version}</version>
</dependency>
```

## 快速开始

以下示例展示了如何创建一个沙箱并执行 Shell 命令。

> **注意**: 在运行此示例之前，请确保 OpenSandbox 服务已启动。服务启动请参考根目录的 [README_zh.md](../../../docs/README_zh.md)。

```java
import com.alibaba.opensandbox.sandbox.Sandbox;
import com.alibaba.opensandbox.sandbox.config.ConnectionConfig;
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxException;
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.Execution;
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.RunCommandRequest;

public class QuickStart {
    public static void main(String[] args) {
        // 1. 配置连接信息
        ConnectionConfig config = ConnectionConfig.builder()
            .domain("api.opensandbox.io")
            .apiKey("your-api-key")
            .build();

        // 2. 使用 try-with-resources 创建 Sandbox
        try (Sandbox sandbox = Sandbox.builder()
                .connectionConfig(config)
                .image("ubuntu")
                .build()) {

            // 3. 执行 Shell 命令
            Execution execution = sandbox
                    .commands()
                    .run("echo 'Hello Sandbox!'");

            // 4. 打印输出
            System.out.println(execution.getLogs().getStdout().get(0).getText());

            // 5. 清理资源 (自动调用 sandbox.close())
            // 注意: 如果希望立即终止远程沙箱实例，仍需显式调用 kill()
            sandbox.kill();
        } catch (SandboxException e) {
            // 处理 Sandbox 特定异常
            System.err.println("沙箱错误: [" + e.getError().getCode() + "] " + e.getError().getMessage());
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}
```

## 核心功能示例

### 1. 生命周期管理

管理沙箱的生命周期，包括续期、暂停、恢复和状态查询。

```java
// 续期沙箱
// 此操作将沙箱的过期时间重置为 (当前时间 + duration)
sandbox.renew(Duration.ofMinutes(30));

// 暂停执行 (挂起所有进程)
sandbox.pause();

// 恢复执行
sandbox.resume();

// 获取当前状态
SandboxInfo info = sandbox.getInfo();
System.out.println("当前状态: " + info.getStatus().getState());
```

### 2. 自定义健康检查

定义自定义逻辑来判断沙箱是否健康。这可以覆盖默认的 Ping 检查。

```java
Sandbox sandbox = Sandbox.builder()
    .connectionConfig(config)
    .image("nginx:latest")
    // 自定义检查：等待 80 端口可访问
    .healthCheck(sb -> {
        try {
            // 1. 获取沙箱 80 端口映射的外部访问地址
            SandboxEndpoint endpoint = sb.getEndpoint(80);

            // 2. 执行你的连接检查逻辑 (例如 HTTP 请求, Socket 连接等)
            // return checkConnection(endpoint.getEndpoint());
            return true;
        } catch (Exception e) {
            return false;
        }
    })
    .build();
```

### 3. 命令执行与流式响应

执行命令并实时处理输出流。

```java
// 创建流式输出处理器
ExecutionHandlers handlers = ExecutionHandlers.builder()
    .onStdout(msg -> System.out.println("STDOUT: " + msg.getText()))
    .onStderr(msg -> System.err.println("STDERR: " + msg.getText()))
    .onExecutionComplete(complete ->
        System.out.println("命令执行耗时: " + complete.getExecutionTimeInMillis() + "ms")
    )
    .build();

// 带处理器的命令执行
RunCommandRequest request = RunCommandRequest.builder()
    .command("for i in {1..5}; do echo \"Count $i\"; sleep 0.5; done")
    .handlers(handlers)
    .build();

sandbox.commands().run(request);
```

### 4. 全面的文件操作

管理文件和目录，包括读写、列表、删除和搜索。

```java
// 1. 写入文件
sandbox.files().write(List.of(
    WriteEntry.builder()
        .path("/tmp/hello.txt")
        .data("Hello World")
        .mode(644)
        .build()
));

// 2. 读取文件
String content = sandbox.files().readFile("/tmp/hello.txt", "UTF-8", null);
System.out.println("文件内容: " + content);

// 3. 搜索/列表文件
List<EntryInfo> files = sandbox.files().search(
    SearchEntry.builder()
        .path("/tmp")
        .pattern("*.txt")
        .build()
);
files.forEach(f -> System.out.println("找到文件: " + f.getPath()));

// 4. 删除文件
sandbox.files().deleteFiles(List.of("/tmp/hello.txt"));
```

### 5. 沙箱管理 (Sandbox Manager)

使用 `SandboxManager` 进行管理操作，如查询现有沙箱列表。

```java
SandboxManager manager = SandboxManager.builder()
    .connectionConfig(config)
    .build();

import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxState;

// ...

// 列出运行中的沙箱
PagedSandboxInfos sandboxes = manager.listSandboxInfos(
    SandboxFilter.builder()
        .states(SandboxState.RUNNING)
        .pageSize(10)
        .page(1)
        .build()
);

sandboxes.getSandboxInfos().forEach(info -> {
    System.out.println("Found sandbox: " + info.getId());
    // 执行管理操作
    manager.killSandbox(info.getId());
});

// Try-with-resources 会自动调用 manager.close()
// manager.close();
```

## 配置说明

### 1. 连接配置 (Connection Configuration)

`ConnectionConfig` 类管理与 API 服务器的连接设置。

| 参数             | 描述                         | 默认值                   | 环境变量               |
| ---------------- | ---------------------------- | ------------------------ | ---------------------- |
| `apiKey`         | 用于认证的 API Key           | 必填                     | `OPEN_SANDBOX_API_KEY` |
| `domain`         | 沙箱服务的端点域名           | 必填 (或 localhost:8080) | `OPEN_SANDBOX_DOMAIN`  |
| `protocol`       | HTTP 协议 (http/https)       | `http`                   | -                      |
| `requestTimeout` | API 请求超时时间             | 30 秒                    | -                      |
| `debug`          | 是否开启 HTTP 请求的调试日志 | `false`                  | -                      |
| `headers`        | 自定义 HTTP 请求头           | 空                       | -                      |
| `connectionPool` | 共享 OKHttp 连接池           | SDK 每实例创建            | -                      |

```java
// 1. 基础配置
ConnectionConfig config = ConnectionConfig.builder()
    .apiKey("your-key")
    .domain("api.opensandbox.io")
    .requestTimeout(Duration.ofSeconds(60))
    .build();

// 2. 进阶配置：共享连接池 (Shared Connection Pool)
// 如果你需要创建大量 Sandbox 实例，建议共享连接池以节省资源。
// SDK 默认连接保活时间为 30 秒。
ConnectionPool sharedPool = new ConnectionPool(50, 30, TimeUnit.SECONDS);

ConnectionConfig sharedConfig = ConnectionConfig.builder()
    .apiKey("your-key")
    .domain("api.opensandbox.io")
    .connectionPool(sharedPool) // 注入共享连接池
    .build();
```

### 2. 沙箱创建配置 (Sandbox Creation Configuration)

`Sandbox.builder()` 用于配置沙箱环境。

| 参数           | 描述                   | 默认值                          |
| -------------- | ---------------------- | ------------------------------- |
| `image`        | 使用的 Docker 镜像     | 必填                            |
| `timeout`      | 自动终止的超时时间     | 10 分钟                         |
| `entrypoint`   | 容器启动入口命令       | `["tail", "-f", "/dev/null"]`   |
| `resource`     | CPU 和内存限制         | `{"cpu": "1", "memory": "2Gi"}` |
| `env`          | 环境变量               | 空                              |
| `metadata`     | 自定义元数据标签       | 空                              |
| `readyTimeout` | 等待沙箱就绪的最大时间 | 30 秒                           |

```java
Sandbox sandbox = Sandbox.builder()
    .connectionConfig(config)
    .image("python:3.11")
    .timeout(Duration.ofMinutes(30))
    .resource(map -> {
        map.put("cpu", "2");
        map.put("memory", "4Gi");
    })
    .env("PYTHONPATH", "/app")
    .metadata("project", "demo")
    .build();
```
