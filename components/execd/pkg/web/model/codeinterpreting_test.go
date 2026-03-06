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
	"strings"
	"testing"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
)

func TestRunCodeRequestValidate(t *testing.T) {
	req := RunCodeRequest{
		Code: "print('hi')",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected validation success: %v", err)
	}

	req.Code = ""
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when code is empty")
	}
}

func TestRunCommandRequestValidate(t *testing.T) {
	req := RunCommandRequest{Command: "ls"}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected command validation success: %v", err)
	}

	req.TimeoutMs = -100
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when timeout is negative")
	}

	req.TimeoutMs = 0
	req.Command = "ls"
	if err := req.Validate(); err != nil {
		t.Fatalf("expected success when timeout is omitted/zero: %v", err)
	}

	req.TimeoutMs = 10
	req.Command = ""
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when command is empty")
	}

	req.Command = "ls"
	req.Uid = int64Ptr(-1)
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when uid is negative")
	}

	req.Uid = int64Ptr(42)
	req.Gid = int64Ptr(4_294_967_296)
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when gid exceeds uint32 range")
	}

	req.Gid = int64Ptr(0)
	if err := req.Validate(); err != nil {
		t.Fatalf("expected success with explicit uid/gid: %v", err)
	}
}

func TestServerStreamEventToJSON(t *testing.T) {
	event := ServerStreamEvent{
		Type:           StreamEventTypeStdout,
		Text:           "hello",
		ExecutionCount: 3,
	}

	data := event.ToJSON()
	var decoded ServerStreamEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}
	if decoded.Type != event.Type || decoded.Text != event.Text || decoded.ExecutionCount != event.ExecutionCount {
		t.Fatalf("unexpected decoded event: %#v", decoded)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

func TestServerStreamEventSummary(t *testing.T) {
	longText := strings.Repeat("a", 120)
	tests := []struct {
		name     string
		event    ServerStreamEvent
		contains []string
	}{
		{
			name: "basic stdout",
			event: ServerStreamEvent{
				Type:           StreamEventTypeStdout,
				Text:           "hello",
				ExecutionCount: 2,
			},
			contains: []string{"type=stdout", "text=hello"},
		},
		{
			name: "truncated text and error",
			event: ServerStreamEvent{
				Type:  StreamEventTypeError,
				Text:  longText,
				Error: &execute.ErrorOutput{EName: "ValueError", EValue: "boom"},
			},
			contains: []string{
				"type=error",
				"text=" + strings.Repeat("a", 100) + "...",
				"error=ValueError: boom",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.event.Summary()
			for _, want := range tt.contains {
				if !strings.Contains(summary, want) {
					t.Fatalf("summary missing %q, got: %s", want, summary)
				}
			}
		})
	}
}
