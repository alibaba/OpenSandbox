#!/bin/bash
# SPDX-FileCopyrightText: 2025 Weibo, Inc.
#
# SPDX-License-Identifier: Apache-2.0

set -e

echo "[entrypoint] USERNAME=${USERNAME} SESSION_ID=${SESSION_ID}"

# Ensure Node.js is in PATH using the default version bundled in the base image
source /opt/opensandbox/code-interpreter-env.sh node

SESSION_WORKSPACE=""

# mount via orangefs — optional, only attempted when the binary is present
# and both USERNAME and SESSION_ID are set.
if [ -x /usr/local/bin/orangefs ] && [ -n "${USERNAME:-}" ] && [ -n "${SESSION_ID:-}" ]; then
  MOUNT_PATH="/workspace/${USERNAME}/${SESSION_ID}"
  echo "[entrypoint] creating mount path: ${MOUNT_PATH}"
  mkdir -p "$MOUNT_PATH"

  echo "[entrypoint] starting orangefs mount..."
  nohup /usr/local/bin/orangefs posix mount \
    --rs-addr="${ORANGEFS_RS_ADDR:-}" \
    --token="${ORANGEFS_TOKEN:-}" \
    --volume-name="${ORANGEFS_VOLUME:-}" \
    --subpath="${USERNAME}/${SESSION_ID}" \
    --mount-point="$MOUNT_PATH" > /tmp/orangefs.log 2>&1 &

  echo "[entrypoint] waiting for mount to be ready..."
  MOUNT_TIMEOUT=30
  MOUNT_ELAPSED=0
  while ! df -h 2>/dev/null | grep -q "$MOUNT_PATH"; do
    if [ "$MOUNT_ELAPSED" -ge "$MOUNT_TIMEOUT" ]; then
      break
    fi
    sleep 1
    MOUNT_ELAPSED=$((MOUNT_ELAPSED + 1))
  done

  if df -h | grep -q "$MOUNT_PATH"; then
    echo "[entrypoint] mount successful: ${MOUNT_PATH}"
    df -h
    SESSION_WORKSPACE="$MOUNT_PATH"
  else
    echo "[entrypoint] mount FAILED — orangefs log:"
    cat /tmp/orangefs.log
  fi
else
  echo "[entrypoint] skipping orangefs mount — USERNAME or SESSION_ID not set, using /workspace directly"
fi

# --- Claude Code profile init ---
# If CLAUDE_PROFILE_PATH points to a mounted user-profile volume, link the user-level
# config files (/root/.claude.json, settings.json, CLAUDE.md) into the container.
# The projects/ directory (session conversation history) is handled separately below.
if [ -n "${CLAUDE_PROFILE_PATH:-}" ] && [ -d "${CLAUDE_PROFILE_PATH}" ]; then
  echo "[entrypoint] linking Claude Code profile from ${CLAUDE_PROFILE_PATH}"
  mkdir -p "${CLAUDE_PROFILE_PATH}/.claude" /root/.claude
  if [ -f "${CLAUDE_PROFILE_PATH}/.claude.json" ]; then
    ln -sf "${CLAUDE_PROFILE_PATH}/.claude.json" /root/.claude.json
  fi
  for f in settings.json CLAUDE.md; do
    if [ -f "${CLAUDE_PROFILE_PATH}/.claude/${f}" ]; then
      ln -sf "${CLAUDE_PROFILE_PATH}/.claude/${f}" "/root/.claude/${f}"
    fi
  done
else
  echo "[entrypoint] CLAUDE_PROFILE_PATH not set or missing — Claude Code config will not persist across container restarts"
fi

# Bootstrap /root/.claude.json from ANTHROPIC_API_KEY when no profile file exists,
# so first-run auth still works from the environment variable.
if [ -n "${ANTHROPIC_API_KEY:-}" ] && [ ! -f /root/.claude.json ]; then
  echo "[entrypoint] bootstrapping /root/.claude.json from ANTHROPIC_API_KEY"
  echo '{}' > /root/.claude.json
fi

# --- Claude Code session history ---
# Redirect /root/.claude/projects/ into the session workspace so that conversation
# history survives container destruction and can be recovered on resume (same SESSION_ID).
if [ -n "${SESSION_WORKSPACE}" ]; then
  PROJECTS_DIR="${SESSION_WORKSPACE}/.claude-projects"
  mkdir -p "$PROJECTS_DIR" /root/.claude
  rm -rf /root/.claude/projects
  ln -sf "$PROJECTS_DIR" /root/.claude/projects
  echo "[entrypoint] Claude Code projects linked to session workspace: ${PROJECTS_DIR}"
fi

echo "[entrypoint] starting claude-agent-server on port ${PORT:-3000}..."
exec node /app/dist/server.js
