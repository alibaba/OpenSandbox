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

package com.alibaba.opensandbox.sandbox.config

import okhttp3.ConnectionPool
import java.time.Duration
import java.util.concurrent.TimeUnit

/**
 * Sandbox operations connection configuration.
 */
class ConnectionConfig private constructor(
    /** API key for authentication with sandbox service */
    private val apiKey: String?,
    /** Base URL for the sandbox management API */
    private val domain: String?,
    /** Protocol to use (http/https) */
    val protocol: String,
    /** Timeout for HTTP requests to the management API */
    val requestTimeout: Duration,
    /** Enable debug logging for HTTP requests */
    val debug: Boolean = false,
    /** user agent */
    val userAgent: String = DEFAULT_USER_AGENT,
    /** User defined headers */
    val headers: Map<String, String> = mutableMapOf(),
    /** Connection pool */
    val connectionPool: ConnectionPool,
    /** Whether the connection pool is managed by the user */
    val connectionPoolManagedByUser: Boolean,
) {
    companion object {
        private const val DEFAULT_DOMAIN = "localhost:8080"
        private const val DEFAULT_PROTOCOL = "http"
        private const val ENV_API_KEY = "OPEN_SANDBOX_API_KEY"
        private const val ENV_DOMAIN = "OPEN_SANDBOX_DOMAIN"

        private const val DEFAULT_USER_AGENT = "OpenSandbox-Kotlin-SDK/1.0.1"

        @JvmStatic
        fun builder(): Builder = Builder()
    }

    fun getApiKey(): String {
        return this.apiKey ?: System.getenv(ENV_API_KEY) ?: ""
    }

    fun getDomain(): String {
        return this.domain ?: System.getenv(ENV_DOMAIN) ?: DEFAULT_DOMAIN
    }

    fun getBaseUrl(): String {
        val currentDomain = getDomain()
        // Allow domain to override protocol if it explicitly starts with a scheme
        if (currentDomain.startsWith("http://") || currentDomain.startsWith("https://")) {
            return currentDomain
        }
        return "$protocol://$currentDomain"
    }

    /**
     * Builder for [ConnectionConfig].
     */
    class Builder internal constructor() {
        private var apiKey: String? = null
        private var domain: String? = null
        private var protocol: String = DEFAULT_PROTOCOL
        private var requestTimeout: Duration = Duration.ofSeconds(30)
        private var debug: Boolean = false
        private var headers: Map<String, String> = mutableMapOf()
        private var connectionPool: ConnectionPool = ConnectionPool(32, 1, TimeUnit.MINUTES)
        private var connectionPoolManagedByUser: Boolean = false

        fun apiKey(apiKey: String): Builder {
            require(apiKey.isNotBlank()) { "API key cannot be blank" }
            this.apiKey = apiKey
            return this
        }

        fun domain(domain: String): Builder {
            require(domain.isNotBlank()) { "Domain cannot be blank" }
            this.domain = domain
            return this
        }

        /**
         * Sets the protocol
         * Defaults to "http".
         */
        fun protocol(protocol: String): Builder {
            this.protocol = protocol.lowercase()
            return this
        }

        fun requestTimeout(requestTimeout: Duration): Builder {
            require(!requestTimeout.isNegative && !requestTimeout.isZero) {
                "Request timeout must be positive, got: $requestTimeout"
            }
            this.requestTimeout = requestTimeout
            return this
        }

        fun connectionPool(connectionPool: ConnectionPool): Builder {
            this.connectionPool = connectionPool
            this.connectionPoolManagedByUser = true
            return this
        }

        fun debug(enable: Boolean = true): Builder {
            this.debug = enable
            return this
        }

        fun headers(headers: Map<String, String>): Builder {
            this.headers = headers
            return this
        }

        fun headers(configure: MutableMap<String, String>.() -> Unit): Builder {
            val map = mutableMapOf<String, String>()
            map.configure()
            this.headers = map
            return this
        }

        fun addHeader(
            key: String,
            value: String,
        ): Builder {
            require(key.isNotBlank()) { "Header key cannot be blank" }
            val mutableHeaders = this.headers.toMutableMap()
            mutableHeaders[key] = value
            this.headers = mutableHeaders
            return this
        }

        fun build(): ConnectionConfig {
            return ConnectionConfig(
                apiKey = apiKey,
                domain = domain,
                protocol = protocol,
                requestTimeout = requestTimeout,
                debug = debug,
                userAgent = DEFAULT_USER_AGENT,
                headers = headers,
                connectionPool = connectionPool,
                connectionPoolManagedByUser = connectionPoolManagedByUser,
            )
        }
    }
}
