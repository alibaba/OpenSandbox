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

# Optional: enable mitm addon script (e.g., inject request headers)
export OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT=/opt/opensandbox/mitmscripts/add_header.py

# Optional: bypass decryption for selected domains (semicolon-separated regex list)
export OPENSANDBOX_EGRESS_MITMPROXY_IGNORE_HOSTS='.*\.log\.aliyuncs\.com;.*\.example\.internal'
```

## Configuration Reference

| Variable | Required | Purpose | Default |
|------|----------|------|--------|
| `OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT` | Yes | Enable transparent mitmproxy (`1/true/on`, etc.) | Disabled |
| `OPENSANDBOX_EGRESS_MITMPROXY_PORT` | No | mitmdump listen port; `iptables` redirects `80/443` here | `18081` |
| `OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT` | No | mitm addon script path (`-s`) | Empty |
| `OPENSANDBOX_EGRESS_MITMPROXY_IGNORE_HOSTS` | No | Host/IP regex list for TLS pass-through (`;` separated) | Empty |
| `OPENSANDBOX_EGRESS_MITMPROXY_CONFDIR` | No | mitm config and CA directory (passed as `--set confdir=`, also used as `HOME`) | Default directory under `/var/lib/mitmproxy` |
| `OPENSANDBOX_EGRESS_MITMPROXY_UPSTREAM_TRUST_DIR` | No | Trust directory for upstream TLS verification (OpenSSL style) | `/etc/ssl/certs` |

Notes:

- `OPENSANDBOX_EGRESS_MITMPROXY_IGNORE_HOSTS` means **no decryption**, not “completely bypass mitm process”.
- In transparent mode, mitmproxy generally recommends matching by IP/range; verify SNI/resolve behavior if using domain regex only.
- Before mitm, `iptables`, and CA export are ready, `GET /healthz` returns `503 (mitm not ready)` to prevent premature readiness.

## Common Configuration Templates

### 1) Enable Transparent MITM Only

```bash
export OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT=true
```

### 2) Enable with Header Injection

```bash
export OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT=true
export OPENSANDBOX_EGRESS_MITMPROXY_SCRIPT=/opt/opensandbox/mitmscripts/add_header.py
```

Built-in example script: `/opt/opensandbox/mitmscripts/add_header.py` (adds `X-OpenSandbox-Egress: 1`).

### 3) Bypass Decryption for Specific Domains (e.g. log upload)

```bash
export OPENSANDBOX_EGRESS_MITMPROXY_TRANSPARENT=true
export OPENSANDBOX_EGRESS_MITMPROXY_IGNORE_HOSTS='.*\.log\.aliyuncs\.com'
```

### 4) Use a Fixed CA (consistent fingerprint across replicas)

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
