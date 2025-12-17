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

package com.alibaba.opensandbox.sandbox.domain.models.sandboxes

import java.time.OffsetDateTime
import java.util.UUID

/**
 * High-level lifecycle state of the sandbox.
 *
 * Common state values:
 * - Pending: Sandbox is being provisioned
 * - Running: Sandbox is running and ready to accept requests
 * - Pausing: Sandbox is in the process of pausing
 * - Paused: Sandbox has been paused while retaining its state
 * - Stopping: Sandbox is being terminated
 * - Terminated: Sandbox has been successfully terminated
 * - Failed: Sandbox encountered a critical error
 *
 * State transitions:
 * - Pending → Running (after creation completes)
 * - Running → Pausing (when pause is requested)
 * - Pausing → Paused (pause operation completes)
 * - Paused → Running (when resume is requested)
 * - Running/Paused → Stopping (when kill is requested or TTL expires)
 * - Stopping → Terminated (kill/timeout operation completes)
 * - Pending/Running/Paused → Failed (on error)
 *
 * Note: New state values may be added in future versions.
 * Clients should handle unknown state values gracefully.
 */
object SandboxState {
    const val PENDING = "Pending"
    const val RUNNING = "Running"
    const val PAUSING = "Pausing"
    const val PAUSED = "Paused"
    const val STOPPING = "Stopping"
    const val TERMINATED = "Terminated"
    const val FAILED = "Failed"
    const val UNKNOWN = "Unknown"
}

/**
 * Filter criteria for listing sandboxes.
 *
 * @property states Filter by sandbox states (e.g., RUNNING, PAUSED)
 * @property metadata Filter by metadata key-value pairs
 * @property pageSize Number of items per page
 * @property page Page number (0-indexed)
 */
class SandboxFilter private constructor(
    val states: List<String>?,
    val metadata: Map<String, String>?,
    val pageSize: Int?,
    val page: Int?,
) {
    companion object {
        @JvmStatic
        fun builder(): Builder = Builder()
    }

    class Builder {
        private var states: List<String>? = null
        private var metadata: Map<String, String>? = null
        private var pageSize: Int? = null
        private var page: Int? = null

        fun states(states: List<String>): Builder {
            this.states = states
            return this
        }

        fun states(vararg states: String): Builder {
            this.states = states.toList()
            return this
        }

        fun metadata(metadata: Map<String, String>): Builder {
            this.metadata = metadata
            return this
        }

        fun metadata(configure: MutableMap<String, String>.() -> Unit): Builder {
            val map = mutableMapOf<String, String>()
            map.configure()
            this.metadata = map
            return this
        }

        fun pageSize(pageSize: Int): Builder {
            require(pageSize > 0) { "Page size must be positive" }
            this.pageSize = pageSize
            return this
        }

        fun page(page: Int): Builder {
            require(page >= 0) { "Page must be non-negative" }
            this.page = page
            return this
        }

        fun build(): SandboxFilter {
            return SandboxFilter(
                states = states,
                metadata = metadata,
                pageSize = pageSize,
                page = page,
            )
        }
    }
}

/**
 * Specification for a sandbox container image.
 *
 * @property image The image reference (e.g., "ubuntu:22.04", "python:3.11")
 * @property auth Authentication credentials for private registries
 */
class SandboxImageSpec private constructor(
    val image: String,
    val auth: SandboxImageAuth?,
) {
    companion object {
        @JvmStatic
        fun builder(): Builder = Builder()
    }

    class Builder {
        private var image: String? = null
        private var auth: SandboxImageAuth? = null

        fun image(image: String): Builder {
            require(image.isNotBlank()) { "Image cannot be blank" }
            this.image = image
            return this
        }

        fun auth(auth: SandboxImageAuth): Builder {
            this.auth = auth
            return this
        }

        fun auth(
            username: String,
            password: String,
        ): Builder {
            this.auth =
                SandboxImageAuth.builder()
                    .username(username)
                    .password(password)
                    .build()
            return this
        }

        fun build(): SandboxImageSpec {
            val imageValue = image ?: throw IllegalArgumentException("Image must be specified")
            return SandboxImageSpec(
                image = imageValue,
                auth = auth,
            )
        }
    }
}

