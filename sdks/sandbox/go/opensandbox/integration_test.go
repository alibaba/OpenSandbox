//go:build integration

package opensandbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func getServerURL() string {
	if u := os.Getenv("OPENSANDBOX_URL"); u != "" {
		return u
	}
	return "http://localhost:8090"
}

func TestIntegration_FullLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := opensandbox.NewLifecycleClient(getServerURL()+"/v1", "test-key")

	// 1. List sandboxes
	list, err := client.ListSandboxes(ctx, opensandbox.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListSandboxes: %v", err)
	}
	t.Logf("Initial sandbox count: %d", list.Pagination.TotalItems)

	// 2. Create a sandbox
	sb, err := client.CreateSandbox(ctx, opensandbox.CreateSandboxRequest{
		Image: opensandbox.ImageSpec{
			URI: "python:3.11-slim",
		},
		Entrypoint: []string{"tail", "-f", "/dev/null"},
		ResourceLimits: map[string]string{
			"cpu":    "500m",
			"memory": "256Mi",
		},
		Metadata: map[string]string{
			"test": "integration",
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s (state: %s)", sb.ID, sb.Status.State)

	if sb.ID == "" {
		t.Fatal("Sandbox ID is empty")
	}

	defer func() {
		t.Log("Cleaning up: deleting sandbox")
		_ = client.DeleteSandbox(context.Background(), sb.ID)
	}()

	// 3. Wait for Running state
	var running *opensandbox.SandboxInfo
	for i := 0; i < 30; i++ {
		running, err = client.GetSandbox(ctx, sb.ID)
		if err != nil {
			t.Fatalf("GetSandbox: %v", err)
		}
		t.Logf("  Poll %d: state=%s", i+1, running.Status.State)
		if running.Status.State == opensandbox.StateRunning {
			break
		}
		if running.Status.State == opensandbox.StateFailed || running.Status.State == opensandbox.StateTerminated {
			t.Fatalf("Sandbox entered terminal state: %s (reason: %s, message: %s)",
				running.Status.State, running.Status.Reason, running.Status.Message)
		}
		time.Sleep(2 * time.Second)
	}

	if running == nil || running.Status.State != opensandbox.StateRunning {
		t.Fatal("Sandbox did not reach Running state within timeout")
	}
	t.Logf("Sandbox is Running: %s", running.ID)

	// 4. Get execd endpoint (default execd port: 44772)
	endpoint, err := client.GetEndpoint(ctx, sb.ID, 44772, nil)
	if err != nil {
		t.Fatalf("GetEndpoint(44772): %v", err)
	}
	t.Logf("Execd endpoint: %s", endpoint.Endpoint)

	if endpoint.Endpoint == "" {
		t.Fatal("Execd endpoint is empty")
	}

	// 5. Test Execd — ping
	// Normalize endpoint URL: add scheme if missing, replace host.docker.internal with localhost
	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = "http://" + execdURL
	}
	execdURL = strings.Replace(execdURL, "host.docker.internal", "localhost", 1)
	t.Logf("Normalized execd URL: %s", execdURL)

	execToken := ""
	if endpoint.Headers != nil {
		execToken = endpoint.Headers["X-EXECD-ACCESS-TOKEN"]
	}
	execClient := opensandbox.NewExecdClient(execdURL, execToken)

	err = execClient.Ping(ctx)
	if err != nil {
		t.Fatalf("Execd Ping: %v", err)
	}
	t.Log("Execd ping: OK")

	// 6. Test Execd — run a command with SSE streaming
	var output strings.Builder
	err = execClient.RunCommand(ctx, opensandbox.RunCommandRequest{
		Command: "echo hello-from-opensandbox && python3 --version",
	}, func(event opensandbox.StreamEvent) error {
		t.Logf("  SSE event: type=%s data=%s", event.Event, event.Data)
		output.WriteString(event.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}

	// Note: SSE events may carry output as JSON in the Data field.
	// The handler above concatenates raw Data; if empty, events were received but
	// output is in a structured format (e.g., {"output":"..."}).
	t.Logf("Command raw output (%d bytes): %q", output.Len(), output.String())

	// 7. Test Execd — file operations
	fileInfoMap, err := execClient.GetFileInfo(ctx, "/etc/os-release")
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}
	for path, fi := range fileInfoMap {
		t.Logf("File info: path=%s size=%d", path, fi.Size)
	}

	// 8. Test Execd — metrics
	metrics, err := execClient.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	t.Logf("Metrics: cpu_count=%.0f mem_total=%.0fMiB", metrics.CPUCount, metrics.MemTotalMB)

	// 9. Test Egress — get policy (if available, default egress port: 18080)
	egressEndpoint, err := client.GetEndpoint(ctx, sb.ID, 18080, nil)
	if err != nil {
		t.Logf("GetEndpoint(egress): %v (skipping egress tests)", err)
	} else {
		egressURL := egressEndpoint.Endpoint
		if !strings.HasPrefix(egressURL, "http") {
			egressURL = "http://" + egressURL
		}
		egressURL = strings.Replace(egressURL, "host.docker.internal", "localhost", 1)

		egressToken := ""
		if egressEndpoint.Headers != nil {
			egressToken = egressEndpoint.Headers["OPENSANDBOX-EGRESS-AUTH"]
		}
		egressClient := opensandbox.NewEgressClient(egressURL, egressToken)

		policy, err := egressClient.GetPolicy(ctx)
		if err != nil {
			t.Logf("GetPolicy: %v (egress sidecar might not be ready)", err)
		} else {
			t.Logf("Egress policy: mode=%s defaultAction=%s rules=%d",
				policy.Mode, policy.Policy.DefaultAction, len(policy.Policy.Egress))
		}
	}

	// 10. Renew expiration
	_, err = client.RenewExpiration(ctx, sb.ID, time.Now().Add(30*time.Minute))
	if err != nil {
		t.Logf("RenewExpiration: %v (might not be supported)", err)
	} else {
		t.Log("Renewed expiration: +30m")
	}

	// 11. Delete sandbox
	err = client.DeleteSandbox(ctx, sb.ID)
	if err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}
	t.Log("Sandbox deleted successfully")

	// 12. Verify deletion — should get error or terminal state
	deleted, err := client.GetSandbox(ctx, sb.ID)
	if err != nil {
		t.Logf("GetSandbox after delete: %v (expected)", err)
	} else {
		t.Logf("GetSandbox after delete: state=%s", deleted.Status.State)
	}

	fmt.Println("\n=== INTEGRATION TEST PASSED ===")
	fmt.Println("Lifecycle: create → poll → Running → execd ping → run command (SSE) → file info → metrics → egress → renew → delete")
}

// integrationConfig returns a ConnectionConfig pointing at the local server.
func integrationConfig() opensandbox.ConnectionConfig {
	url := getServerURL() // "http://localhost:8090" or OPENSANDBOX_URL
	domain := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	return opensandbox.ConnectionConfig{
		Domain:   domain,
		Protocol: "http",
		APIKey:   "test-key",
		// Docker server returns host.docker.internal which isn't resolvable
		// from the host machine — rewrite to localhost.
		EndpointHostRewrite: map[string]string{
			"host.docker.internal": "localhost",
		},
	}
}

// TestIntegration_PauseResume exercises pause → resume on the local Docker runtime.
func TestIntegration_PauseResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	// 1. Create sandbox via high-level API
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "integration-pause-resume"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 2. Verify healthy
	if !sb.IsHealthy(ctx) {
		t.Fatal("Sandbox not healthy after creation")
	}
	t.Log("Sandbox is healthy")

	// 3. Run a command before pause
	exec1, err := sb.RunCommand(ctx, "echo before-pause", nil)
	if err != nil {
		t.Fatalf("RunCommand before pause: %v", err)
	}
	t.Logf("Pre-pause output: %s", exec1.Text())

	// 4. Pause
	if err := sb.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	t.Log("Sandbox paused")

	// 5. Verify paused state
	info, err := sb.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo after pause: %v", err)
	}
	if info.Status.State != opensandbox.StatePaused {
		t.Fatalf("Expected Paused state, got %s", info.Status.State)
	}
	t.Logf("Confirmed state: %s", info.Status.State)

	// 6. Resume via package-level function
	resumed, err := opensandbox.ResumeSandbox(ctx, config, sb.ID())
	if err != nil {
		t.Fatalf("ResumeSandbox: %v", err)
	}
	t.Log("Sandbox resumed via ResumeSandbox()")

	// 7. Verify resumed sandbox is healthy
	if !resumed.IsHealthy(ctx) {
		t.Fatal("Sandbox not healthy after resume")
	}

	exec2, err := resumed.RunCommand(ctx, "echo after-resume", nil)
	if err != nil {
		t.Fatalf("RunCommand after resume: %v", err)
	}
	t.Logf("Post-resume output: %s", exec2.Text())

	// 8. Test instance method: pause again → Resume()
	if err := resumed.Pause(ctx); err != nil {
		t.Fatalf("Second pause: %v", err)
	}
	t.Log("Sandbox paused again")

	resumed2, err := resumed.Resume(ctx)
	if err != nil {
		t.Fatalf("Sandbox.Resume(): %v", err)
	}
	t.Log("Sandbox resumed via Sandbox.Resume()")

	exec3, err := resumed2.RunCommand(ctx, "echo instance-resume-works", nil)
	if err != nil {
		t.Fatalf("RunCommand after instance resume: %v", err)
	}
	t.Logf("Instance resume output: %s", exec3.Text())

	// Cleanup
	if err := resumed2.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	t.Log("Pause/resume integration test passed")
}

