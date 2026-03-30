//go:build e2e

package tests

import (
	"os"
	"testing"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
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

	// Override auth header if using server proxy (staging setups use X-API-Key)
	if useProxy {
		config.AuthHeader = "X-API-Key"
	}

	return config
}

func getSandboxImage() string {
	if img := os.Getenv("OPENSANDBOX_SANDBOX_DEFAULT_IMAGE"); img != "" {
		return img
	}
	return "python:3.11-slim"
}
