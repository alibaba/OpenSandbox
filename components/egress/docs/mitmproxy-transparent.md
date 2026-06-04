# Python mitmproxy Transparent Mode (with Egress)

Transparent mode starts `mitmdump --mode transparent` inside the sidecar and redirects local outbound `TCP 80/443` traffic to the mitmproxy listener via `iptables`. Its core benefits are:

- **No application changes**: no need to set `HTTP_PROXY`; app traffic is intercepted transparently.
- **Observability and extensibility**: use mitm scripts for header injection, auditing, and debugging.
- **Controlled bypass**: use `ignore_hosts` for pass-through TLS (forward only, no decryption).

Typical use case: add L7 visibility/processing at the egress boundary without changing the application networking stack.

## Quick Setup (Minimum Working Config)

### Prerequisites

- Linux network namespace with `CAP_NET_ADMIN` in the container.
- `mitmdump` installed and `mitmproxy` user present in the image (included in official egress image).
- Client/system trusts the mitm root CA; otherwise HTTPS handshakes will fail.

### Enable Transparent MITM

```bash
export OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT=true
```

By default, mitmproxy listens on `18081` and transparent redirect rules are set automatically.

### Common Optional Settings

```bash
# Optional: change listening port (default: 18081)
export OPENSANDBOX_EGRESS_MITMPROXY_PORT=18081

# Optional: load an additional user-defined mitm addon (loaded after the system addon)
export OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT=/path/to/your/addon.py
```

To bypass decryption for selected domains, edit the baked-in
`components/egress/mitmproxy/config.yaml` and rebuild the image — see
"Static Configuration (config.yaml)" below.

## Configuration Reference

### Environment Variables (Per-Deployment Overrides)

| Variable | Required | Purpose | Default |
|------|----------|------|--------|
| `OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT` | Yes | Enable transparent mitmproxy (`1/true/on`, etc.) | Disabled |
| `OPENSANDBOX_EGRESS_MITMPROXY_PORT` | No | mitmdump listen port; `iptables` redirects `80/443` here | `18081` |
| `OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT` | No | Additional user mitm addon script path (`-s`); loaded after the system addon | Empty |
| `OPENSANDBOX_EGRESS_MITMPROXY_UPSTREAM_TRUST_DIR` | No | Trust directory for upstream TLS verification (OpenSSL style); overrides the config.yaml default | `/etc/ssl/certs` |
| `OPENSANDBOX_EGRESS_MITMPROXY_SSL_INSECURE` | No | Skip upstream TLS verification (`1/true/on`); use when clients connect by IP and SNI is unavailable | Disabled |

Notes:

- In transparent mode, mitmproxy generally recommends matching by IP/range; verify SNI/resolve behavior if using domain regex only.
- Before mitm, `iptables`, and CA export are ready, `GET /healthz` returns `503 (mitm not ready)` to prevent premature readiness.

### Static Configuration (config.yaml)

Fleet-wide, rarely-changing mitm options live in
`components/egress/mitmproxy/config.yaml`, baked into the image at
`/var/lib/mitmproxy/.mitmproxy/config.yaml` and auto-loaded by mitmdump.
This is the single source of truth for:

- `mode` (`transparent`) — mitm default is `regular`
- `listen_host` (`127.0.0.1`) — mitm default is `0.0.0.0`
- `stream_large_bodies` (`10m`) — mitm default is unset (entire body buffered)
- `ssl_verify_upstream_trusted_confdir` (`/etc/ssl/certs`) — mitm default is unset; overridable per-deployment via env
- `ignore_hosts` (`[]`) — matches the mitm default; kept in the file as a discoverable extension point for operators adding TLS pass-through entries

Only deviations from the mitm built-in defaults are declared in `config.yaml` (the `ignore_hosts` line is the one intentional exception, kept for discoverability). Other options that happen to match the default (`connection_strategy=lazy`, `http2=true`, etc.) are omitted — the file is the diff against upstream defaults, not a full enumeration.

Precedence: command-line `--set` (from env overrides) > `config.yaml` > mitmproxy built-in defaults.

To change a static option for the whole fleet: edit `config.yaml`, rebuild the egress image, redeploy. To bypass decryption for a specific host **temporarily** in one deployment, the option is to edit and remount `config.yaml` rather than pass an env override.

## Common Configuration Templates

### 1) Enable Transparent MITM Only

```bash
export OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT=true
```

### 2) System Addon (Always On)

The bundled system addon at `/var/egress/mitmscripts/system.py` is shipped in the egress image and loaded automatically whenever transparent mode is enabled. It stays wire-transparent (no headers added or altered) and currently provides:

- Forces streaming (`flow.response.stream = True`) for SSE (`text/event-stream`) and chunked responses, so each chunk is forwarded immediately instead of being buffered up to the `stream_large_bodies=1m` threshold (critical for LLM streaming UX).

The system addon is always loaded and cannot be disabled via configuration. To override its behavior, supply a user addon via `OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT`; user addons are loaded after the system addon and may observe or override its hooks.

### 3) Add a User Addon Alongside the System Addon

```bash
export OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT=true
export OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT=/path/to/your/addon.py
```

The user addon is loaded after the system addon (`-s system.py -s user.py`), so user hooks observe and may override system behavior.

### 4) Bypass Decryption for Specific Domains (e.g. log upload)

Edit `components/egress/mitmproxy/config.yaml` and append to `ignore_hosts`,
then rebuild the egress image:

```yaml
ignore_hosts:
  - '.*\.log\.aliyuncs\.com'
```

`ignore_hosts` means **no decryption**, not "completely bypass mitm process":
mitm still proxies the TCP connection, it just forwards bytes without
breaking TLS, and addons do not see request/response content.

### 5) Use a Fixed CA (consistent fingerprint across replicas)

If CA files already exist in `confdir`, mitmproxy reuses them instead of regenerating on each startup. Typical paths:

- `/var/lib/mitmproxy/.mitmproxy/mitmproxy-ca.pem` (private key)
- `/var/lib/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem` (public cert)

Ensure correct permissions (for example `mitmproxy:mitmproxy`, private key mode `600`).

## Relationship with Policy/DNS

Transparent mitmproxy does not automatically consume egress `NetworkPolicy`. Domain allow/deny behavior is still determined by DNS + (optional) nft rules. If L7 policy enforcement is needed, implement it in mitm scripts.

## Implementation Notes and Limits

Startup flow (high level):

1. Start mitmdump as user `mitmproxy`, listening on `127.0.0.1:<port>`.
2. Wait until the local listener is reachable.
3. Apply IPv4 `iptables` redirect rules: except loopback and mitmproxy-owned traffic, redirect outbound `80/443` to mitm port.

Limits:

- Currently IPv4 `iptables` only; IPv6 is not automatically handled.
- Non-Linux environments (for example local macOS runtime) are not supported for transparent mode.
- Full HTTPS decryption introduces CPU/memory and certificate trust overhead; benchmark before production rollout.
