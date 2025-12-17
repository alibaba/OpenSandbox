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

.PHONY: build
build: vet ## Build the binary.
	go build -o bin/execd main.go
