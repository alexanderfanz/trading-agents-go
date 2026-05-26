.PHONY: all fmt lint vet test test-race build clean

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

# Run unit tests
test:
	go test -v ./...

# Run unit tests with the Go race detector enabled (matches CI checks)
test-race:
	go test -race -v ./...

# Compile the highly optimized Go orchestrator binary
build:
	go build -o tradingagents cmd/tradingagents/main.go

# Clean compiled binaries
clean:
	rm -f tradingagents
