# asic-driver Dockerfile - Multi-stage build

# Stage 1: Build eBPF programs
FROM ubuntu:22.04 AS ebpf-builder

RUN apt-get update && apt-get install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY internal/ebpf/ ./internal/ebpf/

RUN clang -g -O2 -target bpf -D__TARGET_ARCH_x86 \
    -I/usr/include/bpf \
    -c internal/ebpf/asic-driver.bpf.c -o internal/ebpf/asic-driver.bpf.o && \
    llvm-strip -g internal/ebpf/asic-driver.bpf.o

# Stage 2: Build Go binaries
FROM golang:1.21-alpine AS go-builder

RUN apk add --no-cache \
    git \
    make \
    protobuf \
    protobuf-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .
COPY --from=ebpf-builder /build/internal/ebpf/asic-driver.bpf.o ./internal/ebpf/

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /bin/asic-driver-server ./cmd/asic-driver-server

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /bin/asic-driver-client ./cmd/asic-driver-client

# Stage 3: Runtime image
FROM ubuntu:22.04

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    libbpf0 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy binaries
COPY --from=go-builder /bin/asic-driver-server /usr/local/bin/
COPY --from=go-builder /bin/asic-driver-client /usr/local/bin/
COPY --from=ebpf-builder /build/internal/ebpf/asic-driver.bpf.o /opt/asic-driver/

# Create non-root user (server needs root for eBPF)
RUN useradd -m -u 1000 asic-driver

WORKDIR /home/asic-driver

# Expose gRPC port
EXPOSE 80

# Default command
CMD ["asic-driver-server"]
