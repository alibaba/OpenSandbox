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
"""E2E coverage for the synchronous Python sandbox pool."""

from __future__ import annotations

import os
import threading
import time
import uuid
from collections import defaultdict, deque
from collections.abc import Callable
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone

import httpx
import pytest
from opensandbox import SandboxManagerSync, SandboxSync
from opensandbox.config import ConnectionConfigSync
from opensandbox.exceptions import (
    PoolEmptyException,
    PoolNotRunningException,
)
from opensandbox.models.sandboxes import SandboxFilter
from opensandbox.pool import (
    AcquirePolicy,
    IdleEntry,
    InMemoryPoolStateStore,
    PoolCreationSpec,
    PoolState,
    PoolStateStore,
    RedisPoolStateStore,
    SandboxPoolSync,
    StoreCounters,
)

from tests.base_e2e_test import (
    create_connection_config_sync,
    get_e2e_sandbox_resource,
    get_sandbox_image,
)

MAX_IDLE = 2
RECONCILE_INTERVAL = timedelta(seconds=1)
PRIMARY_LOCK_TTL = timedelta(seconds=4)
DRAIN_TIMEOUT = timedelta(milliseconds=300)
AWAIT_TIMEOUT = timedelta(minutes=2)


@pytest.mark.e2e
class TestSandboxPoolSingleNodeE2ESync:
    """Single-process in-memory pool E2E scenarios."""

    def setup_method(self) -> None:
        self.tag = _tag("py-pool")
        self.pool_name = f"pool-{self.tag}"
        self.store = InMemoryPoolStateStore()
        self.manager = SandboxManagerSync.create(create_connection_config_sync())
        self.borrowed: list[SandboxSync] = []
        self.pool = _create_pool(
            pool_name=self.pool_name,
            owner_id=f"owner-{self.tag}",
            state_store=self.store,
            tag=self.tag,
            max_idle=MAX_IDLE,
        )
        self.pool.start()

    def teardown_method(self) -> None:
        _cleanup_borrowed(self.borrowed)
        _cleanup_pool(self.pool)
        _cleanup_tagged_sandboxes(self.manager, self.tag)
        self.manager.close()

    @pytest.mark.timeout(240)
    def test_warmup_acquire_fail_fast_and_command(self) -> None:
        _eventually(
            "pool becomes healthy with warm idle",
            lambda: self.pool.snapshot().state == PoolState.HEALTHY
            and self.pool.snapshot().idle_count >= 1,
        )

        sandbox = self.pool.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)
        self.borrowed.append(sandbox)
        assert sandbox.is_healthy()

        result = sandbox.commands.run("echo py-pool-basic-ok")
        assert result.error is None
        assert result.logs.stdout[0].text == "py-pool-basic-ok"

    @pytest.mark.timeout(240)
    def test_resize_release_fail_fast_and_direct_create_fallback(self) -> None:
        _eventually("pool has warm idle", lambda: self.pool.snapshot().idle_count >= 1)

        self.pool.resize(0)
        released = self.pool.release_all_idle()
        assert released >= 0
        _eventually("idle drains after resize zero", lambda: self.pool.snapshot().idle_count == 0)

        with pytest.raises(PoolEmptyException):
            self.pool.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)

        direct = self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
        self.borrowed.append(direct)
        assert direct.is_healthy()

    @pytest.mark.timeout(240)
    def test_stale_idle_fallback_shutdown_restart_and_snapshot(self) -> None:
        self.store.put_idle(self.pool_name, f"missing-{time.monotonic_ns()}")

        fallback = self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
        self.borrowed.append(fallback)
        assert fallback.is_healthy()

        self.pool.shutdown(graceful=True)
        with pytest.raises(PoolNotRunningException):
            self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)

        stopped = self.pool.snapshot()
        assert stopped.state == PoolState.STOPPED
        assert stopped.lifecycle_state.value == "STOPPED"

        self.pool.start()
        _eventually(
            "pool restarts and warms idle",
            lambda: self.pool.snapshot().state == PoolState.HEALTHY
            and self.pool.snapshot().idle_count >= 1,
        )
        entries = self.pool.snapshot_idle_entries()
        assert entries
        assert all(entry.sandbox_id for entry in entries)
        assert all(entry.expires_at > datetime.now(timezone.utc) for entry in entries)

    @pytest.mark.timeout(360)
    def test_lifecycle_idempotency_resize_rewarm_and_release_remote(self) -> None:
        self.pool.start()
        _eventually("pool warms before lifecycle checks", lambda: self.pool.snapshot().idle_count >= 1)

        self.pool.shutdown(False)
        self.pool.shutdown(False)
        assert self.pool.snapshot().state == PoolState.STOPPED
        with pytest.raises(PoolNotRunningException):
            self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)

        self.store.put_idle(self.pool_name, f"injected-a-{uuid.uuid4().hex}")
        self.store.put_idle(self.pool_name, f"injected-b-{uuid.uuid4().hex}")
        assert self.pool.release_all_idle() == 2
        assert self.pool.snapshot().idle_count == 0

        self.pool.start()
        _eventually("pool rewarms after restart", lambda: self.pool.snapshot().idle_count >= 1)

        self.pool.resize(0)
        released = self.pool.release_all_idle()
        assert released >= 1
        _eventually(
            "releaseAllIdle reduces remote tagged sandboxes",
            lambda: self.pool.snapshot().idle_count == 0
            and _count_tagged_sandboxes(self.manager, self.tag) == 0,
            timeout=timedelta(seconds=60),
        )

        self.pool.resize(1)
        _eventually(
            "resize from zero to positive rewarms idle",
            lambda: self.pool.snapshot().state == PoolState.HEALTHY
            and self.pool.snapshot().idle_count >= 1,
        )

    @pytest.mark.timeout(360)
    def test_concurrent_acquire_resize_and_shutdown_do_not_duplicate_or_deadlock(self) -> None:
        _eventually("pool reaches target idle", lambda: self.pool.snapshot().idle_count >= MAX_IDLE)

        start = threading.Event()
        acquired_ids: set[str] = set()
        acquired_lock = threading.Lock()
        errors: list[BaseException] = []

        def worker(index: int) -> None:
            try:
                start.wait()
                sandbox = self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
                self.borrowed.append(sandbox)
                with acquired_lock:
                    assert sandbox.id not in acquired_ids
                    acquired_ids.add(sandbox.id)
                result = sandbox.commands.run(f"echo py-pool-concurrent-{index}")
                assert result.error is None
            except BaseException as exc:
                errors.append(exc)
                raise

        with ThreadPoolExecutor(max_workers=4) as executor:
            futures = [executor.submit(worker, i) for i in range(4)]
            start.set()
            for future in as_completed(futures, timeout=180):
                future.result()

        assert not errors
        assert len(acquired_ids) == 4
        assert _count_tagged_sandboxes(self.manager, self.tag) <= 8

        # Race acquire and graceful shutdown. POOL_NOT_RUNNING is the expected rejected path.
        self.pool.resize(1)
        self.pool.start()
        _eventually("pool rewarmed before shutdown race", lambda: self.pool.snapshot().idle_count >= 1)
        race_errors: list[BaseException] = []
        start.clear()

        def acquire_during_shutdown() -> None:
            try:
                start.wait()
                sandbox = self.pool.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
                self.borrowed.append(sandbox)
            except PoolNotRunningException:
                return
            except BaseException as exc:
                race_errors.append(exc)
                raise

        def shutdown_during_acquire() -> None:
            start.wait()
            self.pool.shutdown(True)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(acquire_during_shutdown) for _ in range(4)]
            futures.append(executor.submit(shutdown_during_acquire))
            start.set()
            for future in as_completed(futures, timeout=180):
                future.result()

        assert not race_errors

    @pytest.mark.timeout(360)
    def test_concurrent_start_shutdown_stress_single_node(self) -> None:
        errors: list[BaseException] = []
        start = threading.Event()

        def worker(index: int) -> None:
            try:
                start.wait()
                for _ in range(3):
                    if index % 2 == 0:
                        self.pool.start()
                    else:
                        self.pool.shutdown(index % 3 == 0)
                    time.sleep(0.05)
            except BaseException as exc:
                errors.append(exc)
                raise

        with ThreadPoolExecutor(max_workers=4) as executor:
            futures = [executor.submit(worker, i) for i in range(4)]
            start.set()
            for future in as_completed(futures, timeout=180):
                future.result()

        assert not errors
        self.pool.start()
        _eventually("pool remains usable after lifecycle stress", lambda: self.pool.snapshot().idle_count >= 1)

    @pytest.mark.timeout(300)
    def test_warmup_preparer_and_pool_isolation(self) -> None:
        _cleanup_pool(self.pool)

        marker_path = f"/tmp/{self.tag}-prepared.txt"

        def preparer(sandbox: SandboxSync) -> None:
            result = sandbox.commands.run(f"printf prepared > {marker_path}")
            assert result.error is None

        prepared_pool = _create_pool(
            pool_name=f"prepared-{self.pool_name}",
            owner_id=f"prepared-owner-{self.tag}",
            state_store=InMemoryPoolStateStore(),
            tag=self.tag,
            max_idle=1,
            warmup_sandbox_preparer=preparer,
        )
        other_tag = _tag("py-pool-other")
        other_pool = _create_pool(
            pool_name=f"pool-{other_tag}",
            owner_id=f"owner-{other_tag}",
            state_store=InMemoryPoolStateStore(),
            tag=other_tag,
            max_idle=1,
        )
        other_manager = SandboxManagerSync.create(create_connection_config_sync())
        try:
            prepared_pool.start()
            _eventually("prepared pool warms", lambda: prepared_pool.snapshot().idle_count >= 1)
            sandbox = prepared_pool.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)
            self.borrowed.append(sandbox)
            result = sandbox.commands.run(f"cat {marker_path}")
            assert result.error is None
            assert result.logs.stdout[0].text == "prepared"

            other_pool.start()
            _eventually("other pool warms", lambda: other_pool.snapshot().idle_count >= 1)
            assert _count_tagged_sandboxes(self.manager, self.tag) >= 1
            assert _count_tagged_sandboxes(other_manager, other_tag) >= 1

            prepared_pool.resize(0)
            prepared_pool.release_all_idle()
            _eventually("prepared pool drains", lambda: prepared_pool.snapshot().idle_count == 0)
            assert other_pool.snapshot().idle_count >= 1
        finally:
            _cleanup_pool(prepared_pool)
            _cleanup_pool(other_pool)
            _cleanup_tagged_sandboxes(other_manager, other_tag)
            other_manager.close()

    @pytest.mark.timeout(240)
    def test_broken_connection_degrades_and_healthy_pool_still_works(self) -> None:
        _cleanup_pool(self.pool)
        bad_tag = _tag("py-pool-bad")
        bad_pool = _create_pool(
            pool_name=f"bad-{self.pool_name}",
            owner_id=f"bad-owner-{self.tag}",
            state_store=InMemoryPoolStateStore(),
            tag=bad_tag,
            max_idle=1,
            connection_config=_broken_connection_config(),
            degraded_threshold=1,
            warmup_ready_timeout=timedelta(seconds=1),
            acquire_ready_timeout=timedelta(seconds=1),
        )
        try:
            bad_pool.start()
            _eventually(
                "bad pool enters degraded state",
                lambda: bad_pool.snapshot().state == PoolState.DEGRADED,
                timeout=timedelta(seconds=60),
                interval=timedelta(seconds=1),
            )
            snapshot = bad_pool.snapshot()
            assert snapshot.last_error
            assert snapshot.idle_count == 0
            with pytest.raises(PoolEmptyException):
                bad_pool.acquire(timedelta(minutes=1), AcquirePolicy.FAIL_FAST)
            with pytest.raises(Exception):
                bad_pool.acquire(timedelta(minutes=1), AcquirePolicy.DIRECT_CREATE)
        finally:
            _cleanup_pool(bad_pool)
            _cleanup_tagged_sandboxes(self.manager, bad_tag)

        healthy_tag = _tag("py-pool-good")
        healthy_pool = _create_pool(
            pool_name=f"healthy-{self.pool_name}",
            owner_id=f"healthy-owner-{self.tag}",
            state_store=InMemoryPoolStateStore(),
            tag=healthy_tag,
            max_idle=1,
        )
        try:
            healthy_pool.start()
            _eventually(
                "healthy pool still warms after broken pool path",
                lambda: healthy_pool.snapshot().idle_count >= 1,
            )
            sandbox = healthy_pool.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)
            self.borrowed.append(sandbox)
            assert sandbox.is_healthy()
        finally:
            _cleanup_pool(healthy_pool)
            _cleanup_tagged_sandboxes(self.manager, healthy_tag)


