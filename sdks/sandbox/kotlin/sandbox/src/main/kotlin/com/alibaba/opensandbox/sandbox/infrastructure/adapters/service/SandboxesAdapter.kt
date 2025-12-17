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

package com.alibaba.opensandbox.sandbox.infrastructure.adapters.service

import com.alibaba.opensandbox.sandbox.HttpClientProvider
import com.alibaba.opensandbox.sandbox.api.SandboxesApi
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PagedSandboxInfos
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxCreateResponse
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxEndpoint
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxFilter
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxImageSpec
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxInfo
import com.alibaba.opensandbox.sandbox.domain.services.Sandboxes
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toApiRenewRequest
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toPagedSandboxInfos
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toSandboxCreateResponse
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toSandboxEndpoint
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toSandboxInfo
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.toSandboxException
import org.slf4j.LoggerFactory
import java.time.Duration
import java.time.OffsetDateTime
import java.util.UUID

/**
 * Implementation of [Sandboxes] that adapts OpenAPI-generated [SandboxesApi].
 *
 * This adapter provides a clean abstraction layer between business logic and
 * the auto-generated API client, handling all model conversions and error mapping.
 */
internal class SandboxesAdapter(
    private val provider: HttpClientProvider,
) : Sandboxes {
    private val logger = LoggerFactory.getLogger(SandboxesAdapter::class.java)

    private val api = SandboxesApi(provider.config.getBaseUrl(), provider.authenticatedClient)

    override fun createSandbox(
        spec: SandboxImageSpec,
        entrypoint: List<String>,
        env: Map<String, String>,
        metadata: Map<String, String>,
        timeout: Duration,
        resource: Map<String, String>,
    ): SandboxCreateResponse {
        logger.info("Creating sandbox with image: {}", spec.image)

        return try {
            val createRequest = SandboxModelConverter.toApiCreateSandboxRequest(spec, entrypoint, env, metadata, timeout, resource)
            val apiResponse = api.sandboxesPost(createRequest)
            val response = apiResponse.toSandboxCreateResponse()

            logger.info("Successfully created sandbox: {}", response.id)

            response
        } catch (e: Exception) {
            throw e.toSandboxException()
        }
    }

    override fun getSandboxInfo(sandboxId: UUID): SandboxInfo {
        logger.debug("Retrieving sandbox information: {}", sandboxId)

        return try {
            api.sandboxesSandboxIdGet(sandboxId).toSandboxInfo()
        } catch (e: Exception) {
            throw e.toSandboxException()
        }
    }

    override fun listSandboxes(filter: SandboxFilter): PagedSandboxInfos {
        logger.debug("Listing sandboxes with filter: {}", filter)
        val metadataQuery: String? =
            filter.metadata?.entries?.joinToString("&") { "${it.key}=${it.value}" }
        return try {
            api.sandboxesGet(filter.states, metadataQuery, filter.page, filter.pageSize).toPagedSandboxInfos()
        } catch (e: Exception) {
            throw e.toSandboxException()
        }
    }

    override fun getSandboxEndpoint(
        sandboxId: UUID,
        port: Int,
    ): SandboxEndpoint {
        logger.debug("Retrieving sandbox endpoint: {}, port {}", sandboxId, port)
        return try {
            api.sandboxesSandboxIdEndpointsPortGet(sandboxId, port).toSandboxEndpoint()
        } catch (e: Exception) {
            logger.error("Failed to retrieve sandbox endpoint for sandbox {}", sandboxId, e)
            throw e.toSandboxException()
        }
    }

    override fun pauseSandbox(sandboxId: UUID) {
        logger.info("Pausing sandbox: {}", sandboxId)

        try {
            api.sandboxesSandboxIdPausePost(sandboxId)
            logger.info("Initiated pause for sandbox: {}", sandboxId)
        } catch (e: Exception) {
            logger.error("Failed to initiate pause sandbox: {}", sandboxId, e)
            throw e.toSandboxException()
        }
    }

    override fun resumeSandbox(sandboxId: UUID) {
        logger.info("Resuming sandbox: {}", sandboxId)

        try {
            api.sandboxesSandboxIdResumePost(sandboxId)
            logger.info("Initiated resume for sandbox: {}", sandboxId)
        } catch (e: Exception) {
            logger.error("Failed initiate resume sandbox: {}", sandboxId, e)
            throw e.toSandboxException()
        }
    }

    override fun renewSandboxExpiration(
        sandboxId: UUID,
        newExpirationTime: OffsetDateTime,
    ) {
        logger.info("Renew sandbox {} expiration to {}", sandboxId, newExpirationTime)

        try {
            api.sandboxesSandboxIdRenewExpirationPost(
                sandboxId,
                newExpirationTime.toApiRenewRequest(),
            )
            logger.info("Successfully renewed sandbox {} expiration", sandboxId)
        } catch (e: Exception) {
            logger.error("Failed to renew sandbox {} expiration", sandboxId, e)
            throw e.toSandboxException()
        }
    }

    override fun killSandbox(sandboxId: UUID) {
        logger.info("Terminating sandbox: {}", sandboxId)

        return try {
            api.sandboxesSandboxIdDelete(sandboxId)
            logger.info("Successfully terminated sandbox: {}", sandboxId)
        } catch (e: Exception) {
            logger.error("Failed to terminate sandbox: {}", sandboxId, e)
            throw e.toSandboxException()
        }
    }
}
