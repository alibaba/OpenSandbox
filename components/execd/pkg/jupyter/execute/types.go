// Copyright 2025 Alibaba Group Holding Ltd.
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

// Package execute provides functionality for executing Jupyter kernel code via WebSocket
package execute

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MessageType represents Jupyter message types
type MessageType string

const (
	// MsgExecuteRequest requests code execution
	MsgExecuteRequest MessageType = "execute_request"

	// MsgExecuteInput represents the input code
	MsgExecuteInput MessageType = "execute_input"

	// MsgExecuteResult represents execution results
	MsgExecuteResult MessageType = "execute_result"

	// MsgDisplayData represents data to be displayed
	MsgDisplayData MessageType = "display_data"

	// MsgStream represents stream output (stdout/stderr)
	MsgStream MessageType = "stream"

	// MsgError represents errors during execution
	MsgError MessageType = "error"

	// MsgStatus represents kernel status updates
	MsgStatus MessageType = "status"

	// MsgClearOutput represents clearing output
	MsgClearOutput MessageType = "clear_output"

	// MsgComm represents communication messages
	MsgComm MessageType = "comm"

	// MsgCommOpen represents opening communication
	MsgCommOpen MessageType = "comm_open"

	// MsgCommClose represents closing communication
	MsgCommClose MessageType = "comm_close"

	// MsgCommMsg representscommunication message content
	MsgCommMsg MessageType = "comm_msg"

	// MsgKernelInfo represents kernel information request
	MsgKernelInfo MessageType = "kernel_info_request"

	// MsgKernelInfoReply represents kernel information response
	MsgKernelInfoReply MessageType = "kernel_info_reply"

	MsgExecuteReply MessageType = "execute_reply"
)

// StreamType representsoutput stream type
type StreamType string

const (
	// StreamStdout represents standard output stream
	StreamStdout StreamType = "stdout"

	// StreamStderr representsstandard error stream
	StreamStderr StreamType = "stderr"
)

// ExecutionState represents kernel execution state
type ExecutionState string

const (
	// StateIdle representskernel is idle
	StateIdle ExecutionState = "idle"

	// StateBusy representskernel is busy
	StateBusy ExecutionState = "busy"

	// StateStarting representskernel is starting
	StateStarting ExecutionState = "starting"
)

// Header defines Jupyter message header
type Header struct {
	// MessageID is the unique identifier of the message
	MessageID string `json:"msg_id"`

	// Username is the username sending the message
	Username string `json:"username"`

	// Session is the session identifier
	Session string `json:"session"`

	// Date is the timestamp when the message was sent
	Date string `json:"date"`

	// MessageType is the type of the message
	MessageType string `json:"msg_type"`

	// Version is the version of the message protocol
	Version string `json:"version"`
}

// Message defines the basic structure of Jupyter messages
type Message struct {
	// Header is the message header
	Header Header `json:"header"`

	// ParentHeader is the parent message header, used to track requests and responses
	ParentHeader Header `json:"parent_header"`

	// Metadata is the metadata related to the message
	Metadata map[string]interface{} `json:"metadata"`

	// Content is the actual content of the message
	Content json.RawMessage `json:"content"`

	// Buffers is the binary buffer
	Buffers [][]byte `json:"buffers"`

	// Channel is the channel of the message
	Channel string `json:"channel"`
}

// ExecuteRequest defines the request content for code execution
type ExecuteRequest struct {
	// Code is the code to execute
	Code string `json:"code"`

	// Silent represents whether to execute in silent mode
	Silent bool `json:"silent"`

	// StoreHistory represents whether to store execution history
	StoreHistory bool `json:"store_history"`

	// UserExpressions contains expressions to evaluate in the execution context
	UserExpressions map[string]string `json:"user_expressions"`

	// AllowStdin represents whether to allow reading from standard input
	AllowStdin bool `json:"allow_stdin"`

	// StopOnError represents whether to stop execution when an error is encountered
	StopOnError bool `json:"stop_on_error"`
}

// StreamOutput represents stream output content
type StreamOutput struct {
	// Name is the stream name (stdout or stderr)
	Name StreamType `json:"name"`

	// Text is the text content of the stream
	Text string `json:"text"`
}

// ExecuteResult represents the result of code execution
type ExecuteResult struct {
	// ExecutionCount is the execution counter value
	ExecutionCount int `json:"execution_count"`

	// Data contains result data in different formats
	Data map[string]interface{} `json:"data"`

	// Metadata is the metadata related to the result
	Metadata map[string]interface{} `json:"metadata"`
}

type ExecuteReply struct {
	// ExecutionCount is the execution counter value
	ExecutionCount int `json:"execution_count"`

	Status string `json:"status"`

	ErrorOutput `json:",inline"`
}

// DisplayData representsdata to display
type DisplayData struct {
	// Data contains display data in different formats
	Data map[string]interface{} `json:"data"`

	// Metadata is the metadata related to display data
	Metadata map[string]interface{} `json:"metadata"`
}

// ErrorOutput representserrors during execution
type ErrorOutput struct {
	// EName is the name of the error
	EName string `json:"ename"`

	// EValue is the value of the error
	EValue string `json:"evalue"`

	// Traceback is the traceback of the error
	Traceback []string `json:"traceback"`
}

func (e *ErrorOutput) String() string {
	return fmt.Sprintf(`
Error: %s
Value: %s
Traceback: %s
`, e.EName, e.EValue, strings.Join(e.Traceback, "\n"))
}

// StatusUpdate represents kernel status update
type StatusUpdate struct {
	// ExecutionState is the execution state of the kernel
	ExecutionState ExecutionState `json:"execution_state"`
}

// ExecutionResult represents the complete result of code execution
type ExecutionResult struct {
	// Status represents the status of execution
	Status string `json:"status"`

	// ExecutionCount is the execution counter value
	ExecutionCount int `json:"execution_count"`

	// Stream contains all stream output
	Stream []*StreamOutput `json:"stream"`

	// Error contains errors during execution (if any)
	Error *ErrorOutput `json:"error"`

	// ExecutionTime is the total time of code execution
	ExecutionTime time.Duration `json:"execution_time"`

	// ExecutionData
	ExecutionData map[string]interface{} `json:"execution_data"`
}

// CallbackHandler defines callback functions for handling different types of messages
type CallbackHandler struct {
	// OnExecuteResult handles execution result messages
	OnExecuteResult func(*ExecuteResult)

	// OnStream handles stream output messages
	OnStream func(...*StreamOutput)

	// OnDisplayData handles display data messages
	OnDisplayData func(*DisplayData)

	// OnError handles error messages
	OnError func(*ErrorOutput)

	// OnStatus handles status update messages
	OnStatus func(*StatusUpdate)
}
