# OpenSandbox Security Review

**Date:** 2026-03-02
**Branch:** `claude/security-codebase-review-MOwFm`
**Scope:** Full codebase — server (Python/FastAPI), execd (Go/Gin), egress (Go), ingress (Go)

---

## Executive Summary

OpenSandbox is a sandbox orchestration platform that provisions isolated container workloads
(Docker or Kubernetes) for agent/AI code execution. The security posture is generally
thoughtful — capability dropping, AppArmor/seccomp hooks, PIDs limits, constant-time
token comparison in the egress sidecar, path traversal mitigations — but several gaps
remain that security engineers should address before production deployment.

---

## Identified Vulnerabilities

### CRITICAL

#### C-1 — No Authentication by Default
**File:** `server/src/middleware/auth.py:102-104`, `server/example.config.toml:25`

```python
# If no API keys are configured, skip authentication
if not self.valid_api_keys:
    return await call_next(request)
```

The example configuration ships with `api_key` **commented out**. When no key is set,
every request — including sandbox creation, deletion, and proxy access — is allowed
without credentials. An operator who follows the example verbatim deploys an open API.

**Recommendation:**
- Change the default behavior to deny all requests when no key is configured (fail-closed).
- Or emit a prominent startup warning that authentication is disabled.
- Document the risk explicitly in the example config and README.

---

#### C-2 — Wildcard CORS + `allow_credentials=True`
**File:** `server/src/main.py:88-94`

```python
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],           # opens all origins
    allow_credentials=True,        # contradicts the wildcard
    allow_methods=["*"],
    allow_headers=["*"],
)
```

The comment acknowledges this must be tightened for production but it ships open.
Per the CORS spec, `allow_credentials=True` with `allow_origins=["*"]` is rejected by
browsers but some non-browser HTTP clients and older framework versions silently accept
it. More critically, this configuration is ready to be mis-deployed as-is.

**Recommendation:**
- Remove `allow_credentials=True` from the default, or replace `"*"` with an explicit
  configurable allowlist.
- Move CORS origins to the TOML config so operators must consciously set them.

---

### HIGH

#### H-1 — Proxy Route Bypasses API Key Authentication
**File:** `server/src/middleware/auth.py:97-100`

```python
# Skip authentication only for the exact proxy-to-sandbox route shape
if self._is_proxy_path(request.url.path):
    return await call_next(request)
```

Any request matching `/sandboxes/{id}/proxy/{port}/...` is forwarded to the target
sandbox without API key validation. This means a sandbox service is publicly reachable
via the server's proxy even when API key auth is enabled for all other routes.
Intended for browser use cases, but it opens a persistent unauthenticated tunnel into
every sandbox.

**Recommendation:**
- Make proxy authentication opt-in via a config flag rather than opt-out.
- At minimum, restrict to same-origin requests or require a short-lived per-sandbox
  access token rather than no token.

---

#### H-2 — execd Token Comparison is Not Constant-Time
**File:** `components/execd/pkg/web/router.go:107-112`

```go
requestedToken := ctx.GetHeader(model.ApiAccessTokenHeader)
if requestedToken == "" || requestedToken != token {
    ctx.AbortWithStatusJSON(http.StatusUnauthorized, ...)
}
```

The egress `policy_server.go` correctly uses `crypto/subtle.ConstantTimeCompare`, but
the execd `accessTokenMiddleware` uses a plain Go `!=` string comparison. This is
susceptible to timing side-channels that allow an attacker to enumerate the correct
token byte-by-byte.

**Recommendation:**
```go
if subtle.ConstantTimeCompare([]byte(requestedToken), []byte(token)) != 1 {
```

---

#### H-3 — Unrestricted File System Access in execd
**Files:** `components/execd/pkg/web/controller/filesystem*.go`

The execd HTTP API exposes:
- Arbitrary file read/download (`GET /files/download?path=<any>`)
- Arbitrary file write/upload (`POST /files/upload` with `metadata.path`)
- Recursive directory removal (`DELETE /directories?path=<any>`)
- File rename/move to arbitrary destinations (`POST /files/mv`)

