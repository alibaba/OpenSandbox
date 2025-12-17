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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/beego/beego/v2/server/web/context"

	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

func newFilesystemController(t *testing.T, method, rawURL string, body []byte) (*FilesystemController, *httptest.ResponseRecorder) {
	t.Helper()
	ctrl := &FilesystemController{}
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(method, rawURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	ctx := context.NewContext()
	ctx.Reset(rec, req)
	if len(body) > 0 {
		ctx.Input.RequestBody = body
	}
	ctrl.Init(ctx, "FilesystemController", "", nil)
	ctrl.Data = make(map[interface{}]interface{})
	return ctrl, rec
}

func TestFilesystemControllerGetFilesInfo(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "foo.txt")
	if err := os.WriteFile(target, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	query := fmt.Sprintf("/files/info?path=%s", url.QueryEscape(target))
	ctrl, rec := newFilesystemController(t, http.MethodGet, query, nil)

	ctrl.GetFilesInfo()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var resp map[string]model.FileInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	info, ok := resp[target]
	if !ok {
		t.Fatalf("response missing entry for %s", target)
	}
	if info.Path == "" || info.Size == 0 {
		t.Fatalf("unexpected file info: %#v", info)
	}
}

func TestFilesystemControllerSearchFiles(t *testing.T) {
	tmpDir := t.TempDir()
	a := filepath.Join(tmpDir, "alpha.txt")
	b := filepath.Join(tmpDir, "beta.log")
	if err := os.WriteFile(a, []byte("alpha"), 0o644); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := os.WriteFile(b, []byte("beta"), 0o644); err != nil {
		t.Fatalf("write beta: %v", err)
	}

	rawURL := fmt.Sprintf("/files/search?path=%s&pattern=%s", url.QueryEscape(tmpDir), url.QueryEscape("*.txt"))
	ctrl, rec := newFilesystemController(t, http.MethodGet, rawURL, nil)

	ctrl.SearchFiles()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var files []model.FileInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &files); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(files) != 1 || files[0].Path != a {
		t.Fatalf("expected only %s, got %#v", a, files)
	}
}

func TestFilesystemControllerReplaceContent(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "content.txt")
	if err := os.WriteFile(target, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	body, err := json.Marshal(map[string]model.ReplaceFileContentItem{
		target: {
			Old: "world",
			New: "universe",
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	ctrl, rec := newFilesystemController(t, http.MethodPost, "/files/replace", body)

	ctrl.ReplaceContent()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello universe" {
		t.Fatalf("unexpected content: %s", string(data))
	}
}

func TestFilesystemControllerSearchFilesHandlesAbsentDir(t *testing.T) {
	rawURL := "/files/search?path=/not/exists"
	ctrl, rec := newFilesystemController(t, http.MethodGet, rawURL, nil)

	ctrl.SearchFiles()

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestReplaceContentFailsUnknownFile(t *testing.T) {
	payload, _ := json.Marshal(map[string]model.ReplaceFileContentItem{
		filepath.Join(t.TempDir(), "missing.txt"): {
			Old: "old",
			New: "new",
		},
	})
	ctrl, rec := newFilesystemController(t, http.MethodPost, "/files/replace", payload)

	ctrl.ReplaceContent()

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected failure status, got %d", rec.Code)
	}
}
