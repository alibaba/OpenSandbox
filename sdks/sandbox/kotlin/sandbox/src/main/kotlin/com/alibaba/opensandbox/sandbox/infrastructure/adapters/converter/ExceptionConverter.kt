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

import com.alibaba.opensandbox.sandbox.api.infrastructure.ClientError
import com.alibaba.opensandbox.sandbox.api.infrastructure.ClientException
import com.alibaba.opensandbox.sandbox.api.infrastructure.ServerError
import com.alibaba.opensandbox.sandbox.api.infrastructure.ServerException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxApiException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxError
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxError.Companion.UNEXPECTED_RESPONSE
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxException
import com.alibaba.opensandbox.sandbox.domain.exceptions.SandboxInternalException
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement
import java.io.IOException
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ClientError as ExecdClientError
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ClientException as ExecdClientException
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ServerError as ExecdServerError
import com.alibaba.opensandbox.sandbox.api.execd.infrastructure.ServerException as ExecdServerException

fun Exception.toSandboxException(): SandboxException {
    return when (this) {
        is SandboxException -> this
        is ClientException, is ServerException,
        is ExecdClientException, is ExecdServerException,
        -> this.toApiException()
        is IOException ->
            SandboxInternalException(
                message = "Network connectivity error: ${this.message}",
                cause = this,
            )
        is IllegalStateException, is IllegalArgumentException ->
            SandboxInternalException(
                message = "SDK internal usage error: ${this.message}",
                cause = this,
            )
        is UnsupportedOperationException ->
            SandboxInternalException(
                message = "Operation not supported: ${this.message}",
                cause = this,
            )
        else ->
            SandboxInternalException(
                message = "Unexpected SDK error occurred: ${this.message}",
                cause = this,
            )
    }
}

private fun Exception.toApiException(): SandboxApiException {
    val (statusCode, rawResponse) =
        when (this) {
            is ClientException -> this.statusCode to this.response
            is ServerException -> this.statusCode to this.response
            is ExecdClientException -> this.statusCode to this.response
            is ExecdServerException -> this.statusCode to this.response
            else -> 0 to null
        }

    val errorBody =
        when (rawResponse) {
            is ClientError<*> -> rawResponse.body
            is ExecdServerError<*> -> rawResponse.body
            is ServerError<*> -> rawResponse.body
            is ExecdClientError<*> -> rawResponse.body
            else -> null
        }

    val sandboxError =
        parseSandboxError(errorBody) ?: if (errorBody is String) {
            SandboxError(UNEXPECTED_RESPONSE, errorBody)
        } else {
            SandboxError(UNEXPECTED_RESPONSE)
        }

    return SandboxApiException(
        message = this.message,
        statusCode = statusCode,
        cause = this,
        error = sandboxError,
    )
}

fun parseSandboxError(body: Any?): SandboxError? {
    if (body == null) return null

    return runCatching {
        val jsonElement: JsonElement =
            when (body) {
                is String -> jsonParser.parseToJsonElement(body)
                else -> jsonParser.encodeToJsonElement(body)
            }

        val generic = jsonParser.decodeFromJsonElement<GenericErrorBody>(jsonElement)

        if (!generic.code.isNullOrBlank()) {
            SandboxError(code = generic.code, message = generic.message)
        } else {
            null
        }
    }.getOrNull()
}

@Serializable
private data class GenericErrorBody(
    val code: String? = null,
    val message: String? = null,
)
