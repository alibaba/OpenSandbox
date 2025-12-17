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

package com.alibaba.opensandbox.codeinterpreter.domain.services

import com.alibaba.opensandbox.codeinterpreter.domain.models.execd.executions.CodeContext
import com.alibaba.opensandbox.codeinterpreter.domain.models.execd.executions.RunCodeRequest
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.Execution
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.ExecutionHandlers

/**
 * Code execution operations for multi-language code interpretation.
 *
 * This service provides advanced code execution capabilities with context management,
 * session persistence, and multi-language support.
 */
interface Codes {
    /**
     * Creates a new execution context for code interpretation.
     *
     * @param language The programming language for this context (e.g., "python", "javascript")
     * @return A new [CodeContext] with the specified configuration
     */
    fun createContext(language: String): CodeContext

    /**
     * Executes code within the specified context.
     *
     * @param request The code execution request containing code and context
     * @return Execution with stdout, stderr, exit code, and execution metadata
     */
    fun run(request: RunCodeRequest): Execution


    /**
     * Executes code within the specified context.
     *
     * @param code The code to run
     * @param context The context to run code
     * @param handlers execution events handlers
     * @return Execution with stdout, stderr, exit code, and execution metadata
     */
    fun run(code: String, context: CodeContext, handlers: ExecutionHandlers): Execution {
        return run(RunCodeRequest.builder().code(code).context(context).handlers(handlers).build())
    }


    /**
     * Interrupts a currently running code execution.
     *
     * @param executionId The unique identifier of the execution to interrupt
     */
    fun interrupt(executionId: String)
}
