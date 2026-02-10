#!/bin/bash

# Data Miner Production Workflow Runner
# Runs the complete end-to-end workflow with production optimizations

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Project configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_DIR="$PROJECT_ROOT/data"
DOCUMENTS_DIR="$DATA_DIR/documents"
JSON_DIR="$DATA_DIR/json"
CHECKPOINT_DIR="$DATA_DIR/checkpoints"

# Default configuration
DEFAULT_CATEGORIES="cs.AI,cs.LG,cs.CV,cs.CL,cs.NE,stat.ML"
DEFAULT_MAX_PAPERS=50
DEFAULT_DELAY=2
DEFAULT_WORKERS=8
DEFAULT_CHUNK_SIZE=150
DEFAULT_CHUNK_OVERLAP=25
DEFAULT_OUTPUT="$JSON_DIR/ai_knowledge_base.json"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${PURPLE}$1${NC}"
}

# Function to show usage
show_usage() {
    echo "Data Miner Production Workflow Runner"
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -c, --categories CATEGORIES    Comma-separated arXiv categories (default: $DEFAULT_CATEGORIES)"
    echo "  -n, --max-papers NUM         Maximum papers per category (default: $DEFAULT_MAX_PAPERS)"
    echo "  -d, --delay SECONDS          Delay between downloads (default: $DEFAULT_DELAY)"
    echo "  -w, --workers NUM           Number of processing workers (default: $DEFAULT_WORKERS)"
    echo "  -s, --chunk-size NUM         Text chunk size (default: $DEFAULT_CHUNK_SIZE)"
    echo "  -o, --output FILE            Output file (default: $DEFAULT_OUTPUT)"
    echo "  -l, --cloudflare-limit NUM  Daily Cloudflare request limit"
    echo "  --no-arxiv                   Skip arXiv mining, only process existing PDFs"
    echo "  --dry-run                    Show configuration without running"
    echo "  -h, --help                   Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                          # Run with defaults"
    echo "  $0 -c \"cs.AI,cs.LG\" -n 100               # Specific categories and more papers"
    echo "  $0 --no-arxiv                              # Process existing PDFs only"
    echo "  $0 -l 1000 --workers 12                   # Custom Cloudflare limit and workers"
}

# Parse command line arguments
CATEGORIES="$DEFAULT_CATEGORIES"
MAX_PAPERS="$DEFAULT_MAX_PAPERS"
DELAY="$DEFAULT_DELAY"
WORKERS="$DEFAULT_WORKERS"
CHUNK_SIZE="$DEFAULT_CHUNK_SIZE"
CHUNK_OVERLAP="$DEFAULT_CHUNK_OVERLAP"
OUTPUT="$DEFAULT_OUTPUT"
CLOUDFLARE_LIMIT=""
NO_ARXIV=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--categories)
            CATEGORIES="$2"
            shift 2
            ;;
        -n|--max-papers)
            MAX_PAPERS="$2"
            shift 2
            ;;
        -d|--delay)
            DELAY="$2"
            shift 2
            ;;
        -w|--workers)
            WORKERS="$2"
            shift 2
            ;;
        -s|--chunk-size)
            CHUNK_SIZE="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT="$2"
            shift 2
            ;;
        -l|--cloudflare-limit)
            CLOUDFLARE_LIMIT="$2"
            shift 2
            ;;
        --no-arxiv)
            NO_ARXIV=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Print configuration header
print_header "🚀 Data Miner Production Workflow"
echo "======================================="

# Create necessary directories
print_status "Setting up directories..."
mkdir -p "$DOCUMENTS_DIR"
mkdir -p "$JSON_DIR"
mkdir -p "$CHECKPOINT_DIR"

# Show configuration
print_header "📋 Configuration"
echo "  📂 Documents Directory: $DOCUMENTS_DIR"
echo "  📄 Output File: $OUTPUT"
echo "  🔧 Processing Workers: $WORKERS"
echo "  📝 Chunk Size: $CHUNK_SIZE words"
echo "  🔄 Chunk Overlap: $CHUNK_OVERLAP words"
echo ""
echo "  📚 arXiv Mining:"
if [ "$NO_ARXIV" = true ]; then
    echo "    ❌ Disabled (processing existing PDFs only)"
else
    echo "    ✅ Enabled"
    echo "    🏷️  Categories: $CATEGORIES"
    echo "    📄 Max Papers per Category: $MAX_PAPERS"
    echo "    ⏱️  Download Delay: ${DELAY}s"
fi
echo ""
echo "  🌐 Embeddings Configuration:"
if [ -n "$CLOUDFLARE_LIMIT" ]; then
    echo "    ☁️  Cloudflare Daily Limit: $CLOUDFLARE_LIMIT requests"
elif [ -n "$CLOUDFLARE_DAILY_LIMIT" ]; then
    echo "    ☁️  Cloudflare Daily Limit: $CLOUDFLARE_DAILY_LIMIT requests (from env)"
