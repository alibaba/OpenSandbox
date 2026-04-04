package opensandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newLifecycleServer creates an httptest.Server and a LifecycleClient pointing at it.
func newLifecycleServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *LifecycleClient) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := NewLifecycleClient(srv.URL, "test-api-key")
	return srv, client
}

// newEgressServer creates an httptest.Server and an EgressClient pointing at it.
func newEgressServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *EgressClient) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := NewEgressClient(srv.URL, "test-egress-token")
	return srv, client
}

// newExecdServer creates an httptest.Server and an ExecdClient pointing at it.
func newExecdServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *ExecdClient) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := NewExecdClient(srv.URL, "test-execd-token")
	return srv, client
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------------------
// Lifecycle tests
// ---------------------------------------------------------------------------

func TestCreateSandbox(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	want := SandboxInfo{
		ID: "sbx-123",
		Status: SandboxStatus{
			State: StatePending,
		},
		CreatedAt: now,
	}

	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes" {
			t.Errorf("expected /sandboxes, got %s", r.URL.Path)
		}

		var req CreateSandboxRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Image.URI != "python:3.12" {
			t.Errorf("expected image python:3.12, got %s", req.Image.URI)
		}

		jsonResponse(w, http.StatusCreated, want)
	})

	got, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{
		Image:      ImageSpec{URI: "python:3.12"},
		Entrypoint: []string{"/bin/sh"},
		ResourceLimits: ResourceLimits{
			"cpu":    "500m",
			"memory": "512Mi",
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Status.State != StatePending {
		t.Errorf("State = %q, want %q", got.Status.State, StatePending)
	}
}

func TestCreateSandbox_ImageAuth(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req CreateSandboxRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Image.Auth == nil {
			t.Fatal("expected ImageAuth to be set")
		}
		if req.Image.Auth.Username != "user" {
			t.Errorf("Username = %q, want %q", req.Image.Auth.Username, "user")
		}
		if req.Image.Auth.Password != "pass" {
			t.Errorf("Password = %q, want %q", req.Image.Auth.Password, "pass")
		}

		jsonResponse(w, http.StatusCreated, SandboxInfo{
			ID:        "sbx-auth",
			Status:    SandboxStatus{State: StatePending},
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		})
	})

	_, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{
		Image: ImageSpec{
			URI:  "registry.example.com/private:latest",
			Auth: &ImageAuth{Username: "user", Password: "pass"},
		},
		Entrypoint:     []string{"/bin/sh"},
		ResourceLimits: ResourceLimits{"cpu": "500m"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with ImageAuth: %v", err)
	}
}

func TestCreateSandbox_ManualCleanup(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var raw map[string]json.RawMessage
		json.Unmarshal(body, &raw)

		if _, exists := raw["timeout"]; exists {
			t.Error("expected timeout to be omitted from request when ManualCleanup is true")
		}

		jsonResponse(w, http.StatusCreated, SandboxInfo{
			ID:        "sbx-manual",
			Status:    SandboxStatus{State: StatePending},
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		})
	})

	_, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{
		Image:          ImageSpec{URI: "python:3.12"},
		Entrypoint:     []string{"/bin/sh"},
		ResourceLimits: ResourceLimits{"cpu": "500m"},
		// Timeout is nil — simulates ManualCleanup (no timeout sent)
	})
	if err != nil {
		t.Fatalf("CreateSandbox with ManualCleanup: %v", err)
	}
}

