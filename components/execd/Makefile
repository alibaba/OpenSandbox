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

.PHONY: multi-build
multi-build: vet ## Cross-compile for linux/windows/darwin amd64/arm64.
	@mkdir -p bin
	@for os in linux windows darwin; do \
		for arch in amd64 arm64; do \
			out=bin/execd-$${os}-$${arch}; \
			[ "$${os}" = "windows" ] && out="$${out}.exe"; \
			echo ">> building $${os}/$${arch} -> $${out}"; \
			GOOS=$${os} GOARCH=$${arch} CGO_ENABLED=0 go build -o "$${out}" main.go || exit $$?; \
		done; \
	done