else
    echo "    ☁️  Cloudflare Daily Limit: 5000 requests (default)"
fi
echo "    🤖 Ollama Model: nomic-embed-text (fallback)"

if [ "$DRY_RUN" = true ]; then
    print_header "🔍 Dry Run - Configuration Summary"
    echo "Configuration shown above. No processing will be performed."
    exit 0
fi

# Build the project
print_status "Building the project..."
cd "$PROJECT_ROOT"
go build -o data-miner ./cmd/data-miner

if [ $? -ne 0 ]; then
    print_error "Build failed!"
    exit 1
fi

print_success "Build completed successfully!"

# Check existing PDFs
PDF_COUNT=$(find "$DOCUMENTS_DIR" -name "*.pdf" -type f | wc -l)
print_status "Found $PDF_COUNT existing PDF files"

# Prepare environment variables
ENV_VARS=""
if [ -n "$CLOUDFLARE_LIMIT" ]; then
    ENV_VARS="CLOUDFLARE_DAILY_LIMIT=$CLOUDFLARE_LIMIT"
fi

# Prepare command
CMD="./data-miner"
ARGS=()

if [ "$NO_ARXIV" = false ]; then
    ARGS+=("-arxiv-enable")
    ARGS+=("-arxiv-categories" "$CATEGORIES")
    ARGS+=("-arxiv-max-papers" "$MAX_PAPERS")
    ARGS+=("-arxiv-delay" "$DELAY")
fi

ARGS+=("-input" "$DOCUMENTS_DIR")
ARGS+=("-output" "$OUTPUT")
ARGS+=("-chunk-size" "$CHUNK_SIZE")
ARGS+=("-chunk-overlap" "$CHUNK_OVERLAP")
ARGS+=("-workers" "$WORKERS")

# Show the command that will be run
print_header "🚀 Executing Workflow"
echo "Command: $ENV_VARS $CMD ${ARGS[*]}"
echo ""

# Run the workflow
print_status "Starting integrated workflow..."
print_status "This may take several minutes depending on the number of papers..."
echo ""

if [ -n "$ENV_VARS" ]; then
    eval "$ENV_VARS $CMD ${ARGS[*]}"
else
    "$CMD" "${ARGS[@]}"
fi

# Verify results
echo ""
print_header "📊 Results Analysis"

# Check output file
if [ -f "$OUTPUT" ]; then
    FILE_SIZE=$(stat -c%s "$OUTPUT" 2>/dev/null || stat -f%z "$OUTPUT" 2>/dev/null || echo "0")
    print_success "Output file created: $OUTPUT ($FILE_SIZE bytes)"
else
    print_error "Output file not found!"
    exit 1
fi

# Count processed PDFs
NEW_PDF_COUNT=$(find "$DOCUMENTS_DIR" -name "*.pdf" -type f | wc -l)
PROCESSED_PAPERS=$((NEW_PDF_COUNT - PDF_COUNT))
print_success "Papers downloaded: $PROCESSED_PAPERS (total: $NEW_PDF_COUNT)"

# Analyze JSON output
if command -v jq >/dev/null 2>&1; then
    RECORD_COUNT=$(jq length "$OUTPUT" 2>/dev/null || echo "0")
    print_success "JSON records created: $RECORD_COUNT"
    
    if [ "$RECORD_COUNT" -gt 0 ]; then
        # Check if embeddings are present
        EMBEDDING_DIM=$(jq '.[0].embedding | length' "$OUTPUT" 2>/dev/null || echo "0")
        if [ "$EMBEDDING_DIM" -gt 0 ]; then
            print_success "Embeddings generated: $EMBEDDING_DIM dimensions"
        else
            print_warning "No embeddings found in output"
        fi
        
        # Show sample content
        SAMPLE_TITLE=$(jq -r '.[0].file_name' "$OUTPUT" 2>/dev/null || echo "N/A")
        print_status "Sample file: $SAMPLE_TITLE"
    fi
else
    print_warning "jq not installed - cannot analyze JSON structure"
fi

# Performance summary
echo ""
print_header "📈 Production Summary"
echo "✅ Workflow completed successfully"
echo "📁 Documents processed: $NEW_PDF_COUNT PDFs"
echo "🧠 Neural embeddings generated with hybrid system"
echo "🌐 Used Cloudflare + Ollama fallback for optimal performance"
echo "💾 Output saved to: $OUTPUT"

# Next steps
echo ""
print_header "🎯 Next Steps"
echo "1. 📊 Analyze the generated embeddings: jq '. | length' $OUTPUT"
echo "2. 🤖 Use with your training pipeline"
echo "3. 🔄 Schedule regular runs: $0 -c \"$CATEGORIES\" -n $MAX_PAPERS"
echo "4. 📈 Monitor Cloudflare quota usage"

print_success "Production workflow completed successfully!"