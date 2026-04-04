.PHONY: help build test lint lint-fix lint-vuln lint-security lint-security-full clean

# stackgraph is not part of the parent go.work, so disable it
export GOWORK=off

help:
	@echo "Available commands:"
	@echo "  make build              - Build the stackgraph binary"
	@echo "  make test               - Run all tests"
	@echo "  make lint               - Run golangci-lint"
	@echo "  make lint-fix           - Run golangci-lint with auto-fix"
	@echo "  make lint-vuln          - Run govulncheck for known vulnerabilities"
	@echo "  make lint-security      - Run Trivy filesystem scan (HIGH/CRITICAL only)"
	@echo "  make lint-security-full - Run Trivy scan including all severities"
	@echo "  make clean              - Remove build artifacts"

build:
	@echo "Building stackgraph..."
	go build -o bin/stackgraph ./cmd/stackgraph

test:
	@echo "Running tests..."
	go test ./...

lint:
	@echo "Running golangci-lint..."
	@export PATH="$$(go env GOPATH)/bin:$$PATH" && \
	golangci-lint run

lint-fix:
	@echo "Running golangci-lint with auto-fix..."
	@export PATH="$$(go env GOPATH)/bin:$$PATH" && \
	golangci-lint run --fix

lint-vuln:
	@echo "Running govulncheck to scan for vulnerabilities..."
	@export PATH="$$(go env GOPATH)/bin:$$PATH" && \
	if ! command -v govulncheck >/dev/null 2>&1; then \
		echo "Installing govulncheck..."; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
	fi && \
	govulncheck ./...

lint-security:
	@trivy fs --severity HIGH,CRITICAL --exit-code 1 .

lint-security-full:
	@trivy fs .

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
