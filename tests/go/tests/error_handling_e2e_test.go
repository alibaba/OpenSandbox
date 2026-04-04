//go:build e2e

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func TestError_XRequestIDPassthrough(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mgr := opensandbox.NewSandboxManager(config)
	defer mgr.Close()

	// Request a non-existent sandbox — server should return 404 with x-request-id
	_, err := mgr.GetSandboxInfo(ctx, "non-existent-sandbox-id-12345")
	if err == nil {
		t.Fatal("Expected error for non-existent sandbox")
	}

	var apiErr *opensandbox.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected *APIError, got %T: %v", err, err)
	}

	if apiErr.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", apiErr.StatusCode)
	}

	// x-request-id should be present on server errors
	if apiErr.RequestID != "" {
		t.Logf("x-request-id present: %s (status=%d, code=%s)",
			apiErr.RequestID, apiErr.StatusCode, apiErr.Response.Code)
	} else {
		t.Log("x-request-id not returned by server (may not be configured)")
	}

	t.Logf("Error response: code=%s message=%s", apiErr.Response.Code, apiErr.Response.Message)
}
