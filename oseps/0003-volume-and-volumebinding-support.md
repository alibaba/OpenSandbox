---
title: Volume Support
authors:
  - "@hittyt"
creation-date: 2026-01-29
last-updated: 2026-02-03
status: implementing
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
- `volumes[]`: defines storage mounts for the sandbox. Each entry includes a named backend-specific struct (e.g., `host`, `ossfs`, `pvc`, `nfs`) and common mount settings (`name`, `mountPath`, `readOnly`, `subPath`).

The core API describes what storage is required using strongly-typed backend definitions. Each backend type has its own dedicated struct with explicit fields, making the schema self-documenting and enabling compile-time validation in typed SDKs. Runtime providers translate the model into platform-specific mounts.

### Notes/Constraints/Caveats

- Sandbox runtime (Docker/Kubernetes) and storage backend (host/ossfs/pvc) are independent dimensions. The API is designed so the same SDK request can target different runtimes; if a runtime cannot support a backend, it must return a clear validation error.
- OSS/S3/GitFS are popular production backends; this proposal keeps the model extensible so these can be supported early by adding new backend structs.
- The MVP targets Docker with `host` and `ossfs` backends, and Kubernetes with `host`, `ossfs`, and `pvc` backends. Other backends (e.g., `nfs`) are described for future extension and may be unsupported initially.
- Kubernetes template merging currently replaces lists; this proposal requires list-merge or append behavior for volumes/volumeMounts to preserve user input.
- Exactly one backend struct must be specified per volume entry; specifying zero or multiple backend structs is a validation error.

### Risks and Mitigations

- Security risk: Docker hostPath mounts can expose host data. Mitigation: enforce allowlist prefixes, forbid path traversal, and use `readOnly: true` for read-only access when appropriate.
- Portability risk: different backends behave differently. Mitigation: keep core API minimal and require explicit backend selection.
- Operational risk: storage misconfiguration causes startup failures. Mitigation: validate mounts early and provide clear error responses.

## Design Details

### API schema changes
Add to `CreateSandboxRequest`:

```yaml
volumes:
  # Host path mount (read-write by default)
  - name: workdir
    host:
      path: "/data/opensandbox/user-a"
    mountPath: /mnt/work
    subPath: "task-001"

  # OSSFS mount
  - name: data
    ossfs:
      bucket: "my-bucket"
      endpoint: "oss-cn-hangzhou.aliyuncs.com"
      path: "/sandbox/user-a"
      accessKeyId: "AKIDEXAMPLE"
      accessKeySecret: "SECRETEXAMPLE"
      version: "2.0"
    mountPath: /mnt/data

  # PVC mount (Kubernetes, read-only)
  - name: models
    pvc:
      claimName: "shared-models-pvc"
    mountPath: /mnt/models
    readOnly: true

  # NFS mount (future, read-only)
  - name: shared
    nfs:
      server: "nfs.example.com"
      path: "/exports/sandbox"
      options: "nfsvers=4.1,hard,timeo=600"
    mountPath: /mnt/shared
    readOnly: true
```

### Core semantics
- `volumes[]` declares storage mounts. Each volume entry contains:
  - `name`: unique identifier for the volume within the sandbox.
  - Exactly one backend struct (`host`, `ossfs`, `pvc`, `nfs`, etc.) with backend-specific typed fields.
  - `mountPath`: absolute path inside the container where the volume is mounted.
  - `readOnly` (optional): if true, the volume is mounted as read-only. Defaults to false (read-write).
  - `subPath` (optional): subdirectory under the backend path to mount.

### Backend struct definitions
Each backend type is defined as a distinct struct with explicit typed fields:

**`host`** - Host path bind mount:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path on the host filesystem |

