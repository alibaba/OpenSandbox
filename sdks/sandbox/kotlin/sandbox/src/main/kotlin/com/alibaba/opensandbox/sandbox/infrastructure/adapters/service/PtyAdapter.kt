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
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxApiException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxError
import com.alibaba.opensandbox.sandbox.domain.models.execd.pty.PtyMode
import com.alibaba.opensandbox.sandbox.domain.models.execd.pty.PtySession
import com.alibaba.opensandbox.sandbox.domain.models.execd.pty.PtySessionStatus
import com.alibaba.opensandbox.sandbox.domain.models.execd.pty.PtyWebSocket
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxEndpoint
import com.alibaba.opensandbox.sandbox.domain.services.Pty
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.jsonParser
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.parseSandboxError
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.toSandboxException
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import okhttp3.Headers.Companion.toHeaders
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.slf4j.LoggerFactory

/**
 * Implementation of [Pty] backed by execd's PTY HTTP endpoints.
 *
 * execd's interactive PTY channel is a WebSocket and cannot be modelled by the OpenAPI
 * generator, so this adapter is handwritten transport over the shared OkHttp client, following
 * the same endpoint/header wiring, scheme resolution and error mapping as the other execd adapters.
 */
internal class PtyAdapter(
    private val httpClientProvider: HttpClientProvider,
    private val execdEndpoint: SandboxEndpoint,
) : Pty {
    companion object {
        private const val PTY_PATH = "/pty"
        private const val REQUEST_ID_HEADER = "X-Request-ID"
        private val JSON_MEDIA_TYPE = "application/json".toMediaType()
    }

    private val logger = LoggerFactory.getLogger(PtyAdapter::class.java)

    // Honor an https domain even when `protocol` is left at its default, matching how the
    // lifecycle base URL is resolved, so secured/server-proxy deployments use the right scheme.
    private val httpScheme = resolveHttpScheme()
    private val execdBaseUrl = "$httpScheme://${execdEndpoint.endpoint}"
    private val execdApiClient =
        httpClientProvider.httpClient.newBuilder()
            .addInterceptor { chain ->
                val requestBuilder = chain.request().newBuilder()
                execdEndpoint.headers.forEach { (key, value) ->
                    requestBuilder.header(key, value)
                }
                chain.proceed(requestBuilder.build())
            }
            .build()

    override fun createSession(
        cwd: String?,
        command: String?,
    ): PtySession {
        try {
            val body = jsonParser.encodeToString(CreatePtySessionRequest(cwd, command))
            val request =
                Request.Builder()
                    .url("$execdBaseUrl$PTY_PATH")
                    .post(body.toRequestBody(JSON_MEDIA_TYPE))
                    .headers(execdEndpoint.headers.toHeaders())
                    .build()
            execdApiClient.newCall(request).execute().use { response ->
                val payload = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    throw apiException("Failed to create PTY session", response, payload)
                }
                val parsed = jsonParser.decodeFromString<CreatePtySessionResponse>(payload)
                return PtySession(parsed.sessionId)
            }
        } catch (e: Exception) {
            logger.error("Failed to create PTY session", e)
            throw e.toSandboxException()
        }
    }

    override fun getSession(sessionId: String): PtySessionStatus {
        try {
            val request =
                Request.Builder()
                    .url("$execdBaseUrl$PTY_PATH/$sessionId")
                    .get()
                    .headers(execdEndpoint.headers.toHeaders())
                    .build()
            execdApiClient.newCall(request).execute().use { response ->
                val payload = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    throw apiException("Failed to get PTY session $sessionId", response, payload)
                }
                val parsed = jsonParser.decodeFromString<PtySessionStatusResponse>(payload)
                return PtySessionStatus(parsed.sessionId, parsed.running, parsed.outputOffset)
            }
        } catch (e: Exception) {
            logger.error("Failed to get PTY session {}", sessionId, e)
            throw e.toSandboxException()
        }
    }

    override fun deleteSession(sessionId: String) {
        try {
            val request =
                Request.Builder()
                    .url("$execdBaseUrl$PTY_PATH/$sessionId")
                    .delete()
                    .headers(execdEndpoint.headers.toHeaders())
                    .build()
            execdApiClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    val payload = response.body?.string().orEmpty()
                    throw apiException("Failed to delete PTY session $sessionId", response, payload)
                }
            }
        } catch (e: Exception) {
            logger.error("Failed to delete PTY session {}", sessionId, e)
            throw e.toSandboxException()
        }
    }

    override fun webSocket(
        sessionId: String,
        mode: PtyMode,
        since: Long?,
        takeover: Boolean,
    ): PtyWebSocket {
        val scheme = if (httpScheme == "https") "wss" else "ws"
        val params =
            buildList {
                if (mode == PtyMode.PIPE) add("pty=0")
                if (since != null) add("since=$since")
                if (takeover) add("takeover=1")
            }
        val query = if (params.isEmpty()) "" else "?" + params.joinToString("&")
        val url = "$scheme://${execdEndpoint.endpoint}$PTY_PATH/$sessionId/ws$query"
        // Send the same headers the REST/HTTP calls send on the handshake: the user-configured
        // ConnectionConfig.headers (applied by HttpClientProvider) plus the endpoint routing/auth
        // headers, with endpoint headers taking precedence on conflicts.
        val headers = httpClientProvider.config.headers + execdEndpoint.headers
        return PtyWebSocket(url, headers)
    }

    private fun resolveHttpScheme(): String {
        val domain = httpClientProvider.config.getDomain()
        return when {
            domain.startsWith("https://", ignoreCase = true) -> "https"
            domain.startsWith("http://", ignoreCase = true) -> "http"
            else -> httpClientProvider.config.protocol
        }
    }

    private fun apiException(
        action: String,
        response: Response,
        payload: String,
    ): SandboxApiException =
        SandboxApiException(
            message = "$action. Status code: ${response.code}, Body: $payload",
            statusCode = response.code,
            error = parseSandboxError(payload) ?: SandboxError(SandboxError.UNEXPECTED_RESPONSE),
            requestId = response.header(REQUEST_ID_HEADER),
        )

    @Serializable
    private data class CreatePtySessionRequest(
        val cwd: String? = null,
        val command: String? = null,
    )

    @Serializable
    private data class CreatePtySessionResponse(
        @SerialName("session_id") val sessionId: String,
    )

    @Serializable
    private data class PtySessionStatusResponse(
        @SerialName("session_id") val sessionId: String,
        @SerialName("running") val running: Boolean = false,
        @SerialName("output_offset") val outputOffset: Long = 0,
    )
}
