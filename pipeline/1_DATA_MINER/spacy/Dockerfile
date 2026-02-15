# Multi-stage Dockerfile for Go-Spacy production deployment
# This Dockerfile creates an optimized production image with minimal attack surface

# Build stage
FROM ubuntu:22.04 as builder

# Avoid prompts from apt
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies required for building
RUN apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    python3 \
    python3-dev \
    python3-pip \
    wget \
    git \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Go
ARG GO_VERSION=1.21.5
RUN wget -O go.tar.gz "https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    && tar -C /usr/local -xzf go.tar.gz \
    && rm go.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV CGO_ENABLED=1

# Install Python dependencies
RUN pip3 install --no-cache-dir --upgrade pip setuptools wheel \
    && pip3 install --no-cache-dir spacy \
    && python3 -m spacy download en_core_web_sm

# Set working directory
WORKDIR /workspace

# Copy go mod files first (for better caching)
COPY go.mod ./

# Download Go dependencies (no go.sum needed since we have no external deps)
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the project with optimizations
ENV CFLAGS="-O3 -DNDEBUG"
ENV CXXFLAGS="-O3 -DNDEBUG"
RUN make clean && make BUILD_MODE=release

# Verify the build
RUN export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$PWD/lib \
    && go test -v -run TestBasicFunctionality -timeout=2m ./...

# Runtime stage - minimal image
FROM ubuntu:22.04 as runtime

# Avoid prompts from apt
ENV DEBIAN_FRONTEND=noninteractive

# Install minimal runtime dependencies
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Install Python runtime dependencies
RUN pip3 install --no-cache-dir --upgrade pip \
    && pip3 install --no-cache-dir spacy \
    && python3 -m spacy download en_core_web_sm \
    && python3 -c "import spacy; nlp = spacy.load('en_core_web_sm'); print('Spacy model loaded successfully')"

# Create non-root user for security
RUN groupadd -r gouser && useradd -r -g gouser gouser

# Create directories for the application
RUN mkdir -p /app/lib /app/docs \
    && chown -R gouser:gouser /app

# Copy built artifacts from builder stage
COPY --from=builder --chown=gouser:gouser /workspace/lib/ /app/lib/
COPY --from=builder --chown=gouser:gouser /workspace/README.md /workspace/LICENSE /app/
COPY --from=builder --chown=gouser:gouser /workspace/docs/ /app/docs/

# Set library path
ENV LD_LIBRARY_PATH="/app/lib:${LD_LIBRARY_PATH}"

# Health check script
RUN echo '#!/bin/bash\nexport LD_LIBRARY_PATH="/app/lib:${LD_LIBRARY_PATH}"\npython3 -c "import spacy; nlp = spacy.load(\"en_core_web_sm\"); print(\"Health check passed\")"' \
    > /app/healthcheck.sh && chmod +x /app/healthcheck.sh

# Switch to non-root user
USER gouser

# Set working directory
WORKDIR /app

# Add health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD /app/healthcheck.sh

# Metadata
LABEL maintainer="Go-Spacy Contributors" \
      version="1.0" \
      description="Go-Spacy NLP library runtime container" \
      org.opencontainers.image.title="Go-Spacy" \
      org.opencontainers.image.description="Golang bindings for Spacy NLP" \
      org.opencontainers.image.url="https://github.com/am-sokolov/go-spacy" \
      org.opencontainers.image.source="https://github.com/am-sokolov/go-spacy" \
      org.opencontainers.image.licenses="MIT"

# Default command (can be overridden)
CMD ["/bin/bash"]

# Development stage (optional) - includes build tools
FROM builder as development

# Install additional development tools
RUN apt-get update && apt-get install -y \
    vim \
    curl \
    less \
    htop \
    && rm -rf /var/lib/apt/lists/*

# Install Go development tools
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest \
    && go install mvdan.cc/gofumpt@latest \
    && go install golang.org/x/tools/cmd/goimports@latest

# Set up development environment
ENV PS1="\[\033[01;32m\]\u@\h\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ "

WORKDIR /workspace

# Default command for development
CMD ["/bin/bash"]