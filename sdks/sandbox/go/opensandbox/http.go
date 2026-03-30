package opensandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// defaultTimeout is 0 (no global timeout) because a non-zero value kills
// long-lived SSE streaming connections. Use per-request context deadlines
// instead to control individual call timeouts.
const defaultTimeout = 0

// Client is the base HTTP client shared by LifecycleClient and EgressClient.
type Client struct {
	baseURL    string
	apiKey     string
	authHeader string
	httpClient *http.Client
	timeout    *time.Duration // stored separately, applied after all options
	headers    map[string]string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) {
		cl.httpClient = c
	}
}

// WithTimeout sets the HTTP client timeout. The timeout is applied after all
// options, so it is safe to combine with WithHTTPClient in any order.
func WithTimeout(d time.Duration) Option {
	return func(cl *Client) {
		cl.timeout = &d
	}
}

// WithHeaders adds custom HTTP headers to all requests. These are applied
// before the auth and content-type headers, so they cannot override those.
func WithHeaders(headers map[string]string) Option {
	return func(cl *Client) {
		if cl.headers == nil {
			cl.headers = make(map[string]string, len(headers))
		}
		for k, v := range headers {
			cl.headers[k] = v
		}
	}
}

// WithAuthHeader overrides the default auth header name. Use this when the
// server expects a different header (e.g. "X-API-Key" instead of
// "OPEN-SANDBOX-API-KEY").
func WithAuthHeader(header string) Option {
	return func(cl *Client) {
		cl.authHeader = header
	}
}

// NewClient creates a new base Client. The authHeader parameter specifies
// which HTTP header carries the API key (e.g. "OPEN-SANDBOX-API-KEY" for
// lifecycle, "OPENSANDBOX-EGRESS-AUTH" for egress).
func NewClient(baseURL, apiKey, authHeader string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		authHeader: authHeader,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	// Apply deferred timeout after all options so it works regardless of
	// WithHTTPClient ordering and guards against a nil httpClient.
	if c.timeout != nil {
		if c.httpClient == nil {
			c.httpClient = &http.Client{}
		}
		c.httpClient.Timeout = *c.timeout
	}
	return c
}

// doRequest executes an HTTP request with JSON encoding and auth headers.
// If body is nil, no request body is sent. If result is non-nil, the
// response body is decoded into it.
func (c *Client) doRequest(ctx context.Context, method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("opensandbox: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("opensandbox: create request: %w", err)
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if c.apiKey != "" {
		req.Header.Set(c.authHeader, c.apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("opensandbox: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return handleError(resp)
	}

	// No content (e.g. 204)
	if resp.StatusCode == http.StatusNoContent || result == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("opensandbox: decode response: %w", err)
	}
	return nil
}

// handleError reads the response body and returns an *APIError.
func handleError(resp *http.Response) error {
	data, _ := io.ReadAll(resp.Body)

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("X-Request-Id"),
	}

	// Try to decode as JSON ErrorResponse; fall back to raw body.
	if err := json.Unmarshal(data, &apiErr.Response); err != nil || apiErr.Response.Code == "" {
		apiErr.Response = ErrorResponse{
			Code:    http.StatusText(resp.StatusCode),
			Message: string(data),
		}
	}
	return apiErr
}
