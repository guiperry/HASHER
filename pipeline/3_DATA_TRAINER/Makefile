.PHONY: all build test clean install run lint format help toolchain build-go docker-openwrt build-cuda firmware cuda

# CUDA Configuration
CUDA_DIR := ../../pkg/hashing/methods/cuda
CUDA_LIB := $(CUDA_DIR)/libcuda_hash.so
CUDA_SRC := $(CUDA_DIR)/cuda_bridge.cu
NVCC := /usr/local/cuda-12.6/bin/nvcc
CUDA_ARCH := sm_52
CUDA_FLAGS := -shared -Xcompiler -fPIC -arch=$(CUDA_ARCH) -O3

# Application name 
APP_NAME := data-trainer
MAIN_PATH := cmd/data-trainer

# Go build flags
BUILD_FLAGS := -v
LDFLAGS := -ldflags "-X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

# Directories
BUILD_DIR := bin
COVERAGE_DIR := coverage

# Go files
GO_FILES := $(shell find . -name "*.go" -type f)
GO_MOD_FILES := go.mod go.sum

# Go tool versions
GO_VERSION := 1.21
GOLANGCI_LINT_VERSION := v1.54.2

# CUDA is configured above


help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

all: build ## Build the application

build: $(CUDA_LIB) ## Build the application binary (includes CUDA)
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./$(MAIN_PATH)

build-debug: ## Build the application with debug symbols
	@echo "Building $(APP_NAME) (debug)..."
	@mkdir -p $(BUILD_DIR)
	go build -gcflags="all=-N -l" $(BUILD_FLAGS) -o $(BUILD_DIR)/$(APP_NAME)-debug ./$(MAIN_PATH)

run: build ## Build and run the application
	@echo "Running $(APP_NAME)..."
	@LD_LIBRARY_PATH=$(CUDA_DIR):$$LD_LIBRARY_PATH ./$(BUILD_DIR)/$(APP_NAME)

run-debug: build-debug ## Run the debug version
	@echo "Running $(APP_NAME) (debug)..."
	@LD_LIBRARY_PATH=$(CUDA_DIR):$$LD_LIBRARY_PATH ./$(BUILD_DIR)/$(APP_NAME)-debug

test: ## Run all tests
	@echo "Running tests..."
	go test -v -race -coverprofile=$(COVERAGE_DIR)/coverage.out ./...

test-coverage: test ## Generate test coverage report
	@echo "Generating coverage report..."
	go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report generated: $(COVERAGE_DIR)/coverage.html"

test-bench: ## Run benchmarks
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

lint: ## Run linter
	@echo "Running golangci-lint..."
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci-lint/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	golangci-lint run

format: ## Format Go code
	@echo "Formatting Go code..."
	go fmt ./...
	goimports -w .

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

tidy: ## Tidy go modules
	@echo "Tidying go modules..."
	go mod tidy

clean: cuda-clean ## Clean all build artifacts
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	rm -f coverage.out
	go clean -cache


deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

update-deps: ## Update dependencies
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

install: build ## Install the application
	@echo "Installing $(APP_NAME)..."
	cp $(BUILD_DIR)/$(APP_NAME) $(GOPATH)/bin/




# Build uBPF locally to ensure it matches your host architecture
toolchain:
	bash setup_toolchain.sh
	



# HASHER Firmware Build


# Build the artifacts using the local host toolchain (CUDA 10.2 + GCC)
build-local:
	bash setup_toolchain.sh

# Package into a lean OpenWRT Rootfs for testing
firmware: build-local
	docker build -t hasher-openwrt:latest -f Dockerfile.openwrt .

# Run the OpenWRT instance with host GPU access
run-test:
	docker run --rm --runtime=nvidia --gpus all \
		-v $(PWD)/data:/app/data \
		hasher-openwrt:latest

# Docker development targets

docker-build-native: ## Build Docker image to compile native components
	@echo "Building Docker image for native component compilation..."
	docker build -t $(APP_NAME):latest .

dev-docker: ## Run development environment in Docker
	docker-compose up --build

dev-docker-bg: ## Run development environment in background
	docker-compose up --build -d

dev-docker-stop: ## Stop development environment
	docker-compose down

dev-docker-logs: ## Show development logs
	docker-compose logs -f

docker-run: ## Run in Docker
	@echo "Running $(APP_NAME) in Docker..."
	docker run --rm -it $(APP_NAME):latest

docker-push: ## Push Docker image to registry
	@echo "Pushing Docker image..."
	docker push $(APP_NAME):latest

docker-clean: ## Clean Docker artifacts
	@echo "Cleaning Docker artifacts..."
	docker system prune -f
	docker volume prune -f

