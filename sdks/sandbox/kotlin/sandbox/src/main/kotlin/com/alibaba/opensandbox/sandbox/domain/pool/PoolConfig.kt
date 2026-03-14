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

package com.alibaba.opensandbox.sandbox.domain.pool

import com.alibaba.opensandbox.sandbox.config.ConnectionConfig
import java.util.UUID
import kotlin.math.ceil

/**
 * Configuration for a client-side sandbox pool.
 *
 * @property poolName User-defined name and namespace for this logical pool (required).
 * @property ownerId Unique process identity for primary lock ownership. If not provided, a UUID-based default is generated.
 * @property maxIdle Standby idle target/cap (required).
 * @property warmupConcurrency Max concurrent creation workers during replenish (default: max(1, ceil(maxIdle * 0.2))).
 * @property primaryLockTtl Lock TTL for distributed primary ownership (default: 60s).
 * @property stateStore Injected [PoolStateStore] implementation (required).
 * @property connectionConfig Connection config for lifecycle API (required).
 * @property creationSpec Template for creating sandboxes (replenish and direct-create) (required).
 * @property reconcileInterval Interval between reconcile ticks (default: 30s).
 * @property degradedThreshold Consecutive create failures required to transition to DEGRADED (default: 3).
 * @property drainTimeout Max wait during graceful shutdown for in-flight ops (default: 30s).
 */
data class PoolConfig(
    val poolName: String,
    val ownerId: String,
    val maxIdle: Int,
    val warmupConcurrency: Int,
    val primaryLockTtl: java.time.Duration,
    val stateStore: PoolStateStore,
    val connectionConfig: ConnectionConfig,
    val creationSpec: PoolCreationSpec,
    val reconcileInterval: java.time.Duration,
    val degradedThreshold: Int,
    val drainTimeout: java.time.Duration,
) {
    init {
        require(poolName.isNotBlank()) { "poolName must not be blank" }
        require(ownerId.isNotBlank()) { "ownerId must not be blank" }
        require(maxIdle >= 0) { "maxIdle must be >= 0" }
        require(warmupConcurrency > 0) { "warmupConcurrency must be positive" }
        require(degradedThreshold > 0) { "degradedThreshold must be positive" }
        require(!reconcileInterval.isNegative && !reconcileInterval.isZero) { "reconcileInterval must be positive" }
        require(!primaryLockTtl.isNegative && !primaryLockTtl.isZero) { "primaryLockTtl must be positive" }
        require(!drainTimeout.isNegative) { "drainTimeout must be non-negative" }
    }

    companion object {
        private val DEFAULT_RECONCILE_INTERVAL = java.time.Duration.ofSeconds(30)
        private val DEFAULT_PRIMARY_LOCK_TTL = java.time.Duration.ofSeconds(60)
        private const val DEFAULT_DEGRADED_THRESHOLD = 3
        private val DEFAULT_DRAIN_TIMEOUT = java.time.Duration.ofSeconds(30)

        @JvmStatic
        fun builder(): Builder = Builder()
    }

    class Builder {
        private var poolName: String? = null
        private var ownerId: String? = null
        private var maxIdle: Int? = null
        private var warmupConcurrency: Int? = null
        private var primaryLockTtl: java.time.Duration = DEFAULT_PRIMARY_LOCK_TTL
        private var stateStore: PoolStateStore? = null
        private var connectionConfig: ConnectionConfig? = null
        private var creationSpec: PoolCreationSpec? = null
        private var reconcileInterval: java.time.Duration = DEFAULT_RECONCILE_INTERVAL
        private var degradedThreshold: Int = DEFAULT_DEGRADED_THRESHOLD
        private var drainTimeout: java.time.Duration = DEFAULT_DRAIN_TIMEOUT

        fun poolName(poolName: String): Builder {
            this.poolName = poolName
            return this
        }

        fun ownerId(ownerId: String): Builder {
            this.ownerId = ownerId
            return this
        }

        fun maxIdle(maxIdle: Int): Builder {
            this.maxIdle = maxIdle
            return this
        }

        fun warmupConcurrency(warmupConcurrency: Int): Builder {
            this.warmupConcurrency = warmupConcurrency
            return this
        }

        fun primaryLockTtl(primaryLockTtl: java.time.Duration): Builder {
            this.primaryLockTtl = primaryLockTtl
            return this
        }

        fun stateStore(stateStore: PoolStateStore): Builder {
            this.stateStore = stateStore
            return this
        }

        fun connectionConfig(connectionConfig: ConnectionConfig): Builder {
            this.connectionConfig = connectionConfig
            return this
        }

        fun creationSpec(creationSpec: PoolCreationSpec): Builder {
            this.creationSpec = creationSpec
            return this
        }

        fun reconcileInterval(reconcileInterval: java.time.Duration): Builder {
            this.reconcileInterval = reconcileInterval
            return this
        }

        fun degradedThreshold(degradedThreshold: Int): Builder {
            this.degradedThreshold = degradedThreshold
            return this
        }

        fun drainTimeout(drainTimeout: java.time.Duration): Builder {
            this.drainTimeout = drainTimeout
            return this
        }

        private fun generateDefaultOwnerId(): String {
            return "pool-owner-${UUID.randomUUID()}"
        }

        fun build(): PoolConfig {
            val name = poolName ?: throw IllegalArgumentException("poolName is required")
            val owner = ownerId ?: generateDefaultOwnerId()
            val max = maxIdle ?: throw IllegalArgumentException("maxIdle is required")
            val store = stateStore ?: throw IllegalArgumentException("stateStore is required")
            val conn = connectionConfig ?: throw IllegalArgumentException("connectionConfig is required")
            val spec = creationSpec ?: throw IllegalArgumentException("creationSpec is required")

            val warmup = warmupConcurrency ?: ceil(max * 0.2).toInt().coerceAtLeast(1)

            return PoolConfig(
                poolName = name,
                ownerId = owner,
                maxIdle = max,
                warmupConcurrency = warmup,
                primaryLockTtl = primaryLockTtl,
                stateStore = store,
                connectionConfig = conn,
                creationSpec = spec,
                reconcileInterval = reconcileInterval,
                degradedThreshold = degradedThreshold,
                drainTimeout = drainTimeout,
            )
        }
    }
}