These operations are performed with the UID of the execd process (typically root in
Docker). When host bind-mounts are in scope, an agent that compromises the API token
can read or overwrite host files reachable through those mounts.

**Recommendation:**
- Enforce a configurable `chroot`-style root in the filesystem controller (e.g.,
  restrict all paths to `/workspace` unless explicitly allowed).
- Validate that `targetPath` in uploads stays within the expected root.

---

### MEDIUM

#### M-1 — Sandbox ID Not Validated; Possible Label-Filter Injection
**File:** `server/src/services/docker.py:224-229`

```python
label_selector = f"{SANDBOX_ID_LABEL}={sandbox_id}"
containers = self.docker_client.containers.list(
    all=True, filters={"label": label_selector}
)
```

`sandbox_id` is a raw path parameter taken from the URL. If it contains `,` or `=`
the Docker label filter may match unexpected containers, returning other sandboxes'
metadata to an unauthorized caller or masking a not-found condition.

**Recommendation:**
- Validate `sandbox_id` against a strict allowlist pattern (e.g., `^[a-zA-Z0-9-]{1,64}$`)
  at the route layer before it reaches the service.

---

#### M-2 — Environment Variable Keys Not Sanitized
**File:** `server/src/services/docker.py:1583-1588`

```python
for key, value in env_dict.items():
    if value is None:
        continue
    environment.append(f"{key}={value}")
```

Environment variable keys are appended without rejecting keys that contain `=` or
newline characters. While Docker's own parser provides a secondary defense, a key
containing `=` will silently produce a malformed entry that may override another
variable or be parsed unexpectedly inside the container.

**Recommendation:**
- Reject `env` keys that contain `=`, `\n`, `\r`, or NUL bytes.

---

#### M-3 — No Request Body Size Limit
**File:** `server/src/main.py` (no body-size middleware)

FastAPI/uvicorn does not enforce a request body size limit by default. An attacker
can send arbitrarily large `CreateSandboxRequest` payloads, exhausting server memory.
The proxy handler (`proxy_sandbox_endpoint_request`) also streams responses without
a cap.

**Recommendation:**
- Add a Starlette `ContentSizeLimitMiddleware` or configure uvicorn's
  `--limit-concurrency` and limit body size in the ASGI layer.

---

#### M-4 — No Rate Limiting
**File:** `server/src/main.py`

The API has no rate limiting. An unauthenticated (or authenticated) client can hammer
`POST /sandboxes` to exhaust host resources, or brute-force the `OPEN-SANDBOX-API-KEY`
header.

**Recommendation:**
- Add a per-IP rate limiter (e.g., `slowapi` for FastAPI) on sandbox-creation and
  authentication-critical endpoints.

---

#### M-5 — Image URI Allows Internal Registry Probing
**File:** `server/src/api/schema.py:46-53`

The `image.uri` field has no format validation. Combined with the `image.auth`
credential pass-through, a caller can specify URIs pointing at internal or
air-gapped registries and use the server's network access to probe or pull from them.

**Recommendation:**
- Implement an allowlist of permitted registry hosts in the TOML config (similar to
  `storage.allowed_host_paths`), defaulting to empty = deny-all for production.

---

#### M-6 — Port Allocation TOCTOU Race
**File:** `server/src/services/docker.py:624-638`

```python
with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        sock.bind(("0.0.0.0", port))
    except OSError:
        continue
    return port
```

The socket is closed immediately after the bind check; Docker then tries to bind the
same port. Under concurrent sandbox creation the same port can be returned twice.

**Recommendation:**
- Use `SO_REUSEPORT` or hold the socket open until Docker claims it, or use Docker's
  native port-allocation mechanism.

---

### LOW

#### L-1 — Sensitive Data May Appear in Structured Error Responses
**File:** `server/src/main.py:118` and service layer throughout