func TestGetSandbox(t *testing.T) {
	want := SandboxInfo{
		ID: "sbx-456",
		Status: SandboxStatus{
			State: StateRunning,
		},
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-456" {
			t.Errorf("expected /sandboxes/sbx-456, got %s", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.GetSandbox(context.Background(), "sbx-456")
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Status.State != StateRunning {
		t.Errorf("State = %q, want %q", got.Status.State, StateRunning)
	}
}

func TestListSandboxes(t *testing.T) {
	want := ListSandboxesResponse{
		Items: []SandboxInfo{
			{ID: "sbx-1", Status: SandboxStatus{State: StateRunning}, CreatedAt: time.Now().UTC().Truncate(time.Second)},
			{ID: "sbx-2", Status: SandboxStatus{State: StatePaused}, CreatedAt: time.Now().UTC().Truncate(time.Second)},
		},
		Pagination: PaginationInfo{
			Page:        1,
			PageSize:    20,
			TotalItems:  2,
			TotalPages:  1,
			HasNextPage: false,
		},
	}

	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/sandboxes") {
			t.Errorf("expected /sandboxes prefix, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") != "1" {
			t.Errorf("expected page=1, got %s", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("pageSize") != "20" {
			t.Errorf("expected pageSize=20, got %s", r.URL.Query().Get("pageSize"))
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.ListSandboxes(context.Background(), ListOptions{
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListSandboxes: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
	if got.Pagination.TotalItems != 2 {
		t.Errorf("TotalItems = %d, want 2", got.Pagination.TotalItems)
	}
}

func TestDeleteSandbox(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-789" {
			t.Errorf("expected /sandboxes/sbx-789, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteSandbox(context.Background(), "sbx-789")
	if err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}
}

func TestResumeSandbox(t *testing.T) {
	var resumed bool
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-paused/resume" {
			resumed = true
			w.WriteHeader(http.StatusAccepted)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})

	err := client.ResumeSandbox(context.Background(), "sbx-paused")
	if err != nil {
		t.Fatalf("ResumeSandbox: %v", err)
	}
	if !resumed {
		t.Error("expected resume endpoint to be called")
	}
}

func TestSandbox_Resume(t *testing.T) {
	var resumeCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resume"):
			resumeCalled = true
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/endpoints/"):
			jsonResponse(w, http.StatusOK, Endpoint{
				Endpoint: "http://execd.test:8080",
				Headers:  map[string]string{"X-EXECD-ACCESS-TOKEN": "tok"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	config := ConnectionConfig{Domain: srv.URL}
	sb := &Sandbox{
		id:     "sbx-resume-test",
		config: &config,
	}

	got, err := sb.Resume(context.Background())
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !resumeCalled {
		t.Error("expected resume endpoint to be called")
	}
	if got.ID() != "sbx-resume-test" {
		t.Errorf("ID = %q, want %q", got.ID(), "sbx-resume-test")
	}
}

func TestPauseSandbox(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-pause/pause" {
			t.Errorf("expected /sandboxes/sbx-pause/pause, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	})

	err := client.PauseSandbox(context.Background(), "sbx-pause")
	if err != nil {
		t.Fatalf("PauseSandbox: %v", err)
	}
}

func TestAPIError(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusNotFound, ErrorResponse{
			Code:    "SANDBOX_NOT_FOUND",
			Message: "sandbox sbx-missing does not exist",
		})
	})

	_, err := client.GetSandbox(context.Background(), "sbx-missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
	if apiErr.Response.Code != "SANDBOX_NOT_FOUND" {
		t.Errorf("Code = %q, want %q", apiErr.Response.Code, "SANDBOX_NOT_FOUND")
	}
	if !strings.Contains(apiErr.Error(), "SANDBOX_NOT_FOUND") {
		t.Errorf("Error() = %q, expected to contain SANDBOX_NOT_FOUND", apiErr.Error())
	}
}

// ---------------------------------------------------------------------------
// Egress tests
// ---------------------------------------------------------------------------

func TestGetPolicy(t *testing.T) {
	want := PolicyStatusResponse{
		Status:          "active",
		Mode:            "enforce",
		EnforcementMode: "strict",
		Policy: &NetworkPolicy{
			DefaultAction: "deny",
			Egress: []NetworkRule{
				{Action: "allow", Target: "api.example.com"},
			},
		},
	}

	_, client := newEgressServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/policy" {
			t.Errorf("expected /policy, got %s", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.GetPolicy(context.Background())
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
	if got.Policy == nil || len(got.Policy.Egress) != 1 {
		t.Fatal("expected 1 egress rule")
	}
	if got.Policy.Egress[0].Target != "api.example.com" {
		t.Errorf("Target = %q, want %q", got.Policy.Egress[0].Target, "api.example.com")
	}
}

func TestPatchPolicy(t *testing.T) {
	want := PolicyStatusResponse{
		Status: "active",
		Mode:   "enforce",
		Policy: &NetworkPolicy{
			DefaultAction: "deny",
			Egress: []NetworkRule{
				{Action: "allow", Target: "api.example.com"},
				{Action: "allow", Target: "cdn.example.com"},
			},
		},
	}

	_, client := newEgressServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}

		var rules []NetworkRule
		json.NewDecoder(r.Body).Decode(&rules)
		if len(rules) != 1 {
			t.Errorf("expected 1 rule in request, got %d", len(rules))
		}

		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.PatchPolicy(context.Background(), []NetworkRule{
		{Action: "allow", Target: "cdn.example.com"},
	})
	if err != nil {
		t.Fatalf("PatchPolicy: %v", err)
	}
	if got.Policy == nil || len(got.Policy.Egress) != 2 {
		t.Fatalf("expected 2 egress rules, got %v", got.Policy)
	}
}

// ---------------------------------------------------------------------------
// Execd tests
// ---------------------------------------------------------------------------

func TestPing(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/ping" {
			t.Errorf("expected /ping, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestRunCommand_SSE(t *testing.T) {
	ssePayload := "event: stdout\ndata: hello world\n\nevent: stderr\ndata: warning\n\nevent: result\ndata: {\"exit_code\": 0}\n\n"

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/command" {
			t.Errorf("expected /command, got %s", r.URL.Path)
		}

		var req RunCommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Command != "echo hello" {
			t.Errorf("Command = %q, want %q", req.Command, "echo hello")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	})

	var mu sync.Mutex
	var events []StreamEvent
	err := client.RunCommand(context.Background(), RunCommandRequest{
		Command: "echo hello",
	}, func(event StreamEvent) error {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Event != "stdout" || events[0].Data != "hello world" {
		t.Errorf("event[0] = %+v, want stdout/hello world", events[0])
	}
	if events[1].Event != "stderr" || events[1].Data != "warning" {
		t.Errorf("event[1] = %+v, want stderr/warning", events[1])
	}
	if events[2].Event != "result" {
		t.Errorf("event[2].Event = %q, want result", events[2].Event)
	}
}

func TestGetFileInfo(t *testing.T) {
	want := map[string]FileInfo{
		"/tmp/test.txt": {
			Path:       "/tmp/test.txt",
			Size:       1024,
			ModifiedAt: time.Now().UTC().Truncate(time.Second),
			CreatedAt:  time.Now().UTC().Truncate(time.Second),
			Owner:      "root",
			Group:      "root",
			Mode:       0644,
		},
	}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/files/info") {
			t.Errorf("expected /files/info, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("path") != "/tmp/test.txt" {
			t.Errorf("expected path=/tmp/test.txt, got %s", r.URL.Query().Get("path"))
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.GetFileInfo(context.Background(), "/tmp/test.txt")
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}
	info, ok := got["/tmp/test.txt"]
	if !ok {
		t.Fatal("expected /tmp/test.txt in result")
	}
	if info.Size != 1024 {
		t.Errorf("Size = %d, want 1024", info.Size)
	}
	if info.Owner != "root" {
		t.Errorf("Owner = %q, want root", info.Owner)
	}
}

func TestUploadFile(t *testing.T) {
	// Create a temp file to upload.
	tmpFile, err := os.CreateTemp("", "opensandbox-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("file contents here")
	tmpFile.Close()

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/files/upload" {
			t.Errorf("expected /files/upload, got %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart content type, got %s", r.Header.Get("Content-Type"))
		}

		// Verify metadata part exists.
		r.ParseMultipartForm(1 << 20)
		metaStr := r.FormValue("metadata")
		if metaStr == "" {
			t.Error("expected metadata form field")
		}
		var meta FileMetadata
		json.Unmarshal([]byte(metaStr), &meta)
		if meta.Path != "/sandbox/upload.txt" {
			t.Errorf("metadata path = %q, want /sandbox/upload.txt", meta.Path)
		}

		// Verify file part exists.
		file, _, fErr := r.FormFile("file")
		if fErr != nil {
			t.Errorf("expected file part: %v", fErr)
		} else {
			data, _ := io.ReadAll(file)
			if string(data) != "file contents here" {
				t.Errorf("file content = %q, want %q", string(data), "file contents here")
			}
			file.Close()
		}

		w.WriteHeader(http.StatusOK)
	})

	err = client.UploadFile(context.Background(), tmpFile.Name(), "/sandbox/upload.txt")
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
}

func TestGetMetrics(t *testing.T) {
	want := Metrics{
		CPUCount:   4,
		CPUUsedPct: 25.5,
		MemTotalMB: 8192,
		MemUsedMB:  4096,
		Timestamp:  time.Now().Unix(),
	}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/metrics" {
			t.Errorf("expected /metrics, got %s", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.GetMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if got.CPUCount != 4 {
		t.Errorf("CPUCount = %f, want 4", got.CPUCount)
	}
	if got.MemTotalMB != 8192 {
		t.Errorf("MemTotalMB = %f, want 8192", got.MemTotalMB)
	}
}

// ---------------------------------------------------------------------------
// SSE streaming test
// ---------------------------------------------------------------------------

func TestStreamSSE(t *testing.T) {
	ssePayload := strings.Join([]string{
		"event: start",
		"data: initializing",
		"",
		"event: progress",
		"data: step 1",
		"data: step 2",
		"",
		"id: evt-3",
		"event: done",
		"data: complete",
		"",
		": this is a comment",
		"event: final",
		"data: goodbye",
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	}))
	defer srv.Close()

	client := NewExecdClient(srv.URL, "tok")

	var events []StreamEvent
	err := client.RunCommand(context.Background(), RunCommandRequest{
		Command: "test",
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %+v", len(events), events)
	}

	// Event 1: start
	if events[0].Event != "start" || events[0].Data != "initializing" {
		t.Errorf("event[0] = %+v", events[0])
	}

	// Event 2: progress with multi-line data
	if events[1].Event != "progress" || events[1].Data != "step 1\nstep 2" {
		t.Errorf("event[1] = %+v, want progress/step 1\\nstep 2", events[1])
	}

	// Event 3: done with ID
	if events[2].Event != "done" || events[2].Data != "complete" || events[2].ID != "evt-3" {
		t.Errorf("event[2] = %+v", events[2])
	}

	// Event 4: final (comment should be skipped)
	if events[3].Event != "final" || events[3].Data != "goodbye" {
		t.Errorf("event[3] = %+v", events[3])
	}
}

// ---------------------------------------------------------------------------
// NDJSON streaming test
// ---------------------------------------------------------------------------

func TestStreamSSE_NDJSON(t *testing.T) {
	// Simulate the real execd server format: raw JSON blobs separated by blank lines.
	ndjsonPayload := "{\"type\":\"stdout\",\"data\":\"hello\"}\n\n{\"type\":\"result\",\"exit_code\":0}\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ndjsonPayload))
	}))
	defer srv.Close()

	client := NewExecdClient(srv.URL, "tok")

	var events []StreamEvent
	err := client.RunCommand(context.Background(), RunCommandRequest{
		Command: "test",
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}

	// NDJSON events with a "type" field should have Event populated.
	if events[0].Event != "stdout" {
		t.Errorf("event[0].Event = %q, want %q", events[0].Event, "stdout")
	}
	if events[0].Data != `{"type":"stdout","data":"hello"}` {
		t.Errorf("event[0].Data = %q", events[0].Data)
	}
	if events[1].Event != "result" {
		t.Errorf("event[1].Event = %q, want %q", events[1].Event, "result")
	}
	if events[1].Data != `{"type":"result","exit_code":0}` {
		t.Errorf("event[1].Data = %q", events[1].Data)
	}
}

// ---------------------------------------------------------------------------
// Auth header tests
// ---------------------------------------------------------------------------

func TestLifecycleAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("OPEN-SANDBOX-API-KEY")
		if got != "my-lifecycle-key" {
			t.Errorf("OPEN-SANDBOX-API-KEY = %q, want %q", got, "my-lifecycle-key")
		}
		jsonResponse(w, http.StatusOK, SandboxInfo{ID: "sbx-1", CreatedAt: time.Now()})
	}))
	defer srv.Close()

	client := NewLifecycleClient(srv.URL, "my-lifecycle-key")
	_, err := client.GetSandbox(context.Background(), "sbx-1")
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}
}

func TestExecdAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-EXECD-ACCESS-TOKEN")
		if got != "my-execd-token" {
			t.Errorf("X-EXECD-ACCESS-TOKEN = %q, want %q", got, "my-execd-token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewExecdClient(srv.URL, "my-execd-token")
	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Handler error propagation
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Phase 1: SandboxManager tests
// ---------------------------------------------------------------------------

func TestSandboxManager_ListFilter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	want := ListSandboxesResponse{
		Items: []SandboxInfo{
			{ID: "sbx-a", Status: SandboxStatus{State: StateRunning}, Metadata: map[string]string{"env": "prod"}, CreatedAt: now},
		},
		Pagination: PaginationInfo{Page: 1, PageSize: 10, TotalItems: 1, TotalPages: 1, HasNextPage: false},
	}

	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		q := r.URL.Query()
		// Verify state filter
		states := q["state"]
		if len(states) != 1 || states[0] != "Running" {
			t.Errorf("expected state=[Running], got %v", states)
		}

		// Verify metadata filter
		meta := q.Get("metadata")
		if meta == "" {
			t.Error("expected metadata query param")
		}
		if !strings.Contains(meta, "env=prod") {
			t.Errorf("expected metadata to contain env=prod, got %q", meta)
		}

		// Verify pagination
		if q.Get("page") != "1" {
			t.Errorf("expected page=1, got %s", q.Get("page"))
		}
		if q.Get("pageSize") != "10" {
			t.Errorf("expected pageSize=10, got %s", q.Get("pageSize"))
		}

		jsonResponse(w, http.StatusOK, want)
	})

	mgr := &SandboxManager{lifecycle: client}
	got, err := mgr.ListSandboxInfos(context.Background(), ListOptions{
		States:   []SandboxState{StateRunning},
		Metadata: map[string]string{"env": "prod"},
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListSandboxInfos: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got.Items))
	}
	if got.Items[0].ID != "sbx-a" {
		t.Errorf("ID = %q, want %q", got.Items[0].ID, "sbx-a")
	}
	if got.Items[0].Metadata["env"] != "prod" {
		t.Errorf("Metadata[env] = %q, want %q", got.Items[0].Metadata["env"], "prod")
	}
}

func TestSandboxManager_ListMultipleStates(t *testing.T) {
	want := ListSandboxesResponse{
		Items: []SandboxInfo{
			{ID: "sbx-1", Status: SandboxStatus{State: StateRunning}, CreatedAt: time.Now().UTC().Truncate(time.Second)},
			{ID: "sbx-2", Status: SandboxStatus{State: StatePaused}, CreatedAt: time.Now().UTC().Truncate(time.Second)},
		},
		Pagination: PaginationInfo{Page: 1, PageSize: 20, TotalItems: 2, TotalPages: 1},
	}

	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		states := r.URL.Query()["state"]
		if len(states) != 2 {
			t.Errorf("expected 2 state params, got %d: %v", len(states), states)
		}
		jsonResponse(w, http.StatusOK, want)
	})

	mgr := &SandboxManager{lifecycle: client}
	got, err := mgr.ListSandboxInfos(context.Background(), ListOptions{
		States: []SandboxState{StateRunning, StatePaused},
	})
	if err != nil {
		t.Fatalf("ListSandboxInfos: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
}

func TestSandboxManager_GetSandboxInfo(t *testing.T) {
	want := SandboxInfo{
		ID:        "sbx-get",
		Status:    SandboxStatus{State: StateRunning},
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-get" {
			t.Errorf("expected /sandboxes/sbx-get, got %s", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, want)
	})

	mgr := &SandboxManager{lifecycle: client}
	got, err := mgr.GetSandboxInfo(context.Background(), "sbx-get")
	if err != nil {
		t.Fatalf("GetSandboxInfo: %v", err)
	}
	if got.ID != "sbx-get" {
		t.Errorf("ID = %q, want %q", got.ID, "sbx-get")
	}
}