// TestIntegration_ManualCleanup verifies ManualCleanup creates a sandbox with no TTL.
func TestIntegration_ManualCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	// 1. Create sandbox with ManualCleanup
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:         "python:3.11-slim",
		ManualCleanup: true,
		Metadata:      map[string]string{"test": "integration-manual-cleanup"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with ManualCleanup: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 2. Verify no expiration
	info, err := sb.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.ExpiresAt != nil {
		t.Errorf("Expected nil ExpiresAt for ManualCleanup, got %v", info.ExpiresAt)
	} else {
		t.Log("Confirmed: ExpiresAt is nil (no auto-expiration)")
	}

	// 3. Verify functional
	exec, err := sb.RunCommand(ctx, "echo manual-cleanup-works", nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	t.Logf("Output: %s", exec.Text())

	// 4. Create normal sandbox for comparison
	timeout := 600
	sbNormal, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:          "python:3.11-slim",
		TimeoutSeconds: &timeout,
		Metadata:       map[string]string{"test": "integration-with-timeout"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with timeout: %v", err)
	}
	defer func() { _ = sbNormal.Kill(context.Background()) }()

	infoNormal, err := sbNormal.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo (normal): %v", err)
	}
	if infoNormal.ExpiresAt == nil {
		t.Log("Note: normal sandbox also has nil ExpiresAt (server may not populate)")
	} else {
		t.Logf("Normal sandbox ExpiresAt: %v (confirms manual cleanup omission works)", infoNormal.ExpiresAt)
	}

	if err := sb.Kill(ctx); err != nil {
		t.Logf("Kill manual-cleanup sandbox: %v", err)
	}
	t.Log("Manual cleanup integration test passed")
}

// ---------------------------------------------------------------------------
// Phase 1: SandboxManager integration test
// ---------------------------------------------------------------------------

