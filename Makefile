# Makefile for spanza - WireGuard relay tool

.PHONY: all build test test-race test-coverage clean run fmt vet lint security gosec vulncheck check help install-lint-tools install-security-tools

# Default target
all: help

# Build the spanza binary
build:
	@echo "Building spanza..."
	go build -o spanza .

# Run all tests with verbose output
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -cover ./...

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -race ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f spanza
	go clean

# Build and run spanza
run: build
	@echo "Running spanza..."
	./spanza

# Format Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run go vet for static analysis
vet:
	@echo "Running go vet..."
	go vet ./...

# Install linting tools if needed
install-lint-tools:
	@echo "Installing linting tools..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)

# Run golangci-lint
lint: install-lint-tools
	@echo "Running golangci-lint..."
	golangci-lint run ./...

# Install security tools if needed
install-security-tools:
	@echo "Installing security scanning tools..."
	@which govulncheck > /dev/null || (echo "Installing govulncheck..." && go install golang.org/x/vuln/cmd/govulncheck@latest)
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securego/gosec/v2/cmd/gosec@latest)

# Run gosec security scanner
gosec: install-security-tools
	@echo "Running gosec security scanner..."
	gosec ./...

# Run vulnerability scanner
vulncheck: install-security-tools
	@echo "Running vulnerability scanner..."
	govulncheck ./...

# Run all security checks
security: gosec vulncheck
	@echo "Security scanning complete"

# Run all quality and security checks
check: fmt vet lint test test-race security

# Show available targets
help:
	@echo "Available targets:"
	@echo "  build         - Build the spanza binary"
	@echo "  test          - Run all tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  test-race     - Run tests with race detector"
	@echo "  clean         - Clean build artifacts"
	@echo "  run           - Build and run spanza"
	@echo "  fmt           - Format Go code"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run golangci-lint"
	@echo "  gosec         - Run gosec security scanner"
	@echo "  vulncheck     - Run vulnerability scanner"
	@echo "  security      - Run all security checks"
	@echo "  check         - Run fmt, vet, lint, test, test-race, and security"
	@echo "  help          - Show this help message"
