#!/bin/bash

# Data Miner - Document Structuring Engine Runner
# Production-ready PDF to ML-ready data pipeline

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Default configuration
INPUT_DIR="${INPUT_DIR:-$PROJECT_ROOT/data/documents}"
OUTPUT_FILE="${OUTPUT_FILE:-$PROJECT_ROOT/data/json/ai_knowledge_base.json}"
WORKERS="${WORKERS:-4}"
CHUNK_SIZE="${CHUNK_SIZE:-300}"
CHUNK_OVERLAP="${CHUNK_OVERLAP:-50}"
OLLAMA_MODEL="${OLLAMA_MODEL:-nomic-embed-text}"
OLLAMA_HOST="${OLLAMA_HOST:-http://localhost:11434}"
BATCH_SIZE="${BATCH_SIZE:-16}"

# Check dependencies
echo "🔍 Checking dependencies..."

# Check if pdftotext is available
if ! command -v pdftotext &> /dev/null; then
    echo "❌ pdftotext not found. Please install:"
    echo "   Ubuntu/Debian: sudo apt-get install poppler-utils"
    echo "   macOS: brew install poppler"
    echo "   Other: https://poppler.freedesktop.org/"
    exit 1
fi

echo "✅ Dependencies checked - Ollama check will be handled by the application"

# Check if input directory exists
if [ ! -d "$INPUT_DIR" ]; then
    echo "❌ Input directory '$INPUT_DIR' does not exist"
    echo "   Creating directory for you..."
    mkdir -p "$INPUT_DIR"
    echo "💡 Place PDF files in '$INPUT_DIR' and run again"
    exit 1
fi

# Count PDF files
PDF_COUNT=$(find "$INPUT_DIR" -name "*.pdf" -type f | wc -l)
if [ "$PDF_COUNT" -eq 0 ]; then
    echo "❌ No PDF files found in '$INPUT_DIR'"
    echo "   Please add some PDF files to process"
    exit 1
fi

echo "📄 Found $PDF_COUNT PDF files to process"
echo "🔧 Configuration:"
echo "   Input Directory: $INPUT_DIR"
echo "   Output File: $OUTPUT_FILE"
echo "   Workers: $WORKERS"
echo "   Chunk Size: $CHUNK_SIZE words"
echo "   Chunk Overlap: $CHUNK_OVERLAP words"
echo "   Ollama Model: $OLLAMA_MODEL"
echo "   Ollama Host: $OLLAMA_HOST"
echo "   Batch Size: $BATCH_SIZE"
echo ""

# Check if dataminer binary exists
DATAMINER_BINARY="$PROJECT_ROOT/cmd/dataminer/dataminer"
if [ ! -f "$DATAMINER_BINARY" ]; then
    echo "🔨 Dataminer binary not found, building..."
    cd "$PROJECT_ROOT/cmd/dataminer" && go build -o dataminer main.go
fi

# Run the neural miner
echo "🚀 Starting Data Miner..."
exec "$DATAMINER_BINARY" \
    -input "$INPUT_DIR" \
    -output "$OUTPUT_FILE" \
    -workers "$WORKERS" \
    -chunk-size "$CHUNK_SIZE" \
    -chunk-overlap "$CHUNK_OVERLAP" \
    -model "$OLLAMA_MODEL" \
    -host "$OLLAMA_HOST" \
    -batch-size "$BATCH_SIZE"