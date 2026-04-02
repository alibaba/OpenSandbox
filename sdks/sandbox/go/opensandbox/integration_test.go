//go:build integration

package opensandbox_test

import (
	"context"
	"fmt"
	"os"
	"strings"
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
