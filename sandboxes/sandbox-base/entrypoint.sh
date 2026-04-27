#!/bin/bash
# SPDX-FileCopyrightText: 2025 Weibo, Inc.
#
# SPDX-License-Identifier: Apache-2.0

set -e

echo "[entrypoint] USERNAME=${USERNAME} SESSION_ID=${SESSION_ID}"

# Ensure Node.js is in PATH using the default version bundled in the base image
source /opt/opensandbox/code-interpreter-env.sh node

# mount via orangefs — optional, only attempted when the binary is present.
if [ -x /usr/local/bin/orangefs ]; then
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
  sleep 2

  if mountpoint -q "$MOUNT_PATH"; then
    echo "[entrypoint] mount successful: ${MOUNT_PATH}"
  else
    echo "[entrypoint] mount FAILED — orangefs log:"
    cat /tmp/orangefs.log
  fi
fi

echo "[entrypoint] starting claude-agent-server on port ${PORT:-3000}..."
exec node /app/dist/server.js
