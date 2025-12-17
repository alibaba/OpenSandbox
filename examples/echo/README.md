# Echo Example

A minimal example demonstrating how to use OpenSandbox with a simple Ubuntu image to execute a basic command.

## Start OpenSandbox server [local]

Pre-pull the example image:

```shell
docker pull ubuntu:22.04
```

Start the local OpenSandbox server, logs will be visible in the terminal:

```shell
git clone git@github.com:alibaba/OpenSandbox.git
cd OpenSandbox/server
cp example.config.toml ~/.sandbox.toml
uv sync
uv run python -m src.main
```

## Run the Example

```shell
# Install OpenSandbox package
uv pip install --index-url https://test.pypi.org/simple/ --extra-index-url https://pypi.org/simple opensandbox

# Run the example
uv run python examples/echo/main.py
```

The script creates a sandbox using the Ubuntu 22.04 image, executes a simple `echo` command, and prints the output. It connects to `localhost:8080` by default and does not require an API key.

![Echo screenshot](./screenshot.jpg)

## Environment Variables

- `SANDBOX_DOMAIN`: Sandbox service address (default: `localhost:8080`)
- `SANDBOX_API_KEY`: API key if your server requires authentication
- `SANDBOX_IMAGE`: Docker image to use (default: `ubuntu:22.04`)
