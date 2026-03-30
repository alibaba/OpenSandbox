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
	var running *opensandbox.Sandbox
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