@pytest.mark.e2e
class TestSandboxPoolPseudoDistributedE2ESync:
    """Multiple pool instances sharing a pseudo-distributed state store."""

    def setup_method(self) -> None:
        self.manager = SandboxManagerSync.create(create_connection_config_sync())
        self.borrowed: list[SandboxSync] = []
        self.pools: list[SandboxPoolSync] = []
        self.tag = _tag("py-pool-dist")

    def teardown_method(self) -> None:
        _cleanup_borrowed(self.borrowed)
        for pool in self.pools:
            _cleanup_pool(pool)
        _cleanup_tagged_sandboxes(self.manager, self.tag)
        self.manager.close()

    @pytest.mark.timeout(360)
    def test_cross_node_acquire_resize_propagation_and_single_writer(self) -> None:
        pool_name = f"pool-dist-{self.tag}"
        store = PseudoDistributedPoolStateStore()
        pool_a = _create_pool(pool_name, f"owner-a-{self.tag}", store, self.tag, 2)
        pool_b = _create_pool(pool_name, f"owner-b-{self.tag}", store, self.tag, 2)
        self.pools.extend([pool_a, pool_b])

        pool_a.start()
        pool_b.start()
        _eventually("distributed pool warms", lambda: pool_a.snapshot().idle_count >= 1)

        sandbox = pool_b.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)
        self.borrowed.append(sandbox)
        assert sandbox.is_healthy()
        result = sandbox.commands.run("echo py-dist-acquire-ok")
        assert result.error is None

        _eventually("primary owner established", lambda: store.current_owner(pool_name) is not None)
        time.sleep(3)
        put_counts = store.put_counts_by_owner(pool_name)
        assert len(put_counts) == 1
        assert store.current_owner(pool_name) in put_counts

        pool_b.resize(0)
        pool_a.release_all_idle()
        pool_b.release_all_idle()
        _eventually("distributed idle drains", lambda: pool_a.snapshot().idle_count == 0)
        time.sleep(RECONCILE_INTERVAL.total_seconds() * 2)
        assert pool_a.snapshot().idle_count == 0
        with pytest.raises(PoolEmptyException):
            pool_a.acquire(timedelta(minutes=2), AcquirePolicy.FAIL_FAST)

    @pytest.mark.timeout(360)
    def test_primary_failover_after_leader_shutdown(self) -> None:
        pool_name = f"pool-failover-{self.tag}"
        owner_a = f"owner-a-{self.tag}"
        owner_b = f"owner-b-{self.tag}"
        store = PseudoDistributedPoolStateStore()
        pool_a = _create_pool(pool_name, owner_a, store, self.tag, 1)
        pool_b = _create_pool(pool_name, owner_b, store, self.tag, 1)
        self.pools.extend([pool_a, pool_b])

        pool_a.start()
        pool_b.start()
        _eventually("primary owner established", lambda: store.current_owner(pool_name) is not None)
        first_owner = store.current_owner(pool_name)
        assert first_owner in {owner_a, owner_b}

        leader = pool_a if first_owner == owner_a else pool_b
        expected_next = owner_b if first_owner == owner_a else owner_a
        leader.shutdown(False)

        _eventually(
            "primary owner fails over",
            lambda: store.current_owner(pool_name) == expected_next,
            timeout=timedelta(seconds=45),
            interval=timedelta(milliseconds=500),
        )

    @pytest.mark.timeout(360)
    def test_lost_lock_window_drops_orphans_and_keeps_remote_count_bounded(self) -> None:
        pool_name = f"pool-renew-window-{self.tag}"
        owner = f"owner-a-{self.tag}"
        store = PseudoDistributedPoolStateStore()
        store.set_fail_renew_when_put_count_at_least(pool_name, owner, 1)
        pool = _create_pool(pool_name, owner, store, self.tag, 2)
        self.pools.append(pool)

        pool.start()
        _eventually("renew failure leaves one idle", lambda: pool.snapshot().idle_count == 1)
        time.sleep(3)
        assert pool.snapshot().idle_count == 1
        _eventually(
            "remote count remains bounded after orphan cleanup",
            lambda: _count_tagged_sandboxes(self.manager, self.tag) <= 2,
            timeout=timedelta(seconds=45),
        )

    @pytest.mark.timeout(420)
    def test_jitter_follower_acquire_and_node_restart_stay_bounded(self) -> None:
        pool_name = f"pool-jitter-{self.tag}"
        owner_a = f"owner-a-{self.tag}"
        owner_b = f"owner-b-{self.tag}"
        store = PseudoDistributedPoolStateStore()
        pool_a = _create_pool(pool_name, owner_a, store, self.tag, 1)
        pool_b = _create_pool(pool_name, owner_b, store, self.tag, 1)
        self.pools.extend([pool_a, pool_b])
        pool_a.start()
        pool_b.start()
        _eventually("distributed pool warms before jitter", lambda: pool_a.snapshot().idle_count >= 1)

        for i in range(6):
            pool_a.resize(i % 3)
            pool_b.resize((i + 1) % 3)
            time.sleep(0.2)
        pool_a.resize(1)
        pool_b.resize(1)

        _eventually(
            "idle remains bounded after maxIdle jitter",
            lambda: pool_a.snapshot().idle_count <= 2,
            timeout=timedelta(seconds=45),
        )
        assert _count_tagged_sandboxes(self.manager, self.tag) <= 3

        _eventually(
            "leader exists before follower acquire",
            lambda: store.current_owner(pool_name) is not None,
            timeout=timedelta(seconds=30),
            interval=timedelta(milliseconds=500),
        )
        current_owner = store.current_owner(pool_name)
        assert current_owner in {owner_a, owner_b}
        leader = pool_a if current_owner == owner_a else pool_b
        follower = pool_b if current_owner == owner_a else pool_a
        expected_next = owner_b if current_owner == owner_a else owner_a

        sandbox = follower.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
        self.borrowed.append(sandbox)
        assert sandbox.is_healthy()
        result = sandbox.commands.run("echo py-follower-acquire-ok")
        assert result.error is None

        leader.shutdown(False)
        _eventually(
            "leadership transfers to follower",
            lambda: store.current_owner(pool_name) == expected_next,
            timeout=timedelta(seconds=45),
            interval=timedelta(milliseconds=500),
        )
        leader.start()
        _eventually(
            "restarted node reports healthy",
            lambda: leader.snapshot().state == PoolState.HEALTHY,
            timeout=timedelta(seconds=45),
        )
        restarted = leader.acquire(timedelta(minutes=5), AcquirePolicy.DIRECT_CREATE)
        self.borrowed.append(restarted)
        assert restarted.is_healthy()
        _eventually(
            "restart does not cause runaway idle pollution",
            lambda: leader.snapshot().idle_count <= 1
            and _count_tagged_sandboxes(self.manager, self.tag) <= 4,
            timeout=timedelta(seconds=45),
        )


