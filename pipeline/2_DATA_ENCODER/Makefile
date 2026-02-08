# Makefile for Data Encoder

# Variables
BINARY_NAME=data-encoder
MAIN_PACKAGE=.
GO_FILES=$(shell find . -name "*.go" -type f)
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Default target
.PHONY: all
all: clean build

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) $(MAIN_PACKAGE)

# Build for current platform only
.PHONY: build-local
build-local:
	@echo "Building $(BINARY_NAME) for local platform..."
	go build -o bin/$(BINARY_NAME) $(MAIN_PACKAGE)

# Build for multiple platforms
.PHONY: build-all
build-all: build-linux build-darwin build-windows

# Build for Linux
.PHONY: build-linux
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)

# Build for macOS
.PHONY: build-darwin
build-darwin:
	@echo "Building $(BINARY_NAME) for macOS..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)

# Build for Windows
.PHONY: build-windows
build-windows:
	@echo "Building $(BINARY_NAME) for Windows..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run the program with default settings
.PHONY: run
run: build
	@echo "Running $(BINARY_NAME) with default settings..."
	./bin/$(BINARY_NAME)

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
.PHONY: lint
lint:
	@echo "Linting code..."
	golangci-lint run

# Vet code
.PHONY: vet
vet:
	@echo "Vetting code..."
	go vet ./...

# Check for vulnerabilities
.PHONY: security
security:
	@echo "Checking for security vulnerabilities..."
	gosec ./...

# Full quality check
.PHONY: check
check: fmt vet lint test
	@echo "All checks completed."

# Create release directory
.PHONY: release
release: clean build-all
	@echo "Creating release package..."
	mkdir -p release
	cp bin/$(BINARY_NAME)-* release/
	tar -czf release/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz -C release $(BINARY_NAME)-linux-amd64
	tar -czf release/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz -C release $(BINARY_NAME)-darwin-amd64
	tar -czf release/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz -C release $(BINARY_NAME)-darwin-arm64
	zip -j release/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip release/$(BINARY_NAME)-windows-amd64.exe
	@echo "Release packages created in release/ directory."

# Install binary to system
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp bin/$(BINARY_NAME) /usr/local/bin/

# Uninstall from system
.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(BINARY_NAME) from /usr/local/bin..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)

# Show help
.PHONY: help
help:
	@echo "Data Encoder Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build        - Build the encoder binary for current platform"
	@echo "  build-local  - Build without version info for local development"
	@echo "  build-all    - Build for all supported platforms"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  run          - Build and run with default settings"
	@echo "  deps         - Install and tidy dependencies"
	@echo "  clean        - Remove build artifacts"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  vet          - Run go vet"
	@echo "  security     - Run security check"
	@echo "  check        - Run full quality checks (fmt, vet, lint, test)"
	@echo "  release      - Create release packages"
	@echo "  install      - Install binary to /usr/local/bin"
	@echo "  uninstall    - Remove binary from /usr/local/bin"
	@echo "  help         - Show this help message"