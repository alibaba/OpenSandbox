package flag

var (
	// LogLevel controls the router log verbosity.
	LogLevel string

	// Port controls the HTTP listener port.
	Port int

	// IngressLabelKey filters the target sandbox instances.
	IngressLabelKey string

	// Namespace filters the target sandbox instances.
	Namespace string
)