@pytest.mark.e2e
class TestSandboxPoolRedisDistributedE2ESync:
    """Redis-backed multi-instance pool E2E scenarios."""

    def setup_method(self) -> None:
        redis_url = os.getenv("OPENSANDBOX_TEST_REDIS_URL")
        if not redis_url:
            pytest.skip("Set OPENSANDBOX_TEST_REDIS_URL to run Redis-backed pool E2E tests")
        redis_module = pytest.importorskip("redis")
        self.redis = redis_module.Redis.from_url(redis_url, decode_responses=True)
        self.key_prefix = f"opensandbox:e2e:{uuid.uuid4()}"
        self.manager = SandboxManagerSync.create(create_connection_config_sync())
        self.borrowed: list[SandboxSync] = []
        self.pools: list[SandboxPoolSync] = []
        self.tag = _tag("py-pool-redis")

    def teardown_method(self) -> None:
        _cleanup_borrowed(self.borrowed)
        for pool in self.pools:
            _cleanup_pool(pool)
        _cleanup_tagged_sandboxes(self.manager, self.tag)
        self.manager.close()
        for key in self.redis.scan_iter(f"{self.key_prefix}:*"):
            self.redis.delete(key)
        self.redis.close()

    @pytest.mark.timeout(360)
    def test_redis_cross_node_acquire_resize_and_failover(self) -> None:
        pool_name = f"redis-pool-{self.tag}"
        store_a = RedisPoolStateStore(self.redis, self.key_prefix)
        store_b = RedisPoolStateStore(self.redis, self.key_prefix)
        pool_a = _create_pool(pool_name, f"owner-a-{self.tag}", store_a, self.tag, 2)
        pool_b = _create_pool(pool_name, f"owner-b-{self.tag}", store_b, self.tag, 2)
        self.pools.extend([pool_a, pool_b])

        pool_a.start()
        pool_b.start()
        _eventually("Redis pool warms", lambda: pool_a.snapshot().idle_count >= 1)

        sandbox = pool_b.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST)
        self.borrowed.append(sandbox)
        assert sandbox.is_healthy()
        result = sandbox.commands.run("echo py-redis-dist-ok")
        assert result.error is None

        pool_b.resize(0)
        _eventually("Redis idle drains after shared resize", lambda: pool_a.snapshot().idle_count == 0)
        time.sleep(RECONCILE_INTERVAL.total_seconds() * 2)
        assert pool_a.snapshot().idle_count == 0
        with pytest.raises(PoolEmptyException):
            pool_a.acquire(timedelta(minutes=2), AcquirePolicy.FAIL_FAST)

        pool_a.shutdown(False)
        pool_b.resize(1)
        _eventually("remaining Redis node replenishes", lambda: pool_b.snapshot().idle_count >= 1)
        assert store_b.try_acquire_primary_lock(
            pool_name, f"owner-b-{self.tag}", PRIMARY_LOCK_TTL
        )

    @pytest.mark.timeout(360)
    def test_redis_concurrent_cross_node_acquire_and_atomic_take(self) -> None:
        pool_name = f"redis-concurrent-{self.tag}"
        store_a = RedisPoolStateStore(self.redis, self.key_prefix)
        store_b = RedisPoolStateStore(self.redis, self.key_prefix)
        pool_a = _create_pool(pool_name, f"owner-a-{self.tag}", store_a, self.tag, 2)
        pool_b = _create_pool(pool_name, f"owner-b-{self.tag}", store_b, self.tag, 2)
        self.pools.extend([pool_a, pool_b])
        pool_a.start()
        pool_b.start()
        _eventually("Redis pool warms two idle", lambda: pool_a.snapshot().idle_count >= 2)

        start = threading.Event()
        with ThreadPoolExecutor(max_workers=2) as executor:
            futures = [
                executor.submit(lambda: (start.wait(), pool_a.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST))[1]),
                executor.submit(lambda: (start.wait(), pool_b.acquire(timedelta(minutes=5), AcquirePolicy.FAIL_FAST))[1]),
            ]
            start.set()
            sandboxes = [future.result(timeout=90) for future in futures]
        self.borrowed.extend(sandboxes)
        ids = {sandbox.id for sandbox in sandboxes}
        assert len(ids) == 2
        assert all(sandbox.is_healthy() for sandbox in sandboxes)

        store = RedisPoolStateStore(self.redis, self.key_prefix)
        contention_pool = f"redis-store-contention-{uuid.uuid4()}"
        for i in range(50):
            store.put_idle(contention_pool, f"id-{i}")

        taken: set[str] = set()
        taken_lock = threading.Lock()

        def take_until_empty() -> None:
            while True:
                sandbox_id = store.try_take_idle(contention_pool)
                if sandbox_id is None:
                    return
                with taken_lock:
                    assert sandbox_id not in taken
                    taken.add(sandbox_id)

        with ThreadPoolExecutor(max_workers=16) as executor:
            futures = [executor.submit(take_until_empty) for _ in range(16)]
            for future in as_completed(futures, timeout=30):
                future.result()

        assert len(taken) == 50
        assert store.snapshot_counters(contention_pool).idle_count == 0


