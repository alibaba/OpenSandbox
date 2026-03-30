package opensandbox

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ConnectionConfig holds the configuration for connecting to an OpenSandbox server.
type ConnectionConfig struct {
	// Domain is the server address (e.g. "localhost:8080").
	// Falls back to OPEN_SANDBOX_DOMAIN env var, then DefaultDomain.
	Domain string

	// Protocol is "http" or "https".
	// Falls back to OPEN_SANDBOX_PROTOCOL env var, then DefaultProtocol.
	Protocol string

	// APIKey is the authentication token.
	// Falls back to OPEN_SANDBOX_API_KEY env var.
	APIKey string

	// UseServerProxy routes execd/egress requests through the sandbox server
	// instead of connecting directly to the sandbox endpoint.
	UseServerProxy bool

	// RequestTimeout is the timeout for non-streaming HTTP requests.
	// Zero means no timeout. Defaults to DefaultRequestTimeout.
	RequestTimeout time.Duration

	// Headers are custom HTTP headers added to all requests.
	Headers map[string]string

	// HTTPClient is an optional custom HTTP client. If nil, a default is created.
	HTTPClient *http.Client

	// AuthHeader overrides the default lifecycle auth header name.
	// Default is "OPEN-SANDBOX-API-KEY". Use "X-API-Key" for proxied deployments.
	AuthHeader string
}

// GetDomain returns the configured domain, falling back to env var and default.
func (c *ConnectionConfig) GetDomain() string {
	if c.Domain != "" {
		return c.Domain
	}
	if v := os.Getenv("OPEN_SANDBOX_DOMAIN"); v != "" {
		return v
	}
	return DefaultDomain
}

// GetProtocol returns the configured protocol, falling back to env var and default.
func (c *ConnectionConfig) GetProtocol() string {
	if c.Protocol != "" {
		return c.Protocol
	}
	if v := os.Getenv("OPEN_SANDBOX_PROTOCOL"); v != "" {
		return v
	}
	return DefaultProtocol
}

// GetAPIKey returns the configured API key, falling back to env var.
func (c *ConnectionConfig) GetAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	return os.Getenv("OPEN_SANDBOX_API_KEY")
}

// GetBaseURL returns the lifecycle API base URL (e.g. "http://localhost:8080").
// Note: this does NOT append /v1. The lifecycle client's baseURL should be
// set by the caller if a version prefix is needed (e.g. for local OpenSandbox
// servers that require /v1). Staging/proxy deployments typically don't use /v1.
func (c *ConnectionConfig) GetBaseURL() string {
	domain := c.GetDomain()
	protocol := c.GetProtocol()

	// If domain already has a scheme, use it as-is.
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return strings.TrimRight(domain, "/")
	}

	return fmt.Sprintf("%s://%s", protocol, domain)
}

// GetAuthHeader returns the auth header name for lifecycle requests.
func (c *ConnectionConfig) GetAuthHeader() string {
	if c.AuthHeader != "" {
		return c.AuthHeader
	}
	return "OPEN-SANDBOX-API-KEY"
}

// GetRequestTimeout returns the request timeout, defaulting to DefaultRequestTimeout.
func (c *ConnectionConfig) GetRequestTimeout() time.Duration {
	if c.RequestTimeout > 0 {
		return c.RequestTimeout
	}
	return DefaultRequestTimeout
}

// clientOpts builds the common Option slice from config fields.
func (c *ConnectionConfig) clientOpts(includeAuthHeader bool) []Option {
	var opts []Option
	if includeAuthHeader && c.AuthHeader != "" {
		opts = append(opts, WithAuthHeader(c.AuthHeader))
	}
	if c.HTTPClient != nil {
		opts = append(opts, WithHTTPClient(c.HTTPClient))
	}
	if t := c.GetRequestTimeout(); t > 0 {
		opts = append(opts, WithTimeout(t))
	}
	if len(c.Headers) > 0 {
		opts = append(opts, WithHeaders(c.Headers))
	}
	return opts
}

// lifecycleClient creates a LifecycleClient from this config.
// Appends the API version prefix (/v1) to the base URL, as required by
// NewLifecycleClient and the OpenSandbox lifecycle API spec.
func (c *ConnectionConfig) lifecycleClient() *LifecycleClient {
	return NewLifecycleClient(c.GetBaseURL()+"/"+APIVersion, c.GetAPIKey(), c.clientOpts(true)...)
}

// execdClient creates an ExecdClient for a resolved endpoint.
func (c *ConnectionConfig) execdClient(endpointURL, token string) *ExecdClient {
	return NewExecdClient(endpointURL, token, c.clientOpts(true)...)
}

// egressClient creates an EgressClient for a resolved endpoint.
func (c *ConnectionConfig) egressClient(endpointURL, token string) *EgressClient {
	return NewEgressClient(endpointURL, token, c.clientOpts(false)...)
}
