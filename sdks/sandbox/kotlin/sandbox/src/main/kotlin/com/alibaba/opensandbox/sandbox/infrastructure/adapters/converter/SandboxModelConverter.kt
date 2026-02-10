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

package com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter

// API Models
import com.alibaba.opensandbox.sandbox.api.models.CreateSandboxRequest
import com.alibaba.opensandbox.sandbox.api.models.CreateSandboxResponse
import com.alibaba.opensandbox.sandbox.api.models.Endpoint
import com.alibaba.opensandbox.sandbox.api.models.ImageSpec
import com.alibaba.opensandbox.sandbox.api.models.ImageSpecAuth
import com.alibaba.opensandbox.sandbox.api.models.ListSandboxesResponse
import com.alibaba.opensandbox.sandbox.api.models.RenewSandboxExpirationRequest
import com.alibaba.opensandbox.sandbox.api.models.RenewSandboxExpirationResponse
import com.alibaba.opensandbox.sandbox.api.models.execd.Metrics
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.Host
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.NetworkPolicy
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.NetworkRule
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PVC
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PagedSandboxInfos
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PaginationInfo
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxCreateResponse
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxEndpoint
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxImageAuth
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxImageSpec
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxInfo
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxMetrics
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxRenewResponse
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.Volume
import java.time.Duration
import java.time.OffsetDateTime
import com.alibaba.opensandbox.sandbox.api.models.Host as ApiHost
import com.alibaba.opensandbox.sandbox.api.models.NetworkPolicy as ApiNetworkPolicy
import com.alibaba.opensandbox.sandbox.api.models.NetworkRule as ApiNetworkRule
import com.alibaba.opensandbox.sandbox.api.models.PVC as ApiPVC
import com.alibaba.opensandbox.sandbox.api.models.PaginationInfo as ApiPaginationInfo
import com.alibaba.opensandbox.sandbox.api.models.Sandbox as ApiSandbox
import com.alibaba.opensandbox.sandbox.api.models.SandboxStatus as ApiSandboxStatus
import com.alibaba.opensandbox.sandbox.api.models.Volume as ApiVolume
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxStatus as DomainSandboxStatus

internal object SandboxModelConverter {
    /**
     * Converts Domain ImageSpec -> API ImageSpec
     */
    fun SandboxImageSpec.toApiImageSpec(): ImageSpec {
        return ImageSpec(
            uri = this.image,
            auth =
                this.auth?.let {
                    ImageSpecAuth(
                        username = it.username,
                        password = it.password,
                    )
                },
        )
    }

    /**
     * Converts Time -> API renew Request
     */
    fun OffsetDateTime.toApiRenewRequest(): RenewSandboxExpirationRequest {
        return RenewSandboxExpirationRequest(
            expiresAt = this,
        )
    }

    /**
     * Converts Domain NetworkPolicy -> API NetworkPolicy
     */
    fun NetworkPolicy.toApiNetworkPolicy(): ApiNetworkPolicy {
        val apiDefaultAction =
            defaultAction?.let { action ->
                when (action) {
                    NetworkPolicy.DefaultAction.ALLOW -> ApiNetworkPolicy.DefaultAction.allow
                    NetworkPolicy.DefaultAction.DENY -> ApiNetworkPolicy.DefaultAction.deny
                }
            }
        val apiEgress =
            egress?.map { rule ->
                ApiNetworkRule(
                    action =
                        when (rule.action) {
                            NetworkRule.Action.ALLOW -> ApiNetworkRule.Action.allow
                            NetworkRule.Action.DENY -> ApiNetworkRule.Action.deny
                        },
                    target = rule.target,
                )
            }
        return ApiNetworkPolicy(
            defaultAction = apiDefaultAction,
            egress = apiEgress,
        )
    }

    /**
     * Converts Domain Host -> API Host
     */
    fun Host.toApiHost(): ApiHost {
        return ApiHost(path = this.path)
    }

    /**
     * Converts Domain PVC -> API PVC
     */
    fun PVC.toApiPVC(): ApiPVC {
        return ApiPVC(claimName = this.claimName)
    }

