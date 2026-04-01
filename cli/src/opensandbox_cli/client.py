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

"""SDK client factory stored in Click context."""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from datetime import timedelta
from typing import Any

import click
import httpx
from opensandbox.config.connection_sync import ConnectionConfigSync
from opensandbox.models.sandboxes import SandboxFilter
from opensandbox.sync.manager import SandboxManagerSync
from opensandbox.sync.sandbox import SandboxSync

from opensandbox_cli.output import OutputFormatter

# Full UUID pattern: 8-4-4-4-12 hex characters
_UUID_RE = re.compile(
    r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"
)


@dataclass
class ClientContext:
    """Shared context passed via ``ctx.obj`` to all Click commands."""

    resolved_config: dict[str, Any]
    output: OutputFormatter
    _connection_config: ConnectionConfigSync | None = field(
        default=None, init=False, repr=False
    )
    _manager: SandboxManagerSync | None = field(
        default=None, init=False, repr=False
    )
    _devops_client: httpx.Client | None = field(
        default=None, init=False, repr=False
    )

    @property
    def connection_config(self) -> ConnectionConfigSync:
        if self._connection_config is None:
            cfg = self.resolved_config
            self._connection_config = ConnectionConfigSync(
                api_key=cfg.get("api_key"),
                domain=cfg.get("domain"),
                protocol=cfg.get("protocol", "http"),
                request_timeout=timedelta(seconds=cfg.get("request_timeout", 30)),
                use_server_proxy=cfg.get("use_server_proxy", False),
            )
        return self._connection_config

    def get_devops_client(self) -> httpx.Client:
        """Return a cached HTTP client for experimental diagnostics endpoints."""
        if self._devops_client is None:
            config = self.connection_config
            headers = dict(config.headers)
            headers.setdefault("Accept", "text/plain")
            headers.setdefault("User-Agent", config.user_agent)
            if config.api_key:
                headers["OPEN-SANDBOX-API-KEY"] = config.api_key

            self._devops_client = httpx.Client(
                base_url=config.get_base_url(),
                headers=headers,
                timeout=config.request_timeout.total_seconds(),
            )
        return self._devops_client

    def get_manager(self) -> SandboxManagerSync:
        """Return a lazily-created ``SandboxManagerSync``."""
        if self._manager is None:
            self._manager = SandboxManagerSync.create(self.connection_config)
        return self._manager

    def resolve_sandbox_id(self, prefix: str) -> str:
        """Resolve a sandbox ID prefix to the full ID (Docker-style).

        If *prefix* looks like a complete UUID, it is returned as-is without
        querying the server.  Otherwise **all pages** of sandboxes are fetched
        so that prefix collisions on later pages are never missed.
        """
        # Skip resolution for full UUIDs
        if _UUID_RE.match(prefix):
            return prefix

        mgr = self.get_manager()
        matches: list[str] = []
        page = 0

        while True:
            result = mgr.list_sandbox_infos(
                SandboxFilter(page=page, page_size=100)
            )
            if result.sandbox_infos:
                matches.extend(
                    info.id
                    for info in result.sandbox_infos
                    if info.id.startswith(prefix)
                )
            # Stop early if we already found >1 match (ambiguous)
            if len(matches) > 1:
                break
            if not result.pagination.has_next_page:
                break
            page += 1

        if len(matches) == 1:
            return matches[0]
        elif len(matches) == 0:
            raise click.ClickException(
                f"No sandbox found with ID prefix '{prefix}'"
            )
        else:
            ids_str = ", ".join(matches[:5])
            if len(matches) > 5:
                ids_str += ", ..."
            raise click.ClickException(
                f"Ambiguous ID prefix '{prefix}' matches {len(matches)} sandboxes: {ids_str}"
            )

    def connect_sandbox(
        self, sandbox_id: str, *, skip_health_check: bool = True
    ) -> SandboxSync:
        """Connect to an existing sandbox by ID (supports prefix matching)."""
        sandbox_id = self.resolve_sandbox_id(sandbox_id)
        return SandboxSync.connect(
            sandbox_id,
            connection_config=self.connection_config,
            skip_health_check=skip_health_check,
        )

    def close(self) -> None:
        """Release resources."""
        if self._manager is not None:
            self._manager.close()
            self._manager = None
        if self._devops_client is not None:
            self._devops_client.close()
            self._devops_client = None
        if self._connection_config is not None:
            self._connection_config.close_transport_if_owned()
            self._connection_config = None
