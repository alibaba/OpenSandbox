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
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxInternalException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxReadyTimeoutException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxUnhealthyException
import com.alibaba.opensandbox.sandbox.domain.models.execd.DEFAULT_EXECD_PORT
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxEndpoint
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxImageSpec
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxInfo
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxMetrics
import com.alibaba.opensandbox.sandbox.domain.services.Commands
import com.alibaba.opensandbox.sandbox.domain.services.Filesystem
import com.alibaba.opensandbox.sandbox.domain.services.Health
import com.alibaba.opensandbox.sandbox.domain.services.Metrics
import com.alibaba.opensandbox.sandbox.domain.services.Sandboxes
import com.alibaba.opensandbox.sandbox.infrastructure.factory.AdapterFactory
import okhttp3.ConnectionPool
import org.slf4j.LoggerFactory
import java.time.Duration
import java.time.OffsetDateTime
import java.util.UUID

/**
 * Main entrypoint for the Open Sandbox SDK providing secure, isolated execution environments.
 *
 * This class provides a comprehensive interface for interacting with containerized sandbox
 * environments, combining lifecycle management with high-level operations for file system
 * access, command execution, and real-time monitoring.
 *
 * ## Key Features
 *
 * - **Secure Isolation**: Complete Linux OS access in isolated containers
 * - **File System Operations**: Create, read, update, delete files and directories
 * - **Multi-language Execution**: Support for Python, Java, Bash, and other languages
 * - **Real-time Command Execution**: Streaming output with timeout handling
 * - **Resource Management**: CPU, memory, and storage constraints
 * - **Lifecycle Management**: Create, pause, resume, terminate operations
 * - **Health Monitoring**: Automatic readiness detection and status tracking
 *
 * ## Usage Example
 *
 * ```kotlin
 * // Create and configure a sandbox
 * val sandbox = Sandbox.builder()
 *     .image("python:3.11")
 *     .resource(mapOf("cpu" to "1", "memory" to "500Mi"))
 *     .timeout(Duration.ofMinutes(30))
 *     .build()
 *
 * // Use the sandbox
 * sandbox.writeFile("script.py", "print('Hello World')")
 * val result = sandbox.execute("python script.py")
 * println(result.stdout) // Output: Hello World
 *
 * // Always clean up resources
 * sandbox.terminate()
 * ```
 *
 */
