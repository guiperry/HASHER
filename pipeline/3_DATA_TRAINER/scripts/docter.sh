#!/bin/bash
# doctor.sh - Diagnostic and Environment Setup

echo "--- HASHER Hardware Diagnostic ---"

# 1. Check CUDA Version
if command -v nvcc > /dev/null; then
    CUDA_VERSION=$(nvcc --version | grep "release" | awk '{print $6}' | cut -c2-)
    echo "[✓] Found CUDA: $CUDA_VERSION"
    export CUDA_TAG=$CUDA_VERSION
else
    echo "[!] CUDA not found. Defaulting to CPU-only simulation."
    export CUDA_TAG="none"
fi

# 2. Check Go Version
if command -v go > /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}' | cut -c3-)
    echo "[✓] Found Go: $GO_VERSION"
else
    echo "[!] Go is not installed. Required for the Data Trainer."
    exit 1
fi

# 3. Check for uBPF
if [ ! -f "/usr/local/lib/libubpf.so" ]; then
    echo "[i] uBPF not found locally. Preparing for local build..."
    export BUILD_UBPF=true
fi

# Generate local.env for the Makefile
echo "CUDA_VERSION=$CUDA_TAG" > local.env
echo "GO_VERSION=$GO_VERSION" >> local.env