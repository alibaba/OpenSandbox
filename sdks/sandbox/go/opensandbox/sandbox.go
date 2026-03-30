package opensandbox

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SandboxCreateOptions configures sandbox creation.
type SandboxCreateOptions struct {
	// Image is the container image URI (required).
	Image string

	// Entrypoint is the command to run. Defaults to DefaultEntrypoint.
	Entrypoint []string

	// Resource limits (e.g. {"cpu": "500m", "memory": "256Mi"}).
	// Defaults to DefaultResourceLimits.
	ResourceLimits ResourceLimits

	// TimeoutSeconds is the sandbox TTL. Nil means use server default.
	TimeoutSeconds *int

	// Env variables injected into the sandbox.
	Env map[string]string

	// Metadata for filtering and tagging.
	Metadata map[string]string

	// NetworkPolicy for egress control.
	NetworkPolicy *NetworkPolicy

	// Volumes to mount.
	Volumes []Volume

	// Extensions for provider-specific parameters.
	Extensions map[string]string

	// SkipHealthCheck skips the WaitUntilReady call after creation.
	SkipHealthCheck bool

	// ReadyTimeout overrides DefaultReadyTimeoutSeconds.
	ReadyTimeout time.Duration

	// HealthCheckInterval overrides DefaultHealthCheckPollingInterval.
	HealthCheckInterval time.Duration

	// HealthCheck is a custom health check function. If nil, execd /ping is used.
	HealthCheck func(ctx context.Context, sb *Sandbox) (bool, error)
}

// Sandbox is the high-level object wrapping lifecycle, execd, and egress clients.
// Use CreateSandbox or ConnectSandbox to obtain an instance.
type Sandbox struct {
	id     string
	config *ConnectionConfig

	lifecycle *LifecycleClient
	execd     *ExecdClient
	egress    *EgressClient
}

// ID returns the sandbox identifier.
func (s *Sandbox) ID() string { return s.id }

// CreateSandbox creates a new sandbox and waits for it to be ready.
func CreateSandbox(ctx context.Context, config ConnectionConfig, opts SandboxCreateOptions) (*Sandbox, error) {
	if opts.Image == "" {
		return nil, &InvalidArgumentError{Field: "Image", Message: "image is required"}
	}

	entrypoint := opts.Entrypoint
	if len(entrypoint) == 0 {
		entrypoint = DefaultEntrypoint
	}
	limits := opts.ResourceLimits
	if limits == nil {
		limits = DefaultResourceLimits
	}
	timeout := opts.TimeoutSeconds
	if timeout == nil {
		t := DefaultTimeoutSeconds
		timeout = &t
	}

	lc := config.lifecycleClient()

	req := CreateSandboxRequest{
		Image:          ImageSpec{URI: opts.Image},
		Entrypoint:     entrypoint,
		ResourceLimits: limits,
		Timeout:        timeout,
		Env:            opts.Env,
		Metadata:       opts.Metadata,
		NetworkPolicy:  opts.NetworkPolicy,
		Volumes:        opts.Volumes,
		Extensions:     opts.Extensions,
	}

	created, err := lc.CreateSandbox(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("opensandbox: create sandbox: %w", err)
	}

	sb := &Sandbox{
		id:        created.ID,
		config:    &config,
		lifecycle: lc,
	}

	// Poll until Running
	if err := sb.waitForRunning(ctx); err != nil {
		// Best-effort cleanup
		_ = lc.DeleteSandbox(context.Background(), created.ID)
		return nil, err
	}

	// Resolve execd endpoint
	if err := sb.resolveExecd(ctx); err != nil {
		_ = lc.DeleteSandbox(context.Background(), created.ID)
		return nil, fmt.Errorf("opensandbox: resolve execd: %w", err)
	}

	// Wait until execd is ready
	if !opts.SkipHealthCheck {
		readyOpts := ReadyOptions{
			Timeout:         opts.ReadyTimeout,
			PollingInterval: opts.HealthCheckInterval,
			HealthCheck:     opts.HealthCheck,
		}
		if err := sb.WaitUntilReady(ctx, readyOpts); err != nil {
			_ = lc.DeleteSandbox(context.Background(), created.ID)
			return nil, err
		}
	}

	return sb, nil
}