class Sandbox internal constructor(
    val id: UUID,
    private val sandboxService: Sandboxes,
    private val fileSystemService: Filesystem,
    private val commandService: Commands,
    private val healthService: Health,
    private val metricsService: Metrics,
    private val customHealthCheck: ((sandbox: Sandbox) -> Boolean)? = null,
    private val httpClientProvider: HttpClientProvider,
) : AutoCloseable {
    private val logger = LoggerFactory.getLogger(Sandbox::class.java)

    /**
     * Provides access to file system operations within the sandbox.
     *
     * Allows writing, reading, listing, and deleting files and directories.
     *
     * @return Service for filesystem manipulation
     */
    fun files() = fileSystemService

    /**
     * Provides access to command execution operations.
     *
     * Allows running shell commands, capturing output, and managing processes.
     *
     * @return Service for command execution
     */
    fun commands() = commandService

    /**
     * Provides access to sandbox metrics and monitoring.
     *
     * Allows retrieving resource usage statistics (CPU, memory) and other performance metrics.
     *
     * @return Service for metrics retrieval
     */
    fun metrics() = metricsService

    /**
     * Provides access to shared httpclient provider
     *
     * Allows retrieving underlying http client resources initialized with connection config
     */
    fun httpClientProvider() = httpClientProvider

    companion object {
        private val logger = LoggerFactory.getLogger(Sandbox::class.java)

        /**
         * Creates a new [Builder] for fluent sandbox configuration.
         *
         * @return A new Builder instance
         */
        @JvmStatic
        fun builder(): Builder = Builder()

        /**
         * Creates a new [Connector] for fluent sandbox configuration.
         *
         * @return A new Connector instance
         */
        @JvmStatic
        fun connector(): Connector = Connector()

        /**
         * Creates a sandbox instance with the provided configuration.
         *
         * @param imageSpec Container image specification
         * @param env Environment variables (optional)
         * @param metadata Metadata for the sandbox (optional)
         * @param timeout Sandbox timeout in seconds
         * @param resource Resource limits (optional)
         * @param connectionConfig Connection configuration
         * @param healthCheck Custom health check function (optional)
         * @return Fully configured and ready Sandbox instance
         * @throws SandboxException if sandbox creation or initialization fails
         */
        private fun create(
            imageSpec: SandboxImageSpec,
            entrypoint: List<String>,
            env: Map<String, String>,
            metadata: Map<String, String>,
            timeout: Duration,
            readyTimeout: Duration,
            resource: Map<String, String>,
            connectionConfig: ConnectionConfig,
            healthCheck: ((Sandbox) -> Boolean)? = null,
            healthCheckPollingInterval: Duration,
            connectionPool: ConnectionPool? = null,
        ): Sandbox {
            logger.info("Start creating sandbox with image: {} (timeout: {}s)", imageSpec.image, timeout.seconds)

            val httpClientProvider = HttpClientProvider(connectionConfig)
            val factory = AdapterFactory(httpClientProvider)
            var sandboxId: UUID? = null
            var sandboxService: Sandboxes? = null

            try {
                sandboxService = factory.createSandboxes()
                val response =
                    sandboxService.createSandbox(
                        imageSpec,
                        entrypoint,
                        env,
                        metadata,
                        timeout,
                        resource,
                    )
                sandboxId = response.id

                val execdEndpoint = sandboxService.getSandboxEndpoint(response.id, DEFAULT_EXECD_PORT)
                val fileSystemService = factory.createFilesystem(execdEndpoint)
                val commandService = factory.createCommands(execdEndpoint)
                val metricsService = factory.createMetrics(execdEndpoint)
                val healthService = factory.createHealth(execdEndpoint)

                val sandbox =
                    Sandbox(
                        id = response.id,
                        sandboxService = sandboxService,
                        fileSystemService = fileSystemService,
                        commandService = commandService,
                        metricsService = metricsService,
                        healthService = healthService,
                        customHealthCheck = healthCheck,
                        httpClientProvider = httpClientProvider,
                    )

                sandbox.checkReady(readyTimeout, healthCheckPollingInterval)

                logger.info("Sandbox {} is ready and available for use", sandbox.id)

                return sandbox
            } catch (e: Exception) {
                // Attempt cleanup of remote resource if creation was successful but initialization failed
                if (sandboxId != null && sandboxService != null) {
                    try {
                        logger.warn("Sandbox creation failed during initialization. Attempting to terminate zombie sandbox: {}", sandboxId)
                        sandboxService.killSandbox(sandboxId)
                    } catch (cleanupEx: Exception) {
                        logger.error("Failed to clean up sandbox {} after creation failure", sandboxId, cleanupEx)
                        e.addSuppressed(cleanupEx)
                    }
                }

                httpClientProvider.close()
                when (e) {
                    is SandboxException -> throw e
                    else -> {
                        logger.error("Unexpected exception during sandbox creation", e)
                        throw SandboxInternalException(
                            message = "Internal exception when creating sandbox: ${e.message}",
                            cause = e,
                        )
                    }
                }
            }
        }

        /**
         * Connects to an existing sandbox instance by ID.
         *
         * This method allows you to reconnect to a previously created sandbox that
         * is still running, enabling you to resume work or share sandbox access.
         *
         * @param sandboxId Unique identifier of the existing sandbox
         * @return Connected Sandbox instance
         * @throws SandboxException if connection fails
         */
        private fun connect(
            sandboxId: UUID,
            connectionConfig: ConnectionConfig,
            healthCheck: ((Sandbox) -> Boolean)? = null,
        ): Sandbox {
            logger.info("Connecting to existing sandbox: {}", sandboxId)
            val httpClientProvider = HttpClientProvider(connectionConfig)
            val factory = AdapterFactory(httpClientProvider)

            try {
                val sandboxService = factory.createSandboxes()

                val execdEndpoint = sandboxService.getSandboxEndpoint(sandboxId, DEFAULT_EXECD_PORT)
                val fileSystemService = factory.createFilesystem(execdEndpoint)
                val commandService = factory.createCommands(execdEndpoint)
                val metricsService = factory.createMetrics(execdEndpoint)
                val healthService = factory.createHealth(execdEndpoint)

                val sandbox =
                    Sandbox(
                        id = sandboxId,
                        sandboxService = sandboxService,
                        fileSystemService = fileSystemService,
                        commandService = commandService,
                        metricsService = metricsService,
                        healthService = healthService,
                        customHealthCheck = healthCheck,
                        httpClientProvider = httpClientProvider,
                    )

                if (!sandbox.isHealthy()) {
                    throw SandboxUnhealthyException(
                        message = "Failed to connect unhealthy sandbox $sandboxId",
                    )
                }

                logger.info("Sandbox {} connected", sandbox.id)

                return sandbox
            } catch (e: Exception) {
                httpClientProvider.close()
                when (e) {
                    is SandboxException -> throw e
                    else -> {
                        logger.error("Unexpected exception during sandbox connection", e)
                        throw SandboxInternalException(
                            message = "Failed to connect to sandbox: ${e.message}",
                            cause = e,
                        )
                    }
                }
            }
        }
    }

    /**
     * Gets the current status of this sandbox.
     *
     * @return Current sandbox status including state and metadata
     * @throws SandboxException if status cannot be retrieved
     */
    fun getInfo(): SandboxInfo {
        return sandboxService.getSandboxInfo(id)
    }

    /**
     * Gets the current status of this sandbox.
     *
     * @return Current sandbox status including state and metadata
     * @throws SandboxException if status cannot be retrieved
     */
    fun getEndpoint(port: Int): SandboxEndpoint {
        return sandboxService.getSandboxEndpoint(id, port)
    }

    /**
     * Gets the current status of this sandbox.
     *
     * @return Current sandbox status including state and metadata
     */
    fun getMetrics(): SandboxMetrics {
        return metricsService.getMetrics(id)
    }

    /**
     * Renew the sandbox expiration time to delay automatic termination.
     *
     * The new expiration time will be set to the current time plus the provided duration.
     *
     * @param timeout Duration to add to the current time to set the new expiration
     * @throws SandboxException if the operation fails
     */
    fun renew(timeout: Duration) {
        logger.info("Renew sandbox {} timeout, estimated expiration to {}", id, OffsetDateTime.now().plus(timeout))
        sandboxService.renewSandboxExpiration(id, OffsetDateTime.now().plus(timeout))
    }

    /**
     * Pauses the sandbox while preserving its state.
     *
     * The sandbox will transition to PAUSED state and can be resumed later.
     * All running processes will be suspended.
     *
     * @throws SandboxException if pause operation fails
     */
    fun pause() {
        logger.info("Pausing sandbox: {}", id)
        sandboxService.pauseSandbox(id)
    }

    /**
     * Resumes a previously paused sandbox.
     *
     * The sandbox will transition from PAUSED to RUNNING state and all
     * suspended processes will be resumed.
     *
     * @throws SandboxException if resume operation fails
     */
    fun resume() {
        logger.info("Resuming sandbox: {}", id)
        sandboxService.resumeSandbox(id)
    }

    /**
     * This method sends a termination signal to the remote sandbox instance, causing it to stop immediately.
     * This is an irreversible operation.
     *
     * Note: This method does NOT close the local `Sandbox` object resources (like connection pools).
     * You should call `close()` or use a try-with-resources block to clean up local resources.
     *
     * @throws SandboxException if termination fails
     */
    fun kill() {
        sandboxService.killSandbox(id)
    }

    /**
     * Closes this resource, relinquishing any underlying resources.
     *
     * This method closes the local HTTP client resources associated with this sandbox instance.
     * It does **NOT** terminate the remote sandbox instance. If you wish to terminate the remote
     * sandbox, call [kill] before closing.
     *
     * If this sandbox was created with a user-managed (shared) connection pool, the pool will NOT be closed.
     * If it was created with a default (dedicated) pool, the pool will be evicted and destroyed.
     */
    override fun close() {
        try {
            httpClientProvider.close()
        } catch (e: Exception) {
            logger.warn("Error closing resources", e)
        }
    }

    /**
     * Waits for the sandbox to pass a custom health check with polling.
     *
     * @param timeout Maximum time to wait for health check to pass
     * @param pollingInterval Time between health check attempts
     * @throws SandboxReadyTimeoutException if health check doesn't pass within timeout
     * @throws SandboxException if health check fails
     */
    fun checkReady(
        timeout: Duration,
        pollingInterval: Duration,
    ) {
        logger.info("Waiting for sandbox {} to pass health check (timeout: {}s)", id, timeout.seconds)

        val deadline = System.currentTimeMillis() + timeout.toMillis()
        var attempt = 0
        var lastException: Throwable? = null

        while (System.currentTimeMillis() < deadline) {
            attempt++
            logger.debug("Health check attempt #{} for sandbox {}", attempt, id)

            val isHealthy =
                try {
                    isHealthy()
                } catch (e: Exception) {
                    lastException = e
                    logger.debug("Health check attempt #{} failed with exception: {}", attempt, e.message)
                    false
                }

            if (isHealthy) {
                logger.info("Sandbox {} passed health check after {} attempts", id, attempt)
                return
            }

            if (lastException == null) {
                logger.debug("Health check attempt #{} returned false", attempt)
            }

            Thread.sleep(pollingInterval.toMillis())
        }

        val errorDetail =
            if (lastException != null) {
                "Last error: ${lastException.message}"
            } else {
                "Check returned false continuously"
            }

        val finalMessage = "Sandbox health check timed out after ${timeout.seconds}s ($attempt attempts). $errorDetail"

        logger.error(finalMessage, lastException)

        throw SandboxReadyTimeoutException(
            message = finalMessage,
        )
    }

    /**
     * Checks if the sandbox is healthy and responsive.
     *
     * @return true if sandbox is healthy, false otherwise
     */
    fun isHealthy(): Boolean {
        return customHealthCheck?.invoke(this) ?: ping()
    }

    /**
     * Checks if the sandbox is alive.
     *
     * @return `true` if the sandbox is healthy.
     */
    private fun ping(): Boolean {
        return healthService.ping(id)
    }

    /**
     * Fluent connector for connecting to existing sandbox instances.
     *
     * This class provides a type-safe, fluent interface for configuring connection
     * parameters to connect to a running sandbox instance.
     *
     * ## Basic Usage
     *
     * ```kotlin
     * val sandbox = Sandbox.connector()
     *     .sandboxId("existing-sandbox-id")
     *     .build()
     * ```
     *
     * ## Advanced Configuration
     *
     * ```kotlin
     * val sandbox = Sandbox.connector()
     *     .sandboxId("existing-sandbox-id")
     *     .apiKey("your-api-key")
     *     .domain("api.custom-domain.com/v1")
     *     .requestTimeout(Duration.ofSeconds(60))
     *     .healthCheck { sandbox -> sandbox.isHealthy() }
     *     .build()
     * ```
     */
    class Connector internal constructor() {
        /**
         * Sandbox ID to connect to
         */
        private var sandboxId: UUID? = null

        /**
         * Connection config
         */
        private var connectionConfig: ConnectionConfig? = null

        /**
         * Health check logic
         */
        private var healthCheck: ((Sandbox) -> Boolean)? = null

        /**
         * Sets the sandbox ID to connect to.
         *
         * @param sandboxId UUID string of the existing sandbox
         * @return This connector for method chaining
         * @throws InvalidArgumentException if sandboxId is blank
         */
        fun sandboxId(sandboxId: UUID): Connector {
            this.sandboxId = sandboxId
            return this
        }

        fun healthCheck(healthCheck: (Sandbox) -> Boolean): Connector {
            this.healthCheck = healthCheck
            return this
        }

        fun connectionConfig(connectionConfig: ConnectionConfig): Connector {
            this.connectionConfig = connectionConfig
            return this
        }

        /**
         * Connects to the existing sandbox with the configured parameters.
         *
         * This method performs the following steps:
         * 1. Validates all required configuration
         * 2. Delegates to Sandbox.connect() to connect to the sandbox
         * 3. Returns a connected Sandbox instance
         *
         * @return Connected Sandbox instance
         * @throws InvalidArgumentException if required configuration is missing or invalid
         * @throws SandboxException if sandbox connection fails
         */
        fun connect(): Sandbox {
            // Validate required configuration
            val id =
                sandboxId ?: throw InvalidArgumentException(
                    message = "Sandbox ID must be specified",
                )
            return connect(
                sandboxId = id,
                connectionConfig = connectionConfig ?: ConnectionConfig.builder().build(),
                healthCheck = healthCheck,
            )
        }
    }

    /**
     * Fluent builder for creating and configuring sandbox instances.
     *
     * This class provides a type-safe, fluent interface for configuring all aspects
     * of sandbox creation, from sandbox images and resource limits to environment
     * variables and lifecycle settings.
     *
     * ## Basic Usage
     *
     * ```kotlin
     * val sandbox = Sandbox.builder()
     *     .image("python:3.11")
     *     .build()
     * ```
     *
     * ## Advanced Configuration
     *
     * ```kotlin
     * val sandbox = Sandbox.builder()
     *     .image("myregistry.com/app:latest")
     *     .imageAuth("username", "password")
     *     .entrypoint("python", "-u", "app.py")
     *     .resource {
     *         put("cpu", "1000m")
     *         put("memory", "2Gi")
     *     }
     *     .env {
     *         put("LOG_LEVEL", "info")
     *     }
     *     .metadata {
     *         put("project", "my-project")
     *         put("team", "backend")
     *     }
     *     .timeout(Duration.ofMinutes(30))
     *     .readyTimeout(Duration.ofSeconds(120))
     *     .build()
     * ```
     */
    class Builder internal constructor() {
        /**
         * Image config
         */
        private var imageSpec: SandboxImageSpec? = null

        /**
         * Sandbox entrypoint
         */
        private var entrypoint: List<String> = listOf("tail", "-f", "/dev/null")

        /**
         * Resource limits config
         */
        private val resource = mutableMapOf("cpu" to "1", "memory" to "2Gi")

        /**
         * Env
         */
        private val env = mutableMapOf<String, String>()

        /**
         * Metadata
         */
        private val metadata = mutableMapOf<String, String>()

        /**
         * Lifecycle config
         */
        private var timeout: Duration = Duration.ofSeconds(600)
        private var readyTimeout: Duration = Duration.ofSeconds(30)
        private var healthCheckPollingInterval: Duration = Duration.ofMillis(200)
        private var healthCheck: ((Sandbox) -> Boolean)? = null

        /**
         * Connection config
         */
        private var connectionConfig: ConnectionConfig? = null

        /**
         * Sets the sandbox image for the sandbox.
         *
         * @param image Sandbox image reference (e.g., "ubuntu:22.04", "python:3.11")
         * @return This builder for method chaining
         * @throws InvalidArgumentException if image is blank
         */
        fun image(image: String): Builder {
            if (image.isBlank()) {
                throw InvalidArgumentException(
                    message = "Image cannot be blank",
                )
            }
            this.imageSpec =
                SandboxImageSpec.builder()
                    .image(image)
                    .build()
            return this
        }

        /**
         * Sets the sandbox image specification.
         *
         * @param imageSpec Complete image specification including image and optional auth
         * @return This builder for method chaining
         */
        fun imageSpec(imageSpec: SandboxImageSpec): Builder {
            this.imageSpec = imageSpec
            return this
        }

        /**
         * Sets the entrypoint command for the sandbox.
         *
         * @param entrypoint List of command and arguments to use as entrypoint
         * @return This builder for method chaining
         */
        fun entrypoint(entrypoint: List<String>): Builder {
            this.entrypoint = entrypoint
            return this
        }

        /**
         * Sets the entrypoint command for the sandbox.
         *
         * @param entrypoint Vararg command and arguments to use as entrypoint
         * @return This builder for method chaining
         */
        fun entrypoint(vararg entrypoint: String): Builder {
            this.entrypoint = entrypoint.toList()
            return this
        }

        /**
         * Sets resource limits for the sandbox using a fluent configuration block.
         *
         * @param configure Configuration block for resource limits
         * @return This builder for method chaining
         */
        fun resource(configure: MutableMap<String, String>.() -> Unit): Builder {
            resource.configure()
            return this
        }

        /**
         * Sets resource limits for the sandbox.
         *
         * @param resource Resource limits map
         * @return This builder for method chaining
         */
        fun resource(resource: Map<String, String>): Builder {
            this.resource.clear()
            this.resource.putAll(resource)
            return this
        }

        /**
         * Adds a single environment variable.
         *
         * @param key Environment variable name
         * @param value Environment variable value
         * @return This builder for method chaining
         */
        fun env(
            key: String,
            value: String,
        ): Builder {
            if (key.isBlank()) {
                throw InvalidArgumentException(
                    message = "Environment variable key cannot be blank",
                )
            }
            env[key] = value
            return this
        }

        /**
         * Adds multiple environment variables.
         *
         * @param env Map of environment variables to add
         * @return This builder for method chaining
         */
        fun env(env: Map<String, String>): Builder {
            this.env.putAll(env)
            return this
        }

        /**
         * Configures environment variables using a fluent configuration block.
         *
         * @param configure Configuration block that receives a mutable map
         * @return This builder for method chaining
         */
        fun env(configure: MutableMap<String, String>.() -> Unit): Builder {
            env.configure()
            return this
        }

        /**
         * Adds a single metadata entry.
         *
         * @param key Metadata key
         * @param value Metadata value
         * @return This builder for method chaining
         */
        fun metadata(
            key: String,
            value: String,
        ): Builder {
            if (key.isBlank()) {
                throw InvalidArgumentException(
                    message = "Metadata key cannot be blank",
                )
            }
            metadata[key] = value
            return this
        }

        /**
         * Adds multiple metadata entries.
         *
         * @param metadata Map of metadata to add
         * @return This builder for method chaining
         */
        fun metadata(metadata: Map<String, String>): Builder {
            this.metadata.putAll(metadata)
            return this
        }

        /**
         * Configures metadata using a fluent configuration block.
         *
         * @param configure Configuration block that receives a mutable map
         * @return This builder for method chaining
         */
        fun metadata(configure: MutableMap<String, String>.() -> Unit): Builder {
            metadata.configure()
            return this
        }

        /**
         * Sets the sandbox timeout (automatic termination time).
         *
         * @param timeout Maximum sandbox lifetime
         * @return This builder for method chaining
         * @throws InvalidArgumentException if timeout is negative or zero
         */
        fun timeout(timeout: Duration): Builder {
            if (timeout.isNegative || timeout.isZero) {
                throw InvalidArgumentException(
                    message = "Timeout must be positive, got: $timeout",
                )
            }
            this.timeout = timeout
            return this
        }

        /**
         * Sets the timeout for waiting for sandbox readiness.
         *
         * @param readyTimeout Maximum time to wait for sandbox to become ready
         * @return This builder for method chaining
         * @throws InvalidArgumentException if timeout is negative or zero
         */
        fun readyTimeout(readyTimeout: Duration): Builder {
            if (readyTimeout.isNegative || readyTimeout.isZero) {
                throw InvalidArgumentException(
                    message = "Ready timeout must be positive, got: $readyTimeout",
                )
            }
            this.readyTimeout = readyTimeout
            return this
        }

        /**
         * Sets the interval between readiness polling attempts.
         *
         * @param pollingInterval Time between readiness checks
         * @return This builder for method chaining
         * @throws InvalidArgumentException if interval is negative or zero
         */
        fun healthCheckPollingInterval(pollingInterval: Duration): Builder {
            if (pollingInterval.isNegative || pollingInterval.isZero) {
                throw InvalidArgumentException(
                    message = "Ready polling interval must be positive, got: $pollingInterval",
                )
            }
            this.healthCheckPollingInterval = pollingInterval
            return this
        }

        fun healthCheck(healthCheck: (Sandbox) -> Boolean): Builder {
            this.healthCheck = healthCheck
            return this
        }

        fun connectionConfig(connectionConfig: ConnectionConfig): Builder {
            this.connectionConfig = connectionConfig
            return this
        }

        /**
         * Creates and starts the sandbox with the configured parameters.
         *
         * This method performs the following steps:
         * 1. Validates all required configuration
         * 2. Delegates to Sandbox.create() to create the sandbox
         * 3. Returns a fully initialized Sandbox instance
         *
         * @return Fully configured and ready Sandbox instance
         * @throws InvalidArgumentException if required configuration is missing or invalid
         * @throws SandboxException if sandbox creation or initialization fails
         */
        fun build(): Sandbox {
            // Validate required configuration
            val spec =
                imageSpec ?: throw InvalidArgumentException(
                    message = "Sandbox image must be specified",
                )

            // Validate image specification
            if (spec.image.isBlank()) {
                throw InvalidArgumentException("Sandbox image cannot be blank")
            }

            return create(
                imageSpec = spec,
                entrypoint = entrypoint,
                env = env,
                metadata = metadata,
                timeout = timeout,
                readyTimeout = readyTimeout,
                resource = resource,
                connectionConfig = connectionConfig ?: ConnectionConfig.builder().build(),
                healthCheckPollingInterval = healthCheckPollingInterval,
                healthCheck = healthCheck,
            )
        }
    }
}