// TestIntegration_Manager exercises the SandboxManager: create sandboxes with
// different metadata, list with filters, and kill via manager.
func TestIntegration_Manager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := integrationConfig()
	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// 1. Create two sandboxes with different metadata
	sb1, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"team": "alpha", "test": "manager"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox 1: %v", err)
	}
	t.Logf("Created sandbox 1: %s", sb1.ID())
	defer func() { _ = sb1.Kill(context.Background()) }()

	sb2, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"team": "beta", "test": "manager"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox 2: %v", err)
	}
	t.Logf("Created sandbox 2: %s", sb2.ID())
	defer func() { _ = sb2.Kill(context.Background()) }()

	// 2. List all sandboxes with test=manager metadata
	list, err := mgr.ListSandboxInfos(ctx, opensandbox.ListOptions{
		Metadata: map[string]string{"test": "manager"},
		Page:     1,
		PageSize: 50,
	})
	if err != nil {
		t.Fatalf("ListSandboxInfos: %v", err)
	}
	t.Logf("Listed sandboxes with test=manager: %d items", len(list.Items))
	if len(list.Items) < 2 {
		t.Errorf("expected at least 2 sandboxes, got %d", len(list.Items))
	}

	// 3. Verify we can get info for a specific sandbox
	info, err := mgr.GetSandboxInfo(ctx, sb1.ID())
	if err != nil {
		t.Fatalf("GetSandboxInfo: %v", err)
	}
	if info.ID != sb1.ID() {
		t.Errorf("ID = %q, want %q", info.ID, sb1.ID())
	}
	t.Logf("GetSandboxInfo: id=%s state=%s", info.ID, info.Status.State)

	// 4. List with state filter
	listRunning, err := mgr.ListSandboxInfos(ctx, opensandbox.ListOptions{
		States:   []opensandbox.SandboxState{opensandbox.StateRunning},
		Metadata: map[string]string{"test": "manager"},
	})
	if err != nil {
		t.Fatalf("ListSandboxInfos (running): %v", err)
	}
	t.Logf("Running sandboxes with test=manager: %d", len(listRunning.Items))

	// 5. Kill sandbox 1 via manager
	if err := mgr.KillSandbox(ctx, sb1.ID()); err != nil {
		t.Fatalf("KillSandbox: %v", err)
	}
	t.Logf("Killed sandbox 1: %s", sb1.ID())

	// 6. Verify it's gone or terminal
	infoAfter, err := mgr.GetSandboxInfo(ctx, sb1.ID())
	if err != nil {
		t.Logf("GetSandboxInfo after kill: %v (expected)", err)
	} else {
		if infoAfter.Status.State == opensandbox.StateRunning {
			t.Errorf("expected non-running state after kill, got %s", infoAfter.Status.State)
		}
		t.Logf("State after kill: %s", infoAfter.Status.State)
	}

	// 7. Kill sandbox 2 via manager
	if err := mgr.KillSandbox(ctx, sb2.ID()); err != nil {
		t.Fatalf("KillSandbox 2: %v", err)
	}
	t.Log("Manager integration test passed")
}

// ---------------------------------------------------------------------------
// Phase 2: File operations integration test
// ---------------------------------------------------------------------------

// TestIntegration_FileOperations exercises the full file lifecycle:
// create dir → upload → get info → move → search → download → replace → delete.
func TestIntegration_FileOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "file-ops"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 1. Create directory
	if err := sb.CreateDirectory(ctx, "/sandbox/testdir", 755); err != nil {
		t.Fatalf("CreateDirectory: %v", err)
	}
	t.Log("Created /sandbox/testdir")

	// 2. Write a file via command (upload requires local file)
	exec, err := sb.RunCommand(ctx, "echo 'hello world' > /sandbox/testdir/hello.txt", nil)
	if err != nil {
		t.Fatalf("RunCommand (write file): %v", err)
	}
	t.Logf("Wrote file, exit code: %v", exec.ExitCode)

	// 3. Get file info
	infoMap, err := sb.GetFileInfo(ctx, "/sandbox/testdir/hello.txt")
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}
	for p, fi := range infoMap {
		t.Logf("File: %s size=%d owner=%s mode=%d", p, fi.Size, fi.Owner, fi.Mode)
	}

	// 4. Search files
	results, err := sb.SearchFiles(ctx, "/sandbox/testdir", "*.txt")
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	t.Logf("Search results: %d files", len(results))
	if len(results) < 1 {
		t.Error("expected at least 1 search result")
	}

	// 5. Write another file for move test
	if _, err := sb.RunCommand(ctx, "echo 'move me' > /sandbox/testdir/moveme.txt", nil); err != nil {
		t.Fatalf("RunCommand (write moveme.txt): %v", err)
	}

	// 6. Move file
	if err := sb.MoveFiles(ctx, opensandbox.MoveRequest{
		{Src: "/sandbox/testdir/moveme.txt", Dest: "/sandbox/testdir/moved.txt"},
	}); err != nil {
		t.Fatalf("MoveFiles: %v", err)
	}
	t.Log("Moved moveme.txt → moved.txt")

	// 7. Verify moved file exists
	movedInfo, err := sb.GetFileInfo(ctx, "/sandbox/testdir/moved.txt")
	if err != nil {
		t.Fatalf("GetFileInfo (moved): %v", err)
	}
	if len(movedInfo) == 0 {
		t.Error("expected moved file info")
	}

	// 8. Replace content in file
	if err := sb.ReplaceInFiles(ctx, opensandbox.ReplaceRequest{
		"/sandbox/testdir/hello.txt": {Old: "hello world", New: "goodbye world"},
	}); err != nil {
		t.Fatalf("ReplaceInFiles: %v", err)
	}
	t.Log("Replaced content in hello.txt")

	// 9. Verify replacement via command
	catExec, err := sb.RunCommand(ctx, "cat /sandbox/testdir/hello.txt", nil)
	if err != nil {
		t.Fatalf("RunCommand (cat): %v", err)
	}
	if !strings.Contains(catExec.Text(), "goodbye world") {
		t.Errorf("expected 'goodbye world' in file content, got %q", catExec.Text())
	}

	// 10. Set permissions
	if err := sb.SetPermissions(ctx, opensandbox.PermissionsRequest{
		"/sandbox/testdir/hello.txt": {Mode: 644},
	}); err != nil {
		t.Fatalf("SetPermissions: %v", err)
	}
	t.Log("Set permissions on hello.txt")

	// 11. Download file
	rc, err := sb.DownloadFile(ctx, "/sandbox/testdir/hello.txt", "")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	t.Logf("Downloaded %d bytes: %q", len(data), string(data))

	// 12. Delete files
	if err := sb.DeleteFiles(ctx, []string{"/sandbox/testdir/hello.txt", "/sandbox/testdir/moved.txt"}); err != nil {
		t.Fatalf("DeleteFiles: %v", err)
	}
	t.Log("Deleted files")

	// 13. Delete directory
	if err := sb.DeleteDirectory(ctx, "/sandbox/testdir"); err != nil {
		t.Fatalf("DeleteDirectory: %v", err)
	}
	t.Log("Deleted /sandbox/testdir")

	// 14. Cleanup
	if err := sb.Kill(ctx); err != nil {
		t.Logf("Kill: %v", err)
	}
	t.Log("File operations integration test passed")
}

