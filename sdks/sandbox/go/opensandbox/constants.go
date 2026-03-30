package opensandbox

import "time"

const (
	// DefaultExecdPort is the standard port for the execd service inside a sandbox.
	DefaultExecdPort = 44772

	// DefaultEgressPort is the standard port for the egress sidecar inside a sandbox.
	DefaultEgressPort = 18080

	// DefaultTimeoutSeconds is the default sandbox TTL in seconds.
	DefaultTimeoutSeconds = 600

	// DefaultReadyTimeoutSeconds is the default timeout for WaitUntilReady.
	DefaultReadyTimeoutSeconds = 30

	// DefaultHealthCheckPollingInterval is the default polling interval for WaitUntilReady.
	DefaultHealthCheckPollingInterval = 200 * time.Millisecond

	// DefaultRequestTimeout is the default HTTP request timeout.
	DefaultRequestTimeout = 30 * time.Second

	// APIVersion is the lifecycle API version prefix.
	APIVersion = "v1"

	// DefaultDomain is the default OpenSandbox server address.
	DefaultDomain = "localhost:8080"

	// DefaultProtocol is the default protocol for connecting to the server.
	DefaultProtocol = "http"
)

// DefaultEntrypoint keeps the sandbox alive for interactive use.
var DefaultEntrypoint = []string{"tail", "-f", "/dev/null"}

// DefaultResourceLimits provides sensible defaults for sandbox resource limits.
var DefaultResourceLimits = ResourceLimits{
	"cpu":    "1",
	"memory": "2Gi",
}
