#!/usr/bin/env bash
set -euo pipefail

echo "Disk usage before Docker cleanup:"
df -h /

docker ps -aq | xargs -r docker rm -f || true
docker builder prune -af || true
docker system prune -af --volumes || true
rm -rf "${HOME:-/home/admin}/.docker/buildx/activity"/* || true

echo "Disk usage after Docker cleanup:"
df -h /