// ConnectSandbox connects to an existing running sandbox by ID.
func ConnectSandbox(ctx context.Context, config ConnectionConfig, sandboxID string, opts ...ReadyOptions) (*Sandbox, error) {
	if sandboxID == "" {
		return nil, &InvalidArgumentError{Field: "sandboxID", Message: "sandbox ID is required"}
	}

	lc := config.lifecycleClient()

	sb := &Sandbox{
		id:        sandboxID,
		config:    &config,
		lifecycle: lc,
	}

	if err := sb.resolveExecd(ctx); err != nil {
		return nil, fmt.Errorf("opensandbox: resolve execd: %w", err)
	}

	// Optional readiness check
	if len(opts) > 0 {
		if err := sb.WaitUntilReady(ctx, opts[0]); err != nil {
			return nil, err
		}
	}

	return sb, nil
}

// Kill terminates the sandbox. This is irreversible.
func (s *Sandbox) Kill(ctx context.Context) error {
	return s.lifecycle.DeleteSandbox(ctx, s.id)
}

// Close releases local HTTP resources. Does NOT terminate the sandbox.
func (s *Sandbox) Close() {
	// No-op for now — Go's http.Client doesn't need explicit close.
	// Placeholder for future transport pooling.
}

// Pause pauses the sandbox while preserving its state.
func (s *Sandbox) Pause(ctx context.Context) error {
	return s.lifecycle.PauseSandbox(ctx, s.id)
}

// GetInfo returns the sandbox's current info (status, metadata, image, etc.).
func (s *Sandbox) GetInfo(ctx context.Context) (*SandboxInfo, error) {
	return s.lifecycle.GetSandbox(ctx, s.id)
}

// GetMetrics returns current system resource metrics from the sandbox.
func (s *Sandbox) GetMetrics(ctx context.Context) (*Metrics, error) {
	if s.execd == nil {
		return nil, fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.GetMetrics(ctx)
}

// IsHealthy checks whether the sandbox's execd service is responsive.
func (s *Sandbox) IsHealthy(ctx context.Context) bool {
	if s.execd == nil {
		return false
	}
	return s.execd.Ping(ctx) == nil
}

// Renew extends the sandbox's expiration by the given duration from now.
func (s *Sandbox) Renew(ctx context.Context, duration time.Duration) (*RenewExpirationResponse, error) {
	return s.lifecycle.RenewExpiration(ctx, s.id, time.Now().Add(duration))
}

// GetEndpoint retrieves the public access endpoint for a service port.
func (s *Sandbox) GetEndpoint(ctx context.Context, port int) (*Endpoint, error) {
	useProxy := s.config.UseServerProxy
	return s.lifecycle.GetEndpoint(ctx, s.id, port, &useProxy)
}

// GetEgressPolicy retrieves the current egress network policy.
func (s *Sandbox) GetEgressPolicy(ctx context.Context) (*PolicyStatusResponse, error) {
	if err := s.resolveEgress(ctx); err != nil {
		return nil, err
	}
	return s.egress.GetPolicy(ctx)
}

// PatchEgressRules merges network rules into the current egress policy.
func (s *Sandbox) PatchEgressRules(ctx context.Context, rules []NetworkRule) (*PolicyStatusResponse, error) {
	if err := s.resolveEgress(ctx); err != nil {
		return nil, err
	}
	return s.egress.PatchPolicy(ctx, rules)
}

// RunCommand executes a shell command and returns the structured result.
func (s *Sandbox) RunCommand(ctx context.Context, command string, handlers *ExecutionHandlers) (*Execution, error) {
	return s.RunCommandWithOpts(ctx, RunCommandRequest{Command: command}, handlers)
}

// RunCommandWithOpts executes a command with full options.
func (s *Sandbox) RunCommandWithOpts(ctx context.Context, req RunCommandRequest, handlers *ExecutionHandlers) (*Execution, error) {
	if s.execd == nil {
		return nil, fmt.Errorf("opensandbox: execd client not initialized")
	}

	exec := &Execution{}
	err := s.execd.RunCommand(ctx, req, func(event StreamEvent) error {
		return processStreamEvent(exec, event, handlers)
	})
	if err != nil {
		return exec, err
	}
	return exec, nil
}

// Ping checks if the execd service is responsive.
func (s *Sandbox) Ping(ctx context.Context) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.Ping(ctx)
}

