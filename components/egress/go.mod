module github.com/alibaba/opensandbox/egress

go 1.24.0

require (
	github.com/alibaba/opensandbox/internal v0.0.0
	github.com/miekg/dns v1.1.61
	golang.org/x/sys v0.31.0
)

require (
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/mod v0.18.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/tools v0.22.0 // indirect
)

replace github.com/alibaba/opensandbox/internal => ../internal
