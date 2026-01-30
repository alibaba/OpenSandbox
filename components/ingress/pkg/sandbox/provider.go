// Copyright 2025 Alibaba Group Holding Ltd.
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

package sandbox

import (
	"context"
	"errors"
)

const (
	// Provider type constants
	ProviderTypeBatchSandbox = "batchsandbox"
)

// Standard errors for Provider operations
var (
	// ErrSandboxNotFound indicates the sandbox resource does not exist
	ErrSandboxNotFound = errors.New("sandbox not found")

	// ErrSandboxNotReady indicates the sandbox exists but is not ready
	// This includes: not enough ready replicas, missing endpoints, invalid configuration
	ErrSandboxNotReady = errors.New("sandbox not ready")
)

// Provider defines the interface for sandbox resource providers
// Implementations include BatchSandboxProvider, AgentSandboxProvider, etc.
type Provider interface {
	// GetEndpoint retrieves the IP address for a sandbox by its name
	// The namespace is determined by the provider's configuration
	// Returns the first available IP from the endpoints annotation
	// Returns error if sandbox not found or no endpoints available
	// Note: This is a local cache query, no network I/O involved
	GetEndpoint(name string) (string, error)

	// Start initializes and starts the provider's informer cache
	// Waits for cache sync before returning
	// Must be called before using GetEndpoint
	Start(ctx context.Context) error
}

// ProviderFactory creates a Provider instance based on the provider type
type ProviderFactory interface {
	CreateProvider(providerType string) (Provider, error)
}
