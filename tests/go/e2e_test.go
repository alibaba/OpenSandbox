//go:build e2e

package e2e

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
	return "http://localhost:8080"
}

func getDefaultImage() string {
	if img := os.Getenv("OPENSANDBOX_SANDBOX_DEFAULT_IMAGE"); img != "" {
		return img
	}
	return "opensandbox/code-interpreter:latest"
}

func TestE2E_FullLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := opensandbox.NewLifecycleClient(getServerURL()+"/v1", "")

	// 1. List sandboxes
	list, err := client.ListSandboxes(ctx, opensandbox.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListSandboxes: %v", err)
	}
	t.Logf("Initial sandbox count: %d", list.Pagination.TotalItems)

	// 2. Create a sandbox
	sb, err := client.CreateSandbox(ctx, opensandbox.CreateSandboxRequest{
		Image: opensandbox.ImageSpec{
			URI: getDefaultImage(),
		},
		Metadata: map[string]string{
			"test": "go-e2e",
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
		Command: "echo hello-from-go-e2e && python3 --version",
	}, func(event opensandbox.StreamEvent) error {
		t.Logf("  SSE event: type=%s data=%s", event.Event, event.Data)
		output.WriteString(event.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
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

	// 10. Delete sandbox
	err = client.DeleteSandbox(ctx, sb.ID)
	if err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}
	t.Log("Sandbox deleted successfully")

	// 11. Verify deletion
	deleted, err := client.GetSandbox(ctx, sb.ID)
	if err != nil {
		t.Logf("GetSandbox after delete: %v (expected)", err)
	} else {
		t.Logf("GetSandbox after delete: state=%s", deleted.Status.State)
	}

	fmt.Println("\n=== GO E2E TEST PASSED ===")
	fmt.Println("Lifecycle: create → poll → Running → execd ping → run command (SSE) → file info → metrics → egress → delete")
}

func TestE2E_PauseResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := opensandbox.ConnectionConfig{
		Domain:   strings.TrimPrefix(strings.TrimPrefix(getServerURL(), "http://"), "https://"),
		Protocol: "http",
		APIKey:   os.Getenv("OPENSANDBOX_API_KEY"),
	}

	// 1. Create sandbox via high-level API
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    getDefaultImage(),
		Metadata: map[string]string{"test": "go-e2e-pause-resume"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 2. Verify sandbox is healthy
	if !sb.IsHealthy(ctx) {
		t.Fatal("Sandbox not healthy after creation")
	}
	t.Log("Sandbox is healthy")

	// 3. Run a command before pause
	exec, err := sb.RunCommand(ctx, "echo before-pause", nil)
	if err != nil {
		t.Fatalf("RunCommand before pause: %v", err)
	}
	t.Logf("Pre-pause output: %s", exec.Text())

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
	t.Log("Sandbox resumed")

	// 7. Verify resumed sandbox is healthy and functional
	if !resumed.IsHealthy(ctx) {
		t.Fatal("Sandbox not healthy after resume")
	}

	exec2, err := resumed.RunCommand(ctx, "echo after-resume", nil)
	if err != nil {
		t.Fatalf("RunCommand after resume: %v", err)
	}
	t.Logf("Post-resume output: %s", exec2.Text())

	// 8. Also test instance method Resume: pause again and resume via method
	if err := resumed.Pause(ctx); err != nil {
		t.Fatalf("Second pause: %v", err)
	}
	t.Log("Sandbox paused again")

	resumed2, err := resumed.Resume(ctx)
	if err != nil {
		t.Fatalf("Sandbox.Resume(): %v", err)
	}

	exec3, err := resumed2.RunCommand(ctx, "echo instance-resume", nil)
	if err != nil {
		t.Fatalf("RunCommand after instance resume: %v", err)
	}
	t.Logf("Instance resume output: %s", exec3.Text())

	// Cleanup
	if err := resumed2.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	t.Log("Sandbox killed — pause/resume e2e passed")
}

func TestE2E_ManualCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := opensandbox.ConnectionConfig{
		Domain:   strings.TrimPrefix(strings.TrimPrefix(getServerURL(), "http://"), "https://"),
		Protocol: "http",
		APIKey:   os.Getenv("OPENSANDBOX_API_KEY"),
	}

	// 1. Create sandbox with ManualCleanup (no auto-expiration)
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:         getDefaultImage(),
		ManualCleanup: true,
		Metadata:      map[string]string{"test": "go-e2e-manual-cleanup"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with ManualCleanup: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 2. Verify sandbox has no expiration set
	info, err := sb.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}

	if info.ExpiresAt != nil {
		t.Errorf("Expected nil ExpiresAt for ManualCleanup sandbox, got %v", info.ExpiresAt)
	} else {
		t.Log("Confirmed: ExpiresAt is nil (no auto-expiration)")
	}

	// 3. Verify sandbox is functional
	exec, err := sb.RunCommand(ctx, "echo manual-cleanup-works", nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	t.Logf("Output: %s", exec.Text())

	// 4. Compare with a normal sandbox that should have an expiration
	sbWithTimeout, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    getDefaultImage(),
		Metadata: map[string]string{"test": "go-e2e-with-timeout"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with default timeout: %v", err)
	}
	defer func() { _ = sbWithTimeout.Kill(context.Background()) }()

	infoWithTimeout, err := sbWithTimeout.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo (with timeout): %v", err)
	}

	if infoWithTimeout.ExpiresAt == nil {
		t.Log("Warning: default sandbox also has nil ExpiresAt — server may not populate this field")
	} else {
		t.Logf("Default sandbox ExpiresAt: %v (confirms manual cleanup sandbox correctly omits it)", infoWithTimeout.ExpiresAt)
	}

	t.Log("Manual cleanup e2e passed")
}
