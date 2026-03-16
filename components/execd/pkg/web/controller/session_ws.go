// Copyright 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// execd runs inside a container; auth-header check is the access gate.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// SessionWebSocket handles GET /ws/session/:sessionId — bidirectional stdin/stdout steering.
func (c *CodeInterpretingController) SessionWebSocket() {
	sessionID := c.ctx.Param("sessionId")

	// 1. Look up session BEFORE upgrade so we can still return HTTP errors.
	session := codeRunner.GetBashSession(sessionID)
	if session == nil {
		c.RespondError(http.StatusNotFound, model.ErrorCodeContextNotFound, "session not found")
		return
	}

	// 2. Acquire exclusive WS lock (prevents concurrent connections).
	if !session.LockWS() {
		c.RespondError(http.StatusConflict, model.ErrorCodeRuntimeError, "session already connected")
		return
	}
	defer session.UnlockWS()

	// 3. Upgrade HTTP → WebSocket.
	conn, err := wsUpgrader.Upgrade(c.ctx.Writer, c.ctx.Request, nil)
	if err != nil {
		// gorilla writes the HTTP error response automatically.
		return
	}
	defer conn.Close()

	// writeMu serializes all writes to conn — gorilla/websocket requires this.
	var writeMu sync.Mutex
	writeJSON := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
		return conn.WriteJSON(v)
	}

	usePTY := c.ctx.Query("pty") == "1"

	// 4. Start bash if not already running.
	if !session.IsRunning() {
		var startErr error
		if usePTY {
			startErr = session.StartPTY()
		} else {
			startErr = session.Start()
		}
		if startErr != nil {
			_ = writeJSON(model.ServerFrame{
				Type:  "error",
				Error: "failed to start bash",
				Code:  model.WSErrCodeStartFailed,
			})
			return
		}
	}

	// 5. Replay buffered output if ?since= is provided.
	if sinceStr := c.ctx.Query("since"); sinceStr != "" {
		if since, parseErr := strconv.ParseInt(sinceStr, 10, 64); parseErr == nil {
			replayData, nextOffset, _ := codeRunner.ReplaySessionOutput(sessionID, since)
			if len(replayData) > 0 {
				_ = writeJSON(model.ServerFrame{
					Type:   "replay",
					Data:   string(replayData),
					Offset: nextOffset,
				})
			}
		}
	}

	// 6. Send connected frame — mode derived from actual session state, not the request parameter,
	// so reconnecting clients always receive the correct terminal assumptions.
	mode := "pipe"
	if session.IsPTY() {
		mode = "pty"
	}
	_ = writeJSON(model.ServerFrame{
		Type:      "connected",
		SessionID: sessionID,
		Mode:      mode,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 7. Ping/pong keepalive — RFC 6455 control-level pings every 30s.
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck
		return nil
	})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				writeMu.Lock()
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
				writeErr := conn.WriteMessage(websocket.PingMessage, nil)
				writeMu.Unlock()
				if writeErr != nil {
					cancel()
					return
				}
			}
		}
	}()

	// 8. Write pump — stdout scanner.
	// Buffer sized to 16 MiB to handle large JSON/base64 lines from agent tools.
	go func() {
		stdout := session.StdoutPipe()
		if stdout == nil {
			return
		}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := scanner.Text() + "\n"
			// Write to replay buffer so reconnecting clients can catch up.
			codeRunner.WriteSessionOutput(sessionID, []byte(line))
			if writeErr := writeJSON(model.ServerFrame{
				Type:      "stdout",
				Data:      line,
				Timestamp: time.Now().UnixMilli(),
			}); writeErr != nil {
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			_ = writeJSON(model.ServerFrame{
				Type:  "error",
				Error: "stdout read error: " + err.Error(),
				Code:  model.WSErrCodeRuntimeError,
			})
			cancel()
		}
	}()

	// 9. Write pump — stderr scanner (pipe mode only; PTY merges stderr into ptmx).
	// Buffer sized to 16 MiB to match stdout pump.
	if !session.IsPTY() {
		go func() {
			stderr := session.StderrPipe()
			if stderr == nil {
				return
			}
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					return
				default:
				}
				line := scanner.Text() + "\n"
				// Write to replay buffer so reconnecting clients can catch up.
				codeRunner.WriteSessionOutput(sessionID, []byte(line))
				if writeErr := writeJSON(model.ServerFrame{
					Type:      "stderr",
					Data:      line,
					Timestamp: time.Now().UnixMilli(),
				}); writeErr != nil {
					return
				}
			}
			if err := scanner.Err(); err != nil && ctx.Err() == nil {
				_ = writeJSON(model.ServerFrame{
					Type:  "error",
					Error: "stderr read error: " + err.Error(),
					Code:  model.WSErrCodeRuntimeError,
				})
				cancel()
			}
		}()
	}

	// 10. Exit watcher — sends exit frame when bash process ends.
	go func() {
		defer cancel()
		doneCh := session.Done()
		if doneCh == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-doneCh:
		}
		exitCode := session.ExitCode()
		_ = writeJSON(model.ServerFrame{Type: "exit", ExitCode: &exitCode})
	}()

	// 11. Read pump — client → bash stdin.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck
	for {
		var frame model.ClientFrame
		if readErr := conn.ReadJSON(&frame); readErr != nil {
			if ctx.Err() == nil {
				cancel()
			}
			break
		}
		conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck

		switch frame.Type {
		case "stdin":
			if _, writeErr := session.WriteStdin([]byte(frame.Data)); writeErr != nil {
				_ = writeJSON(model.ServerFrame{
					Type:  "error",
					Error: writeErr.Error(),
					Code:  model.WSErrCodeStdinWriteFailed,
				})
				cancel()
				return
			}
		case "signal":
			session.SendSignal(frame.Signal)
		case "resize":
			if session.IsPTY() {
				_ = session.ResizePTY(uint16(frame.Cols), uint16(frame.Rows))
			}
			// Silently ignored in pipe mode; accepted to avoid client errors.
		case "ping":
			_ = writeJSON(model.ServerFrame{Type: "pong"})
		default:
			_ = writeJSON(model.ServerFrame{
				Type:  "error",
				Error: "unknown frame type",
				Code:  model.WSErrCodeInvalidFrame,
			})
		}
	}
}
