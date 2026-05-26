.PHONY: all fmt lint vet test test-race build clean coverage coverage-html

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE)

# Default target runs all major quality checks, tests, and compiles the binary
all: fmt vet test build

# Format all Go source files using gofmt
fmt:
	go fmt ./...

# Run standard go vet static analysis
vet:
	go vet ./...

# Run golangci-lint locally (requires golangci-lint installed on your system)
lint:
	golangci-lint run

lint-once:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run

# Run unit tests
test:
	go test -v ./...

# Run unit tests with the Go race detector enabled (matches CI checks)
test-race:
	go test -race -v ./...

# Run unit tests with coverage profile (matches CI)
coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic -v ./...

# Open html report in browser
coverage-html: coverage
	go tool cover -html=coverage.out


# Compile the highly optimized Go orchestrator binary
build:
	go build -ldflags="$(LDFLAGS)" -trimpath -o tradingagents cmd/tradingagents/main.go

# Clean compiled binaries
clean:
	rm -f tradingagents
