#!/bin/bash
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

set -ex

TAG=${TAG:-latest}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
GIT_COMMIT=${GIT_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo "unknown")}
BUILD_TIME=${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || realpath "$(dirname "$0")/../..")
cd "${REPO_ROOT}"

docker buildx rm ingress-builder || true

docker buildx create --use --name ingress-builder

docker buildx inspect --bootstrap

docker buildx ls

docker buildx build \
  -t opensandbox/ingress:${TAG} \
  -t sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/ingress:${TAG} \
  -f components/ingress/Dockerfile \
  --build-arg VERSION="${VERSION}" \
  --build-arg GIT_COMMIT="${GIT_COMMIT}" \
  --build-arg BUILD_TIME="${BUILD_TIME}" \
  --platform linux/amd64 \
  --push \
  .
