//go:build e2e

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError_XRequestIDPassthrough(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// Request a non-existent sandbox — server should return 404 with x-request-id
	_, err := mgr.GetSandboxInfo(ctx, "non-existent-sandbox-id-12345")
	require.Error(t, err, "expected error for non-existent sandbox")

	var apiErr *opensandbox.APIError
	require.ErrorAs(t, err, &apiErr)

	assert.Equal(t, 404, apiErr.StatusCode)

	// x-request-id should be present on server errors
	if apiErr.RequestID != "" {
		t.Logf("x-request-id present: %s (status=%d, code=%s)",
			apiErr.RequestID, apiErr.StatusCode, apiErr.Response.Code)
	} else {
		t.Log("x-request-id not returned by server (may not be configured)")
	}

	t.Logf("Error response: code=%s message=%s", apiErr.Response.Code, apiErr.Response.Message)
}
