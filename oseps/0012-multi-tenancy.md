---
title: Multi-Tenancy Support for Kubernetes Runtime
authors:
  - "@Pangjiping"
creation-date: 2026-04-29
last-updated: 2026-05-07
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
  - [TenantProvider Abstraction](#tenantprovider-abstraction)
  - [Tenants Config File Format (FileTenantProvider)](#tenants-config-file-format-filetenantprovider)
  - [Auth Middleware Flow](#auth-middleware-flow)
  - [Sandbox Service — Namespace Resolution](#sandbox-service--namespace-resolution)
  - [Startup Guards](#startup-guards)
  - [Deployment Changes](#deployment-changes)
  - [Tenant Isolation Model (Reference)](#tenant-isolation-model-reference)
- [Test Plan](#test-plan)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed](#infrastructure-needed)
- [Upgrade & Migration Strategy](#upgrade--migration-strategy)
<!-- /toc -->

## Summary

Add multi-tenancy support to OpenSandbox Server when running on Kubernetes. A new config file `tenants.toml` maps API keys to Kubernetes namespaces, enabling K8s-level isolation between tenants. Opt-in: when `tenants.toml` exists, server enters multi-tenant mode; when absent, single-tenant behavior unchanged.

**Docker runtime is explicitly unsupported.** If `runtime.type = "docker"` and `tenants.toml` exists, the server refuses to start with a clear error. Multi-tenancy requires Kubernetes namespaces — Docker has no equivalent isolation primitive.

## Motivation

Current deployment shares a single API key and a single K8s namespace across all sandbox consumers. Problems:

1. **No workload isolation.** All sandboxes in one namespace — one misbehaving consumer affects all. ResourceQuota, NetworkPolicy, LimitRange cannot be per-consumer.
2. **No credential isolation.** One shared key = no per-consumer audit trail, no per-consumer revocation, no per-consumer rate limiting.

Multi-tenancy gives each tenant its own namespace and API key(s), single server deployment.

### Goals

- Define tenants in independent config file (`tenants.toml`), zero changes to `server.toml`
- Each tenant → dedicated K8s namespace
- Multiple API keys per tenant (key rotation without downtime)
- Hot-reload via fsnotify — no restart
- Single-tenant mode fully intact when `tenants.toml` absent
- Docker runtime explicitly unsupported — server refuses to start if `tenants.toml` present with `runtime.type = "docker"`

### Non-Goals

- Docker runtime multi-tenancy — Docker has no namespace concept; `tenants.toml` with Docker is a startup error, not silently ignored
- Ingress gateway tenant isolation — ingress is a data-plane routing layer, intentionally tenant-unaware; isolation at proxy layer relies on unguessable sandbox IDs + signed tokens + K8s NetworkPolicy
- Dynamic tenant CRUD via REST API (future OSEP)
- Per-tenant rate limiting at server layer (delegate to K8s/ingress)
- Server-side resource quotas (delegate to K8s ResourceQuota)
- Migration tooling (manual, documented)

## Requirements

- `tenants.toml` existence = sole trigger for multi-tenant mode
- When `tenants.toml` exists, `server.api_key` in `server.toml` MUST be rejected
- Each tenant entry MUST have: `name`, `namespace`, `api_keys` (non-empty)
- Auth MUST use constant-time comparison on API keys
- Startup MUST validate all tenant namespaces exist and are accessible
- Sandbox `create`/`get`/`list`/`delete` operate within authenticated tenant's namespace
- Proxy routes MUST validate tenant ownership of target sandbox
- Tenant config changes propagate to all server replicas without restart
- `runtime.type = "docker"` with `tenants.toml` present MUST cause a fatal startup error — multi-tenancy is a K8s-only feature and Docker has no namespace primitive

## Proposal

Introduce a `TenantProvider` abstraction for tenant resolution. The initial implementation is `FileTenantProvider`, backed by `tenants.toml` at `~/.opensandbox/tenants.toml` (overridable via `SANDBOX_TENANTS_CONFIG_PATH`). Auth middleware depends only on the interface, not the file — this leaves room for future providers (HTTP API, K8s Secret, external IAM) without touching auth code.

```
                 ┌───────────────────────────────┐
                 │  server.toml  (unchanged)      │
                 │  [server] api_key = "..."      │
                 │  [kubernetes] namespace = "..." │
                 └───────────────────────────────┘
                            +
                 ┌───────────────────────────────┐
                 │  tenants.toml  (new, optional) │
                 │  [[tenants]]                   │
                 │  name = "team-a"               │
                 │  namespace = "ns-a"            │
                 │  api_keys = ["key1", "key2"]   │
                 └───────────────────────────────┘

            FileTenantProvider (initial backend)
            TenantProvider interface (extension point)
```

**Request routing flow:**

```
Server startup
       │
       ├── runtime.type = "docker" AND tenants.toml exists?
       │       └── YES → FATAL: exit with error. Docker has no namespace isolation.
       │
       └── runtime.type = "kubernetes" (or Docker without tenants.toml)
               │
Request with OPEN-SANDBOX-API-KEY header
       │
       ├── tenants.toml exists?
       │       ├── YES → lookup key in tenant api_keys
       │       │       ├── found  → inject tenant context, route to tenant.namespace
       │       │       └── not found → 401
       │       └── NO  → validate against server.api_key (legacy single-tenant)
       │               ├── valid   → route to kubernetes.namespace
       │               └── invalid → 401
```

### Notes/Constraints/Caveats

- **Docker runtime NOT supported.** If `runtime.type = "docker"` and `tenants.toml` exists, server exits with a fatal error at startup. Docker daemon has no namespace concept — multi-tenancy isolation is impossible. This is a hard rejection, not a silent skip.
- **`server.api_key` disabled in multi-tenant.** Must migrate it into `tenants.toml` as a tenant entry.
- **No server-side quotas.** Delegated to K8s ResourceQuota/LimitRange per namespace.
- **In-memory lookup, no file I/O on hot path.** Config loaded into `dict[str, TenantEntry]` at startup and on fsnotify events.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Plaintext API keys in `tenants.toml` | File permissions 0600; ConfigMap with restricted RBAC; future: K8s Secret reference |
| ConfigMap update delay on multi-replica | kubelet syncs ~1 min; fsnotify triggers reload on each replica independently |
| Namespace doesn't exist at tenant creation | Startup validation; `create_sandbox` returns clear 400 |
| Timing attack on API key comparison | `secrets.compare_digest` (constant-time) |
| Informer memory growth with many namespaces | Lazily created per namespace, only for active sandboxes |

## Design Details

Implementation in 6 steps. No step blocks another except where noted.

---

### TenantProvider Abstraction

Tenant resolution is behind a `TenantProvider` interface, decoupling auth middleware from any specific config source. This lets the initial implementation ship with a simple file-based provider while leaving a clean extension point for enterprise deployments that already manage tenants in an external IAM or tenant management system.

**Interface (`opensandbox_server/tenants/provider.py`):**

```python
class TenantProvider(Protocol):
    def lookup(self, api_key: str) -> Optional[TenantEntry]:
        """Resolve API key → tenant. Returns None if not recognized.
        Raises TenantProviderUnavailable if provider cannot serve."""
        ...

    def list_tenants(self) -> List[TenantEntry]:
        """All known tenant entries (startup validation)."""
        ...

    def ready(self) -> bool:
        """True once provider can serve lookups."""
        ...

    def start(self) -> None:
        """Start background resources (watchers, connections). Called at server startup."""
        ...

    def close(self) -> None:
        """Release resources. Called on server shutdown."""
        ...

    def on_reload(self, callback: Callable[[List[TenantEntry]], None]) -> None:
        """Register callback invoked on tenant data change.
        Not all providers support this; those that don't may ignore."""
        ...
```

**Exception:**
- `TenantProviderUnavailable` — raised when provider cannot serve lookups (e.g., HTTP endpoint unreachable + cache expired beyond `max_stale_seconds`)

**Data model (`opensandbox_server/tenants/models.py`):**

```python
@dataclass(frozen=True)
class TenantEntry:
    name: str
    namespace: str
    api_keys: List[str]
```

---

#### Provider 1 — FileTenantProvider

Backed by `tenants.toml`, loaded at startup, hot-reloaded via filesystem mtime polling.

- Implements full `TenantProvider` interface
- `start()` parses file and starts watcher thread (2s mtime poll)
- `ready()` returns `True` after initial file parse succeeds
- `on_reload` triggers on file change; auth middleware picks up new key→tenant mappings without restart
- File delete → all entries cleared (all tenant keys → 401)
- Parse error during reload → log warning, keep previous state (no downtime)
- Watcher monitors parent directory for ConfigMap atomic symlink swap

---

#### Provider 2 — HTTPTenantProvider

Per-key lookup against a remote HTTP endpoint with in-memory TTL cache. No background thread, no file persistence, no bulk fetch. Keys not looked up are not cached.

**Endpoint contract:**

```
GET {endpoint}
Header: OPEN-SANDBOX-API-KEY: <api_key>    // 客户端原始 key 原封不动转发

200 OK:
{
    "namespace": "ns-a",       // target K8s namespace for this key
    "ttl": 60                  // suggested cache duration in seconds
}

401 Unauthorized:
{
    "code": "UNAUTHORIZED",
    "message": "API key is invalid or revoked"
}
```

Server 将客户端的 `OPEN-SANDBOX-API-KEY` 原封不动转发给 HTTP provider 做校验。Provider 是权威方 — 决定 key 是否有效、映射到哪个 namespace。Server 只需要 `namespace` + `ttl`。

**Cache behavior:**

| Scenario | Action |
|----------|--------|
| Cache hit + within server-suggested TTL | Return cached entry immediately |
| Cache hit + TTL expired | Sync GET → success: update cache with new TTL; failure + within `max_stale_seconds`: return stale; failure + beyond `max_stale_seconds`: raise `TenantProviderUnavailable` |
| Cache miss | Sync GET → 200: cache + return; 401: return `None`; network error: raise `TenantProviderUnavailable` |
| Remote returns 401 for previously cached key | Evict from cache + return `None` (key revoked) |

**Configuration (`HTTPTenantProviderConfig`):**

| Field | Default | Description |
|-------|---------|-------------|
| `endpoint` | (required) | Remote tenant lookup URL |
| `max_stale_seconds` | 300 | Maximum time to serve stale cache when endpoint unreachable |
| `timeout_seconds` | 5 | HTTP request timeout |
| `auth_header` | None | Optional header name for provider-level authentication |
| `auth_token` | None | Optional token value for provider-level authentication |

**Security properties:**
- No persistent cache file → no disk attack surface, no stale file after long downtime
- Cold start (`start()`) only marks ready, does not bulk-fetch (per-key on demand)
- Revoked key (401) immediately evicted from cache
- Max stale bounds the window where unreachable endpoint + stale cache could allow a revoked key

---

#### Provider Selection

Provider type is determined at startup:

```python
# Config field: tenant_provider_type = "file" | "http"
# Or auto-detect:
if tenants.toml exists:
    provider = FileTenantProvider(path)
elif http_tenant_endpoint configured:
    provider = HTTPTenantProvider(config)
else:
    provider = None  # single-tenant mode

provider.start()
if not provider.ready():
    → SystemExit
```

Auth middleware depends only on `TenantProvider`, not on any specific implementation. Switching backends does not touch auth code.

---

### Tenants Config File Format (FileTenantProvider)

**Package:** `opensandbox_server/tenants/`

**File:** `tenants.toml` (path resolved via `SANDBOX_TENANTS_CONFIG_PATH` env or default `~/.opensandbox/tenants.toml`)

```toml
[[tenants]]
name = "team-a"
namespace = "sandbox-team-a"
api_keys = ["sk-a-1", "sk-a-2"]

[[tenants]]
name = "team-b"
namespace = "sandbox-team-b"
api_keys = ["sk-b-1"]
```

**Validation rules (on parse):**
- Each tenant must have non-empty `name`, `namespace`, `api_keys`
- Duplicate `api_keys` across tenants → `ValueError`, server exits

---

### Auth Middleware Flow

**Modify:** `middleware/auth.py`

**Mode detection:** `TenantProvider` instance passed in → multi-tenant; `None` → single-tenant. Middleware depends only on the `TenantProvider` interface, not on `FileTenantProvider`.

**Startup validation:**
```
if provider is not None AND server.api_key is set:
    → SystemExit("Remove server.api_key from server.toml")
```

**Auth flow (pseudocode):**
```
authenticate(request) → TenantEntry | None:
  api_key = request.headers["OPEN-SANDBOX-API-KEY"]

  if multi-tenant mode:
      return provider.lookup(api_key)  # TenantEntry or None
  else:
      return None if constant_time_compare(server.api_key, api_key) else None
      # None with non-empty valid_keys = single-tenant, allow
      # None with empty valid_keys = no keys configured, reject
```

**Tenant context propagation:**
```
dispatch(request):
  tenant = authenticate(request)
  if multi-tenant and tenant is None → 401
  if single-tenant and auth failed → 401
  request.state.tenant = tenant              # TenantEntry | None
  ContextVar("current_tenant").set(tenant)   # for downstream access
```

Downstream code reads tenant via `get_current_tenant() → TenantEntry | None`.

---

### Sandbox Service — Namespace Resolution

**Modify:** `services/kubernetes_service.py`

All K8s API calls replace `self.namespace` with runtime-resolved namespace:

```
_resolve_namespace():
  tenant = get_current_tenant()
  return tenant.namespace if tenant else self.namespace  # config default

_resolve_tenant_name():
  tenant = get_current_tenant()
  return tenant.name if tenant else "default"
```

Methods affected: `create_sandbox`, `list_sandboxes`, `get_sandbox`, `delete_sandbox`.

**Sandbox labels on create:** add `opensandbox.io/tenant = <tenant_name>`.

**Proxy route ownership:** proxy routes (`/sandboxes/{id}/proxy/{port}/...`) bypass API key auth by design — end users hitting sandboxes don't carry `OPEN-SANDBOX-API-KEY`. Ingress gateway is intentionally tenant-unaware.

Isolation at proxy layer relies on:
- **Unguessable sandbox IDs** (random UUIDs) — knowing one tenant's sandbox ID doesn't reveal another's
- **Signed route tokens** (OSEP-0011) — time-limited, cryptographically bound to a single sandbox
- **K8s namespace isolation** — even if traffic reaches a pod, NetworkPolicy restricts cross-namespace pod-to-pod communication

No tenant context is injected on proxy paths. The server resolves the sandbox endpoint purely by sandbox ID and forwards. Tenancy is enforced at lifecycle API boundaries (create/get/list/delete), not at data-plane proxy boundaries.

---

### Startup Guards

**Modify:** `main.py` or `app.py` — before server start.

```
validate_tenant_startup():
  1. Docker + tenants.toml → SystemExit
  2. Missing tenant namespaces → SystemExit (list missing)
  3. server.api_key + tenants.toml coexisting → SystemExit
```

Namespace validation: iterate all tenant entries, call `k8s.read_namespace()` for each. Collect missing. All must exist at startup.

---

### Deployment Changes

**New files:** `deploy/kubernetes/configmap-tenants.yaml`, modify `rbac.yaml`, `deployment.yaml`.

- **Split ConfigMaps:** `opensandbox-server` (server.toml) + `opensandbox-tenants` (tenants.toml)
- **Deployment:** mount both ConfigMaps, set `SANDBOX_TENANTS_CONFIG_PATH` env var
- **RBAC:** upgrade `Role` → `ClusterRole` + `ClusterRoleBinding` (multi-namespace access required)

---

### Tenant Isolation Model (Reference)

Server does not enforce quotas. Isolation delegated to K8s:

| Isolation dimension | K8s mechanism | Scope |
|--------------------|---------------|-------|
| Resource quota | `ResourceQuota` | Per-ns CPU, memory, storage |
| Default limits | `LimitRange` | Per-ns default container resources |
| Network policy | `NetworkPolicy` | Per-ns ingress/egress |
| Sandbox count | `count/batchsandboxes` via `ResourceQuota` | Per-ns CR count |
| RBAC | `RoleBinding` | Per-ns API access |

Cluster admin creates per-tenant namespace with ResourceQuota + LimitRange before tenant onboarding.

## Test Plan

**Unit tests:**
- Duplicate API keys across tenants → `ValueError` at config parse
- Auth: multi-tenant rejects `server.api_key`; accepts valid tenant key; rejects invalid → 401
- TenantLoader: file delete → entries cleared; new key → live in lookup; parse error → old entries kept
- Docker + tenants → `SystemExit`

**Integration tests:**
- Create with tenant A key → sandbox in ns-a with label `opensandbox.io/tenant=team-a`
- List with tenant A → only ns-a sandboxes
- Get/delete tenant A sandbox with tenant B key → 404
- Hot reload: new key works without restart; removed key → 401
- Legacy: delete tenants.toml → server.api_key works again

**End-to-end:**
- Key rotation: add new key, verify both work, remove old key
- Multi-replica: update ConfigMap, all replicas pick up within 60s

## Drawbacks

- **Two config files.** Mitigated by clear startup logging of which mode is active.
- **ClusterRole required.** Broader RBAC than single-namespace RoleBinding. Inherent to multi-tenancy; scoped by resource types.
- **No dynamic tenant CRUD.** Static config only. REST API / CRD deferred to future OSEP.

## Alternatives

| Approach | Rejected because |
|----------|-----------------|
| Embed tenants in `server.toml` | Tenant changes require server restart |
| Couple auth directly to `tenants.toml` file format | Locks out enterprise deployments where tenants already live in IAM/external systems; `TenantProvider` interface avoids this |
| SQLite for tenant storage | Single-node; breaks multi-replica |
| One server instance per tenant | High operational cost (N processes) |
| Soft multi-tenancy (labels, one namespace) | No K8s-native isolation; ResourceQuota/NetworkPolicy not per-tenant |
| Single API key per tenant | No key rotation; replacing key causes downtime |

## Infrastructure Needed

- One K8s namespace per tenant (cluster admin creates)
- Per-namespace ResourceQuota + LimitRange (recommended)
- `opensandbox-tenants` ConfigMap in server namespace
- ClusterRole + ClusterRoleBinding for server ServiceAccount

## Upgrade & Migration Strategy

**Existing single-tenant → multi-tenant:**

1. Create target namespace(s)
2. Write `tenants.toml` with existing key as a tenant entry (same namespace)
3. Mount via ConfigMap alongside `server.toml`
4. Deploy — old key continues working as tenant key
5. Optionally remove `api_key` from `server.toml`
6. Add more tenants as needed

**Rollback:** Delete `tenants.toml` ConfigMap, restart. Falls back to `server.api_key` + `kubernetes.namespace`.

**No data migration needed.** Existing sandboxes stay in their namespace.
