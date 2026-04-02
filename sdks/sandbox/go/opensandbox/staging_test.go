//go:build staging

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

// TestStaging_FullLifecycle tests the Go SDK against the arpi staging server.
// The staging server differs from local OpenSandbox:
//   - No /v1/ prefix (routes at /sandboxes directly)
//   - Auth header: X-API-Key (not OPEN-SANDBOX-API-KEY)
//   - Proxy endpoints use the staging domain, not host.docker.internal
//
// Run: STAGING_URL=https://your-server STAGING_API_KEY=your-key go test -tags staging -run TestStaging -v -timeout 3m
func TestStaging_FullLifecycle(t *testing.T) {
	stagingURL := os.Getenv("STAGING_URL")
	if stagingURL == "" {
		t.Fatal("STAGING_URL must be set for staging tests")
	}
	apiKey := os.Getenv("STAGING_API_KEY")
	if apiKey == "" {
		t.Fatal("STAGING_API_KEY must be set for staging tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Staging uses X-API-Key and no /v1/ prefix
	client := opensandbox.NewLifecycleClient(stagingURL, apiKey,
		opensandbox.WithAuthHeader("X-API-Key"))

	// 1. List sandboxes
	list, err := client.ListSandboxes(ctx, opensandbox.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListSandboxes: %v", err)
	}
	t.Logf("Initial sandbox count: %d", list.Pagination.TotalItems)

	// 2. Create sandbox
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
			"test": "staging-go-sdk",
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

	// 3. Wait for Running
	var running *opensandbox.SandboxInfo
	for i := 0; i < 60; i++ {
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

	// 4. Get execd endpoint (use server proxy — pod IPs aren't reachable externally)
	useProxy := true
	endpoint, err := client.GetEndpoint(ctx, sb.ID, 44772, &useProxy)
	if err != nil {
		t.Fatalf("GetEndpoint(44772): %v", err)
	}
	t.Logf("Execd endpoint: %s", endpoint.Endpoint)

	// Normalize URL
	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = "https://" + execdURL
	}
	t.Logf("Normalized execd URL: %s", execdURL)

	// 5. Execd ping — proxy requires same API key as lifecycle
	execToken := apiKey
	if endpoint.Headers != nil {
		if v, ok := endpoint.Headers["X-EXECD-ACCESS-TOKEN"]; ok {
			execToken = v
		}
	}
	execClient := opensandbox.NewExecdClient(execdURL, execToken,
		opensandbox.WithAuthHeader("X-API-Key"))

	if err := execClient.Ping(ctx); err != nil {
		t.Fatalf("Execd Ping: %v", err)
	}
	t.Log("Execd ping: OK")

	// 6. Run command with SSE
	var output strings.Builder
	err = execClient.RunCommand(ctx, opensandbox.RunCommandRequest{
		Command: "echo hello-from-staging && uname -a",
	}, func(event opensandbox.StreamEvent) error {
		t.Logf("  SSE event: type=%s data=%s", event.Event, event.Data)
		output.WriteString(event.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	t.Logf("Command raw output (%d bytes): %q", output.Len(), output.String())

	if output.Len() == 0 {
		t.Error("Expected non-empty command output")
	}

	// 7. File info
	fileInfoMap, err := execClient.GetFileInfo(ctx, "/etc/os-release")
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}
	for path, fi := range fileInfoMap {
		t.Logf("File info: path=%s size=%d", path, fi.Size)
	}

	// 8. Metrics
	metrics, err := execClient.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	t.Logf("Metrics: cpu_count=%.0f mem_total=%.0fMiB", metrics.CPUCount, metrics.MemTotalMB)

	// 9. Egress (may not be available)
	egressEndpoint, err := client.GetEndpoint(ctx, sb.ID, 18080, &useProxy)
	if err != nil {
		t.Logf("GetEndpoint(egress/18080): %v (skipping egress tests)", err)
	} else {
		egressURL := egressEndpoint.Endpoint
		if !strings.HasPrefix(egressURL, "http") {
			egressURL = "https://" + egressURL
		}
		egressToken := ""
		if egressEndpoint.Headers != nil {
			egressToken = egressEndpoint.Headers["OPENSANDBOX-EGRESS-AUTH"]
		}
		egressClient := opensandbox.NewEgressClient(egressURL, egressToken)
		policy, err := egressClient.GetPolicy(ctx)
		if err != nil {
			t.Logf("GetPolicy: %v (egress sidecar may not be ready)", err)
		} else {
			t.Logf("Egress policy: mode=%s rules=%d", policy.Mode, len(policy.Policy.Egress))
		}
	}

	// 10. Delete
	if err := client.DeleteSandbox(ctx, sb.ID); err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}
	t.Log("Sandbox deleted")

	// 11. Verify deletion
	_, err = client.GetSandbox(ctx, sb.ID)
	if err != nil {
		t.Logf("GetSandbox after delete: %v (expected)", err)
	}

	fmt.Println("\n=== STAGING INTEGRATION TEST PASSED ===")
	fmt.Println("Full lifecycle on remote staging: create → poll → execd ping → run command (SSE) → file info → metrics → delete")
}

// stagingConfig returns a ConnectionConfig for the staging server using the
// high-level API (CreateSandbox, ResumeSandbox, etc.).
func stagingConfig(t *testing.T) opensandbox.ConnectionConfig {
	t.Helper()
	stagingURL := os.Getenv("STAGING_URL")
	if stagingURL == "" {
		t.Fatal("STAGING_URL must be set")
	}
	apiKey := os.Getenv("STAGING_API_KEY")
	if apiKey == "" {
		t.Fatal("STAGING_API_KEY must be set")
	}
	// Staging uses X-API-Key, HTTPS, no /v1/ prefix, and server proxy.
	domain := strings.TrimPrefix(strings.TrimPrefix(stagingURL, "https://"), "http://")
	return opensandbox.ConnectionConfig{
		Domain:         domain,
		Protocol:       "https",
		APIKey:         apiKey,
		AuthHeader:     "X-API-Key",
		UseServerProxy: true,
	}
}

// TestStaging_PauseResume exercises the pause → resume flow on staging.
// The staging k8s runtime may not support pause — in that case, this test
// verifies the SDK correctly surfaces the API error with proper typing.
// Full pause/resume is covered by TestIntegration_PauseResume (Docker runtime).
func TestStaging_PauseResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := stagingConfig(t)

	// 1. Create sandbox
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "staging-pause-resume"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 2. Verify healthy and run command
	if !sb.IsHealthy(ctx) {
		t.Fatal("Sandbox not healthy after creation")
	}
	exec1, err := sb.RunCommand(ctx, "echo before-pause", nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	t.Logf("Pre-pause output: %s", exec1.Text())

	// 3. Attempt pause — may fail on k8s runtime
	pauseErr := sb.Pause(ctx)
	if pauseErr != nil {
		// Verify the error is a proper APIError with the right code
		apiErr, ok := pauseErr.(*opensandbox.APIError)
		if !ok {
			t.Fatalf("Expected *APIError from unsupported Pause, got %T: %v", pauseErr, pauseErr)
		}
		if apiErr.StatusCode != 501 {
			t.Fatalf("Expected 501 for unsupported Pause, got %d: %s", apiErr.StatusCode, apiErr.Error())
		}
		t.Logf("Pause correctly returned 501 (not supported on this runtime): %s", apiErr.Error())
		t.Log("Pause/resume not supported on staging k8s — verified error handling. Full flow covered by integration tests (Docker runtime).")
		return
	}

	// If pause succeeded (runtime supports it), exercise full flow
	t.Log("Sandbox paused")

	info, err := sb.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo after pause: %v", err)
	}
	if info.Status.State != opensandbox.StatePaused {
		t.Fatalf("Expected Paused state, got %s", info.Status.State)
	}

	resumed, err := opensandbox.ResumeSandbox(ctx, config, sb.ID())
	if err != nil {
		t.Fatalf("ResumeSandbox: %v", err)
	}
	t.Log("Sandbox resumed")

	if !resumed.IsHealthy(ctx) {
		t.Fatal("Not healthy after resume")
	}

	exec2, err := resumed.RunCommand(ctx, "echo after-resume", nil)
	if err != nil {
		t.Fatalf("RunCommand after resume: %v", err)
	}
	t.Logf("Post-resume output: %s", exec2.Text())

	if err := resumed.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	t.Log("Pause/resume staging test passed")
}

// TestStaging_ManualCleanup verifies that ManualCleanup creates a sandbox with
// no auto-expiration (ExpiresAt is nil).
func TestStaging_ManualCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := stagingConfig(t)

	// 1. Create sandbox with ManualCleanup
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:         "python:3.11-slim",
		ManualCleanup: true,
		Metadata:      map[string]string{"test": "staging-manual-cleanup"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with ManualCleanup: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())
	defer func() { _ = sb.Kill(context.Background()) }()

	// 2. Verify sandbox has no expiration
	info, err := sb.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.ExpiresAt != nil {
		t.Errorf("Expected nil ExpiresAt for ManualCleanup, got %v", info.ExpiresAt)
	} else {
		t.Log("Confirmed: ExpiresAt is nil (no auto-expiration)")
	}

	// 3. Verify sandbox is functional
	exec, err := sb.RunCommand(ctx, "echo manual-cleanup-works", nil)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	t.Logf("Output: %s", exec.Text())

	// 4. Create a normal sandbox for comparison
	sbNormal, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "staging-with-timeout"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox with default timeout: %v", err)
	}
	defer func() { _ = sbNormal.Kill(context.Background()) }()

	infoNormal, err := sbNormal.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo (normal): %v", err)
	}
	if infoNormal.ExpiresAt == nil {
		t.Log("Note: normal sandbox also has nil ExpiresAt — server may not populate this field")
	} else {
		t.Logf("Normal sandbox ExpiresAt: %v (confirms manual cleanup omission is working)", infoNormal.ExpiresAt)
	}

	// 5. Cleanup
	if err := sb.Kill(ctx); err != nil {
		t.Logf("Kill manual-cleanup sandbox: %v", err)
	}
	t.Log("Manual cleanup staging test passed")
}