Exception messages (e.g., Docker daemon errors, Kubernetes errors) are surfaced
directly to API callers in the `message` field of error responses. These can contain
internal hostnames, file paths, and configuration details.

**Recommendation:**
- Map internal exceptions to safe user-facing codes. Log the full detail server-side
  only.

---

#### L-2 — f-string Logging Bypasses Lazy Evaluation
**File:** `server/src/services/k8s/kubernetes_service.py:98`

```python
logger.error(f"Failed to initialize Kubernetes client: {e}")
```

f-strings are evaluated before the logger's level check. Use `%s` style or
`logging.exception()` to avoid unnecessary string construction and to ensure
exception context (traceback) is captured.

---

#### L-3 — `Content-Disposition` Header Not Quoted
**File:** `components/execd/pkg/web/controller/filesystem_download.go:58`

```go
c.ctx.Header("Content-Disposition", "attachment; filename="+filepath.Base(filePath))
```

If `filePath` contains spaces or semicolons, the `filename` parameter is not
quoted, which can confuse some HTTP clients. More critically, characters like
`"` or `;` in the filename could be used to inject additional header parameters.

**Recommendation:**
```go
c.ctx.Header("Content-Disposition",
    fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(filepath.Base(filePath), `"`, `\"`)))
```

---

## Architecture Designs for Future Security Features

### 1. Network Traffic for Multi-Agent Communication Channels

Three tiered designs, ordered by implementation complexity:

**Design A — Label-Gated Direct Channel (Lowest Complexity)**

Each sandbox receives an `allowedPeers` label set at creation time. The egress
sidecar already controls outbound DNS/TCP; extend it with a peer-list check:

```
CreateSandboxRequest:
  extensions:
    allowed_peers: "sandbox-abc123,sandbox-def456"
