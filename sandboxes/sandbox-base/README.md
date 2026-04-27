# OpenSandbox Sandbox Base Image

The `sandbox-base` image is the default runtime for OpenSandbox Claude Code sandboxes. It bundles:

- **Claude Code CLI** (`@anthropic-ai/claude-code`) — pre-installed, ready to use
- **Code interpreter** — Python 3.10–3.14, Node.js 18/20/22, multi-version toolchain (see [code-interpreter](../code-interpreter/README.md))
- **claude-agent-server** — HTTP API server at port 3000 for driving Claude Code sessions via REST/SSE
- **OrangeFS client** — optional distributed filesystem for workspace persistence

## Environment Variables

### Core

| Variable | Required | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | Recommended | Anthropic API key. If set and `/root/.claude.json` does not yet exist, the entrypoint creates an empty config so Claude Code's first-run auth picks up the key automatically. |

### Workspace Persistence (OrangeFS)

These three variables enable per-session workspace persistence via OrangeFS. All three must be set together for the mount to proceed; any one missing skips the mount gracefully.

| Variable | Description |
|---|---|
| `USERNAME` | User identifier. Determines the subpath in OrangeFS: `{USERNAME}/{SESSION_ID}`. |
| `SESSION_ID` | Session identifier. Combined with `USERNAME` to form a unique workspace path. |
| `ORANGEFS_RS_ADDR` | OrangeFS registry server address. |
| `ORANGEFS_TOKEN` | OrangeFS authentication token. |
| `ORANGEFS_VOLUME` | OrangeFS volume name. |

When the mount succeeds, the workspace is available at `/workspace/{USERNAME}/{SESSION_ID}`. Claude Code's project conversation history (`/root/.claude/projects/`) is automatically redirected into this path under `.claude-projects/`, so resuming a sandbox with the same `SESSION_ID` restores prior conversation context.

### Claude Code Config Persistence (User Profile)

| Variable | Description |
|---|---|
| `CLAUDE_PROFILE_PATH` | Path inside the container where a user-profile volume is mounted. The entrypoint symlinks `/root/.claude.json`, `settings.json`, and `CLAUDE.md` from this directory so user-level config (auth, preferences, global instructions) survives container replacement. |

The profile volume itself is a standard PVC or Docker named volume mounted by the caller at sandbox creation time.

## Fallback Behaviour

All persistence variables are optional. The table below shows what is lost and what still works when each is omitted.

| What is omitted | Effect |
|---|---|
| None omitted (full config) | Full persistence: auth, preferences, workspace files, and conversation history all survive container destruction. |
| `CLAUDE_PROFILE_PATH` not set | Claude Code config is ephemeral. Auth and preferences reset on every new container. OrangeFS workspace (files) is unaffected. |
| OrangeFS vars (`USERNAME` / `SESSION_ID` / etc.) not set | Workspace uses `/workspace` directly inside the container. Files and conversation history are lost when the container is destroyed. User profile (auth, settings) is unaffected if `CLAUDE_PROFILE_PATH` is set. |
| `ANTHROPIC_API_KEY` not set and no profile | Claude Code runs but prompts for interactive login on first use. |
| All omitted | Fully ephemeral: works for one-off tasks, nothing persists. |

## Creating a Sandbox with Full Persistence

The following example shows a complete `CreateSandboxRequest` body using the OpenSandbox lifecycle API. Replace placeholders with your actual values.

```json
{
  "image": {
    "uri": "opensandbox/sandbox-base:latest"
  },
  "timeout": 3600,
  "env": {
    "ANTHROPIC_API_KEY": "<your-anthropic-api-key>",

    "USERNAME": "<user-id>",
    "SESSION_ID": "<session-id>",
    "ORANGEFS_RS_ADDR": "<orangefs-registry-addr>",
    "ORANGEFS_TOKEN": "<orangefs-token>",
    "ORANGEFS_VOLUME": "<orangefs-volume>",

    "CLAUDE_PROFILE_PATH": "/root/.claude-profile"
  },
  "volumes": [
    {
      "name": "claude-user-profile",
      "mountPath": "/root/.claude-profile",
      "pvc": {
        "claimName": "claude-config-<user-id>",
        "createIfNotExists": true,
        "storage": "1Gi"
      }
    }
  ],
  "entrypoint": ["/entrypoint.sh"]
}
```

### Python SDK example

```python
from opensandbox import Sandbox

sandbox = await Sandbox.create(
    image="opensandbox/sandbox-base:latest",
    connection_config=config,
    env={
        "ANTHROPIC_API_KEY": anthropic_api_key,
        # Session workspace via OrangeFS
        "USERNAME": user_id,
        "SESSION_ID": session_id,
        "ORANGEFS_RS_ADDR": orangefs_addr,
        "ORANGEFS_TOKEN": orangefs_token,
        "ORANGEFS_VOLUME": orangefs_volume,
        # User profile persistence
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

### Minimal (no persistence)

If you only need a one-shot sandbox with no state carried over between runs, no volumes or OrangeFS config is required:

```json
{
  "image": { "uri": "opensandbox/sandbox-base:latest" },
  "timeout": 3600,
  "env": {
    "ANTHROPIC_API_KEY": "<your-anthropic-api-key>"
  },
  "entrypoint": ["/entrypoint.sh"]
}
```

Claude Code will authenticate from `ANTHROPIC_API_KEY` on first run. All state is discarded when the container exits.

## Persistence Architecture

```
/root/.claude-profile/          ← user-profile volume (PVC, per-user)
  .claude.json                  ← auth tokens, model, global settings
  .claude/
    settings.json               ← user preferences
    CLAUDE.md                   ← global instructions

/root/                          ← container-local (ephemeral)
  .claude.json    → symlink → /root/.claude-profile/.claude.json
  .claude/
    settings.json → symlink → /root/.claude-profile/.claude/settings.json
    CLAUDE.md     → symlink → /root/.claude-profile/.claude/CLAUDE.md
    projects/     → symlink → /workspace/<USERNAME>/<SESSION_ID>/.claude-projects/

/workspace/<USERNAME>/<SESSION_ID>/   ← OrangeFS (per-session)
  <your files>
  .claude-projects/                   ← Claude Code conversation history
```

## Port

The claude-agent-server listens on port **3000** by default. Override with `PORT`.
