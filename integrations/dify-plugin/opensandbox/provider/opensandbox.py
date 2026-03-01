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

from __future__ import annotations

import os
from typing import Any

from dify_plugin import ToolProvider
from dify_plugin.errors.tool import ToolProviderCredentialValidationError
from opensandbox.config.connection_sync import ConnectionConfigSync
from opensandbox.models.sandboxes import SandboxFilter
from opensandbox.sync.manager import SandboxManagerSync


def _normalize_domain(base_url: str) -> str:
    base_url = base_url.strip().rstrip("/")
    if base_url.endswith("/v1"):
        base_url = base_url[:-3]
    return base_url


class OpenSandboxProvider(ToolProvider):
    def _validate_credentials(self, credentials: dict[str, Any]) -> None:
        # Try credentials first, then fall back to environment variables
        base_url = credentials.get("opensandbox_base_url", "") or os.environ.get("OPENSANDBOX_BASE_URL", "")
        api_key = credentials.get("opensandbox_api_key", "") or os.environ.get("OPENSANDBOX_API_KEY", "")
        if not base_url or not api_key:
            raise ToolProviderCredentialValidationError(
                "Missing OpenSandbox base URL or API key. "
                "Provide via credentials or OPENSANDBOX_BASE_URL/OPENSANDBOX_API_KEY env vars."
            )

        config = ConnectionConfigSync(
            domain=_normalize_domain(base_url),
            api_key=api_key,
        )

        manager = None
        try:
            manager = SandboxManagerSync.create(connection_config=config)
            _ = manager.list_sandbox_infos(SandboxFilter(page_size=1))
        except Exception as exc:
            raise ToolProviderCredentialValidationError(str(exc)) from exc
        finally:
            try:
                if manager is not None:
                    manager.close()
            except Exception:
                pass