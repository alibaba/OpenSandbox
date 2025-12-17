# Claude Code Example

Access Claude via the `claude-cli` npm package in OpenSandbox.

## Start OpenSandbox server [local]

Pre-pull the code-interpreter image (includes Node.js):

```shell
docker pull opensandbox/code-interpreter:latest

# use acr from China
# docker pull sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:latest
```

Then start the local OpenSandbox server, stdout logs will be visible in the terminal:

```shell
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/server
cp example.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main
```

## Create and Accessing the Claude Sandbox

```shell
# Install OpenSandbox package
uv pip install --index-url https://test.pypi.org/simple/ --extra-index-url https://pypi.org/simple opensandbox

# Run the example (requires SANDBOX_DOMAIN / SANDBOX_API_KEY / ANTHROPIC_AUTH_TOKEN)
uv run python examples/claude-code/main.py
```

The script installs the Claude CLI (`npm i -g @anthropic-ai/claude-code@latest`) at runtime (Node.js is already in the code-interpreter image), then sends a simple request `claude "Compute 1+1=?."`. Auth is passed via `ANTHROPIC_AUTH_TOKEN`, and you can override endpoint/model with `ANTHROPIC_BASE_URL` / `ANTHROPIC_MODEL`.

![Claude Code screenshot](./screenshot.jpg)

## Environment Variables

- `SANDBOX_DOMAIN`: Sandbox service address (default: `localhost:8080`)
- `SANDBOX_API_KEY`: API key if your server requires authentication (optional for local)
- `SANDBOX_IMAGE`: Sandbox image to use (default: `opensandbox/code-interpreter:latest`)
- `ANTHROPIC_AUTH_TOKEN`: Your Anthropic auth token (required)
- `ANTHROPIC_BASE_URL`: Anthropic API endpoint (optional; e.g., self-hosted proxy)
- `ANTHROPIC_MODEL`: Model name (default: `claude_sonnet4`)

## References
- [claude-code](https://www.npmjs.com/package/claude-code) - NPM package for Claude Code CLI
