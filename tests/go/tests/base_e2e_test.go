package tests

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
	"github.com/stretchr/testify/require"
)

func getConnectionConfig(t *testing.T) opensandbox.ConnectionConfig {
	t.Helper()

	domain := os.Getenv("OPENSANDBOX_TEST_DOMAIN")
	if domain == "" {
		domain = "localhost:8080"
	}

	protocol := os.Getenv("OPENSANDBOX_TEST_PROTOCOL")
	if protocol == "" {
		protocol = "http"
	}

	apiKey := os.Getenv("OPENSANDBOX_TEST_API_KEY")
	if apiKey == "" {
		apiKey = "e2e-test"
	}

	useProxy := os.Getenv("OPENSANDBOX_TEST_USE_SERVER_PROXY") == "true"

	config := opensandbox.ConnectionConfig{
		Domain:         domain,
		Protocol:       protocol,
		APIKey:         apiKey,
		UseServerProxy: useProxy,
	}

	if useProxy {
		config.AuthHeader = "X-API-Key"
	}

	return config
}

func connectionConfigForStreaming(t *testing.T) opensandbox.ConnectionConfig {
	t.Helper()
	c := getConnectionConfig(t)
	c.RequestTimeout = 3 * time.Minute
	return c
}

func getSandboxImage() string {
	if img := os.Getenv("OPENSANDBOX_SANDBOX_DEFAULT_IMAGE"); img != "" {
		return img
	}
	return "python:3.11-slim"
}

func createTestSandbox(t *testing.T) (context.Context, *opensandbox.Sandbox) {
	t.Helper()
	config := connectionConfigForStreaming(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { sb.Kill(context.Background()) })
	return ctx, sb
}

func newExecdClientForSandbox(t *testing.T, ctx context.Context, sb *opensandbox.Sandbox) *opensandbox.ExecdClient {
	t.Helper()

	endpoint, err := sb.GetEndpoint(ctx, opensandbox.DefaultExecdPort)
	require.NoError(t, err)
	require.NotEmpty(t, endpoint.Endpoint)

	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = "http://" + execdURL
	}
	execdURL = strings.Replace(execdURL, "host.docker.internal", "localhost", 1)

	token := ""
	if endpoint.Headers != nil {
		token = endpoint.Headers["X-EXECD-ACCESS-TOKEN"]
	}
	return opensandbox.NewExecdClient(execdURL, token)
}