func TestSandboxManager_KillSandbox(t *testing.T) {
	var called bool
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-kill" {
			t.Errorf("expected /sandboxes/sbx-kill, got %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	mgr := &SandboxManager{lifecycle: client}
	err := mgr.KillSandbox(context.Background(), "sbx-kill")
	if err != nil {
		t.Fatalf("KillSandbox: %v", err)
	}
	if !called {
		t.Error("expected DELETE to be called")
	}
}

func TestSandboxManager_PauseSandbox(t *testing.T) {
	var called bool
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-mgr-pause/pause" {
			t.Errorf("expected /sandboxes/sbx-mgr-pause/pause, got %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusAccepted)
	})

	mgr := &SandboxManager{lifecycle: client}
	err := mgr.PauseSandbox(context.Background(), "sbx-mgr-pause")
	if err != nil {
		t.Fatalf("PauseSandbox: %v", err)
	}
	if !called {
		t.Error("expected pause endpoint to be called")
	}
}

func TestSandboxManager_ResumeSandbox(t *testing.T) {
	var called bool
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-mgr-resume/resume" {
			t.Errorf("expected /sandboxes/sbx-mgr-resume/resume, got %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusAccepted)
	})

	mgr := &SandboxManager{lifecycle: client}
	err := mgr.ResumeSandbox(context.Background(), "sbx-mgr-resume")
	if err != nil {
		t.Fatalf("ResumeSandbox: %v", err)
	}
	if !called {
		t.Error("expected resume endpoint to be called")
	}
}

func TestSandboxManager_RenewSandbox(t *testing.T) {
	wantExpiry := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)

	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-renew/renew-expiration" {
			t.Errorf("expected /sandboxes/sbx-renew/renew-expiration, got %s", r.URL.Path)
		}

		var req RenewExpirationRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ExpiresAt.IsZero() {
			t.Error("expected non-zero ExpiresAt")
		}

		jsonResponse(w, http.StatusOK, RenewExpirationResponse{ExpiresAt: wantExpiry})
	})

	mgr := &SandboxManager{lifecycle: client}
	got, err := mgr.RenewSandbox(context.Background(), "sbx-renew", 1*time.Hour)
	if err != nil {
		t.Fatalf("RenewSandbox: %v", err)
	}
	if got.ExpiresAt.IsZero() {
		t.Error("expected non-zero ExpiresAt in response")
	}
}

// ---------------------------------------------------------------------------
// Phase 2: File operation tests
// ---------------------------------------------------------------------------

func TestCreateDirectory(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/directories" {
			t.Errorf("expected /directories, got %s", r.URL.Path)
		}

		var body map[string]map[string]int
		json.NewDecoder(r.Body).Decode(&body)
		dirEntry, ok := body["/sandbox/mydir"]
		if !ok {
			t.Error("expected /sandbox/mydir key in request body")
		}
		if dirEntry["mode"] != 755 {
			t.Errorf("mode = %d, want 755", dirEntry["mode"])
		}

		w.WriteHeader(http.StatusOK)
	})

	err := client.CreateDirectory(context.Background(), "/sandbox/mydir", 755)
	if err != nil {
		t.Fatalf("CreateDirectory: %v", err)
	}
}

func TestDeleteDirectory(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/directories") {
			t.Errorf("expected /directories path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("path") != "/sandbox/mydir" {
			t.Errorf("expected path=/sandbox/mydir, got %s", r.URL.Query().Get("path"))
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.DeleteDirectory(context.Background(), "/sandbox/mydir")
	if err != nil {
		t.Fatalf("DeleteDirectory: %v", err)
	}
}

func TestDeleteFiles(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/files") {
			t.Errorf("expected /files path, got %s", r.URL.Path)
		}

		paths := r.URL.Query()["path"]
		if len(paths) != 2 {
			t.Errorf("expected 2 path params, got %d: %v", len(paths), paths)
		}

		w.WriteHeader(http.StatusOK)
	})

	err := client.DeleteFiles(context.Background(), []string{"/tmp/a.txt", "/tmp/b.txt"})
	if err != nil {
		t.Fatalf("DeleteFiles: %v", err)
	}
}

func TestMoveFiles(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/files/mv" {
			t.Errorf("expected /files/mv, got %s", r.URL.Path)
		}

		var req MoveRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req) != 1 {
			t.Fatalf("expected 1 move item, got %d", len(req))
		}
		if req[0].Src != "/tmp/old.txt" || req[0].Dest != "/tmp/new.txt" {
			t.Errorf("move item = %+v, want src=/tmp/old.txt dest=/tmp/new.txt", req[0])
		}

		w.WriteHeader(http.StatusOK)
	})

	err := client.MoveFiles(context.Background(), MoveRequest{
		{Src: "/tmp/old.txt", Dest: "/tmp/new.txt"},
	})
	if err != nil {
		t.Fatalf("MoveFiles: %v", err)
	}
}

func TestSearchFiles(t *testing.T) {
	want := []FileInfo{
		{Path: "/sandbox/test.py", Size: 256, Owner: "root", Group: "root", Mode: 644},
		{Path: "/sandbox/test2.py", Size: 128, Owner: "root", Group: "root", Mode: 644},
	}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/files/search") {
			t.Errorf("expected /files/search, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("path") != "/sandbox" {
			t.Errorf("expected path=/sandbox, got %s", r.URL.Query().Get("path"))
		}
		if r.URL.Query().Get("pattern") != "*.py" {
			t.Errorf("expected pattern=*.py, got %s", r.URL.Query().Get("pattern"))
		}

		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.SearchFiles(context.Background(), "/sandbox", "*.py")
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 files, got %d", len(got))
	}
	if got[0].Path != "/sandbox/test.py" {
		t.Errorf("Path[0] = %q, want /sandbox/test.py", got[0].Path)
	}
}

