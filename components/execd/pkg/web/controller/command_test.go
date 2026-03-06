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
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

func setupCommandController(method, path string) (*CodeInterpretingController, *httptest.ResponseRecorder) {
	ctx, w := newTestContext(method, path, nil)
	ctrl := NewCodeInterpretingController(ctx)
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

func TestBuildExecuteCommandRequest_MapsUidGidAndTimeout(t *testing.T) {
	ctrl := &CodeInterpretingController{}
	uid := int64(1001)
	gid := int64(1002)

	execReq := ctrl.buildExecuteCommandRequest(model.RunCommandRequest{
		Command:    "id",
		Cwd:        "/tmp",
		TimeoutMs:  2500,
		Uid:        &uid,
		Gid:        &gid,
		Background: true,
	})

	if execReq.Language != runtime.BackgroundCommand {
		t.Fatalf("expected background command language, got %s", execReq.Language)
	}
	if execReq.Timeout != 2500*time.Millisecond {
		t.Fatalf("unexpected timeout: %s", execReq.Timeout)
	}
	if execReq.Uid == nil || *execReq.Uid != 1001 {
		t.Fatalf("unexpected uid: %#v", execReq.Uid)
	}
	if execReq.Gid == nil || *execReq.Gid != 1002 {
		t.Fatalf("unexpected gid: %#v", execReq.Gid)
	}
}
