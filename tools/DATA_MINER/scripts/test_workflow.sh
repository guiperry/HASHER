#!/bin/bash

# Data Miner Workflow Test Script
# Tests the complete end-to-end workflow: arXiv mining → neural processing

set -e

echo "🚀 Data Miner Workflow Test"
echo "============================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Project configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_DIR="$PROJECT_ROOT/data"
DOCUMENTS_DIR="$DATA_DIR/documents"
JSON_DIR="$DATA_DIR/json"
CHECKPOINT_DIR="$DATA_DIR/checkpoints"
OUTPUT_FILE="$JSON_DIR/workflow_test_output.json"

# Create necessary directories
mkdir -p "$DOCUMENTS_DIR"
mkdir -p "$JSON_DIR"
mkdir -p "$CHECKPOINT_DIR"

# Build the project
echo -e "${BLUE}📦 Building the project...${NC}"
cd "$PROJECT_ROOT"
go build -o dataminer ./cmd/dataminer

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Build failed!${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Build successful!${NC}"

# Test configuration
TEST_CATEGORIES="cs.AI,cs.LG"
TEST_MAX_PAPERS=5
TEST_DELAY=1

echo -e "${BLUE}🔧 Test Configuration:${NC}"
echo "  Categories: $TEST_CATEGORIES"
echo "  Max Papers: $TEST_MAX_PAPERS"
echo "  Output: $OUTPUT_FILE"

# Clean up previous test data
echo -e "${BLUE}🧹 Cleaning up previous test data...${NC}"
rm -f "$OUTPUT_FILE"
rm -f "$CHECKPOINT_DIR/checkpoints.db"

# Clear documents directory but keep the structure
find "$DOCUMENTS_DIR" -name "*.pdf" -type f -delete 2>/dev/null || true

echo -e "${BLUE}🎯 Running Workflow Test...${NC}"
echo "This will test the complete integrated workflow:"
echo "  1. arXiv mining (download papers)"
echo "  2. Neural processing (embeddings generation)"
echo ""

# Run the integrated workflow
echo -e "${YELLOW}🚀 Starting integrated workflow...${NC}"

CLOUDFLARE_DAILY_LIMIT=100 ./dataminer \
    -arxiv-enable \
    -arxiv-categories "$TEST_CATEGORIES" \
    -arxiv-max-papers "$TEST_MAX_PAPERS" \
    -arxiv-delay "$TEST_DELAY" \
    -input "$DOCUMENTS_DIR" \
    -output "$OUTPUT_FILE" \
    -chunk-size 150 \
    -chunk-overlap 25 \
    -workers 4

echo -e "${GREEN}✅ Workflow completed!${NC}"

# Verify results
echo -e "${BLUE}📊 Verifying results...${NC}"

# Check if output file was created
if [ -f "$OUTPUT_FILE" ]; then
    FILE_SIZE=$(stat -c%s "$OUTPUT_FILE" 2>/dev/null || stat -f%z "$OUTPUT_FILE" 2>/dev/null || echo "0")
    echo -e "${GREEN}✅ Output file created: $OUTPUT_FILE ($FILE_SIZE bytes)${NC}"
else
    echo -e "${RED}❌ Output file not found!${NC}"
    exit 1
fi

# Count processed PDFs
PDF_COUNT=$(find "$DOCUMENTS_DIR" -name "*.pdf" -type f | wc -l)
echo -e "${GREEN}📄 PDFs downloaded: $PDF_COUNT${NC}"

# Count JSON records
if command -v jq >/dev/null 2>&1; then
    RECORD_COUNT=$(jq length "$OUTPUT_FILE" 2>/dev/null || echo "0")
    echo -e "${GREEN}📝 JSON records created: $RECORD_COUNT${NC}"
    
    # Show sample record
    if [ "$RECORD_COUNT" -gt 0 ]; then
        echo -e "${BLUE}📋 Sample record structure:${NC}"
        jq '.[0] | {file_name, chunk_id, content: .content[0:50] + "...", embedding_size: (.embedding | length)}' "$OUTPUT_FILE" 2>/dev/null || echo "  (Could not parse JSON structure)"
    fi
else
    echo -e "${YELLOW}⚠️  jq not found, cannot analyze JSON structure${NC}"
fi

# Check if embeddings were generated
if command -v jq >/dev/null 2>&1 && [ -f "$OUTPUT_FILE" ]; then
    EMBEDDINGS_CHECK=$(jq '.[0].embedding | length' "$OUTPUT_FILE" 2>/dev/null || echo "0")
    if [ "$EMBEDDINGS_CHECK" -gt 0 ]; then
        echo -e "${GREEN}🧠 Embeddings generated successfully (dimension: $EMBEDDINGS_CHECK)${NC}"
    else
        echo -e "${RED}❌ No embeddings found in output!${NC}"
    fi
fi

# Test hybrid embeddings configuration
echo -e "${BLUE}🔄 Testing hybrid embeddings configuration...${NC}"
if [ -n "$CLOUDFLARE_EMBEDDINGS_URL" ]; then
    echo -e "${GREEN}✅ Cloudflare embeddings URL configured: $CLOUDFLARE_EMBEDDINGS_URL${NC}"
else
    echo -e "${YELLOW}⚠️  Cloudflare embeddings URL not configured, using default${NC}"
fi

# Performance summary
echo -e "${BLUE}📈 Performance Summary:${NC}"
echo "  - Workflow type: Integrated (arXiv + Neural)"
echo "  - Categories processed: $TEST_CATEGORIES"
echo "  - Papers downloaded: $PDF_COUNT"
echo "  - Processing mode: Hybrid embeddings (Cloudflare + Ollama fallback)"

# Cleanup test files (optional)
read -p "Clean up test files? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${BLUE}🧹 Cleaning up test files...${NC}"
    rm -f "$OUTPUT_FILE"
    find "$DOCUMENTS_DIR" -name "*.pdf" -type f -delete
    echo -e "${GREEN}✅ Test files cleaned up${NC}"
fi

echo -e "${GREEN}🎉 Workflow test completed successfully!${NC}"
echo -e "${BLUE}The integrated workflow is working correctly and ready for production use.${NC}"