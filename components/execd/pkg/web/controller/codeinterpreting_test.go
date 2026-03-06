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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

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

func TestGetContext_NotFoundReturns404(t *testing.T) {
	ctx, w := newTestContext(http.MethodGet, "/code/contexts/missing", nil)
	ctx.Params = append(ctx.Params, gin.Param{Key: "contextId", Value: "missing"})
	ctrl := NewCodeInterpretingController(ctx)

	previous := codeRunner
	codeRunner = runtime.NewController("", "")
	t.Cleanup(func() { codeRunner = previous })

	ctrl.GetContext()

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Code != model.ErrorCodeContextNotFound {
		t.Fatalf("unexpected error code: %s", resp.Code)
	}
	if resp.Message != "context missing not found" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
}

func TestGetContext_MissingIDReturns400(t *testing.T) {
	ctx, w := newTestContext(http.MethodGet, "/code/contexts/", nil)
	ctrl := NewCodeInterpretingController(ctx)

	ctrl.GetContext()

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Code != model.ErrorCodeMissingQuery {
		t.Fatalf("unexpected error code: %s", resp.Code)
	}
	if resp.Message != "missing path parameter 'contextId'" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
}
