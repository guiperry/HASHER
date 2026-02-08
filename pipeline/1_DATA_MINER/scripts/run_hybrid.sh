#!/bin/bash

# Optimized Data Miner Runner with Hybrid Embeddings
# This script sets optimal environment variables and runs as neural miner with Cloudflare + Ollama hybrid embeddings

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Detect available CPU cores
CPU_CORES=$(nproc)
echo "Detected $CPU_CORES CPU cores"

# Set optimal environment variables for Ollama (even on CPU)
export OLLAMA_NUM_PARALLEL=8
export OLLAMA_MAX_LOADED_MODELS=1
export OLLAMA_MAX_QUEUE=512
export OLLAMA_FLASH_ATTENTION=1
export OLLAMA_KV_CACHE_TYPE=f16
export OLLAMA_LOAD_TIMEOUT=10m

# Configure Cloudflare embeddings - loaded from .env file
# export CLOUDFLARE_EMBEDDINGS_URL="your_endpoint_here"
export CLOUDFLARE_DAILY_LIMIT="${CLOUDFLARE_DAILY_LIMIT:-5000}"

# Simple GPU detection
GPU_AVAILABLE=false
if command -v nvidia-smi >/dev/null 2>&1; then
    echo "GPU detected, checking CUDA compatibility"
    # Simple version check for CUDA 11+
    if nvidia-smi --query-gpu=driver_version --format=csv,noheader,nounits 2>/dev/null | grep -q "driver.*[0-9]" | head -1 | cut -d'.' -f1 | cut -d'.' -f1; then
        echo "Compatible CUDA detected for GPU embeddings"
        GPU_AVAILABLE=true
        export CUDA_VISIBLE_DEVICES=0
        export OLLAMA_GPU_OVERHEAD=1073741824
        export OLLAMA_SCHED_SPREAD=false
    else
        echo "No GPU detected, optimizing for CPU performance"
        export OLLAMA_GPU_OVERHEAD=0
        export OLLAMA_SCHED_SPREAD=false
    fi
else
    echo "nvidia-smi not available"
    GPU_AVAILABLE=false
    export OLLAMA_GPU_OVERHEAD=0
    export OLLAMA_SCHED_SPREAD=false
fi

echo "Environment variables configured:"
echo "  OLLAMA_NUM_PARALLEL: $OLLAMA_NUM_PARALLEL"
echo "  OLLAMA_MAX_LOADED_MODELS: $OLLAMA_MAX_LOADED_MODELS"
echo "  OLLAMA_FLASH_ATTENTION: $OLLAMA_FLASH_ATTENTION"
echo "  OLLAMA_KV_CACHE_TYPE: $OLLAMA_KV_CACHE_TYPE"
echo "  CLOUDFLARE_EMBEDDINGS_URL: $CLOUDFLARE_EMBEDDINGS_URL"
echo "  CLOUDFLARE_DAILY_LIMIT: $CLOUDFLARE_DAILY_LIMIT"
echo "  GOMAXPROCS: $GOMAXPROCS"
echo "  GOGC: $GOGC"
echo "  GPU_AVAILABLE: $GPU_AVAILABLE"

# Start Ollama if not running
if ! curl -s http://localhost:11434/api/tags &>/dev/null; then
    echo "Starting Ollama with optimizations..."
    ollama serve > /tmp/ollama_hybrid.log 2>&1 &
    echo "Waiting for Ollama to be ready..."
    for i in {1..30}; do
        if curl -s http://localhost:11434/api/tags &>/dev/null; then
            echo "Ollama is ready!"
            break
        fi
        echo "Waiting for Ollama... ($i/30)"
        sleep 1
    done
    
    if ! curl -s http://localhost:11434/api/tags &>/dev/null; then
        echo "Failed to start Ollama. Check /tmp/ollama_hybrid.log"
        exit 1
    fi
fi

# Run neural miner with optimized default parameters
echo "Running Data Miner with Hybrid Embeddings..."

# Default optimized parameters
DEFAULT_INPUT="$PROJECT_ROOT/data/documents"
DEFAULT_OUTPUT="$PROJECT_ROOT/data/json/ai_knowledge_base.json"
DEFAULT_WORKERS=$CPU_CORES
DEFAULT_CHUNK_SIZE=100
DEFAULT_CHUNK_OVERLAP=25
DEFAULT_BATCH_SIZE=4

# Parse command line arguments or use defaults
# Check if first argument is a flag (starts with -)
if [[ "${1:0:1}" == "-" ]]; then
    INPUT_DIR="$DEFAULT_INPUT"
    OUTPUT_FILE="$DEFAULT_OUTPUT"
    NUM_WORKERS="$DEFAULT_WORKERS"
    CHUNK_SIZE="$DEFAULT_CHUNK_SIZE"
    BATCH_SIZE="$DEFAULT_BATCH_SIZE"
    FLAGS=("${@}")
else
    INPUT_DIR="${1:-$DEFAULT_INPUT}"
    OUTPUT_FILE="${2:-$DEFAULT_OUTPUT}"
    NUM_WORKERS="${3:-$DEFAULT_WORKERS}"
    CHUNK_SIZE="${4:-$DEFAULT_CHUNK_SIZE}"
    BATCH_SIZE="${5:-$DEFAULT_BATCH_SIZE}"
    FLAGS=("${@:6}")
fi

echo "Using parameters:"
echo "  Input: $INPUT_DIR"
echo "  Output: $OUTPUT_FILE"
echo "  Workers: $NUM_WORKERS"
echo "  Chunk size: $CHUNK_SIZE"
echo "  Batch size: $BATCH_SIZE"
echo "  Embeddings: Hybrid (Cloudflare + Ollama fallback)"
echo "  Daily quota: $CLOUDFLARE_DAILY_LIMIT requests"

# Check if dataminer binary exists
DATAMINER_BINARY="$PROJECT_ROOT/cmd/dataminer/dataminer"
if [ ! -f "$DATAMINER_BINARY" ]; then
    echo "🔨 Dataminer binary not found, building..."
    cd "$PROJECT_ROOT/cmd/dataminer" && go build -o dataminer main.go
fi

# Run neural miner
exec "$DATAMINER_BINARY" \
    -input "$INPUT_DIR" \
    -output "$OUTPUT_FILE" \
    -workers "$NUM_WORKERS" \
    -chunk-size "$CHUNK_SIZE" \
    -chunk-overlap "$DEFAULT_CHUNK_OVERLAP" \
    -batch-size "$BATCH_SIZE" \
    "${FLAGS[@]}"