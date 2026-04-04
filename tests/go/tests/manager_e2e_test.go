//go:build e2e

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func TestManager_ListSandboxes(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	result, err := mgr.ListSandboxInfos(ctx, opensandbox.ListOptions{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListSandboxInfos: %v", err)
	}

	t.Logf("Listed %d sandboxes (page %d/%d)",
		len(result.Items), result.Pagination.Page, result.Pagination.TotalPages)
}

func TestManager_ListByState(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create a sandbox to ensure there's at least one Running
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
		Metadata: map[string]string{
			"test": "go-e2e-manager",
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb.Kill(context.Background())

	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// Filter by Running state
	result, err := mgr.ListSandboxInfos(ctx, opensandbox.ListOptions{
		States: []opensandbox.SandboxState{opensandbox.StateRunning},
	})
	if err != nil {
		t.Fatalf("ListSandboxInfos(Running): %v", err)
	}

	if len(result.Items) == 0 {
		t.Error("Expected at least one Running sandbox")
	}

	// Verify all returned sandboxes are Running
	for _, item := range result.Items {
		if item.Status.State != opensandbox.StateRunning {
			t.Errorf("Expected Running state, got %s for sandbox %s", item.Status.State, item.ID)
		}
	}
	t.Logf("Found %d Running sandboxes", len(result.Items))
}

func TestManager_GetAndKill(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create via high-level API
	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}

	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// Get via manager
	info, err := mgr.GetSandboxInfo(ctx, sb.ID())
	if err != nil {
		t.Fatalf("GetSandboxInfo: %v", err)
	}
	if info.ID != sb.ID() {
		t.Errorf("ID mismatch: got %s, want %s", info.ID, sb.ID())
	}
	t.Logf("Got sandbox %s via manager (state=%s)", info.ID, info.Status.State)

	// Kill via manager
	if err := mgr.KillSandbox(ctx, sb.ID()); err != nil {
		t.Fatalf("KillSandbox: %v", err)
	}
	t.Log("Killed sandbox via manager")
}
