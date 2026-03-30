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

	// NDJSON events should have empty Event field and raw JSON as Data.
	if events[0].Event != "" {
		t.Errorf("event[0].Event = %q, want empty", events[0].Event)
	}
	if events[0].Data != `{"type":"stdout","data":"hello"}` {
		t.Errorf("event[0].Data = %q", events[0].Data)
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
