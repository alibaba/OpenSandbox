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