func TestSetPermissions(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/files/permissions" {
			t.Errorf("expected /files/permissions, got %s", r.URL.Path)
		}

		var req PermissionsRequest
		json.NewDecoder(r.Body).Decode(&req)
		perm, ok := req["/tmp/script.sh"]
		if !ok {
			t.Error("expected /tmp/script.sh key in request")
		}
		if perm.Mode != 755 {
			t.Errorf("Mode = %d, want 755", perm.Mode)
		}
		if perm.Owner != "root" {
			t.Errorf("Owner = %q, want root", perm.Owner)
		}

		w.WriteHeader(http.StatusOK)
	})

	err := client.SetPermissions(context.Background(), PermissionsRequest{
		"/tmp/script.sh": {Owner: "root", Group: "root", Mode: 755},
	})
	if err != nil {
		t.Fatalf("SetPermissions: %v", err)
	}
}

func TestReplaceInFiles(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/files/replace" {
			t.Errorf("expected /files/replace, got %s", r.URL.Path)
		}

		var req ReplaceRequest
		json.NewDecoder(r.Body).Decode(&req)
		item, ok := req["/tmp/config.txt"]
		if !ok {
			t.Error("expected /tmp/config.txt key in request")
		}
		if item.Old != "localhost" || item.New != "production.example.com" {
			t.Errorf("replace item = %+v", item)
		}

		w.WriteHeader(http.StatusOK)
	})

	err := client.ReplaceInFiles(context.Background(), ReplaceRequest{
		"/tmp/config.txt": {Old: "localhost", New: "production.example.com"},
	})
	if err != nil {
		t.Fatalf("ReplaceInFiles: %v", err)
	}
}

func TestDownloadFile(t *testing.T) {
	fileContent := "hello from sandbox file"

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/files/download") {
			t.Errorf("expected /files/download, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("path") != "/sandbox/output.txt" {
			t.Errorf("expected path=/sandbox/output.txt, got %s", r.URL.Query().Get("path"))
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fileContent))
	})

	rc, err := client.DownloadFile(context.Background(), "/sandbox/output.txt", "")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != fileContent {
		t.Errorf("content = %q, want %q", string(data), fileContent)
	}
}

func TestDownloadFile_Range(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		rangeHdr := r.Header.Get("Range")
		if rangeHdr != "bytes=0-4" {
			t.Errorf("Range = %q, want %q", rangeHdr, "bytes=0-4")
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("hello"))
	})

	rc, err := client.DownloadFile(context.Background(), "/sandbox/big.bin", "bytes=0-4")
	if err != nil {
		t.Fatalf("DownloadFile range: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", string(data), "hello")
	}
}

// ---------------------------------------------------------------------------
// Phase 3: CodeInterpreter / Code context tests
// ---------------------------------------------------------------------------

func TestCreateContext(t *testing.T) {
	want := CodeContext{ID: "ctx-123", Language: "python"}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/code/context" {
			t.Errorf("expected /code/context, got %s", r.URL.Path)
		}

		var req CreateContextRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Language != "python" {
			t.Errorf("Language = %q, want python", req.Language)
		}

		jsonResponse(w, http.StatusCreated, want)
	})

	got, err := client.CreateContext(context.Background(), CreateContextRequest{Language: "python"})
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if got.ID != "ctx-123" {
		t.Errorf("ID = %q, want ctx-123", got.ID)
	}
	if got.Language != "python" {
		t.Errorf("Language = %q, want python", got.Language)
	}
}

func TestGetContext(t *testing.T) {
	want := CodeContext{ID: "ctx-456", Language: "python"}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/code/contexts/ctx-456" {
			t.Errorf("expected /code/contexts/ctx-456, got %s", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.GetContext(context.Background(), "ctx-456")
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if got.ID != "ctx-456" {
		t.Errorf("ID = %q, want ctx-456", got.ID)
	}
}

func TestListContexts(t *testing.T) {
	want := []CodeContext{
		{ID: "ctx-1", Language: "python"},
		{ID: "ctx-2", Language: "python"},
	}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/code/contexts") {
			t.Errorf("expected /code/contexts, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("language") != "python" {
			t.Errorf("expected language=python, got %s", r.URL.Query().Get("language"))
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.ListContexts(context.Background(), "python")
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 contexts, got %d", len(got))
	}
	if got[0].ID != "ctx-1" {
		t.Errorf("ID[0] = %q, want ctx-1", got[0].ID)
	}
}

func TestDeleteContext(t *testing.T) {
	var called bool
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/code/contexts/ctx-del" {
			t.Errorf("expected /code/contexts/ctx-del, got %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusOK)
	})

	err := client.DeleteContext(context.Background(), "ctx-del")
	if err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}
	if !called {
		t.Error("expected DELETE to be called")
	}
}

