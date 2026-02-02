---
title: Volume Support
authors:
  - "yutian.taoyt"
creation-date: 2026-01-29
last-updated: 2026-02-02
status: draft
---

# OSEP-0003: Volume Support

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Requirements](#requirements)
- [Proposal](#proposal)
  - [Notes/Constraints/Caveats](#notesconstraintscaveats)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
- [Test Plan](#test-plan)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed](#infrastructure-needed)
- [Upgrade & Migration Strategy](#upgrade--migration-strategy)
<!-- /toc -->

## Summary

Introduce a runtime-neutral volume model in the Lifecycle API to enable persistent storage mounts across Docker and Kubernetes sandboxes. The proposal adds explicit volume definitions, mount semantics, and security constraints so that artifacts can persist beyond sandbox lifecycles without relying on file transfers.

This proposal focuses on file persistence via filesystem mounts. It is not a general-purpose storage abstraction (e.g., block or object storage APIs); those are only supported indirectly when exposed as a filesystem by the runtime or host.

```text
Time --------------------------------------------------------------->

Volume lifecycle:  [provisioned]-------------------------[retained]--->
Sandbox lifecycle:           [create]---[running]---[stop/delete]
                              |                         |
                          bind volume              unbind volume
```

## Motivation

OpenSandbox users running long-lived agents need artifacts (web pages, images, reports) to persist after a sandbox is terminated or restarted. Today, the API only supports transient filesystem operations via upload/download and provides no mount semantics; as a result, users must move large outputs out-of-band. This proposal adds first-class storage semantics while maintaining runtime portability and security boundaries.

### Goals

- Add a volume mount field to the Lifecycle API without breaking existing clients.
- Support Docker bind mounts (local path) and OSS mounts as the initial MVP.
- Provide secure, explicit controls for read/write access and path isolation.
- Keep runtime-specific details out of the core API where possible.

### Non-Goals

- Full-featured storage orchestration (auto-provisioning, snapshots, backups).
- Automatic cross-sandbox sharing or locking semantics are out of scope; only explicit volume mounts are supported.
- Guaranteeing portability for every storage backend in every runtime.
- Managing backend storage lifecycle (provisioning, resizing, and cleanup) is out of scope; users own and manage underlying storage resources independently.

## Requirements

- Backward compatible with existing sandbox creation requests.
- Works with both Docker and Kubernetes runtimes.
- Enforces path safety and explicit read/write permissions.
- Supports per-sandbox isolation (via subPath or equivalent).
- Clear error messages when a runtime does not support a requested backend.

## Proposal

Add a new optional field to the Lifecycle API:
- `volumes[]`: defines storage mounts for the sandbox. Each entry includes a named backend-specific struct (e.g., `local`, `oss`, `nfs`) and common mount settings (`name`, `mountPath`, `accessMode`, `subPath`).

The core API describes what storage is required using strongly-typed backend definitions. Each backend type has its own dedicated struct with explicit fields, making the schema self-documenting and enabling compile-time validation in typed SDKs. Runtime providers translate the model into platform-specific mounts.

### Notes/Constraints/Caveats

- Sandbox runtime (Docker/Kubernetes) and storage backend (local/OSS/S3) are independent dimensions. The API is designed so the same SDK request can target different runtimes; if a runtime cannot support a backend, it must return a clear validation error.
- OSS/S3/GitFS are popular production backends; this proposal keeps the model extensible so these can be supported early by adding new backend structs.
- The MVP targets Docker with `local` and `oss` backends. Other backends (e.g., `nfs`) are described for future extension and may be unsupported initially.
- Kubernetes template merging currently replaces lists; this proposal requires list-merge or append behavior for volumes/volumeMounts to preserve user input.
- Exactly one backend struct must be specified per volume entry; specifying zero or multiple backend structs is a validation error.

### Risks and Mitigations

- Security risk: Docker hostPath mounts can expose host data. Mitigation: enforce allowlist prefixes, forbid path traversal, and require explicit `accessMode=RW` for write access.
- Portability risk: different backends behave differently. Mitigation: keep core API minimal and require explicit backend selection.
- Operational risk: storage misconfiguration causes startup failures. Mitigation: validate mounts early and provide clear error responses.

## Design Details

### API schema changes
Add to `CreateSandboxRequest`:

```yaml
volumes:
  # Local host path mount
  - name: workdir
    local:
      hostPath: "/data/opensandbox/user-a"
    mountPath: /mnt/work
    accessMode: RW
    subPath: "task-001"

  # OSS mount
  - name: data
    oss:
      bucket: "my-bucket"
      endpoint: "oss-cn-hangzhou.aliyuncs.com"
      path: "/sandbox/user-a"
      accessKeyId: "AKIDEXAMPLE"
      accessKeySecret: "SECRETEXAMPLE"
    mountPath: /mnt/data
    accessMode: RW

  # NFS mount (future)
  - name: shared
    nfs:
      server: "nfs.example.com"
      path: "/exports/sandbox"
    mountPath: /mnt/shared
    accessMode: RO
```

### Core semantics
- `volumes[]` declares storage mounts. Each volume entry contains:
  - `name`: unique identifier for the volume within the sandbox.
  - Exactly one backend struct (`local`, `oss`, `nfs`, etc.) with backend-specific typed fields.
  - `mountPath`: absolute path inside the container where the volume is mounted.
  - `accessMode`: `RW` (read/write) or `RO` (read-only).
  - `subPath` (optional): subdirectory under the backend path to mount.

### API enum specifications
Enumerations are fixed and validated by the API:
- `accessMode`: use short forms `RW` (read/write) and `RO` (read-only). Examples in this document follow that convention.

### Backend struct definitions
Each backend type is defined as a distinct struct with explicit typed fields:

**`local`** - Host path bind mount:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `hostPath` | string | Yes | Absolute path on the host filesystem |

**`oss`** - Alibaba Cloud OSS mount:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bucket` | string | Yes | OSS bucket name |
| `endpoint` | string | Yes | OSS endpoint URL (e.g., `oss-cn-hangzhou.aliyuncs.com`) |
| `path` | string | No | Path prefix within the bucket (default: `/`) |
| `accessKeyId` | string | Yes* | Access key ID for authentication |
| `accessKeySecret` | string | Yes* | Access key secret for authentication |

*Future enhancement: support `credentialRef` for secret references instead of inline credentials.

**`nfs`** - NFS mount (future):
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `server` | string | Yes | NFS server hostname or IP |
| `path` | string | Yes | Absolute export path on the NFS server |

Additional backends (e.g., `s3`, `pvc`) can be added by defining new structs following this pattern.

### Backend constraints
Validation rules for each backend struct to reduce runtime-only failures:

- **`local`**: `hostPath` must be an absolute path (e.g., `/data/opensandbox/user-a`). Reject relative paths and require normalization before validation.
- **`nfs`**: `server` must be a valid hostname or IP. `path` must be an absolute path (e.g., `/exports/sandbox`).
- **`oss`**: `bucket` must be a valid bucket name. `endpoint` must be a valid OSS endpoint. `accessKeyId` and `accessKeySecret` are required unless `credentialRef` is provided (future). The runtime performs the mount during sandbox creation.

These constraints are enforced in request validation and surfaced as clear API errors; runtimes may apply stricter checks.

### Permissions and ownership
Volume permissions are a frequent source of runtime failures and must be explicit in the contract:
- Default behavior: OpenSandbox does not automatically fix ownership or permissions on mounted storage. Users are responsible for ensuring the backend target is writable by the sandbox process UID/GID.
- Docker: host path permissions are enforced by the host filesystem. Even with `accessMode=RW`, writes will fail if the host path is not writable by the container user.
- Kubernetes: filesystem permissions vary by storage driver. Future enhancement: add optional `fsGroup` field to backend structs that support it for pod-level volume access control.

### Concurrency and isolation
SubPath provides path-level isolation, not concurrency control. If multiple sandboxes mount the same volume without distinct `subPath` values and use `accessMode=RW`, they may overwrite each other. OpenSandbox does not provide file-locking or coordination; users are responsible for handling concurrent access safely.

### Docker mapping
- `local` backend maps to bind mounts. `local.hostPath + subPath` resolves to a concrete host directory.
- The host config uses `mounts`/`binds` with `readOnly` derived from `accessMode`.
- If the resolved host path does not exist, the request fails validation (do not auto-create host directories in MVP to avoid permission and security pitfalls).
- Allowed host paths are restricted by a server-side allowlist; users must specify a `local.hostPath` under permitted prefixes. The allowlist is an operator-configured policy and should be documented for users of a given deployment.
- `oss` backend requires the runtime to mount a filesystem (e.g., via ossfs) during sandbox creation using the struct fields. If the runtime does not support OSS mounting, the request is rejected.

### Kubernetes mapping
- `nfs` backend maps to Kubernetes `nfs` volume source: `nfs.server` → `volumes[].nfs.server`, `nfs.path` → `volumes[].nfs.path`.
- `mountPath` maps to `volumeMounts.mountPath`.
- `subPath` maps to `volumeMounts.subPath`.
- `oss` backend maps to OSS CSI driver or equivalent runtime-specific mount configured with the struct fields.
- `local` backend maps to `hostPath` volume source and is node-local. For persistence guarantees in multi-node clusters, users must pin scheduling (node affinity) or use LocalPersistentVolume; otherwise data can disappear if the pod is rescheduled.

### Example: Local host path mount
Create a sandbox that mounts a local host directory:

```yaml
volumes:
  - name: workdir
    local:
      hostPath: "/data/opensandbox/user-a"
    mountPath: /mnt/work
    accessMode: RW
    subPath: "task-001"
```

Python SDK example (local):

```python
from opensandbox.api.lifecycle.client import AuthenticatedClient
from opensandbox.api.lifecycle.api.sandboxes import post_sandboxes
from opensandbox.api.lifecycle.models.create_sandbox_request import CreateSandboxRequest
from opensandbox.api.lifecycle.models.image_spec import ImageSpec
from opensandbox.api.lifecycle.models.resource_limits import ResourceLimits
from opensandbox.api.lifecycle.models.volume import Volume
from opensandbox.api.lifecycle.models.local_backend import LocalBackend

client = AuthenticatedClient(base_url="https://api.opensandbox.io", token="YOUR_API_KEY")

resource_limits = ResourceLimits.from_dict({"cpu": "500m", "memory": "512Mi"})
request = CreateSandboxRequest(
    image=ImageSpec(uri="python:3.11"),
    timeout=3600,
    resource_limits=resource_limits,
    entrypoint=["python", "-c", "print('hello')"],
    volumes=[
        Volume(
            name="workdir",
            local=LocalBackend(
                host_path="/data/opensandbox/user-a",
            ),
            mount_path="/mnt/work",
            access_mode="RW",
            sub_path="task-001",
        )
    ],
)

post_sandboxes.sync(client=client, body=request)
```

### Example: OSS mount (runtime-specific)
Create a sandbox that mounts an OSS bucket prefix via runtime-provided filesystem mount (e.g., ossfs or CSI):

```yaml
volumes:
  - name: workdir
    oss:
      bucket: "my-bucket"
      endpoint: "oss-cn-hangzhou.aliyuncs.com"
      path: "/sandbox/user-a"
      accessKeyId: "AKIDEXAMPLE"
      accessKeySecret: "SECRETEXAMPLE"
    mountPath: /mnt/work
    accessMode: RW
    subPath: "task-001"
```

Runtime mapping (Docker):
- host path: created by the runtime mount step under a configured mount root (e.g., `/mnt/oss/<bucket>/<path>`), then bind-mounted into the container
- container path: `/mnt/work`
- accessMode: `RW`

### Example: Python SDK (lifecycle client)
Use the Python SDK lifecycle client to create a sandbox with an OSS volume mount (future typed model):

```python
from opensandbox.api.lifecycle.client import AuthenticatedClient
from opensandbox.api.lifecycle.api.sandboxes import post_sandboxes
from opensandbox.api.lifecycle.models.create_sandbox_request import CreateSandboxRequest
from opensandbox.api.lifecycle.models.image_spec import ImageSpec
from opensandbox.api.lifecycle.models.resource_limits import ResourceLimits
from opensandbox.api.lifecycle.models.volume import Volume
from opensandbox.api.lifecycle.models.oss_backend import OSSBackend

client = AuthenticatedClient(base_url="https://api.opensandbox.io", token="YOUR_API_KEY")

resource_limits = ResourceLimits.from_dict({"cpu": "500m", "memory": "512Mi"})
request = CreateSandboxRequest(
    image=ImageSpec(uri="python:3.11"),
    timeout=3600,
    resource_limits=resource_limits,
    entrypoint=["python", "-c", "print('hello')"],
    volumes=[
        Volume(
            name="workdir",
            oss=OSSBackend(
                bucket="my-bucket",
                endpoint="oss-cn-hangzhou.aliyuncs.com",
                path="/sandbox/user-a",
                access_key_id="AKIDEXAMPLE",
                access_key_secret="SECRETEXAMPLE",
            ),
            mount_path="/mnt/work",
            access_mode="RW",
            sub_path="task-001",
        )
    ],
)

post_sandboxes.sync(client=client, body=request)
```

### Example: Kubernetes NFS (future)
Create a sandbox that mounts an NFS export with subPath isolation (non-MVP):

```yaml
volumes:
  - name: workdir
    nfs:
      server: "nfs.example.com"
      path: "/exports/sandbox"
    mountPath: /mnt/work
    accessMode: RW
    subPath: "task-001"
```

Runtime mapping (Kubernetes):
```yaml
volumes:
  - name: workdir
    nfs:
      server: nfs.example.com
      path: /exports/sandbox
containers:
  - name: sandbox
    volumeMounts:
      - name: workdir
        mountPath: /mnt/work
        readOnly: false  # derived from accessMode=RW
        subPath: task-001
```

Python SDK example (NFS, future):

```python
from opensandbox.api.lifecycle.client import AuthenticatedClient
from opensandbox.api.lifecycle.api.sandboxes import post_sandboxes
from opensandbox.api.lifecycle.models.create_sandbox_request import CreateSandboxRequest
from opensandbox.api.lifecycle.models.image_spec import ImageSpec
from opensandbox.api.lifecycle.models.resource_limits import ResourceLimits
from opensandbox.api.lifecycle.models.volume import Volume
from opensandbox.api.lifecycle.models.nfs_backend import NFSBackend

client = AuthenticatedClient(base_url="https://api.opensandbox.io", token="YOUR_API_KEY")

resource_limits = ResourceLimits.from_dict({"cpu": "500m", "memory": "512Mi"})
request = CreateSandboxRequest(
    image=ImageSpec(uri="python:3.11"),
    timeout=3600,
    resource_limits=resource_limits,
    entrypoint=["python", "-c", "print('hello')"],
    volumes=[
        Volume(
            name="workdir",
            nfs=NFSBackend(
                server="nfs.example.com",
                path="/exports/sandbox",
            ),
            mount_path="/mnt/work",
            access_mode="RW",
            sub_path="task-001",
        )
    ],
)

post_sandboxes.sync(client=client, body=request)
```

### Provider validation
- Reject unsupported backend types per runtime.
- Validate that exactly one backend struct is specified per volume entry.
- Normalize and validate `subPath` against traversal; reject `..` and absolute path inputs.
- Enforce allowlist prefixes for `local.hostPath` in Docker.
- For `oss` backend, validate required fields (`bucket`, `endpoint`, `accessKeyId`, `accessKeySecret`) and reject missing credentials.
- For `nfs` backend, validate required fields (`server`, `path`).
- `subPath` is created if missing under the resolved backend path; if creation fails due to permissions or policy, the request is rejected.

### Configuration (example)
Host path allowlists are configured by the control plane (server/execd) and enforced at validation time. Example `config.toml`:

```toml
[storage]
allow_host_paths = ["/data/opensandbox", "/tmp/sandbox"]
oss_mount_root = "/mnt/oss"
```

## Test Plan

- Unit tests for schema validation and path normalization.
- Unit tests for backend struct validation:
  - Reject volume entries with zero or multiple backend structs.
  - Validate required fields per backend type.
- Provider unit tests:
  - Docker `local`: bind mount generation, read-only enforcement, allowlist rejection.
  - Docker `oss`: mount option validation, credential validation, mount failure handling.
- Integration tests for sandbox creation with volumes in Docker.
- Negative tests for unsupported backends and invalid paths.

## Drawbacks

- Adds API surface area and increases runtime provider complexity.
- Docker bind mounts introduce security considerations and operational policy requirements.

## Alternatives

- Keep using file upload/download only: simpler but does not satisfy persistence requirements.
- Use runtime-specific `extensions` only: faster to ship but fractures API consistency and increases client complexity.

## Infrastructure Needed

The runtime must have the ability to perform filesystem mounts for the requested backend types (e.g., ossfs for `oss` backend). For `oss` backend, the MVP assumes the runtime can mount using the struct fields provided in the request; `credentialRef` for secret references is a future enhancement.

## Upgrade & Migration Strategy

This change is additive and backward compatible. Existing clients continue to work without modification. If a client submits volume fields to a runtime that does not support them, the API will return a clear validation error.
