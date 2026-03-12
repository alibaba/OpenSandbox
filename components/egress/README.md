# OpenSandbox Egress Sidecar

The **Egress Sidecar** is a core component of OpenSandbox that provides **FQDN-based egress control**. It runs alongside the sandbox application container (sharing the same network namespace) and enforces declared network policies.

## Features

- **FQDN-based Allowlist**: Control outbound traffic by domain name (e.g., `api.github.com`).
- **Wildcard Support**: Allow subdomains using wildcards (e.g., `*.pypi.org`).
- **Transparent Interception**: Uses transparent DNS proxying; no application configuration required.
- **Dynamic DNS (dns+nft mode)**: When a domain is allowed and the proxy resolves it, the resolved A/AAAA IPs are added to nftables with TTL so that default-deny + domain-allow is enforced at the network layer.
- **Privilege Isolation**: Requires `CAP_NET_ADMIN` only for the sidecar; the application container runs unprivileged.
- **Graceful Degradation**: If `CAP_NET_ADMIN` is missing, it warns and disables enforcement instead of crashing.

## Architecture

The egress control is implemented as a **Sidecar** that shares the network namespace with the sandbox application.

1.  **DNS Proxy (Layer 1)**:
    - Runs on `127.0.0.1:15353`.
    - `iptables` rules redirect all port 53 (DNS) traffic to this proxy.
    - Filters queries based on the allowlist.
    - Returns `NXDOMAIN` for denied domains.

