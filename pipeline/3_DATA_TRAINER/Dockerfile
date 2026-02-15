# Multi-stage Dockerfile for HASHER Data Trainer with BM1382 "Camouflage" support
# Stage 1: Build stage with CUDA and uBPF compilation
FROM nvidia/cuda:12.2.0-devel-ubuntu22.04 AS builder

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    clang \
    llvm \
    libelf-dev \
    libpfm4-dev \
    make \
    wget \
    curl \
    cmake \
    libboost-all-dev \
    libbpf-dev

# Install Go (reuse version from training app)
RUN wget -O go.tar.gz https://go.dev/dl/go1.21.0.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"

# Build uBPF VM from source
RUN git clone https://github.com/iovisor/ubpf.git /tmp/ubpf && \
    cd /tmp/ubpf && \
    git submodule update --init --recursive && \
    cmake -S . -B build -DUBPF_ENABLE_TESTS=true && \
    cmake --build build && \
    cp build/src/libubpf.so /usr/local/lib/ || cp build/libubpf.so /usr/local/lib/ && \
    cp src/inc/ubpf.h /usr/local/include/ && \
    ldconfig

# Create workspace
WORKDIR /app

# Copy source code
COPY pkg/simulator/ebpf_maps.h ./pkg/simulator/
COPY pkg/simulator/neural_kernel.c ./pkg/simulator/
COPY pkg/simulator/hardware_prep.go ./pkg/simulator/
COPY pkg/training/ ./pkg/training/
COPY cmd/data-trainer/ ./cmd/data-trainer/
COPY internal/ ./internal/
COPY go.mod go.sum ./
COPY pkg/simulator/cuda_bridge.cu ./pkg/simulator/

# Compile eBPF bytecode
RUN clang -O2 -target bpf -c pkg/simulator/neural_kernel.c -o pkg/simulator/neural_kernel.o

# Compile CUDA bridge to shared library
RUN nvcc -Xcompiler -fPIC -shared -o pkg/simulator/libcuda_bridge.so pkg/simulator/cuda_bridge.cu

# Stage 2: Runtime stage with Go application
FROM nvidia/cuda:12.2.0-runtime-ubuntu22.04

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    libelf1 \
    libpfm4

# Copy compiled artifacts from builder stage
COPY --from=builder /usr/local/go /usr/local/go
COPY --from=builder /tmp/ubpf/vm/ubpf /usr/local/lib/
COPY --from=builder /app/pkg/simulator/neural_kernel.o ./pkg/simulator/
COPY --from=builder /app/pkg/simulator/libcuda_bridge.so ./pkg/simulator/
COPY --from=builder /app/pkg/simulator/ebpf_maps.h ./pkg/simulator/
COPY --from=builder /app/pkg/simulator/hardware_prep.go ./pkg/simulator/
COPY --from=builder /app/pkg/simulator/cuda_bridge.cu ./pkg/simulator/
COPY --from=builder /app/pkg/training/ ./pkg/training/
COPY --from=builder /app/internal/ ./internal/
COPY --from=builder /app/cmd/data-trainer/ ./cmd/data-trainer/
COPY --from=builder /app/go.mod ./go.mod
COPY --from=builder /app/go.sum ./go.sum

# Set environment variables for execution
ENV PATH="/usr/local/go/bin:${PATH}"
ENV LD_LIBRARY_PATH="/usr/local/cuda/lib64:/usr/local/lib"
ENV CUDA_VISIBLE_DEVICES=0

# Expose BPF maps for the Evolutionary GRPO Harness
VOLUME /sys/fs/bpf

# Data volume for training data
VOLUME /app/data

# Build the Go trainer application
RUN cd /app && \
    /usr/local/go/bin/go build -ldflags "-w -s" -o /app/bin/data-trainer ./cmd/data-trainer/

# Set working directory
WORKDIR /app

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD /app/bin/data-trainer --version || exit 1

# Entry point
ENTRYPOINT ["/app/bin/data-trainer"]

# Labels for container management
LABEL maintainer="HASHER Data Trainer Team"
LABEL version="1.0.0"
LABEL description="HASHER Data Trainer with BM1382 'Camouflage' Bitcoin header support"
LABEL cuda.version="12.2.0"
LABEL go.version="1.21.0"

# Default configuration
ENV DATA_PATH=/app/data
ENV LOG_LEVEL=info
ENV ENABLE_CUDA=true
ENV ENABLE_EBPF=true
ENV TARGET_HASH_RATE=500000000
ENV POPULATION_SIZE=128
ENV MAX_GENERATIONS=500