/**
 * Authentication credentials for container registries.
 *
 * @property username Registry username
 * @property password Registry password or access token
 */
class SandboxImageAuth private constructor(
    val username: String,
    val password: String,
) {
    companion object {
        @JvmStatic
        fun builder(): Builder = Builder()
    }

    class Builder {
        private var username: String? = null
        private var password: String? = null

        fun username(username: String): Builder {
            require(username.isNotBlank()) { "Username cannot be blank" }
            this.username = username
            return this
        }

        fun password(password: String): Builder {
            require(password.isNotBlank()) { "Password cannot be blank" }
            this.password = password
            return this
        }

        fun build(): SandboxImageAuth {
            val usernameValue = username ?: throw IllegalArgumentException("Username must be specified")
            val passwordValue = password ?: throw IllegalArgumentException("Password must be specified")
            return SandboxImageAuth(
                username = usernameValue,
                password = passwordValue,
            )
        }
    }
}

/**
 * Detailed information about a sandbox instance.
 *
 * @property id Unique identifier of the sandbox
 * @property status Current status of the sandbox
 * @property entrypoint Command line arguments used to start the sandbox
 * @property expiresAt Timestamp when the sandbox is scheduled for automatic termination
 * @property createdAt Timestamp when the sandbox was created
 * @property image Image specification used to create this sandbox
 * @property metadata Custom metadata attached to the sandbox
 */
class SandboxInfo(
    val id: UUID,
    val status: SandboxStatus,
    val entrypoint: List<String>,
    val expiresAt: OffsetDateTime,
    val createdAt: OffsetDateTime,
    val image: SandboxImageSpec,
    val metadata: Map<String, String>? = null,
)

/**
 * Status information for a sandbox.
 *
 * @property state Current state (e.g., RUNNING, PENDING, PAUSED, TERMINATED)
 * @property reason Short reason code for the current state
 * @property message Human-readable message explaining the status
 * @property lastTransitionAt Timestamp of the last state transition
 */
class SandboxStatus(
    val state: String,
    val reason: String?,
    val message: String?,
    val lastTransitionAt: java.time.OffsetDateTime?,
)

/**
 * Response returned when a sandbox is created.
 *
 * @property id Unique identifier of the newly created sandbox
 */
class SandboxCreateResponse(
    val id: UUID,
)

/**
 * Connection endpoint information for a sandbox.
 *
 * @property endpoint Sandbox endpoint
 */
class SandboxEndpoint(
    val endpoint: String,
)

/**
 * A paginated list of sandbox information.
 *
 * @property sandboxInfos List of sandbox details for the current page
 * @property pagination Pagination metadata
 */
class PagedSandboxInfos(
    val sandboxInfos: List<SandboxInfo>,
    val pagination: PaginationInfo,
)

/**
 * Pagination metadata.
 *
 * @property page Current page number (0-indexed)
 * @property pageSize Number of items per page
 * @property totalItems Total number of items across all pages
 * @property totalPages Total number of pages
 * @property hasNextPage True if there is a next page available
 */
class PaginationInfo(
    val page: Int,
    val pageSize: Int,
    val totalItems: Int,
    val totalPages: Int,
    val hasNextPage: Boolean,
)

/**
 * Real-time resource usage metrics for a sandbox.
 *
 * @property cpuCount Number of CPU cores available/allocated
 * @property cpuUsedPercentage Current CPU usage as a percentage (0.0 - 100.0)
 * @property memoryTotalInMiB Total memory available in Mebibytes
 * @property memoryUsedInMiB Memory currently used in Mebibytes
 * @property timestamp Timestamp of the metric collection (Unix epoch milliseconds)
 */
class SandboxMetrics(
    val cpuCount: Float,
    val cpuUsedPercentage: Float,
    val memoryTotalInMiB: Float,
    val memoryUsedInMiB: Float,
    val timestamp: Long,
)
