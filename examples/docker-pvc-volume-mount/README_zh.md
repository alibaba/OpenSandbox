# Docker PVC（命名卷）挂载示例

本示例演示如何使用 OpenSandbox 的 `pvc` 后端将 Docker 命名卷（named volume）挂载到沙箱容器中。在 Docker 运行时下，`pvc.claimName` 映射为 Docker 命名卷 —— 相比宿主机路径绑定挂载（host path），命名卷更安全、更便于跨沙箱共享数据。

> **什么是 `pvc`？** `pvc` 后端是一个运行时无关的抽象。在 Kubernetes 中它映射为 PersistentVolumeClaim；在 Docker 中它映射为命名卷。同一个 API 请求可在两种运行时上工作。详见 [OSEP-0003](../../oseps/0003-volume-and-volumebinding-support.md) 设计文档。

## 为什么使用命名卷而非宿主机路径？

| | 宿主机路径（`host` 后端） | 命名卷（`pvc` 后端） |
|---|---|---|
| **安全性** | 暴露宿主机文件系统路径 | Docker 管理存储位置，不暴露宿主机路径 |
| **配置** | 需要 `allowed_host_paths` 白名单 | 无需白名单配置 |
| **跨沙箱共享** | 所有容器必须约定同一宿主机路径 | 引用相同的卷名即可 |
| **可移植性** | 依赖宿主机目录结构 | 在任何 Docker 主机上均可使用 |
| **生命周期** | 用户手动管理宿主机目录 | `docker volume create/rm` 管理 |

## 演示场景

| # | 场景 | 说明 |
|---|------|------|
| 1 | **读写挂载** | 挂载命名卷，支持双向文件读写 |
| 2 | **只读挂载** | 挂载命名卷，沙箱不可修改 |
| 3 | **跨沙箱共享** | 两个沙箱通过同一命名卷共享数据，无需暴露宿主机路径 |
| 4 | **SubPath 挂载** | 仅挂载命名卷的子目录（与 K8s PVC subPath 语义一致） |

## 前置条件

### 1. 启动 OpenSandbox 服务

```shell
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker
opensandbox-server
```

### 2. 创建 Docker 命名卷

```shell
# 创建命名卷
docker volume create opensandbox-pvc-demo

# 通过临时容器写入一个标记文件
docker run --rm -v opensandbox-pvc-demo:/data alpine \
  sh -c "echo 'hello-from-named-volume' > /data/marker.txt"
```

### 3. 安装 Python SDK

```shell
uv pip install opensandbox
```

### 4. 拉取沙箱镜像

```shell
docker pull registry.cn-hangzhou.aliyuncs.com/acs/ubuntu:latest
```

## 运行

```shell
SANDBOX_IMAGE=registry.cn-hangzhou.aliyuncs.com/acs/ubuntu:latest \
  uv run python examples/docker-pvc-volume-mount/main.py
```

脚本会自动创建命名卷并写入测试数据。也可以通过环境变量自定义镜像和服务地址：

```shell
SANDBOX_IMAGE=ubuntu SANDBOX_DOMAIN=localhost:8080 \
  uv run python examples/docker-pvc-volume-mount/main.py
```

## 预期输出

