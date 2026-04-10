package opensandbox

import "context"

// EgressClient provides methods for the OpenSandbox Egress API.
// It connects to the egress sidecar endpoint running inside a specific sandbox.
type EgressClient struct {
	*Client
}

// NewEgressClient creates a new EgressClient.
// baseURL is the sandbox-specific egress sidecar endpoint
// (e.g. "http://localhost:18080").
// authToken is the value for the OPENSANDBOX-EGRESS-AUTH header; pass ""
// if the sidecar does not require authentication.
func NewEgressClient(baseURL, authToken string, opts ...Option) *EgressClient {
	return &EgressClient{
		Client: NewClient(baseURL, authToken, "OPENSANDBOX-EGRESS-AUTH", opts...),
	}
}

// GetPolicy returns the currently enforced egress policy and sidecar metadata.
func (c *EgressClient) GetPolicy(ctx context.Context) (*PolicyStatusResponse, error) {
	var resp PolicyStatusResponse
	if err := c.doRequest(ctx, "GET", "/policy", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PatchPolicy merges the given network rules into the current egress policy.
// Existing rules remain unless overridden. Rule conflict behavior is determined
// by the server.
func (c *EgressClient) PatchPolicy(ctx context.Context, rules []NetworkRule) (*PolicyStatusResponse, error) {
	var resp PolicyStatusResponse
	if err := c.doRequest(ctx, "PATCH", "/policy", rules, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
