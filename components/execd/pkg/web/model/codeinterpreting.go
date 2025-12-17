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

package model

import (
	"encoding/json"

	"github.com/go-playground/validator/v10"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
)

// RunCodeRequest represents a code execution request.
type RunCodeRequest struct {
	Context CodeContext `json:"context,omitempty"`
	Code    string      `json:"code" validate:"required"`
}

func (r *RunCodeRequest) Validate() error {
	validate := validator.New()
	return validate.Struct(r)
}

// CodeContext tracks session metadata.
type CodeContext struct {
	ID                 string `json:"id,omitempty"`
	CodeContextRequest `json:",inline"`
}

type CodeContextRequest struct {
	Language string `json:"language,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
}

// RunCommandRequest represents a shell command execution request.
type RunCommandRequest struct {
	Command    string `json:"command" validate:"required"`
	Cwd        string `json:"cwd,omitempty"`
	Background bool   `json:"background,omitempty"`
}

func (r *RunCommandRequest) Validate() error {
	validate := validator.New()
	return validate.Struct(r)
}

type ServerStreamEventType string

const (
	StreamEventTypeInit     ServerStreamEventType = "init"
	StreamEventTypeStatus   ServerStreamEventType = "status"
	StreamEventTypeError    ServerStreamEventType = "error"
	StreamEventTypeStdout   ServerStreamEventType = "stdout"
	StreamEventTypeStderr   ServerStreamEventType = "stderr"
	StreamEventTypeResult   ServerStreamEventType = "result"
	StreamEventTypeComplete ServerStreamEventType = "execution_complete"
	StreamEventTypeCount    ServerStreamEventType = "execution_count"
	StreamEventTypePing     ServerStreamEventType = "ping"
)

// ServerStreamEvent is emitted to clients over SSE.
type ServerStreamEvent struct {
	Type           ServerStreamEventType `json:"type,omitempty"`
	Text           string                `json:"text,omitempty"`
	ExecutionCount int                   `json:"execution_count,omitempty"`
	ExecutionTime  int64                 `json:"execution_time,omitempty"`
	Timestamp      int64                 `json:"timestamp,omitempty"`
	Results        map[string]any        `json:"results,omitempty"`
	Error          *execute.ErrorOutput  `json:"error,omitempty"`
}

// ToJSON serializes the event for streaming.
func (s ServerStreamEvent) ToJSON() []byte {
	bytes, _ := json.Marshal(s)
	return bytes
}