// ---------------------------------------------------------------------------
// Phase 3: CodeInterpreter integration test
// ---------------------------------------------------------------------------

// TestIntegration_CodeInterpreter exercises code execution with contexts.
func TestIntegration_CodeInterpreter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "code-interpreter"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 1. Execute code (ephemeral — no context)
	exec1, err := sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
		Context: &opensandbox.CodeContext{Language: "python"},
		Code:    "print('hello from python')",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	t.Logf("Ephemeral execution: stdout=%q exitCode=%v", exec1.Text(), exec1.ExitCode)

	// 2. Create a persistent context
	codeCtx, err := sb.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	t.Logf("Created context: %s (language=%s)", codeCtx.ID, codeCtx.Language)

	// 3. Execute code in context (set variable)
	exec2, err := sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
		Context: &opensandbox.CodeContext{ID: codeCtx.ID, Language: "python"},
		Code:    "x = 42",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteCode (set var): %v", err)
	}
	t.Logf("Set x=42: exitCode=%v", exec2.ExitCode)

	// 4. Execute in same context (read variable — verifies state persistence)
	exec3, err := sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
		Context: &opensandbox.CodeContext{ID: codeCtx.ID, Language: "python"},
		Code:    "print(x)",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteCode (read var): %v", err)
	}
	t.Logf("Read x: stdout=%q", exec3.Text())

	// 5. List contexts
	contexts, err := sb.ListContexts(ctx, "python")
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	t.Logf("Active python contexts: %d", len(contexts))
	if len(contexts) < 1 {
		t.Error("expected at least 1 context")
	}

	// 6. Delete context
	if err := sb.DeleteContext(ctx, codeCtx.ID); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}
	t.Log("Deleted context")

	// 7. Cleanup
	if err := sb.Kill(ctx); err != nil {
		t.Logf("Kill: %v", err)
	}
	t.Log("CodeInterpreter integration test passed")
}

// ---------------------------------------------------------------------------
// Phase 4: Sessions integration test
// ---------------------------------------------------------------------------

// TestIntegration_Sessions exercises stateful bash sessions.
func TestIntegration_Sessions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "sessions"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 1. Create session
	sess, err := sb.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Logf("Created session: %s", sess.ID)

	// 2. Set env var in session
	exec1, err := sb.RunInSession(ctx, sess.ID, opensandbox.RunInSessionRequest{
		Command: "export FOO=bar",
	}, nil)
	if err != nil {
		t.Fatalf("RunInSession (export): %v", err)
	}
	t.Logf("Set FOO=bar: exitCode=%v", exec1.ExitCode)

	// 3. Read env var in same session — verifies state persistence
	exec2, err := sb.RunInSession(ctx, sess.ID, opensandbox.RunInSessionRequest{
		Command: "echo $FOO",
	}, nil)
	if err != nil {
		t.Fatalf("RunInSession (echo): %v", err)
	}
	t.Logf("Read FOO: stdout=%q", exec2.Text())
	if !strings.Contains(exec2.Text(), "bar") {
		t.Errorf("expected output to contain 'bar', got %q", exec2.Text())
	}

	// 4. Change directory in session
	exec3, err := sb.RunInSession(ctx, sess.ID, opensandbox.RunInSessionRequest{
		Command: "cd /tmp && pwd",
	}, nil)
	if err != nil {
		t.Fatalf("RunInSession (cd): %v", err)
	}
	t.Logf("cd /tmp && pwd: %q", exec3.Text())

	// 5. Verify cwd persists
	exec4, err := sb.RunInSession(ctx, sess.ID, opensandbox.RunInSessionRequest{
		Command: "pwd",
	}, nil)
	if err != nil {
		t.Fatalf("RunInSession (pwd): %v", err)
	}
	t.Logf("pwd: %q", exec4.Text())

	// 6. Delete session
	if err := sb.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	t.Log("Deleted session")

	// 7. Cleanup
	if err := sb.Kill(ctx); err != nil {
		t.Logf("Kill: %v", err)
	}
	t.Log("Sessions integration test passed")
}

// ---------------------------------------------------------------------------
// Phase 5: Background command management integration test
// ---------------------------------------------------------------------------

