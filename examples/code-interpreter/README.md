# Code Interpreter Sandbox

Complete demonstration of running Python code using the Code Interpreter SDK.

## Getting Code Interpreter image

Pull the prebuilt image from a registry:

```shell
docker pull opensandbox/code-interpreter:latest

# use acr from China
# docker pull sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:latest
```

## Start OpenSandbox server [local]

Start the local OpenSandbox server:

```shell
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/server
cp example.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main
```

## Create and access the Code Interpreter Sandbox

```shell
# Install OpenSandbox packages
uv pip install opensandbox opensandbox-code-interpreter

# Run the example (requires SANDBOX_DOMAIN / SANDBOX_API_KEY)
uv run python examples/code-interpreter/main.py
```

The script creates a Sandbox + CodeInterpreter, runs a Python code snippet and prints stdout/result, then terminates the remote instance.

## Environment variables

- `SANDBOX_DOMAIN`: Sandbox service address (default: `localhost:8080`)
- `SANDBOX_API_KEY`: API key if your server requires authentication
- `SANDBOX_IMAGE`: Sandbox image to use (default: `opensandbox/code-interpreter:latest`)
- `PYTHON_VERSION`: Python version inside the sandbox (default: `3.11`)
