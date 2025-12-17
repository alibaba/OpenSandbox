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

"""Shared constants for sandbox services."""

SANDBOX_ID_LABEL = "sandbox.dev/id"
SANDBOX_EXPIRES_AT_LABEL = "sandbox.dev/expiresAt"
# Host-mapped ports recorded on containers (bridge mode).
SANDBOX_EMBEDDING_PROXY_PORT_LABEL = "sandbox.dev/embeddingProxyPort"  # maps container 44772 -> host port
SANDBOX_HTTP_PORT_LABEL = "sandbox.dev/httpPort"  # maps container 8080 -> host port


class SandboxErrorCodes:
    """Canonical error codes for sandbox service operations."""

    DOCKER_INITIALIZATION_ERROR = "DOCKER::INITIALIZATION_ERROR"
    CONTAINER_QUERY_FAILED = "DOCKER::SANDBOX_QUERY_FAILED"
    SANDBOX_NOT_FOUND = "DOCKER::SANDBOX_NOT_FOUND"
    IMAGE_PULL_FAILED = "DOCKER::SANDBOX_IMAGE_PULL_FAILED"
    CONTAINER_START_FAILED = "DOCKER::SANDBOX_START_FAILED"
    SANDBOX_DELETE_FAILED = "DOCKER::SANDBOX_DELETE_FAILED"
    SANDBOX_NOT_RUNNING = "DOCKER::SANDBOX_NOT_RUNNING"
    SANDBOX_PAUSE_FAILED = "DOCKER::SANDBOX_PAUSE_FAILED"
    SANDBOX_NOT_PAUSED = "DOCKER::SANDBOX_NOT_PAUSED"
    SANDBOX_RESUME_FAILED = "DOCKER::SANDBOX_RESUME_FAILED"
    INVALID_EXPIRATION = "DOCKER::INVALID_EXPIRATION"
    EXPIRATION_NOT_EXTENDED = "DOCKER::EXPIRATION_NOT_EXTENDED"
    EXECD_START_FAILED = "DOCKER::SANDBOX_EXECD_START_FAILED"
    EXECD_DISTRIBUTION_FAILED = "DOCKER::SANDBOX_EXECD_DISTRIBUTION_FAILED"
    BOOTSTRAP_INSTALL_FAILED = "DOCKER::SANDBOX_BOOTSTRAP_INSTALL_FAILED"
    INVALID_ENTRYPOINT = "DOCKER::INVALID_ENTRYPOINT"
    INVALID_PORT = "DOCKER::INVALID_PORT"
    INVALID_METADATA_LABEL = "SANDBOX::INVALID_METADATA_LABEL"
    NETWORK_MODE_ENDPOINT_UNAVAILABLE = "DOCKER::NETWORK_MODE_ENDPOINT_UNAVAILABLE"


__all__ = [
    "SANDBOX_ID_LABEL",
    "SANDBOX_EXPIRES_AT_LABEL",
    "SANDBOX_EMBEDDING_PROXY_PORT_LABEL",
    "SANDBOX_HTTP_PORT_LABEL",
    "SandboxErrorCodes",
]
