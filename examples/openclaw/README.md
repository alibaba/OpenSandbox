# OpenClaw Gateway Example

Launch an [OpenClaw](https://github.com/openclaw/openclaw) Gateway inside an OpenSandbox instance and expose its HTTP endpoint. The script polls the gateway until it returns HTTP 200, then prints the reachable endpoint.

## Start OpenSandbox server [local]

You can find the latest OpenClaw container image [here](https://github.com/openclaw/openclaw/pkgs/container/openclaw).

### Notes (Docker runtime requirement)

The server uses `runtime.type = "docker"` by default, so it **must** be able to reach a running Docker daemon.

- **Docker Desktop**: ensure Docker Desktop is running, then verify with `docker version`.
- **Colima (macOS)**: start it first (`colima start`) and export the socket before starting the server:

```shell
export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
```

Pre-pull the OpenClaw image:

```shell
docker pull ghcr.io/openclaw/openclaw:latest
```

Start the OpenSandbox server (logs will stay in the terminal):

```shell
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker
opensandbox-server
```

If you see errors like `FileNotFoundError: [Errno 2] No such file or directory` from `docker/transport/unixconn.py`, it usually means the Docker unix socket is missing or Docker is not running.

## Create and Access the OpenClaw Sandbox

This example is hard-coded for a quick start:
- OpenSandbox server: `http://localhost:8080`
- Image: `ghcr.io/openclaw/openclaw:latest`
- Gateway port: `18789`
- Timeout: `3600s`
- Token: `OPENCLAW_GATEWAY_TOKEN` (default: `dummy-token-for-sandbox`)

Install dependencies from the project root:

```shell
uv pip install opensandbox requests
```

Run the example (set a real token if you need authenticated access):

```shell
export OPENCLAW_GATEWAY_TOKEN="$(openssl rand -hex 32)"
uv run python examples/openclaw/main.py
```

You should see output similar to:

```text
Creating openclaw sandbox with image=ghcr.io/openclaw/openclaw:latest on OpenSandbox server http://localhost:8080...
[check] sandbox ready after 7.1s
Openclaw started finished. Please refer to 127.0.0.1:56123
```

The endpoint printed at the end (e.g., `127.0.0.1:56123`) is the OpenClaw Gateway address exposed from the sandbox.

## References
- [OpenClaw](https://github.com/openclaw/openclaw)
- [OpenSandbox Python SDK](https://pypi.org/project/opensandbox/)