// GetFileInfo retrieves file metadata.
func (s *Sandbox) GetFileInfo(ctx context.Context, path string) (map[string]FileInfo, error) {
	if s.execd == nil {
		return nil, fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.GetFileInfo(ctx, path)
}

// ReadyOptions configures WaitUntilReady behavior.
type ReadyOptions struct {
	Timeout         time.Duration
	PollingInterval time.Duration
	HealthCheck     func(ctx context.Context, sb *Sandbox) (bool, error)
}

// WaitUntilReady polls the execd /ping endpoint until it responds or the timeout expires.
func (s *Sandbox) WaitUntilReady(ctx context.Context, opts ReadyOptions) error {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = time.Duration(DefaultReadyTimeoutSeconds) * time.Second
	}
	interval := opts.PollingInterval
	if interval == 0 {
		interval = DefaultHealthCheckPollingInterval
	}

	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var healthy bool
		if opts.HealthCheck != nil {
			var err error
			healthy, err = opts.HealthCheck(ctx, s)
			if err != nil {
				lastErr = err
			}
		} else {
			err := s.execd.Ping(ctx)
			healthy = err == nil
			if err != nil {
				lastErr = err
			}
		}

		if healthy {
			return nil
		}

		time.Sleep(interval)
	}

	return &SandboxReadyTimeoutError{
		SandboxID: s.id,
		Elapsed:   timeout.String(),
		LastErr:   lastErr,
	}
}

// waitForRunning polls the lifecycle API until the sandbox reaches Running state.
func (s *Sandbox) waitForRunning(ctx context.Context) error {
	for i := 0; i < 120; i++ {
		info, err := s.lifecycle.GetSandbox(ctx, s.id)
		if err != nil {
			return fmt.Errorf("opensandbox: get sandbox status: %w", err)
		}
		state := string(info.Status.State)
		if state == string(StateRunning) {
			return nil
		}
		if state == string(StateFailed) || state == string(StateTerminated) {
			return fmt.Errorf("opensandbox: sandbox %s entered terminal state: %s (%s)",
				s.id, state, info.Status.Reason)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("opensandbox: sandbox %s did not reach Running state", s.id)
}

// resolveExecd resolves the execd endpoint and creates the ExecdClient.
func (s *Sandbox) resolveExecd(ctx context.Context) error {
	if s.execd != nil {
		return nil
	}

	useProxy := s.config.UseServerProxy
	endpoint, err := s.lifecycle.GetEndpoint(ctx, s.id, DefaultExecdPort, &useProxy)
	if err != nil {
		return err
	}

	execdURL := endpoint.Endpoint
	if !strings.HasPrefix(execdURL, "http") {
		execdURL = s.config.GetProtocol() + "://" + execdURL
	}

	token := ""
	if endpoint.Headers != nil {
		token = endpoint.Headers["X-EXECD-ACCESS-TOKEN"]
	}
	// If server proxy mode, use the API key for auth
	if s.config.UseServerProxy && token == "" {
		token = s.config.GetAPIKey()
	}

	s.execd = s.config.execdClient(execdURL, token)
	return nil
}

// resolveEgress resolves the egress endpoint and creates the EgressClient.
func (s *Sandbox) resolveEgress(ctx context.Context) error {
	if s.egress != nil {
		return nil
	}

	useProxy := s.config.UseServerProxy
	endpoint, err := s.lifecycle.GetEndpoint(ctx, s.id, DefaultEgressPort, &useProxy)
	if err != nil {
		return err
	}

	egressURL := endpoint.Endpoint
	if !strings.HasPrefix(egressURL, "http") {
		egressURL = s.config.GetProtocol() + "://" + egressURL
	}

	token := ""
	if endpoint.Headers != nil {
		token = endpoint.Headers["OPENSANDBOX-EGRESS-AUTH"]
	}

	s.egress = s.config.egressClient(egressURL, token)
	return nil
}
