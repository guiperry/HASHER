# Data Miner - Document Structuring Engine

A production-ready Go application that processes PDF documents and converts them into ML-ready JSON files with embeddings using Ollama.

## Features

- **PDF Processing**: Extract text from PDF files using pdftotext
- **Text Chunking**: Intelligent sliding window chunking with configurable overlap for optimal context preservation
- **Embeddings Integration**: Batch processing with Ollama API (nomic-embed-text model by default)
- **Concurrent Processing**: Worker pool pattern for high-throughput document processing
- **Checkpointing**: Resume capability using bbolt embedded key-value store
- **Progress Tracking**: Real-time progress bars with ETA calculations
- **ML-Ready Output**: JSON files optimized for training data pipelines
- **arXiv Integration**: Automated mining of academic papers from arXiv.org with category-based selection
- **Background Mining**: Continuous background service for periodic paper harvesting
- **Category Taxonomy**: Full arXiv category taxonomy support with intelligent filtering

## Quick Start

1. **Prerequisites**:
   - Go 1.22+
   - Ollama server running with embeddings extension
   - pdftotext (from poppler-utils package)
   - PDF documents to process (or use arXiv mining)

2. **Installation**:
   ```bash
   go mod tidy
   go build -o dataminer .
   ```

3. **Usage**:
    
    **🚀 Integrated Workflow (Recommended)**:
    ```bash
    # Complete workflow: arXiv mining → neural processing
    ./run_workflow.sh
    
    # Custom configuration
    ./run_workflow.sh -c "cs.AI,cs.LG" -n 100 -w 12 -l 2000
    
    # Process existing PDFs only
    ./run_workflow.sh --no-arxiv
    
    # Direct workflow execution
    ./dataminer -arxiv-enable -arxiv-categories "cs.AI,cs.LG" -arxiv-max-papers 50
    ```
    
    **Traditional PDF Processing**:
    ```bash
    # Using the convenience script
    ./run.sh
    
    # Direct execution
    ./dataminer -input ./documents -output training_data.json
    ```
    
    **arXiv Mining - One-time Download**:
    ```bash
    # Mine recent papers from recommended ML/AI categories
    ./dataminer -arxiv-enable -arxiv-max-papers 50
    
    # Mine specific categories
    ./dataminer -arxiv-enable -arxiv-categories "cs.AI,cs.LG,stat.ML" -arxiv-max-papers 100
    ```
    
    **arXiv Mining - Background Service**:
    ```bash
    # Run background service that mines every hour
    ./dataminer -arxiv-enable -arxiv-background -arxiv-interval "1h" -arxiv-max-papers 25
    ```

## Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `-input` | `./documents` | Directory containing PDF files |
| `-output` | `ai_knowledge_base.json` | Output JSON file path |
| `-workers` | `4` | Number of concurrent workers |
| `-chunk-size` | `300` | Words per chunk |
| `-chunk-overlap` | `50` | Words overlap between chunks |
| `-model` | `nomic-embed-text` | Ollama embedding model |
| `-host` | `http://localhost:11434` | Ollama API host |
| `-checkpoint` | `checkpoints.db` | Checkpoint database file |
| `-batch-size` | `16` | Batch size for embedding API calls |
| **arXiv Mining Flags** | | |
| `-arxiv-enable` | `false` | Enable arXiv mining functionality |
| `-arxiv-background` | `false` | Run arXiv miner in background service mode |
| `-arxiv-categories` | `recommended categories` | Comma-separated list of arXiv categories |
| `-arxiv-max-papers` | `100` | Maximum papers to download per category |
| `-arxiv-interval` | `24h` | Run interval for background mode (e.g., 24h, 1h) |
| `-arxiv-delay` | `3` | Delay between downloads in seconds |
| `-arxiv-sort-by` | `submittedDate` | Sort by field (relevance, lastUpdatedDate, submittedDate) |
| `-arxiv-sort-order` | `descending` | Sort order (ascending, descending) |

## Output Schema

The generated JSON files contain DocumentRecord objects:

```go
type DocumentRecord struct {
    FileName  string    `json:"file_name"`
    ChunkID   int32     `json:"chunk_id"`
    Content   string    `json:"content"`
    Embedding []float32 `json:"embedding"`
}
```

## Architecture

The application follows a producer-consumer pattern with workflow orchestration:

### 🔄 Integrated Workflow
1. **Workflow Orchestrator**: Manages end-to-end pipeline with automatic coordination
2. **arXiv Mining**: Automatically downloads papers when no documents exist
3. **Neural Processing**: Processes downloaded papers with hybrid embeddings
4. **Graceful Shutdown**: Handles interruptions with proper cleanup

