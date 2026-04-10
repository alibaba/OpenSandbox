package tests

import (
	"context"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

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
	require.NoError(t, err)
	defer sb.Kill(context.Background())

	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// Filter by Running state
	result, err := mgr.ListSandboxInfos(ctx, opensandbox.ListOptions{
		States: []opensandbox.SandboxState{opensandbox.StateRunning},
	})
	require.NoError(t, err)

	require.NotEmpty(t, result.Items, "expected at least one Running sandbox")

	// Verify all returned sandboxes are Running
	for _, item := range result.Items {
		require.Equal(t, opensandbox.StateRunning, item.Status.State, "sandbox %s", item.ID)
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
	require.NoError(t, err)

	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// Get via manager
	info, err := mgr.GetSandboxInfo(ctx, sb.ID())
	require.NoError(t, err)
	require.Equal(t, sb.ID(), info.ID)
	t.Logf("Got sandbox %s via manager (state=%s)", info.ID, info.Status.State)

	// Kill via manager
	require.NoError(t, mgr.KillSandbox(ctx, sb.ID()))
	t.Log("Killed sandbox via manager")
}