// TestStaging_Manager exercises the SandboxManager on the staging server.
// Creates 2 sandboxes with different metadata, lists with filter, kills via manager.
func TestStaging_Manager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := stagingConfig(t)
	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// 1. Create two sandboxes with different metadata
	sb1, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"team": "alpha", "test": "staging-manager"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox 1: %v", err)
	}
	t.Logf("Created sandbox 1: %s", sb1.ID())
	defer func() { _ = sb1.Kill(context.Background()) }()

	sb2, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"team": "beta", "test": "staging-manager"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox 2: %v", err)
	}
	t.Logf("Created sandbox 2: %s", sb2.ID())
	defer func() { _ = sb2.Kill(context.Background()) }()

	// 2. List all with test=staging-manager metadata
	list, err := mgr.ListSandboxInfos(ctx, opensandbox.ListOptions{
		Metadata: map[string]string{"test": "staging-manager"},
		Page:     1,
		PageSize: 50,
	})
	if err != nil {
		t.Fatalf("ListSandboxInfos: %v", err)
	}
	t.Logf("Listed sandboxes with test=staging-manager: %d items", len(list.Items))
	if len(list.Items) < 2 {
		t.Errorf("expected at least 2 sandboxes, got %d", len(list.Items))
	}

	// 3. Get info for specific sandbox
	info, err := mgr.GetSandboxInfo(ctx, sb1.ID())
	if err != nil {
		t.Fatalf("GetSandboxInfo: %v", err)
	}
	t.Logf("GetSandboxInfo: id=%s state=%s", info.ID, info.Status.State)

	// 4. Kill sandbox 1 via manager
	if err := mgr.KillSandbox(ctx, sb1.ID()); err != nil {
		t.Fatalf("KillSandbox: %v", err)
	}
	t.Logf("Killed sandbox 1 via manager: %s", sb1.ID())

	// 5. Verify killed
	infoAfter, err := mgr.GetSandboxInfo(ctx, sb1.ID())
	if err != nil {
		t.Logf("GetSandboxInfo after kill: %v (expected)", err)
	} else {
		if infoAfter.Status.State == opensandbox.StateRunning {
			t.Errorf("expected non-running state after kill, got %s", infoAfter.Status.State)
		}
	}

	// 6. Clean up sandbox 2
	if err := mgr.KillSandbox(ctx, sb2.ID()); err != nil {
		t.Fatalf("KillSandbox 2: %v", err)
	}
	t.Log("Manager staging test passed")
}

