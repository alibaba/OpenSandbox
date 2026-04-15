// Copyright 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opensandbox

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// LifecycleClient provides methods for the OpenSandbox Lifecycle API.
type LifecycleClient struct {
	*Client
}

// NewLifecycleClient creates a new LifecycleClient.
// baseURL should include the version prefix (e.g. "http://localhost:8080/v1").
func NewLifecycleClient(baseURL, apiKey string, opts ...Option) *LifecycleClient {
	return &LifecycleClient{
		Client: NewClient(baseURL, apiKey, "OPEN-SANDBOX-API-KEY", opts...),
	}
}

// ListOptions configures filtering and pagination for ListSandboxes.
type ListOptions struct {
	// States filters by lifecycle state. Multiple values use OR logic.
	States []SandboxState
	// Metadata filters by key-value metadata (AND logic).
	Metadata map[string]string
	// Page number (1-based). Defaults to 1.
	Page int
	// PageSize is the number of items per page. Defaults to 20.
	PageSize int
}

// ListSandboxes returns a paginated list of sandboxes with optional filtering.
func (c *LifecycleClient) ListSandboxes(ctx context.Context, opts ListOptions) (*ListSandboxesResponse, error) {
	params := url.Values{}
	for _, s := range opts.States {
		params.Add("state", string(s))
	}
	if len(opts.Metadata) > 0 {
		metaVals := url.Values{}
		for k, v := range opts.Metadata {
			metaVals.Set(k, v)
		}
		params.Set("metadata", metaVals.Encode())
	}
	if opts.Page > 0 {
		params.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PageSize > 0 {
		params.Set("pageSize", strconv.Itoa(opts.PageSize))
	}

	path := "/sandboxes"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var resp ListSandboxesResponse
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateSandbox creates a new sandbox from a container image.
func (c *LifecycleClient) CreateSandbox(ctx context.Context, req CreateSandboxRequest) (*SandboxInfo, error) {
	var resp SandboxInfo
	if err := c.doRequest(ctx, "POST", "/sandboxes", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSandbox retrieves the complete sandbox information by ID.
func (c *LifecycleClient) GetSandbox(ctx context.Context, id string) (*SandboxInfo, error) {
	var resp SandboxInfo
	if err := c.doRequest(ctx, "GET", "/sandboxes/"+url.PathEscape(id), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteSandbox deletes a sandbox, scheduling it for termination.
func (c *LifecycleClient) DeleteSandbox(ctx context.Context, id string) error {
	return c.doRequest(ctx, "DELETE", "/sandboxes/"+url.PathEscape(id), nil, nil)
}

// PauseSandbox pauses a running sandbox while preserving its state.
func (c *LifecycleClient) PauseSandbox(ctx context.Context, id string) error {
	return c.doRequest(ctx, "POST", "/sandboxes/"+url.PathEscape(id)+"/pause", nil, nil)
}

// ResumeSandbox resumes a paused sandbox.
func (c *LifecycleClient) ResumeSandbox(ctx context.Context, id string) error {
	return c.doRequest(ctx, "POST", "/sandboxes/"+url.PathEscape(id)+"/resume", nil, nil)
}

// RenewExpiration renews the sandbox's absolute expiration time.
// The caller is responsible for computing the desired expiresAt time.
func (c *LifecycleClient) RenewExpiration(ctx context.Context, id string, expiresAt time.Time) (*RenewExpirationResponse, error) {
	req := RenewExpirationRequest{
		ExpiresAt: expiresAt.UTC(),
	}
	var resp RenewExpirationResponse
	if err := c.doRequest(ctx, "POST", "/sandboxes/"+url.PathEscape(id)+"/renew-expiration", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetEndpoint retrieves the public access endpoint for a service running
// on the specified port inside the sandbox. If useServerProxy is non-nil,
// the server proxy query parameter is included.
func (c *LifecycleClient) GetEndpoint(ctx context.Context, sandboxID string, port int, useServerProxy *bool) (*Endpoint, error) {
	path := fmt.Sprintf("/sandboxes/%s/endpoints/%d", url.PathEscape(sandboxID), port)
	params := url.Values{}
	if useServerProxy != nil {
		params.Set("use_server_proxy", strconv.FormatBool(*useServerProxy))
	}
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var resp Endpoint
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
