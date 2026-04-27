# Claude Code Example

Access Claude via the `claude-cli` npm package in OpenSandbox.

## Start OpenSandbox server [local]

Pre-pull the code-interpreter image (includes Node.js):

```shell
docker pull sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.2

# use docker hub
# docker pull opensandbox/code-interpreter:v1.0.2
```

Then start the local OpenSandbox server, stdout logs will be visible in the terminal:

```shell
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker
opensandbox-server
```

## Create and Access the Claude Sandbox

```shell
# Install OpenSandbox package
uv pip install opensandbox

# Run the example (requires SANDBOX_DOMAIN / SANDBOX_API_KEY / ANTHROPIC_AUTH_TOKEN)
uv run python examples/claude-code/main.py
```

The script installs the Claude CLI (`npm i -g @anthropic-ai/claude-code@latest`) at runtime (Node.js is already in the code-interpreter image), then sends a simple request `claude "Compute 1+1=?."`. Auth is passed via `ANTHROPIC_AUTH_TOKEN`, and you can override endpoint/model with `ANTHROPIC_BASE_URL` / `ANTHROPIC_MODEL`.

![Claude Code screenshot](./screenshot.jpg)

## Environment Variables

- `SANDBOX_DOMAIN`: Sandbox service address (default: `localhost:8080`)
- `SANDBOX_API_KEY`: API key if your server requires authentication (optional for local)
- `SANDBOX_IMAGE`: Sandbox image to use (default: `sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.2`)
- `ANTHROPIC_AUTH_TOKEN`: Your Anthropic auth token (required)
- `ANTHROPIC_BASE_URL`: Anthropic API endpoint (optional; e.g., self-hosted proxy)
- `ANTHROPIC_MODEL`: Model name (default: `claude_sonnet4`)

## Session Persistence

The example above treats each sandbox as ephemeral — Claude Code auth and workspace files are lost when the container exits.

For production use, the **`sandbox-base`** image has Claude Code pre-installed and supports two-tier persistence:

- **User profile** (per-user, shared across sessions): auth tokens, settings, and global instructions survive container replacement by mounting a dedicated volume.
- **Session workspace** (per-session): files and Claude Code conversation history persist via OrangeFS and are restored when you resume a sandbox with the same `SESSION_ID`.

Pass the following when calling `POST /sandboxes` (or the Python SDK's `Sandbox.create`):

```python
sandbox = await Sandbox.create(
    image="opensandbox/sandbox-base:latest",
    connection_config=config,
    env={
        "ANTHROPIC_API_KEY": anthropic_api_key,
        # --- session workspace (OrangeFS) ---
        "USERNAME": user_id,
        "SESSION_ID": session_id,
        "ORANGEFS_RS_ADDR": orangefs_addr,
        "ORANGEFS_TOKEN": orangefs_token,
        "ORANGEFS_VOLUME": orangefs_volume,
        # --- user profile persistence ---
        "CLAUDE_PROFILE_PATH": "/root/.claude-profile",
    },
    volumes=[
        {
            "name": "claude-user-profile",
            "mountPath": "/root/.claude-profile",
            "pvc": {
                "claimName": f"claude-config-{user_id}",
                "createIfNotExists": True,
                "storage": "1Gi",
            },
        }
    ],
    entrypoint=["/entrypoint.sh"],
)
```

All persistence variables are optional — omitting them falls back gracefully to a fully ephemeral sandbox. See [sandboxes/sandbox-base/README.md](../../sandboxes/sandbox-base/README.md) for the complete environment variable reference and fallback behaviour table.

## References
- [claude-code](https://www.npmjs.com/package/claude-code) - NPM package for Claude Code CLI
- [sandbox-base](../../sandboxes/sandbox-base/README.md) - Sandbox Base image reference
