from __future__ import annotations

import threading
import time
from datetime import timedelta
from typing import Any, cast

import pytest

from opensandbox.config.connection_sync import ConnectionConfigSync
from opensandbox.exceptions import (
    PoolAcquireFailedException,
    PoolEmptyException,
    PoolNotRunningException,
)
from opensandbox.pool import AcquirePolicy, InMemoryPoolStateStore, PoolCreationSpec
from opensandbox.sync.pool import SandboxPoolSync


def test_acquire_fail_fast_empty_raises_pool_empty() -> None:
    pool = _create_pool(max_idle=0)
    pool.start()
    try:
        with pytest.raises(PoolEmptyException) as exc:
            pool.acquire(policy=AcquirePolicy.FAIL_FAST)
        assert exc.value.error.code == "POOL_EMPTY"
    finally:
        pool.shutdown(False)


def test_acquire_fail_fast_stale_idle_raises_and_kills_candidate() -> None:
    store = InMemoryPoolStateStore()
    store.put_idle("pool", "stale-1")
    manager = FakeManager()
    pool = _create_pool(max_idle=0, store=store, manager=manager)
    pool.start()

    try:
        with pytest.raises(PoolAcquireFailedException) as exc:
            pool.acquire(policy=AcquirePolicy.FAIL_FAST)
        assert exc.value.error.code == "POOL_ACQUIRE_FAILED"
        assert store.snapshot_counters("pool").idle_count == 0
        assert manager.killed == ["stale-1"]
    finally:
        pool.shutdown(False)


def test_acquire_direct_create_when_empty() -> None:
    FakeSandbox.reset()
    pool = _create_pool(max_idle=0)
    pool.start()

    try:
        sandbox = pool.acquire(sandbox_timeout=timedelta(minutes=5))
        fake_sandbox = cast(FakeSandbox, sandbox)
        assert sandbox.id == "created-1"
        assert fake_sandbox.renewed == [timedelta(minutes=5)]
    finally:
        pool.shutdown(False)


def test_acquire_when_stopped_raises_pool_not_running() -> None:
    pool = _create_pool(max_idle=0)

    with pytest.raises(PoolNotRunningException) as exc:
        pool.acquire(policy=AcquirePolicy.FAIL_FAST)

    assert exc.value.error.code == "POOL_NOT_RUNNING"


def test_start_warms_idle_and_resize_zero_shrinks() -> None:
    FakeSandbox.reset()
    store = InMemoryPoolStateStore()
    manager = FakeManager()
    pool = _create_pool(max_idle=2, store=store, manager=manager)
    pool.start()

    try:
        _eventually(lambda: pool.snapshot().idle_count == 2)
        pool.resize(0)
        _eventually(lambda: pool.snapshot().idle_count == 0)
        assert len(manager.killed) >= 2
    finally:
        pool.shutdown(False)


def test_graceful_shutdown_is_bounded_by_drain_timeout_during_warmup() -> None:
    FakeSandbox.reset()
    entered_preparer = threading.Event()
    release_preparer = threading.Event()

    def blocking_preparer(sandbox: FakeSandbox) -> None:
        entered_preparer.set()
        release_preparer.wait(timeout=5)

    pool = SandboxPoolSync(
        pool_name="pool",
        owner_id="owner-1",
        max_idle=1,
        warmup_concurrency=1,
        state_store=InMemoryPoolStateStore(),
        connection_config=ConnectionConfigSync(),
        creation_spec=PoolCreationSpec(image="ubuntu:22.04"),
        reconcile_interval=timedelta(milliseconds=20),
        primary_lock_ttl=timedelta(seconds=5),
        drain_timeout=timedelta(milliseconds=50),
        warmup_sandbox_preparer=blocking_preparer,  # type: ignore[arg-type]
        sandbox_manager_factory=lambda config: FakeManager(),  # type: ignore[arg-type,return-value]
        sandbox_factory=FakeSandbox,  # type: ignore[arg-type]
    )
    pool.start()
    try:
        assert entered_preparer.wait(timeout=2)

        started = time.monotonic()
        pool.shutdown(graceful=True)
        elapsed = time.monotonic() - started

        assert elapsed < 1.0
        assert pool.snapshot().lifecycle_state.value == "STOPPED"
    finally:
        release_preparer.set()
        pool.shutdown(False)


def _create_pool(
    *,
    max_idle: int,
    store: InMemoryPoolStateStore | None = None,
    manager: FakeManager | None = None,
) -> SandboxPoolSync:
    return SandboxPoolSync(
        pool_name="pool",
        owner_id="owner-1",
        max_idle=max_idle,
        warmup_concurrency=2,
        state_store=store or InMemoryPoolStateStore(),
        connection_config=ConnectionConfigSync(),
        creation_spec=PoolCreationSpec(image="ubuntu:22.04"),
        reconcile_interval=timedelta(milliseconds=20),
        primary_lock_ttl=timedelta(seconds=5),
        drain_timeout=timedelta(milliseconds=50),
        sandbox_manager_factory=lambda config: manager or FakeManager(),  # type: ignore[arg-type,return-value]
        sandbox_factory=FakeSandbox,  # type: ignore[arg-type]
    )


def _eventually(condition: Any, timeout: float = 2.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if condition():
            return
        time.sleep(0.01)
    raise AssertionError("condition did not become true")


class FakeManager:
    def __init__(self) -> None:
        self.killed: list[str] = []
        self.closed = False

    def kill_sandbox(self, sandbox_id: str) -> None:
        self.killed.append(sandbox_id)

    def close(self) -> None:
        self.closed = True


class FakeSandbox:
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
    def create(cls, *args: Any, **kwargs: Any) -> FakeSandbox:
        cls.created_count += 1
        return cls(f"created-{cls.created_count}")

    @classmethod
    def connect(cls, sandbox_id: str, *args: Any, **kwargs: Any) -> FakeSandbox:
        if sandbox_id.startswith("stale"):
            raise RuntimeError("stale sandbox")
        return cls(sandbox_id)

    def renew(self, timeout: timedelta) -> None:
        self.renewed.append(timeout)

    def kill(self) -> None:
        self.killed = True

    def close(self) -> None:
        self.closed = True