    /**
     * Converts Domain Volume -> API Volume
     */
    fun Volume.toApiVolume(): ApiVolume {
        return ApiVolume(
            name = this.name,
            mountPath = this.mountPath,
            readOnly = this.readOnly,
            host = this.host?.toApiHost(),
            pvc = this.pvc?.toApiPVC(),
            subPath = this.subPath,
        )
    }

    fun toApiCreateSandboxRequest(
        spec: SandboxImageSpec,
        entrypoint: List<String>,
        env: Map<String, String>,
        metadata: Map<String, String>,
        timeout: Duration,
        resource: Map<String, String>,
        networkPolicy: NetworkPolicy?,
        extensions: Map<String, String>,
        volumes: List<Volume>?,
    ): CreateSandboxRequest {
        return CreateSandboxRequest(
            image = spec.toApiImageSpec(),
            entrypoint = entrypoint,
            env = env,
            metadata = metadata,
            timeout = timeout.seconds.toInt(),
            resourceLimits = resource,
            networkPolicy = networkPolicy?.toApiNetworkPolicy(),
            extensions = extensions,
            volumes = volumes?.map { it.toApiVolume() },
        )
    }

    /**
     * API Sandbox -> Domain SandboxInfo
     */
    fun ApiSandbox.toSandboxInfo(): SandboxInfo {
        return SandboxInfo(
            id = this.id,
            entrypoint = this.entrypoint,
            expiresAt = this.expiresAt,
            createdAt = this.createdAt,
            image = this.image.toImageSpec(),
            status = this.status.toSandboxStatus(),
            metadata = metadata,
        )
    }

    /**
     * API ImageSpec -> Domain ImageSpec
     */
    fun ImageSpec.toImageSpec(): SandboxImageSpec {
        val builder =
            SandboxImageSpec.builder()
                .image(uri)

        auth?.let { authInfo ->
            val sandboxAuth =
                SandboxImageAuth.builder()
                    .username(authInfo.username.orEmpty())
                    .password(authInfo.password.orEmpty())
                    .build()
            builder.auth(sandboxAuth)
        }

        return builder.build()
    }

    /**
     * API Status -> Domain Status
     */
    fun ApiSandboxStatus.toSandboxStatus(): DomainSandboxStatus {
        return DomainSandboxStatus(
            state = this.state,
            reason = this.reason,
            message = this.message,
            lastTransitionAt = this.lastTransitionAt,
        )
    }

    /**
     * API Endpoint -> Domain Endpoint
     */
    fun Endpoint.toSandboxEndpoint(): SandboxEndpoint {
        return SandboxEndpoint(this.endpoint)
    }

    /**
     * API Create Response -> Domain Create Response
     */
    fun CreateSandboxResponse.toSandboxCreateResponse(): SandboxCreateResponse {
        return SandboxCreateResponse(
            id = this.id,
        )
    }

    fun ApiPaginationInfo.toPaginationInfo(): PaginationInfo {
        return PaginationInfo(
            page = this.page,
            pageSize = this.pageSize,
            totalItems = this.totalItems,
            totalPages = this.totalPages,
            hasNextPage = this.hasNextPage,
        )
    }

    /**
     * API List Response -> Domain Paged Infos
     */
    fun ListSandboxesResponse.toPagedSandboxInfos(): PagedSandboxInfos {
        return PagedSandboxInfos(
            items.map { it.toSandboxInfo() },
            pagination.toPaginationInfo(),
        )
    }

    fun Metrics.toSandboxMetrics(): SandboxMetrics {
        return SandboxMetrics(
            cpuCount = this.cpuCount,
            cpuUsedPercentage = cpuUsedPct,
            memoryTotalInMiB = memTotalMib,
            memoryUsedInMiB = memUsedMib,
            timestamp = this.timestamp,
        )
    }

    fun RenewSandboxExpirationResponse.toSandboxRenewResponse(): SandboxRenewResponse {
        return SandboxRenewResponse(
            expiresAt = this.expiresAt,
        )
    }
}
