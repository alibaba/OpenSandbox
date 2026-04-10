package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrent_CreateFiveSandboxes(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	const count = 5
	var wg sync.WaitGroup
	sandboxes := make([]*opensandbox.Sandbox, count)
	errors := make([]error, count)

	t.Logf("Creating %d sandboxes concurrently...", count)
	start := time.Now()

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
				Image: getSandboxImage(),
				Metadata: map[string]string{
					"test":  "go-e2e-concurrent",
					"index": fmt.Sprintf("%d", idx),
				},
			})
			sandboxes[idx] = sb
			errors[idx] = err
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	defer func() {
		for _, sb := range sandboxes {
			if sb != nil {
				sb.Kill(context.Background())
			}
		}
	}()

	succeeded := 0
	for i := 0; i < count; i++ {
		if errors[i] != nil {
			t.Logf("Sandbox %d failed: %v", i, errors[i])
		} else {
			succeeded++
			t.Logf("Sandbox %d: %s (healthy=%v)", i, sandboxes[i].ID(), sandboxes[i].IsHealthy(ctx))
		}
	}

	t.Logf("Created %d/%d sandboxes in %s", succeeded, count, elapsed.Round(time.Millisecond))
	minRequired := 3
	require.GreaterOrEqual(t, succeeded, minRequired,
		"expected at least %d/%d sandboxes to succeed, only %d did", minRequired, count, succeeded)

	var cmdWg sync.WaitGroup
	for i := 0; i < count; i++ {
		if sandboxes[i] == nil {
			continue
		}
		cmdWg.Add(1)
		go func(idx int) {
			defer cmdWg.Done()
			exec, err := sandboxes[idx].RunCommand(ctx, fmt.Sprintf("echo sandbox-%d", idx), nil)
			if !assert.NoError(t, err, "command on sandbox %d", idx) {
				return
			}
			t.Logf("Sandbox %d output: %s", idx, exec.Text())
		}(i)
	}
	cmdWg.Wait()
	t.Log("All concurrent sandboxes created, verified, and responding independently")
}
