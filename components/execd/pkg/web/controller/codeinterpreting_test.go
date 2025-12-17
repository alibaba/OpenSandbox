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

package controller

import (
	"testing"

	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

func TestBuildExecuteCodeRequestDefaultsToCommand(t *testing.T) {
	ctrl := &CodeInterpretingController{}
	req := model.RunCodeRequest{
		Code: "echo 1",
		Context: model.CodeContext{
			ID:                 "session-1",
			CodeContextRequest: model.CodeContextRequest{},
		},
	}

	execReq := ctrl.buildExecuteCodeRequest(req)

	if execReq.Language != runtime.Command {
		t.Fatalf("expected default language %s, got %s", runtime.Command, execReq.Language)
	}
	if execReq.Context != "session-1" || execReq.Code != "echo 1" {
		t.Fatalf("unexpected execute request: %#v", execReq)
	}
}

func TestBuildExecuteCodeRequestRespectsLanguage(t *testing.T) {
	ctrl := &CodeInterpretingController{}
	req := model.RunCodeRequest{
		Code: "print(1)",
		Context: model.CodeContext{
			ID: "session-2",
			CodeContextRequest: model.CodeContextRequest{
				Language: "python",
			},
		},
	}

	execReq := ctrl.buildExecuteCodeRequest(req)

	if execReq.Language != runtime.Language("python") {
		t.Fatalf("expected python language, got %s", execReq.Language)
	}
}