**`ossfs`** - Alibaba Cloud OSS mount via ossfs:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bucket` | string | Yes | OSS bucket name |
| `endpoint` | string | Yes | OSS endpoint URL (e.g., `oss-cn-hangzhou.aliyuncs.com`) |
| `path` | string | No | Path prefix within the bucket (default: `/`) |
| `accessKeyId` | string | Yes* | Access key ID for authentication |
| `accessKeySecret` | string | Yes* | Access key secret for authentication |
| `version` | string | No | ossfs version: `1.0` or `2.0` (default: `1.0`) |
| `options` | []string | No | Mount options list (e.g., `["allow_other", "umask=0022"]`) |

*Future enhancement: support `credentialRef` for secret references instead of inline credentials.

**`pvc`** - Kubernetes PersistentVolumeClaim mount:
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `claimName` | string | Yes | Name of the PersistentVolumeClaim in the same namespace |

**`nfs`** - NFS mount (future):
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `server` | string | Yes | NFS server hostname or IP |
| `path` | string | Yes | Absolute export path on the NFS server |
| `options` | string | No | Comma-separated mount options (e.g., `nfsvers=4.1,hard,timeo=600`) |

Additional backends (e.g., `s3`) can be added by defining new structs following this pattern.

### Backend constraints
Validation rules for each backend struct to reduce runtime-only failures:

- **`host`**: `path` must be an absolute path (e.g., `/data/opensandbox/user-a`). Reject relative paths and require normalization before validation.
- **`ossfs`**: `bucket` must be a valid bucket name. `endpoint` must be a valid OSS endpoint. `accessKeyId` and `accessKeySecret` are required unless `credentialRef` is provided (future). `version` must be `1.0` or `2.0`; if omitted, defaults to `1.0`. The runtime performs the mount during sandbox creation.
- **`pvc`**: `claimName` must be a valid Kubernetes resource name. The PVC must exist in the same namespace as the sandbox pod; the runtime validates existence at scheduling time.
- **`nfs`**: `server` must be a valid hostname or IP. `path` must be an absolute path (e.g., `/exports/sandbox`).

These constraints are enforced in request validation and surfaced as clear API errors; runtimes may apply stricter checks.

### Permissions and ownership
Volume permissions are a frequent source of runtime failures and must be explicit in the contract:
- Default behavior: OpenSandbox does not automatically fix ownership or permissions on mounted storage. Users are responsible for ensuring the backend target is writable by the sandbox process UID/GID.
- Docker: host path permissions are enforced by the host filesystem. Even with `readOnly: false`, writes will fail if the host path is not writable by the container user.
- Kubernetes: filesystem permissions vary by storage driver. Future enhancement: add optional `fsGroup` field to backend structs that support it for pod-level volume access control.

### Concurrency and isolation
SubPath provides path-level isolation, not concurrency control. If multiple sandboxes mount the same volume without distinct `subPath` values and use `readOnly: false`, they may overwrite each other. OpenSandbox does not provide file-locking or coordination; users are responsible for handling concurrent access safely.

### Docker mapping
- `host` backend maps to bind mounts. `host.path + subPath` resolves to a concrete host directory.
- The host config uses `mounts`/`binds` with `ReadOnly` set from `readOnly` field.
- If the resolved host path does not exist, the request fails validation (do not auto-create host directories in MVP to avoid permission and security pitfalls).
- Allowed host paths are restricted by a server-side allowlist; users must specify a `host.path` under permitted prefixes. The allowlist is an operator-configured policy and should be documented for users of a given deployment.
- `ossfs` backend requires the runtime to mount OSS via ossfs during sandbox creation using the struct fields. If the runtime does not support ossfs mounting, the request is rejected.

### Kubernetes mapping
- `pvc` backend maps to Kubernetes `persistentVolumeClaim` volume source: `pvc.claimName` → `volumes[].persistentVolumeClaim.claimName`.
- `nfs` backend maps to Kubernetes `nfs` volume source: `nfs.server` → `volumes[].nfs.server`, `nfs.path` → `volumes[].nfs.path`.
- `mountPath` maps to `volumeMounts.mountPath`.
- `subPath` maps to `volumeMounts.subPath`.
- `ossfs` backend maps to OSS CSI driver or equivalent runtime-specific mount configured with the struct fields.
- `host` backend maps to `hostPath` volume source and is node-local. For persistence guarantees in multi-node clusters, users must pin scheduling (node affinity) or use LocalPersistentVolume; otherwise data can disappear if the pod is rescheduled.

### Example: Host path mount
Create a sandbox that mounts a host directory:

```yaml
volumes:
  - name: workdir
    host:
      path: "/data/opensandbox/user-a"
    mountPath: /mnt/work
    subPath: "task-001"