func TestDeleteContextsByLanguage(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/code/contexts") {
			t.Errorf("expected /code/contexts path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("language") != "python" {
			t.Errorf("expected language=python, got %s", r.URL.Query().Get("language"))
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.DeleteContextsByLanguage(context.Background(), "python")
	if err != nil {
		t.Fatalf("DeleteContextsByLanguage: %v", err)
	}
}

func TestExecuteCode_SSE(t *testing.T) {
	// Simulate execd SSE response for code execution
	ssePayload := strings.Join([]string{
		`{"type":"init","text":"exec-001","timestamp":1000}`,
		"",
		`{"type":"stdout","text":"4","timestamp":1001}`,
		"",
		`{"type":"result","results":{"text/plain":"4"},"timestamp":1002}`,
		"",
		`{"type":"execution_complete","timestamp":1003,"execution_time":50}`,
		"",
	}, "\n")

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/code" {
			t.Errorf("expected /code, got %s", r.URL.Path)
		}

		var req RunCodeRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Code != "2+2" {
			t.Errorf("Code = %q, want 2+2", req.Code)
		}
		if req.Context == nil || req.Context.Language != "python" {
			t.Errorf("expected context with language python, got %+v", req.Context)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	})

	var events []StreamEvent
	err := client.ExecuteCode(context.Background(), RunCodeRequest{
		Context: &CodeContext{Language: "python"},
		Code:    "2+2",
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	if events[0].Event != "init" {
		t.Errorf("event[0].Event = %q, want init", events[0].Event)
	}
	if events[1].Event != "stdout" {
		t.Errorf("event[1].Event = %q, want stdout", events[1].Event)
	}
	if events[2].Event != "result" {
		t.Errorf("event[2].Event = %q, want result", events[2].Event)
	}
	if events[3].Event != "execution_complete" {
		t.Errorf("event[3].Event = %q, want execution_complete", events[3].Event)
	}
}

func TestExecuteCode_InContext(t *testing.T) {
	ssePayload := `{"type":"stdout","text":"hello from context","timestamp":1000}` + "\n\n" +
		`{"type":"execution_complete","timestamp":1001,"execution_time":10}` + "\n\n"

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req RunCodeRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Context == nil {
			t.Fatal("expected context in request")
		}
		if req.Context.ID != "ctx-persist" {
			t.Errorf("Context.ID = %q, want ctx-persist", req.Context.ID)
		}
		if req.Context.Language != "python" {
			t.Errorf("Context.Language = %q, want python", req.Context.Language)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	})

	var events []StreamEvent
	err := client.ExecuteCode(context.Background(), RunCodeRequest{
		Context: &CodeContext{ID: "ctx-persist", Language: "python"},
		Code:    "print('hello from context')",
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCode in context: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestInterruptCode(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/code") {
			t.Errorf("expected /code path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("id") != "session-interrupt" {
			t.Errorf("expected id=session-interrupt, got %s", r.URL.Query().Get("id"))
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.InterruptCode(context.Background(), "session-interrupt")
	if err != nil {
		t.Fatalf("InterruptCode: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Phase 4: Session tests
// ---------------------------------------------------------------------------

func TestCreateSession(t *testing.T) {
	want := Session{ID: "sess-abc"}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/session" {
			t.Errorf("expected /session, got %s", r.URL.Path)
		}
		jsonResponse(w, http.StatusCreated, want)
	})

	got, err := client.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if got.ID != "sess-abc" {
		t.Errorf("ID = %q, want sess-abc", got.ID)
	}
}

func TestRunInSession_SSE(t *testing.T) {
	ssePayload := `{"type":"stdout","text":"bar","timestamp":2000}` + "\n\n" +
		`{"type":"execution_complete","timestamp":2001,"execution_time":5}` + "\n\n"

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/session/sess-run/run" {
			t.Errorf("expected /session/sess-run/run, got %s", r.URL.Path)
		}

		var req RunInSessionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Command != "echo $FOO" {
			t.Errorf("Command = %q, want echo $FOO", req.Command)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	})

	var events []StreamEvent
	err := client.RunInSession(context.Background(), "sess-run", RunInSessionRequest{
		Command: "echo $FOO",
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunInSession: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Event != "stdout" {
		t.Errorf("event[0].Event = %q, want stdout", events[0].Event)
	}
}

func TestDeleteSession(t *testing.T) {
	var called bool
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/session/sess-del" {
			t.Errorf("expected /session/sess-del, got %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusOK)
	})

	err := client.DeleteSession(context.Background(), "sess-del")
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if !called {
		t.Error("expected DELETE to be called")
	}
}

// ---------------------------------------------------------------------------
// Phase 5: Command management tests
// ---------------------------------------------------------------------------

func TestGetCommandStatus(t *testing.T) {
	started := time.Now().Add(-10 * time.Second).UTC().Truncate(time.Second)
	finished := time.Now().UTC().Truncate(time.Second)
	exitCode := int32(0)
	want := CommandStatusResponse{
		ID:         "cmd-status-1",
		Content:    "hello\n",
		Running:    false,
		ExitCode:   &exitCode,
		StartedAt:  started,
		FinishedAt: &finished,
	}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/command/status/cmd-status-1" {
			t.Errorf("expected /command/status/cmd-status-1, got %s", r.URL.Path)
		}
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.GetCommandStatus(context.Background(), "cmd-status-1")
	if err != nil {
		t.Fatalf("GetCommandStatus: %v", err)
	}
	if got.ID != "cmd-status-1" {
		t.Errorf("ID = %q, want cmd-status-1", got.ID)
	}
	if got.Running {
		t.Error("expected Running=false")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", got.ExitCode)
	}
	if got.Content != "hello\n" {
		t.Errorf("Content = %q, want %q", got.Content, "hello\n")
	}
}

func TestGetCommandStatus_Running(t *testing.T) {
	want := CommandStatusResponse{
		ID:        "cmd-running",
		Running:   true,
		StartedAt: time.Now().UTC().Truncate(time.Second),
	}

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, want)
	})

	got, err := client.GetCommandStatus(context.Background(), "cmd-running")
	if err != nil {
		t.Fatalf("GetCommandStatus: %v", err)
	}
	if !got.Running {
		t.Error("expected Running=true")
	}
	if got.ExitCode != nil {
		t.Errorf("expected nil ExitCode for running command, got %d", *got.ExitCode)
	}
}

func TestGetCommandLogs(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/command/cmd-logs-1/logs" {
			t.Errorf("expected /command/cmd-logs-1/logs, got %s", r.URL.Path)
		}

		// Verify Accept header
		if r.Header.Get("Accept") != "text/plain" {
			t.Errorf("Accept = %q, want text/plain", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("EXECD-COMMANDS-TAIL-CURSOR", "42")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("line1\nline2\n"))
	})

	got, err := client.GetCommandLogs(context.Background(), "cmd-logs-1", nil)
	if err != nil {
		t.Fatalf("GetCommandLogs: %v", err)
	}
	if got.Output != "line1\nline2\n" {
		t.Errorf("Output = %q, want %q", got.Output, "line1\nline2\n")
	}
	if got.Cursor != 42 {
		t.Errorf("Cursor = %d, want 42", got.Cursor)
	}
}

func TestGetCommandLogs_WithCursor(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		if cursor != "42" {
			t.Errorf("expected cursor=42, got %s", cursor)
		}

		w.Header().Set("EXECD-COMMANDS-TAIL-CURSOR", "99")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("line3\n"))
	})

	cursor := int64(42)
	got, err := client.GetCommandLogs(context.Background(), "cmd-logs-2", &cursor)
	if err != nil {
		t.Fatalf("GetCommandLogs with cursor: %v", err)
	}
	if got.Output != "line3\n" {
		t.Errorf("Output = %q, want %q", got.Output, "line3\n")
	}
	if got.Cursor != 99 {
		t.Errorf("Cursor = %d, want 99", got.Cursor)
	}
}