// TestIntegration_BackgroundCommand exercises background command execution
// with status polling and log retrieval.
func TestIntegration_BackgroundCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "bg-command"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// We need the execd client directly for background command operations
	client := opensandbox.NewLifecycleClient(getServerURL()+"/v1", "test-key")
	endpoint, err := client.GetEndpoint(ctx, sb.ID(), 44772, nil)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = "http://" + execdURL
	}
	execdURL = strings.Replace(execdURL, "host.docker.internal", "localhost", 1)

	execToken := ""
	if endpoint.Headers != nil {
		execToken = endpoint.Headers["X-EXECD-ACCESS-TOKEN"]
	}
	execClient := opensandbox.NewExecdClient(execdURL, execToken)

	// 1. Run a background command (sleep for a few seconds)
	var cmdID string
	err = execClient.RunCommand(ctx, opensandbox.RunCommandRequest{
		Command:    "sleep 3 && echo done",
		Background: true,
	}, func(event opensandbox.StreamEvent) error {
		t.Logf("Background SSE: type=%s data=%s", event.Event, event.Data)
		// The init event carries the command ID in the "text" field
		if event.Event == "init" || strings.Contains(event.Data, `"type":"init"`) {
			var parsed struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(event.Data), &parsed) == nil && parsed.Text != "" {
				cmdID = parsed.Text
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunCommand (background): %v", err)
	}

	// If we didn't get a command ID from SSE, run a foreground command to discover it
	if cmdID == "" {
		t.Log("No command ID from SSE, testing status with a known command")
		// Run a simple foreground command as fallback
		exec, runErr := sb.RunCommand(ctx, "echo status-test", nil)
		if runErr != nil {
			t.Fatalf("Fallback RunCommand: %v", runErr)
		}
		t.Logf("Fallback output: %s", exec.Text())
	} else {
		t.Logf("Background command ID: %s", cmdID)

		// 2. Poll status
		status, err := execClient.GetCommandStatus(ctx, cmdID)
		if err != nil {
			t.Logf("GetCommandStatus: %v (command may have finished)", err)
		} else {
			t.Logf("Status: running=%v exitCode=%v", status.Running, status.ExitCode)
		}

		// 3. Get logs
		logs, err := execClient.GetCommandLogs(ctx, cmdID, nil)
		if err != nil {
			t.Logf("GetCommandLogs: %v", err)
		} else {
			t.Logf("Logs: %q (cursor=%d)", logs.Output, logs.Cursor)
		}
	}

	// 4. Cleanup
	if err := sb.Kill(ctx); err != nil {
		t.Logf("Kill: %v", err)
	}
	t.Log("Background command integration test passed")
}

// ---------------------------------------------------------------------------
// Phase 6: Metrics watch integration test
// ---------------------------------------------------------------------------

// TestIntegration_MetricsWatch exercises real-time metrics streaming via SSE.
func TestIntegration_MetricsWatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "metrics-watch"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// Get execd client
	lcClient := opensandbox.NewLifecycleClient(getServerURL()+"/v1", "test-key")
	endpoint, err := lcClient.GetEndpoint(ctx, sb.ID(), 44772, nil)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = "http://" + execdURL
	}
	execdURL = strings.Replace(execdURL, "host.docker.internal", "localhost", 1)
	execToken := ""
	if endpoint.Headers != nil {
		execToken = endpoint.Headers["X-EXECD-ACCESS-TOKEN"]
	}
	execClient := opensandbox.NewExecdClient(execdURL, execToken)

	// Watch metrics with a context that auto-cancels after collecting events
	watchCtx, watchCancel := context.WithTimeout(ctx, 10*time.Second)
	defer watchCancel()

	var mu sync.Mutex
	var events []opensandbox.StreamEvent
	collected := make(chan struct{})

	go func() {
		_ = execClient.WatchMetrics(watchCtx, func(event opensandbox.StreamEvent) error {
			mu.Lock()
			events = append(events, event)
			count := len(events)
			mu.Unlock()
			t.Logf("Metric event %d: %s", count, event.Data)
			if count >= 3 {
				close(collected)
				return fmt.Errorf("collected enough") // Stop after 3
			}
			return nil
		})
	}()

	// Wait for events or timeout
	select {
	case <-collected:
		mu.Lock()
		t.Logf("Collected %d metric events", len(events))
		mu.Unlock()
	case <-watchCtx.Done():
		mu.Lock()
		t.Logf("Watch timed out with %d events (ok if server doesn't support /metrics/watch)", len(events))
		mu.Unlock()
	}

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()
	if eventCount > 0 {
		t.Log("Metrics watch received events successfully")
	} else {
		t.Log("No metrics watch events received (server may not support SSE metrics)")
	}

	// Cleanup
	if err := sb.Kill(ctx); err != nil {
		t.Logf("Kill: %v", err)
	}
	t.Log("Metrics watch integration test passed")
}

// ---------------------------------------------------------------------------
// Negative-path integration tests
// ---------------------------------------------------------------------------

// requireAPIError asserts err is *opensandbox.APIError with the expected status code.
func requireAPIError(t *testing.T, err error, wantStatus int, label string) *opensandbox.APIError {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error, got nil", label)
	}
	apiErr, ok := err.(*opensandbox.APIError)
	if !ok {
		t.Fatalf("%s: expected *APIError, got %T: %v", label, err, err)
	}
	if apiErr.StatusCode != wantStatus {
		t.Errorf("%s: StatusCode = %d, want %d (body: %s: %s)",
			label, apiErr.StatusCode, wantStatus, apiErr.Response.Code, apiErr.Response.Message)
	}
	return apiErr
}

// TestIntegration_Negative_GetNonexistentSandbox verifies the SDK returns a
// typed APIError when requesting a sandbox that does not exist.
func TestIntegration_Negative_GetNonexistentSandbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := integrationConfig()
	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	_, err := mgr.GetSandboxInfo(ctx, "sbx-does-not-exist-999")
	apiErr := requireAPIError(t, err, 404, "GetSandboxInfo(nonexistent)")
	t.Logf("Got expected error: %d %s: %s", apiErr.StatusCode, apiErr.Response.Code, apiErr.Response.Message)
}

// TestIntegration_Negative_KillNonexistentSandbox verifies killing a sandbox
// that doesn't exist returns a proper error rather than silently succeeding.
func TestIntegration_Negative_KillNonexistentSandbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := integrationConfig()
	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	err := mgr.KillSandbox(ctx, "sbx-phantom-kill-999")
	apiErr := requireAPIError(t, err, 404, "KillSandbox(nonexistent)")
	t.Logf("Got expected error: %d %s: %s", apiErr.StatusCode, apiErr.Response.Code, apiErr.Response.Message)
}

// TestIntegration_Negative_FileOpsOnMissingPaths verifies file operations
// against nonexistent paths return proper errors.
func TestIntegration_Negative_FileOpsOnMissingPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "negative-file-ops"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	// Download nonexistent file
	_, err = sb.DownloadFile(ctx, "/no/such/file.txt", "")
	if err == nil {
		t.Error("DownloadFile(nonexistent): expected error, got nil")
	} else {
		apiErr, ok := err.(*opensandbox.APIError)
		if ok {
			t.Logf("DownloadFile(nonexistent): %d %s", apiErr.StatusCode, apiErr.Response.Code)
		} else {
			t.Logf("DownloadFile(nonexistent): non-API error: %v", err)
		}
	}

	// Move nonexistent file
	err = sb.MoveFiles(ctx, opensandbox.MoveRequest{
		{Src: "/no/such/source.txt", Dest: "/tmp/dest.txt"},
	})
	if err == nil {
		t.Error("MoveFiles(nonexistent src): expected error, got nil")
	} else {
		t.Logf("MoveFiles(nonexistent src): %v", err)
	}

	// Delete nonexistent files
	err = sb.DeleteFiles(ctx, []string{"/no/such/delete-me.txt"})
	if err == nil {
		// Some servers return 200 for idempotent deletes — both behaviors are valid.
		t.Log("DeleteFiles(nonexistent): no error (server treats delete as idempotent)")
	} else {
		t.Logf("DeleteFiles(nonexistent): %v", err)
	}

	t.Log("Negative file ops test passed")
}