```

Python SDK example (host):

```python
from opensandbox.api.lifecycle.client import AuthenticatedClient
from opensandbox.api.lifecycle.api.sandboxes import post_sandboxes
from opensandbox.api.lifecycle.models.create_sandbox_request import CreateSandboxRequest
from opensandbox.api.lifecycle.models.image_spec import ImageSpec
from opensandbox.api.lifecycle.models.resource_limits import ResourceLimits
from opensandbox.api.lifecycle.models.volume import Volume
from opensandbox.api.lifecycle.models.host import Host

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
            host=Host(
                path="/data/opensandbox/user-a",
            ),
            mount_path="/mnt/work",
            sub_path="task-001",
        )
    ],
)

post_sandboxes.sync(client=client, body=request)
```

### Example: OSSFS mount
Create a sandbox that mounts an OSS bucket via ossfs:

```yaml
volumes:
  - name: workdir
    ossfs:
      bucket: "my-bucket"
      endpoint: "oss-cn-hangzhou.aliyuncs.com"
      path: "/sandbox/user-a"
      accessKeyId: "AKIDEXAMPLE"
      accessKeySecret: "SECRETEXAMPLE"
      version: "2.0"
      options:
        - "allow_other"
        - "umask=0022"
    mountPath: /mnt/work
    subPath: "task-001"
```

Runtime mapping (Docker):
- host path: created by ossfs mount under a configured mount root (e.g., `/mnt/ossfs/<bucket>/<path>`), then bind-mounted into the container
- container path: `/mnt/work`
- readOnly: false (default, read-write)

### Example: Python SDK (lifecycle client)
Use the Python SDK lifecycle client to create a sandbox with an OSSFS volume mount (future typed model):

```python
from opensandbox.api.lifecycle.client import AuthenticatedClient
from opensandbox.api.lifecycle.api.sandboxes import post_sandboxes
from opensandbox.api.lifecycle.models.create_sandbox_request import CreateSandboxRequest
from opensandbox.api.lifecycle.models.image_spec import ImageSpec
from opensandbox.api.lifecycle.models.resource_limits import ResourceLimits
from opensandbox.api.lifecycle.models.volume import Volume
from opensandbox.api.lifecycle.models.ossfs import OSSFS

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
            ossfs=OSSFS(
                bucket="my-bucket",
                endpoint="oss-cn-hangzhou.aliyuncs.com",
                path="/sandbox/user-a",
                access_key_id="AKIDEXAMPLE",
                access_key_secret="SECRETEXAMPLE",
                version="2.0",
                options=["allow_other", "umask=0022"],
            ),
            mount_path="/mnt/work",
            sub_path="task-001",
        )
    ],
)

post_sandboxes.sync(client=client, body=request)
```

### Example: Kubernetes PVC mount
Create a sandbox that mounts an existing PersistentVolumeClaim:

```yaml
volumes:
  - name: models
    pvc:
      claimName: "shared-models-pvc"
    mountPath: /mnt/models
    readOnly: true
    subPath: "v1.0"
```

Runtime mapping (Kubernetes):
```yaml
volumes:
  - name: models
    persistentVolumeClaim:
      claimName: shared-models-pvc
containers:
  - name: sandbox
    volumeMounts:
      - name: models
        mountPath: /mnt/models
        readOnly: true
        subPath: v1.0
