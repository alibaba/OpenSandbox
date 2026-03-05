# Docker OSSFS 挂载示例

本示例演示如何使用新版 SDK 的 `ossfs` volume 模型，在 Docker 运行时将阿里云 OSS 挂载到沙箱容器。

## 覆盖场景

1. **基础读写挂载**（OSSFS backend）。
2. **跨沙箱共享数据**（同一 OSSFS backend path）。
3. **同一 backend path + 不同 `subPath` 挂载**。

## 前置条件

### 1) 启动 OpenSandbox 服务（Docker runtime）

请确保服务端主机满足：

- 已安装 `ossfs`
- 已启用 FUSE
- 已配置可写的 `storage.ossfs_mount_root`（默认 `/mnt/ossfs`）

示例配置：

```toml
[runtime]
type = "docker"

[storage]
ossfs_mount_root = "/mnt/ossfs"
```

启动服务：

```bash
opensandbox-server
```

### 2) 安装 Python SDK

```bash
uv pip install opensandbox
```

如果当前 PyPI 版本还不包含 OSSFS 相关模型，可从源码安装：

```bash
pip install -e sdks/sandbox/python
```

### 3) 配置 OSS 参数

```bash
export SANDBOX_DOMAIN=localhost:8080
export SANDBOX_API_KEY=your-api-key
export SANDBOX_IMAGE=ubuntu

export OSS_BUCKET=your-bucket
export OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com
export OSS_PATH=/               # 可选，默认 "/"
export OSS_ACCESS_KEY_ID=your-ak
export OSS_ACCESS_KEY_SECRET=your-sk
# export OSS_SECURITY_TOKEN=... # 可选，STS 场景
```

## 运行

```bash
uv run python examples/docker-ossfs-volume-mount/main.py
```

## SDK 最小示例

```python
from opensandbox import Sandbox
from opensandbox.models.sandboxes import OSSFS, Volume

sandbox = await Sandbox.create(
    image="ubuntu",
    volumes=[
        Volume(
            name="oss-data",
            ossfs=OSSFS(
                bucket="your-bucket",
                endpoint="oss-cn-hangzhou.aliyuncs.com",
                path="/datasets",
                accessKeyId="your-ak",
                accessKeySecret="your-sk",
            ),
            mountPath="/mnt/data",
            subPath="train",      # 可选
            readOnly=False,       # 可选
        )
    ],
)
```

## 说明

- 当前实现使用**内联凭据**（`accessKeyId` / `accessKeySecret`）。
- Docker 运行时采用**按需挂载**（mount-or-reuse），不是预挂载所有 bucket。

## 参考

- [OSEP-0003: Volume 与 VolumeBinding 支持](../../oseps/0003-volume-and-volumebinding-support.md)
- [Sandbox Lifecycle API 规范](../../specs/sandbox-lifecycle.yml)
