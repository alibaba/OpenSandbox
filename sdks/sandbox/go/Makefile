.PHONY: all build vet test lint generate

all: build vet test

generate:
	cd api/lifecycle && oapi-codegen --config cfg.yaml ../specs/sandbox-lifecycle.yml
	cd api/execd    && oapi-codegen --config cfg.yaml ../specs/execd-api.yaml
	cd api/egress   && oapi-codegen --config cfg.yaml ../specs/egress-api.yaml
	@echo "Generated all API clients from OpenAPI specs"

build:
	go build ./...

vet:
	go vet ./...

test:
	go test -v ./...

lint:
	@which staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed — skipping lint"
