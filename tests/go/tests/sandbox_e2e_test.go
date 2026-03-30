//go:build e2e

package tests

import (
	"context"
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

	// Verify healthy
	if !sb.IsHealthy(ctx) {
		t.Error("Sandbox should be healthy after creation")
	}

	// GetInfo
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

	// GetMetrics
	metrics, err := sb.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if metrics.CPUCount == 0 {
		t.Error("Expected non-zero CPU count")
	}
	t.Logf("Metrics: cpu=%.0f, mem=%.0fMiB", metrics.CPUCount, metrics.MemTotalMB)

	// Kill
	if err := sb.Kill(ctx); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	t.Log("Sandbox killed successfully")
}

func TestSandbox_Renew(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb.Kill(context.Background())

	_, err = sb.Renew(ctx, 30*time.Minute)
	if err != nil {
		t.Logf("Renew: %v (may not be supported)", err)
	} else {
		t.Log("Renewed expiration: +30m")
	}
}

func TestSandbox_GetEndpoint(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb.Kill(context.Background())

	endpoint, err := sb.GetEndpoint(ctx, opensandbox.DefaultExecdPort)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if endpoint.Endpoint == "" {
		t.Error("Expected non-empty endpoint")
	}
	t.Logf("Endpoint: %s", endpoint.Endpoint)
}
