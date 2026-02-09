# HASHER Hashing Methods

This package provides a unified interface for multiple hashing methods used in the HASHER project, with automatic hardware detection and fallback capabilities.

## Architecture Overview

```
pkg/hashing/
├── core/                    # Canonical SHA-256 implementations and interfaces
│   ├── interface.go          # HashMethod interface and types
│   └── sha256_canonical.go  # Reference Double SHA-256 implementation
├── methods/                 # Specific hashing method implementations
│   ├── asic/              # Method 1: Direct ASIC Device Hashing
│   ├── software/           # Method 2: Software Fallback Hashing  
│   ├── cuda/               # Method 3: CUDA Simulator (Training Only!)
│   ├── ubpf/               # Method 4: uBPF lib binary on ASIC (Simulated)
│   └── ebpf/               # Method 5: eBPF OpenWRT Kernel (Future)
├── hardware/                # Hardware abstraction and detection
│   ├── bitcoin_header.go     # Bitcoin header preparation utilities
│   └── device_detector.go  # Auto-detection of available methods
├── factory/                # Factory pattern for method selection
│   ├── factory.go          # HashMethodFactory for intelligent selection
│   └── config.go           # Configuration management
└── example/                # Usage examples
    └── main.go            # Complete demonstration
```

## Hashing Methods

### 1. ASIC Hardware (`methods/asic`)
- **Purpose**: Direct access to Bitmain ASIC hardware via `/dev/bitmain-asic`
- **Interface**: USB device communication with gRPC server fallback
- **Performance**: ~500 GH/s (32 chips @ 250 MHz)
- **Status**: ✅ Production Ready

### 2. Software Fallback (`methods/software`)
- **Purpose**: Pure Go implementation using `crypto/sha256`
- **Interface**: Standard Go crypto library
- **Performance**: ~1 MH/s on typical CPU
- **Status**: ✅ Production Ready (fallback)

### 3. CUDA Simulator (`methods/cuda`)
- **Purpose**: GPU acceleration for training pipeline only
- **Interface**: CUDA kernels with full SHA-256 implementation
- **Performance**: ~50 GH/s on modern GPU
- **Status**: ⚠️ Training Only

### 4. uBPF Simulator (`methods/ubpf`)
- **Purpose**: uBPF simulation with USB and CGMiner API support
- **Interface**: USB device access + CGMiner API integration
- **Performance**: ~100 MH/s simulated
- **Status**: ⚠️ Simulation Only

### 5. eBPF OpenWRT (`methods/ebpf`)
- **Purpose**: eBPF kernel module on flashed ASIC (future)
- **Interface**: OpenWRT kernel integration
- **Performance**: TBD (dependent on implementation)
- **Status**: ❌ Future Implementation

## Key Features

### Automatic Hardware Detection
The system automatically detects available hardware:
- ASIC: Checks `/dev/bitmain-asic` accessibility
- CUDA: Uses `nvidia-smi` to detect GPUs
- uBPF: Searches for CGMiner binary and API access
- Software: Always available as fallback

### Canonical SHA-256 Implementation
All methods delegate to a single, canonical Double SHA-256 implementation:
- Ensures consistency across all methods
- Uses `crypto/sha256` for correctness
- Includes Bitcoin-specific mining utilities

### Factory Pattern with Fallback Chain
Intelligent method selection with configurable priority:
```go
// Production order (default)
[]string{"asic", "software", "cuda", "ubpf", "ebpf"}

// Training order
[]string{"cuda", "asic", "software", "ubpf", "ebpf"}
```

### Configuration Management
JSON-based configuration with runtime reloading:
```json
{
  "preferred_order": ["asic", "software", "cuda", "ubpf", "ebpf"],
  "asic_device": "/dev/bitmain-asic",
  "cgminer_path": "/opt/cgminer/cgminer",
  "enable_fallback": true,
  "training_mode": false
}
```

## Usage Examples

