.PHONY: all build vet test lint generate

all: build vet test

generate:
	cd opensandbox/api/lifecycle && oapi-codegen --config cfg.yaml ../specs/sandbox-lifecycle.yml
	cd opensandbox/api/execd    && oapi-codegen --config cfg.yaml ../specs/execd-api.yaml
	cd opensandbox/api/egress   && oapi-codegen --config cfg.yaml ../specs/egress-api.yaml
	@echo "Generated all API clients from OpenAPI specs"

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./opensandbox/ -v

test-integration:
	go test -tags=integration ./opensandbox/ -v -timeout 3m

test-staging:
	go test -tags=staging ./opensandbox/ -v -timeout 3m

lint:
	@which staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed — skipping lint"
