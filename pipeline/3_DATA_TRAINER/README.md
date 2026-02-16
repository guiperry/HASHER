# HASHER Data Trainer

## Overview

The HASHER Data Trainer is a sophisticated machine learning system that transforms obsolete Bitcoin mining hardware into a novel neural inference system. This implementation focuses on the **Evolutionary Training Harness** that optimizes SHA-256 "seeds" (weights) for the HASHER architecture using Group Relative Policy Optimization (GRPO) logic implemented through Evolutionary Strategies (ES).

## Architecture

### Core Components

1. **Evolutionary GRPO Harness** - Implements Group Relative Policy Optimization without traditional backpropagation
2. **vHasher Simulator** - GPU-accelerated SHA-256 simulation for rapid evaluation
3. **CSV Storage Layer** - Efficient weight persistence with metadata
4. **Checkpoint Manager** - Resilient training state management
5. **Cross-Hardware Validator** - Consistency validation between GPU and ASIC
6. **Flash Deployment** - Production deployment with rollback capability

### Key Features

- **Quantum-Resistant Training**: High-velocity salt rotation ensures resistance to quantum attacks
- **Evolutionary Optimization**: GRPO-based selection eliminates need for gradients
- **Hardware Abstraction**: Works with both GPU simulation and physical ASICs
- **Fault Tolerance**: Comprehensive checkpointing and validation systems
- **Production Ready**: Full deployment pipeline with monitoring

## Quick Start

### Prerequisites

- Go 1.21 or later
- CUDA-compatible GPU (optional, for simulation mode)
- Sufficient RAM (recommended 8GB+ for training)

### Installation

```bash
# Clone the repository
git clone https://github.com/lab/hasher/data-trainer.git
cd hasher/data-trainer

# Install dependencies
make deps

# Build the application
make build
```

### Running the Trainer

```bash
# Basic training with default settings
make run

# Or with custom parameters
./bin/data-trainer -epochs 10 -population 64 -data ./my-data

# With configuration file
./bin/data-trainer -config config.json
```

## Configuration

### Configuration File Format

```json
{
  "simulator": {
    "device_type": "vhasher",
    "max_concurrency": 100,
    "target_hash_rate": 500000000,
    "cache_size": 10000,
    "gpu_device": 0,
    "timeout": 30
  },
  "storage": {
    "base_path": "data/weights",
    "layer_size": 1000
  },
  "training": {
    "population_size": 128,
    "max_generations": 500,
    "elite_ratio": 0.25,
    "mutation_rate": 0.05,
    "target_fitness": 0.95,
    "validation_split": 0.1
  },
  "deployment": {
    "bpf_map_path": "/sys/fs/bpf/hasher_weights",
    "deployment_timeout": "300s",
    "max_retries": 3,
    "retry_delay": "5s",
    "validation_mode": "strict",
    "backup_enabled": true,
    "backup_path": "data/backups",
    "rollback_enabled": true
  },
  "validation": {
    "timeout": "30s",
    "max_concurrency": 10,
    "retry_attempts": 3,
    "tolerance_threshold": 0.01,
    "enable_asic": false
  },
  "logging": {
    "level": "info",
    "format": "text",
    "output": "stdout",
    "max_size": 100,
    "max_backups": 10,
    "max_age": 30
  }
}
```

### Command Line Options

```bash
./bin/data-trainer [flags]

Flags:
  -config string   Path to configuration file (default: "")
  -data string     Path to data directory (default: "data")
  -epochs int      Maximum number of training epochs (default: 10)
  -population int  Population size for evolution (default: 32)
  -verbose         Enable verbose logging (default: false)
```

## Training Process

### 1. Data Pipeline

The system ingests processed PDF data through the Data Structuring Engine:

```go
type TrainingRecord struct {
    TokenSequence []int32     // Generated via Tiktoken/BPE
    FeatureVector [12]uint32  // Normalized semantic embeddings
    TargetToken   int32       // The "Label" (next token)
    ContextHash   uint32      // Rolling hash of previous 5 tokens
}
```

### 2. Evolutionary Training

For each target token, the system:

1. **Group Sampling**: Creates population of candidate seeds (64-256)
2. **Parallel Evaluation**: Executes 21-pass SHA-256 recursion on GPU or via optimized software fallback
3. **Reward Calculation**: Evaluates alignment (Hamming-based), stability, and format
4. **Advantage Computation**: Calculates relative performance using **Hamming Similarity Gradient** (total matching bits) instead of just leading zeros
5. **Selection & Mutation**: Keeps elite 25% and generates **Bitcoin-Aware** mutated offspring focusing on the nonce field

### 3. Reward Function

The reward system combines three components:

- **Alignment Reward**: Uses **Hamming Similarity** (count of all 32 matching bits) to provide a continuous gradient for evolution. A prefix-match bonus is applied when the difficulty threshold is met.
- **Stability Reward**: Measure of convergence consistency across the final passes of the 21-pass temporal loop.
- **Format Reward**: Nonce resolves to a valid entry in the token map.

### 4. Key Improvements (v1.1)

