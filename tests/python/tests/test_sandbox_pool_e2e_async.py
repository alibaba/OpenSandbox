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
"""E2E coverage for the asyncio Python sandbox pool."""

from __future__ import annotations

import asyncio
import os
import uuid
from collections.abc import Awaitable, Callable
from datetime import datetime, timedelta, timezone

import pytest
from opensandbox import Sandbox, SandboxManager
from opensandbox.exceptions import PoolEmptyException, PoolNotRunningException
from opensandbox.models.sandboxes import SandboxFilter
from opensandbox.pool import (
    AcquirePolicy,
    AsyncPoolStateStore,
    AsyncRedisPoolStateStore,
    InMemoryAsyncPoolStateStore,
    PoolCreationSpec,
    PoolSnapshot,
    PoolState,
    SandboxPoolAsync,
)

from tests.base_e2e_test import (
    create_connection_config,
    get_e2e_sandbox_resource,
    get_sandbox_image,
)

MAX_IDLE = 2
RECONCILE_INTERVAL = timedelta(seconds=1)
PRIMARY_LOCK_TTL = timedelta(seconds=4)
DRAIN_TIMEOUT = timedelta(milliseconds=300)
AWAIT_TIMEOUT = timedelta(minutes=2)


@pytest.mark.e2e
class TestSandboxPoolSingleNodeE2EAsync:
    """Single-event-loop async in-memory pool E2E scenarios."""

    @pytest.fixture(autouse=True)
    async def _pool_lifecycle(self):
        self.tag = _tag("py-async-pool")
        self.pool_name = f"pool-{self.tag}"
        self.store = InMemoryAsyncPoolStateStore()
        self.manager = await SandboxManager.create(create_connection_config())
        self.borrowed: list[Sandbox] = []
        self.pool = _create_pool(
            pool_name=self.pool_name,
            owner_id=f"owner-{self.tag}",
            state_store=self.store,
            tag=self.tag,
            max_idle=MAX_IDLE,
        )
        await self.pool.start()
        try:
            yield
        finally:
            await _cleanup_borrowed(self.borrowed)
            await _cleanup_pool(self.pool)
            await _cleanup_tagged_sandboxes(self.manager, self.tag)
            await self.manager.close()

    @pytest.mark.timeout(240)
    async def test_async_warmup_acquire_command_resize_and_shutdown(self) -> None:
        await _eventually(
            "async pool becomes healthy with warm idle",
            lambda: _snapshot_matches(
                self.pool,
                lambda snap: snap.state == PoolState.HEALTHY and snap.idle_count >= 1,
            ),
        )

        sandbox = await self.pool.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)
        self.borrowed.append(sandbox)
        assert await sandbox.is_healthy()
        result = await sandbox.commands.run("echo py-async-pool-ok")
        assert result.error is None
        assert result.logs.stdout[0].text == "py-async-pool-ok"

        await self.pool.resize(0)
        released = await self.pool.release_all_idle()
        assert released >= 0
        await _eventually(
            "async idle drains after resize zero",
            lambda: _snapshot_matches(self.pool, lambda snap: snap.idle_count == 0),
        )
        with pytest.raises(PoolEmptyException):
            await self.pool.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)

        direct = await self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
        self.borrowed.append(direct)
        assert await direct.is_healthy()

        await self.pool.shutdown(graceful=True)
        with pytest.raises(PoolNotRunningException):
            await self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)

    @pytest.mark.timeout(300)
    async def test_async_lifecycle_idempotency_release_remote_and_rewarm(self) -> None:
        await self.pool.start()
        await _eventually(
            "async pool warms before lifecycle checks",
            lambda: _snapshot_matches(self.pool, lambda snap: snap.idle_count >= 1),
        )

        await self.pool.shutdown(False)
        await self.pool.shutdown(False)
        assert (await self.pool.snapshot()).state == PoolState.STOPPED
        with pytest.raises(PoolNotRunningException):
            await self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)

        await self.store.put_idle(self.pool_name, f"injected-a-{uuid.uuid4().hex}")
        await self.store.put_idle(self.pool_name, f"injected-b-{uuid.uuid4().hex}")
        assert await self.pool.release_all_idle() == 2
        assert (await self.pool.snapshot()).idle_count == 0

        await self.pool.start()
        await _eventually(
            "async pool rewarms after restart",
            lambda: _snapshot_matches(self.pool, lambda snap: snap.idle_count >= 1),
        )

        await self.pool.resize(0)
        released = await self.pool.release_all_idle()
        assert released >= 1
        await _eventually(
            "async releaseAllIdle reduces remote tagged sandboxes",
            lambda: _async_release_drained(self.pool, self.manager, self.tag),
            timeout=timedelta(seconds=60),
        )

        await self.pool.resize(1)
        await _eventually(
            "async resize from zero to positive rewarms idle",
            lambda: _snapshot_matches(
                self.pool,
                lambda snap: snap.state == PoolState.HEALTHY and snap.idle_count >= 1,
            ),
        )

    @pytest.mark.timeout(240)
    async def test_async_stale_idle_preparer_snapshot_and_context_manager(self) -> None:
        await self.store.put_idle(self.pool_name, f"missing-{uuid.uuid4().hex}")
        fallback = await self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
        self.borrowed.append(fallback)
        assert await fallback.is_healthy()

        await _cleanup_pool(self.pool)
        marker_path = f"/tmp/{self.tag}-prepared.txt"

        async def preparer(sandbox: Sandbox) -> None:
            result = await sandbox.commands.run(f"printf async-prepared > {marker_path}")
            assert result.error is None

        prepared_pool = _create_pool(
            pool_name=f"prepared-{self.pool_name}",
            owner_id=f"prepared-owner-{self.tag}",
            state_store=InMemoryAsyncPoolStateStore(),
            tag=self.tag,
            max_idle=1,
            warmup_sandbox_preparer=preparer,
        )
        async with prepared_pool:
            await _eventually(
                "async prepared pool warms",
                lambda: _snapshot_matches(prepared_pool, lambda snap: snap.idle_count >= 1),
            )
            entries = await prepared_pool.snapshot_idle_entries()
            assert entries
            assert all(entry.expires_at > datetime.now(timezone.utc) for entry in entries)

            sandbox = await prepared_pool.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)
            self.borrowed.append(sandbox)
            result = await sandbox.commands.run(f"cat {marker_path}")
            assert result.error is None
            assert result.logs.stdout[0].text == "async-prepared"