// TestStaging_VolumeMounts exercises volume mounts on the staging k8s cluster.
// The PVC "go-sdk-e2e-pvc" must exist in the sandbox namespace.
func TestStaging_VolumeMounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := stagingConfig(t)

	// PVC read-write
	t.Run("PVCReadWrite", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "staging-vol-pvc-rw"},
			Volumes: []opensandbox.Volume{
				{
					Name:      "pvc-rw",
					PVC:       &opensandbox.PVC{ClaimName: "go-sdk-e2e-pvc"},
					MountPath: "/mnt/pvc",
				},
			},
		})
		if err != nil {
			t.Fatalf("CreateSandbox with PVC: %v", err)
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		// Write and read back
		exec, err := sb.RunCommand(ctx, "echo 'pvc-staging-test' > /mnt/pvc/go-sdk-test.txt && cat /mnt/pvc/go-sdk-test.txt", nil)
		if err != nil {
			t.Fatalf("RunCommand (pvc write/read): %v", err)
		}
		t.Logf("PVC rw output: %q", exec.Text())
		if !strings.Contains(exec.Text(), "pvc-staging-test") {
			t.Errorf("expected 'pvc-staging-test' in output, got %q", exec.Text())
		}

		// Cleanup written file
		sb.RunCommand(ctx, "rm -f /mnt/pvc/go-sdk-test.txt", nil)
		t.Log("PVC read-write staging test passed")
	})

	// PVC read-only
	t.Run("PVCReadOnly", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "staging-vol-pvc-ro"},
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
			t.Fatalf("CreateSandbox with readonly PVC: %v", err)
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		// Read should work
		exec, err := sb.RunCommand(ctx, "ls /mnt/pvc-ro 2>&1; echo exit=$?", nil)
		if err != nil {
			t.Fatalf("RunCommand (ls readonly): %v", err)
		}
		t.Logf("Readonly PVC ls: %q", exec.Text())

		// Write should fail
		execW, err := sb.RunCommand(ctx, "touch /mnt/pvc-ro/fail.txt 2>&1; echo exit=$?", nil)
		if err != nil {
			t.Fatalf("RunCommand (write readonly PVC): %v", err)
		}
		t.Logf("Readonly PVC write attempt: %q", execW.Text())
		if strings.Contains(execW.Text(), "exit=0") && !strings.Contains(strings.ToLower(execW.Text()), "read-only") {
			t.Error("write to readonly PVC mount should have failed")
		}

		t.Log("PVC read-only staging test passed")
	})

	// PVC with subPath
	t.Run("PVCSubPath", func(t *testing.T) {
		sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
			Image:    "python:3.11-slim",
			Metadata: map[string]string{"test": "staging-vol-pvc-sub"},
			Volumes: []opensandbox.Volume{
				{
					Name:      "pvc-sub",
					PVC:       &opensandbox.PVC{ClaimName: "go-sdk-e2e-pvc"},
					MountPath: "/mnt/sub",
					SubPath:   "go-sdk-subdir",
				},
			},
		})
		if err != nil {
			t.Fatalf("CreateSandbox with PVC subPath: %v", err)
		}
		defer func() { _ = sb.Kill(context.Background()) }()

		exec, err := sb.RunCommand(ctx, "echo 'subpath-staging' > /mnt/sub/test.txt && cat /mnt/sub/test.txt", nil)
		if err != nil {
			t.Fatalf("RunCommand (subpath): %v", err)
		}
		t.Logf("SubPath output: %q", exec.Text())
		if !strings.Contains(exec.Text(), "subpath-staging") {
			t.Errorf("expected 'subpath-staging' in output, got %q", exec.Text())
		}

		sb.RunCommand(ctx, "rm -f /mnt/sub/test.txt", nil)
		t.Log("PVC subPath staging test passed")
	})
}

