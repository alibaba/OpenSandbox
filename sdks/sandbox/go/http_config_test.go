package opensandbox

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingReadCloser struct{}

func (f failingReadCloser) Read(_ []byte) (int, error) { return 0, errors.New("boom") }
func (f failingReadCloser) Close() error               { return nil }

func TestHandleError_BodyReadFailure(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     make(http.Header),
		Body:       failingReadCloser{},
	}
	resp.Header.Set("X-Request-Id", "req-read-fail")

	err := handleError(resp)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "expected *APIError, got %T", err)
	require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	assert.Contains(t, apiErr.Response.Message, "failed to read error response body")
}

func TestRewriteEndpointURL_ReplacesAllMatches(t *testing.T) {
	cfg := ConnectionConfig{
		EndpointHostRewrite: map[string]string{
			"host.docker.internal": "localhost",
		},
	}

	in := "http://host.docker.internal/a/http://host.docker.internal/b"
	got := cfg.RewriteEndpointURL(in)
	want := "http://localhost/a/http://localhost/b"
	require.Equal(t, want, got)
}

func TestDefaultRetryConfig_HasRetryableStatusCodes(t *testing.T) {
	cfg := DefaultRetryConfig()
	require.NotEmpty(t, cfg.RetryableStatusCodes, "DefaultRetryConfig should include retryable status codes")
}

func TestOctalMode_DoesNotPanic(t *testing.T) {
	require.Equal(t, 755, OctalMode(0o755))
}

func TestHandleError_JSONBodyStillParsed(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"code":"RATE_LIMIT","message":"slow down"}`)),
	}

	err := handleError(resp)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "expected *APIError, got %T", err)
	require.Equal(t, "RATE_LIMIT", apiErr.Response.Code)
}