### Basic Usage
```go
import "hasher/pkg/hashing/factory"

// Create factory with default configuration
factory := factory.NewHashMethodFactory(nil)

// Get detection report
report := factory.GetDetectionReport()
fmt.Printf("Available methods: %+v\n", report)

// Get best available method
method := factory.GetBestMethod()

// Initialize method
if err := method.Initialize(); err != nil {
    log.Fatal(err)
}

// Use for hashing
hash, err := method.ComputeHash([]byte("test data"))

// Cleanup
method.Shutdown()
```

### Training Configuration
```go
// Use training-optimized configuration
config := factory.TrainingHashMethodConfig()
factory := factory.NewHashMethodFactory(config)
```

### Custom Configuration
```go
config := &factory.HashMethodConfig{
    PreferredOrder: []string{"cuda", "software"},
    EnableFallback: true,
    TrainingMode: true,
}

factory := factory.NewHashMethodFactory(config)
```

## Integration Points

### Updating Existing Code
To migrate from the old simulator package:

```go
// Old approach
// simulator := NewvHasherSimulator(config)

// New approach
factory := factory.NewHashMethodFactory(config)
method := factory.GetBestMethod()
```

### Interface Compatibility
All methods implement the `core.HashMethod` interface:
- `ComputeHash(data []byte) ([32]byte, error)`
- `ComputeBatch(data [][]byte) ([][32]byte, error)`
- `MineHeader(header []byte, nonceStart, nonceEnd uint32) (uint32, error)`
- `MineHeaderBatch(headers [][]byte, nonceStart, nonceEnd uint32) ([]uint32, error)`
- `GetCapabilities() *core.Capabilities`

## Benefits

1. **Consistency**: Single canonical SHA-256 implementation across all methods
2. **Flexibility**: Runtime method selection based on hardware availability
3. **Maintainability**: Clear separation of concerns and interfaces
4. **Performance**: Optimized paths for different use cases (production vs training)
5. **Reliability**: Automatic fallback ensures system always works
6. **Extensibility**: Easy to add new hashing methods

## Migration from Old Simulator

The old simulator package had inconsistent SHA-256 implementations:
- CUDA: Full implementation (accurate)
- uBPF/eBPF: Simplified implementations (inaccurate)
- vHasher: Software fallback (correct)

The new architecture fixes these issues by:
1. Extracting canonical implementation from CUDA as the gold standard
2. Making all methods delegate to the canonical core
3. Fixing hardware models (USB vs SPI confusion)
4. Adding proper hardware detection and fallback management

## Testing

Run the example to test all methods:
```bash
go run pkg/hashing/example/main.go
```

This will:
1. Detect all available hashing methods
2. Show hardware capabilities
3. Demonstrate each method with test data
4. Display performance characteristics

## Next Steps

1. **Complete uBPF CGMiner Integration**: Finish the CGMiner API implementation
2. **Direct USB Access**: Implement direct `/dev/bitmain-asic` communication
3. **Performance Optimization**: Optimize batch sizes and latency for each method
4. **Monitoring Integration**: Add metrics collection for all methods
5. **Testing Framework**: Comprehensive tests for all method combinations

## Troubleshooting

### Method Not Available
Check the detection report for failure reasons:
```go
report := factory.GetDetectionReport()
for _, method := range report.Methods {
    if !method.Available {
        fmt.Printf("%s unavailable: %s\n", method.Name, method.Capabilities.Reason)
    }
}
```

### Performance Issues
1. Ensure hardware detection is working correctly
2. Check if fallback to software is occurring unexpectedly
3. Verify batch sizes are appropriate for the method
4. Monitor hash rates against expected capabilities

### ASIC Device Issues
1. Stop CGMiner: `pkill cgminer`
2. Check device permissions: `ls -l /dev/bitmain-asic`
3. Verify kernel modules: `lsmod | grep bitmain`
4. Check dmesg for errors: `dmesg | grep -i bitmain`