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

//go:build !windows
// +build !windows

package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

// wsTestServer spins up a real httptest.Server with the WS route wired in.
func wsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws/session/:sessionId", func(ctx *gin.Context) {
		NewCodeInterpretingController(ctx).SessionWebSocket()
	})
	return httptest.NewServer(r)
}

// wsURL converts an http:// test-server URL to a ws:// URL.
func wsURL(srv *httptest.Server, sessionID string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/session/" + sessionID
}

// wsURLWithSince appends ?since=<offset> to the WS URL.
func wsURLWithSince(srv *httptest.Server, sessionID string, since int64) string {
	return wsURL(srv, sessionID) + "?since=" + strconv.FormatInt(since, 10)
}

// dialWS opens a WebSocket connection to the test server.
func dialWS(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	dialer := websocket.DefaultDialer
	conn, resp, err := dialer.Dial(url, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("WS dial failed: %v (HTTP %d)", err, resp.StatusCode)
		}
		t.Fatalf("WS dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readFrame reads one JSON ServerFrame from the WebSocket connection.
func readFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) model.ServerFrame {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	var frame model.ServerFrame
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "reading WS frame")
	require.NoError(t, json.Unmarshal(msg, &frame), "unmarshalling WS frame")
	return frame
}

// withFreshRunner swaps codeRunner for a clean controller and restores on cleanup.
func withFreshRunner(t *testing.T) {
	t.Helper()
	prev := codeRunner
	codeRunner = runtime.NewController("", "")
	t.Cleanup(func() { codeRunner = prev })
}

// createTestSession creates a bash session and returns its ID.
func createTestSession(t *testing.T) string {
	t.Helper()
	id, err := codeRunner.CreateBashSession(&runtime.CreateContextRequest{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = codeRunner.DeleteBashSession(id) })
	return id
}

// TestSessionWS_ConnectUnknownSession verifies that connecting to a non-existent
// session returns HTTP 404 before the WebSocket upgrade.
func TestSessionWS_ConnectUnknownSession(t *testing.T) {
	withFreshRunner(t)
	srv := wsTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ws/session/does-not-exist")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSessionWS_PingPong sends an application-level ping frame and expects a pong.
func TestSessionWS_PingPong(t *testing.T) {
	withFreshRunner(t)
	srv := wsTestServer(t)
	defer srv.Close()

	id := createTestSession(t)
	conn := dialWS(t, wsURL(srv, id))

	// Drain the "connected" frame.
	connected := readFrame(t, conn, 5*time.Second)
	require.Equal(t, "connected", connected.Type)

	// Send application ping.
	require.NoError(t, conn.WriteJSON(model.ClientFrame{Type: "ping"}))

	// Expect pong.
	frame := readFrame(t, conn, 5*time.Second)
	require.Equal(t, "pong", frame.Type)
}

// TestSessionWS_StdinForwarding connects to a session, sends a stdin frame
// with an echo command, and verifies that a stdout frame arrives.
func TestSessionWS_StdinForwarding(t *testing.T) {
	withFreshRunner(t)
	srv := wsTestServer(t)
	defer srv.Close()

	id := createTestSession(t)
	conn := dialWS(t, wsURL(srv, id))

	// Drain "connected".
	connected := readFrame(t, conn, 5*time.Second)
	require.Equal(t, "connected", connected.Type)

	// Send a command via stdin.
	require.NoError(t, conn.WriteJSON(model.ClientFrame{
		Type: "stdin",
		Data: "echo hello_ws\n",
	}))

	// Collect frames until we see the expected stdout or timeout.
	deadline := time.Now().Add(10 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var f model.ServerFrame
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if jsonErr := json.Unmarshal(msg, &f); jsonErr != nil {
			continue
		}
		if f.Type == "stdout" && strings.Contains(f.Data, "hello_ws") {
			found = true
			break
		}
	}
	require.True(t, found, "expected stdout frame with 'hello_ws'")
}

// TestSessionWS_ReplayOnConnect connects with ?since=0 and verifies that
// a replay frame arrives before live output when there is buffered data.
func TestSessionWS_ReplayOnConnect(t *testing.T) {
	withFreshRunner(t)
	srv := wsTestServer(t)
	defer srv.Close()

	id := createTestSession(t)

	// Prime the replay buffer by running a command through the HTTP API
	// (RunInSession SSE endpoint), then reconnect via WS with ?since=0.
	// Simpler: connect once, write stdin, disconnect, reconnect with since=0.
	conn1 := dialWS(t, wsURL(srv, id))
	connected := readFrame(t, conn1, 5*time.Second)
	require.Equal(t, "connected", connected.Type)

	// Write to stdin and wait briefly for stdout to land in the replay buffer.
	require.NoError(t, conn1.WriteJSON(model.ClientFrame{
		Type: "stdin",
		Data: "echo replay_test\n",
	}))
	// Wait for stdout to arrive so the replay buffer is populated.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn1.SetReadDeadline(time.Now().Add(5 * time.Second))
		var f model.ServerFrame
		_, msg, err := conn1.ReadMessage()
		if err != nil {
			break
		}
		if jsonErr := json.Unmarshal(msg, &f); jsonErr != nil {
			continue
		}
		if f.Type == "stdout" && strings.Contains(f.Data, "replay_test") {
			break
		}
	}

	// Close first connection to release the WS lock.
	conn1.Close()
	// Give the server a moment to release the lock.
	time.Sleep(100 * time.Millisecond)

	// Reconnect with ?since=0 — should receive a replay frame.
	conn2 := dialWS(t, wsURLWithSince(srv, id, 0))
	defer conn2.Close()

	// We expect a replay frame before the connected frame (replay is sent first).
	deadline = time.Now().Add(10 * time.Second)
	foundReplay := false
	for time.Now().Before(deadline) {
		conn2.SetReadDeadline(time.Now().Add(5 * time.Second))
		var f model.ServerFrame
		_, msg, err := conn2.ReadMessage()
		if err != nil {
			break
		}
		if jsonErr := json.Unmarshal(msg, &f); jsonErr != nil {
			continue
		}
		if f.Type == "replay" {
			require.Contains(t, f.Data, "replay_test", "replay frame should contain buffered output")
			foundReplay = true
			break
		}
	}
	require.True(t, foundReplay, "expected replay frame with buffered output")
}

// TestSessionWS_ExitFrame runs a short-lived command and verifies that
// an exit frame is received with code 0 after bash exits.
func TestSessionWS_ExitFrame(t *testing.T) {
	withFreshRunner(t)
	srv := wsTestServer(t)
	defer srv.Close()

	id := createTestSession(t)
	conn := dialWS(t, wsURL(srv, id))

	connected := readFrame(t, conn, 5*time.Second)
	require.Equal(t, "connected", connected.Type)

	// Ask bash to exit cleanly.
	require.NoError(t, conn.WriteJSON(model.ClientFrame{
		Type: "stdin",
		Data: "exit 0\n",
	}))

	// Collect frames looking for the exit frame.
	deadline := time.Now().Add(10 * time.Second)
	foundExit := false
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var f model.ServerFrame
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if jsonErr := json.Unmarshal(msg, &f); jsonErr != nil {
			continue
		}
		if f.Type == "exit" {
			require.NotNil(t, f.ExitCode, "exit frame must include exit_code")
			require.Equal(t, 0, *f.ExitCode)
			foundExit = true
			break
		}
	}
	require.True(t, foundExit, "expected exit frame with code 0")
}
