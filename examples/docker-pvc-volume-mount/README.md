# Docker PVC (Named Volume) Mount Example

This example demonstrates how to mount Docker named volumes into sandbox containers using the OpenSandbox `pvc` backend. In Docker runtime, `pvc.claimName` maps to a Docker named volume -- providing a more convenient and secure alternative to host-path bind mounts for sharing data across sandboxes.

> **What is `pvc`?** The `pvc` backend is a runtime-neutral abstraction. In Kubernetes it maps to a PersistentVolumeClaim; in Docker it maps to a named volume. The same API request works on both runtimes. See [OSEP-0003](../../oseps/0003-volume-and-volumebinding-support.md) for the design.

## Why Named Volumes over Host Paths?

| | Host path (`host` backend) | Named volume (`pvc` backend) |
|---|---|---|
| **Security** | Exposes host filesystem paths | Docker manages storage location; no host path exposed |
| **Setup** | Requires `allowed_host_paths` allowlist | No allowlist needed |
| **Cross-sandbox sharing** | All containers must agree on a host path | Reference the same volume name |
| **Portability** | Tied to host directory structure | Works on any Docker host |
| **Lifecycle** | User manages host directories | `docker volume create/rm` |

## Scenarios

| # | Scenario | Description |
|---|----------|-------------|
| 1 | **Read-write mount** | Mount a named volume for bidirectional file I/O |
| 2 | **Read-only mount** | Mount a named volume that sandboxes cannot modify |
| 3 | **Cross-sandbox sharing** | Two sandboxes share data through the same named volume |
| 4 | **SubPath mount** | Mount only a subdirectory of a named volume (consistent with K8s PVC subPath) |

## Prerequisites

### 1. Start OpenSandbox Server

```shell
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker
opensandbox-server
```

### 2. Create a Docker Named Volume

```shell
# Create the named volume
docker volume create opensandbox-pvc-demo

# Seed it with a marker file via a temporary container
docker run --rm -v opensandbox-pvc-demo:/data alpine \
  sh -c "echo 'hello-from-named-volume' > /data/marker.txt"
```

### 3. Install Python SDK

```shell
uv pip install opensandbox
```

### 4. Pull the Sandbox Image

```shell
docker pull ubuntu:latest
```

## Run

```shell
uv run python examples/docker-pvc-volume-mount/main.py
```

The script automatically creates the named volume and seeds it with test data. You can also specify a custom volume name or image:

```shell
SANDBOX_IMAGE=ubuntu SANDBOX_DOMAIN=localhost:8080 uv run python examples/docker-pvc-volume-mount/main.py
```

## Expected Output

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

## SDK Usage Quick Reference

### Python (async)

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
            readOnly=False,       # optional, default is False
            subPath="datasets/train",  # optional, mount a subdirectory
        ),
    ],
)
```

### Python (sync)

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
            subPath="datasets/train",  # optional
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
      subPath: "datasets/train",  // optional
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
    .subPath("datasets/train")  // optional
    .build();

Sandbox sandbox = Sandbox.builder()
    .image("ubuntu")
    .volume(volume)
    .build();
```

## Cleanup

```shell
docker volume rm opensandbox-pvc-demo
```

## References

- [OSEP-0003: Volume and VolumeBinding Support](../../oseps/0003-volume-and-volumebinding-support.md) -- Design proposal
- [Sandbox Lifecycle API Spec](../../specs/sandbox-lifecycle.yml) -- OpenAPI schema for volume definitions
- [Host Volume Mount Example](../host-volume-mount/) -- Host path bind mount example (alternative approach)
