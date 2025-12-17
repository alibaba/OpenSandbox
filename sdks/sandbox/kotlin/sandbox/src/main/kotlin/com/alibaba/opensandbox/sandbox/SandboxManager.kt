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

package com.alibaba.opensandbox.sandbox

import com.alibaba.opensandbox.sandbox.config.ConnectionConfig
import com.alibaba.opensandbox.sandbox.domain.exceptions.InvalidArgumentException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxException
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PagedSandboxInfos
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxFilter
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxInfo
import com.alibaba.opensandbox.sandbox.domain.services.Sandboxes
import com.alibaba.opensandbox.sandbox.infrastructure.factory.AdapterFactory
import org.slf4j.LoggerFactory
import java.time.Duration
import java.time.OffsetDateTime
import java.util.UUID

/**
 * Sandbox management interface for administrative operations and monitoring sandbox instances.
 *
 * This class provides a centralized interface for managing sandbox instances,
 * enabling administrative operations and sandbox discovery.
 * It focuses on high-level management operations rather than individual sandbox interactions.
 *
 * ## Key Features
 *
 * - **Sandbox Discovery**: List and filter sandbox instances by various criteria
 * - **Administrative Operations**: Individual sandbox management operations
 * - **Connection Pool Management**: Efficient HTTP client reuse for multiple operations
 *
 * ## Usage Example
 *
 * ```kotlin
 * val manager = SandboxManager.builder()
 *     .connectionConfig(connectionConfig)
 *     .build()
 *
 * // List all running sandboxes
 * val runningSandboxes = manager.listSandboxInfos(
 *     SandboxFilter.builder().state("RUNNING").build()
 * )
 *
 * // Individual operations
 * val sandboxId = UUID.fromString("sandbox-id")
 * manager.getSandboxInfo(sandboxId)
 * manager.pauseSandbox(sandboxId)
 * manager.resumeSandbox(sandboxId)
 * manager.killSandbox(sandboxId)
 *
 * // Cleanup
 * manager.close()
 * ```
 *
 * **Note**: This class is designed for administrative operations.
 * For individual sandbox interactions, use the [Sandbox] class directly.
 */
class SandboxManager internal constructor(
    private val sandboxService: Sandboxes,
    private val httpClientProvider: HttpClientProvider,
) : AutoCloseable {
    private val logger = LoggerFactory.getLogger(SandboxManager::class.java)

    /**
     * Provides access to shared httpclient provider
     *
     * Allows retrieving underlying http client resources initialized with connection config
     */
    fun httpClientProvider() = httpClientProvider

    companion object {
        @JvmStatic
        fun builder(): Builder = Builder()

        internal fun create(connectionConfig: ConnectionConfig): SandboxManager {
            val httpClientProvider = HttpClientProvider(connectionConfig)
            val factory = AdapterFactory(httpClientProvider)
            val sandboxService = factory.createSandboxes()
            return SandboxManager(sandboxService, httpClientProvider)
        }
    }

    fun listSandboxInfos(filter: SandboxFilter): PagedSandboxInfos {
        return sandboxService.listSandboxes(filter)
    }

    /**
     * Gets information for a single sandbox by its ID.
     *
     * @param sandboxId Sandbox ID to retrieve information for
     * @return SandboxInfo for the specified sandbox
     * @throws SandboxException if the operation fails
     */
    fun getSandboxInfo(sandboxId: UUID): SandboxInfo {
        logger.debug("Getting info for sandbox: {}", sandboxId)
        return sandboxService.getSandboxInfo(sandboxId)
    }

    /**
     * Terminates a single sandbox.
     *
     * @param sandboxId Sandbox ID to terminate
     * @throws SandboxException if the operation fails
     */
    fun killSandbox(sandboxId: UUID) {
        logger.info("Terminating sandbox: {}", sandboxId)
        sandboxService.killSandbox(sandboxId)
        logger.info("Successfully terminated sandbox: {}", sandboxId)
    }

    /**
     * Renew expiration time for a single sandbox.
     *
     * The new expiration time will be set to the current time plus the provided duration.
     *
     * @param sandboxId Sandbox ID to renew
     * @param timeout Duration to add to the current time to set the new expiration
     * @throws SandboxException if the operation fails
     */
    fun renewSandbox(
        sandboxId: UUID,
        timeout: Duration,
    ) {
        logger.info("Renew expiration for sandbox {} to {}", sandboxId, OffsetDateTime.now().plus(timeout))
        sandboxService.renewSandboxExpiration(sandboxId, OffsetDateTime.now().plus(timeout))
    }

    /**
     * Pauses a single sandbox while preserving its state.
     *
     * @param sandboxId Sandbox ID to pause
     * @throws SandboxException if the operation fails
     */
    fun pauseSandbox(sandboxId: UUID) {
        logger.info("Pausing sandbox: {}", sandboxId)
        sandboxService.pauseSandbox(sandboxId)
    }

    /**
     * Resumes a previously paused sandbox.
     *
     * @param sandboxId Sandbox ID to resume
     * @throws SandboxException if the operation fails
     */
    fun resumeSandbox(sandboxId: UUID) {
        logger.info("Resuming sandbox: {}", sandboxId)
        sandboxService.resumeSandbox(sandboxId)
    }

    /**
     * Closes this resource, relinquishing any underlying resources.
     *
     * This method closes the local HTTP client resources associated with this sandbox manager instance.
     */
    override fun close() {
        try {
            httpClientProvider.close()
        } catch (e: Exception) {
            logger.warn("Error closing resources", e)
        }
    }

    class Builder internal constructor() {
        /**
         * Connection config
         */
        private var connectionConfig: ConnectionConfig? = null

        fun connectionConfig(connectionConfig: ConnectionConfig): Builder {
            this.connectionConfig = connectionConfig
            return this
        }

        /**
         * Creates the sandbox manager with the configured parameters.
         *
         * @return Fully configured SandboxManager instance
         * @throws InvalidArgumentException if required configuration is missing or invalid
         * @throws SandboxException if manager creation fails
         */
        fun build(): SandboxManager {
            return SandboxManager.create(
                connectionConfig = connectionConfig ?: ConnectionConfig.builder().build(),
            )
        }
    }
}
