// Package opensandbox provides Go client libraries for the OpenSandbox
// Lifecycle, Egress, and Execd APIs.
//
// Lifecycle types are generated from OpenAPI specifications using oapi-codegen.
// Run `make generate` or `go generate ./...` to regenerate after spec changes.
package opensandbox

import "time"

// ---------------------------------------------------------------------------
// Lifecycle types (generated from sandbox-lifecycle.yml via type aliases)
// ---------------------------------------------------------------------------

// Note: the generated client is at opensandbox/api/lifecycle/ and can be
// used directly for consumers who prefer the raw generated interface.
// These aliases provide the ergonomic public API.

// SandboxState represents the high-level lifecycle state of a sandbox.
type SandboxState string

const (
	StatePending    SandboxState = "Pending"
	StateRunning    SandboxState = "Running"
	StatePausing    SandboxState = "Pausing"
	StatePaused     SandboxState = "Paused"
	StateStopping   SandboxState = "Stopping"
	StateTerminated SandboxState = "Terminated"
	StateFailed     SandboxState = "Failed"
)

// SandboxStatus provides detailed status information with lifecycle state
// and transition details.
type SandboxStatus struct {
	State            SandboxState `json:"state"`
	Reason           string       `json:"reason,omitempty"`
	Message          string       `json:"message,omitempty"`
	LastTransitionAt *time.Time   `json:"lastTransitionAt,omitempty"`
}

// ImageSpec describes the container image used to provision a sandbox.
type ImageSpec struct {
	URI  string     `json:"uri"`
	Auth *ImageAuth `json:"auth,omitempty"`
}

// ImageAuth holds registry authentication credentials for private images.
type ImageAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ResourceLimits defines runtime resource constraints as key-value pairs.
// Common keys: "cpu" (e.g. "500m"), "memory" (e.g. "512Mi"), "gpu" (e.g. "1").
type ResourceLimits map[string]string

// Volume defines a storage mount for a sandbox.
type Volume struct {
	Name      string `json:"name"`
	Host      *Host  `json:"host,omitempty"`
	PVC       *PVC   `json:"pvc,omitempty"`
	OSSFS     *OSSFS `json:"ossfs,omitempty"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
	SubPath   string `json:"subPath,omitempty"`
}

// Host represents a host path bind mount backend.
type Host struct {
	Path string `json:"path"`
}

// PVC represents a platform-managed named volume backend.
type PVC struct {
	ClaimName string `json:"claimName"`
}

// OSSFS represents an Alibaba Cloud OSS mount backend via ossfs.
type OSSFS struct {
	Bucket          string   `json:"bucket"`
	Endpoint        string   `json:"endpoint"`
	Version         string   `json:"version,omitempty"`
	Options         []string `json:"options,omitempty"`
	AccessKeyID     string   `json:"accessKeyId"`
	AccessKeySecret string   `json:"accessKeySecret"`
}

// NetworkPolicy defines the egress network policy for a sandbox.
type NetworkPolicy struct {
	DefaultAction string        `json:"defaultAction,omitempty"`
	Egress        []NetworkRule `json:"egress,omitempty"`
}

// NetworkRule defines a single egress allow/deny rule.
type NetworkRule struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

// CreateSandboxRequest is the request body for creating a new sandbox.
type CreateSandboxRequest struct {
	Image          ImageSpec         `json:"image"`
	Timeout        *int              `json:"timeout,omitempty"`
	ResourceLimits ResourceLimits    `json:"resourceLimits"`
	Env            map[string]string `json:"env,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Entrypoint     []string          `json:"entrypoint"`
	NetworkPolicy  *NetworkPolicy    `json:"networkPolicy,omitempty"`
	Volumes        []Volume          `json:"volumes,omitempty"`
	Extensions     map[string]string `json:"extensions,omitempty"`
}

// Sandbox represents a runtime execution environment provisioned from a
// container image.
type Sandbox struct {
	ID         string            `json:"id"`
	Image      *ImageSpec        `json:"image,omitempty"`
	Status     SandboxStatus     `json:"status"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Entrypoint []string          `json:"entrypoint"`
	ExpiresAt  *time.Time        `json:"expiresAt,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
}

