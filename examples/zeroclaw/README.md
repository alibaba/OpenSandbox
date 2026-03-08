# ZeroClaw Gateway Example

Launch a [ZeroClaw](https://github.com/zeroclaw-labs/zeroclaw) Gateway inside an OpenSandbox instance and expose its HTTP endpoint. The script polls the gateway health check until it returns HTTP 200, then prints the reachable endpoint.

This example uses a thin wrapper image built from the official ZeroClaw image. The wrapper adds the shell/runtime pieces that OpenSandbox's Docker bootstrap path expects, while still using the upstream `zeroclaw` binary.

## Build the ZeroClaw Sandbox Image

Build the local wrapper image:

```shell
cd examples/zeroclaw
./build.sh
```

By default, this builds `opensandbox/zeroclaw:latest`. You can override the image name with `IMAGE=...` or `TAG=...`.

## Start OpenSandbox server [local]

The wrapper image copies the official ZeroClaw binary from `ghcr.io/zeroclaw-labs/zeroclaw:latest`.

### Notes (Docker runtime requirement)

The server uses `runtime.type = "docker"` by default, so it **must** be able to reach a running Docker daemon.

- **Docker Desktop**: ensure Docker Desktop is running, then verify with `docker version`.
- **Colima (macOS)**: start it first (`colima start`) and export the socket before starting the server:

```shell
export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
```

Pre-pull the upstream ZeroClaw image if you want to warm the cache before building:

```shell
docker pull ghcr.io/zeroclaw-labs/zeroclaw:latest
```

Start the OpenSandbox server (logs will stay in the terminal):

```shell
uv pip install opensandbox-server
opensandbox-server init-config ~/.sandbox.toml --example docker
opensandbox-server
```

If you see errors like `FileNotFoundError: [Errno 2] No such file or directory` from `docker/transport/unixconn.py`, it usually means the Docker unix socket is missing or Docker is not running.

## Create and Access the ZeroClaw Sandbox

This example is hard-coded for a quick start:
- OpenSandbox server: `http://localhost:8080`
- Image: `opensandbox/zeroclaw:latest`
- Gateway port: `42617`
- Timeout: `3600s`
- Command: `zeroclaw gateway --host 0.0.0.0 --port 42617`
- Env: `ZEROCLAW_ALLOW_PUBLIC_BIND=true`

Install dependencies from the project root:

```shell
uv pip install opensandbox requests
```

Run the example:

```shell
uv run python examples/zeroclaw/main.py
```

Or override the image name:

```shell
export ZEROCLAW_SANDBOX_IMAGE=opensandbox/zeroclaw:latest
uv run python examples/zeroclaw/main.py
```

You should see output similar to:

```text
Creating zeroclaw sandbox with image=opensandbox/zeroclaw:latest on OpenSandbox server http://localhost:8080...
[check] sandbox ready after 0.8s
Zeroclaw gateway started. Please refer to 127.0.0.1:56234/proxy/42617
```

The endpoint printed at the end (for example, `127.0.0.1:56234/proxy/42617`) is the ZeroClaw Gateway address exposed from the sandbox. The readiness probe in this example is `GET /health`.

## References
- [ZeroClaw](https://github.com/zeroclaw-labs/zeroclaw)
- [OpenSandbox Python SDK](https://pypi.org/project/opensandbox/)
