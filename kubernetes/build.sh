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

set -e

# Default values
TAG=${TAG:-latest}
COMPONENT=${COMPONENT:-controller}
PUSH=${PUSH:-true}

# Image repository
ACR_REPO="sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox"

# Component specific settings
if [ "$COMPONENT" == "controller" ]; then
    IMAGE_NAME="controller"
    BUILD_ARG="--build-arg PACKAGE=cmd/controller/main.go"
elif [ "$COMPONENT" == "task-executor" ]; then
    IMAGE_NAME="task-executor"
    BUILD_ARG="--build-arg PACKAGE=cmd/task-executor/main.go --build-arg USERID=0"
else
    echo "Error: Unknown component: $COMPONENT"
    echo "Available components: controller, task-executor"
    exit 1
fi

echo "========================================="
echo "Building $COMPONENT"
echo "Image: $IMAGE_NAME"
echo "Tag: $TAG"
echo "Push: $PUSH"
echo "========================================="

# Build for multiple platforms
PLATFORMS="linux/amd64,linux/arm64"

if [ "$PUSH" == "true" ]; then
    # Build and push to ACR registry
    docker buildx build \
        --platform $PLATFORMS \
        $BUILD_ARG \
        -t ${ACR_REPO}/${IMAGE_NAME}:${TAG} \
        --push \
        -f Dockerfile \
        .
    
    echo "========================================="
    echo "Successfully built and pushed:"
    echo "  ${ACR_REPO}/${IMAGE_NAME}:${TAG}"
    echo "========================================="
else
    # Build only (for local testing)
    docker buildx build \
        --platform linux/amd64 \
        $BUILD_ARG \
        -t ${IMAGE_NAME}:${TAG} \
        -f Dockerfile \
        --load \
        .
    
    echo "========================================="
    echo "Successfully built (local only):"
    echo "  ${IMAGE_NAME}:${TAG}"
    echo "========================================="
fi
