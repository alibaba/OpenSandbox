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

package com.alibaba.opensandbox.sandbox.domain.models.execd.executions

import kotlin.time.Duration

/**
 * Parameters for command execution.
 *
 * @property command The command content to execute
 * @property background Whether to run in background (detached)
 * @property workingDirectory Directory to execute command in
 * @property timeout Maximum execution time; server will terminate when reached.  Null means the server will not enforce any timeout.
 * @property handlers Optional execution handlers
 */
class RunCommandRequest private constructor(
    val command: String,
    val background: Boolean,
    val workingDirectory: String?,
    val timeout: Duration?,
    val handlers: ExecutionHandlers?,
) {
    companion object {
        @JvmStatic
        fun builder(): Builder = Builder()
    }

    class Builder {
        private var command: String? = null
        private var background: Boolean = false
        private var workingDirectory: String? = null
        private var timeout: Duration? = null
        private var handlers: ExecutionHandlers? = null

        fun command(command: String): Builder {
            require(command.isNotBlank()) { "Command cannot be blank" }
            this.command = command
            return this
        }

        fun background(background: Boolean): Builder {
            this.background = background
            return this
        }

        fun workingDirectory(workingDirectory: String?): Builder {
            this.workingDirectory = workingDirectory
            return this
        }

        /**
         * Maximum execution time; server will terminate the command when reached.
         * If omitted, the server will not enforce any timeout.
         */
        fun timeout(timeout: Duration?): Builder {
            this.timeout = timeout
            return this
        }

        fun handlers(handlers: ExecutionHandlers?): Builder {
            this.handlers = handlers
            return this
        }

        fun build(): RunCommandRequest {
            val commandValue = command ?: throw IllegalArgumentException("Command must be specified")
            return RunCommandRequest(
                command = commandValue,
                background = background,
                workingDirectory = workingDirectory,
                timeout = timeout,
                handlers = handlers,
            )
        }
    }
}
