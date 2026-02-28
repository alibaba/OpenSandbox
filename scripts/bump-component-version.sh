#!/bin/bash
# Copyright 2026 Alibaba Group Holding Ltd.
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

# Bump egress or execd image version across the entire project.
# Usage: from repo root:
#   ./scripts/bump-component-version.sh egress v1.0.2
#   ./scripts/bump-component-version.sh execd v1.0.7
#   ./scripts/bump-component-version.sh v1.0.2              # same as: egress v1.0.2

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Parse args: [egress|execd] NEW_VERSION  or  NEW_VERSION (default egress)
COMPONENT=""
NEW_VERSION=""
if [ $# -eq 1 ]; then
  COMPONENT="egress"
  NEW_VERSION="$1"
elif [ $# -eq 2 ]; then
  COMPONENT="$1"
  NEW_VERSION="$2"
else
  echo "Usage: $0 [egress|execd] NEW_VERSION" >&2
  echo "       $0 NEW_VERSION   # bumps egress" >&2
  echo "Example: $0 egress v1.0.2" >&2
  echo "Example: $0 execd 1.0.7" >&2
  exit 1
fi

case "$COMPONENT" in
  egress|execd|code-interpreter) ;;
  *)
    echo "Error: unsupported component: $COMPONENT" >&2
    exit 0
    ;;
esac

# Normalize version: ensure 'v' prefix
if [[ ! "$NEW_VERSION" =~ ^v ]]; then
  NEW_VERSION="v${NEW_VERSION}"
fi

# Pattern and replacement for this component (e.g. egress:vX.Y.Z -> egress:NEW_VERSION)
PATTERN="${COMPONENT}:v[0-9]+\.[0-9]+\.[0-9]+"
REPLACEMENT="${COMPONENT}:${NEW_VERSION}"

files=()
while IFS= read -r f; do
  [ -n "$f" ] && files+=("$f")
done < <(grep -rEl --exclude-dir=.git --exclude-dir=__pycache__ --exclude-dir=.venv --exclude-dir=node_modules "$PATTERN" . 2>/dev/null || true)

updated=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  if perl -i -pe "s/$PATTERN/$REPLACEMENT/g" "$f" 2>/dev/null; then
    echo "Updated $f"
    ((updated++)) || true
  fi
done

if [ "$updated" -eq 0 ]; then
  echo "No files were updated (no matches for $PATTERN)." >&2
  exit 1
fi

echo "Done. Bumped $COMPONENT version to $NEW_VERSION in $updated file(s)."