```

Python SDK example (PVC):

```python
from opensandbox.api.lifecycle.client import AuthenticatedClient
from opensandbox.api.lifecycle.api.sandboxes import post_sandboxes
from opensandbox.api.lifecycle.models.create_sandbox_request import CreateSandboxRequest
from opensandbox.api.lifecycle.models.image_spec import ImageSpec
from opensandbox.api.lifecycle.models.resource_limits import ResourceLimits
from opensandbox.api.lifecycle.models.volume import Volume
from opensandbox.api.lifecycle.models.pvc import PVC

client = AuthenticatedClient(base_url="https://api.opensandbox.io", token="YOUR_API_KEY")

resource_limits = ResourceLimits.from_dict({"cpu": "500m", "memory": "512Mi"})
request = CreateSandboxRequest(
    image=ImageSpec(uri="python:3.11"),
    timeout=3600,
    resource_limits=resource_limits,
    entrypoint=["python", "-c", "print('hello')"],
    volumes=[
        Volume(
            name="models",
            pvc=PVC(
                claim_name="shared-models-pvc",
            ),
            mount_path="/mnt/models",
            read_only=True,
            sub_path="v1.0",
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
      options: "nfsvers=4.1,hard,timeo=600"
    mountPath: /mnt/work
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
        readOnly: false
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
from opensandbox.api.lifecycle.models.nfs import NFS

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
            nfs=NFS(
                server="nfs.example.com",
                path="/exports/sandbox",
                options="nfsvers=4.1,hard,timeo=600",
            ),
            mount_path="/mnt/work",
            sub_path="task-001",
        )
    ],
)

post_sandboxes.sync(client=client, body=request)
```

### Provider validation
- Reject unsupported backend types per runtime (e.g., `pvc` is only valid in Kubernetes).
- Validate that exactly one backend struct is specified per volume entry.
- Normalize and validate `subPath` against traversal; reject `..` and absolute path inputs.
- Enforce allowlist prefixes for `host.path` in Docker.
- For `ossfs` backend, validate required fields (`bucket`, `endpoint`, `accessKeyId`, `accessKeySecret`) and reject missing credentials.
- For `pvc` backend, validate `claimName` is a valid Kubernetes resource name.
- For `nfs` backend, validate required fields (`server`, `path`).
- `subPath` is created if missing under the resolved backend path; if creation fails due to permissions or policy, the request is rejected.

### Configuration (example)
Host path allowlists are configured by the control plane (server/execd) and enforced at validation time. Example `config.toml`:

```toml
[storage]
allow_host_paths = ["/data/opensandbox", "/tmp/sandbox"]
ossfs_mount_root = "/mnt/ossfs"
```

## Test Plan

- Unit tests for schema validation and path normalization.
- Unit tests for backend struct validation:
  - Reject volume entries with zero or multiple backend structs.
  - Validate required fields per backend type.
- Provider unit tests:
  - Docker `host`: bind mount generation, read-only enforcement, allowlist rejection.
  - Docker `ossfs`: mount option validation, credential validation, version validation (`1.0`/`2.0`), mount failure handling.
  - Kubernetes `pvc`: PVC reference validation, volume mount generation.
- Integration tests for sandbox creation with volumes in Docker and Kubernetes.
- Negative tests for unsupported backends and invalid paths.

## Drawbacks

- Adds API surface area and increases runtime provider complexity.
- Docker bind mounts introduce security considerations and operational policy requirements.

## Alternatives

- Keep using file upload/download only: simpler but does not satisfy persistence requirements.
- Use runtime-specific `extensions` only: faster to ship but fractures API consistency and increases client complexity.

## Infrastructure Needed

The runtime must have the ability to perform filesystem mounts for the requested backend types. For `ossfs` backend, the runtime must have ossfs 1.0 or 2.0 installed; the MVP assumes the runtime can mount using the struct fields provided in the request. `credentialRef` for secret references is a future enhancement.

## Upgrade & Migration Strategy

This change is additive and backward compatible. Existing clients continue to work without modification. If a client submits volume fields to a runtime that does not support them, the API will return a clear validation error.
