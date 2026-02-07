# hasher-host Dockerfile

# Stage 1: Build Go binary (using Ubuntu to match runtime)
FROM ubuntu:22.04 AS go-builder

# Install dependencies
RUN apt-get update && apt-get install -y \
    git \
    make \
    protobuf-compiler \
    gcc \
    g++ \
    libusb-1.0-0-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.24
RUN apt-get update && apt-get install -y wget && rm -rf /var/lib/apt/lists/* && \
    wget -O go1.24.11.linux-amd64.tar.gz https://go.dev/dl/go1.24.11.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.24.11.linux-amd64.tar.gz && \
    rm go1.24.11.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build hasher-host binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags '-s -w' \
    -o /bin/hasher-host ./cmd/driver/hasher-host

# Stage 2: Runtime image
FROM ubuntu:22.04

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    libusb-1.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Copy binary
COPY --from=go-builder /bin/hasher-host /usr/local/bin/

# Create non-root user
RUN useradd -m -u 1000 hasher-host

WORKDIR /home/hasher-host

# Expose API port
EXPOSE 8080

# Default command
CMD ["hasher-host"]
