# Credential Vault

Credential Vault is OpenSandbox's outbound credential broker for sandboxed agents and developer tools. Real credentials are written to the egress sidecar by the host-side SDK, while the sandbox process only receives fake or empty credential values. When tools such as Claude Code, Git, curl, package managers, or model API clients make allowed outbound HTTPS requests, the sidecar matches the request against Credential Vault bindings and injects the required authentication headers on the way out. This lets existing tools keep their normal workflows while keeping real secrets out of the sandbox environment, command line, filesystem, and logs, reducing credential exfiltration risk from prompt injection or untrusted code.

## How It Works

Credential Vault is implemented by the egress sidecar. A sandbox must be created
with both a `network_policy` and `credential_proxy.enabled = true` in the Python
SDK. The lifecycle API field name is `credentialProxy.enabled`.

At a high level:

1. The lifecycle server attaches the egress sidecar to the sandbox.
2. The SDK writes credentials and bindings to the sidecar Credential Vault API.
3. The sandbox process runs with fake or empty credential environment variables.
4. When the sandbox makes an HTTPS request, transparent MITM in the sidecar
   inspects the request metadata.
5. If exactly one binding matches the request scheme, host, port, method, and
   path, the sidecar injects the configured auth header.
6. Secret values are redacted from vault responses and response headers.

The active vault used by the MITM process is served over a local Unix domain
socket inside the sidecar. The sandbox workload cannot fetch this active state
over the normal server proxy path.

Credential bindings are intentionally precise. Prefer a default-deny egress
policy and a narrow path match, for example `/v1/*` for Anthropic API calls.

## Auth Types

Each binding uses an `auth` rule to describe how the referenced credential is
rendered into the outbound request:

- `bearer`: injects `Authorization: Bearer <credential>`.
- `basic`: injects `Authorization: Basic <credential>`. The credential value
  must already be base64-encoded `username:password`.
- `apiKey`: injects the credential value into the configured header name.
- `customHeaders`: injects multiple configured headers, each backed by its own
  credential.

Simple examples:

```python
auth={"type": "bearer", "credential": "github-token"}
```

```http
Authorization: Bearer <github-token>
```

```python
auth={"type": "basic", "credential": "registry-basic"}
```

```http
Authorization: Basic <base64(username:password)>
```

```python
auth={"type": "apiKey", "name": "x-api-key", "credential": "anthropic-api-key"}
```

```http
x-api-key: <anthropic-api-key>
```

```python
auth={
    "type": "customHeaders",
    "headers": [
        {"name": "X-Client-Id", "credential": "client-id"},
        {"name": "X-Client-Secret", "credential": "client-secret"},
    ],
}
```

```http
X-Client-Id: <client-id>
X-Client-Secret: <client-secret>
```

## Requirements

- Server config sets `[egress].image`.
- Sandbox create request includes `network_policy`.
- Sandbox create request sets `credential_proxy=CredentialProxyConfig(enabled=True)`.
- The sandbox image has the tools you want to run. For Claude Code, use an image
  with Node.js and npm, such as the OpenSandbox code-interpreter image.

## Claude Code With Anthropic

This example installs Claude Code in the sandbox and calls the official
Anthropic API endpoint. The real API key is read on the host and written to
Credential Vault. The sandbox only sees a fake `ANTHROPIC_API_KEY`.

Before running the script:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
# Optional: export ANTHROPIC_MODEL="<a Claude Code supported Anthropic model>"
```

Run:

```python
import os
from datetime import timedelta

from opensandbox import SandboxSync
from opensandbox.models.sandboxes import (
    Credential,
    CredentialBinding,
    CredentialProxyConfig,
    NetworkPolicy,
    NetworkRule,
    SandboxImageSpec,
)


ANTHROPIC_HOST = "api.anthropic.com"
ANTHROPIC_BASE_URL = "https://api.anthropic.com"
REAL_API_KEY = os.environ["ANTHROPIC_API_KEY"]


sandbox_env = {
    "ANTHROPIC_BASE_URL": ANTHROPIC_BASE_URL,
    "ANTHROPIC_API_KEY": "fake-key-inside-sandbox",
}
if os.getenv("ANTHROPIC_MODEL"):
    sandbox_env["ANTHROPIC_MODEL"] = os.environ["ANTHROPIC_MODEL"]


sandbox = SandboxSync.create(
    image=SandboxImageSpec(
        os.getenv("SANDBOX_IMAGE", "opensandbox/code-interpreter:latest")
    ),
    timeout=timedelta(minutes=15),
    env=sandbox_env,
    network_policy=NetworkPolicy(
        defaultAction="deny",
        egress=[
            NetworkRule(action="allow", target=ANTHROPIC_HOST),
            NetworkRule(action="allow", target="registry.npmjs.org"),
        ],
    ),
    credential_proxy=CredentialProxyConfig(enabled=True),
)

try:
    sandbox.credential_vault.create(
        credentials=[
            Credential(
                name="anthropic-api-key",
                source={"value": REAL_API_KEY},
            )
        ],
        bindings=[
            CredentialBinding(
                name="anthropic-api",
                match={
                    "schemes": ["https"],
                    "ports": [443],
                    "hosts": [ANTHROPIC_HOST],
                    "methods": ["GET", "POST"],
                    "paths": ["/v1/*"],
                },
                auth={
                    "type": "apiKey",
                    "name": "x-api-key",
                    "credential": "anthropic-api-key",
                },
            )
        ],
    )

    sandbox.commands.run(
        "npm install -g @anthropic-ai/claude-code --no-audit --no-fund"
    )
    result = sandbox.commands.run("claude -p '1+1'")
    output = "".join(part.text for part in result.logs.stdout)
    print(output)
finally:
    sandbox.kill()
    sandbox.close()
```

The Claude Code process reads the fake key from `ANTHROPIC_API_KEY`, but the
outbound HTTPS request to `api.anthropic.com/v1/*` receives the real `x-api-key`
header from Credential Vault. If your environment uses a private npm mirror,
replace `registry.npmjs.org` in the network policy and the `npm install`
command with that mirror host.

## Binding Guidance

- Use `defaultAction="deny"` and only allow the service hosts required by the
  tool.
- Scope bindings by path whenever possible, for example `/v1/*`.
- Avoid overlapping bindings at the same precedence; ambiguous matches are
  rejected.
- Do not put real secrets in sandbox `env`, command arguments, files, or
  metadata.
- Keep fake environment variables when a CLI refuses to start without a key; the
  vault-injected header is what authenticates the outbound request.
