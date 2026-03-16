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

package model

// ClientFrame is a JSON frame sent from the WebSocket client to the server.
type ClientFrame struct {
	Type   string `json:"type"`
	Data   string `json:"data,omitempty"`   // stdin payload (plain text)
	Cols   int    `json:"cols,omitempty"`   // resize — PTY mode only
	Rows   int    `json:"rows,omitempty"`   // resize — PTY mode only
	Signal string `json:"signal,omitempty"` // signal name, e.g. "SIGINT"
}

// ServerFrame is a JSON frame sent from the server to the WebSocket client.
type ServerFrame struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"` // connected
	Mode      string `json:"mode,omitempty"`        // connected: "pipe" | "pty"
	Data      string `json:"data,omitempty"`        // stdout/stderr/replay payload
	Offset    int64  `json:"offset,omitempty"`      // replay: next byte offset
	ExitCode  *int   `json:"exit_code,omitempty"`   // exit — pointer so 0 is marshalled
	Error     string `json:"error,omitempty"`       // error description
	Code      string `json:"code,omitempty"`        // machine-readable error code
	Timestamp int64  `json:"timestamp,omitempty"`
}

// WebSocket error code constants.
const (
	WSErrCodeSessionGone      = "SESSION_GONE"
	WSErrCodeStartFailed      = "START_FAILED"
	WSErrCodeStdinWriteFailed = "STDIN_WRITE_FAILED"
	WSErrCodeInvalidFrame     = "INVALID_FRAME"
	WSErrCodeAlreadyConnected = "ALREADY_CONNECTED"
	WSErrCodeRuntimeError     = "RUNTIME_ERROR"
)
