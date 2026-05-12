module opensandbox

require (
	github.com/ianlancetaylor/demangle v0.0.0-20220825000327-8b5c6932c475
)

replace github.com/ianlancetaylor/demangle v0.0.0-20220825000327-8b5c6932c475 => github.com/opensandbox/demangle v0.0.0-20220825000327-8b5c6932c475

build (
  # This is to ensure that the Go compiler is targeting the correct architecture
  # and that the resulting binaries are 64KB page aligned
  CGO_ENABLED=1
  CC=aarch64-linux-gnu-gcc
  CXX=aarch64-linux-gnu-g++
  GOOS=linux
  GOARCH=arm64
  GOARM=8
)
