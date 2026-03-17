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
	"context"
	"io"
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
	// Do NOT defer UnlockWS here — we release it manually after pump goroutines
	// finish, so a reconnecting client cannot start new scanners on the shared pipe
	// while stale scanners from the previous connection are still blocked in Scan().

	// 3. Upgrade HTTP → WebSocket.
	conn, err := wsUpgrader.Upgrade(c.ctx.Writer, c.ctx.Request, nil)
	if err != nil {
		// gorilla writes the HTTP error response automatically.
		session.UnlockWS()
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
			session.UnlockWS()
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 5. Snapshot the replay buffer offset THEN attach the live pipe — in that order.
	//
	// Why this order matters:
	//   - Snapshotting first captures a definite "replay up to here" watermark.
	//   - AttachOutput installs the PipeWriter so the broadcast goroutine begins
	//     queuing bytes into the pipe immediately.
	//   - Any byte produced between snapshot and attach lands in the pipe only
	//     (not in the replay frame), so each byte is delivered exactly once.
	//   - If we attached first and snapshotted second, bytes produced in that
	//     window would appear in both the replay frame and the live pipe (duplicate).
	var replayData []byte
	var replayNextOffset int64
	if sinceStr := c.ctx.Query("since"); sinceStr != "" {
		if since, parseErr := strconv.ParseInt(sinceStr, 10, 64); parseErr == nil {
			replayData, replayNextOffset, _ = codeRunner.ReplaySessionOutput(sessionID, since)
		}
	}

	stdout, stderr, detach := session.AttachOutput()
	var pumpWg sync.WaitGroup
	defer func() {
		cancel()
		detach()
		pumpWg.Wait()
		session.UnlockWS()
	}()

	// 6. Send replay frame now that the live sink is attached — no gap, no duplicates.
	if len(replayData) > 0 {
		_ = writeJSON(model.ServerFrame{
			Type:   "replay",
			Data:   string(replayData),
			Offset: replayNextOffset,
		})
	}

	// 7. Send connected frame — mode derived from actual session state, not the request parameter,
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

	// 8. Ping/pong keepalive — RFC 6455 control-level pings every 30s.
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

	// streamPump reads raw byte chunks from r and forwards them as WS frames of the given type.
	// Raw reads (rather than line scanning) are required so that PTY prompts, progress spinners,
	// and cursor-control sequences — which are often written without a trailing newline — are
	// delivered immediately without buffering.
	streamPump := func(r io.Reader, frameType string) {
		defer pumpWg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				if ctx.Err() != nil {
					return
				}
				if writeErr := writeJSON(model.ServerFrame{
					Type:      frameType,
					Data:      string(buf[:n]),
					Timestamp: time.Now().UnixMilli(),
				}); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				if readErr != io.EOF && ctx.Err() == nil {
					_ = writeJSON(model.ServerFrame{
						Type:  "error",
						Error: frameType + " read error: " + readErr.Error(),
						Code:  model.WSErrCodeRuntimeError,
					})
					cancel()
				}
				return
			}
		}
	}

	// 8. Write pump — stdout (raw byte chunks).
	pumpWg.Add(1)
	go streamPump(stdout, "stdout")

	// 9. Write pump — stderr (pipe mode only; PTY merges stderr into ptmx; nil in PTY mode).
	if stderr != nil {
		pumpWg.Add(1)
		go streamPump(stderr, "stderr")
	}

	// 10. Exit watcher — sends exit frame when bash process ends, then closes the
	// connection so the read loop's ReadJSON unblocks immediately rather than waiting
	// up to 60s for the deadline. Without this, reconnect attempts during that window
	// hit "session already connected" even though the process is already gone.
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
		// Close with a normal closure code so the read loop gets an error immediately.
		writeMu.Lock()
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "process exited"))
		writeMu.Unlock()
		conn.Close()
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
				if resizeErr := session.ResizePTY(uint16(frame.Cols), uint16(frame.Rows)); resizeErr != nil {
					_ = writeJSON(model.ServerFrame{
						Type:  "error",
						Error: "resize failed: " + resizeErr.Error(),
						Code:  model.WSErrCodeRuntimeError,
					})
				}
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