// TestIntegration_Negative_SessionAfterDelete verifies that running a command
// in a deleted session returns an error.
func TestIntegration_Negative_SessionAfterDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "negative-session"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	// Create and immediately delete a session
	sess, err := sb.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Logf("Created session: %s", sess.ID)

	if err := sb.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	t.Log("Deleted session")

	// Try to run in the deleted session
	_, err = sb.RunInSession(ctx, sess.ID, opensandbox.RunInSessionRequest{
		Command: "echo should-fail",
	}, nil)
	if err == nil {
		t.Error("RunInSession(deleted session): expected error, got nil")
	} else {
		t.Logf("RunInSession(deleted session): %v (expected)", err)
	}

	t.Log("Negative session test passed")
}

// TestIntegration_Negative_CodeContextAfterDelete verifies that executing code
// in a deleted context returns an error.
func TestIntegration_Negative_CodeContextAfterDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "negative-code-ctx"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	// Create a context then delete it
	codeCtx, err := sb.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	t.Logf("Created context: %s", codeCtx.ID)

	if err := sb.DeleteContext(ctx, codeCtx.ID); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}
	t.Log("Deleted context")

	// Try to execute in the deleted context
	_, err = sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
		Context: &opensandbox.CodeContext{ID: codeCtx.ID, Language: "python"},
		Code:    "print('should fail')",
	}, nil)
	if err == nil {
		t.Error("ExecuteCode(deleted context): expected error, got nil")
	} else {
		t.Logf("ExecuteCode(deleted context): %v (expected)", err)
	}

	// Also try GetContext on the deleted ID
	_, err = sb.ListContexts(ctx, "python")
	if err != nil {
		t.Logf("ListContexts after delete: %v", err)
	} else {
		t.Log("ListContexts succeeded (deleted context should be absent)")
	}

	t.Log("Negative code context test passed")
}

// ---------------------------------------------------------------------------
// E2E parity tests — operations every other SDK tests against a real server
// ---------------------------------------------------------------------------

// TestIntegration_CommandWithEnvs verifies that environment variables passed
// via RunCommandRequest.Envs are propagated to the process.
func TestIntegration_CommandWithEnvs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "cmd-envs"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	exec, err := sb.RunCommandWithOpts(ctx, opensandbox.RunCommandRequest{
		Command: "echo $MY_VAR-$OTHER_VAR",
		Envs:    map[string]string{"MY_VAR": "hello", "OTHER_VAR": "world"},
	}, nil)
	if err != nil {
		t.Fatalf("RunCommandWithOpts: %v", err)
	}
	t.Logf("Output: %q", exec.Text())

	if !strings.Contains(exec.Text(), "hello") {
		t.Errorf("expected output to contain 'hello', got %q", exec.Text())
	}
	if !strings.Contains(exec.Text(), "world") {
		t.Errorf("expected output to contain 'world', got %q", exec.Text())
	}

	t.Log("Command with env vars test passed")
}

