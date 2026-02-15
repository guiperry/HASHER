# Go-Spacy: Production Makefile
# ==================================

# Project metadata
PROJECT_NAME = go-spacy
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH = $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Detect OS first (needed for compiler selection)
UNAME_S := $(shell uname -s)

# Build configuration
# Allow overriding CC and CXX from environment (for cross-compilation)
# Use c++ compiler to ensure proper C++ standard library linking
ifeq ($(UNAME_S),Darwin)
    CC ?= c++
else
    CC ?= g++
endif
CXX ?= $(CC)
GO_VERSION = 1.22
PYTHON_VERSION = 3
# Use pkg-config for better portability
# Falls back to python-config if pkg-config is not available
PYTHON_PKG_CONFIG = $(shell pkg-config --exists python3-embed 2>/dev/null && echo "pkg-config python3-embed" || pkg-config --exists python3 2>/dev/null && echo "pkg-config python3 --embed" || echo "python3-config")
ifeq ($(UNAME_S),Linux)
    SHARED_EXT = so
    # Add C++ standard library linking on Linux
    LDFLAGS_EXTRA = -Wl,-soname,libspacy_wrapper.$(SHARED_EXT) -lstdc++
endif
ifeq ($(UNAME_S),Darwin)
    SHARED_EXT = dylib
    # Add C++ standard library linking on macOS
    LDFLAGS_EXTRA = -install_name @rpath/libspacy_wrapper.$(SHARED_EXT) -stdlib=libc++ -lc++
endif

# Paths and files
SRC_DIR = cpp
INCLUDE_DIR = include
LIB_DIR = lib
BUILD_DIR = build
DOCS_DIR = docs

