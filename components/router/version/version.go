package version

import "fmt"

// Version package values is auto-generated, the following values will be overridden at build time.
var (
	// Version represents the version of taskline suite.
	Version = "1.0.0"

	// BuildTime is the time when taskline-operator binary is built
	BuildTime = "assigned-at-build-time"

	// GitCommit is the commit id to build taskline-operator
	GitCommit = "assigned-at-build-time"
)

// EchoVersion is used to echo current binary build info for diagnosing
func EchoVersion() {
	fmt.Println("#####################################################")
	fmt.Printf("  Current binary is built at: %s\n", BuildTime)
	fmt.Printf("  It's git commit id is: %s\n", GitCommit)
	fmt.Println("#####################################################")
}
