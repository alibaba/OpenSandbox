# Code Interpreter C# 示例

[English](README.md) | 中文

使用 OpenSandbox Code Interpreter C# SDK 运行多语言代码的完整演示。

## 前置条件

- .NET 8.0 SDK 或更高版本
- Docker（本地运行必需）
- OpenSandbox 服务器运行中

## 获取 Code Interpreter 镜像

从镜像仓库拉取预构建镜像：

```shell
docker pull opensandbox/code-interpreter:v1.0.1

# 或使用阿里云镜像仓库（中国）
# docker pull sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.1
```

## 启动 OpenSandbox 服务器

启动本地 OpenSandbox 服务器：

```shell
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/server
cp example.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main
```

## 运行示例

```shell
cd examples/code-interpreter-csharp

# 使用默认设置运行（localhost:8080）
dotnet run

# 或使用自定义环境变量
SANDBOX_DOMAIN=localhost:8080 SANDBOX_IMAGE=opensandbox/code-interpreter:v1.0.1 dotnet run
```

## 环境变量

| 变量 | 描述 | 默认值 |
|------|------|--------|
| `SANDBOX_DOMAIN` | 沙箱服务地址 | `localhost:8080` |
| `SANDBOX_API_KEY` | 身份验证 API 密钥 | (无) |
| `SANDBOX_IMAGE` | 使用的 Docker 镜像 | `opensandbox/code-interpreter:v1.0.1` |

## 本示例演示的功能

1. **多语言代码执行**：Python、Java、Go、TypeScript
2. **上下文管理**：创建、使用、列出和删除执行上下文
3. **状态持久化**：变量在上下文内的多次运行之间持久化
4. **流式输出**：使用事件处理器实时处理 stdout/stderr
5. **文件操作**：在沙箱中写入和读取文件
6. **Shell 命令**：通过 Commands 服务执行 shell 命令

## 示例输出

```text
=== OpenSandbox Code Interpreter C# Example ===

Domain: localhost:8080
Image: opensandbox/code-interpreter:v1.0.1

Creating sandbox...
Sandbox created: sbx-abc123

=== Python Example ===
[Python stdout] Hello from Python!
[Python result] {'py': '3.11.0', 'sum': 4}

=== Java Example ===
[Java stdout] Hello from Java!
[Java stdout] 2 + 3 = 5
[Java result] 5

=== Go Example ===
[Go stdout] Hello from Go!
[Go stdout] 3 + 4 = 7

=== TypeScript Example ===
[TypeScript stdout] Hello from TypeScript!
[TypeScript stdout] sum = 6

=== Context Management Example ===
Created context: ctx-xyz789
Set x = 42 in context
[Context stdout] x = 42
[Context result] 84
Total contexts: 1
Deleted context: ctx-xyz789

=== Streaming Example ===
[Stream] Count: 0
[Stream] Count: 1
[Stream] Count: 2
[Stream] Count: 3
[Stream] Count: 4
[Stream] Done!
[Stream] Completed in 523ms

=== File Operations Example ===
Wrote /tmp/hello.txt
Read content: Hello from C#!
[Shell] Hello from C#! - via shell

=== Cleanup ===
Sandbox terminated.

=== Example completed successfully! ===
```

## 代码结构

```csharp
// 1. 创建连接配置
var config = new ConnectionConfig(new ConnectionConfigOptions
{
    Domain = "localhost:8080",
    ApiKey = "your-api-key"
});

// 2. 创建带有 code interpreter 镜像的沙箱
await using var sandbox = await Sandbox.CreateAsync(new SandboxCreateOptions
{
    ConnectionConfig = config,
    Image = "opensandbox/code-interpreter:v1.0.1",
    Entrypoint = new[] { "/opt/opensandbox/code-interpreter.sh" }
});

// 3. 创建代码解释器
var interpreter = await CodeInterpreter.CreateAsync(sandbox);

// 4. 运行各种语言的代码
var result = await interpreter.Codes.RunAsync(
    "print('Hello!')",
    new RunCodeOptions { Language = SupportedLanguage.Python });

// 5. 访问结果
foreach (var msg in result.Logs.Stdout)
{
    Console.WriteLine(msg.Text);
}
```

## 相关文档

- [OpenSandbox C# SDK](../../sdks/sandbox/csharp/README_zh.md)
- [Code Interpreter C# SDK](../../sdks/code-interpreter/csharp/README_zh.md)
- [Code Interpreter Python 示例](../code-interpreter/README.md)