func TestInterruptCommand(t *testing.T) {
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/command") {
			t.Errorf("expected /command path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("id") != "cmd-int" {
			t.Errorf("expected id=cmd-int, got %s", r.URL.Query().Get("id"))
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.InterruptCommand(context.Background(), "cmd-int")
	if err != nil {
		t.Fatalf("InterruptCommand: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Phase 6: Metrics watch test
// ---------------------------------------------------------------------------

func TestWatchMetrics_SSE(t *testing.T) {
	// Simulate SSE metric events (NDJSON format)
	ssePayload := strings.Join([]string{
		`{"type":"metrics","cpu_count":4,"cpu_used_pct":10.5,"mem_total_mib":8192,"mem_used_mib":2048,"timestamp":1000}`,
		"",
		`{"type":"metrics","cpu_count":4,"cpu_used_pct":15.2,"mem_total_mib":8192,"mem_used_mib":2100,"timestamp":1001}`,
		"",
		`{"type":"metrics","cpu_count":4,"cpu_used_pct":12.0,"mem_total_mib":8192,"mem_used_mib":2050,"timestamp":1002}`,
		"",
	}, "\n")

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/metrics/watch" {
			t.Errorf("expected /metrics/watch, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	})

	var events []StreamEvent
	err := client.WatchMetrics(context.Background(), func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("WatchMetrics: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 metric events, got %d", len(events))
	}
	if events[0].Event != "metrics" {
		t.Errorf("event[0].Event = %q, want metrics", events[0].Event)
	}

	// Verify we can parse the metric data from events
	var m Metrics
	if err := json.Unmarshal([]byte(events[0].Data), &m); err != nil {
		t.Fatalf("unmarshal metric: %v", err)
	}
	if m.CPUCount != 4 {
		t.Errorf("CPUCount = %f, want 4", m.CPUCount)
	}
	if m.CPUUsedPct != 10.5 {
		t.Errorf("CPUUsedPct = %f, want 10.5", m.CPUUsedPct)
	}
}

func TestWatchMetrics_ContextCancel(t *testing.T) {
	// Use a handler that blocks until context is cancelled to verify
	// the client respects cancellation.
	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// Write one event then stall
		w.Write([]byte(`{"type":"metrics","cpu_count":2,"cpu_used_pct":5,"mem_total_mib":4096,"mem_used_mib":1024,"timestamp":1}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		// Block until request context done (client disconnects)
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())

	var eventCount int
	go func() {
		// Cancel after a short delay to let the first event arrive
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := client.WatchMetrics(ctx, func(event StreamEvent) error {
		eventCount++
		return nil
	})

	// Should get context cancelled error
	if err == nil {
		t.Error("expected error from cancelled context")
	}
	if eventCount < 1 {
		t.Errorf("expected at least 1 event before cancel, got %d", eventCount)
	}
}

// ---------------------------------------------------------------------------
// Execution result aggregation tests
// ---------------------------------------------------------------------------

func TestExecution_ProcessStreamEvents(t *testing.T) {
	// Test the full Execution aggregation pipeline with all event types
	exec := &Execution{}
	events := []StreamEvent{
		{Event: "init", Data: `{"type":"init","text":"exec-42","timestamp":100}`},
		{Event: "stdout", Data: `{"type":"stdout","text":"hello","timestamp":101}`},
		{Event: "stderr", Data: `{"type":"stderr","text":"warn: something","timestamp":102}`},
		{Event: "result", Data: `{"type":"result","results":{"text/plain":"42"},"timestamp":103}`},
		{Event: "execution_complete", Data: `{"type":"execution_complete","timestamp":104,"execution_time":200}`},
	}

	for _, ev := range events {
		if err := processStreamEvent(exec, ev, nil); err != nil {
			t.Fatalf("processStreamEvent(%s): %v", ev.Event, err)
		}
	}

	if exec.ID != "exec-42" {
		t.Errorf("ID = %q, want exec-42", exec.ID)
	}
	if len(exec.Stdout) != 1 || exec.Stdout[0].Text != "hello" {
		t.Errorf("Stdout = %+v, want [hello]", exec.Stdout)
	}
	if len(exec.Stderr) != 1 || exec.Stderr[0].Text != "warn: something" {
		t.Errorf("Stderr = %+v", exec.Stderr)
	}
	if len(exec.Results) != 1 || exec.Results[0].Text() != "42" {
		t.Errorf("Results = %+v", exec.Results)
	}
	if exec.Complete == nil {
		t.Error("expected Complete to be set")
	}
	if exec.ExitCode == nil || *exec.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", exec.ExitCode)
	}
	if exec.Text() != "hello" {
		t.Errorf("Text() = %q, want hello", exec.Text())
	}
}

func TestExecution_ErrorEvent(t *testing.T) {
	exec := &Execution{}
	event := StreamEvent{
		Event: "error",
		Data:  `{"type":"error","error":{"ename":"NameError","evalue":"name 'x' is not defined","traceback":["line 1"]}}`,
	}

	if err := processStreamEvent(exec, event, nil); err != nil {
		t.Fatalf("processStreamEvent: %v", err)
	}

	if exec.Error == nil {
		t.Fatal("expected Error to be set")
	}
	if exec.Error.Name != "NameError" {
		t.Errorf("Error.Name = %q, want NameError", exec.Error.Name)
	}
	if exec.Error.Value != "name 'x' is not defined" {
		t.Errorf("Error.Value = %q", exec.Error.Value)
	}
	if len(exec.Error.Traceback) != 1 {
		t.Errorf("Traceback len = %d, want 1", len(exec.Error.Traceback))
	}
}

func TestExecution_HandlersInvoked(t *testing.T) {
	exec := &Execution{}
	var initCalled, stdoutCalled, stderrCalled, resultCalled, completeCalled bool

	handlers := &ExecutionHandlers{
		OnInit:     func(e ExecutionInit) error { initCalled = true; return nil },
		OnStdout:   func(m OutputMessage) error { stdoutCalled = true; return nil },
		OnStderr:   func(m OutputMessage) error { stderrCalled = true; return nil },
		OnResult:   func(r ExecutionResult) error { resultCalled = true; return nil },
		OnComplete: func(c ExecutionComplete) error { completeCalled = true; return nil },
	}

	events := []StreamEvent{
		{Data: `{"type":"init","text":"x","timestamp":1}`},
		{Data: `{"type":"stdout","text":"out","timestamp":2}`},
		{Data: `{"type":"stderr","text":"err","timestamp":3}`},
		{Data: `{"type":"result","results":{"text/plain":"ok"},"timestamp":4}`},
		{Data: `{"type":"execution_complete","timestamp":5,"execution_time":100}`},
	}

	for _, ev := range events {
		if err := processStreamEvent(exec, ev, handlers); err != nil {
			t.Fatalf("processStreamEvent: %v", err)
		}
	}

	if !initCalled {
		t.Error("OnInit not called")
	}
	if !stdoutCalled {
		t.Error("OnStdout not called")
	}
	if !stderrCalled {
		t.Error("OnStderr not called")
	}
	if !resultCalled {
		t.Error("OnResult not called")
	}
	if !completeCalled {
		t.Error("OnComplete not called")
	}
}

// ---------------------------------------------------------------------------
// OctalMode helper test
// ---------------------------------------------------------------------------

func TestOctalMode(t *testing.T) {
	tests := []struct {
		mode os.FileMode
		want int
	}{
		{0755, 755},
		{0644, 644},
		{0700, 700},
		{0777, 777},
	}
	for _, tc := range tests {
		got := OctalMode(tc.mode)
		if got != tc.want {
			t.Errorf("OctalMode(%o) = %d, want %d", tc.mode, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Handler error propagation
// ---------------------------------------------------------------------------

func TestStreamSSE_HandlerError(t *testing.T) {
	ssePayload := "event: first\ndata: a\n\nevent: second\ndata: b\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	}))
	defer srv.Close()

	client := NewExecdClient(srv.URL, "tok")
	stopErr := fmt.Errorf("stop after first")

	var count int
	err := client.RunCommand(context.Background(), RunCommandRequest{Command: "x"}, func(event StreamEvent) error {
		count++
		if count == 1 {
			return stopErr
		}
		return nil
	})
	if err != stopErr {
		t.Errorf("expected stopErr, got %v", err)
	}
	if count != 1 {
		t.Errorf("handler called %d times, want 1", count)
	}
}

// ---------------------------------------------------------------------------
// Command with environment variables
// ---------------------------------------------------------------------------

func TestRunCommand_WithEnvs(t *testing.T) {
	ssePayload := `{"type":"stdout","text":"bar","timestamp":1000}` + "\n\n" +
		`{"type":"execution_complete","timestamp":1001,"execution_time":5}` + "\n\n"

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req RunCommandRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Envs == nil {
			t.Fatal("expected Envs to be set")
		}
		if req.Envs["FOO"] != "bar" {
			t.Errorf("Envs[FOO] = %q, want bar", req.Envs["FOO"])
		}
		if req.Envs["BAZ"] != "qux" {
			t.Errorf("Envs[BAZ] = %q, want qux", req.Envs["BAZ"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	})

	err := client.RunCommand(context.Background(), RunCommandRequest{
		Command: "echo $FOO",
		Envs:    map[string]string{"FOO": "bar", "BAZ": "qux"},
	}, func(event StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("RunCommand with Envs: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Background command execution
// ---------------------------------------------------------------------------

func TestRunCommand_Background(t *testing.T) {
	ssePayload := `{"type":"init","text":"cmd-bg-123","timestamp":1000}` + "\n\n" +
		`{"type":"execution_complete","timestamp":1001,"execution_time":0}` + "\n\n"

	_, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req RunCommandRequest
		json.NewDecoder(r.Body).Decode(&req)

		if !req.Background {
			t.Error("expected Background=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ssePayload))
	})

	var events []StreamEvent
	err := client.RunCommand(context.Background(), RunCommandRequest{
		Command:    "sleep 30",
		Background: true,
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunCommand background: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// X-Request-ID passthrough
// ---------------------------------------------------------------------------

func TestAPIError_RequestID(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-abc-123")
		jsonResponse(w, http.StatusNotFound, ErrorResponse{
			Code:    "SANDBOX_NOT_FOUND",
			Message: "not found",
		})
	})

	_, err := client.GetSandbox(context.Background(), "sbx-missing")
	if err == nil {
		t.Fatal("expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.RequestID != "req-abc-123" {
		t.Errorf("RequestID = %q, want req-abc-123", apiErr.RequestID)
	}
	if !strings.Contains(apiErr.Error(), "req-abc-123") {
		t.Errorf("Error() = %q, expected to contain request ID", apiErr.Error())
	}
}

// ---------------------------------------------------------------------------
// Network policy at create time
// ---------------------------------------------------------------------------

func TestCreateSandbox_WithNetworkPolicy(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req CreateSandboxRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.NetworkPolicy == nil {
			t.Fatal("expected NetworkPolicy to be set")
		}
		if req.NetworkPolicy.DefaultAction != "deny" {
			t.Errorf("DefaultAction = %q, want deny", req.NetworkPolicy.DefaultAction)
		}
		if len(req.NetworkPolicy.Egress) != 1 {
			t.Fatalf("expected 1 egress rule, got %d", len(req.NetworkPolicy.Egress))
		}
		if req.NetworkPolicy.Egress[0].Target != "api.example.com" {
			t.Errorf("Target = %q, want api.example.com", req.NetworkPolicy.Egress[0].Target)
		}

		jsonResponse(w, http.StatusCreated, SandboxInfo{
			ID:        "sbx-policy",
			Status:    SandboxStatus{State: StatePending},
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		})
	})

	_, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{
		Image:      ImageSpec{URI: "python:3.12"},
		Entrypoint: []string{"/bin/sh"},
		ResourceLimits: ResourceLimits{"cpu": "500m"},
		NetworkPolicy: &NetworkPolicy{
			DefaultAction: "deny",
			Egress: []NetworkRule{
				{Action: "allow", Target: "api.example.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with NetworkPolicy: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Volume mounts at create time
// ---------------------------------------------------------------------------

func TestCreateSandbox_WithVolumes(t *testing.T) {
	_, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req CreateSandboxRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Volumes) != 2 {
			t.Fatalf("expected 2 volumes, got %d", len(req.Volumes))
		}

		// Host volume
		v0 := req.Volumes[0]
		if v0.Name != "data" {
			t.Errorf("Volume[0].Name = %q, want data", v0.Name)
		}
		if v0.Host == nil || v0.Host.Path != "/host/data" {
			t.Errorf("Volume[0].Host = %+v, want /host/data", v0.Host)
		}
		if v0.MountPath != "/mnt/data" {
			t.Errorf("Volume[0].MountPath = %q, want /mnt/data", v0.MountPath)
		}
		if v0.ReadOnly {
			t.Error("Volume[0] should not be ReadOnly")
		}

		// PVC volume with subPath and readOnly
		v1 := req.Volumes[1]
		if v1.PVC == nil || v1.PVC.ClaimName != "my-pvc" {
			t.Errorf("Volume[1].PVC = %+v, want my-pvc", v1.PVC)
		}
		if !v1.ReadOnly {
			t.Error("Volume[1] should be ReadOnly")
		}
		if v1.SubPath != "subdir" {
			t.Errorf("Volume[1].SubPath = %q, want subdir", v1.SubPath)
		}

		jsonResponse(w, http.StatusCreated, SandboxInfo{
			ID:        "sbx-vols",
			Status:    SandboxStatus{State: StatePending},
			CreatedAt: time.Now().UTC().Truncate(time.Second),
		})
	})

	_, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{
		Image:          ImageSpec{URI: "python:3.12"},
		Entrypoint:     []string{"/bin/sh"},
		ResourceLimits: ResourceLimits{"cpu": "500m"},
		Volumes: []Volume{
			{
				Name:      "data",
				Host:      &Host{Path: "/host/data"},
				MountPath: "/mnt/data",
			},
			{
				Name:      "pvc-vol",
				PVC:       &PVC{ClaimName: "my-pvc"},
				MountPath: "/mnt/pvc",
				ReadOnly:  true,
				SubPath:   "subdir",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with Volumes: %v", err)
	}
}
