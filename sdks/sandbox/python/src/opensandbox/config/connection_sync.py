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
Synchronous connection configuration for OpenSandbox SDK.

This mirrors ConnectionConfig (async) but uses httpx sync transports.
"""

import os
from datetime import timedelta

import httpx
from pydantic import BaseModel, ConfigDict, Field, PrivateAttr, field_validator


class ConnectionConfigSync(BaseModel):
    """
    Synchronous connection configuration shared across all sync SDK HTTP clients.

    Ownership rules:
    - If `transport` is not provided, the SDK creates a default HTTPTransport and will close it.
    - If `transport` is provided, the SDK will NOT close it (user owns it).
    """

    model_config = ConfigDict(arbitrary_types_allowed=True)

    _owns_transport: bool = PrivateAttr(default=True)

    api_key: str | None = Field(
        default=None, description="API key for authentication with sandbox service"
    )
    domain: str | None = Field(
        default=None, description="Base domain for the sandbox management API"
    )
    protocol: str = Field(default="http", description="Protocol to use (http/https)")
    request_timeout: timedelta = Field(
        default=timedelta(seconds=30),
        description="Timeout for HTTP requests to the management API",
    )
    debug: bool = Field(default=False, description="Enable debug logging for HTTP requests")
    user_agent: str = Field(
        default="OpenSandbox-Python-SDK/0.1.0", description="User agent string"
    )
    headers: dict[str, str] = Field(default_factory=dict, description="User defined headers")

    transport: httpx.BaseTransport = Field(
        default_factory=httpx.HTTPTransport,
        description=(
            "Shared httpx transport instance used by all sync SDK HTTP clients. "
            "Pass a custom transport (e.g. HTTPTransport with custom limits/proxies) "
            "to control connection pooling, proxies, retries, etc."
        ),
    )

    _ENV_API_KEY = "OPEN_SANDBOX_API_KEY"
    _ENV_DOMAIN = "OPEN_SANDBOX_DOMAIN"
    _DEFAULT_DOMAIN = "localhost:8080"

    def model_post_init(self, __context: object) -> None:
        self._owns_transport = "transport" not in self.model_fields_set

    def close_transport_if_owned(self) -> None:
        """Close the transport only if it was created by default_factory."""
        if not self._owns_transport:
            return
        try:
            self.transport.close()
        except Exception:
            pass

    @field_validator("protocol")
    @classmethod
    def protocol_must_be_valid(cls, v: str) -> str:
        v = v.lower()
        if v not in ("http", "https"):
            raise ValueError("Protocol must be 'http' or 'https'")
        return v

    @field_validator("request_timeout")
    @classmethod
    def timeout_must_be_positive(cls, v: timedelta) -> timedelta:
        if v.total_seconds() <= 0:
            raise ValueError(f"Request timeout must be positive, got: {v}")
        return v

    def get_api_key(self) -> str:
        return self.api_key or os.getenv(self._ENV_API_KEY, "")

    def get_domain(self) -> str:
        return self.domain or os.getenv(self._ENV_DOMAIN, self._DEFAULT_DOMAIN)

    def get_base_url(self) -> str:
        domain = self.get_domain()
        if domain.startswith("http://") or domain.startswith("https://"):
            return domain
        return f"{self.protocol}://{domain}"
