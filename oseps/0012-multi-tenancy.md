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
  - [Configuration Model](#configuration-model)
  - [Tenant Data Model](#tenant-data-model)
  - [auth Middleware Changes](#auth-middleware-changes)
  - [Sandbox Service: Dynamic Namespace Resolution](#sandbox-service-dynamic-namespace-resolution)
  - [Hot Reload via fsnotify](#hot-reload-via-fsnotify)
  - [Kubernetes RBAC Requirements](#kubernetes-rbac-requirements)
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

### Configuration Model

**`tenants.toml` — no changes to `server.toml`:**

```toml
# tenants.toml
# File existence activates multi-tenant mode.
# Delete or rename this file to revert to single-tenant mode.

[[tenants]]
name = "team-alpha"
namespace = "opensandbox-alpha"
api_keys = [
    "osk-4xz...",
    "osk-9yz...",     # second key for rotation
]

[[tenants]]
name = "team-beta"
namespace = "opensandbox-beta"
api_keys = ["osk-b3t4..."]
```

**Environment variable override:**

```
SANDBOX_TENANTS_CONFIG_PATH=/etc/opensandbox/tenants.d/tenants.toml
```

**Kubernetes deployment (two ConfigMaps, separate concerns):**

```yaml
# Server infrastructure config — restart on change
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
    execd_image = "..."
    # ... no api_key needed when using tenants ...

---
# Tenant config — fsnotify hot reload, no restart
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

---
apiVersion: apps/v1
kind: Deployment
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
      volumes:
      - name: server-config
        configMap:
          name: opensandbox-server
      - name: tenant-config
        configMap:
          name: opensandbox-tenants
```

### Tenant Data Model

```python
# New file: opensandbox_server/tenants/models.py

class TenantEntry(BaseModel):
    """A tenant with API keys and a K8s namespace."""
    name: str                          # unique identifier, e.g. "team-alpha"
    namespace: str                     # K8s namespace for sandbox workloads
    api_keys: list[str]                # at least one; supports key rotation

class TenantsConfig(BaseModel):
    entries: list[TenantEntry]

    @model_validator(mode="after")
    def _reject_duplicate_keys(self) -> "TenantsConfig":
        """Reject duplicate API keys across tenants — a hard config error."""
        seen: dict[str, str] = {}  # api_key -> tenant name
        for entry in self.entries:
            for k in entry.api_keys:
                if k in seen:
                    raise ValueError(
                        f"Duplicate api_key across tenants: "
                        f"'{seen[k]}' and '{entry.name}' both declare '{k}'"
                    )
                seen[k] = entry.name
        return self

    def lookup(self, api_key: str) -> Optional[TenantEntry]:
        """Constant-time lookup across all tenant keys."""
        for entry in self.entries:
            for k in entry.api_keys:
                if secrets.compare_digest(k, api_key):
                    return entry
        return None
```

### Auth Middleware Changes

`AuthMiddleware._load_api_keys()` current behavior: returns `set[str]` from `server.api_key`.

**New behavior when `tenants.toml` exists:**
- Load `TenantsConfig` from file, build `{api_key: TenantEntry}` dict
- Reject `server.api_key` — it is not in the tenant map
- `dispatch()` injects `TenantEntry` into `request.state.tenant` via `ContextVar`

```python
# middleware/auth.py — key_changes

def _load_api_keys(self) -> dict[str, Optional[TenantEntry]]:
    tenants_cfg = load_tenants_config()
    if tenants_cfg is not None:
        # Multi-tenant mode: server.api_key is ignored.
        # TenantsConfig model_validator already rejected duplicates.
        return {k: e for e in tenants_cfg.entries for k in e.api_keys}

    # Single-tenant mode: legacy behavior
    key = self.config.server.api_key
    if key and key.strip():
        return {key: None}
    return {}
```

### Sandbox Service: Dynamic Namespace Resolution

`KubernetesSandboxService` uses `self.namespace` today. With multi-tenancy:

```python
# kubernetes_service.py — key changes

def _namespace_for(self, tenant: Optional[TenantEntry]) -> str:
    if tenant is not None:
        return tenant.namespace
    return self.namespace  # single-tenant fallback

async def create_sandbox(self, request):
    tenant = get_current_tenant()
    ns = self._namespace_for(tenant)
    labels["opensandbox.io/tenant"] = tenant.name if tenant else "default"
    # ... use ns instead of self.namespace throughout ...

def list_sandboxes(self, request):
    tenant = get_current_tenant()
    ns = self._namespace_for(tenant)
    workloads = self.workload_provider.list_workloads(namespace=ns, ...)
    # ...
```

**Docker runtime guard:**

```python
# main.py or factory.py

if app_config.runtime.type == "docker" and tenants_config is not None:
    logger.error("tenants.toml is not supported with runtime.type='docker'")
    raise SystemExit(1)
```

### Tenant Isolation via Kubernetes Namespace

The server does not enforce resource limits. Tenant isolation is provided entirely by Kubernetes namespace mechanisms:

| Isolation dimension | K8s mechanism | Scope |
|--------------------|---------------|-------|
| Resource quota | `ResourceQuota` | Per-namespace CPU, memory, storage caps |
| Default limits | `LimitRange` | Per-namespace default and max container resources |
| Network policy | `NetworkPolicy` | Per-namespace ingress/egress rules |
| Sandbox count | `count/batchsandboxes` via `ResourceQuota` | Per-namespace CR count limit |
| RBAC | `RoleBinding` | Per-namespace API access control |

Each tenant's namespace is configured independently by the cluster administrator. The server's only responsibility is placing sandbox workloads in the correct namespace. All enforcement is handled by the Kubernetes scheduler and admission controllers.

Example per-namespace configuration:

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
    requests.storage: "500Gi"
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

### Hot Reload via fsnotify

```python
# New file: opensandbox_server/tenants/loader.py

class TenantLoader:
    """Load and watch tenants.toml for changes."""

    def __init__(self, path: Path):
        self._entries: dict[str, TenantEntry] = {}
        self._lock = threading.Lock()
        if path.exists():
            self._reload(path)
            self._start_watcher(path)

    def lookup(self, api_key: str) -> Optional[TenantEntry]:
        with self._lock:
            return self._entries.get(api_key)

    def _reload(self, path: Path) -> None:
        data = tomllib.loads(path.read_text())
        new_entries = {}
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

    def _start_watcher(self, path: Path) -> None:
        # watchfiles on parent dir for ConfigMap atomic symlink swap
        ...
```

`AuthMiddleware` holds a `TenantLoader` reference and calls `loader.lookup(api_key)` on each request. The read lock is only held for the dict access.

### Kubernetes RBAC Requirements

Multi-tenancy requires the server's ServiceAccount to operate across multiple namespaces:

```yaml
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
