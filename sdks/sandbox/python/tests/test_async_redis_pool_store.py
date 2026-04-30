from __future__ import annotations

import asyncio
import os
import uuid
from collections.abc import AsyncIterator
from datetime import datetime, timedelta, timezone
from typing import Any

import pytest

from opensandbox.exceptions import PoolStateStoreUnavailableException
from opensandbox.pool import AsyncRedisPoolStateStore


@pytest.fixture()
async def async_redis_store() -> AsyncIterator[tuple[AsyncRedisPoolStateStore, Any, str]]:
    redis_url = os.getenv("OPENSANDBOX_TEST_REDIS_URL")
    if not redis_url:
        pytest.skip("Set OPENSANDBOX_TEST_REDIS_URL to run AsyncRedisPoolStateStore tests")

    redis_module = pytest.importorskip("redis.asyncio")
    redis_client = redis_module.Redis.from_url(redis_url)
    key_prefix = f"opensandbox:test:{uuid.uuid4()}"
    store = AsyncRedisPoolStateStore(redis_client, key_prefix=key_prefix)
    try:
        yield store, redis_client, key_prefix
    finally:
        async for key in redis_client.scan_iter(f"{key_prefix}:*"):
            await redis_client.delete(key)
        await redis_client.aclose()


@pytest.mark.asyncio
async def test_async_redis_store_put_and_take_idle_fifo(
    async_redis_store: tuple[AsyncRedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = async_redis_store

    await store.put_idle("pool", "id-1")
    await store.put_idle("pool", "id-2")
    await store.put_idle("pool", "id-3")

    assert await store.try_take_idle("pool") == "id-1"
    assert await store.try_take_idle("pool") == "id-2"
    assert await store.try_take_idle("pool") == "id-3"
    assert await store.try_take_idle("pool") is None


@pytest.mark.asyncio
async def test_async_redis_store_put_idle_is_idempotent(
    async_redis_store: tuple[AsyncRedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = async_redis_store

    await store.put_idle("pool", "id-1")
    await store.put_idle("pool", "id-1")

    assert (await store.snapshot_counters("pool")).idle_count == 1
    assert await store.try_take_idle("pool") == "id-1"
    assert await store.try_take_idle("pool") is None


@pytest.mark.asyncio
async def test_async_redis_store_reap_expired_idle(
    async_redis_store: tuple[AsyncRedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = async_redis_store

    await store.set_idle_entry_ttl("pool", timedelta(milliseconds=50))
    await store.put_idle("pool", "id-1")
    await asyncio.sleep(0.1)
    await store.reap_expired_idle("pool", datetime.now(timezone.utc))

    assert (await store.snapshot_counters("pool")).idle_count == 0
    assert await store.try_take_idle("pool") is None


@pytest.mark.asyncio
async def test_async_redis_store_primary_lock_owner_semantics(
    async_redis_store: tuple[AsyncRedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = async_redis_store

    assert await store.try_acquire_primary_lock("pool", "owner-1", timedelta(seconds=60))
    assert await store.try_acquire_primary_lock("pool", "owner-1", timedelta(seconds=60))
    assert await store.renew_primary_lock("pool", "owner-1", timedelta(seconds=60))
    assert not await store.try_acquire_primary_lock(
        "pool", "owner-2", timedelta(seconds=60)
    )
    assert not await store.renew_primary_lock("pool", "owner-2", timedelta(seconds=60))

    await store.release_primary_lock("pool", "owner-2")
    assert not await store.try_acquire_primary_lock(
        "pool", "owner-2", timedelta(seconds=60)
    )

    await store.release_primary_lock("pool", "owner-1")
    assert await store.try_acquire_primary_lock("pool", "owner-2", timedelta(seconds=60))


@pytest.mark.asyncio
async def test_async_redis_store_max_idle_is_shared(
    async_redis_store: tuple[AsyncRedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = async_redis_store

    assert await store.get_max_idle("pool") is None
    await store.set_max_idle("pool", 7)
    assert await store.get_max_idle("pool") == 7
    await store.set_max_idle("pool", 0)
    assert await store.get_max_idle("pool") == 0


@pytest.mark.asyncio
async def test_async_redis_store_wraps_client_failures() -> None:
    store = AsyncRedisPoolStateStore(_BrokenAsyncRedis())

    with pytest.raises(PoolStateStoreUnavailableException):
        await store.get_max_idle("pool")


class _BrokenAsyncRedis:
    async def get(self, key: str) -> str | None:
        raise RuntimeError("redis unavailable")
