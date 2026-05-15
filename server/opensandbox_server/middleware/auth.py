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

"""
Authentication middleware for OpenSandbox Lifecycle API.

API keys are validated against the OPEN-SANDBOX-API-KEY header.
In multi-tenant mode (tenant_loader provided), delegates key lookup to
TenantLoader and injects TenantEntry into request.state.tenant.
"""

from __future__ import annotations

import contextvars
import re
import secrets
from typing import Callable, Optional

from fastapi import Request, Response, status
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

from opensandbox_server.config import AppConfig, get_config
from opensandbox_server.tenants.models import TenantEntry

SANDBOX_API_KEY_HEADER = "OPEN-SANDBOX-API-KEY"

_current_tenant: contextvars.ContextVar[TenantEntry | None] = contextvars.ContextVar(
    "current_tenant", default=None
)


def get_current_tenant() -> TenantEntry | None:
    return _current_tenant.get()


class AuthMiddleware(BaseHTTPMiddleware):

    EXEMPT_PATHS = ["/health", "/docs", "/redoc", "/openapi.json"]
    _PROXY_PATH_RE = re.compile(r"^(/v1)?/sandboxes/[^/]+/proxy/\d+(/|$)")

    @staticmethod
    def _is_proxy_path(path: str) -> bool:
        if ".." in path:
            return False
        return bool(AuthMiddleware._PROXY_PATH_RE.match(path))

    def __init__(self, app, config: Optional[AppConfig] = None, tenant_loader=None):
        super().__init__(app)
        self.config = config or get_config()
        self.tenant_loader = tenant_loader
        self._valid_api_keys: dict[str, None] = self._load_api_keys()

    def _load_api_keys(self) -> dict[str, None]:
        if self.tenant_loader is not None:
            if self.config.server.api_key:
                raise SystemExit(
                    "server.api_key must not be set when tenants.toml is present. "
                    "Migrate the key into tenants.toml."
                )
            return {}
        key = self.config.server.api_key
        if key and key.strip():
            return {key: None}
        return {}

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        if any(request.url.path.startswith(path) for path in self.EXEMPT_PATHS):
            return await call_next(request)

        if self._is_proxy_path(request.url.path):
            return await call_next(request)

        api_key = request.headers.get(SANDBOX_API_KEY_HEADER)

        if self.tenant_loader is not None:
            if not api_key:
                return self._missing_api_key_response()
            tenant = self.tenant_loader.lookup(api_key)
            if tenant is None:
                return self._invalid_api_key_response()
            request.state.tenant = tenant
            _current_tenant.set(tenant)
            return await call_next(request)

        if not self._valid_api_keys:
            return await call_next(request)

        if not api_key:
            return self._missing_api_key_response()

        if not any(secrets.compare_digest(k, api_key) for k in self._valid_api_keys):
            return self._invalid_api_key_response()

        request.state.tenant = None
        _current_tenant.set(None)
        return await call_next(request)

    @staticmethod
    def _missing_api_key_response() -> JSONResponse:
        return JSONResponse(
            status_code=status.HTTP_401_UNAUTHORIZED,
            content={
                "code": "MISSING_API_KEY",
                "message": (
                    "Authentication credentials are missing. "
                    f"Provide API key via {SANDBOX_API_KEY_HEADER} header."
                ),
            },
        )

    @staticmethod
    def _invalid_api_key_response() -> JSONResponse:
        return JSONResponse(
            status_code=status.HTTP_401_UNAUTHORIZED,
            content={
                "code": "INVALID_API_KEY",
                "message": "Authentication credentials are invalid. Check your API key and try again.",
            },
        )
