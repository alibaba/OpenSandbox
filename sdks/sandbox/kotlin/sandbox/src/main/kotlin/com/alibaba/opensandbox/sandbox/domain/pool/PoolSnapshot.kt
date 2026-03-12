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

/**
 * Point-in-time snapshot of pool state for observability.
 *
 * @property state Current pool state (HEALTHY, DEGRADED, DRAINING, STOPPED).
 * @property idleCount Number of idle sandboxes in the store.
 * @property lastError Last error message if pool is DEGRADED or after failure; null otherwise.
 */
data class PoolSnapshot(
    val state: PoolState,
    val idleCount: Int,
    val lastError: String? = null,
)
