from __future__ import annotations

import os
import time
import uuid
from collections.abc import Iterator
from datetime import datetime, timedelta, timezone
from typing import Any

import pytest

from opensandbox.exceptions import PoolStateStoreUnavailableException
from opensandbox.pool import RedisPoolStateStore


@pytest.fixture()
def redis_store() -> Iterator[tuple[RedisPoolStateStore, Any, str]]:
    redis_url = os.getenv("OPENSANDBOX_TEST_REDIS_URL")
    if not redis_url:
        pytest.skip("Set OPENSANDBOX_TEST_REDIS_URL to run RedisPoolStateStore tests")

    redis_module = pytest.importorskip("redis")
    redis_client = redis_module.Redis.from_url(redis_url)
    key_prefix = f"opensandbox:test:{uuid.uuid4()}"
    store = RedisPoolStateStore(redis_client, key_prefix=key_prefix)
    try:
        yield store, redis_client, key_prefix
    finally:
        for key in redis_client.scan_iter(f"{key_prefix}:*"):
            redis_client.delete(key)
        redis_client.close()


def test_redis_store_put_and_take_idle_fifo(
    redis_store: tuple[RedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = redis_store

    store.put_idle("pool", "id-1")
    store.put_idle("pool", "id-2")
    store.put_idle("pool", "id-3")

    assert store.try_take_idle("pool") == "id-1"
    assert store.try_take_idle("pool") == "id-2"
    assert store.try_take_idle("pool") == "id-3"
    assert store.try_take_idle("pool") is None


def test_redis_store_put_idle_is_idempotent(
    redis_store: tuple[RedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = redis_store

    store.put_idle("pool", "id-1")
    store.put_idle("pool", "id-1")

    assert store.snapshot_counters("pool").idle_count == 1
    assert store.try_take_idle("pool") == "id-1"
    assert store.try_take_idle("pool") is None


def test_redis_store_reap_expired_idle(
    redis_store: tuple[RedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = redis_store

    store.set_idle_entry_ttl("pool", timedelta(milliseconds=50))
    store.put_idle("pool", "id-1")
    time.sleep(0.1)
    store.reap_expired_idle("pool", datetime.now(timezone.utc))

    assert store.snapshot_counters("pool").idle_count == 0
    assert store.try_take_idle("pool") is None


def test_redis_store_primary_lock_owner_semantics(
    redis_store: tuple[RedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = redis_store

    assert store.try_acquire_primary_lock("pool", "owner-1", timedelta(seconds=60))
    assert store.try_acquire_primary_lock("pool", "owner-1", timedelta(seconds=60))
    assert store.renew_primary_lock("pool", "owner-1", timedelta(seconds=60))
    assert not store.try_acquire_primary_lock("pool", "owner-2", timedelta(seconds=60))
    assert not store.renew_primary_lock("pool", "owner-2", timedelta(seconds=60))

    store.release_primary_lock("pool", "owner-2")
    assert not store.try_acquire_primary_lock("pool", "owner-2", timedelta(seconds=60))

    store.release_primary_lock("pool", "owner-1")
    assert store.try_acquire_primary_lock("pool", "owner-2", timedelta(seconds=60))


def test_redis_store_max_idle_is_shared(
    redis_store: tuple[RedisPoolStateStore, Any, str],
) -> None:
    store, _, _ = redis_store

    assert store.get_max_idle("pool") is None
    store.set_max_idle("pool", 7)
    assert store.get_max_idle("pool") == 7
    store.set_max_idle("pool", 0)
    assert store.get_max_idle("pool") == 0


def test_redis_store_wraps_client_failures() -> None:
    store = RedisPoolStateStore(_BrokenRedis())

    with pytest.raises(PoolStateStoreUnavailableException):
        store.get_max_idle("pool")


class _BrokenRedis:
    def get(self, key: str) -> str | None:
        raise RuntimeError("redis unavailable")