def _create_pool(
    pool_name: str,
    owner_id: str,
    state_store: PoolStateStore,
    tag: str,
    max_idle: int,
    warmup_sandbox_preparer: Callable[[SandboxSync], None] | None = None,
    connection_config: ConnectionConfigSync | None = None,
    degraded_threshold: int = 3,
    warmup_ready_timeout: timedelta = timedelta(seconds=30),
    acquire_ready_timeout: timedelta = timedelta(seconds=30),
) -> SandboxPoolSync:
    return SandboxPoolSync(
        pool_name=pool_name,
        owner_id=owner_id,
        max_idle=max_idle,
        warmup_concurrency=1,
        state_store=state_store,
        connection_config=connection_config or create_connection_config_sync(),
        creation_spec=PoolCreationSpec(
            image=get_sandbox_image(),
            entrypoint=["tail", "-f", "/dev/null"],
            metadata={"tag": tag, "suite": "sandbox-pool-python-e2e"},
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
        degraded_threshold=degraded_threshold,
        warmup_ready_timeout=warmup_ready_timeout,
        acquire_ready_timeout=acquire_ready_timeout,
    )


def _broken_connection_config() -> ConnectionConfigSync:
    return ConnectionConfigSync(
        domain="127.0.0.1:9",
        api_key="broken-e2e-test",
        request_timeout=timedelta(seconds=1),
        transport=httpx.HTTPTransport(
            limits=httpx.Limits(max_connections=2, max_keepalive_connections=0)
        ),
    )


def _eventually(
    description: str,
    condition: Callable[[], bool],
    timeout: timedelta = AWAIT_TIMEOUT,
    interval: timedelta = timedelta(seconds=1),
) -> None:
    deadline = time.monotonic() + timeout.total_seconds()
    last_error: BaseException | None = None
    while time.monotonic() < deadline:
        try:
            if condition():
                return
        except BaseException as exc:
            last_error = exc
        time.sleep(interval.total_seconds())
    if last_error is not None:
        raise AssertionError(f"Timed out waiting for {description}") from last_error
    raise AssertionError(f"Timed out waiting for {description}")


def _cleanup_pool(pool: SandboxPoolSync) -> None:
    try:
        pool.resize(0)
    except Exception:
        pass
    try:
        pool.release_all_idle()
    except Exception:
        pass
    try:
        pool.shutdown(False)
    except Exception:
        pass


def _cleanup_borrowed(sandboxes: list[SandboxSync]) -> None:
    for sandbox in sandboxes:
        try:
            sandbox.kill()
        except Exception:
            pass
        try:
            sandbox.close()
        except Exception:
            pass
    sandboxes.clear()


def _cleanup_tagged_sandboxes(manager: SandboxManagerSync, tag: str) -> None:
    try:
        infos = manager.list_sandbox_infos(
            SandboxFilter(metadata={"tag": tag}, page_size=50)
        )
        for info in infos.sandbox_infos:
            try:
                manager.kill_sandbox(info.id)
            except Exception:
                pass
    except Exception:
        pass


def _count_tagged_sandboxes(manager: SandboxManagerSync, tag: str) -> int:
    infos = manager.list_sandbox_infos(SandboxFilter(metadata={"tag": tag}, page_size=50))
    return len(infos.sandbox_infos)


def _tag(prefix: str) -> str:
    return f"{prefix}-{uuid.uuid4().hex[:8]}"


@dataclass
class _PseudoPoolState:
    entries: dict[str, IdleEntry] = field(default_factory=dict)
    queue: deque[str] = field(default_factory=deque)


class PseudoDistributedPoolStateStore:
    """Thread-safe in-process store with real owner/TTL lock semantics."""

    def __init__(self) -> None:
        self._lock = threading.RLock()
        self._pools: dict[str, _PseudoPoolState] = defaultdict(_PseudoPoolState)
        self._max_idle: dict[str, int] = {}
        self._idle_ttl: dict[str, timedelta] = defaultdict(lambda: timedelta(hours=24))
        self._owners: dict[str, tuple[str, datetime]] = {}
        self._put_counts: dict[str, dict[str, int]] = defaultdict(lambda: defaultdict(int))
        self._fail_renew_after_puts: dict[tuple[str, str], int] = {}

    def try_take_idle(self, pool_name: str) -> str | None:
        with self._lock:
            state = self._pools[pool_name]
            now = _now()
            while state.queue:
                sandbox_id = state.queue.popleft()
                entry = state.entries.pop(sandbox_id, None)
                if entry is None:
                    continue
                if entry.expires_at > now:
                    return sandbox_id
            return None

    def put_idle(self, pool_name: str, sandbox_id: str) -> None:
        if not sandbox_id or not sandbox_id.strip():
            raise ValueError("sandbox_id must not be blank")
        with self._lock:
            state = self._pools[pool_name]
            owner = self.current_owner(pool_name)
            if owner is not None:
                self._put_counts[pool_name][owner] += 1
            expires_at = _now() + self._idle_ttl[pool_name]
            if sandbox_id not in state.entries:
                state.queue.append(sandbox_id)
            state.entries.setdefault(sandbox_id, IdleEntry(sandbox_id, expires_at))

    def remove_idle(self, pool_name: str, sandbox_id: str) -> None:
        with self._lock:
            self._pools[pool_name].entries.pop(sandbox_id, None)

    def try_acquire_primary_lock(
        self, pool_name: str, owner_id: str, ttl: timedelta
    ) -> bool:
        with self._lock:
            current = self._owners.get(pool_name)
            now = _now()
            if current is None or current[1] <= now or current[0] == owner_id:
                self._owners[pool_name] = (owner_id, now + ttl)
                return True
            return False

    def renew_primary_lock(
        self, pool_name: str, owner_id: str, ttl: timedelta
    ) -> bool:
        with self._lock:
            threshold = self._fail_renew_after_puts.get((pool_name, owner_id))
            if threshold is not None and self._put_counts[pool_name][owner_id] >= threshold:
                self._owners.pop(pool_name, None)
                return False
            current = self._owners.get(pool_name)
            now = _now()
            if current is None or current[0] != owner_id or current[1] <= now:
                return False
            self._owners[pool_name] = (owner_id, now + ttl)
            return True

    def release_primary_lock(self, pool_name: str, owner_id: str) -> None:
        with self._lock:
            current = self._owners.get(pool_name)
            if current is not None and current[0] == owner_id:
                self._owners.pop(pool_name, None)

    def reap_expired_idle(self, pool_name: str, now: datetime) -> None:
        with self._lock:
            state = self._pools[pool_name]
            expired = [
                sandbox_id
                for sandbox_id, entry in state.entries.items()
                if entry.expires_at <= now
            ]
            for sandbox_id in expired:
                state.entries.pop(sandbox_id, None)
            if expired:
                state.queue = deque(
                    sandbox_id for sandbox_id in state.queue if sandbox_id in state.entries
                )

    def snapshot_counters(self, pool_name: str) -> StoreCounters:
        with self._lock:
            self.reap_expired_idle(pool_name, _now())
            return StoreCounters(idle_count=len(self._pools[pool_name].entries))

    def snapshot_idle_entries(self, pool_name: str) -> list[IdleEntry]:
        with self._lock:
            self.reap_expired_idle(pool_name, _now())
            state = self._pools[pool_name]
            return [
                entry
                for sandbox_id in state.queue
                if (entry := state.entries.get(sandbox_id)) is not None
            ]

    def get_max_idle(self, pool_name: str) -> int | None:
        with self._lock:
            return self._max_idle.get(pool_name)

    def set_max_idle(self, pool_name: str, max_idle: int) -> None:
        if max_idle < 0:
            raise ValueError("max_idle must be >= 0")
        with self._lock:
            self._max_idle[pool_name] = max_idle

    def set_idle_entry_ttl(self, pool_name: str, idle_ttl: timedelta) -> None:
        if idle_ttl.total_seconds() <= 0:
            raise ValueError("idle_ttl must be positive")
        with self._lock:
            self._idle_ttl[pool_name] = idle_ttl

    def current_owner(self, pool_name: str) -> str | None:
        with self._lock:
            current = self._owners.get(pool_name)
            if current is None:
                return None
            if current[1] <= _now():
                self._owners.pop(pool_name, None)
                return None
            return current[0]

    def put_counts_by_owner(self, pool_name: str) -> dict[str, int]:
        with self._lock:
            return dict(self._put_counts[pool_name])

    def set_fail_renew_when_put_count_at_least(
        self, pool_name: str, owner_id: str, count: int
    ) -> None:
        with self._lock:
            self._fail_renew_after_puts[(pool_name, owner_id)] = count


def _now() -> datetime:
    return datetime.now(timezone.utc)
