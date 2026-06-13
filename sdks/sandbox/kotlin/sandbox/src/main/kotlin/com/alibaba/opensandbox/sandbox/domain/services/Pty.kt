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

package com.alibaba.opensandbox.sandbox.domain.services

import com.alibaba.opensandbox.sandbox.domain.models.execd.pty.PtyMode
import com.alibaba.opensandbox.sandbox.domain.models.execd.pty.PtySession
import com.alibaba.opensandbox.sandbox.domain.models.execd.pty.PtySessionStatus

/**
 * Interactive pseudo-terminal (PTY) session lifecycle for a sandbox.
 *
 * A PTY session is a long-lived shell driven over a WebSocket. This service manages the
 * session lifecycle over execd's HTTP API ([createSession] / [getSession] / [deleteSession])
 * and builds the WebSocket URL ([webSocketUrl]) that a client connects to in order to stream
 * the interactive session. Driving the WebSocket itself (binary stdin/stdout frames, resize,
 * takeover) is left to the caller, which can use any WebSocket client.
 *
 * PTY is only supported on Unix-like platforms (Linux/macOS).
 */
interface Pty {
    /**
     * Creates a new PTY session. The shell does not start until the first WebSocket attaches.
     *
     * @param cwd Optional working directory for the shell
     * @param command Optional command to run instead of the default login shell
     * @return The created session
     */
    fun createSession(
        cwd: String? = null,
        command: String? = null,
    ): PtySession

    /**
     * Retrieves the current status of a PTY session.
     *
     * @param sessionId Identifier of the PTY session
     * @return Session status, including the output offset usable for replay
     */
    fun getSession(sessionId: String): PtySessionStatus

    /**
     * Tears down a PTY session on the server side.
     *
     * @param sessionId Identifier of the PTY session
     */
    fun deleteSession(sessionId: String)

    /**
     * Builds the WebSocket URL used to attach to a PTY session.
     *
     * @param sessionId Identifier of the PTY session
     * @param mode Streaming mode ([PtyMode.PTY] by default; [PtyMode.PIPE] adds `pty=0`)
     * @param since Optional byte offset to replay buffered output from on reconnect
     * @param takeover When true, evict the current holder (`takeover=1`) and attach to the same shell
     * @return A `ws://` or `wss://` URL (scheme derived from the connection protocol)
     */
    fun webSocketUrl(
        sessionId: String,
        mode: PtyMode = PtyMode.PTY,
        since: Long? = null,
        takeover: Boolean = false,
    ): String
}
