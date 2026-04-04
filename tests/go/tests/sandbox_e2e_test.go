//go:build e2e

package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func TestSandbox_CreateAndKill(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:      getSandboxImage(),
		Entrypoint: []string{"tail", "-f", "/dev/null"},
		ResourceLimits: opensandbox.ResourceLimits{
			"cpu":    "500m",
			"memory": "256Mi",
		},
		Metadata: map[string]string{
			"test": "go-e2e-create",
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", sb.ID())

	defer func() {
		if err := sb.Kill(context.Background()); err != nil {
			t.Logf("Kill cleanup: %v", err)
		}
	}()

	if !sb.IsHealthy(ctx) {
		t.Error("Sandbox should be healthy after creation")
	}

	info, err := sb.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.ID != sb.ID() {
		t.Errorf("ID mismatch: got %s, want %s", info.ID, sb.ID())
	}
	if info.Status.State != opensandbox.StateRunning {
		t.Errorf("Expected Running state, got %s", info.Status.State)
	}
	t.Logf("Info: state=%s, created=%s", info.Status.State, info.CreatedAt)

	metrics, err := sb.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if metrics.CPUCount == 0 {
		t.Error("Expected non-zero CPU count")
	}
	t.Logf("Metrics: cpu=%.0f, mem=%.0fMiB", metrics.CPUCount, metrics.MemTotalMB)

	if err := sb.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	t.Log("Sandbox killed successfully")
}

func TestSandbox_Renew(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	_, err := sb.Renew(ctx, 30*time.Minute)
	if err != nil {
		t.Logf("Renew: %v (may not be supported)", err)
	} else {
		t.Log("Renewed expiration: +30m")
	}
}

func TestSandbox_GetEndpoint(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	endpoint, err := sb.GetEndpoint(ctx, opensandbox.DefaultExecdPort)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if endpoint.Endpoint == "" {
		t.Error("Expected non-empty endpoint")
	}
	t.Logf("Endpoint: %s", endpoint.Endpoint)
}

func TestSandbox_ConnectToExisting(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sb1, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb1.Kill(context.Background())

	sb2, err := opensandbox.ConnectSandbox(ctx, config, sb1.ID(), opensandbox.ReadyOptions{})
	if err != nil {
		t.Fatalf("ConnectSandbox: %v", err)
	}

	if sb2.ID() != sb1.ID() {
		t.Errorf("IDs should match: %s vs %s", sb1.ID(), sb2.ID())
	}

	exec, err := sb2.RunCommand(ctx, "echo connected", nil)
	if err != nil {
		t.Fatalf("RunCommand via connected sandbox: %v", err)
	}
	if !strings.Contains(exec.Text(), "connected") {
		t.Errorf("Expected 'connected' in output, got: %q", exec.Text())
	}
	t.Log("ConnectSandbox works")
}

func TestSandbox_Session(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	session, err := sb.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Logf("Created session: %s", session.ID)

	sb.RunInSession(ctx, session.ID, opensandbox.RunInSessionRequest{
		Command: "export MY_VAR=hello_session",
	}, nil)

	exec, err := sb.RunInSession(ctx, session.ID, opensandbox.RunInSessionRequest{
		Command: "echo $MY_VAR",
	}, nil)
	if err != nil {
		t.Fatalf("RunInSession (read var): %v", err)
	}
	if !strings.Contains(exec.Text(), "hello_session") {
		t.Errorf("Session state not preserved, got: %q", exec.Text())
	}
	t.Log("Session state persists across commands")

	err = sb.DeleteSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	t.Log("Session deleted")
}

func TestSandbox_ManualCleanup(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create with no timeout (manual cleanup)
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb.Kill(context.Background())

	info, err := sb.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	t.Logf("Sandbox %s created (expiresAt=%v)", info.ID, info.ExpiresAt)
}

func TestSandbox_NetworkPolicyCreate(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
		NetworkPolicy: &opensandbox.NetworkPolicy{
			DefaultAction: "deny",
			Egress: []opensandbox.NetworkRule{
				{Action: "allow", Target: "pypi.org"},
				{Action: "allow", Target: "*.python.org"},
			},
		},
	})
	if err != nil {
		// Server may require egress.image config for network policies
		t.Skipf("CreateSandbox with NetworkPolicy: %v (egress sidecar may not be configured)", err)
	}
	defer sb.Kill(context.Background())

	// Verify sandbox is running
	if !sb.IsHealthy(ctx) {
		t.Error("Sandbox with network policy should be healthy")
	}
	t.Log("Sandbox created with deny-default network policy + 2 allow rules")
}

func TestSandbox_EgressPolicyGetAndPatch(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Get current policy
	policy, err := sb.GetEgressPolicy(ctx)
	if err != nil {
		t.Logf("GetEgressPolicy: %v (egress sidecar may not be available)", err)
		t.Skip("Egress sidecar not available")
	}
	t.Logf("Current policy: mode=%s", policy.Mode)

	// Patch with new rule
	patched, err := sb.PatchEgressRules(ctx, []opensandbox.NetworkRule{
		{Action: "allow", Target: "example.com"},
	})
	if err != nil {
		t.Fatalf("PatchEgressRules: %v", err)
	}
	t.Logf("Patched policy: mode=%s", patched.Mode)
}

func TestSandbox_PauseAndResume(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb.Kill(context.Background())

	// Pause
	err = sb.Pause(ctx)
	if err != nil {
		t.Logf("Pause: %v (may not be supported by runtime)", err)
		t.Skip("Pause not supported")
	}
	t.Log("Pause requested")

	// Poll until Paused
	for i := 0; i < 30; i++ {
		info, err := sb.GetInfo(ctx)
		if err != nil {
			t.Fatalf("GetInfo during pause: %v", err)
		}
		t.Logf("  Poll %d: state=%s", i+1, info.Status.State)
		if info.Status.State == opensandbox.StatePaused {
			t.Log("Sandbox is Paused")
			break
		}
		if info.Status.State == opensandbox.StateFailed {
			t.Fatalf("Sandbox failed: %s", info.Status.Reason)
		}
		time.Sleep(2 * time.Second)
	}

	// Resume — need to use manager since Sandbox doesn't have Resume yet
	mgr := opensandbox.NewSandboxManager(config)
	err = mgr.ResumeSandbox(ctx, sb.ID())
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	t.Log("Resume requested")

	// Poll until Running again
	for i := 0; i < 30; i++ {
		info, err := sb.GetInfo(ctx)
		if err != nil {
			t.Fatalf("GetInfo during resume: %v", err)
		}
		t.Logf("  Poll %d: state=%s", i+1, info.Status.State)
		if info.Status.State == opensandbox.StateRunning {
			t.Log("Sandbox is Running again after resume")
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("Sandbox did not resume to Running state")
}
