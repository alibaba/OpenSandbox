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

package com.alibaba.opensandbox.sandbox.domain.services

import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PagedSandboxInfos
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxCreateResponse
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxEndpoint
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxFilter
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxImageSpec
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxInfo
import java.time.Duration
import java.time.OffsetDateTime
import java.util.UUID

/**
 * Core sandbox lifecycle management service.
 *
 * This service provides a clean abstraction over sandbox creation, management,
 * and termination operations, completely isolating business logic from API implementation details.
 */
interface Sandboxes {
    /**
     * Creates a new sandbox with the specified configuration.
     *
     * @param spec Image specification for the sandbox
     * @return Sandbox create response
     */
    fun createSandbox(
        spec: SandboxImageSpec,
        entrypoint: List<String>,
        env: Map<String, String>,
        metadata: Map<String, String>,
        timeout: Duration,
        resource: Map<String, String>,
    ): SandboxCreateResponse

    /**
     * Retrieves information about an existing sandbox.
     *
     * @param sandboxId Unique identifier of the sandbox
     * @return Current sandbox information
     */
    fun getSandboxInfo(sandboxId: UUID): SandboxInfo

    /**
     * Lists sandboxes with optional filtering.
     *
     * @param filter Optional filter criteria
     * @return List of sandbox information matching the filter
     */
    fun listSandboxes(filter: SandboxFilter): PagedSandboxInfos

    /**
     * Get sandbox endpoint
     *
     * @param sandboxId sandbox id
     * @param port endpoint port number
     * @return Target sandbox endpoint
     */
    fun getSandboxEndpoint(
        sandboxId: UUID,
        port: Int,
    ): SandboxEndpoint

    /**
     * Pauses a running sandbox, preserving its state.
     *
     * @param sandboxId Unique identifier of the sandbox
     */
    fun pauseSandbox(sandboxId: UUID)

    /**
     * Resumes a paused sandbox.
     *
     * @param sandboxId Unique identifier of the sandbox
     */
    fun resumeSandbox(sandboxId: UUID)

    /**
     * Renew the expiration time of a sandbox.
     *
     * @param sandboxId Unique identifier of the sandbox
     * @param newExpirationTime New expiration timestamp
     */
    fun renewSandboxExpiration(
        sandboxId: UUID,
        newExpirationTime: OffsetDateTime,
    )

    /**
     * Terminates a sandbox and releases all associated resources.
     *
     * @param sandboxId Unique identifier of the sandbox
     */
    fun killSandbox(sandboxId: UUID)
}
