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

set -ex

TAG=${TAG:-latest}
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || realpath "$(dirname "$0")/../..")
cd "${REPO_ROOT}"

docker buildx rm egress-builder || true
docker buildx create --use --name egress-builder
docker buildx inspect --bootstrap
docker buildx ls

docker buildx build \
  -t opensandbox/egress:${TAG} \
  -t sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/egress:${TAG} \
  -f components/egress/Dockerfile \
  --platform linux/amd64,linux/arm64 \
  --push \
  .