// PaginationInfo contains pagination metadata for list responses.
type PaginationInfo struct {
	Page        int  `json:"page"`
	PageSize    int  `json:"pageSize"`
	TotalItems  int  `json:"totalItems"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
}

// ListSandboxesResponse is the paginated response from listing sandboxes.
type ListSandboxesResponse struct {
	Items      []Sandbox      `json:"items"`
	Pagination PaginationInfo `json:"pagination"`
}

// Endpoint describes a public access endpoint for a service running inside
// a sandbox.
type Endpoint struct {
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// RenewExpirationRequest is the request body for renewing sandbox expiration.
type RenewExpirationRequest struct {
	ExpiresAt time.Time `json:"expiresAt"`
}

// RenewExpirationResponse is the response from renewing sandbox expiration.
type RenewExpirationResponse struct {
	ExpiresAt time.Time `json:"expiresAt"`
}

// ---------------------------------------------------------------------------
// Egress types (hand-written — egress spec types use *string which hurts
// ergonomics; the generated client is at opensandbox/api/egress/)
// ---------------------------------------------------------------------------

// PolicyStatusResponse is the response from the egress policy endpoints.
type PolicyStatusResponse struct {
	Status          string         `json:"status,omitempty"`
	Mode            string         `json:"mode,omitempty"`
	EnforcementMode string         `json:"enforcementMode,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Policy          *NetworkPolicy `json:"policy,omitempty"`
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

// ErrorResponse is the standard error response for non-2xx HTTP responses.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIError wraps an ErrorResponse with the HTTP status code.
type APIError struct {
	StatusCode int
	Response   ErrorResponse
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return e.Response.Code + ": " + e.Response.Message
}

// ---------------------------------------------------------------------------
// Execd types (hand-written — execd endpoints use SSE streaming, multipart
// upload, and text responses not representable in generated client)
// ---------------------------------------------------------------------------

// CodeContext represents a code execution context with session identifier.
type CodeContext struct {
	ID       string `json:"id,omitempty"`
	Language string `json:"language"`
}

// CreateContextRequest is the request body for creating a code execution context.
type CreateContextRequest struct {
	Language string `json:"language"`
}

// RunCodeRequest is the request body for executing code in a context.
type RunCodeRequest struct {
	Context *CodeContext `json:"context,omitempty"`
	Code    string      `json:"code"`
}

// Session represents a bash session with a unique identifier.
type Session struct {
	ID string `json:"session_id"`
}

// CreateSessionRequest is the optional request body for creating a bash session.
type CreateSessionRequest struct {
	Cwd string `json:"cwd,omitempty"`
}

// RunCommandRequest is the request body for executing a shell command.
type RunCommandRequest struct {
	Command    string            `json:"command"`
	Cwd        string            `json:"cwd,omitempty"`
	Background bool              `json:"background,omitempty"`
	Timeout    int64             `json:"timeout,omitempty"`
	UID        *int32            `json:"uid,omitempty"`
	GID        *int32            `json:"gid,omitempty"`
	Envs       map[string]string `json:"envs,omitempty"`
}

// RunInSessionRequest is the request body for running a command in an existing bash session.
type RunInSessionRequest struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"`
	Timeout int64  `json:"timeout,omitempty"`
}

// CommandStatusResponse contains the status of a command execution.
type CommandStatusResponse struct {
	ID         string     `json:"id"`
	Content    string     `json:"content"`
	Running    bool       `json:"running"`
	ExitCode   *int32     `json:"exit_code,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// CommandLogsResponse contains the stdout/stderr output and cursor for
// incremental log polling.
type CommandLogsResponse struct {
	Output string
	Cursor int64
}

// FileInfo contains file metadata including path and permissions.
type FileInfo struct {
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
	CreatedAt  time.Time `json:"created_at"`
	Owner      string    `json:"owner"`
	Group      string    `json:"group"`
	Mode       int       `json:"mode"`
}

// Permission defines file ownership and mode settings.
type Permission struct {
	Owner string `json:"owner,omitempty"`
	Group string `json:"group,omitempty"`
	Mode  int    `json:"mode"`
}

// PermissionsRequest maps file paths to their desired permission settings.
type PermissionsRequest map[string]Permission

// MoveItem defines a single file move/rename operation.
type MoveItem struct {
	Src  string `json:"src"`
	Dest string `json:"dest"`
}

// MoveRequest is a list of file move/rename operations.
type MoveRequest []MoveItem

// ReplaceItem defines a text replacement operation for a single file.
type ReplaceItem struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// ReplaceRequest maps file paths to their replacement operations.
type ReplaceRequest map[string]ReplaceItem

// FileMetadata is the metadata sent alongside file uploads.
type FileMetadata struct {
	Path  string `json:"path"`
	Owner string `json:"owner,omitempty"`
	Group string `json:"group,omitempty"`
	Mode  int    `json:"mode,omitempty"`
}

// Metrics contains system resource usage metrics.
type Metrics struct {
	CPUCount   float64 `json:"cpu_count"`
	CPUUsedPct float64 `json:"cpu_used_pct"`
	MemTotalMB float64 `json:"mem_total_mib"`
	MemUsedMB  float64 `json:"mem_used_mib"`
	Timestamp  int64   `json:"timestamp"`
}
