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

"""Redis BRPOP workers consuming renew-intent payloads."""

from __future__ import annotations

import asyncio
import logging
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Optional

from redis.exceptions import RedisError

from src.config import AppConfig
from src.integrations.renew_intent.redis_client import connect_renew_intent_redis_from_config
from src.integrations.renew_intent.constants import (
    BRPOP_TIMEOUT_SECONDS,
    INTENT_MAX_AGE_SECONDS,
    LOCK_KEY_PREFIX,
    LOCK_TTL_SECONDS,
)
from src.integrations.renew_intent.controller import AccessRenewController
from src.integrations.renew_intent.intent import parse_renew_intent_json
from src.services.extension_service import ExtensionService, require_extension_service
from src.services.factory import create_sandbox_service
from src.services.sandbox_service import SandboxService

if TYPE_CHECKING:
    from redis.asyncio import Redis

logger = logging.getLogger(__name__)


class RenewIntentRunner:
    """Owns Redis client, BRPOP workers, and AccessRenewController."""

    def __init__(
        self,
        app_config: AppConfig,
        sandbox_service: SandboxService,
        extension_service: ExtensionService,
        redis_client: Redis,
    ) -> None:
        self._app_config = app_config
        self._sandbox_service = sandbox_service
        self._redis = redis_client
        ri = app_config.renew_intent
        self._queue_key = ri.redis.queue_key
        self._concurrency = ri.redis.consumer_concurrency
        self._controller = AccessRenewController(
            app_config, sandbox_service, extension_service, redis_client
        )
        self._stop = asyncio.Event()
        self._tasks: list[asyncio.Task[None]] = []

    @classmethod
    async def start(cls, app_config: AppConfig) -> Optional["RenewIntentRunner"]:
        ri = app_config.renew_intent
        if not ri.enabled or not ri.redis.enabled:
            return None

        try:
            redis_client = await connect_renew_intent_redis_from_config(app_config)
        except (RedisError, OSError, TimeoutError) as exc:
            logger.error(
                "renew_intent: Redis unavailable, workers not started: %s",
                exc,
            )
            return None

        if redis_client is None:
            logger.warning("renew_intent: Redis client is None; workers not started")
            return None

        sandbox_service = create_sandbox_service(config=app_config)
        extension_service = require_extension_service(sandbox_service)
        runner = cls(app_config, sandbox_service, extension_service, redis_client)
        runner._spawn_workers()
        logger.info(
            "renew_intent: started %s BRPOP workers on queue %s",
            runner._concurrency,
            runner._queue_key,
        )
        return runner

    def _spawn_workers(self) -> None:
        for i in range(self._concurrency):
            self._tasks.append(
                asyncio.create_task(
                    self._worker_loop(i),
                    name=f"renew_intent_brpop_{i}",
                )
            )

    @staticmethod
    def _is_stale(observed_at: datetime) -> bool:
        now = datetime.now(timezone.utc)
        age = (now - observed_at).total_seconds()
        return age > INTENT_MAX_AGE_SECONDS

    async def _handle_payload(self, raw: str) -> None:
        intent = parse_renew_intent_json(raw)
        if intent is None:
            return

        if self._is_stale(intent.observed_at):
            logger.debug(
                "renew_intent: dropped stale intent sandbox=%s",
                intent.sandbox_id,
            )
            return

        lock_key = f"{LOCK_KEY_PREFIX}{intent.sandbox_id}"
        acquired = await self._redis.set(lock_key, "1", nx=True, ex=LOCK_TTL_SECONDS)
        if not acquired:
            logger.debug(
                "renew_intent: lock busy sandbox=%s",
                intent.sandbox_id,
            )
            return

        try:
            if await self._redis.exists(self._controller.cooldown_key(intent.sandbox_id)):
                logger.debug(
                    "renew_intent: cooldown sandbox=%s",
                    intent.sandbox_id,
                )
                return
            await self._controller.process_intent_after_lock(intent)
        finally:
            pass

    async def _worker_loop(self, worker_id: int) -> None:
        while not self._stop.is_set():
            try:
                result = await self._redis.brpop(
                    self._queue_key,
                    BRPOP_TIMEOUT_SECONDS,
                )
            except asyncio.CancelledError:
                raise
            except (RedisError, OSError) as exc:
                logger.warning(
                    "renew_intent: worker %s Redis error: %s",
                    worker_id,
                    exc,
                )
                await asyncio.sleep(1.0)
                continue

            if result is None:
                continue
            _, payload = result
            if not isinstance(payload, str):
                continue
            try:
                await self._handle_payload(payload)
            except Exception as exc:
                logger.exception(
                    "renew_intent: worker %s handle error: %s",
                    worker_id,
                    exc,
                )

    async def stop(self) -> None:
        self._stop.set()
        for t in self._tasks:
            t.cancel()
        await asyncio.gather(*self._tasks, return_exceptions=True)
        self._tasks.clear()
        try:
            await self._redis.aclose()
        except Exception as exc:
            logger.debug("renew_intent: redis close: %s", exc)


async def start_renew_intent_runner(app_config: AppConfig) -> Optional[RenewIntentRunner]:
    """Start runner or return ``None`` if disabled or Redis unavailable."""
    return await RenewIntentRunner.start(app_config)
