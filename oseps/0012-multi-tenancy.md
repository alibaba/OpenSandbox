---
title: Multi-Tenancy Support for Kubernetes Runtime
authors:
  - "@Pangjiping"
creation-date: 2026-04-29
last-updated: 2026-04-29
status: draft
---

# OSEP-0012: Multi-Tenancy Support for Kubernetes Runtime

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
  - [Step 1: Tenant Data Model + Config Loading](#step-1-tenant-data-model--config-loading)
  - [Step 2: Hot Reload via fsnotify](#step-2-hot-reload-via-fsnotify)
  - [Step 3: Auth Middleware — Tenant-Aware Key Resolution](#step-3-auth-middleware--tenant-aware-key-resolution)
  - [Step 4: Sandbox Service — Dynamic Namespace Resolution](#step-4-sandbox-service--dynamic-namespace-resolution)
  - [Step 5: Startup Guards — Docker Rejection + Namespace Validation](#step-5-startup-guards--docker-rejection--namespace-validation)
  - [Step 6: Deployment Manifests — ConfigMaps + RBAC](#step-6-deployment-manifests--configmaps--rbac)
  - [Tenant Isolation Model (Reference)](#tenant-isolation-model-reference)
- [Test Plan](#test-plan)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed](#infrastructure-needed)
- [Upgrade & Migration Strategy](#upgrade--migration-strategy)
<!-- /toc -->

## Summary

Add multi-tenancy support to OpenSandbox Server when running on Kubernetes. A new independent configuration file `tenants.toml` maps API keys to Kubernetes namespaces, enabling strong K8s-level isolation between tenants. The feature is opt-in: when `tenants.toml` exists, the server enters multi-tenant mode; when absent, the current single-tenant behavior is unchanged. Docker runtime is unaffected.

## Motivation

Current OpenSandbox deployment shares a single API key and a single Kubernetes namespace across all sandbox consumers. This creates two problems:

1. **No workload isolation.** All sandboxes land in one namespace. A misbehaving sandbox or accidental deletion can affect all users. K8s features like ResourceQuota, NetworkPolicy, and LimitRange cannot be applied per consumer.

2. **No credential isolation.** One shared API key means no audit trail per consumer, no ability to revoke one consumer's access without rotating the key for everyone, and no per-consumer rate limiting.

Multi-tenancy solves both by giving each tenant its own namespace and its own API key(s), while keeping the server as a single deployment.

### Goals

- Define tenants as independent configuration, requiring zero changes to `server.toml`
- Each tenant maps to a dedicated Kubernetes namespace, enabling K8s-native isolation
- Support multiple API keys per tenant for key rotation without downtime
- Tenant configuration hot-reloads via fsnotify — no server restart
- Keep single-tenant mode fully intact when `tenants.toml` is absent
- Kubernetes runtime only; Docker runtime unchanged

### Non-Goals

- Docker runtime multi-tenancy (not applicable — Docker uses local daemon, no namespace concept)
- Dynamic tenant CRUD via REST API (may be addressed in a future OSEP)
- Per-tenant rate limiting at the server layer (delegate to K8s or ingress)
- Server-side sandbox resource quotas (delegate to K8s ResourceQuota)
- Migration tooling for existing single-tenant setups (manual migration, documented)

## Requirements

- `tenants.toml` file existence is the sole trigger for multi-tenant mode
- When `tenants.toml` exists, `server.api_key` in `server.toml` MUST be rejected
- Each tenant entry MUST specify: `name`, `namespace`, `api_keys` (non-empty list)
- Auth middleware MUST perform constant-time comparison on API keys
- Server startup MUST validate that all tenant namespaces exist and are accessible
- Sandbox `create` / `get` / `list` / `delete` MUST operate within the authenticated tenant's namespace
- Proxy routes MUST validate tenant ownership of the target sandbox
- Tenant configuration changes MUST propagate to all server replicas without restart
- `runtime.type = "docker"` MUST ignore `tenants.toml` entirely

## Proposal

Introduce a standalone `tenants.toml` config file. Its presence activates multi-tenant mode. The file lives at `~/.opensandbox/tenants.toml` by default, overridable via `SANDBOX_TENANTS_CONFIG_PATH`.

```
                   ┌───────────────────────────────┐
                   │  server.toml  (unchanged)      │
                   │  [server]                      │
                   │  api_key = "legacy-key"        │
                   │  [kubernetes]                  │
                   │  namespace = "default-ns"      │
                   └───────────────────────────────┘
                              +
                   ┌───────────────────────────────┐
                   │  tenants.toml  (new, optional) │
                   │  [[tenants]]                   │
                   │  name = "team-a"               │
                   │  namespace = "ns-a"            │
                   │  api_keys = ["key1", "key2"]   │
                   │  [[tenants]]                   │
                   │  name = "team-b"               │
                   │  namespace = "ns-b"            │
                   │  api_keys = ["key3"]           │
                   └───────────────────────────────┘
```

**Resolution flow:**

```
Request with OPEN-SANDBOX-API-KEY header
       │
       ├── tenants.toml exists?
       │       │
       │       ├── YES → lookup key in tenant api_keys list
       │       │       ├── found  → inject tenant context, route to tenant.namespace
       │       │       └── not found → 401
       │       │
       │       └── NO  → validate against server.api_key (legacy single-tenant)
       │               ├── valid   → route to kubernetes.namespace
       │               └── invalid → 401
```

### Notes/Constraints/Caveats

- **Kubernetes only.** `runtime.type = "docker"` uses local Docker daemon — namespaces do not apply. Loading `tenants.toml` with Docker is a configuration error at startup.
- **`server.api_key` disabled.** When `tenants.toml` exists, the legacy key stops working. The admin must explicitly migrate it into `tenants.toml` as a tenant entry. This avoids ambiguity about which namespace the legacy key maps to.
- **No server-side quotas.** Resource limits are enforced by Kubernetes ResourceQuota and LimitRange in each namespace. The server does not duplicate these checks.
- **`tenants.toml` read without lock on every request.** The file is loaded into an in-memory `dict[str, TenantEntry]` on startup and on fsnotify events. Read path is a simple dict lookup — no file I/O on the hot path.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| `tenants.toml` contains plaintext API keys | File permissions 0600; ConfigMap with restricted RBAC; future: support K8s Secret reference |
| ConfigMap update delay on multi-replica | kubelet syncs within ~1 minute; fsnotify triggers reload on all replicas independently |
| Namespace does not exist at tenant creation time | Startup validation checks all namespaces; `create_sandbox` returns clear 400 on missing namespace |
| Timing attack on API key comparison | `secrets.compare_digest` for constant-time comparison |
| Informer memory growth with many namespaces | Informers are lazily created per namespace, only for namespaces that have active sandboxes |

## Design Details

Implementation split into 6 ordered steps. Each step lists files changed, dependencies, and verification criteria.

---

### Step 1: Tenant Data Model + Config Loading

**Files:** `opensandbox_server/tenants/__init__.py` (new), `opensandbox_server/tenants/models.py` (new)

**Depends on:** nothing (no other step blocks)

**What:**

```python
# opensandbox_server/tenants/models.py

from pydantic import BaseModel, model_validator

class TenantEntry(BaseModel):
    name: str
    namespace: str
    api_keys: list[str]

class TenantsConfig(BaseModel):
    entries: list[TenantEntry]

    @model_validator(mode="after")
    def _reject_duplicate_keys(self) -> "TenantsConfig":
        seen: dict[str, str] = {}
        for entry in self.entries:
            for k in entry.api_keys:
                if k in seen:
                    raise ValueError(
                        f"Duplicate api_key across tenants: "
                        f"'{seen[k]}' and '{entry.name}' both declare '{k}'"
                    )
                seen[k] = entry.name
        return self
```

```python
# opensandbox_server/tenants/__init__.py

import os
import tomllib
from pathlib import Path
from .models import TenantsConfig

DEFAULT_PATH = Path.home() / ".opensandbox" / "tenants.toml"

def load_tenants_config(path: Path | None = None) -> TenantsConfig | None:
    """Return parsed config, or None if file absent (single-tenant mode)."""
    if path is None:
        path = Path(os.environ.get("SANDBOX_TENANTS_CONFIG_PATH", DEFAULT_PATH))
    if not path.exists():
        return None
    data = tomllib.loads(path.read_text())
    return TenantsConfig(entries=[
        {"name": t["name"], "namespace": t["namespace"], "api_keys": t["api_keys"]}
        for t in data.get("tenants", [])
    ])

def validate_namespaces(config: TenantsConfig, k8s_client) -> list[str]:
    """Check all tenant namespaces exist. Return list of missing namespaces."""
    missing = []
    for entry in config.entries:
        try:
            k8s_client.core_v1_api.read_namespace(entry.namespace)
        except Exception:
            missing.append(entry.namespace)
    return missing
```

**Verification:** Unit test `TenantsConfig._reject_duplicate_keys` raises on duplicate; `load_tenants_config` returns `None` when file absent; returns `TenantsConfig` when file present.

---

### Step 2: Hot Reload via fsnotify

**Files:** `opensandbox_server/tenants/loader.py` (new)

**Depends on:** Step 1 (imports models)

**What:**

```python
# opensandbox_server/tenants/loader.py

import threading
import tomllib
from pathlib import Path
from watchfiles import watch
from .models import TenantEntry

class TenantLoader:
    """Thread-safe in-memory tenant registry. Reloads on file change."""

    def __init__(self, path: Path):
        self._entries: dict[str, TenantEntry] = {}
        self._lock = threading.Lock()
        self._path = path
        self._stop_event = threading.Event()
        if path.exists():
            self._reload()
            self._start_watcher()

    def lookup(self, api_key: str) -> TenantEntry | None:
        with self._lock:
            return self._entries.get(api_key)

    def _reload(self) -> None:
        data = tomllib.loads(self._path.read_text())
        new_entries: dict[str, TenantEntry] = {}
        for raw in data.get("tenants", []):
            entry = TenantEntry(**raw)
            for k in entry.api_keys:
                if k in new_entries:
                    raise ValueError(
                        f"Duplicate api_key '{k}' across tenants "
                        f"'{new_entries[k].name}' and '{entry.name}'"
                    )
                new_entries[k] = entry
        with self._lock:
            self._entries = new_entries

    def _start_watcher(self) -> None:
        """Watch parent dir for ConfigMap atomic symlink swap."""
        parent = self._path.parent
        def _watch():
            for changes in watch(parent, stop_event=self._stop_event):
                for _change_type, changed_path in changes:
                    if Path(changed_path).resolve() == self._path.resolve():
                        try:
                            self._reload()
                        except Exception:
                            pass  # log, keep old entries
        t = threading.Thread(target=_watch, daemon=True)
        t.start()

    def stop(self) -> None:
        self._stop_event.set()
```

**Verification:** Unit test: file deleted mid-run → entries cleared; new key added → `lookup` returns new entry immediately.

---

### Step 3: Auth Middleware — Tenant-Aware Key Resolution

**Files:** `opensandbox_server/middleware/auth.py` (modify)

**Depends on:** Step 1, Step 2 (uses `TenantLoader` and `TenantsConfig`)

**What:**

Existing `_load_api_keys()` returns `set[str]`. Change return type to `dict[str, TenantEntry | None]`:

```python
# middleware/auth.py

def __init__(self, app, config, tenant_loader=None):
    self.app = app
    self.config = config
    self.tenant_loader = tenant_loader  # None in single-tenant mode
    self._valid_api_keys: dict[str, TenantEntry | None] = self._load_api_keys()

def _load_api_keys(self) -> dict[str, TenantEntry | None]:
    if self.tenant_loader is not None:
        # Multi-tenant: server.api_key is rejected.
        if self.config.server.api_key:
            raise SystemExit(
                "server.api_key must not be set when tenants.toml is present. "
                "Migrate the key into tenants.toml."
            )
        # Return empty dict — TenantLoader handles lookup at request time.
        return {}

    # Single-tenant: legacy behavior
    key = self.config.server.api_key
    if key and key.strip():
        return {key: None}
    return {}

def _authenticate(self, request) -> TenantEntry | None:
    api_key = request.headers.get("OPEN-SANDBOX-API-KEY", "")
    if self.tenant_loader is not None:
        return self.tenant_loader.lookup(api_key)
    # Single-tenant: dict lookup with constant-time compare
    for k in self._valid_api_keys:
        if secrets.compare_digest(k, api_key):
            return None  # None means single-tenant, no tenant context
    return None  # auth failed
```

`dispatch()` injects tenant into request state:

```python
# In dispatch():
tenant = self._authenticate(request)
if tenant is None and self.tenant_loader is not None:
    return 401 response  # multi-tenant mode: key not found
if tenant is None and not self._valid_api_keys:
    return 401 response  # single-tenant mode: key not found
# tenant is None with valid _valid_api_keys = single-tenant, allow
request.state.tenant = tenant  # TenantEntry or None
# Set ContextVar for downstream access
_current_tenant.set(tenant)
```

Add ContextVar for sandbox service access:

```python
# middleware/auth.py (top-level)

import contextvars

_current_tenant: contextvars.ContextVar[TenantEntry | None] = \
    contextvars.ContextVar("current_tenant", default=None)

def get_current_tenant() -> TenantEntry | None:
    return _current_tenant.get()
```

**Verification:** Unit test: multi-tenant mode rejects `server.api_key` at startup; valid tenant key → request passes with `request.state.tenant` set; invalid key → 401.

---

### Step 4: Sandbox Service — Dynamic Namespace Resolution

**Files:** `opensandbox_server/services/kubernetes_service.py` (modify)

**Depends on:** Step 3 (calls `get_current_tenant()`)

**What:**

Replace all `self.namespace` usage with runtime-resolved namespace:

```python
# kubernetes_service.py

from opensandbox_server.middleware.auth import get_current_tenant

class KubernetesSandboxService:
    def _resolve_namespace(self) -> str:
        tenant = get_current_tenant()
        if tenant is not None:
            return tenant.namespace
        return self.namespace  # config file default

    def _resolve_tenant_name(self) -> str:
        tenant = get_current_tenant()
        return tenant.name if tenant else "default"

    async def create_sandbox(self, request):
        ns = self._resolve_namespace()
        labels = {
            "app.kubernetes.io/managed-by": "opensandbox",
            "opensandbox.io/tenant": self._resolve_tenant_name(),
        }
        # ... use ns instead of self.namespace for all K8s API calls ...

    def list_sandboxes(self, request):
        ns = self._resolve_namespace()
        return self.workload_provider.list_workloads(namespace=ns)

    def get_sandbox(self, sandbox_id):
        ns = self._resolve_namespace()
        return self.workload_provider.get_workload(sandbox_id, namespace=ns)

    def delete_sandbox(self, sandbox_id):
        ns = self._resolve_namespace()
        return self.workload_provider.delete_workload(sandbox_id, namespace=ns)
```

**Proxy route ownership check** — add before proxying:

```python
# In proxy handler:
sandbox = sandbox_service.get_sandbox(sandbox_id)
if sandbox is None:
    return 404
# If sandbox has tenant label, verify it matches request tenant
sandbox_tenant = sandbox.labels.get("opensandbox.io/tenant")
current_tenant = get_current_tenant()
if current_tenant and sandbox_tenant != current_tenant.name:
    return 404  # don't leak existence
```

**Verification:** Integration test: create with tenant A key → sandbox in ns-a with label; list with tenant B key → empty; get tenant A sandbox with tenant B key → 404.

---

### Step 5: Startup Guards — Docker Rejection + Namespace Validation

**Files:** `opensandbox_server/main.py` or `opensandbox_server/app.py` (modify)

**Depends on:** Step 1, Step 2

**What:**

```python
# main.py — near startup, after config load, before server start

def _validate_tenant_startup(app_config, tenants_config, tenant_loader):
    # Guard 1: Docker + tenants.toml = error
    if app_config.runtime.type == "docker" and tenants_config is not None:
        raise SystemExit(
            "tenants.toml is not supported with runtime.type='docker'. "
            "Remove tenants.toml or switch to runtime.type='kubernetes'."
        )

    # Guard 2: All tenant namespaces must exist
    if tenants_config is not None:
        missing = validate_namespaces(tenants_config, k8s_client)
        if missing:
            raise SystemExit(
                f"Tenant namespaces not found: {missing}. "
                f"Create namespaces before starting the server."
            )

    # Guard 3: server.api_key must not coexist with tenants.toml
    if tenants_config is not None and app_config.server.api_key:
        raise SystemExit(
            "server.api_key is set but tenants.toml is present. "
            "Remove server.api_key from server.toml."
        )
```

**Verification:** Unit test: Docker+tenants → `SystemExit`; missing namespace → `SystemExit` with list; `server.api_key`+tenants → `SystemExit`.

---

### Step 6: Deployment Manifests — ConfigMaps + RBAC

**Files:** `deploy/kubernetes/*.yaml` (modify), `deploy/kubernetes/rbac.yaml` (modify)

**Depends on:** Steps 1–5 complete (server code ready)

**6a. Split ConfigMaps:**

```yaml
# deploy/kubernetes/configmap-server.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: opensandbox-server
data:
  server.toml: |
    [server]
    host = "0.0.0.0"
    port = 8080
    [runtime]
    type = "kubernetes"
    execd_image = "opensandbox/execd:latest"

---
# deploy/kubernetes/configmap-tenants.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: opensandbox-tenants
data:
  tenants.toml: |
    [[tenants]]
    name = "team-alpha"
    namespace = "opensandbox-alpha"
    api_keys = ["osk-xxx"]
```

**6b. Deployment volume mounts:**

```yaml
# deploy/kubernetes/deployment.yaml — add to existing
spec:
  template:
    spec:
      containers:
      - name: server
        volumeMounts:
        - name: server-config
          mountPath: /etc/opensandbox/server.d
        - name: tenant-config
          mountPath: /etc/opensandbox/tenants.d
        env:
        - name: SANDBOX_TENANTS_CONFIG_PATH
          value: /etc/opensandbox/tenants.d/tenants.toml
      volumes:
      - name: server-config
        configMap:
          name: opensandbox-server
      - name: tenant-config
        configMap:
          name: opensandbox-tenants
```

**6c. RBAC upgrade — Role → ClusterRole:**

```yaml
# deploy/kubernetes/rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: opensandbox-server
rules:
  - apiGroups: ["sandbox.opensandbox.io"]
    resources: ["batchsandboxes", "agentsandboxes"]
    verbs: ["*"]
  - apiGroups: [""]
    resources: ["pods", "secrets", "persistentvolumeclaims"]
    verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: opensandbox-server
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: opensandbox-server
subjects:
  - kind: ServiceAccount
    name: opensandbox-server
    namespace: opensandbox-system
```

### Tenant Isolation Model (Reference)

Server does not enforce quotas. Isolation delegated to K8s:

| Isolation dimension | K8s mechanism | Scope |
|--------------------|---------------|-------|
| Resource quota | `ResourceQuota` | Per-ns CPU, memory, storage |
| Default limits | `LimitRange` | Per-ns default container resources |
| Network policy | `NetworkPolicy` | Per-ns ingress/egress |
| Sandbox count | `count/batchsandboxes` via `ResourceQuota` | Per-ns CR count |
| RBAC | `RoleBinding` | Per-ns API access |

Example per-tenant namespace setup (cluster admin responsibility):

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: tenant-quota
  namespace: opensandbox-alpha
spec:
  hard:
    count/batchsandboxes.sandbox.opensandbox.io: "100"
    requests.cpu: "50"
    requests.memory: "100Gi"
---
apiVersion: v1
kind: LimitRange
metadata:
  name: tenant-limits
  namespace: opensandbox-alpha
spec:
  limits:
  - type: Container
    default:
      cpu: "2"
      memory: "4Gi"
    defaultRequest:
      cpu: "100m"
      memory: "128Mi"
```

## Test Plan

**Unit tests:**
- `TenantsConfig.lookup()`: valid key → tenant; invalid key → None; duplicate keys across tenants → `ValueError` at config load time
- `AuthMiddleware`: rejects `server.api_key` when tenants loaded; accepts any valid tenant key
- `TenantLoader._reload()`: file deleted mid-run → clear entries; new key added → live in lookup
- Docker runtime startup: error if `tenants.toml` exists with `runtime.type = "docker"`

**Integration tests:**
- Create sandbox with tenant A key → lands in tenant A namespace with `opensandbox.io/tenant=team-a` label
- List sandboxes with tenant A key → only returns sandboxes in tenant A namespace
- Get sandbox with tenant B key for tenant A's sandbox → 404
- Delete sandbox with wrong tenant key → 404
- Hot reload: update tenants.toml, add new key, new key works without restart
- Hot reload: remove tenant, old key returns 401
- Legacy mode: delete tenants.toml, server.api_key works again
- runtime.type=docker + tenants.toml → server refuses to start

**End-to-end scenarios:**
- Key rotation: add new key to api_keys, verify both old and new work, then remove old key
- Multi-replica: update ConfigMap, verify all replicas pick up change within 60 seconds

## Drawbacks

- **Two config files to manage.** Operators must understand the split. Mitigated by clear docs and the server logging which mode it's in at startup.
- **ClusterRole required.** Multi-namespace access needs broader RBAC than single-namespace RoleBinding. This is inherent to multi-tenancy and can be scoped by omitting unnecessary resource types.
- **No dynamic tenant CRUD.** Tenants are defined statically in `tenants.toml`. A REST API for tenant management is deferred to a future OSEP (likely via a `Tenant` CRD).

## Alternatives

| Approach | Rejected because |
|----------|-----------------|
| Embed tenants in `server.toml` | Mixes infrastructure config with tenant config; any tenant change requires server restart |
| SQLite for tenant storage | Single-node; breaks with multi-replica deployments |
| One server instance per tenant | High operational cost; N processes to manage, scale, and monitor |
| Soft multi-tenancy (labels, one namespace) | No K8s-native isolation; ResourceQuota, NetworkPolicy cannot be per-tenant |
| Single API key per tenant | No key rotation; replacing a key causes downtime for all clients |

## Infrastructure Needed

- One Kubernetes namespace per tenant (created by cluster admin before tenant onboarding)
- Per-namespace ResourceQuota and LimitRange (optional, recommended)
- `opensandbox-tenants` ConfigMap in the server's namespace
- ClusterRole + ClusterRoleBinding for the server ServiceAccount (upgrade from RoleBinding)

## Upgrade & Migration Strategy

**For existing single-tenant deployments:**

1. Create the target namespace(s) in Kubernetes.
2. Write `tenants.toml` with the existing `server.api_key` value as a tenant entry:

   ```toml
   [[tenants]]
   name = "default"
   namespace = "opensandbox"       # existing namespace
   api_keys = ["<existing-key>"]   # value from server.toml server.api_key
   ```

3. Mount `tenants.toml` via ConfigMap alongside `server.toml`.
4. Deploy. The server enters multi-tenant mode. The old key continues to work because it is now a tenant key.
5. Optionally remove `api_key` from `server.toml` (it is now unused).
6. Add additional tenants as needed.

**Rollback:** Delete `tenants.toml` ConfigMap and restart. The server falls back to `server.api_key` and `kubernetes.namespace`.

**No data migration needed.** Existing sandbox resources stay in their namespace and continue to function. New sandboxes created with the same key land in the same namespace.
