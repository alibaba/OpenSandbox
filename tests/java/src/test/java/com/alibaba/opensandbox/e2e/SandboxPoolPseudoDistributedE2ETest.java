/*
 * Copyright 2025 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package com.alibaba.opensandbox.e2e;

import static org.junit.jupiter.api.Assertions.*;

import com.alibaba.opensandbox.sandbox.Sandbox;
import com.alibaba.opensandbox.sandbox.SandboxManager;
import com.alibaba.opensandbox.sandbox.domain.exceptions.PoolEmptyException;
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.Execution;
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.RunCommandRequest;
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PagedSandboxInfos;
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxFilter;
import com.alibaba.opensandbox.sandbox.domain.pool.AcquirePolicy;
import com.alibaba.opensandbox.sandbox.domain.pool.IdleEntry;
import com.alibaba.opensandbox.sandbox.domain.pool.PoolCreationSpec;
import com.alibaba.opensandbox.sandbox.domain.pool.PoolStateStore;
import com.alibaba.opensandbox.sandbox.domain.pool.StoreCounters;
import com.alibaba.opensandbox.sandbox.pool.SandboxPool;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.function.BooleanSupplier;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

@Tag("e2e")
@DisplayName("SandboxPool E2E Tests (Pseudo-Distributed)")
public class SandboxPoolPseudoDistributedE2ETest extends BaseE2ETest {
    private static final Duration RECONCILE_INTERVAL = Duration.ofSeconds(1);
    private static final Duration PRIMARY_LOCK_TTL = Duration.ofSeconds(2);
    private static final Duration DRAIN_TIMEOUT = Duration.ofMillis(200);
    private static final Duration AWAIT_TIMEOUT = Duration.ofMinutes(2);

    private final List<SandboxPool> pools = new ArrayList<>();
    private final List<Sandbox> borrowed = new CopyOnWriteArrayList<>();

    private SandboxManager sandboxManager;
    private String tag;

    @AfterEach
    void teardown() {
        for (Sandbox sandbox : borrowed) {
            killAndCloseQuietly(sandbox);
        }
        borrowed.clear();

        for (SandboxPool pool : pools) {
            try {
                pool.resize(0);
                pool.releaseAllIdle();
            } catch (Exception ignored) {
            }
            try {
                pool.shutdown(false);
            } catch (Exception ignored) {
            }
        }
        pools.clear();

        if (sandboxManager != null && tag != null) {
            cleanupTaggedSandboxes(tag);
        }
        if (sandboxManager != null) {
            try {
                sandboxManager.close();
            } catch (Exception ignored) {
            }
        }
    }

    @Test
    @DisplayName("shared distributed store supports cross-node acquire and resize propagation")
    @Timeout(value = 6, unit = java.util.concurrent.TimeUnit.MINUTES)
    void testCrossNodeAcquireAndResizePropagation() throws Exception {
        tag = "e2e-pool-dist-" + UUID.randomUUID().toString().substring(0, 8);
        String poolName = "pool-dist-" + tag;
        String ownerA = "owner-a-" + tag;
        String ownerB = "owner-b-" + tag;
        sandboxManager = SandboxManager.builder().connectionConfig(sharedConnectionConfig).build();

        PseudoDistributedPoolStateStore store = new PseudoDistributedPoolStateStore();
        SandboxPool poolA = createPool(poolName, ownerA, store, 2);
        SandboxPool poolB = createPool(poolName, ownerB, store, 2);
        pools.add(poolA);
        pools.add(poolB);

        poolA.start();
        poolB.start();

        eventually(
                "distributed pool warmed idle",
                AWAIT_TIMEOUT,
                Duration.ofSeconds(2),
                () -> poolA.snapshot().getIdleCount() >= 1);

        Sandbox sandbox = poolB.acquire(Duration.ofMinutes(5), AcquirePolicy.FAIL_FAST);
        borrowed.add(sandbox);
        assertTrue(sandbox.isHealthy(), "cross-node acquire should return healthy sandbox");
        Execution result =
                sandbox.commands()
                        .run(RunCommandRequest.builder().command("echo dist-acquire-ok").build());
        assertNotNull(result);
        assertNull(result.getError());

        // Resize from one node should propagate through shared store and be honored by leader
        // reconcile.
        poolB.resize(0);
        int released = poolA.releaseAllIdle();
        assertTrue(released >= 0, "releaseAllIdle should be non-negative");
        eventually(
                "idle drained after resize to zero",
                Duration.ofSeconds(30),
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getIdleCount() == 0);
        Thread.sleep(RECONCILE_INTERVAL.multipliedBy(3).toMillis());
        assertTrue(
                poolA.snapshot().getIdleCount() <= 1,
                "idle should stay bounded after resize(0) across distributed nodes");
        poolA.releaseAllIdle();
        poolB.releaseAllIdle();
        eventually(
                "idle converges back to zero after best-effort drain",
                Duration.ofSeconds(20),
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getIdleCount() == 0);

        assertThrows(
                PoolEmptyException.class,
                () -> poolA.acquire(Duration.ofMinutes(2), AcquirePolicy.FAIL_FAST));
    }

    @Test
    @DisplayName("primary lock failover occurs after leader shutdown")
    @Timeout(value = 6, unit = java.util.concurrent.TimeUnit.MINUTES)
    void testPrimaryFailoverAfterLeaderShutdown() throws Exception {
        tag = "e2e-pool-failover-" + UUID.randomUUID().toString().substring(0, 8);
        String poolName = "pool-failover-" + tag;
        String ownerA = "owner-a-" + tag;
        String ownerB = "owner-b-" + tag;
        sandboxManager = SandboxManager.builder().connectionConfig(sharedConnectionConfig).build();

        PseudoDistributedPoolStateStore store = new PseudoDistributedPoolStateStore();
        SandboxPool poolA = createPool(poolName, ownerA, store, 1);
        SandboxPool poolB = createPool(poolName, ownerB, store, 1);
        pools.add(poolA);
        pools.add(poolB);

        poolA.start();
        poolB.start();

        eventually(
                "one owner becomes primary",
                Duration.ofSeconds(30),
                Duration.ofMillis(500),
                () -> store.currentOwner(poolName) != null);
        String firstOwner = store.currentOwner(poolName);
        assertNotNull(firstOwner, "primary owner should be established");

        SandboxPool leader = firstOwner.equals(ownerA) ? poolA : poolB;
        String expectedNextOwner = firstOwner.equals(ownerA) ? ownerB : ownerA;
        leader.shutdown(false);

        eventually(
                "primary owner fails over after ttl",
                Duration.ofSeconds(30),
                Duration.ofMillis(500),
                () -> expectedNextOwner.equals(store.currentOwner(poolName)));
    }

    @Test
    @DisplayName("only one owner writes idle in steady-state (dual-primary guard)")
    @Timeout(value = 6, unit = java.util.concurrent.TimeUnit.MINUTES)
    void testOnlyOneOwnerWritesIdleInSteadyState() throws Exception {
        tag = "e2e-pool-single-writer-" + UUID.randomUUID().toString().substring(0, 8);
        String poolName = "pool-single-writer-" + tag;
        String ownerA = "owner-a-" + tag;
        String ownerB = "owner-b-" + tag;
        sandboxManager = SandboxManager.builder().connectionConfig(sharedConnectionConfig).build();

        PseudoDistributedPoolStateStore store = new PseudoDistributedPoolStateStore();
        SandboxPool poolA = createPool(poolName, ownerA, store, 1);
        SandboxPool poolB = createPool(poolName, ownerB, store, 1);
        pools.add(poolA);
        pools.add(poolB);
        poolA.start();
        poolB.start();

        eventually(
                "pool warms and gets a primary owner",
                AWAIT_TIMEOUT,
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getIdleCount() >= 1 && store.currentOwner(poolName) != null);
        Thread.sleep(Duration.ofSeconds(3).toMillis());

        Map<String, Integer> putCounts = store.putCountsByOwner(poolName);
        assertEquals(
                1, putCounts.size(), "idle writes in steady-state should come from one owner only");
        assertTrue(
                putCounts.containsKey(store.currentOwner(poolName)),
                "steady-state writer should match current primary owner");
    }

    @Test
    @DisplayName(
            "renew failure window drops extra create and orphan cleanup keeps remote count bounded")
    @Timeout(value = 6, unit = java.util.concurrent.TimeUnit.MINUTES)
    void testRenewFailureWindowAndOrphanCleanupBoundedResources() throws Exception {
        tag = "e2e-pool-renew-window-" + UUID.randomUUID().toString().substring(0, 8);
        String poolName = "pool-renew-window-" + tag;
        String ownerA = "owner-a-" + tag;
        sandboxManager = SandboxManager.builder().connectionConfig(sharedConnectionConfig).build();

        PseudoDistributedPoolStateStore store = new PseudoDistributedPoolStateStore();
        // Once one idle is put by ownerA, all subsequent renews for ownerA fail.
        store.setFailRenewWhenPutCountAtLeast(poolName, ownerA, 1);

        SandboxPool poolA = createPool(poolName, ownerA, store, 2);
        pools.add(poolA);
        poolA.start();

        eventually(
                "renew failure window leaves only one idle",
                Duration.ofSeconds(45),
                Duration.ofMillis(500),
                () -> poolA.snapshot().getIdleCount() == 1);
        Thread.sleep(Duration.ofSeconds(3).toMillis());
        assertEquals(1, poolA.snapshot().getIdleCount(), "idle should remain bounded at 1");

        eventually(
                "tagged remote sandbox count stays bounded after orphan cleanup",
                Duration.ofSeconds(45),
                Duration.ofSeconds(1),
                () -> countTaggedSandboxes(tag) <= 2);
    }

    @Test
    @DisplayName("concurrent maxIdle jitter converges without runaway over-allocation")
    @Timeout(value = 6, unit = java.util.concurrent.TimeUnit.MINUTES)
    void testMaxIdleJitterConvergesWithoutRunaway() throws Exception {
        tag = "e2e-pool-jitter-" + UUID.randomUUID().toString().substring(0, 8);
        String poolName = "pool-jitter-" + tag;
        String ownerA = "owner-a-" + tag;
        String ownerB = "owner-b-" + tag;
        sandboxManager = SandboxManager.builder().connectionConfig(sharedConnectionConfig).build();

        PseudoDistributedPoolStateStore store = new PseudoDistributedPoolStateStore();
        SandboxPool poolA = createPool(poolName, ownerA, store, 1);
        SandboxPool poolB = createPool(poolName, ownerB, store, 1);
        pools.add(poolA);
        pools.add(poolB);
        poolA.start();
        poolB.start();

        eventually(
                "initial warmup completes",
                AWAIT_TIMEOUT,
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getIdleCount() >= 1);

        for (int i = 0; i < 6; i++) {
            poolA.resize(i % 3);
            poolB.resize((i + 1) % 3);
            Thread.sleep(200);
        }
        poolA.resize(1);
        poolB.resize(1);

        eventually(
                "idle remains bounded after maxIdle jitter",
                Duration.ofSeconds(45),
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getIdleCount() <= 2);
        assertTrue(
                countTaggedSandboxes(tag) <= 3,
                "tagged sandbox count should stay bounded under jitter");
    }

    @Test
    @DisplayName("non-primary acquire remains available while leadership changes")
    @Timeout(value = 6, unit = java.util.concurrent.TimeUnit.MINUTES)
    void testNonPrimaryAcquireDuringLeadershipChanges() throws Exception {
        tag = "e2e-pool-follower-acquire-" + UUID.randomUUID().toString().substring(0, 8);
        String poolName = "pool-follower-acquire-" + tag;
        String ownerA = "owner-a-" + tag;
        String ownerB = "owner-b-" + tag;
        sandboxManager = SandboxManager.builder().connectionConfig(sharedConnectionConfig).build();

        PseudoDistributedPoolStateStore store = new PseudoDistributedPoolStateStore();
        SandboxPool poolA = createPool(poolName, ownerA, store, 1);
        SandboxPool poolB = createPool(poolName, ownerB, store, 1);
        pools.add(poolA);
        pools.add(poolB);
        poolA.start();
        poolB.start();

        eventually(
                "leader is elected",
                Duration.ofSeconds(30),
                Duration.ofMillis(500),
                () -> store.currentOwner(poolName) != null);

        for (int round = 0; round < 2; round++) {
            String currentOwner = store.currentOwner(poolName);
            assertNotNull(currentOwner, "leader should exist before follower acquire");

            SandboxPool leader = currentOwner.equals(ownerA) ? poolA : poolB;
            SandboxPool follower = currentOwner.equals(ownerA) ? poolB : poolA;
            String expectedNextOwner = currentOwner.equals(ownerA) ? ownerB : ownerA;

            Sandbox sandbox = follower.acquire(Duration.ofMinutes(3), AcquirePolicy.DIRECT_CREATE);
            borrowed.add(sandbox);
            Execution execution =
                    sandbox.commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("echo follower-acquire-ok")
                                            .build());
            assertNotNull(execution);
            assertNull(execution.getError());

            leader.shutdown(false);
            eventually(
                    "leadership transfers to follower",
                    Duration.ofSeconds(30),
                    Duration.ofMillis(500),
                    () -> expectedNextOwner.equals(store.currentOwner(poolName)));
            leader.start();
        }
    }

    @Test
    @DisplayName("node restart re-joins cluster without idle pollution")
    @Timeout(value = 6, unit = java.util.concurrent.TimeUnit.MINUTES)
    void testNodeRestartRejoinsWithoutIdlePollution() throws Exception {
        tag = "e2e-pool-restart-" + UUID.randomUUID().toString().substring(0, 8);
        String poolName = "pool-restart-" + tag;
        String ownerA = "owner-a-" + tag;
        String ownerB = "owner-b-" + tag;
        sandboxManager = SandboxManager.builder().connectionConfig(sharedConnectionConfig).build();

        PseudoDistributedPoolStateStore store = new PseudoDistributedPoolStateStore();
        SandboxPool poolA = createPool(poolName, ownerA, store, 1);
        SandboxPool poolB = createPool(poolName, ownerB, store, 1);
        pools.add(poolA);
        pools.add(poolB);
        poolA.start();
        poolB.start();

        eventually(
                "pool warmed before restart",
                AWAIT_TIMEOUT,
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getIdleCount() >= 1);

        poolA.shutdown(false);
        poolA.start();

        eventually(
                "restarted node reports healthy",
                Duration.ofSeconds(45),
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getState().name().equals("HEALTHY"));
        Sandbox sandbox = poolA.acquire(Duration.ofMinutes(3), AcquirePolicy.DIRECT_CREATE);
        borrowed.add(sandbox);
        assertTrue(sandbox.isHealthy(), "restarted node should still serve acquire");

        eventually(
                "idle count stays bounded after restart",
                Duration.ofSeconds(45),
                Duration.ofSeconds(1),
                () -> poolA.snapshot().getIdleCount() <= 1);
        assertTrue(
                countTaggedSandboxes(tag) <= 3, "restart should not cause runaway idle pollution");
    }

    private SandboxPool createPool(
            String poolName, String ownerId, PseudoDistributedPoolStateStore store, int maxIdle) {
        PoolCreationSpec creationSpec =
                PoolCreationSpec.builder()
                        .image(getSandboxImage())
                        .entrypoint(List.of("tail -f /dev/null"))
                        .metadata(Map.of("tag", tag, "suite", "sandbox-pool-pseudo-dist-e2e"))
                        .env(Map.of("E2E_TEST", "true", "EXECD_API_GRACE_SHUTDOWN", "3s", "EXECD_JUPYTER_IDLE_POLL_INTERVAL", "200ms"))
                        .build();
        return SandboxPool.builder()
                .poolName(poolName)
                .ownerId(ownerId)
                .maxIdle(maxIdle)
                .warmupConcurrency(1)
                .stateStore(store)
                .connectionConfig(sharedConnectionConfig)
                .creationSpec(creationSpec)
                .reconcileInterval(RECONCILE_INTERVAL)
                .primaryLockTtl(PRIMARY_LOCK_TTL)
                .drainTimeout(DRAIN_TIMEOUT)
                .build();
    }

    private void cleanupTaggedSandboxes(String cleanupTag) {
        try {
            PagedSandboxInfos infos =
                    sandboxManager.listSandboxInfos(
                            SandboxFilter.builder()
                                    .metadata(Map.of("tag", cleanupTag))
                                    .pageSize(50)
                                    .build());
            infos.getSandboxInfos()
                    .forEach(
                            info -> {
                                try {
                                    sandboxManager.killSandbox(info.getId());
                                } catch (Exception ignored) {
                                }
                            });
        } catch (Exception ignored) {
        }
    }

    private int countTaggedSandboxes(String queryTag) {
        if (queryTag == null || queryTag.isBlank()) {
            return 0;
        }
        PagedSandboxInfos infos =
                sandboxManager.listSandboxInfos(
                        SandboxFilter.builder()
                                .metadata(Map.of("tag", queryTag))
                                .pageSize(50)
                                .build());
        return infos.getSandboxInfos().size();
    }

    private void eventually(
            String description, Duration timeout, Duration interval, BooleanSupplier condition)
            throws InterruptedException {
        long deadline = System.currentTimeMillis() + timeout.toMillis();
        Throwable lastError = null;
        while (System.currentTimeMillis() < deadline) {
            try {
                if (condition.getAsBoolean()) {
                    return;
                }
            } catch (Throwable t) {
                lastError = t;
            }
            Thread.sleep(interval.toMillis());
        }
        if (lastError != null) {
            fail(
                    "Timed out waiting for "
                            + description
                            + ", last error: "
                            + lastError.getMessage());
        } else {
            fail("Timed out waiting for " + description);
        }
    }

    private static void killAndCloseQuietly(Sandbox sandbox) {
        if (sandbox == null) {
            return;
        }
        try {
            sandbox.kill();
        } catch (Exception ignored) {
        }
        try {
            sandbox.close();
        } catch (Exception ignored) {
        }
    }

    /**
     * A thread-safe in-process store that mimics distributed semantics: - shared idle membership by
     * poolName - owner-based primary lock with TTL - shared maxIdle propagation
     */
    static class PseudoDistributedPoolStateStore implements PoolStateStore {
        private static final Duration IDLE_TTL = Duration.ofHours(24);

        private final Map<String, LinkedHashMap<String, Instant>> idleByPool =
                new LinkedHashMap<>();
        private final Map<String, LockEntry> locks = new LinkedHashMap<>();
        private final Map<String, Integer> maxIdleByPool = new LinkedHashMap<>();
        private final Map<String, Map<String, Integer>> putCountByOwnerByPool = new HashMap<>();
        private final Map<String, Map<String, Integer>> renewCountByOwnerByPool = new HashMap<>();
        private final Map<String, Map<String, Integer>> failRenewAfterPutByOwnerByPool =
                new HashMap<>();

        @Override
        public synchronized String tryTakeIdle(String poolName) {
            LinkedHashMap<String, Instant> entries = idleByPool.get(poolName);
            if (entries == null || entries.isEmpty()) {
                return null;
            }
            reapExpiredIdle(poolName, Instant.now());
            entries = idleByPool.get(poolName);
            if (entries == null || entries.isEmpty()) {
                return null;
            }
            Iterator<Map.Entry<String, Instant>> it = entries.entrySet().iterator();
            if (!it.hasNext()) {
                return null;
            }
            Map.Entry<String, Instant> first = it.next();
            it.remove();
            return first.getKey();
        }

        @Override
        public synchronized void putIdle(String poolName, String sandboxId) {
            LinkedHashMap<String, Instant> entries =
                    idleByPool.computeIfAbsent(poolName, ignored -> new LinkedHashMap<>());
            entries.putIfAbsent(sandboxId, Instant.now().plus(IDLE_TTL));
            String owner = currentOwner(poolName);
            if (owner != null) {
                incrementCounter(putCountByOwnerByPool, poolName, owner);
            }
        }

        @Override
        public synchronized void removeIdle(String poolName, String sandboxId) {
            LinkedHashMap<String, Instant> entries = idleByPool.get(poolName);
            if (entries != null) {
                entries.remove(sandboxId);
            }
        }

        @Override
        public synchronized boolean tryAcquirePrimaryLock(
                String poolName, String ownerId, Duration ttl) {
            Instant now = Instant.now();
            LockEntry lock = locks.get(poolName);
            if (lock == null || !lock.expiresAt.isAfter(now) || lock.ownerId.equals(ownerId)) {
                locks.put(poolName, new LockEntry(ownerId, now.plus(ttl)));
                return true;
            }
            return false;
        }

        @Override
        public synchronized boolean renewPrimaryLock(
                String poolName, String ownerId, Duration ttl) {
            Instant now = Instant.now();
            LockEntry lock = locks.get(poolName);
            if (lock == null || !lock.ownerId.equals(ownerId) || !lock.expiresAt.isAfter(now)) {
                return false;
            }
            incrementCounter(renewCountByOwnerByPool, poolName, ownerId);
            if (shouldFailRenewByPutThreshold(poolName, ownerId)) {
                return false;
            }
            locks.put(poolName, new LockEntry(ownerId, now.plus(ttl)));
            return true;
        }

        @Override
        public synchronized void releasePrimaryLock(String poolName, String ownerId) {
            LockEntry lock = locks.get(poolName);
            if (lock != null && lock.ownerId.equals(ownerId)) {
                locks.remove(poolName);
            }
        }

        @Override
        public synchronized void reapExpiredIdle(String poolName, Instant now) {
            LinkedHashMap<String, Instant> entries = idleByPool.get(poolName);
            if (entries == null || entries.isEmpty()) {
                return;
            }
            entries.entrySet().removeIf(e -> !e.getValue().isAfter(now));
        }

        @Override
        public synchronized StoreCounters snapshotCounters(String poolName) {
            reapExpiredIdle(poolName, Instant.now());
            LinkedHashMap<String, Instant> entries = idleByPool.get(poolName);
            return new StoreCounters(entries == null ? 0 : entries.size());
        }

        @Override
        public synchronized List<IdleEntry> snapshotIdleEntries(String poolName) {
            reapExpiredIdle(poolName, Instant.now());
            LinkedHashMap<String, Instant> entries = idleByPool.get(poolName);
            if (entries == null || entries.isEmpty()) {
                return List.of();
            }
            List<IdleEntry> snapshot = new ArrayList<>(entries.size());
            for (Map.Entry<String, Instant> entry : entries.entrySet()) {
                snapshot.add(new IdleEntry(entry.getKey(), entry.getValue()));
            }
            return snapshot;
        }

        @Override
        public synchronized Integer getMaxIdle(String poolName) {
            return maxIdleByPool.get(poolName);
        }

        @Override
        public synchronized void setMaxIdle(String poolName, int maxIdle) {
            maxIdleByPool.put(poolName, maxIdle);
        }

        synchronized String currentOwner(String poolName) {
            LockEntry lock = locks.get(poolName);
            if (lock == null || !lock.expiresAt.isAfter(Instant.now())) {
                return null;
            }
            return lock.ownerId;
        }

        synchronized Map<String, Integer> putCountsByOwner(String poolName) {
            Map<String, Integer> counters = putCountByOwnerByPool.get(poolName);
            if (counters == null) {
                return Map.of();
            }
            return new HashMap<>(counters);
        }

        synchronized void setFailRenewWhenPutCountAtLeast(
                String poolName, String ownerId, int putThreshold) {
            failRenewAfterPutByOwnerByPool
                    .computeIfAbsent(poolName, ignored -> new HashMap<>())
                    .put(ownerId, putThreshold);
        }

        private void incrementCounter(
                Map<String, Map<String, Integer>> table, String poolName, String ownerId) {
            Map<String, Integer> ownerCounts =
                    table.computeIfAbsent(poolName, ignored -> new HashMap<>());
            ownerCounts.put(ownerId, ownerCounts.getOrDefault(ownerId, 0) + 1);
        }

        private boolean shouldFailRenewByPutThreshold(String poolName, String ownerId) {
            Map<String, Integer> thresholds = failRenewAfterPutByOwnerByPool.get(poolName);
            if (thresholds == null) {
                return false;
            }
            Integer threshold = thresholds.get(ownerId);
            if (threshold == null) {
                return false;
            }
            int putCount =
                    putCountByOwnerByPool.getOrDefault(poolName, Map.of()).getOrDefault(ownerId, 0);
            return putCount >= threshold;
        }

        private static class LockEntry {
            private final String ownerId;
            private final Instant expiresAt;

            private LockEntry(String ownerId, Instant expiresAt) {
                this.ownerId = ownerId;
                this.expiresAt = expiresAt;
            }
        }
    }
}
