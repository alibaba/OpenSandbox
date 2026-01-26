# iFlow CLI Example

Call OpenAI-compatible iFlow/custom HTTP endpoints via the `iflow-cli` npm package in OpenSandbox.

## Start OpenSandbox server [local]

Pre-pull the code-interpreter image (includes Node.js):

```shell
docker pull sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.1

# use docker hub
# docker pull opensandbox/code-interpreter:v1.0.1
```

Start the local OpenSandbox server, logs will be visible in the terminal:

```shell
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/server
cp example.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main
```

## Create and Access the iFlow Sandbox

```shell
# Install OpenSandbox package
uv pip install opensandbox

# Run the example (requires SANDBOX_DOMAIN / SANDBOX_API_KEY / IFLOW_API_KEY; IFLOW_BASE_URL has a default)
uv run python examples/iflow-cli/main.py
```

The script installs the iFlow CLI (`npm install -g @iflow-ai/iflow-cli@latest`) at runtime (Node.js is already in the code-interpreter image), then sends a simple request `iflow "Compute 1 + 1."`. The API key and endpoint are passed via environment variables.

![iFlow screenshot](./screenshot.jpg)

## Environment Variables

- `SANDBOX_DOMAIN`: Sandbox service address (default: `localhost:8080`)
- `SANDBOX_API_KEY`: API key if your server requires authentication (optional for local)
- `SANDBOX_IMAGE`: Sandbox image to use (default: `sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.0.1`)
- `IFLOW_API_KEY`: Your iFlow/DashScope API key (required)
- `IFLOW_BASE_URL`: The iFlow API endpoint URL (default: `https://apis.iflow.cn/v1`)
- `IFLOW_MODEL_NAME`: Model to use (default: `qwen3-coder-plus`)

## References
- [iFlow CLI](https://cli.iflow.cn/) - Official iFlow CLI site