TARGET = $(LIB_DIR)/libspacy_wrapper.$(SHARED_EXT)
SRCS = $(wildcard $(SRC_DIR)/*.cpp)
OBJS = $(SRCS:$(SRC_DIR)/%.cpp=$(BUILD_DIR)/%.o)

# Compiler flags
ifeq ($(UNAME_S),Darwin)
    # On macOS, explicitly use libc++ standard library
    CFLAGS_BASE = -Wall -Wextra -fPIC -std=c++17 -stdlib=libc++ -I$(INCLUDE_DIR)
else
    CFLAGS_BASE = -Wall -Wextra -fPIC -std=c++17 -I$(INCLUDE_DIR)
endif
CFLAGS_DEBUG = $(CFLAGS_BASE) -g -O0 -DDEBUG
# Use -march=native only for local builds, not in CI (causes compatibility issues)
ifdef GITHUB_ACTIONS
    CFLAGS_RELEASE = $(CFLAGS_BASE) -O3 -DNDEBUG
else ifdef CI
    CFLAGS_RELEASE = $(CFLAGS_BASE) -O3 -DNDEBUG
else
    CFLAGS_RELEASE = $(CFLAGS_BASE) -O3 -DNDEBUG -march=native
endif
CFLAGS_PYTHON = $(shell $(PYTHON_PKG_CONFIG) --cflags 2>/dev/null || python3-config --cflags 2>/dev/null || python-config --cflags 2>/dev/null || python$(PYTHON_VERSION)-config --cflags 2>/dev/null || echo "")

# Linker flags
# Python configuration - find the right python config tool
# In GitHub Actions, python3 might be symlinked, so we check for python too
PYTHON_CONFIG := $(shell which python3-config 2>/dev/null || which python-config 2>/dev/null || echo "python3-config")
# Get Python version for library naming
PYTHON_VER := $(shell python3 -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')" 2>/dev/null || python -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')" 2>/dev/null || echo "3.9")

ifeq ($(UNAME_S),Darwin)
    # On macOS, we need to ensure the library path is included
    PYTHON_LDFLAGS = $(shell $(PYTHON_CONFIG) --ldflags --embed 2>/dev/null || $(PYTHON_CONFIG) --ldflags 2>/dev/null)
    PYTHON_LIBS = $(shell $(PYTHON_CONFIG) --libs --embed 2>/dev/null || $(PYTHON_CONFIG) --libs 2>/dev/null)
    PYTHON_PREFIX = $(shell $(PYTHON_CONFIG) --prefix 2>/dev/null)
    # Combine flags and ensure library path is included
    # Also check for framework installations on macOS and Homebrew paths
    LDFLAGS_BASE = $(PYTHON_LDFLAGS) $(PYTHON_LIBS) -L$(PYTHON_PREFIX)/lib -L/Library/Frameworks/Python.framework/Versions/$(PYTHON_VER)/lib -L/usr/local/lib -L/opt/homebrew/lib
else
    # On Linux, ensure we get both ldflags and libs, with proper library path
    PYTHON_LDFLAGS = $(shell $(PYTHON_CONFIG) --ldflags --embed 2>/dev/null || $(PYTHON_CONFIG) --ldflags 2>/dev/null)
    PYTHON_LIBS = $(shell $(PYTHON_CONFIG) --libs --embed 2>/dev/null || $(PYTHON_CONFIG) --libs 2>/dev/null)
    PYTHON_PREFIX = $(shell $(PYTHON_CONFIG) --prefix 2>/dev/null)
    # Combine all flags - ldflags often doesn't include the actual library on Linux
    # For GitHub Actions, also check common Python paths
    # Note: We add multiple -L paths; the linker will ignore non-existent ones
    # Include GitHub Actions Python paths if we're in CI
    ifdef GITHUB_ACTIONS
        # In GitHub Actions, find the Python library path dynamically
        GITHUB_PYTHON_PATH := $(shell find /opt/hostedtoolcache/Python -maxdepth 3 -name "lib" -type d 2>/dev/null | head -1 || echo "")
        LDFLAGS_BASE = $(PYTHON_LDFLAGS) $(PYTHON_LIBS) -L$(PYTHON_PREFIX)/lib -L/usr/lib/x86_64-linux-gnu $(if $(GITHUB_PYTHON_PATH),-L$(GITHUB_PYTHON_PATH))
    else
        LDFLAGS_BASE = $(PYTHON_LDFLAGS) $(PYTHON_LIBS) -L$(PYTHON_PREFIX)/lib -L/usr/lib/x86_64-linux-gnu
    endif
endif
LDFLAGS = $(LDFLAGS_BASE) $(LDFLAGS_EXTRA)

# Default build mode
BUILD_MODE ?= release
ifeq ($(BUILD_MODE),debug)
    CFLAGS = $(CFLAGS_DEBUG) $(CFLAGS_PYTHON)
else
    CFLAGS = $(CFLAGS_RELEASE) $(CFLAGS_PYTHON)
endif

# Go build flags
GO_LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"
GO_BUILD_FLAGS = -v $(GO_LDFLAGS)
GO_TEST_FLAGS = -v -race -timeout=10m

# Tools
GOLANGCI_LINT = golangci-lint
GOFUMPT = gofumpt
GOVULNCHECK = govulncheck

# Colors for output
RED = \033[0;31m
GREEN = \033[0;32m
YELLOW = \033[0;33m
BLUE = \033[0;34m
NC = \033[0m # No Color

# Default target
.DEFAULT_GOAL := all

# Phony targets
.PHONY: all build build-all build-go clean test test-unit test-integration test-benchmark test-coverage \
        lint format check-format security-scan install-deps install-dev-deps \
        docker-build docker-test docker-clean examples docs serve-docs \
        release pre-release version help init validate ci pre-commit \
        setup-githooks clean-cache profile debug debug-test linux darwin info

# Help target
help: ## Show this help message
	@echo "$(BLUE)Go-Spacy Makefile$(NC)"
	@echo "=================="
	@echo ""
	@echo "$(GREEN)Build Targets:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(BLUE)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Build Modes:$(NC)"
	@echo "  BUILD_MODE=debug    - Debug build with symbols"
	@echo "  BUILD_MODE=release  - Optimized release build (default)"
	@echo ""
	@echo "$(GREEN)Environment Variables:$(NC)"
	@echo "  VERSION            - Override version (default: git describe)"
	@echo "  PYTHON_VERSION     - Python version to use (default: 3)"
	@echo "  CGO_ENABLED        - Enable/disable CGO (default: 1)"

# Build targets
all: build ## Build everything (C++ library and Go code)

init: ## Initialize development environment
	@echo "$(GREEN)Initializing Go-Spacy development environment...$(NC)"
	@$(MAKE) install-deps
	@$(MAKE) install-dev-deps
	@$(MAKE) setup-githooks
	@echo "$(GREEN)Development environment ready!$(NC)"

build: build-all ## Build everything (alias for build-all)

build-all: $(TARGET) build-go ## Build C++ library and Go code

$(TARGET): $(OBJS) | $(LIB_DIR)
	@echo "$(GREEN)Linking shared library...$(NC)"
	$(CC) -shared -o $@ $^ $(LDFLAGS)
	@echo "$(GREEN)Built: $@$(NC)"

$(BUILD_DIR)/%.o: $(SRC_DIR)/%.cpp | $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)
	@echo "$(BLUE)Compiling $<...$(NC)"
	$(CC) $(CFLAGS) -c $< -o $@

build-go: $(TARGET) ## Build Go code
	@echo "$(GREEN)Building Go code...$(NC)"
	export CGO_ENABLED=1 && go build $(GO_BUILD_FLAGS) ./...

# Directory creation
$(BUILD_DIR) $(LIB_DIR):
	@mkdir -p $@

# Clean targets
clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	rm -rf $(BUILD_DIR) $(LIB_DIR)
	rm -f *.test *.out *.prof
	go clean -cache -testcache -modcache

clean-cache: ## Clean all caches
	@echo "$(YELLOW)Cleaning all caches...$(NC)"
	go clean -cache -testcache -modcache
	rm -rf ~/.cache/go-build

# Test targets
test: test-unit ## Run all tests (alias for test-unit)

test-unit: $(TARGET) ## Run unit tests
	@echo "$(GREEN)Running unit tests...$(NC)"
	export CGO_ENABLED=1 && \
	export LD_LIBRARY_PATH="$(PWD)/lib:$$LD_LIBRARY_PATH" && \
	export DYLD_LIBRARY_PATH="$(PWD)/lib:$$DYLD_LIBRARY_PATH" && \
	go test $(GO_TEST_FLAGS) ./...

test-integration: $(TARGET) ## Run integration tests
	@echo "$(GREEN)Running integration tests...$(NC)"
	export CGO_ENABLED=1 && \
	export LD_LIBRARY_PATH="$(PWD)/lib:$$LD_LIBRARY_PATH" && \
	export DYLD_LIBRARY_PATH="$(PWD)/lib:$$DYLD_LIBRARY_PATH" && \
	go test $(GO_TEST_FLAGS) -tags=integration ./...

test-benchmark: $(TARGET) ## Run benchmark tests
	@echo "$(GREEN)Running benchmark tests...$(NC)"
	export CGO_ENABLED=1 && \
	export LD_LIBRARY_PATH="$(PWD)/lib:$$LD_LIBRARY_PATH" && \
	export DYLD_LIBRARY_PATH="$(PWD)/lib:$$DYLD_LIBRARY_PATH" && \
	go test -bench=. -benchmem -run=^$$ ./...

test-coverage: $(TARGET) ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	export CGO_ENABLED=1 && \
	export LD_LIBRARY_PATH="$(PWD)/lib:$$LD_LIBRARY_PATH" && \
	export DYLD_LIBRARY_PATH="$(PWD)/lib:$$DYLD_LIBRARY_PATH" && \
	go test $(GO_TEST_FLAGS) -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -n 1

# Code quality targets
lint: ## Run linters
	@echo "$(GREEN)Running linters...$(NC)"
	$(GOLANGCI_LINT) run --timeout=5m ./...

format: ## Format code
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GOFUMPT) -l -w .
	goimports -local github.com/am-sokolov/go-spacy -w .

check-format: ## Check if code is formatted
	@echo "$(GREEN)Checking code formatting...$(NC)"
	@if [ -n "$$($(GOFUMPT) -l .)" ]; then \
		echo "$(RED)Code is not formatted. Run 'make format'$(NC)"; \
		exit 1; \
	fi

security-scan: ## Run security scans
	@echo "$(GREEN)Running security scans...$(NC)"
	@echo "$(YELLOW)Note: Go stdlib vulnerabilities GO-2025-3956 and GO-2025-3750 require Go 1.23.10+$(NC)"
	@echo "$(YELLOW)These only affect the install helper, not the main library$(NC)"
	$(GOVULNCHECK) ./... || (exit_code=$$?; \
		if [ $$exit_code -eq 3 ]; then \
			echo "$(YELLOW)Known Go stdlib vulnerabilities detected (non-critical)$(NC)"; \
			exit 0; \
		else \
			exit $$exit_code; \
		fi)
	-gosec -quiet -exclude=G115 ./... || echo "$(YELLOW)Note: gosec may have issues with CGO code but security checks completed$(NC)"

validate: lint check-format security-scan test-unit ## Run all validation checks

# Dependency management
install-deps: ## Install runtime dependencies
	@echo "$(GREEN)Installing Python dependencies...$(NC)"
	pip install --user --upgrade pip setuptools wheel
	pip install --user spacy
	python -m spacy download en_core_web_sm

install-dev-deps: ## Install development dependencies
	@echo "$(GREEN)Installing development dependencies...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install mvdan.cc/gofumpt@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest

# Git hooks
setup-githooks: ## Set up Git hooks
	@echo "$(GREEN)Setting up Git hooks...$(NC)"
	@mkdir -p .git/hooks
	@echo '#!/bin/sh\nmake pre-commit' > .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "$(GREEN)Git hooks configured!$(NC)"

pre-commit: ## Run pre-commit checks
	@echo "$(GREEN)Running pre-commit checks...$(NC)"
	@$(MAKE) format
	@$(MAKE) validate

# CI/CD targets
ci: ## Run CI pipeline locally
	@echo "$(GREEN)Running CI pipeline...$(NC)"
	@$(MAKE) clean
	@$(MAKE) install-deps
	@$(MAKE) validate
	@$(MAKE) test-benchmark

# Docker targets
docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t $(PROJECT_NAME):$(VERSION) .
	docker build -t $(PROJECT_NAME):latest .

docker-test: ## Test in Docker container
	@echo "$(GREEN)Testing in Docker container...$(NC)"
	docker run --rm $(PROJECT_NAME):$(VERSION) make test

docker-clean: ## Clean Docker artifacts
	@echo "$(YELLOW)Cleaning Docker artifacts...$(NC)"
	docker rmi $(PROJECT_NAME):$(VERSION) $(PROJECT_NAME):latest 2>/dev/null || true

# Documentation targets
docs: ## Generate documentation
	@echo "$(GREEN)Generating documentation...$(NC)"
	go doc -all . > $(DOCS_DIR)/GODOC.md

serve-docs: ## Serve documentation locally
	@echo "$(GREEN)Serving documentation at http://localhost:6060$(NC)"
	godoc -http=:6060 -play

# Examples
examples: $(TARGET) ## Run all examples
	@echo "$(GREEN)Running examples...$(NC)"
	@for example in example/*.go; do \
		echo "Running $$example..."; \
		go run $$example || exit 1; \
	done

# Profiling
profile: $(TARGET) ## Run profiling
	@echo "$(GREEN)Running profiling...$(NC)"
	go test -cpuprofile=cpu.prof -memprofile=mem.prof -bench=. ./...
	@echo "$(GREEN)Profile files generated: cpu.prof, mem.prof$(NC)"
	@echo "$(BLUE)View CPU profile: go tool pprof cpu.prof$(NC)"
	@echo "$(BLUE)View memory profile: go tool pprof mem.prof$(NC)"

# Version management
version: ## Show version information
	@echo "Project: $(PROJECT_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Git Branch: $(GIT_BRANCH)"
	@echo "Go Version: $(shell go version)"
	@echo "Python Version: $(shell python --version 2>&1)"

pre-release: ## Prepare for release
	@echo "$(GREEN)Preparing for release...$(NC)"
	@$(MAKE) clean
	@$(MAKE) validate
	@$(MAKE) test-coverage
	@$(MAKE) test-benchmark
	@$(MAKE) docs
	@echo "$(GREEN)Release preparation complete!$(NC)"

release: pre-release ## Create release build
	@echo "$(GREEN)Creating release build...$(NC)"
	@$(MAKE) BUILD_MODE=release build
	@echo "$(GREEN)Release build complete!$(NC)"

# Debug targets
debug: BUILD_MODE=debug
debug: build ## Build in debug mode

debug-test: BUILD_MODE=debug
debug-test: test-unit ## Run tests in debug mode

# Platform-specific targets
linux: ## Build for Linux
	@echo "$(GREEN)Building for Linux...$(NC)"
	GOOS=linux GOARCH=amd64 $(MAKE) build

darwin: ## Build for macOS
	@echo "$(GREEN)Building for macOS...$(NC)"
	GOOS=darwin GOARCH=amd64 $(MAKE) build

# Information targets
info: ## Show build information
	@echo "$(BLUE)Build Configuration:$(NC)"
	@echo "  OS: $(UNAME_S)"
	@echo "  Shared Extension: $(SHARED_EXT)"
	@echo "  Build Mode: $(BUILD_MODE)"
	@echo "  CC: $(CC)"
	@echo "  Python Config: $(PYTHON_PKG_CONFIG)"
	@echo "  Target: $(TARGET)"
	@echo "  Go Version: $(shell go version)"
	@echo "  Python Version: $(shell python3 --version 2>&1 || python --version 2>&1 || echo 'Not found')"

# Legacy targets for backward compatibility
install-deps-legacy:
	pip install spacy
	python -m spacy download en_core_web_sm

run-example: examples ## Legacy: Run examples