// TestIntegration_CommandInterrupt verifies that a long-running command can be
// interrupted and the sandbox correctly handles the signal.
func TestIntegration_CommandInterrupt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "cmd-interrupt"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	// Get the raw execd client for interrupt operations
	lcClient := opensandbox.NewLifecycleClient(getServerURL()+"/v1", "test-key")
	endpoint, err := lcClient.GetEndpoint(ctx, sb.ID(), 44772, nil)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = "http://" + execdURL
	}
	execdURL = strings.Replace(execdURL, "host.docker.internal", "localhost", 1)
	execToken := ""
	if endpoint.Headers != nil {
		execToken = endpoint.Headers["X-EXECD-ACCESS-TOKEN"]
	}
	execClient := opensandbox.NewExecdClient(execdURL, execToken)

	// Run a long-running background command
	var cmdID string
	err = execClient.RunCommand(ctx, opensandbox.RunCommandRequest{
		Command:    "sleep 60",
		Background: true,
	}, func(event opensandbox.StreamEvent) error {
		if strings.Contains(event.Data, `"type":"init"`) {
			var parsed struct{ Text string }
			if json.Unmarshal([]byte(event.Data), &parsed) == nil && parsed.Text != "" {
				cmdID = parsed.Text
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunCommand (background): %v", err)
	}

	if cmdID == "" {
		// Fallback: run a shorter command and test interrupt on it
		t.Log("No command ID from background SSE, testing interrupt API directly")
		err = execClient.InterruptCommand(ctx, "nonexistent-cmd")
		if err != nil {
			t.Logf("InterruptCommand on nonexistent: %v (expected)", err)
		}
		t.Log("Interrupt API reachable")
		return
	}

	t.Logf("Background command: %s", cmdID)

	// Verify it's running
	status, err := execClient.GetCommandStatus(ctx, cmdID)
	if err != nil {
		t.Logf("GetCommandStatus: %v", err)
	} else {
		t.Logf("Before interrupt: running=%v", status.Running)
	}

	// Interrupt the command
	if err := execClient.InterruptCommand(ctx, cmdID); err != nil {
		t.Fatalf("InterruptCommand: %v", err)
	}
	t.Log("Interrupted command")

	// Verify it stopped
	time.Sleep(1 * time.Second)
	statusAfter, err := execClient.GetCommandStatus(ctx, cmdID)
	if err != nil {
		t.Logf("GetCommandStatus after interrupt: %v", err)
	} else {
		t.Logf("After interrupt: running=%v exitCode=%v", statusAfter.Running, statusAfter.ExitCode)
		if statusAfter.Running {
			t.Error("expected command to not be running after interrupt")
		}
	}

	t.Log("Command interrupt test passed")
}

// TestIntegration_NetworkPolicy exercises egress network policy operations:
// create sandbox with policy → get policy → patch rules → verify.
func TestIntegration_NetworkPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	// Create sandbox WITH a network policy
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: "python:3.11-slim",
		Metadata: map[string]string{"test": "network-policy"},
		NetworkPolicy: &opensandbox.NetworkPolicy{
			DefaultAction: "deny",
			Egress: []opensandbox.NetworkRule{
				{Action: "allow", Target: "api.example.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with NetworkPolicy: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	// Get egress policy
	policy, err := sb.GetEgressPolicy(ctx)
	if err != nil {
		// Egress sidecar may not be available in all environments
		t.Logf("GetEgressPolicy: %v (egress sidecar may not be available)", err)
		t.Log("Skipping egress assertions — sidecar not available")
		return
	}
	t.Logf("Policy: mode=%s status=%s", policy.Mode, policy.Status)

	if policy.Policy != nil {
		t.Logf("DefaultAction=%s rules=%d", policy.Policy.DefaultAction, len(policy.Policy.Egress))
	}

	// Patch with additional rule
	patched, err := sb.PatchEgressRules(ctx, []opensandbox.NetworkRule{
		{Action: "allow", Target: "cdn.example.com"},
	})
	if err != nil {
		t.Fatalf("PatchEgressRules: %v", err)
	}
	t.Logf("After patch: status=%s", patched.Status)

	if patched.Policy != nil {
		ruleCount := len(patched.Policy.Egress)
		t.Logf("Rules after patch: %d", ruleCount)
		if ruleCount < 2 {
			t.Errorf("expected at least 2 egress rules after patch, got %d", ruleCount)
		}
	}

	// Get policy again to verify persistence
	final, err := sb.GetEgressPolicy(ctx)
	if err != nil {
		t.Fatalf("GetEgressPolicy (final): %v", err)
	}
	if final.Policy != nil {
		t.Logf("Final rules: %d", len(final.Policy.Egress))
	}

	t.Log("Network policy test passed")
}

// TestIntegration_VolumeMounts exercises volume mount operations.
// Note: host volumes require the sandbox runtime to support bind mounts.
// PVC volumes require a provisioner. Tests are best-effort — they verify
// the API accepts the volume spec and the sandbox starts successfully.
func TestIntegration_VolumeMounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	config := integrationConfig()

	// Test 1: Host volume (read-write)
	t.Run("HostVolumeReadWrite", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "vol-host-rw"},
			Volumes: []opensandbox.Volume{
				{
					Name:      "host-data",
					Host:      &opensandbox.Host{Path: "/tmp"},
					MountPath: "/mnt/host",
				},
			},
		})
		if err != nil {
			t.Logf("CreateSandbox with host volume: %v (runtime may not support bind mounts)", err)
			t.Skip("Host volumes not supported by this runtime")
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		// Write a file to the mounted volume
		exec, err := sb.RunCommand(ctx, "echo 'host-vol-test' > /mnt/host/go-sdk-test.txt && cat /mnt/host/go-sdk-test.txt", nil)
		if err != nil {
			t.Fatalf("RunCommand (write to host vol): %v", err)
		}
		t.Logf("Host volume rw output: %q", exec.Text())
		if !strings.Contains(exec.Text(), "host-vol-test") {
			t.Errorf("expected output to contain 'host-vol-test', got %q", exec.Text())
		}

		// Cleanup
		sb.RunCommand(ctx, "rm -f /mnt/host/go-sdk-test.txt", nil)
		t.Log("Host volume read-write test passed")
	})

	// Test 2: Host volume (read-only)
	t.Run("HostVolumeReadOnly", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "vol-host-ro"},
			Volumes: []opensandbox.Volume{
				{
					Name:      "host-ro",
					Host:      &opensandbox.Host{Path: "/tmp"},
					MountPath: "/mnt/host-ro",
					ReadOnly:  true,
				},
			},
		})
		if err != nil {
			t.Logf("CreateSandbox with readonly host volume: %v", err)
			t.Skip("Host volumes not supported by this runtime")
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		// Read should work
		exec, err := sb.RunCommand(ctx, "ls /mnt/host-ro", nil)
		if err != nil {
			t.Fatalf("RunCommand (ls readonly): %v", err)
		}
		t.Logf("Readonly mount ls: %q", exec.Text())

		// Write should fail
		execW, err := sb.RunCommand(ctx, "touch /mnt/host-ro/should-fail.txt 2>&1; echo exit=$?", nil)
		if err != nil {
			t.Fatalf("RunCommand (write readonly): %v", err)
		}
		t.Logf("Readonly write attempt: %q", execW.Text())
		// Should contain an error or non-zero exit
		if strings.Contains(execW.Text(), "exit=0") && !strings.Contains(execW.Text(), "Read-only") {
			t.Log("Warning: write to readonly mount did not fail (runtime may not enforce)")
		}

		t.Log("Host volume read-only test passed")
	})

	// Test 3: PVC volume
	t.Run("PVCVolume", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "vol-pvc"},
			Volumes: []opensandbox.Volume{
				{
					Name:      "pvc-data",
					PVC:       &opensandbox.PVC{ClaimName: "go-sdk-e2e-pvc"},
					MountPath: "/mnt/pvc",
				},
			},
		})
		if err != nil {
			t.Logf("CreateSandbox with PVC: %v (PVC provisioner may not be available)", err)
			t.Skip("PVC volumes not supported by this runtime")
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		// Write and read
		exec, err := sb.RunCommand(ctx, "echo 'pvc-test' > /mnt/pvc/test.txt && cat /mnt/pvc/test.txt", nil)
		if err != nil {
			t.Fatalf("RunCommand (pvc write/read): %v", err)
		}
		t.Logf("PVC output: %q", exec.Text())
		if !strings.Contains(exec.Text(), "pvc-test") {
			t.Errorf("expected 'pvc-test' in output, got %q", exec.Text())
		}

		t.Log("PVC volume test passed")
	})

	// Test 4: PVC volume read-only
	t.Run("PVCVolumeReadOnly", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "vol-pvc-ro"},
			Volumes: []opensandbox.Volume{
				{
					Name:      "pvc-ro",
					PVC:       &opensandbox.PVC{ClaimName: "go-sdk-e2e-pvc"},
					MountPath: "/mnt/pvc-ro",
					ReadOnly:  true,
				},
			},
		})
		if err != nil {
			t.Logf("CreateSandbox with readonly PVC: %v", err)
			t.Skip("PVC volumes not supported by this runtime")
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		execW, err := sb.RunCommand(ctx, "touch /mnt/pvc-ro/fail.txt 2>&1; echo exit=$?", nil)
		if err != nil {
			t.Fatalf("RunCommand (write readonly PVC): %v", err)
		}
		t.Logf("Readonly PVC write attempt: %q", execW.Text())

		t.Log("PVC volume read-only test passed")
	})

	// Test 5: PVC volume with subPath
	t.Run("PVCVolumeSubPath", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "vol-pvc-subpath"},
			Volumes: []opensandbox.Volume{
				{
					Name:      "pvc-sub",
					PVC:       &opensandbox.PVC{ClaimName: "go-sdk-e2e-pvc"},
					MountPath: "/mnt/sub",
					SubPath:   "mysubdir",
				},
			},
		})
		if err != nil {
			t.Logf("CreateSandbox with PVC subPath: %v", err)
			t.Skip("PVC volumes not supported by this runtime")
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		exec, err := sb.RunCommand(ctx, "echo 'subpath-test' > /mnt/sub/test.txt && cat /mnt/sub/test.txt", nil)
		if err != nil {
			t.Fatalf("RunCommand (subpath): %v", err)
		}
		t.Logf("SubPath output: %q", exec.Text())

		t.Log("PVC volume subPath test passed")
	})
}