2.  **Network Filter (Layer 2)** (when `OPENSANDBOX_EGRESS_MODE=dns+nft`):
    - Uses `nftables` to enforce IP-level allow/deny. Resolved IPs for allowed domains are added to dynamic allow sets with TTL (dynamic DNS).
    - At startup, the sidecar whitelists **127.0.0.1** (redirect target for the proxy) and **nameserver IPs** from `/etc/resolv.conf` so DNS resolution and proxy upstream work (including private DNS). Nameserver count is capped and invalid IPs are filtered; see [Configuration](#configuration).

## Requirements

- **Runtime**: Docker or Kubernetes.
- **Capabilities**: `CAP_NET_ADMIN` (for the sidecar container only).
- **Kernel**: Linux kernel with `iptables` support.

## Configuration

- Policy bootstrap & runtime:
  - Default deny-all. Seed initial policy via `OPENSANDBOX_EGRESS_RULES` (JSON, same shape as `/policy`); empty/`{}`/`null` stays deny-all.
  - `/policy` at runtime; empty body resets to default deny-all.
- HTTP service:
  - Listen address: `OPENSANDBOX_EGRESS_HTTP_ADDR` (default `:18080`).
  - Auth: `OPENSANDBOX_EGRESS_TOKEN` with header `OPENSANDBOX-EGRESS-AUTH: <token>`; if unset, endpoint is open.
- Mode (`OPENSANDBOX_EGRESS_MODE`, default `dns`):
  - `dns`: DNS proxy only, no nftables (IP/CIDR rules have no effect at L2).
  - `dns+nft`: enable nftables; if nft apply fails, fallback to `dns`. IP/CIDR enforcement and DoH/DoT blocking require this mode.
- **Nameserver exempt**  
  Set `OPENSANDBOX_EGRESS_NAMESERVER_EXEMPT` to a comma-separated list of **nameserver IPs** (e.g. `26.26.26.26` or `26.26.26.26,100.100.2.116`). Only single IPs are supported; CIDR entries are ignored. Traffic to these IPs on port 53 is not redirected to the proxy (iptables RETURN). In `dns+nft` mode, these IPs are also merged into the nft allow set so proxy upstream traffic to them (sent without SO_MARK) is accepted. Use when the upstream is reachable only via a specific route (e.g. tunnel) and SO_MARK would send proxy traffic elsewhere.
- **DNS and nft mode (nameserver whitelist)**  
  In `dns+nft` mode, the sidecar automatically allows:
  - **127.0.0.1** — so packets redirected by iptables to the proxy (127.0.0.1:15353) are accepted by nft.
  - **Nameserver IPs** from `/etc/resolv.conf` — so client DNS and proxy upstream work (e.g. private DNS).  
  Nameserver IPs are validated (unspecified and loopback are skipped) and capped. Use `OPENSANDBOX_EGRESS_MAX_NS` (default `3`; `0` = no cap, `1`–`10` = cap). See [SECURITY-RISKS.md](SECURITY-RISKS.md) for trust and scope of this whitelist.
- **Blocked hostname webhook**  
  - `OPENSANDBOX_EGRESS_DENY_WEBHOOK`: HTTP endpoint URL. When set, egress asynchronously POSTs JSON **only when a hostname is denied**: `{"hostname": "<original query>", "timestamp": "<RFC3339>", "source": "opensandbox-egress", "sandboxId": "<id-or-empty>"}`. Default timeout 5s, up to 3 retries with exponential backoff starting at 1s; 4xx is not retried, 5xx/network errors are retried.
  - `OPENSANDBOX_EGRESS_SANDBOX_ID`: optional sandbox identifier injected into the webhook payload as `sandboxId`. The value is read once at startup (unset → empty string).
  - **Allow requirement**: you must allow the webhook host (or its IP/CIDR) in the policy; with default deny, if you don’t explicitly allow it, the webhook traffic will be blocked by egress itself. Example: `{"defaultAction":"deny","egress":[{"action":"allow","target":"webhook.example.com"}]}`. If a broader deny CIDR covers the resolved IP, it will still be blocked—adjust your policy accordingly.
- DoH/DoT blocking:
  - DoT (tcp/udp 853) blocked by default.
  - Optional DoH over 443: `OPENSANDBOX_EGRESS_BLOCK_DOH_443=true`. If enabled without blocklist, all 443 is dropped.
  - DoH blocklist (IP/CIDR, comma-separated): `OPENSANDBOX_EGRESS_DOH_BLOCKLIST="9.9.9.9,1.1.1.1/32,2001:db8::/32"`.

### Runtime HTTP API

- Default listen address: `:18080` (override with `OPENSANDBOX_EGRESS_HTTP_ADDR`).
- Endpoints:
- `GET /policy` — returns the current policy.
- `POST /policy` — replaces the policy. Empty/whitespace/`{}`/`null` resets to default deny-all.
  - `PATCH /policy` — merge/append rules at runtime. Body **must** be a JSON array of egress rules (not wrapped in an object). New rules are placed before existing ones (same target overrides), so a later PATCH can override prior wildcard denies with a more specific allow, and vice versa.

Examples:

- DNS allowlist (default deny):
  ```bash
  curl -XPOST http://127.0.0.1:18080/policy \
    -d '{"defaultAction":"deny","egress":[{"action":"allow","target":"*.bing.com"}]}'
  ```
- DNS blocklist (default allow):
  ```bash
  curl -XPOST http://127.0.0.1:18080/policy \
    -d '{"defaultAction":"allow","egress":[{"action":"deny","target":"*.bing.com"}]}'
  ```
- IP/CIDR only:
  ```bash
  curl -XPOST http://127.0.0.1:18080/policy \
    -d '{"defaultAction":"deny","egress":[{"action":"allow","target":"1.1.1.1"},{"action":"deny","target":"10.0.0.0/8"}]}'
  ```
- Mixed DNS + IP/CIDR:
  ```bash
  curl -XPOST http://127.0.0.1:18080/policy \
    -d '{"defaultAction":"deny","egress":[{"action":"allow","target":"*.example.com"},{"action":"allow","target":"203.0.113.0/24"},{"action":"deny","target":"*.bad.com"}]}'
  ```
- Merge-only PATCH (override wildcard deny with a specific allow):
  ```bash
  # baseline: deny *.cloudflare.com
  curl -XPOST http://127.0.0.1:18080/policy \
    -d '{"defaultAction":"allow","egress":[{"action":"deny","target":"*.cloudflare.com"}]}'

  # allow a specific host; PATCH rules are prepended, so this wins
  curl -XPATCH http://127.0.0.1:18080/policy \
    -d '[{"action":"allow","target":"www.cloudflare.com"}]'
  ```

## Build & Run

### 1. Build Docker Image

```bash
# Build locally
docker build -t opensandbox/egress:local .

# Or use the build script (multi-arch)
./build.sh
```

### 2. Run Locally (Docker)

To test the sidecar with a sandbox application:

1.  **Start the Sidecar** (creates the network namespace):

    ```bash
    docker run -d --name sandbox-egress \
      --cap-add=NET_ADMIN \
      opensandbox/egress:local
    ```

    *Note: `CAP_NET_ADMIN` is required for `iptables` redirection.*

    After start, push policy via HTTP (empty body resets to deny-all):

    ```bash
    curl -XPOST http://11.167.84.130:18080/policy \
      -H "OPENSANDBOX-EGRESS-AUTH: $OPENSANDBOX_EGRESS_TOKEN" \
      -d '{"defaultAction":"deny","egress":[{"action":"allow","target":"*.bing.com"}]}'
    ```

2.  **Start Application** (shares sidecar's network):

    ```bash
    docker run --rm -it \
      --network container:sandbox-egress \
      curlimages/curl \
      sh
    ```

3.  **Verify**:

    Inside the application container:

    ```bash
    # Allowed domain
    curl -I https://google.com  # Should succeed

    # Denied domain
    curl -I https://github.com  # Should fail (resolve error)
    ```

## Development

- **Language**: Go 1.24+
- **Key Packages**:
    - `pkg/dnsproxy`: DNS server and policy matching logic.
    - `pkg/iptables`: `iptables` rule management.
    - `pkg/nftables`: nftables static/dynamic rules and DNS-resolved IP sets.
    - `pkg/policy`: Policy parsing and definition.
- **Main (egress)**:
    - `nameserver.go`: Builds the list of IPs to whitelist for DNS in nft mode (127.0.0.1 + validated/capped nameservers from resolv.conf).

```bash
# Run tests
go test ./...
```

### E2E benchmark: dns vs dns+nft (sync dynamic IP write)

An end-to-end benchmark compares **dns** (pass-through, no nft write) and **dns+nft** (sync `AddResolvedIPs` before each DNS reply) under real conditions: sidecar in Docker, iptables redirect, real DNS + HTTPS from a client container.

```bash
./tests/bench-dns-nft.sh
```

More details in [docs/benchmark.md](docs/benchmark.md).

## Troubleshooting

- **"iptables setup failed"**: Ensure the sidecar container has `--cap-add=NET_ADMIN`.
- **DNS resolution fails for all domains**:  
  Check upstream reachability from the sidecar (`ip route`, `dig @<upstream> . NS +timeout=3`). In `dns+nft` mode, check logs for `[dns] whitelisting proxy listen + N nameserver(s)`.
- **Traffic not blocked**: If nftables apply fails, the sidecar falls back to dns; check logs, `nft list table inet opensandbox`, and `CAP_NET_ADMIN`.
