# Troubleshooting

English | [中文](TROUBLESHOOTING_zh.md)

## `exec /opt/opensandbox/bootstrap.sh: operation not permitted`

If sandbox logs show:

```text
exec /opt/opensandbox/bootstrap.sh: operation not permitted
```

check the following first:

1. Verify the script exists and is executable inside the sandbox container:
   ```bash
   docker exec -it <sandbox-container> ls -l /opt/opensandbox/bootstrap.sh
   ```
2. Verify runtime security/mount constraints are not blocking execution (for example strict
   confinement or `noexec` mount behavior in host/container runtime setup).
3. If you are running Docker from Snap-based environments (for example Ubuntu Core), prefer
   Docker CE package deployments for production OpenSandbox workloads, because strict runtime
   confinement may block this bootstrap execution path in some setups.
4. Re-run with the latest server and execd images to ensure you include the latest runtime fixes.

If this still reproduces, collect:
- `docker info`
- `docker logs opensandbox-server`
- `docker logs <sandbox-container>`
- your `config.toml` (mask secrets)
