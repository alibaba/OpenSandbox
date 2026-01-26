# Agent-Sandbox Example

This example creates a sandbox backed by `kubernetes-sigs/agent-sandbox` and
executes `echo hello world` via the OpenSandbox Python SDK.

## Prerequisites

- A Kubernetes cluster with the agent-sandbox controller and CRDs installed.
- OpenSandbox server configured with Kubernetes runtime and `workload_provider = "agent-sandbox"`.
- Sandbox image should include `bash` (default example uses `ubuntu:22.04`).

## Start OpenSandbox server

1. Copy the example config and edit it for agent-sandbox:

```shell
cd server
cp example.config.toml ~/.sandbox.toml
```

2. Update `~/.sandbox.toml` with the following sections:

```toml
[runtime]
type = "kubernetes"
execd_image = "opensandbox/execd:latest"

[kubernetes]
namespace = "default"
# kubeconfig_path = "/absolute/path/to/kubeconfig"  # optional if running in-cluster
workload_provider = "agent-sandbox"

[agent_sandbox]
shutdown_policy = "Delete"
```

3. Start the server:

```shell
cd server
uv sync && uv run python -m src.main
```

## Run the example

```shell
uv pip install opensandbox
uv run python examples/agent-sandbox/main.py
```

## Expected output

```text
command output: hello world
```