@pytest.mark.e2e
class TestSandboxPoolRedisDistributedE2EAsync:
    """Redis-backed async pool E2E scenarios."""

    @pytest.fixture(autouse=True)
    async def _redis_lifecycle(self):
        redis_url = os.getenv("OPENSANDBOX_TEST_REDIS_URL")
        if not redis_url:
            pytest.skip("Set OPENSANDBOX_TEST_REDIS_URL to run Redis-backed pool E2E tests")
        redis_module = pytest.importorskip("redis.asyncio")
        self.redis = redis_module.Redis.from_url(redis_url, decode_responses=True)
        self.key_prefix = f"opensandbox:e2e:{uuid.uuid4()}"
        self.manager = await SandboxManager.create(create_connection_config())
        self.borrowed: list[Sandbox] = []
        self.pools: list[SandboxPoolAsync] = []
        self.tag = _tag("py-async-redis-pool")
        try:
            yield
        finally:
            await _cleanup_borrowed(self.borrowed)
            for pool in self.pools:
                await _cleanup_pool(pool)
            await _cleanup_tagged_sandboxes(self.manager, self.tag)
            await self.manager.close()
            async for key in self.redis.scan_iter(f"{self.key_prefix}:*"):
                await self.redis.delete(key)
            await self.redis.aclose()

    @pytest.mark.timeout(360)
    async def test_async_redis_cross_node_acquire_resize_and_concurrent_uniqueness(self) -> None:
        pool_name = f"async-redis-pool-{self.tag}"
        pool_a = _create_pool(
            pool_name,
            f"owner-a-{self.tag}",
            AsyncRedisPoolStateStore(self.redis, self.key_prefix),
            self.tag,
            2,
        )
        pool_b = _create_pool(
            pool_name,
            f"owner-b-{self.tag}",
            AsyncRedisPoolStateStore(self.redis, self.key_prefix),
            self.tag,
            2,
        )
        self.pools.extend([pool_a, pool_b])
        await pool_a.start()
        await pool_b.start()
        await _eventually(
            "async Redis pool warms two idle",
            lambda: _snapshot_matches(pool_a, lambda snap: snap.idle_count >= 2),
        )

        acquired = await asyncio.gather(
            pool_a.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST),
            pool_b.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST),
        )
        self.borrowed.extend(acquired)
        assert len({sandbox.id for sandbox in acquired}) == 2
        assert all([await sandbox.is_healthy() for sandbox in acquired])

        result = await acquired[0].commands.run("echo py-async-redis-ok")
        assert result.error is None

        await pool_b.resize(0)
        await _eventually(
            "async Redis idle drains after shared resize",
            lambda: _snapshot_matches(pool_a, lambda snap: snap.idle_count == 0),
        )
        await asyncio.sleep(RECONCILE_INTERVAL.total_seconds() * 2)
        assert (await pool_a.snapshot()).idle_count == 0
        with pytest.raises(PoolEmptyException):
            await pool_a.acquire(timedelta(minutes=2), AcquirePolicy.FAIL_FAST)


