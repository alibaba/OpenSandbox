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
        private const val API_VERSION = "v1"

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
        return "$protocol://$currentDomain/$API_VERSION"
    }

    /**
     * Builder for [ConnectionConfig].
     *
     * This builder is part of the public SDK surface and is intended to be used directly by end users.
     *
     * ### Defaults & environment variables
     * - If `apiKey` is not provided, the SDK will read it from environment variable `OPEN_SANDBOX_API_KEY`.
     * - If `domain` is not provided, the SDK will read it from environment variable `OPEN_SANDBOX_DOMAIN`,
     *   falling back to `localhost:8080`.
     *
     * ### Lifecycle / resource ownership
     * - If you do **not** provide a custom [ConnectionPool], the SDK creates and owns a default one.
     *   Calling `Sandbox.close()` / `SandboxManager.close()` will close SDK-owned HTTP clients and
     *   release the SDK-owned connection pool.
     * - If you **do** provide a [ConnectionPool] via [connectionPool], it is treated as user-owned
     *   and will **not** be evicted by the SDK on close.
     *
     * ### Notes
     * - `domain` may include a scheme (e.g. `https://example.com`); in that case the SDK will use it
     *   as-is and ignore [protocol] when constructing the base URL.
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

        /**
         * Set the API key used for authentication.
         *
         * If not set, the SDK falls back to environment variable `OPEN_SANDBOX_API_KEY`.
         */
        fun apiKey(apiKey: String): Builder {
            require(apiKey.isNotBlank()) { "API key cannot be blank" }
            this.apiKey = apiKey
            return this
        }

        /**
         * Set the API domain (host[:port]) or a full base URL.
         *
         * Examples:
         * - `pre-agent-sandbox.alibaba-inc.com`
         * - `localhost:8080`
         * - `https://pre-agent-sandbox.alibaba-inc.com` (scheme included; [protocol] will be ignored)
         *
         * If not set, the SDK falls back to environment variable `OPEN_SANDBOX_DOMAIN`
         * and then `localhost:8080`.
         */
        fun domain(domain: String): Builder {
            require(domain.isNotBlank()) { "Domain cannot be blank" }
            this.domain = domain
            return this
        }

        /**
         * Sets the protocol
         * Defaults to "http".
         *
         * Note: if [domain] includes a scheme (starts with `http://` or `https://`),
         * the SDK will use that and ignore this value when building the base URL.
         */
        fun protocol(protocol: String): Builder {
            this.protocol = protocol.lowercase()
            return this
        }

        /**
         * Sets the request timeout used by the management API HTTP client.
         *
         * Must be a positive duration.
         */
        fun requestTimeout(requestTimeout: Duration): Builder {
            require(!requestTimeout.isNegative && !requestTimeout.isZero) {
                "Request timeout must be positive, got: $requestTimeout"
            }
            this.requestTimeout = requestTimeout
            return this
        }

        /**
         * Provide a custom OkHttp [ConnectionPool].
         *
         * Ownership semantics:
         * - When you call this method, the pool is considered user-managed, and the SDK will not
         *   evict it on close.
         */
        fun connectionPool(connectionPool: ConnectionPool): Builder {
            this.connectionPool = connectionPool
            this.connectionPoolManagedByUser = true
            return this
        }

        /**
         * Enable or disable HTTP request logging (headers).
         *
         * This is intended for local debugging. Sensitive headers will be redacted.
         */
        fun debug(enable: Boolean = true): Builder {
            this.debug = enable
            return this
        }

        /**
         * Set extra headers that will be sent with every SDK request.
         *
         * Note: authentication header is managed by the SDK; you normally should not set
         * `OPEN-SANDBOX-API-KEY` manually here.
         */
        fun headers(headers: Map<String, String>): Builder {
            this.headers = headers
            return this
        }

        /**
         * Convenience DSL for setting extra headers.
         *
         * Example:
         * ```
         * ConnectionConfig.builder()
         *   .headers {
         *     put("X-Request-ID", "trace-123")
         *   }
         *   .build()
         * ```
         */
        fun headers(configure: MutableMap<String, String>.() -> Unit): Builder {
            val map = mutableMapOf<String, String>()
            map.configure()
            this.headers = map
            return this
        }

        /**
         * Add a single extra header.
         *
         * This is equivalent to mutating [headers] and overwriting the value for the same key.
         */
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

        /**
         * Build an immutable [ConnectionConfig].
         */
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