```text
OpenSandbox server : localhost:8080
Sandbox image      : ubuntu
Docker volume      : opensandbox-pvc-demo
  Ensuring Docker named volume 'opensandbox-pvc-demo' exists...
  Created volume 'opensandbox-pvc-demo' with marker.txt

============================================================
Scenario 1: Read-Write PVC (Named Volume) Mount
============================================================
  Volume name: opensandbox-pvc-demo
  Mount path : /mnt/data

  [1] Reading marker file from named volume:
  hello-from-named-volume

  [2] Writing a file from inside the sandbox:
  -> Written: /mnt/data/sandbox-output.txt

  [3] Reading back the written file:
  written-by-sandbox

  [4] Listing volume contents:
  ...
  -rw-r--r-- 1 root root   ... marker.txt
  -rw-r--r-- 1 root root   ... sandbox-output.txt

  Scenario 1 completed.

============================================================
Scenario 2: Read-Only PVC (Named Volume) Mount
============================================================
  Volume name: opensandbox-pvc-demo
  Mount path : /mnt/readonly

  [1] Reading marker.txt from read-only mount:
  hello-from-named-volume

  [2] Attempting to write (should fail):
  touch: cannot touch '/mnt/readonly/should-fail.txt': Read-only file system
  Write denied (expected)

  Scenario 2 completed.

============================================================
Scenario 3: Cross-Sandbox Sharing via PVC (Named Volume)
============================================================
  Volume name: opensandbox-pvc-demo

  [Sandbox A] Creating sandbox and writing data...
  [Sandbox A] Wrote /mnt/shared/cross-sandbox.txt

  [Sandbox B] Creating sandbox and reading data...
  [Sandbox B] Reading file written by Sandbox A:
  message-from-sandbox-a

  Cross-sandbox data sharing verified!

  Scenario 3 completed.

============================================================
Scenario 4: SubPath PVC (Named Volume) Mount
============================================================
  Volume name: opensandbox-pvc-demo
  SubPath    : datasets/train
  Mount path : /mnt/training-data

  [1] Listing mounted subpath content:
  ...
  -rw-r--r-- 1 root root   ... data.csv

  [2] Reading data.csv:
  id,value
  1,100
  2,200

  [3] Verifying volume root is NOT visible:
  marker.txt at mount root: NOT-FOUND
  -> Confirmed: subPath isolation is working correctly

  Scenario 4 completed.

============================================================
All scenarios completed successfully!
============================================================
```

## 各 SDK 用法速览

### Python（异步）

```python
from opensandbox import Sandbox
from opensandbox.models.sandboxes import PVC, Volume

sandbox = await Sandbox.create(
    image="ubuntu",
    volumes=[
        Volume(
            name="my-data",
            pvc=PVC(claimName="my-named-volume"),
            mountPath="/mnt/data",
            readOnly=False,       # 可选，默认为 False
            subPath="datasets/train",  # 可选，挂载子目录
        ),
    ],
)
```

### Python（同步）

```python
from opensandbox import SandboxSync
from opensandbox.models.sandboxes import PVC, Volume

sandbox = SandboxSync.create(
    image="ubuntu",
    volumes=[
        Volume(
            name="my-data",
            pvc=PVC(claimName="my-named-volume"),
            mountPath="/mnt/data",
            subPath="datasets/train",  # 可选
        ),
    ],
)
```

### JavaScript / TypeScript

```typescript
import { Sandbox } from "@alibaba-group/opensandbox";

const sandbox = await Sandbox.create({
  image: "ubuntu",
  volumes: [
    {
      name: "my-data",
      pvc: { claimName: "my-named-volume" },
      mountPath: "/mnt/data",
      readOnly: false,
      subPath: "datasets/train",  // 可选
    },
  ],
});
```

### Java / Kotlin

```java
Volume volume = Volume.builder()
    .name("my-data")
    .pvc(PVC.of("my-named-volume"))
    .mountPath("/mnt/data")
    .readOnly(false)
    .subPath("datasets/train")  // 可选
    .build();

Sandbox sandbox = Sandbox.builder()
    .image("ubuntu")
    .volume(volume)
    .build();
```

## 清理

```shell
docker volume rm opensandbox-pvc-demo
```

## 参考资料

- [OSEP-0003: Volume 与 VolumeBinding 支持](../../oseps/0003-volume-and-volumebinding-support.md) — 设计提案
- [Sandbox Lifecycle API 规范](../../specs/sandbox-lifecycle.yml) — Volume 定义的 OpenAPI 规范
- [宿主机目录挂载示例](../host-volume-mount/) — Host path 绑定挂载示例（替代方案）