```

At egress-policy evaluation time, the sidecar resolves peer sandbox IDs to their
container IPs via the lifecycle API and adds them to the nftables allow-set. All
other peer-to-peer traffic is denied.

*Trade-offs:* Simple to implement, uses existing egress infrastructure, but requires
the egress sidecar to make outbound lifecycle API calls (creating a dependency loop
that must be carefully managed).

---

**Design B — Brokered Message Bus with Per-Agent Topics (Medium Complexity)**

Add a lightweight NATS (or Redis Streams) broker as a new OpenSandbox component:

```
Sandbox A  →  execd HTTP API  →  /messages/publish  →  NATS topic:<sandbox-a-id>
Sandbox B  ←  execd HTTP API  ←  /messages/subscribe ←  NATS topic:<sandbox-b-id>
```

The lifecycle server injects per-agent NATS credentials (publish-only to own topic,
subscribe-only to explicitly granted peer topics) as environment variables at
sandbox creation. No direct container-to-container networking is required; the broker
enforces channel ownership.

Message envelope:
```json
{
  "from": "sandbox-abc123",
  "to":   "sandbox-def456",
  "correlation_id": "uuid",
  "timestamp": "2026-03-02T...",
  "payload": "<base64>",
  "sig": "<hmac-sha256 over from+to+correlation_id+timestamp+payload>"
}
```

The broker validates the HMAC before delivering, preventing message spoofing.

*Trade-offs:* Requires a new stateful component; adds latency vs. direct channels;
HMAC key rotation must be managed.

---

**Design C — mTLS Service Mesh (Highest Security, Highest Complexity)**

Each sandbox pod/container receives a short-lived X.509 certificate (signed by an
in-cluster CA) injected by an init container:

```
Lifecycle API  →  issues cert for sandbox-abc123  →  mTLS handshake required for all inter-agent calls
```

Agent-to-agent traffic flows through an Envoy sidecar with mTLS enforcement:
- Certificate SAN encodes the sandbox ID.
- The control plane (lifecycle API acting as xDS server) pushes per-sandbox routing
  rules specifying permitted peer SANs.
- The Envoy sidecar rejects connections from SANs not in the allowlist.

*Trade-offs:* Strong cryptographic identity; works at Layer 4; requires Kubernetes,
cert-manager, and Envoy. Significant operational complexity.

---

### 2. Observability for Traffic Behavior and Analysis

**Design A — DNS + Flow Correlation (Extends Existing Egress)**

The egress DNS proxy already intercepts all queries. Extend it to emit structured
events:

```json
{
  "event": "dns_query",
  "sandbox_id": "abc123",
  "query": "api.example.com",
  "resolved_ips": ["93.184.216.34"],
  "action": "allow",
  "ts": "2026-03-02T12:00:00Z"
}
```

Add a parallel nftables `log` target for TCP SYN packets to capture connection
events. Ship both streams to an OpenTelemetry collector (OTLP):

```
egress-sidecar → OTLP exporter → collector → Loki/ClickHouse
```

A Grafana dashboard correlates DNS → TCP → byte-count per sandbox and flags:
- Connections to IPs with no prior DNS query (direct-IP exfil)
- Spike in egress bytes
- Queries to newly-registered or DGA-like domains

*Trade-offs:* Minimal code changes; reuses egress infrastructure; DNS-over-HTTPS
bypasses DNS interception (mitigate by denying port 853 and port 443 to 8.8.8.8 etc.).

---

**Design B — eBPF Socket-Level Telemetry**

Deploy a privileged eBPF daemonset on the host. Attach `kprobe`s to
`tcp_connect` / `udp_sendmsg` filtered by the sandbox's network namespace:

```c
// pseudocode
SEC("kprobe/tcp_connect")
int trace_connect(struct pt_regs *ctx) {
    // filter by net namespace matching sandbox cgroup
    emit_event(sandbox_id, dst_ip, dst_port, pid, comm)
}
```

Events include: process name, PID, destination IP/port, byte counts.
This works regardless of whether the sandbox uses the host network namespace.

*Trade-offs:* Fine-grained visibility; kernel version dependency (BTF required);
privileged daemonset increases host attack surface.

---

**Design C — execd Behavioral Event Stream**

Extend the execd API with a structured audit endpoint:

```
GET /audit/events?since=<cursor>
```

Every execd operation appends to an append-only ring buffer:
```json
{"ts":"...","op":"file_read","path":"/etc/passwd","size":2048},
{"ts":"...","op":"command","cmd":"curl http://...","pid":1234,"exit":0},
{"ts":"...","op":"network","dst":"93.184.216.34:80","bytes_sent":512}
```

The lifecycle server polls the `/audit/events` stream and ships to the central store.
Anomaly detection rules (e.g., command contains `curl`, `wget`, `nc`) trigger alerts.

*Trade-offs:* Application-level visibility only; can be bypassed by a compromised
execd; useful as a first layer, not a sole control.

---

### 3. Kill Switches for Agent Category Termination

**Design A — Category Label + Group-Delete Endpoint (Lowest Complexity)**

Extend `CreateSandboxRequest` with an optional `category` field stored as a
container label. Add a new endpoint:

```
DELETE /categories/{category}
  → lists all sandboxes with label opensandbox.io/category=<category>
  → calls delete_sandbox() concurrently for each
  → returns {terminated: N, errors: [...]}
```

Implementation in `sandbox_service.py`:
```python
async def terminate_category(category: str) -> TerminateCategoryResponse:
    sandboxes = list_by_label(f"opensandbox.io/category={category}")
    results = await asyncio.gather(
        *[delete_sandbox(s.id) for s in sandboxes],
        return_exceptions=True
    )
    ...
