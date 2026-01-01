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
	"net/http/httptest"
	"testing"

	"github.com/beego/beego/v2/server/web/context"

	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

func setupCommandController(method, path string) (*CodeInterpretingController, *httptest.ResponseRecorder) {
	ctrl := &CodeInterpretingController{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	ctx := context.NewContext()
	ctx.Reset(w, req)
	ctrl.Init(ctx, "CodeInterpretingController", method, nil)
	ctrl.Data = make(map[interface{}]interface{})
	return ctrl, w
}

func TestGetCommandStatus_MissingID(t *testing.T) {
	ctrl, w := setupCommandController(http.MethodGet, "/command/status/")

	ctrl.GetCommandStatus()

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != model.ErrorCodeInvalidRequest {
		t.Fatalf("unexpected error code: %s", resp.Code)
	}
	if resp.Message != "missing command execution id" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
}

func TestGetBackgroundCommandOutput_MissingID(t *testing.T) {
	ctrl, w := setupCommandController(http.MethodGet, "/command/logs/")

	ctrl.GetBackgroundCommandOutput()

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != model.ErrorCodeMissingQuery {
		t.Fatalf("unexpected error code: %s", resp.Code)
	}
	if resp.Message != "missing command execution id" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
}
