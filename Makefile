# Makefile for go-mobilizon-bot
# Provides convenient commands for testing, building, and development

.PHONY: help test test-unit test-integration test-all coverage lint build clean install run

# Default target
.DEFAULT_GOAL := help

## help: Display this help message
help:
	@echo "go-mobilizon-bot Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""

## test: Run unit tests (fast, default)
test: test-unit

## test-unit: Run unit tests only (no integration tests)
test-unit:
	@echo "Running unit tests..."
	go test -v -race -count=1 ./...

## test-integration: Run integration tests only (requires network)
test-integration:
	@echo "Running integration tests..."
	@echo "Note: This will make real API calls to ConcertCloud"
	go test -v -race -tags=integration -count=1 ./...

## test-all: Run both unit and integration tests
test-all: test-unit test-integration

## test-short: Run tests in short mode (skip slow tests)
test-short:
	@echo "Running tests in short mode..."
	go test -short -race ./...

## coverage: Generate test coverage report
coverage:
	@echo "Generating coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## coverage-integration: Generate coverage report including integration tests
coverage-integration:
	@echo "Generating coverage report with integration tests..."
	go test -tags=integration -coverprofile=coverage-integration.out ./...
	go tool cover -html=coverage-integration.out -o coverage-integration.html
	@echo "Integration coverage report generated: coverage-integration.html"

## coverage-combined: Generate combined coverage report
coverage-combined:
	@echo "Generating combined coverage report..."
	@# Run unit tests
	go test -coverprofile=coverage-unit.out ./...
	@# Run integration tests
	go test -tags=integration -coverprofile=coverage-integration.out ./...
	@# Combine coverage files (requires gocovmerge)
	@if command -v gocovmerge >/dev/null 2>&1; then \
		gocovmerge coverage-unit.out coverage-integration.out > coverage-combined.out; \
		go tool cover -html=coverage-combined.out -o coverage-combined.html; \
		echo "Combined coverage report generated: coverage-combined.html"; \
	else \
		echo "gocovmerge not found. Install with: go install github.com/wadey/gocovmerge@latest"; \
	fi

## lint: Run linters (requires golangci-lint)
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install from: https://golangci-lint.run/usage/install/"; \
	fi

## fmt: Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## tidy: Tidy go.mod
tidy:
	@echo "Tidying go.mod..."
	go mod tidy

## build: Build the bot binary
build:
	@echo "Building go-mobilizon-bot..."
	go build -o bin/go-mobilizon-bot .

## build-all: Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build -o bin/go-mobilizon-bot-linux-amd64 .
	GOOS=darwin GOARCH=amd64 go build -o bin/go-mobilizon-bot-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o bin/go-mobilizon-bot-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -o bin/go-mobilizon-bot-windows-amd64.exe .
	@echo "Binaries built in bin/"

## install: Install the bot binary to $GOPATH/bin
install:
	@echo "Installing go-mobilizon-bot..."
	go install .

## clean: Clean build artifacts and test caches
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage*.out coverage*.html
	go clean -testcache
	go clean -cache

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download

## deps-update: Update dependencies
deps-update:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test

## ci: Run tests as CI would (unit + integration)
ci:
	@echo "Running CI checks..."
	@# Format check
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Code is not formatted. Run 'make fmt'"; \
		exit 1; \
	fi
	@# Vet
	go vet ./...
	@# Unit tests
	go test -v -race -coverprofile=coverage.out ./...
	@# Integration tests (allow to fail in CI)
	-go test -v -race -tags=integration ./...

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

## bench-integration: Run integration benchmarks
bench-integration:
	@echo "Running integration benchmarks..."
	go test -tags=integration -bench=. -benchmem ./...

## watch: Watch for changes and run tests (requires entr or similar)
watch:
	@if command -v entr >/dev/null 2>&1; then \
		find . -name '*.go' | entr -c make test-unit; \
	else \
		echo "entr not found. Install with: brew install entr (macOS) or apt install entr (Linux)"; \
	fi

## run: Run the bot (requires configuration)
run: build
	@echo "Running bot..."
	./bin/go-mobilizon-bot

## run-dev: Run the bot with debug logging
run-dev: build
	@echo "Running bot in debug mode..."
	./bin/go-mobilizon-bot --debug

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t go-mobilizon-bot:latest .

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run --rm -v $(PWD)/config:/config go-mobilizon-bot:latest

# Development helpers
## dev-setup: Set up development environment
dev-setup:
	@echo "Setting up development environment..."
	@# Install development tools
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/wadey/gocovmerge@latest
	@# Download dependencies
	go mod download
	@echo "Development environment ready!"

## init: Initialize a new environment (config directories, etc.)
init:
	@echo "Initializing environment..."
	mkdir -p ~/.config/mobilizon
	@echo "Config directory created at ~/.config/mobilizon"
	@echo "Run './bin/go-mobilizon-bot --register' to register the bot"

# Quick test commands for specific packages
## test-concertcloud: Test concertcloud package only
test-concertcloud:
	@echo "Testing concertcloud package..."
	go test -v -race ./concertcloud/...

## test-mobilizon: Test mobilizon package only
test-mobilizon:
	@echo "Testing mobilizon package..."
	go test -v -race ./mobilizon/...

## test-concertcloud-integration: Test concertcloud integration tests
test-concertcloud-integration:
	@echo "Testing concertcloud integration..."
	go test -v -race -tags=integration ./concertcloud/...

## test-mobilizon-integration: Test mobilizon integration tests
test-mobilizon-integration:
	@echo "Testing mobilizon integration..."
	go test -v -race -tags=integration ./mobilizon/...

# Specific commands for the bot
## register: Register the bot with Mobilizon
register: build
	@echo "Registering bot..."
	./bin/go-mobilizon-bot --register

## authorize: Authorize the bot with Mobilizon
authorize: build
	@echo "Authorizing bot..."
	./bin/go-mobilizon-bot --authorize

## sync: Sync events from ConcertCloud to Mobilizon
sync: build
	@echo "Syncing events..."
	@echo "Example: ./bin/go-mobilizon-bot --city=Lausanne --limit=100"
	@echo "Run with specific parameters to sync events"

# Information commands
## version: Show Go version
version:
	@go version

## info: Show project information
info:
	@echo "Project: go-mobilizon-bot"
	@echo "Go version: $$(go version)"
	@echo "Dependencies:"
	@go list -m all
