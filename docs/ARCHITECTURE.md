# Hasher Host-Server Architecture

## Overview

The Hasher system consists of two main components that work together to provide AI inference capabilities using ASIC hardware:

- **Hasher Host**: Orchestrator running on user machine (CLI/API server)
- **Hasher Server**: Low-level service running directly on ASIC device

## Host vs Server Functionality Boundaries

### Hasher Host Responsibilities

**1. Orchestration & Management**
- API server for external interfaces (REST endpoints)
- Network discovery and device selection
- User interface (CLI/TUI) management
- Training loop orchestration
- Model lifecycle management

**2. Data Processing**
- Input data preprocessing and validation
- Training data batching and preparation
- Results aggregation and post-processing
- Temporal ensemble coordination

**3. Crypto-Transformer Operations**
- Model training orchestration
- High-level inference coordination
- Loss calculation and optimization
- Model serialization and checkpointing

**4. Device Management**
- ASIC device discovery and selection
- hasher-server binary deployment and cleanup
- Connection management and health monitoring
- Error handling and fallback logic

### Hasher Server Responsibilities

**1. Low-Level Hardware Operations**
- Direct ASIC hardware control via device drivers
- Raw SHA-256 hash computation
- Matrix seed encoding and decoding
- Hardware-level optimization

**2. Basic Computation Services**
- Single hash computation (`ComputeHash`)
- Batch hash processing (`ComputeBatch`)
- Streaming hash operations (`StreamCompute`)
- Device telemetry and metrics

**3. Network Communication**
- gRPC service endpoint
- Request/response handling
- Connection management
- Error reporting

**4. Resource Management**
- Memory allocation for hash operations
- Device temperature monitoring
- Performance metrics collection
- Hardware state management

## Data Flow Architecture

### Training Flow
```
CLI /train → Hasher Host → [Data Processing] → ASIC Client → Hasher Server → ASIC Hardware
                 ↑                                ↓
         [Model Updates]              [Hash Results]
                 ↓                                ↑
         [Training Loop] ← [Results Aggregation] ←
```

### Inference Flow
```
User Input → Hasher Host → [Preprocessing] → ASIC Client → Hasher Server → ASIC Hardware
                                        ↓                           ↑
                              [Temporal Ensemble]          [Hash Results]
                                        ↓                           ↑
                              [Consensus Formation] ← [Results Aggregation]
```

## Communication Protocols

### Host ↔ Server Communication
- **Protocol**: gRPC
- **Port**: 80 (default)
- **Serialization**: Protocol Buffers
- **Security**: TLS optional (for production)

### Host ↔ Client Communication
- **Protocol**: HTTP/REST
- **Port**: 8080 (default, auto-discovered)
- **Format**: JSON
- **Endpoints**:
  - `POST /api/v1/train` - Training requests
  - `POST /api/v1/infer` - Inference requests
  - `GET /api/v1/health` - Health checks

## Deployment Architecture

### Binary Embedding Strategy
```
hasher-host (embedded binary)
├── hasher-server (MIPS binary)
├── Deployment scripts
└── Cleanup utilities
```

### Auto-Deployment Process
1. **Discovery**: Hasher Host scans network for ASIC devices
2. **Selection**: User selects target device via CLI
3. **Deployment**: Host extracts and uploads hasher-server to ASIC
4. **Configuration**: Server is configured and started on device
5. **Connection**: Host establishes gRPC connection to server
6. **Verification**: Health check confirms server is operational

### Cleanup Strategy
- **Graceful Shutdown**: Server receives SIGTERM and saves state
- **Binary Removal**: Server binary and temp files are cleaned up
- **Connection Closure**: Host closes gRPC connections
- **Device Reset**: ASIC device is returned to clean state

## Failure Handling

### Host-Side Failures
- **Server Unavailable**: Fallback to software mode
- **Network Issues**: Automatic reconnection with backoff
- **Training Failures**: Checkpoint recovery and error reporting

### Server-Side Failures
- **Hardware Errors**: Device reset and status reporting
- **Memory Issues**: Cleanup and restart procedures
- **Communication Loss**: Heartbeat detection and reconnection

## Security Considerations

### Binary Security
- **Signed Binaries**: Cryptographic verification of embedded binaries
- **Checksum Validation**: Integrity checks before deployment
- **Sandboxed Execution**: Restricted execution environment on ASIC

### Network Security
- **Authentication**: Mutual TLS between host and server
- **Authorization**: Role-based access control
- **Encryption**: Encrypted communication channels

## Performance Optimization

### Host-Side Optimizations
- **Batch Processing**: Efficient data batching for ASIC
- **Parallel Processing**: Concurrent request handling
- **Memory Management**: Efficient buffer management
- **Caching**: Result caching for repeated queries

### Server-Side Optimizations
- **Hardware Utilization**: Maximum ASIC throughput
- **Memory Pooling**: Reusable memory buffers
- **Pipeline Optimization**: Optimized hash computation pipelines
- **Direct Memory Access**: Zero-copy operations where possible

## Development Workflow

### Build Process
```bash
# Build for multiple architectures
make build-all

# Cross-compile for MIPS (ASIC)
GOOS=linux GOARCH=mips GOMIPS=softfloat go build -o hasher-server cmd/driver/hasher-server

# Embed binaries in host
go embed -o embedded/binaries.go bin/hasher-server-*
```

### Testing Strategy
- **Unit Tests**: Individual component testing
- **Integration Tests**: Host-server communication
- **Hardware Tests**: Real ASIC device testing
- **Performance Tests**: Benchmarking and profiling

## Configuration Management

### Host Configuration
```yaml
host:
  api_port: 8080
  discovery_subnet: "192.168.1.0/24"
  auto_deploy: true
  cleanup_on_exit: true
```

### Server Configuration
```yaml
server:
  grpc_port: 80
  device_path: "/dev/bitmain-asic"
  max_connections: 10
  health_check_interval: 30s
```

## Future Extensions

### Multi-Device Support
- **Device Farm Management**: Multiple ASIC devices per host
- **Load Balancing**: Distribute work across devices
- **Aggregation**: Combine results from multiple devices

### Cloud Integration
- **Remote Deployment**: Cloud-based ASIC management
- **Federation**: Multiple hosts sharing resources
- **Monitoring**: Centralized telemetry and logging