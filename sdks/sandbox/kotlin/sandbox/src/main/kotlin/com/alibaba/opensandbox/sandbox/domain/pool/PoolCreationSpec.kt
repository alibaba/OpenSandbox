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

import com.alibaba.opensandbox.sandbox.Sandbox
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.NetworkPolicy
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxImageSpec
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.Volume

/**
 * Template for creating sandboxes in the pool (replenish and direct-create).
 *
 * Pool always uses a fixed 24h timeout for created sandboxes; other parameters
 * are taken from this spec. Defaults align with [Sandbox.Builder].
 *
 * @property imageSpec Container image specification (required).
 * @property entrypoint Entrypoint command (default: tail -f /dev/null).
 * @property resource Resource limits (default: cpu=1, memory=2Gi).
 * @property env Environment variables.
 * @property metadata User-defined metadata.
 * @property networkPolicy Optional outbound network policy.
 * @property volumes Optional volume mounts.
 */
data class PoolCreationSpec(
    val imageSpec: SandboxImageSpec,
    val entrypoint: List<String> = DEFAULT_ENTRYPOINT,
    val resource: Map<String, String> = DEFAULT_RESOURCE,
    val env: Map<String, String> = emptyMap(),
    val metadata: Map<String, String> = emptyMap(),
    val networkPolicy: NetworkPolicy? = null,
    val volumes: List<Volume>? = null,
) {
    companion object {
        /** Default entrypoint: keep container running. */
        val DEFAULT_ENTRYPOINT: List<String> = listOf("tail", "-f", "/dev/null")

        /** Default resource limits. */
        val DEFAULT_RESOURCE: Map<String, String> =
            mapOf(
                "cpu" to "1",
                "memory" to "2Gi",
            )

        @JvmStatic
        fun builder(): Builder = Builder()
    }

    class Builder {
        private var imageSpec: SandboxImageSpec? = null
        private var entrypoint: List<String> = DEFAULT_ENTRYPOINT
        private var resource: Map<String, String> = DEFAULT_RESOURCE
        private var env: Map<String, String> = emptyMap()
        private var metadata: Map<String, String> = emptyMap()
        private var networkPolicy: NetworkPolicy? = null
        private var volumes: List<Volume>? = null

        fun imageSpec(imageSpec: SandboxImageSpec): Builder {
            this.imageSpec = imageSpec
            return this
        }

        fun image(image: String): Builder {
            this.imageSpec = SandboxImageSpec.builder().image(image).build()
            return this
        }

        fun entrypoint(entrypoint: List<String>): Builder {
            this.entrypoint = entrypoint
            return this
        }

        fun entrypoint(vararg entrypoint: String): Builder {
            this.entrypoint = entrypoint.toList()
            return this
        }

        fun resource(resource: Map<String, String>): Builder {
            this.resource = resource
            return this
        }

        fun resource(configure: MutableMap<String, String>.() -> Unit): Builder {
            val map = DEFAULT_RESOURCE.toMutableMap()
            map.configure()
            this.resource = map
            return this
        }

        fun env(env: Map<String, String>): Builder {
            this.env = env
            return this
        }

        fun metadata(metadata: Map<String, String>): Builder {
            this.metadata = metadata
            return this
        }

        fun networkPolicy(networkPolicy: NetworkPolicy?): Builder {
            this.networkPolicy = networkPolicy
            return this
        }

        fun volumes(volumes: List<Volume>?): Builder {
            this.volumes = volumes
            return this
        }

        fun build(): PoolCreationSpec {
            val spec = imageSpec ?: throw IllegalArgumentException("PoolCreationSpec imageSpec (or image) must be specified")
            return PoolCreationSpec(
                imageSpec = spec,
                entrypoint = entrypoint,
                resource = resource,
                env = env,
                metadata = metadata,
                networkPolicy = networkPolicy,
                volumes = volumes,
            )
        }
    }
}
