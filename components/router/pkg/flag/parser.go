package flag

import (
	"flag"
)

// InitFlags registers CLI flags and env overrides.
func InitFlags() {
	flag.StringVar(&LogLevel, "log-level", "info", "Server log level")
	flag.IntVar(&Port, "port", 28888, "Server listening port (default: 28888)")
	flag.StringVar(&IngressLabelKey, "ingress-label-key", "", "Server access token for API authentication")
	flag.StringVar(&Namespace, "namespace", "", "API graceful shutdown timeout duration (default: 3s)")

	// Parse flags - these will override environment variables if provided
	flag.Parse()
}
