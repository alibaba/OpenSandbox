#!/bin/sh

# Copyright 2025 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

EXECD="${EXECD:=/opt/opensandbox/execd}"

if [ -z "${EXECD_ENVS:-}" ]; then
	EXECD_ENVS="/opt/opensandbox/.env"
fi
# Best-effort ensure file exists.
if ! mkdir -p "$(dirname "$EXECD_ENVS")" 2>/dev/null; then
	echo "warning: failed to create dir for EXECD_ENVS=$EXECD_ENVS" >&2
fi
if ! touch "$EXECD_ENVS" 2>/dev/null; then
	echo "warning: failed to touch EXECD_ENVS=$EXECD_ENVS" >&2
fi
export EXECD_ENVS

# Fix for Docker 18.09 / old glibc compatibility (Issue #508)
export LD_ASSUME_KERNEL=2.33.0

echo "starting OpenSandbox Execd daemon at $EXECD."
$EXECD &

# Allow chained shell commands (e.g., /test1.sh && /test2.sh)
# Usage:
#   bootstrap.sh -c "/test1.sh && /test2.sh"
# Or set BOOTSTRAP_CMD="/test1.sh && /test2.sh"
CMD=""
if [ "${BOOTSTRAP_CMD:-}" != "" ]; then
	CMD="$BOOTSTRAP_CMD"
elif [ $# -ge 1 ] && [ "$1" = "-c" ]; then
	shift
	CMD="$*"
fi

SHELL_BIN="${BOOTSTRAP_SHELL:-}"
if [ -z "$SHELL_BIN" ]; then
	if command -v bash >/dev/null 2>&1; then
		SHELL_BIN="$(command -v bash)"
	elif command -v sh >/dev/null 2>&1; then
		SHELL_BIN="$(command -v sh)"
	else
		echo "error: neither bash nor sh found in PATH" >&2
		exit 1
	fi
fi

set -x
if [ "$CMD" != "" ]; then
	exec "$SHELL_BIN" -c "$CMD"
fi

if [ $# -eq 0 ]; then
	exec "$SHELL_BIN"
fi

exec "$@"