- **Hamming Gradient**: Replaced binary "all-or-nothing" prefix matching with bit-wise Hamming similarity, allowing the evolutionary process to "learn" from partial matches.
- **Big-Endian Synchronization**: Unified all components (Jitter, CUDA, ASIC) to use **Big-Endian** word extraction, ensuring hash leading bits correctly align with token IDs.
- **Persistent Jitter RPC**: Performance optimized to reuse a single Unix socket connection for entire population batches, reducing syscall overhead by over 99%.
- **Header Isolation**: Added mandatory cloning of Bitcoin headers per seed to prevent state leakage during the 21-pass modification loop.

### 4. Checkpointing

Training progress is automatically checkpointed using:

- **Checkpoint Manager**: bbolt-based state persistence
- **Validation Gate**: Cross-hardware consistency checks
- **Automatic Recovery**: Resume from last known good state

## Development

### Project Structure

```
├── cmd/
│   └── data-trainer/           # Main application
├── pkg/
│   ├── training/          # Evolutionary algorithms
│   ├── simulator/         # vHasher simulation
│   ├── storage/           # CSV/Parquet storage
│   ├── validator/         # Cross-hardware validation
│   └── deployment/        # Flash deployment
├── internal/
│   ├── config/            # Configuration types
│   └── logging/           # Logging utilities
├── data/
│   ├── weights/           # Weight storage
│   ├── checkpoints/       # Training checkpoints
│   └── backups/           # Deployment backups
├── test/                  # Integration tests
└── scripts/              # Utility scripts
```

### Building from Source

```bash
# Development build
make build

# Debug build
make build-debug

# Production build
make release

# Run with race detector
make race
```

### Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run benchmarks
make test-bench

# Run specific package tests
go test -v ./pkg/training
go test -v ./pkg/simulator
go test -v ./pkg/storage
```

### Code Quality

```bash
# Format code
make format

# Run linter
make lint

# Run security scan
make security-scan

# Run all checks
make pre-commit
```

## Performance

### Expected Performance Metrics

- **Hash Rate**: ~500 GH/s (32 chips @ 250 MHz)
- **Target Rotation**: ~10 Hz (USB limited)
- **Security Margin**: ~2^48 hashes per target
- **Training Speed**: ~100 tokens/hour on single GPU
- **Memory Usage**: ~100MB for 1000-token training set

### Optimization Tips

1. **Population Size**: Start with 64, increase to 128 for better convergence
2. **Cache Size**: Increase to 50000 for production workloads
3. **Batch Processing**: Process multiple tokens in parallel when possible
4. **GPU Utilization**: Ensure CUDA memory is sufficient for population size

## Deployment

### Production Deployment

```bash
# Dry run deployment
./bin/data-trainer -config production.json -deploy-dry-run

# Deploy validated weights
./bin/data-trainer -deploy-layer 0 -target-fitness 0.95

# Monitor deployment
make logs
```

### Hardware Requirements

**Minimum:**
- CPU: 4+ cores
- RAM: 8GB
- Storage: 50GB SSD
- GPU: CUDA-compatible (optional)

**Recommended:**
- CPU: 8+ cores
- RAM: 16GB+
- Storage: 100GB+ NVMe SSD
- GPU: RTX 3080+ or equivalent

## Monitoring

### Health Checks

```bash
# Check system health
./bin/data-trainer -health-check

# Monitor training progress
./bin/data-trainer -monitor -refresh 5s

# Validate deployment
./bin/data-trainer -validate-deployment
```

### Metrics

The system exposes key metrics:

- Training convergence rate
- Average fitness per generation
- Hash rate and device utilization
- Memory usage and cache hit rates
- Validation consistency rates

## Troubleshooting

### Common Issues

1. **Low Convergence Rate**
   - Increase population size
   - Adjust mutation rate
   - Verify data quality

2. **Memory Issues**
   - Reduce cache size
   - Lower population size
   - Check for memory leaks

3. **GPU Errors**
   - Verify CUDA installation
   - Check GPU memory availability
   - Reduce concurrency

4. **Storage Issues**
   - Verify disk space
   - Check permissions
   - Monitor I/O performance

### Debug Mode

```bash
# Enable debug logging
./bin/data-trainer -verbose -log-level debug

# Run with instrumentation
./bin/data-trainer -profile

# Validate configuration
./bin/data-trainer -validate-config config.json
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run the test suite
6. Submit a pull request

### Code Standards

- Follow Go best practices
- Add comprehensive tests
- Update documentation
- Use meaningful commit messages
- Ensure all tests pass

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Based on the Evolutionary Training Specification (ETS) v1.0
- Built for the HASHER quantum-resistant neural architecture
- Inspired by Bitcoin mining hardware repurposing research

## Support

- Documentation: [Wiki](https://github.com/lab/hasher/data-trainer/wiki)
- Issues: [GitHub Issues](https://github.com/lab/hasher/data-trainer/issues)
- Discussions: [GitHub Discussions](https://github.com/lab/hasher/data-trainer/discussions)

---

**Note**: This is a research prototype. For production use, ensure proper security audits and performance testing before deployment.