# Hasher Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Driver Binary names
SERVER_BINARY=hasher-server
HOST_BINARY=hasher-host
BIN_DIR=bin

# Proto parameters
PROTO_DIR=internal/proto
PROTO_OUT=.

# eBPF parameters
EBPF_SRCS=internal/ebpf/tracer.bpf.c internal/ebpf/nonce_batcher.bpf.c
EBPF_OBJS=$(EBPF_SRCS:.c=.o)
CLANG=clang
LLVM_STRIP=llvm-strip


.PHONY: all build clean test proto ebpf deps help build-asic-test build-probe build-probe build-probe-v2 build-protocol-discover build-monitor build-diagnostics deploy deploy-probe deploy-probe-v2 deploy-protocol-discover deploy-monitor deploy-diagnostics cli test-cli build-all build-server-mips build-host build-host-all embed-binaries build-crypto-transformer train-crypto-transformer run-crypto-transformer build-simple-hash run-simple-hash build-pipeline-all build-dataminer build-data-encoder build-data-trainer

ANTMINER_IP ?= $(shell [ -f .env ] && source .env && echo $$DEVICE_IP || echo 192.168.12.151)
ANTMINER_USER ?= root
ANTMINER_PASSWORD ?= $(shell [ -f .env ] && source .env && echo $$DEVICE_PASSWORD || echo *********)

SDK_ROOT := $(CURDIR)/toolchain/openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64

# CLI Binary name
CLI_BINARY_NAME := hasher

# Build directory
CLI_BUILD_DIR := ./bin

# Source files
CLI_SRC_DIR := ./cmd/cli

# Test directories
CLI_TEST_DIRS := ./internal/cli/ui ./internal/cli/chat ./internal/client ./internal/crypto_transformer ./tests

# Build flags
GOFLAGS :=
LDFLAGS=-ldflags "-s -w"
TAGS=-tags netgo

help:
	@echo "🛡️  HASHER PoC - Development Commands"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
	@echo "Configuration:"
	@echo "  ANTMINER_IP=$(ANTMINER_IP)"
	@echo "  ANTMINER_USER=$(ANTMINER_USER)"

all: deps proto ebpf build