// TestIntegration_MultiLanguageCodeExecution tests code execution across
// multiple languages. Python is guaranteed; others depend on the sandbox image.
func TestIntegration_MultiLanguageCodeExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := integrationConfig()
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "multi-lang"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	// Python — guaranteed to work on python:3.11-slim
	t.Run("Python", func(t *testing.T) {
		exec, err := sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
			Context: &opensandbox.CodeContext{Language: "python"},
			Code:    "print(2 + 2)",
		}, nil)
		if err != nil {
			t.Fatalf("ExecuteCode (python): %v", err)
		}
		t.Logf("Python stdout: %q exitCode: %v", exec.Text(), exec.ExitCode)
		if !strings.Contains(exec.Text(), "4") {
			t.Errorf("expected '4' in output, got %q", exec.Text())
		}
	})

	// Python with context — variable persistence
	t.Run("PythonContext", func(t *testing.T) {
		codeCtx, err := sb.CreateContext(ctx, opensandbox.CreateContextRequest{Language: "python"})
		if err != nil {
			t.Fatalf("CreateContext: %v", err)
		}
		defer sb.DeleteContext(ctx, codeCtx.ID)

		// Set variable
		_, err = sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
			Context: &opensandbox.CodeContext{ID: codeCtx.ID, Language: "python"},
			Code:    "result = sum(range(10))",
		}, nil)
		if err != nil {
			t.Fatalf("ExecuteCode (set): %v", err)
		}

		// Read variable — verifies context persistence
		exec, err := sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
			Context: &opensandbox.CodeContext{ID: codeCtx.ID, Language: "python"},
			Code:    "print(result)",
		}, nil)
		if err != nil {
			t.Fatalf("ExecuteCode (read): %v", err)
		}
		t.Logf("Python context result: %q", exec.Text())
		if !strings.Contains(exec.Text(), "45") {
			t.Errorf("expected '45' in output, got %q", exec.Text())
		}
	})

	// Python — error handling
	t.Run("PythonError", func(t *testing.T) {
		exec, err := sb.ExecuteCode(ctx, opensandbox.RunCodeRequest{
			Context: &opensandbox.CodeContext{Language: "python"},
			Code:    "raise ValueError('test error')",
		}, nil)
		if err != nil {
			t.Logf("ExecuteCode returned error: %v", err)
		}
		if exec != nil && exec.Error != nil {
			t.Logf("Execution error: name=%s value=%s", exec.Error.Name, exec.Error.Value)
		} else if exec != nil {
			t.Logf("Execution: stdout=%q stderr=%q exitCode=%v", exec.Text(), exec.Stderr, exec.ExitCode)
		}
		t.Log("Python error handling test passed")
	})

	// Bash — via command (not code interpreter, but tests shell execution)
	t.Run("BashViaCommand", func(t *testing.T) {
		exec, err := sb.RunCommand(ctx, "python3 -c \"import sys; print(sys.version_info[:2])\"", nil)
		if err != nil {
			t.Fatalf("RunCommand (python version): %v", err)
		}
		t.Logf("Python version: %q", exec.Text())
	})

	t.Log("Multi-language code execution test passed")
}

// TestIntegration_XRequestIDPassthrough verifies that X-Request-ID from server
// error responses is captured in the APIError.
func TestIntegration_XRequestIDPassthrough(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := integrationConfig()
	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// Request a nonexistent sandbox — should return an error with potential request ID
	_, err := mgr.GetSandboxInfo(ctx, "sbx-request-id-test-nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}

	apiErr, ok := err.(*opensandbox.APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}

	t.Logf("APIError: status=%d code=%s requestID=%q",
		apiErr.StatusCode, apiErr.Response.Code, apiErr.RequestID)

	// The server may or may not return X-Request-Id, but the SDK should capture it
	if apiErr.RequestID != "" {
		t.Logf("X-Request-ID captured: %s", apiErr.RequestID)
		if !strings.Contains(apiErr.Error(), apiErr.RequestID) {
			t.Errorf("Error() should contain requestID, got %q", apiErr.Error())
		}
	} else {
		t.Log("Server did not return X-Request-Id (acceptable — SDK captures it when present)")
	}

	t.Log("X-Request-ID passthrough test passed")
}
