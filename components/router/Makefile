.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go mod tidy && go mod vendor
	go vet ./...

.PHONY: test
test: vet ## Run tests
	go test -v -coverpkg=./... ./pkg/...

##@ Linter

.PHONY: install-golint
install-golint:
	@if ! command -v golangci-lint &> /dev/null; then \
  		echo "installing golangci-lint..."; \
  		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
  	else \
  	    echo "golangci-lint already installed"; \
	fi

.PHONY: golint
golint: fmt install-golint
	golangci-lint run -v --fix ./...

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'github.com/alibaba/opensandbox/router/version.Version=$(VERSION)' \
	-X 'github.com/alibaba/opensandbox/router/version.BuildTime=$(BUILD_TIME)' \
	-X 'github.com/alibaba/opensandbox/router/version.GitCommit=$(GIT_COMMIT)'

.PHONY: build
build: vet ## Build the binary.
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/router main.go
