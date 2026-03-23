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

"""Server proxy path: background renew attempts with per-sandbox lock and rate limit."""

from __future__ import annotations

import asyncio
import logging
import time
from collections import OrderedDict
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from src.integrations.renew_intent.constants import PROXY_RENEW_MAX_TRACKED_SANDBOXES
from src.integrations.renew_intent.controller import AccessRenewController

if TYPE_CHECKING:
    from src.config import AppConfig
    from src.services.extension_service import ExtensionService
    from src.services.sandbox_service import SandboxService

logger = logging.getLogger(__name__)


@dataclass
class _ProxyRenewState:
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)
    last_success_monotonic: float | None = None


class ProxyRenewCoordinator:
    """Schedule renew attempts for `/sandboxes/{id}/proxy/...` without blocking the proxy."""

    def __init__(
        self,
        app_config: "AppConfig",
        sandbox_service: "SandboxService",
        extension_service: "ExtensionService",
    ) -> None:
        self._app_config = app_config
        self._controller = AccessRenewController(
            app_config,
            sandbox_service,
            extension_service,
            redis=None,
        )
        self._states: OrderedDict[str, _ProxyRenewState] = OrderedDict()
        self._max_tracked = PROXY_RENEW_MAX_TRACKED_SANDBOXES
        self._min_interval = float(app_config.renew_intent.min_interval_seconds)

    def _ensure_mru(self, sandbox_id: str) -> _ProxyRenewState:
        if sandbox_id in self._states:
            st = self._states[sandbox_id]
            self._states.move_to_end(sandbox_id)
        else:
            st = _ProxyRenewState()
            self._states[sandbox_id] = st
            self._states.move_to_end(sandbox_id)
        self._evict_lru_unlocked()
        return st

    def _evict_lru_unlocked(self) -> None:
        """Drop oldest scheduled ids until under cap; skip entries whose lock is held."""
        rotations = 0
        max_rotations = max(len(self._states), 1)
        while len(self._states) > self._max_tracked and rotations < max_rotations:
            k, st = self._states.popitem(last=False)
            if st.lock.locked():
                self._states[k] = st
                self._states.move_to_end(k)
                rotations += 1
            else:
                rotations = 0

    def schedule(self, sandbox_id: str) -> None:
        """If ``renew_intent.enabled``, start a background renew task for this sandbox."""
        if not self._app_config.renew_intent.enabled:
            return
        st = self._ensure_mru(sandbox_id)
        asyncio.create_task(
            self._run(sandbox_id, st),
            name=f"renew_intent_proxy_{sandbox_id}",
        )

    async def _run(self, sandbox_id: str, st: _ProxyRenewState | None = None) -> None:
        if st is None:
            st = self._states.get(sandbox_id) or self._ensure_mru(sandbox_id)
        try:
            async with st.lock:
                now = time.monotonic()
                last = st.last_success_monotonic
                if last is not None and (now - last) < self._min_interval:
                    logger.debug(
                        "renew_intent: proxy skip min_interval sandbox=%s (%.1fs < %.1fs)",
                        sandbox_id,
                        now - last,
                        self._min_interval,
                    )
                    return

                ok = await asyncio.to_thread(self._controller.attempt_renew_sync, sandbox_id)
                if ok:
                    st.last_success_monotonic = time.monotonic()
        except Exception:
            logger.exception("renew_intent: proxy renew task failed sandbox=%s", sandbox_id)
