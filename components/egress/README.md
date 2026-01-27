# OpenSandbox Egress Sidecar

The **Egress Sidecar** is a core component of OpenSandbox that provides **FQDN-based egress control**. It runs alongside the sandbox application container (sharing the same network namespace) and enforces declared network policies.

> **Status**: Implementing. Currently supports Layer 1 (DNS Proxy). Layer 2 (Network Filter) is on the roadmap.
> See [OSEP-0001: FQDN-based Egress Control](../../oseps/0001-fqdn-based-egress-control.md) for the detailed design.

## Features

- **FQDN-based Allowlist**: Control outbound traffic by domain name (e.g., `api.github.com`).
- **Wildcard Support**: Allow subdomains using wildcards (e.g., `*.pypi.org`).
- **Transparent Interception**: Uses transparent DNS proxying; no application configuration required.
- **Privilege Isolation**: Requires `CAP_NET_ADMIN` only for the sidecar; the application container runs unprivileged.
- **Graceful Degradation**: If `CAP_NET_ADMIN` is missing, it warns and disables enforcement instead of crashing.

## Architecture

The egress control is implemented as a **Sidecar** that shares the network namespace with the sandbox application.

1.  **DNS Proxy (Layer 1)**:
    - Runs on `127.0.0.1:15353`.
    - `iptables` rules redirect all port 53 (DNS) traffic to this proxy.
    - Filters queries based on the allowlist.
    - Returns `NXDOMAIN` for denied domains.

2.  **Network Filter (Layer 2)** (Roadmap):
    - Will use `nftables` to enforce IP-level restrictions based on resolved domains.

## Requirements

- **Runtime**: Docker or Kubernetes.
- **Capabilities**: `CAP_NET_ADMIN` (for the sidecar container only).
- **Kernel**: Linux kernel with `iptables` support.

## Configuration

- Starts in allow-all mode by default.
- Use the built-in HTTP API to push/update policy; empty/whitespace/`{}`/`null` clears (allow-all).
- Optional token auth to restrict calls to the platform:
  - Set `OPENSANDBOX_EGRESS_TOKEN` in the sidecar container.
  - Callers must send header `OPENSANDBOX-EGRESS-AUTH: <token>`.
  - If token is not set, the endpoint stays open (not recommended for shared environments).
- Optional bootstrap at start via env:
  - `OPENSANDBOX_EGRESS_RULES` (JSON, same shape as `/policy`) seeds initial policy.
  - If unset/empty/`{}`/`null`, sidecar starts allow-all until HTTP updates.

### Runtime HTTP API

- Default listen address: `:18080` (override with `OPENSANDBOX_EGRESS_HTTP_ADDR`).
- Endpoints:
  - `GET /policy` — returns the current policy (`null` when allow-all).
  - `POST /policy` — replaces the policy. Empty/whitespace/`{}`/`null` clears restrictions (allow-all).

Examples:

```bash
curl -XPOST http://11.167.115.8:18080/policy \
  -d '{"defaultAction":"deny","egress":[{"action":"allow","target":"*.bing.com"}]}'

curl -XPOST http://11.167.115.8:18080/policy \
  -d '{"defaultAction":"allow","egress":[{"action":"deny","target":"*.bing.com"}]}'
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

    After start, push policy via HTTP (empty body keeps allow-all):

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
    - `pkg/policy`: Policy parsing and definition.

```bash
# Run tests
go test ./...
```

## Troubleshooting

- **"iptables setup failed"**: Ensure the sidecar container has `--cap-add=NET_ADMIN`.
- **DNS resolution fails for all domains**: Check if the upstream DNS (from `/etc/resolv.conf`) is reachable.
- **Traffic not blocked**: Currently only DNS is filtered. Direct IP access is not yet blocked (Layer 2 pending).