```

A "panic button" script calls `DELETE /categories/untrusted-agents` to
terminate an entire fleet instantly.

*Trade-offs:* Simple; relies on the lifecycle API being reachable (if the API is
compromised the kill switch is also compromised).

---

**Design B — Out-of-Band Category Circuit Breaker**

Maintain a per-category state in a shared store (Redis or etcd) that is checked
independently of the lifecycle API:

```
State machine:   active  →  suspended  →  terminated
```

The egress sidecar reads the category state on every policy evaluation. When
`terminated`, the sidecar drops all traffic (nftables flush + default deny) and
kills its own process (causing the sandbox container to lose network access).
This functions even if the lifecycle API is down.

The lifecycle server watches for `terminated` state changes and calls
`container.kill()` on each matching sandbox.

*Trade-offs:* More resilient than API-only; requires a shared store; sidecar must
be present for network kill to work (host-network-mode sandboxes need a different
mechanism).

---

**Design C — Kubernetes Namespace Termination (Kubernetes Runtime Only)**

Assign each agent category to a dedicated Kubernetes namespace:

```
opensandbox-agents-prod
opensandbox-agents-untrusted
```

A kill switch is:
```bash
kubectl delete namespace opensandbox-agents-untrusted
```

Kubernetes cascades the deletion to all pods, services, and network policies in
the namespace. The lifecycle API watches for namespace deletion events via an
informer and removes the category from its internal registry.

*Trade-offs:* Strongest blast radius; relies on Kubernetes API access; namespace
deletion is asynchronous (pods take time to terminate).

---

### 4. Logging Agent Behavior — Opportunities

| Location | Current State | Recommended Addition |
|---|---|---|
| `server/src/api/lifecycle.py` | No request logging | Structured audit middleware: log `client_ip`, `sandbox_id`, `operation`, `status_code`, `duration_ms` for every request |
| `components/execd/pkg/web/router.go` | Logs method + URL only | Add `sandbox_id` context (from env var injected at creation), redact credentials from logged URLs |
| `components/execd/pkg/web/controller/command.go` | No command content logged | Log command content (configurable; default redacted) + exit code to the execd audit stream |
| `components/execd/pkg/web/controller/filesystem*.go` | No file-op logging | Log path, operation type, file size, outcome to audit stream |
| `components/egress/policy_server.go` | Logs policy updates | Add per-connection log: destination FQDN, resolved IP, rule matched, action taken |
| `server/src/middleware/auth.py` | Logs nothing on auth failure | Log auth failures with client IP and header presence (not key value) to detect brute-force |
| Kubernetes service | Uses f-string logging | Switch to lazy `%s` style; add `request_id` propagation through all log entries |

**Recommended Structured Log Schema (all components):**

```json
{
  "ts": "2026-03-02T12:00:00.000Z",
  "level": "INFO",
  "component": "lifecycle|execd|egress|ingress",
  "sandbox_id": "abc123",
  "category": "untrusted-agents",
  "request_id": "req-uuid",
  "operation": "command.run|file.read|network.connect|sandbox.create|auth.fail",
  "principal": "api-key-hash",   // hash, never the raw key
  "detail": { ... },             // operation-specific fields
  "outcome": "success|denied|error",
  "duration_ms": 42
}
```

All components should ship logs in this schema to a central aggregator. The
`sandbox_id` and `category` fields enable kill-switch correlation: when a category
is terminated, all logs for that category can be immediately queried for forensic
analysis.

---

## Summary Table

| ID | Severity | Component | Title |
|---|---|---|---|
| C-1 | Critical | server/auth | No authentication by default |
| C-2 | Critical | server/main | Wildcard CORS + credentials |
| H-1 | High | server/auth | Proxy route bypasses auth |
| H-2 | High | execd/router | Non-constant-time token comparison |
| H-3 | High | execd/filesystem | Unrestricted filesystem access |
| M-1 | Medium | server/docker | Sandbox ID not validated (label injection) |
| M-2 | Medium | server/docker | Env var keys not sanitized |
| M-3 | Medium | server/main | No request body size limit |
| M-4 | Medium | server/main | No rate limiting |
| M-5 | Medium | server/schema | Image URI allows registry probing |
| M-6 | Medium | server/docker | Port allocation TOCTOU |
| L-1 | Low | server | Internal details in error messages |
| L-2 | Low | server/k8s | f-string logging |
| L-3 | Low | execd/download | Unquoted Content-Disposition filename |
