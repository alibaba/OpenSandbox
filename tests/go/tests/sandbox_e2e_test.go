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

	// Create first
	sb1, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb1.Kill(context.Background())

	// Connect to the same sandbox by ID
	sb2, err := opensandbox.ConnectSandbox(ctx, config, sb1.ID(), opensandbox.ReadyOptions{})
	if err != nil {
		t.Fatalf("ConnectSandbox: %v", err)
	}

	// Verify it's the same sandbox
	if sb2.ID() != sb1.ID() {
		t.Errorf("IDs should match: %s vs %s", sb1.ID(), sb2.ID())
	}

	// Verify it works
	exec, err := sb2.RunCommand(ctx, "echo connected", nil)
	if err != nil {
		t.Fatalf("RunCommand via connected sandbox: %v", err)
	}
	if !strings.Contains(exec.Text(), "connected") {
		t.Errorf("Expected 'connected' in output, got: %q", exec.Text())
	}
	t.Log("ConnectSandbox works — ran command on connected instance")
}

func TestSandbox_Session(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Create session
	session, err := sb.CreateSession(ctx)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID == "" {
		t.Fatal("Session ID is empty")
	}
	t.Logf("Created session: %s", session.ID)

	// Run in session — set variable
	exec, err := sb.RunInSession(ctx, session.ID, opensandbox.RunInSessionRequest{
		Command: "export MY_VAR=hello_session",
	}, nil)
	if err != nil {
		t.Fatalf("RunInSession (set var): %v", err)
	}
	_ = exec

	// Run in session — read variable back (state should persist)
	exec, err = sb.RunInSession(ctx, session.ID, opensandbox.RunInSessionRequest{
		Command: "echo $MY_VAR",
	}, nil)
	if err != nil {
		t.Fatalf("RunInSession (read var): %v", err)
	}
	if !strings.Contains(exec.Text(), "hello_session") {
		t.Errorf("Session state not preserved, got: %q", exec.Text())
	}
	t.Log("Session state persists across commands")

	// Delete session
	err = sb.DeleteSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	t.Log("Session deleted")
}
