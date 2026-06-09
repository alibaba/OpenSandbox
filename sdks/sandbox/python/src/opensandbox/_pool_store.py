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
"""Pool state store implementations."""

from __future__ import annotations

from collections import deque
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from threading import RLock

from opensandbox.pool_types import IdleEntry, StoreCounters, TakeIdleResult


class InMemoryPoolStateStore:
    """Single-process in-memory pool store."""

    def __init__(self) -> None:
        self._default_idle_ttl = timedelta(hours=24)
        self._idle_ttl_by_pool: dict[str, timedelta] = {}
        self._pools: dict[str, _PoolIdleState] = {}
        self._lock = RLock()

    def try_take_idle(self, pool_name: str) -> str | None:
        with self._lock:
            state = self._pools.get(pool_name)
            if state is None:
                return None
            now = _now()
            while state.queue:
                sandbox_id = state.queue.popleft()
                entry = state.entries.pop(sandbox_id, None)
                if entry is None:
                    continue
                if entry.expires_at > now:
                    return sandbox_id
            return None

    def try_take_idle_min_ttl(
        self, pool_name: str, min_remaining_ttl: timedelta
    ) -> TakeIdleResult:
        """Variant of :meth:`try_take_idle` that skips entries with insufficient remaining TTL.

        Returns a :class:`TakeIdleResult` carrying the chosen ID (if any) plus the IDs of
        any **alive** entries skipped because their remaining TTL fell below ``min_remaining_ttl``.
        Already-expired entries are silently dropped — the server has already reaped them.
        """
        if min_remaining_ttl.total_seconds() <= 0:
            return TakeIdleResult(sandbox_id=self.try_take_idle(pool_name))
        with self._lock:
            state = self._pools.get(pool_name)
            if state is None:
                return TakeIdleResult(sandbox_id=None)
            now = _now()
            cutoff = now + min_remaining_ttl
            discarded_alive: list[str] = []
            while state.queue:
                sandbox_id = state.queue.popleft()
                entry = state.entries.pop(sandbox_id, None)
                if entry is None:
                    continue
                if entry.expires_at > cutoff:
                    return TakeIdleResult(
                        sandbox_id=sandbox_id,
                        discarded_alive_sandbox_ids=tuple(discarded_alive),
                    )
                if entry.expires_at > now:
                    discarded_alive.append(sandbox_id)
            return TakeIdleResult(
                sandbox_id=None,
                discarded_alive_sandbox_ids=tuple(discarded_alive),
            )

    def put_idle(self, pool_name: str, sandbox_id: str) -> None:
        if not sandbox_id or not sandbox_id.strip():
            raise ValueError("sandbox_id must not be blank")
        with self._lock:
            state = self._pools.setdefault(pool_name, _PoolIdleState())
            expires_at = _now() + self._resolve_idle_ttl(pool_name)
            if sandbox_id not in state.entries:
                state.queue.append(sandbox_id)
            state.entries.setdefault(sandbox_id, IdleEntry(sandbox_id, expires_at))

    def remove_idle(self, pool_name: str, sandbox_id: str) -> None:
        with self._lock:
            state = self._pools.get(pool_name)
            if state is not None:
                state.entries.pop(sandbox_id, None)

    def try_acquire_primary_lock(
        self, pool_name: str, owner_id: str, ttl: timedelta
    ) -> bool:
        return True

    def renew_primary_lock(
        self, pool_name: str, owner_id: str, ttl: timedelta
    ) -> bool:
        return True

    def release_primary_lock(self, pool_name: str, owner_id: str) -> None:
        return None

    def reap_expired_idle(self, pool_name: str, now: datetime) -> None:
        with self._lock:
            self._reap_locked(pool_name, now)

    def reap_expired_idle_min_ttl(
        self, pool_name: str, now: datetime, min_remaining_ttl: timedelta
    ) -> tuple[str, ...]:
        """Variant of :meth:`reap_expired_idle` that also evicts near-expiry entries.

        Returns IDs of **alive** entries (server-side TTL has not elapsed) that were evicted
        because their remaining TTL fell below ``min_remaining_ttl``. Already-expired entries
        are still evicted but excluded from the return value — the server has reaped them.
        """
        if min_remaining_ttl.total_seconds() <= 0:
            self.reap_expired_idle(pool_name, now)
            return ()
        cutoff = now + min_remaining_ttl
        discarded_alive: list[str] = []
        with self._lock:
            state = self._pools.get(pool_name)
            if state is None:
                return ()
            for sandbox_id, entry in list(state.entries.items()):
                if entry.expires_at > cutoff:
                    continue
                state.entries.pop(sandbox_id, None)
                if entry.expires_at > now:
                    discarded_alive.append(sandbox_id)
            state.queue = deque(
                sandbox_id for sandbox_id in state.queue if sandbox_id in state.entries
            )
        return tuple(discarded_alive)

    def snapshot_counters(self, pool_name: str) -> StoreCounters:
        with self._lock:
            self._reap_locked(pool_name, _now())
            state = self._pools.get(pool_name)
            return StoreCounters(idle_count=0 if state is None else len(state.entries))

    def snapshot_idle_entries(self, pool_name: str) -> list[IdleEntry]:
        with self._lock:
            self._reap_locked(pool_name, _now())
            state = self._pools.get(pool_name)
            if state is None:
                return []
            return [
                entry
                for sandbox_id in state.queue
                if (entry := state.entries.get(sandbox_id)) is not None
            ]

    def get_max_idle(self, pool_name: str) -> int | None:
        return None

    def set_max_idle(self, pool_name: str, max_idle: int) -> None:
        return None

    def set_idle_entry_ttl(self, pool_name: str, idle_ttl: timedelta) -> None:
        if idle_ttl.total_seconds() <= 0:
            raise ValueError("idle_ttl must be positive")
        with self._lock:
            self._idle_ttl_by_pool[pool_name] = idle_ttl

    def _resolve_idle_ttl(self, pool_name: str) -> timedelta:
        return self._idle_ttl_by_pool.get(pool_name, self._default_idle_ttl)

    def _reap_locked(self, pool_name: str, now: datetime) -> None:
        state = self._pools.get(pool_name)
        if state is None:
            return
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


@dataclass
class _PoolIdleState:
    entries: dict[str, IdleEntry] = field(default_factory=dict)
    queue: deque[str] = field(default_factory=deque)


def _now() -> datetime:
    return datetime.now(timezone.utc)
