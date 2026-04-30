from __future__ import annotations

import asyncio
import time
from datetime import timedelta
from typing import Any, cast

import pytest

from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import (
    PoolAcquireFailedException,
    PoolEmptyException,
    PoolNotRunningException,
)
from opensandbox.pool import (
    AcquirePolicy,
    InMemoryAsyncPoolStateStore,
    PoolCreationSpec,
    SandboxPoolAsync,
)


@pytest.mark.asyncio
async def test_async_acquire_fail_fast_empty_raises_pool_empty() -> None:
    pool = _create_pool(max_idle=0)
    await pool.start()
    try:
        with pytest.raises(PoolEmptyException) as exc:
            await pool.acquire(policy=AcquirePolicy.FAIL_FAST)
        assert exc.value.error.code == "POOL_EMPTY"
    finally:
        await pool.shutdown(False)


@pytest.mark.asyncio
async def test_async_acquire_fail_fast_stale_idle_raises_and_kills_candidate() -> None:
    store = InMemoryAsyncPoolStateStore()
    await store.put_idle("pool", "stale-1")
    manager = FakeAsyncManager()
    pool = _create_pool(max_idle=0, store=store, manager=manager)
    await pool.start()

    try:
        with pytest.raises(PoolAcquireFailedException) as exc:
            await pool.acquire(policy=AcquirePolicy.FAIL_FAST)
        assert exc.value.error.code == "POOL_ACQUIRE_FAILED"
        assert (await store.snapshot_counters("pool")).idle_count == 0
        assert manager.killed == ["stale-1"]
    finally:
        await pool.shutdown(False)


@pytest.mark.asyncio
async def test_async_acquire_direct_create_when_empty() -> None:
    FakeAsyncSandbox.reset()
    pool = _create_pool(max_idle=0)
    await pool.start()

    try:
        sandbox = await pool.acquire(sandbox_timeout=timedelta(minutes=5))
        fake_sandbox = cast(FakeAsyncSandbox, sandbox)
        assert sandbox.id == "created-1"
        assert fake_sandbox.renewed == [timedelta(minutes=5)]
    finally:
        await pool.shutdown(False)


@pytest.mark.asyncio
async def test_async_acquire_when_stopped_raises_pool_not_running() -> None:
    pool = _create_pool(max_idle=0)

    with pytest.raises(PoolNotRunningException) as exc:
        await pool.acquire(policy=AcquirePolicy.FAIL_FAST)

    assert exc.value.error.code == "POOL_NOT_RUNNING"


@pytest.mark.asyncio
async def test_async_start_warms_idle_and_resize_zero_shrinks() -> None:
    FakeAsyncSandbox.reset()
    store = InMemoryAsyncPoolStateStore()
    manager = FakeAsyncManager()
    pool = _create_pool(max_idle=2, store=store, manager=manager)
    await pool.start()

    try:
        await _eventually(lambda: _idle_count_equals(pool, 2))
        await pool.resize(0)
        await _eventually(lambda: _idle_count_equals(pool, 0))
        assert len(manager.killed) >= 2
    finally:
        await pool.shutdown(False)


@pytest.mark.asyncio
async def test_async_graceful_shutdown_is_bounded_by_drain_timeout_during_warmup() -> None:
    FakeAsyncSandbox.reset()
    entered_preparer = asyncio.Event()
    release_preparer = asyncio.Event()

    async def blocking_preparer(sandbox: FakeAsyncSandbox) -> None:
        entered_preparer.set()
        await release_preparer.wait()

    pool = SandboxPoolAsync(
        pool_name="pool",
        owner_id="owner-1",
        max_idle=1,
        warmup_concurrency=1,
        state_store=InMemoryAsyncPoolStateStore(),
        connection_config=ConnectionConfig(),
        creation_spec=PoolCreationSpec(image="ubuntu:22.04"),
        reconcile_interval=timedelta(milliseconds=20),
        primary_lock_ttl=timedelta(seconds=5),
        drain_timeout=timedelta(milliseconds=50),
        warmup_sandbox_preparer=blocking_preparer,  # type: ignore[arg-type]
        sandbox_manager_factory=lambda config: _manager_factory(FakeAsyncManager()),
        sandbox_factory=FakeAsyncSandbox,  # type: ignore[arg-type]
    )
    await pool.start()
    try:
        await asyncio.wait_for(entered_preparer.wait(), timeout=2)

        started = time.monotonic()
        await pool.shutdown(graceful=True)
        elapsed = time.monotonic() - started

        assert elapsed < 1.0
        assert (await pool.snapshot()).lifecycle_state.value == "STOPPED"
    finally:
        release_preparer.set()
        await pool.shutdown(False)


def _create_pool(
    *,
    max_idle: int,
    store: InMemoryAsyncPoolStateStore | None = None,
    manager: FakeAsyncManager | None = None,
) -> SandboxPoolAsync:
    return SandboxPoolAsync(
        pool_name="pool",
        owner_id="owner-1",
        max_idle=max_idle,
        warmup_concurrency=2,
        state_store=store or InMemoryAsyncPoolStateStore(),
        connection_config=ConnectionConfig(),
        creation_spec=PoolCreationSpec(image="ubuntu:22.04"),
        reconcile_interval=timedelta(milliseconds=20),
        primary_lock_ttl=timedelta(seconds=5),
        drain_timeout=timedelta(milliseconds=50),
        sandbox_manager_factory=lambda config: _manager_factory(
            manager or FakeAsyncManager()
        ),
        sandbox_factory=FakeAsyncSandbox,  # type: ignore[arg-type]
    )


async def _manager_factory(manager: FakeAsyncManager) -> FakeAsyncManager:
    return manager


async def _idle_count_equals(pool: SandboxPoolAsync, expected: int) -> bool:
    return (await pool.snapshot()).idle_count == expected


async def _eventually(condition: Any, timeout: float = 2.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if await condition():
            return
        await asyncio.sleep(0.01)
    raise AssertionError("condition did not become true")


class FakeAsyncManager:
    def __init__(self) -> None:
        self.killed: list[str] = []
        self.closed = False

    async def kill_sandbox(self, sandbox_id: str) -> None:
        self.killed.append(sandbox_id)

    async def close(self) -> None:
        self.closed = True


class FakeAsyncSandbox:
    created_count = 0

    def __init__(self, sandbox_id: str) -> None:
        self.id = sandbox_id
        self.renewed: list[timedelta] = []
        self.closed = False
        self.killed = False

    @classmethod
    def reset(cls) -> None:
        cls.created_count = 0

    @classmethod
    async def create(cls, *args: Any, **kwargs: Any) -> FakeAsyncSandbox:
        cls.created_count += 1
        return cls(f"created-{cls.created_count}")

    @classmethod
    async def connect(cls, sandbox_id: str, *args: Any, **kwargs: Any) -> FakeAsyncSandbox:
        if sandbox_id.startswith("stale"):
            raise RuntimeError("stale sandbox")
        return cls(sandbox_id)

    async def renew(self, timeout: timedelta) -> None:
        self.renewed.append(timeout)

    async def kill(self) -> None:
        self.killed = True

    async def close(self) -> None:
        self.closed = True
