#
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
#
"""
Base class for E2E tests providing common setup and configuration.
"""

import os
from datetime import timedelta

import httpx
from opensandbox.config import ConnectionConfig, ConnectionConfigSync

DEFAULT_DOMAIN = "localhost:8080"
DEFAULT_PROTOCOL = "http"
DEFAULT_API_KEY = "e2e-test"
DEFAULT_IMAGE = "sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:latest"

TEST_DOMAIN = os.getenv("OPENSANDBOX_TEST_DOMAIN", DEFAULT_DOMAIN)
TEST_PROTOCOL = os.getenv("OPENSANDBOX_TEST_PROTOCOL", DEFAULT_PROTOCOL)
TEST_API_KEY = os.getenv("OPENSANDBOX_TEST_API_KEY", DEFAULT_API_KEY)
TEST_IMAGE = os.getenv("OPENSANDBOX_SANDBOX_DEFAULT_IMAGE", DEFAULT_IMAGE)


def get_sandbox_image() -> str:
    """Get the default sandbox image for E2E tests."""
    return TEST_IMAGE


def create_connection_config() -> ConnectionConfig:
    """Create async ConnectionConfig for E2E tests."""
    return ConnectionConfig(
        domain=TEST_DOMAIN,
        api_key=TEST_API_KEY,
        request_timeout=timedelta(minutes=3),
        protocol=TEST_PROTOCOL,
    )


def create_connection_config_sync() -> ConnectionConfigSync:
    """Create sync ConnectionConfig for E2E tests."""
    return ConnectionConfigSync(
        domain=TEST_DOMAIN,
        api_key=TEST_API_KEY,
        request_timeout=timedelta(minutes=3),
        transport=httpx.HTTPTransport(
            limits=httpx.Limits(
                max_connections=100,
                max_keepalive_connections=20,
                keepalive_expiry=15,
            )
        ),
        protocol=TEST_PROTOCOL,
    )