profile-cpu: ## Run CPU profiling
	@echo "Running CPU profiling..."
	./$(BUILD_DIR)/$(APP_NAME) -cpuprofile=$(COVERAGE_DIR)/cpu.prof
	go tool pprof $(BUILD_DIR)/$(APP_NAME) $(COVERAGE_DIR)/cpu.prof

profile-mem: ## Run memory profiling
	@echo "Running memory profiling..."
	./$(BUILD_DIR)/$(APP_NAME) -memprofile=$(COVERAGE_DIR)/mem.prof
	go tool pprof $(BUILD_DIR)/$(APP_NAME) $(COVERAGE_DIR)/mem.prof

race: ## Run with race detector
	@echo "Running with race detector..."
	go run -race ./$(MAIN_PATH)

mod-check: ## Check for vulnerable dependencies
	@echo "Checking for vulnerable dependencies..."
	go list -json -m all | nancy sleuth

security-scan: ## Run security scan
	@echo "Running security scan..."
	gosec ./...

docs: ## Generate documentation
	@echo "Generating documentation..."
	godoc -http=:6060 &
	@echo "Documentation available at http://localhost:6060"

pre-commit: test lint vet ## Run all pre-commit checks
	@echo "Pre-commit checks completed"

ci: clean deps test lint vet ## Run CI pipeline
	@echo "CI pipeline completed"

check-deps: ## Check for outdated dependencies
	@echo "Checking for outdated dependencies..."
	go list -u -m all

generate: ## Run go generate
	@echo "Running go generate..."
	go generate ./...

tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	go install github.com/sonatypecommunity/nancy/cmd/nancy@latest

# Development shortcuts
dev: format tidy test build ## Full development cycle

quick: format build ## Quick build with formatting

watch: ## Watch for changes and rebuild
	@echo "Watching for changes..."
	@while inotifywait -r -e modify,create,delete .; do \
		echo "Changes detected, rebuilding..."; \
		make build; \
	done

# Release targets
release: clean test lint vet build ## Full release build
	@echo "Release build completed"

version: ## Show version information
	@echo "Go version: $(shell go version)"
	@echo "App version: $(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"
	@echo "Build time: $(shell date)"

# Project-specific targets
trainer: build ## Build trainer specifically
	@echo "Building trainer..."
	@mkdir -p $(BUILD_DIR)
	go build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/data-trainer ./cmd/data-trainer

run-trainer: trainer ## Run trainer
	@echo "Running trainer..."
	./$(BUILD_DIR)/data-trainer

validate: ## Validate training data
	@echo "Validating training data..."
	go run ./cmd/validator --data-path ./data

benchmark: test-bench ## Run performance benchmarks


# 2. Build the Go Trainer (Links to local libubpf.so)
build-go:
	@echo "Building HASHER Data Trainer..."
	go build -o bin/data-trainer ./cmd/data-trainer/

# 3. Build the Lean OpenWRT Testbed
docker-openwrt:
	@echo "Packaging into OpenWRT Rootfs..."
	docker build -t hasher-openwrt:latest -f Dockerfile.openwrt .

# CUDA Build Targets
cuda: $(CUDA_LIB) ## Build CUDA library

cuda-check: ## Check CUDA availability
	@echo "Checking CUDA availability..."
	@$(NVCC) --version
	@nvidia-smi --query-gpu=name,memory.total --format=csv,noheader

$(CUDA_LIB): $(CUDA_SRC)
	@echo "Building CUDA library..."
	@echo "NVCC: $(NVCC)"
	@echo "Architecture: $(CUDA_ARCH)"
	@cd $(CUDA_DIR) && $(NVCC) $(CUDA_FLAGS) -o libcuda_hash.so cuda_bridge.cu
	@echo "CUDA library built: $(CUDA_LIB)"

cuda-clean: ## Clean CUDA build artifacts
	@echo "Cleaning CUDA library..."
	@rm -f $(CUDA_LIB)

# Build everything including CUDA
build-all: cuda build ## Build CUDA library and Go binary

# Run with CUDA
run-cuda: build-all ## Build and run with CUDA acceleration
	@echo "Running with CUDA acceleration..."
	@export LD_LIBRARY_PATH=$(CUDA_DIR):$$LD_LIBRARY_PATH && ./$(BUILD_DIR)/$(APP_NAME) -hash-method=cuda

# Run with auto-detection (CUDA if available)
run-auto: build-all ## Build and run with auto-detected hash method
	@echo "Running with auto-detected hash method..."
	@export LD_LIBRARY_PATH=$(CUDA_DIR):$$LD_LIBRARY_PATH && ./$(BUILD_DIR)/$(APP_NAME)

# Default target
.DEFAULT_GOAL := help