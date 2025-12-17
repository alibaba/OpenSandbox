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
	"testing"
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

	req.Command = ""
	if err := req.Validate(); err == nil {
		t.Fatalf("expected validation error when command is empty")
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