def _create_pool(
    pool_name: str,
    owner_id: str,
    state_store: AsyncPoolStateStore,
    tag: str,
    max_idle: int,
    warmup_sandbox_preparer: Callable[[Sandbox], Awaitable[None]] | None = None,
) -> SandboxPoolAsync:
    return SandboxPoolAsync(
        pool_name=pool_name,
        owner_id=owner_id,
        max_idle=max_idle,
        warmup_concurrency=1,
        state_store=state_store,
        connection_config=create_connection_config(),
        creation_spec=PoolCreationSpec(
            image=get_sandbox_image(),
            entrypoint=["tail", "-f", "/dev/null"],
            metadata={"tag": tag, "suite": "sandbox-pool-python-async-e2e"},
            env={
                "E2E_TEST": "true",
                "EXECD_API_GRACE_SHUTDOWN": "3s",
                "EXECD_JUPYTER_IDLE_POLL_INTERVAL": "1s",
            },
            resource=get_e2e_sandbox_resource(),
        ),
        reconcile_interval=RECONCILE_INTERVAL,
        primary_lock_ttl=PRIMARY_LOCK_TTL,
        drain_timeout=DRAIN_TIMEOUT,
        warmup_sandbox_preparer=warmup_sandbox_preparer,
    )


async def _eventually(
    description: str,
    condition: Callable[[], Awaitable[bool]],
    timeout: timedelta = AWAIT_TIMEOUT,
    interval: timedelta = timedelta(seconds=1),
) -> None:
    deadline = asyncio.get_running_loop().time() + timeout.total_seconds()
    last_error: BaseException | None = None
    while asyncio.get_running_loop().time() < deadline:
        try:
            if await condition():
                return
        except BaseException as exc:
            last_error = exc
        await asyncio.sleep(interval.total_seconds())
    if last_error is not None:
        raise AssertionError(f"Timed out waiting for {description}") from last_error
    raise AssertionError(f"Timed out waiting for {description}")


async def _snapshot_matches(
    pool: SandboxPoolAsync,
    predicate: Callable[[PoolSnapshot], bool],
) -> bool:
    return predicate(await pool.snapshot())


async def _cleanup_pool(pool: SandboxPoolAsync) -> None:
    try:
        await pool.resize(0)
    except Exception:
        pass
    try:
        await pool.release_all_idle()
    except Exception:
        pass
    try:
        await pool.shutdown(False)
    except Exception:
        pass


async def _cleanup_borrowed(sandboxes: list[Sandbox]) -> None:
    for sandbox in sandboxes:
        try:
            await sandbox.kill()
        except Exception:
            pass
        try:
            await sandbox.close()
        except Exception:
            pass
    sandboxes.clear()


async def _cleanup_tagged_sandboxes(manager: SandboxManager, tag: str) -> None:
    try:
        infos = await manager.list_sandbox_infos(
            SandboxFilter(metadata={"tag": tag}, page_size=50)
        )
        for info in infos.sandbox_infos:
            try:
                await manager.kill_sandbox(info.id)
            except Exception:
                pass
    except Exception:
        pass


async def _count_tagged_sandboxes(manager: SandboxManager, tag: str) -> int:
    infos = await manager.list_sandbox_infos(SandboxFilter(metadata={"tag": tag}, page_size=50))
    return len(infos.sandbox_infos)


async def _async_release_drained(
    pool: SandboxPoolAsync,
    manager: SandboxManager,
    tag: str,
) -> bool:
    snapshot = await pool.snapshot()
    return snapshot.idle_count == 0 and await _count_tagged_sandboxes(manager, tag) == 0


def _tag(prefix: str) -> str:
    return f"{prefix}-{uuid.uuid4().hex[:8]}"
