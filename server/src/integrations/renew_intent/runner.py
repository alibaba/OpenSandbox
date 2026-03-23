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
from src.integrations.renew_intent.logutil import (
    RENEW_EVENT_WORKERS_NOT_STARTED,
    RENEW_EVENT_WORKERS_STARTED,
    RENEW_SOURCE_REDIS_QUEUE,
    renew_bundle,
)
from src.services.extension_service import ExtensionService, require_extension_service
from src.services.factory import create_sandbox_service
from src.services.sandbox_service import SandboxService

if TYPE_CHECKING:
    from redis.asyncio import Redis

logger = logging.getLogger(__name__)


class RenewIntentRunner:
    """Redis client, BRPOP workers, and ``AccessRenewController``."""

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
            line, ex = renew_bundle(
                event=RENEW_EVENT_WORKERS_NOT_STARTED,
                source=RENEW_SOURCE_REDIS_QUEUE,
                skip_reason="redis_connect_failed",
                error_type=type(exc).__name__,
            )
            logger.error(f"renew_intent {line} error={exc!s}", extra=ex)
            return None

        if redis_client is None:
            line, ex = renew_bundle(
                event=RENEW_EVENT_WORKERS_NOT_STARTED,
                source=RENEW_SOURCE_REDIS_QUEUE,
                skip_reason="redis_client_none",
            )
            logger.warning(f"renew_intent {line}", extra=ex)
            return None

        sandbox_service = create_sandbox_service(config=app_config)
        extension_service = require_extension_service(sandbox_service)
        runner = cls(app_config, sandbox_service, extension_service, redis_client)
        runner._spawn_workers()
        line, ex = renew_bundle(
            event=RENEW_EVENT_WORKERS_STARTED,
            source=RENEW_SOURCE_REDIS_QUEUE,
            worker_count=runner._concurrency,
            queue_key=runner._queue_key,
        )
        logger.info(f"renew_intent {line}", extra=ex)
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
            return

        lock_key = f"{LOCK_KEY_PREFIX}{intent.sandbox_id}"
        acquired = await self._redis.set(lock_key, "1", nx=True, ex=LOCK_TTL_SECONDS)
        if not acquired:
            return

        if await self._redis.exists(self._controller.cooldown_key(intent.sandbox_id)):
            return
        await self._controller.process_intent_after_lock(intent)

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
                line, ex = renew_bundle(
                    event="worker_redis_error",
                    source=RENEW_SOURCE_REDIS_QUEUE,
                    worker_id=worker_id,
                    error_type=type(exc).__name__,
                )
                logger.warning(f"renew_intent {line} error={exc!s}", extra=ex)
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
                line, ex = renew_bundle(
                    event="worker_handle_error",
                    source=RENEW_SOURCE_REDIS_QUEUE,
                    worker_id=worker_id,
                    error_type=type(exc).__name__,
                )
                logger.exception(f"renew_intent {line}", extra=ex)

    async def stop(self) -> None:
        self._stop.set()
        for t in self._tasks:
            t.cancel()
        await asyncio.gather(*self._tasks, return_exceptions=True)
        self._tasks.clear()
        try:
            await self._redis.aclose()
        except Exception as exc:
            logger.debug(f"renew_intent redis_close error={exc!s}")


async def start_renew_intent_runner(app_config: AppConfig) -> Optional[RenewIntentRunner]:
    """Start runner or return ``None`` if disabled or Redis unavailable."""
    return await RenewIntentRunner.start(app_config)
