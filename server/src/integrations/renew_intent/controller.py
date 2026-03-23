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

"""Renew access path: eligibility checks then ``renew_expiration``."""

from __future__ import annotations

import logging
from datetime import datetime, timedelta, timezone
from typing import TYPE_CHECKING, Optional

from fastapi import HTTPException

from src.api.schema import RenewSandboxExpirationRequest
from src.integrations.renew_intent.constants import COOLDOWN_KEY_PREFIX
from src.integrations.renew_intent.intent import RenewIntent

if TYPE_CHECKING:
    from redis.asyncio import Redis

    from src.config import AppConfig
    from src.services.extension_service import ExtensionService
    from src.services.sandbox_service import SandboxService

logger = logging.getLogger(__name__)


class AccessRenewController:
    """``redis`` optional: set cooldown key after success when connected; omit for proxy-only path."""

    def __init__(
        self,
        app_config: "AppConfig",
        sandbox_service: "SandboxService",
        extension_service: "ExtensionService",
        redis: Optional["Redis"],
    ) -> None:
        self._app_config = app_config
        self._sandbox_service = sandbox_service
        self._extension_service = extension_service
        self._redis = redis
        self._min_interval = app_config.renew_intent.min_interval_seconds

    def cooldown_key(self, sandbox_id: str) -> str:
        return f"{COOLDOWN_KEY_PREFIX}{sandbox_id}"

    def _try_renew_sync(self, sandbox_id: str) -> bool:
        try:
            sandbox = self._sandbox_service.get_sandbox(sandbox_id)
        except HTTPException as exc:
            logger.debug(
                "renew_intent: get_sandbox %s failed: %s",
                sandbox_id,
                exc.detail,
            )
            return False

        if sandbox.status.state.lower() != "running":
            logger.debug(
                "renew_intent: skip %s state=%s",
                sandbox_id,
                sandbox.status.state,
            )
            return False

        if sandbox.expires_at is None:
            logger.debug("renew_intent: skip %s no expires_at", sandbox_id)
            return False

        extend = self._extension_service.get_access_renew_extend_seconds(sandbox_id)
        if extend is None:
            logger.debug("renew_intent: skip %s not opted in", sandbox_id)
            return False

        now = datetime.now(timezone.utc)
        current = sandbox.expires_at
        if current.tzinfo is None:
            current = current.replace(tzinfo=timezone.utc)

        candidate = now + timedelta(seconds=extend)
        new_expires = max(candidate, current)

        req = RenewSandboxExpirationRequest(expires_at=new_expires)
        try:
            self._sandbox_service.renew_expiration(sandbox_id, req)
        except HTTPException as exc:
            logger.warning(
                "renew_intent: renew_expiration failed sandbox=%s detail=%s",
                sandbox_id,
                exc.detail,
            )
            return False
        logger.info("renew_intent: renewed sandbox=%s until %s", sandbox_id, new_expires)
        return True

    def attempt_renew_sync(self, sandbox_id: str) -> bool:
        """Run gates + renew; does not write Redis (cooldown handled by caller)."""
        return self._try_renew_sync(sandbox_id)

    async def process_intent_after_lock(self, intent: RenewIntent) -> None:
        """Renew after lock; on success set cooldown in Redis when configured."""
        import asyncio

        ok = await asyncio.to_thread(self._try_renew_sync, intent.sandbox_id)
        if ok and self._redis is not None:
            await self._redis.set(
                self.cooldown_key(intent.sandbox_id),
                "1",
                ex=self._min_interval,
            )