// TestStaging_NetworkPolicy exercises egress network policy on staging.
func TestStaging_NetworkPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := stagingConfig(t)

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "staging-network-policy"},
		NetworkPolicy: &opensandbox.NetworkPolicy{
			DefaultAction: "deny",
			Egress: []opensandbox.NetworkRule{
				{Action: "allow", Target: "api.example.com"},
			},
		},
	})
	if err != nil {
		// Server may require egress sidecar image config to be set
		t.Logf("CreateSandbox with NetworkPolicy: %v", err)
		t.Skip("NetworkPolicy not available on this server (egress sidecar image may not be configured)")
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	// Get policy
	policy, err := sb.GetEgressPolicy(ctx)
	if err != nil {
		t.Logf("GetEgressPolicy: %v (egress sidecar may not be available)", err)
		t.Skip("Egress sidecar not available on staging")
	}
	t.Logf("Policy: mode=%s status=%s", policy.Mode, policy.Status)
	if policy.Policy != nil {
		t.Logf("Rules: %d, defaultAction=%s", len(policy.Policy.Egress), policy.Policy.DefaultAction)
	}

	// Patch
	patched, err := sb.PatchEgressRules(ctx, []opensandbox.NetworkRule{
		{Action: "allow", Target: "cdn.example.com"},
	})
	if err != nil {
		t.Fatalf("PatchEgressRules: %v", err)
	}
	if patched.Policy != nil && len(patched.Policy.Egress) < 2 {
		t.Errorf("expected at least 2 rules after patch, got %d", len(patched.Policy.Egress))
	}
	t.Logf("After patch: %d rules", len(patched.Policy.Egress))

	t.Log("Network policy staging test passed")
}

// TestStaging_CommandWithEnvs verifies env var injection on staging.
func TestStaging_CommandWithEnvs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := stagingConfig(t)

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:    "python:3.11-slim",
		Metadata: map[string]string{"test": "staging-cmd-envs"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer func() { _ = sb.Kill(context.Background()) }()

	exec, err := sb.RunCommandWithOpts(ctx, opensandbox.RunCommandRequest{
		Command: "echo $SDK_TEST_VAR",
		Envs:    map[string]string{"SDK_TEST_VAR": "staging-env-works"},
	}, nil)
	if err != nil {
		t.Fatalf("RunCommandWithOpts: %v", err)
	}
	t.Logf("Output: %q", exec.Text())
	if !strings.Contains(exec.Text(), "staging-env-works") {
		t.Errorf("expected 'staging-env-works' in output, got %q", exec.Text())
	}

	t.Log("Command with env vars staging test passed")
}
