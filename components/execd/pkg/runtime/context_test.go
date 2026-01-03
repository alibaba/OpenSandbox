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

package runtime

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListContextsAndNewIpynbPath(t *testing.T) {
	c := NewController("http://example", "token")
	c.jupyterClientMap["session-python"] = &jupyterKernel{language: Python}
	c.defaultLanguageJupyterSessions[Go] = "session-go-default"

	pyContexts, err := c.listLanguageContexts(Python)
	if err != nil {
		t.Fatalf("listLanguageContexts returned error: %v", err)
	}
	if len(pyContexts) != 1 || pyContexts[0].ID != "session-python" || pyContexts[0].Language != Python {
		t.Fatalf("unexpected python contexts: %#v", pyContexts)
	}

	allContexts, err := c.listAllContexts()
	if err != nil {
		t.Fatalf("listAllContexts returned error: %v", err)
	}
	if len(allContexts) != 2 {
		t.Fatalf("expected two contexts, got %d", len(allContexts))
	}

	tmpDir := filepath.Join(t.TempDir(), "nested")
	path, err := c.newIpynbPath("abc123", tmpDir)
	if err != nil {
		t.Fatalf("newIpynbPath error: %v", err)
	}
	if _, statErr := os.Stat(tmpDir); statErr != nil {
		t.Fatalf("expected directory to be created: %v", statErr)
	}
	expected := filepath.Join(tmpDir, "abc123.ipynb")
	if path != expected {
		t.Fatalf("unexpected ipynb path: got %s want %s", path, expected)
	}
}

func TestNewContextID_UniqueAndLength(t *testing.T) {
	c := NewController("", "")
	id1 := c.newContextID()
	id2 := c.newContextID()

	if id1 == "" || id2 == "" {
		t.Fatalf("expected non-empty ids")
	}
	if id1 == id2 {
		t.Fatalf("expected unique ids, got identical: %s", id1)
	}
	if len(id1) != 32 || len(id2) != 32 {
		t.Fatalf("expected 32-char ids, got %d and %d", len(id1), len(id2))
	}
}

func TestNewIpynbPath_ErrorWhenCwdIsFile(t *testing.T) {
	c := NewController("", "")
	tmpFile := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(tmpFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("prepare file: %v", err)
	}

	if _, err := c.newIpynbPath("abc", tmpFile); err == nil {
		t.Fatalf("expected error when cwd is a file")
	}
}

func TestListContextUnsupportedLanguage(t *testing.T) {
	c := NewController("", "")
	_, err := c.ListContext(Command.String())
	if err == nil {
		t.Fatalf("expected error for command language")
	}
	if _, err := c.ListContext(BackgroundCommand.String()); err == nil {
		t.Fatalf("expected error for background-command language")
	}
	if _, err := c.ListContext(SQL.String()); err == nil {
		t.Fatalf("expected error for sql language")
	}
}

func TestDeleteContext_NotFound(t *testing.T) {
	c := NewController("", "")
	err := c.DeleteContext("missing")
	if err == nil {
		t.Fatalf("expected ErrContextNotFound")
	}
	if !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteContext_RemovesCacheOnSuccess(t *testing.T) {
	sessionID := "sess-123"

	// mock jupyter server that accepts DELETE
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/sessions/"+sessionID) {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewController(server.URL, "token")
	c.jupyterClientMap[sessionID] = &jupyterKernel{language: Python}
	c.defaultLanguageJupyterSessions[Python] = sessionID

	if err := c.DeleteContext(sessionID); err != nil {
		t.Fatalf("DeleteContext returned error: %v", err)
	}

	if kernel := c.getJupyterKernel(sessionID); kernel != nil {
		t.Fatalf("expected cache to be cleared, found: %+v", kernel)
	}
	if _, ok := c.defaultLanguageJupyterSessions[Python]; ok {
		t.Fatalf("expected default session entry to be removed")
	}
}

func TestDeleteLanguageContext_RemovesCacheOnSuccess(t *testing.T) {
	lang := Python
	session1 := "sess-1"
	session2 := "sess-2"

	// mock jupyter server to accept two deletes
	deleteCalls := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if strings.Contains(r.URL.Path, session1) {
			deleteCalls[session1]++
		} else if strings.Contains(r.URL.Path, session2) {
			deleteCalls[session2]++
		} else {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewController(server.URL, "token")
	c.jupyterClientMap[session1] = &jupyterKernel{language: lang}
	c.jupyterClientMap[session2] = &jupyterKernel{language: lang}
	c.defaultLanguageJupyterSessions[lang] = session2

	if err := c.DeleteLanguageContext(lang); err != nil {
		t.Fatalf("DeleteLanguageContext returned error: %v", err)
	}

	if _, ok := c.jupyterClientMap[session1]; ok {
		t.Fatalf("expected session1 removed from cache")
	}
	if _, ok := c.jupyterClientMap[session2]; ok {
		t.Fatalf("expected session2 removed from cache")
	}
	if _, ok := c.defaultLanguageJupyterSessions[lang]; ok {
		t.Fatalf("expected default entry removed")
	}
	if deleteCalls[session1] != 1 || deleteCalls[session2] != 1 {
		t.Fatalf("unexpected delete calls: %+v", deleteCalls)
	}
}
