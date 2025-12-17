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

package com.alibaba.opensandbox.sandbox.domain.exceptions

/**
 * Base exception class for all sandbox-related errors.
 *
 * Inherits from [RuntimeException] (Unchecked Exception) to avoid forcing
 * Java callers to implement verbose try-catch blocks while still allowing
 * specific error handling when needed.
 */
open class SandboxException(
    message: String? = null,
    cause: Throwable? = null,
    val error: SandboxError,
) : RuntimeException(message, cause)

/**
 * Thrown when the Sandbox API returns an error response (e.g., HTTP 4xx or 5xx) or meet unexpected error when calling api.
 */
class SandboxApiException(
    message: String? = null,
    cause: Throwable? = null,
    val statusCode: Int? = null,
    error: SandboxError = SandboxError(SandboxError.UNEXPECTED_RESPONSE),
) : SandboxException(message, cause, error)

/**
 * Thrown when an unexpected internal error occurs within the SDK
 */
class SandboxInternalException(
    message: String? = null,
    cause: Throwable? = null,
) : SandboxException(
        message = message,
        cause = cause,
        error = SandboxError(SandboxError.INTERNAL_UNKNOWN_ERROR),
    )

/**
 * Thrown when the operation times out waiting for the sandbox to become ready.
 */
class SandboxUnhealthyException(
    message: String? = null,
    cause: Throwable? = null,
) : SandboxException(
        message = message,
        cause = cause,
        error = SandboxError(SandboxError.UNHEALTHY, message),
    )

/**
 * Thrown when the operation times out waiting for the sandbox to become ready.
 */
class SandboxReadyTimeoutException(
    message: String? = null,
    cause: Throwable? = null,
) : SandboxException(
        message = message,
        cause = cause,
        error = SandboxError(SandboxError.READY_TIMEOUT, message),
    )

/**
 * Thrown when an invalid argument is provided to an SDK method.
 * Similar to [IllegalArgumentException] but within the SDK's exception hierarchy.
 */
class InvalidArgumentException(
    message: String? = null,
    cause: Throwable? = null,
) : SandboxException(
        message = message,
        cause = cause,
        error = SandboxError(SandboxError.INVALID_ARGUMENT, message),
    )

/**
 * Defines standardized common error codes and messages for the Sandbox SDK.
 */
data class SandboxError(
    val code: String,
    val message: String? = null,
) {
    companion object {
        const val INTERNAL_UNKNOWN_ERROR = "INTERNAL_UNKNOWN_ERROR"
        const val READY_TIMEOUT = "READY_TIMEOUT"
        const val UNHEALTHY = "UNHEALTHY"
        const val INVALID_ARGUMENT = "INVALID_ARGUMENT"
        const val UNEXPECTED_RESPONSE = "UNEXPECTED_RESPONSE"
    }
}