## proto: Generate Go code from Protocol Buffers
proto:
	@echo "Generating protobuf code..."
	@mkdir -p $(PROTO_OUT)
	protoc --go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/hasher/v1/*.proto

## ebpf: Compile eBPF programs
ebpf: $(EBPF_OBJS)

%.bpf.o: %.bpf.c
	@echo "Compiling eBPF program $<..."
	$(CLANG) -g -O2 -target bpf -D__TARGET_ARCH_x86 \
		-I/usr/include/bpf \
		-Iinternal/ebpf \
		-c $< -o $@
	$(LLVM_STRIP) -g $@

## generate: Generate all code (proto, ebpf, go generate)
generate: proto ebpf
	@echo "Running go generate..."
	$(GOCMD) generate ./...

## deps: Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## build-monitor: Build ASIC monitor tool with diagnostic capabilities
build-monitor:
	@echo "🔨 Building ASIC monitor tool for MIPS (static)..."
	@mkdir -p bin
	@export STAGING_DIR=$(SDK_ROOT)/staging_dir && \
	CGO_ENABLED=1 \
	CC=$(SDK_ROOT)/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl/bin/mips-openwrt-linux-musl-gcc \
	CGO_CFLAGS="-I$(SDK_ROOT)/staging_dir/target-mips_24kc_musl/usr/include" \
	CGO_LDFLAGS="-L$(SDK_ROOT)/staging_dir/target-mips_24kc_musl/usr/lib -lusb-1.0 -static" \
	GOOS=linux GOARCH=mips GOMIPS=softfloat \
	go build -ldflags '-extldflags "-static"' -o bin/monitor-mips cmd/monitor/main.go
	@echo "✅ Build complete"
	@ls -lh bin/monitor-mips

## build-monitor-diagnostics: Build monitor with diagnostics for MIPS (USB-free)
build-monitor-diagnostics:
	@echo "🔨 Building ASIC monitor diagnostics for MIPS (USB-free)..."
	@mkdir -p bin
	GOOS=linux GOARCH=mips GOMIPS=softfloat go build -ldflags "-s -w" \
		-tags "noudp" -o bin/monitor-diagnostics-mips \
		-ldflags '-X "main.DiagnosticMode=true"' cmd/monitor/main.go
	@echo "✅ Build complete"
	@ls -lh bin/monitor-diagnostics-mips

cli:
	@echo "Creating build directory..."
	@mkdir -p $(CLI_BUILD_DIR)
	@echo "Building $(CLI_BINARY_NAME) CLI Tool..."
	@go build $(GOFLAGS) -o $(CLI_BUILD_DIR)/$(CLI_BINARY_NAME) $(CLI_SRC_DIR)
	@echo "Binary created successfully at $(CLI_BUILD_DIR)/$(CLI_BINARY_NAME)"

test-cli:
	@echo "Running tests..."
	@for dir in $(CLI_TEST_DIRS); do \
		echo "Running tests in $$dir..."; \
		go test -v $$dir; \
	done

run:
	@echo "Running Hasher-Host..."
	@./bin/hasher-host   --device=192.168.12.151   --discover=false

deploy: build
	@./scripts/deploy.sh

deploy-server: build-server
	@echo "🚀 Deploying hasher-server to Antminer..."
	@sshpass -p '$(ANTMINER_PASSWORD)' scp -o KexAlgorithms=+diffie-hellman-group14-sha1 \
		-o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no \
		bin/hasher-server $(ANTMINER_USER)@$(ANTMINER_IP):/tmp/hasher-server
	@sshpass -p '$(ANTMINER_PASSWORD)' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
		-o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no \
		$(ANTMINER_USER)@$(ANTMINER_IP) 'chmod +x /tmp/hasher-server'
	@echo "✅ Deployed! Run with:"
	@echo "   sshpass -p '$(ANTMINER_PASSWORD)' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \\"
	@echo "     -o HostKeyAlgorithms=+ssh-rsa $(ANTMINER_USER)@$(ANTMINER_IP) '/tmp/hasher-server --dump-status --dump-interval 2'"

## deploy-monitor: Deploy monitor with diagnostics to ASIC device
deploy-monitor: build-monitor
	@echo "🚀 Deploying ASIC monitor to Antminer..."
	@sshpass -p '$(ANTMINER_PASSWORD)' scp -o KexAlgorithms=+diffie-hellman-group14-sha1 \
		-o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no \
		bin/monitor-mips $(ANTMINER_USER)@$(ANTMINER_IP):/tmp/monitor
	@sshpass -p '$(ANTMINER_PASSWORD)' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
		-o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no \
		$(ANTMINER_USER)@$(ANTMINER_IP) 'chmod +x /tmp/monitor'
	@echo "✅ Deployed! Run with:"
	@echo "   sshpass -p '$(ANTMINER_PASSWORD)' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \\"
	@echo "     -o HostKeyAlgorithms=+ssh-rsa $(ANTMINER_USER)@$(ANTMINER_IP) '/tmp/monitor --diagnostics'"

## deploy-monitor-diagnostics: Deploy monitor diagnostics (USB-free) to ASIC device
deploy-monitor-diagnostics: build-monitor-diagnostics
	@echo "🚀 Deploying monitor diagnostics (USB-free) to Antminer..."
	@sshpass -p '$(ANTMINER_PASSWORD)' scp -o KexAlgorithms=+diffie-hellman-group14-sha1 \
		-o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no \
		bin/monitor-diagnostics-mips $(ANTMINER_USER)@$(ANTMINER_IP):/tmp/monitor-diagnostics
	@sshpass -p '$(ANTMINER_PASSWORD)' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
		-o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no \
		$(ANTMINER_USER)@$(ANTMINER_IP) 'chmod +x /tmp/monitor-diagnostics'
	@echo "✅ Deployed! Run with:"
	@echo "   sshpass -p '$(ANTMINER_PASSWORD)' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \\"
	@echo "     -o HostKeyAlgorithms=+ssh-rsa $(ANTMINER_USER)@$(ANTMINER_IP) '/tmp/monitor-diagnostics --diagnostics'"

# test: Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BIN_DIR)
	rm -f $(EBPF_OUT)
	rm -f coverage.txt
	@echo "🧹 Cleaning build artifacts..."
	rm -rf bin/
	@echo "✅ Clean complete"

## run-server: Run the server
run-server: build-server
	@echo "Starting server..."
	sudo $(BIN_DIR)/$(SERVER_BINARY)

## run-host: Run the host
run-host: build-host
	@echo "Starting host..."
	$(BIN_DIR)/$(HOST_BINARY)

## docker-build: Build Docker image
docker-build:
	docker build -t hasher:latest .

## install: Install binaries to system
install: build
	@echo "Installing binaries..."
	sudo cp $(BIN_DIR)/$(SERVER_BINARY) /usr/local/bin/
	cp $(BIN_DIR)/$(HOST_BINARY) /usr/local/bin/

## lint: Run linters
lint:
	@echo "Running linters..."
	golangci-lint run ./...

# ============================================================================
# Build targets for embedded binaries
# ============================================================================

EMBED_DIR=internal/cli/embedded/bin

## build-server-mips: Build hasher-server for MIPS (Antminer) with static linking
build-server-mips:
	@echo "🔨 Building hasher-server for MIPS (static)..."
	@mkdir -p $(BIN_DIR)
	@export STAGING_DIR=$(SDK_ROOT)/staging_dir && \
	CGO_ENABLED=1 \
	CC=$(SDK_ROOT)/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl/bin/mips-openwrt-linux-musl-gcc \
	CGO_CFLAGS="-I$(SDK_ROOT)/staging_dir/target-mips_24kc_musl/usr/include" \
	CGO_LDFLAGS="-L$(SDK_ROOT)/staging_dir/target-mips_24kc_musl/usr/lib -static" \
	GOOS=linux GOARCH=mips GOMIPS=softfloat \
	$(GOBUILD) -ldflags '-s -w -extldflags "-static"' -o $(BIN_DIR)/hasher-server-mips ./cmd/driver/hasher-server
	@echo "✅ Build complete: $(BIN_DIR)/hasher-server-mips"
	@ls -lh $(BIN_DIR)/hasher-server-mips

# New target for embedding hasher-server into hasher-host
embed-host-server-mips: build-server-mips
	@echo "Embedding hasher-server-mips into hasher-host..."
	@mkdir -p internal/host/embedded
	@cp $(BIN_DIR)/hasher-server-mips internal/host/embedded/hasher-server-mips
	@echo "✅ hasher-server-mips prepared for host embedding."

## build-host-linux-amd64: Build hasher-host for Linux x86_64
build-host-linux-amd64:
	@echo "🔨 Building hasher-host for Linux amd64..."
	@mkdir -p $(BIN_DIR) $(EMBED_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/hasher-host-linux-amd64 ./cmd/driver/hasher-host
	@cp $(BIN_DIR)/hasher-host-linux-amd64 $(EMBED_DIR)/
	@echo "✅ Build complete: $(BIN_DIR)/hasher-host-linux-amd64"

## build-host-darwin-amd64: Build hasher-host for macOS Intel
build-host-darwin-amd64:
	@echo "🔨 Building hasher-host for macOS amd64..."
	@mkdir -p $(BIN_DIR) $(EMBED_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/hasher-host-darwin-amd64 ./cmd/driver/hasher-host
	@cp $(BIN_DIR)/hasher-host-darwin-amd64 $(EMBED_DIR)/

## build-crypto-transformer: Build cryptographic transformer for local testing
build-crypto-transformer:
	@echo "🔐 Building cryptographic transformer..."
	@mkdir -p bin
	@go build -ldflags "-s -w" -o bin/crypto-transformer cmd/crypto_transformer/main.go
	@echo "✅ Crypto-transformer binary created at bin/crypto-transformer"
	@ls -lh bin/crypto-transformer

## train-crypto-transformer: Train cryptographic transformer model
train-crypto-transformer:
	@echo "🎯 Training cryptographic transformer..."
	@cd cmd/crypto_transformer && go run main.go --train

## run-crypto-transformer: Run cryptographic transformer demo
run-crypto-transformer:
	@echo "💬 Starting cryptographic transformer demo..."
	@./bin/crypto-transformer

## run-simple-hash: Run simple hash test
run-simple-hash:
	@echo "🧪 Starting simple hash test..."
	@./bin/simple-hash
	@echo "✅ Build complete: $(BIN_DIR)/hasher-host-darwin-amd64"

## build-host-darwin-arm64: Build hasher-host for macOS Apple Silicon
build-host-darwin-arm64:
	@echo "🔨 Building hasher-host for macOS arm64..."
	@mkdir -p $(BIN_DIR) $(EMBED_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/hasher-host-darwin-arm64 ./cmd/driver/hasher-host
	@cp $(BIN_DIR)/hasher-host-darwin-arm64 $(EMBED_DIR)/
	@echo "✅ Build complete: $(BIN_DIR)/hasher-host-darwin-arm64"

## build-host: Build hasher-host for current platform
build-host:
	@echo "🔨 Building hasher-host for current platform..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(HOST_BINARY) ./cmd/driver/hasher-host
	# This copy is for CLI embedding, so it remains here
	@cp $(BIN_DIR)/hasher-host $(EMBED_DIR)/
	@echo "✅ Build complete: $(BIN_DIR)/$(HOST_BINARY)"

## build-host-all: Build hasher-host for all platforms
build-host-all: build-host-linux-amd64
	@echo "✅ All hasher-host binaries built"

## build-all: Build all binaries for embedding
build-driver: embed-host-server-mips build-host
	@echo "✅ All binaries built and ready for embedding"
	@echo "📦 Embedded binaries in: $(EMBED_DIR)"
	@ls -la $(EMBED_DIR)/

## embed-binaries: Build all and then rebuild CLI with embedded binaries
embed-binaries: build-driver
	@echo "🔨 Copying binaries to CLI embed directory..."
	@cp $(BIN_DIR)/hasher-host $(EMBED_DIR)/
	@cp $(BIN_DIR)/hasher-server-mips internal/host/embedded/hasher-server-mips
	@echo "✅ Build complete: $(BIN_DIR)/"

# ============================================================================
# Pipeline binaries build targets
# ============================================================================

PIPELINE_DIR := pipeline

## build-pipeline: Build all pipeline binaries
define build-pipeline-binary
	@echo "🔨 Building $(1) from $(2)..."
	@mkdir -p $(EMBED_DIR)
	cd $(PIPELINE_DIR)/$(2) && go build -ldflags "-s -w" -o ../../../$(EMBED_DIR)/$(1) $(3)
	@echo "✅ Built: $(EMBED_DIR)/$(1)"
endef

## build-dataminer: Build data miner pipeline binary
build-dataminer:
	$(call build-pipeline-binary,dataminer,1_DATA_MINER,./cmd/dataminer)

## build-data-encoder: Build data encoder pipeline binary
build-data-encoder:
	$(call build-pipeline-binary,data-encoder,2_DATA_ENCODER,.)

## build-data-trainer: Build data trainer pipeline binary
build-data-trainer:
	$(call build-pipeline-binary,data-trainer,3_DATA_TRAINER,./cmd/trainer)

## build-pipeline-all: Build all pipeline binaries
build-pipeline-all: build-dataminer build-data-encoder build-data-trainer
	@echo "✅ All pipeline binaries built"
	@echo "📦 Pipeline binaries in: $(EMBED_DIR)"
	@ls -lh $(EMBED_DIR)/


## build: Build all components
build: generate build-driver build-pipeline-all cli
	@echo "✅ All components built"

## test-training: Test training functionality
test-training:
	@echo "🎯 Testing training functionality..."
	@echo "Starting hasher-host with training API..."
	@$(BIN_DIR)/$(HOST_BINARY) --port 8081 &
	@sleep 2
	@echo "Testing training API call..."
	@curl -X POST http://localhost:8081/api/v1/train \
		-H "Content-Type: application/json" \
		-d '{"epochs": 1, "learning_rate": 0.001, "batch_size": 32, "data_samples": ["test sample"]}' \
		2>/dev/null || echo "❌ Training API test failed"
	@pkill -f hasher-host || true
	@echo "✅ Training test complete"

.DEFAULT_GOAL := help