### 📊 Core Components
1. **Scanner**: Recursively scans input directory for PDF files, skipping already processed files
2. **Worker Pool**: Concurrent goroutines process PDFs (extract → chunk → embed)
3. **Batch Processing**: Optimized API calls with configurable batch sizes
4. **Checkpointing**: Persistent tracking prevents reprocessing of completed files
5. **Progress Tracking**: Real-time feedback with ETA calculations
6. **Hybrid Embeddings**: Cloudflare + Ollama for optimal performance

## Performance Considerations

- **Memory Efficiency**: Streaming JSON writer prevents memory buildup with large datasets
- **Network Optimization**: Batch embedding calls reduce API overhead
- **Concurrent Processing**: Configurable worker count adapts to available CPU cores
- **Resume Capability**: Checkpoint system allows interruption and resumption

## arXiv Mining

### Category Support

The arXiv miner supports the full arXiv category taxonomy. Recommended categories for ML/AI research:

- `cs.AI` - Artificial Intelligence
- `cs.LG` - Machine Learning
- `cs.CV` - Computer Vision
- `cs.CL` - Natural Language Processing
- `cs.NE` - Neural and Evolutionary Computing
- `stat.ML` - Machine Learning (Statistics perspective)
- `stat.AP` - Applications of Statistics
- `math.NA` - Numerical Analysis
- `physics.comp-ph` - Computational Physics
- `q-bio.NC` - Neurons and Cognition
- `q-bio.QM` - Quantitative Methods for Biology

### Mining Modes

**Foreground Mode**: Downloads papers once and exits, allowing you to immediately process them with the neural miner pipeline.

**Background Mode**: Runs as a persistent service that periodically mines new papers according to your schedule.

### API Rate Limiting

The miner includes built-in rate limiting to be respectful to arXiv servers:
- Configurable delays between downloads (default: 3 seconds)
- Retry logic with exponential backoff
- Proper HTTP headers and user agent
- Respect for arXiv's terms of service

### Checkpointing

Both traditional PDF processing and arXiv mining use the same checkpoint system, ensuring papers are never processed twice.

## 🚀 Hybrid Embeddings System

The Data Miner now includes a hybrid embeddings system that combines **Cloudflare Workers AI** embeddings with **local Ollama** embeddings for optimal performance and reliability.

### Features

- **Primary: Cloudflare API**: Fast, cloud-based embeddings via your custom endpoint
- **Fallback: Local Ollama**: CPU-based processing when Cloudflare quota is exhausted
- **Request Tracking**: Daily quota management with automatic fallback
- **High Performance**: Cloudflare embeddings are ~10-20x faster than CPU processing
- **Cost Optimization**: Smart usage limits to stay within quotas

### Configuration

Environment Variables:
```bash
# Cloudflare settings
export CLOUDFLARE_EMBEDDINGS_URL="https://embeddings.knirv.com/embeddings"  # Your endpoint
export CLOUDFLARE_DAILY_LIMIT="5000"  # Daily request limit (default: 5000)

# Ollama fallback settings  
export OLLAMA_NUM_PARALLEL=8
export OLLAMA_MAX_LOADED_MODELS=1
export OLLAMA_FLASH_ATTENTION=1
```

### Usage

**Optimized Runner with Hybrid Embeddings:**
```bash
# Use the hybrid runner (recommended)
./run_hybrid.sh

# Or manual configuration
CLOUDFLARE_DAILY_LIMIT=1000 ./dataminer -input ./documents
```

### Performance Comparison

| Provider | Speed | Cost | Reliability | When to Use |
|----------|--------|------|-------------|--------------|
| Cloudflare | ~50ms | Free tier | High | Primary choice |
| Ollama | ~18s | Local | Medium | Fallback |

**Performance Gain**: **~360x faster** with Cloudflare embeddings vs local CPU processing!

## Integration with Data Miner Ecosystem

This tool is designed to work seamlessly with the broader Data Miner architecture:

- **Training Pipeline**: Generated JSON files feed directly into the Evolutionary Training harness
- **vHasher Compatibility**: Output format matches expected input for the virtual hasher simulator
- **ASIC Deployment**: Embeddings can be mapped to the 12-slot neural frame for hardware acceleration
- **arXiv Integration**: Automatically mines academic papers to keep your knowledge base current

## Error Handling

The application includes comprehensive error handling:
- PDF parsing failures are logged but don't stop processing
- Network timeouts to Ollama are handled gracefully
- File permission issues are clearly reported
- Checkpoint corruption is detected and handled

## Monitoring

- **Progress Bars**: Visual feedback with percentage completion and ETA
- **Logging**: Detailed worker-level logging for debugging
- **Statistics**: Summary of processed files and generated chunks

## Dependencies

### System Dependencies
- **pdftotext**: For PDF text extraction (install via poppler-utils)
- **Ollama**: Embedding generation service (must be running locally)

### Go Dependencies
- **go.etcd.io/bbolt**: Checkpointing database
- **github.com/vbauerster/mpb/v8**: Progress bars
- All other dependencies are included in go.mod