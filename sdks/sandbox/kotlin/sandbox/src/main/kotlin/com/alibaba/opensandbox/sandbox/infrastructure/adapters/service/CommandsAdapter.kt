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
import com.alibaba.opensandbox.sandbox.api.execd.CommandApi
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ClientError
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ClientException
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ResponseType
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ServerError
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ServerException
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.Success
import com.alibaba.opensandbox.sandbox.api.models.execd.EventNode
import com.alibaba.opensandbox.sandbox.domain.exceptions.InvalidArgumentException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxApiException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxError
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxError.Companion.UNEXPECTED_RESPONSE
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.CommandLogs
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.CommandStatus
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.Execution
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.RunCommandRequest
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxEndpoint
import com.alibaba.opensandbox.sandbox.domain.services.Commands
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.ExecutionConverter.toApiRunCommandRequest
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.ExecutionConverter.toCommandStatus
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.ExecutionEventDispatcher
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.jsonParser
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.parseSandboxError
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.toSandboxException
import okhttp3.Headers.Companion.toHeaders
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.slf4j.LoggerFactory

/**
 * Implementation of [Commands] that adapts OpenAPI-generated [CommandApi].
 *
 * This adapter handles command execution within sandboxes, providing both
 * synchronous and streaming execution modes with proper session management.
 */
internal class CommandsAdapter(
    private val httpClientProvider: HttpClientProvider,
    private val execdEndpoint: SandboxEndpoint,
) : Commands {
    companion object {
        private const val RUN_COMMAND_PATH = "/command"
    }

    private val logger = LoggerFactory.getLogger(CommandsAdapter::class.java)
    private val api =
        CommandApi(
            "${httpClientProvider.config.protocol}://${execdEndpoint.endpoint}",
            httpClientProvider.httpClient.newBuilder()
                .addInterceptor { chain ->
                    val requestBuilder = chain.request().newBuilder()
                    execdEndpoint.headers.forEach { (key, value) ->
                        requestBuilder.header(key, value)
                    }
                    chain.proceed(requestBuilder.build())
                }
                .build(),
        )

    override fun run(request: RunCommandRequest): Execution {
        if (request.command.isEmpty()) {
            throw InvalidArgumentException("Command cannot be empty")
        }
        try {
            val httpRequest =
                Request.Builder()
                    .url("${httpClientProvider.config.protocol}://${execdEndpoint.endpoint}$RUN_COMMAND_PATH")
                    .post(
                        jsonParser.encodeToString(request.toApiRunCommandRequest()).toRequestBody("application/json".toMediaType()),
                    )
                    .headers(execdEndpoint.headers.toHeaders())
                    .build()

            val execution = Execution()

            httpClientProvider.sseClient.newCall(httpRequest).execute().use { response ->
                if (!response.isSuccessful) {
                    val errorBodyString = response.body?.string()
                    val sandboxError = parseSandboxError(errorBodyString)
                    val message = "Failed to run commands. Status code: ${response.code}, Body: $errorBodyString"
                    throw SandboxApiException(
                        message = message,
                        statusCode = response.code,
                        error = sandboxError ?: SandboxError(UNEXPECTED_RESPONSE),
                    )
                }

                response.body?.byteStream()?.bufferedReader(Charsets.UTF_8)?.use { reader ->
                    val dispatcher = ExecutionEventDispatcher(execution, request.handlers)
                    reader.lineSequence()
                        .filter(String::isNotBlank)
                        .forEach { line ->
                            try {
                                val eventNode = jsonParser.decodeFromString<EventNode>(line)
                                dispatcher.dispatch(eventNode)
                            } catch (e: Exception) {
                                logger.error("Failed to parse SSE line: {}", line, e)
                            }
                        }
                }
            }
            return execution
        } catch (e: Exception) {
            logger.error("Failed to run command (length: {})", request.command.length, e)
            throw e.toSandboxException()
        }
    }

    override fun interrupt(executionId: String) {
        try {
            api.interruptCommand(executionId)
        } catch (e: Exception) {
            logger.error("Failed to interrupt command", e)
            throw e.toSandboxException()
        }
    }

    override fun getCommandStatus(executionId: String): CommandStatus {
        return try {
            val status = api.getCommandStatus(executionId)
            status.toCommandStatus()
        } catch (e: Exception) {
            logger.error("Failed to get command status", e)
            throw e.toSandboxException()
        }
    }

    override fun getBackgroundCommandLogs(
        executionId: String,
        cursor: Long?,
    ): CommandLogs {
        return try {
            val localVarResponse = api.getBackgroundCommandLogsWithHttpInfo(executionId, cursor)
            val content =
                when (localVarResponse.responseType) {
                    ResponseType.Success -> (localVarResponse as Success<*>).data as String
                    ResponseType.Informational ->
                        throw UnsupportedOperationException("Client does not support Informational responses.")
                    ResponseType.Redirection ->
                        throw UnsupportedOperationException("Client does not support Redirection responses.")
                    ResponseType.ClientError -> {
                        val localVarError = localVarResponse as ClientError<*>
                        throw ClientException(
                            "Client error : ${localVarError.statusCode} ${localVarError.message.orEmpty()}",
                            localVarError.statusCode,
                            localVarResponse,
                        )
                    }
                    ResponseType.ServerError -> {
                        val localVarError = localVarResponse as ServerError<*>
                        throw ServerException(
                            "Server error : ${localVarError.statusCode} ${localVarError.message.orEmpty()} ${localVarError.body}",
                            localVarError.statusCode,
                            localVarResponse,
                        )
                    }
                }
            val cursorHeader =
                localVarResponse.headers["EXECD-COMMANDS-TAIL-CURSOR"]?.firstOrNull()
            val nextCursor = cursorHeader?.toLongOrNull()
            CommandLogs(content = content, cursor = nextCursor)
        } catch (e: Exception) {
            logger.error("Failed to get command logs", e)
            throw e.toSandboxException()
        }
    }
}
