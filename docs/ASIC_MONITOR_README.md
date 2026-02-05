# ASIC MONITOR

This document outlines the complete development workflow for the ASIC MONITOR, covering setup, building, deployment, and testing processes.

## 📋 Table of Contents

1. [Project Overview](#project-overview)
2. [Prerequisites](#prerequisites)
3. [Environment Setup](#environment-setup)
4. [Project Structure](#project-structure)
5. [Build Process](#build-process)
6. [Deployment Process](#deployment-process)
7. [Testing and Validation](#testing-and-validation)
8. [Troubleshooting](#troubleshooting)
9. [Development Best Practices](#development-best-practices)

## 🎯 Project Overview

Hasher transforms obsolete Bitcoin mining hardware (Antminer S2/S3) into a novel machine learning inference system by using SHA-256 ASIC chips as computational primitives for neural network operations. The primary architecture virtualizes a multi-node ensemble into a time-series process on a **single ASIC device**, combining this temporal ensemble learning with formal logical reasoning to achieve robust, explainable, and maximally cost-effective AI inference. The project consists of multiple specialized programs for different aspects of ASIC communication and monitoring.

## 🎯 Project Status

**Phase: Protocol Discovery**

We're currently in the initial discovery phase, understanding how to communicate with the ASIC chips through the USB interface.

### Current Focus: ASIC Monitor
The primary development focus is currently on the `asic-monitor` program, which provides real-time monitoring and communication with ASIC chips via USB interface.

## 🔧 Prerequisites

### Required Tools
- **Go 1.24+** - Primary development language
- **Make** - Build automation
- **SSH client** - Remote deployment
- **sshpass** - Password-based SSH authentication
- **Git** - Version control

### Hardware Requirements
- **Antminer S3** device (or compatible Bitmain hardware)
- **Network connectivity** to the Antminer device
- **USB access** to ASIC chips (for monitor program)

### Hardware Configuration
- **Device**: Antminer S3
- **Controller**: Atheros AR9330 (MIPS 24Kc @ 400MHz)
- **RAM**: 61MB
- **ASIC Chips**: 32x BM1382 (~500 GH/s total)
- **Interface**: USB (`/dev/bitmain-asic`)
- **Architecture**: MIPS (not ARM+PRU as originally planned)

### Important Note
This hardware does **not** have the PRU subsystem we originally designed for. We're adapting the architecture to work with USB-based communication, which limits salt rotation to ~10 Hz (vs 200 Hz with PRU) but still provides excellent security (17,823 years to crack vs 356,458 years).


### Software Dependencies
- **OpenWrt SDK** - MIPS cross-compilation toolchain
- **libusb-1.0** - USB communication library
- **CGO enabled** - For USB integration

## 🌍 Environment Setup

### 1. Clone and Initialize
```bash
git clone https://github.com/guiperry/KNIRVHASHER.git
cd KNIRVHASHER
```

### 2. Configure Device Connection
Create or update `.env` file:
```bash
DEVICE_IP=192.168.12.151
DEVICE_PASSWORD=keperu100
```

### 3. Setup Toolchain
```bash
# Install development toolchain
./scripts/dev-toolchain-setup.sh
```

### 4. Verify SSH Connection
```bash
# Test basic connectivity
ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    root@192.168.12.151 "echo 'Connection successful'"
```

## 📁 Project Structure

```
KNIRVHASHER/
├── cmd/                         # Main programs
│   ├── asic-monitor/            # ASIC monitoring tool (current focus)
├── internal/                    # Private libraries
│   ├── asic/                    # ASIC interface
│   ├── antminer/                # Antminer client
├── scripts/                     # Automation scripts
│   ├── deploy-monitor-usb.sh    # USB-based monitor deployment
│   ├── run-monitor.sh           # Standard monitor deployment
│   ├── deploy.sh                # Generic deployment
│   └── build-mips-cgo.sh        # MIPS build script
├── bin/                         # Compiled binaries (auto-generated)
├── logs/                        # Runtime logs
├── toolchain/                   # Cross-compilation tools
├── docs/                        # Documentation
├── Makefile                     # Build automation
└── go.mod                       # Go module definition
```

## 🔨 Build Process

### Important: Use Make, Not Direct Go Build
**Do NOT use `go build` directly.** Always use the Makefile targets for proper cross-compilation.

### Available Programs
- `asic-test` - Diagnostic and testing tool
- `asic-monitor` - Real-time ASIC monitoring (current focus)
- `device-probe` - Basic device discovery
- `device-probe-v2` - Enhanced device discovery
- `protocol-discover` - Protocol analysis tool
- `device-provision` - Device provisioning

### Build Commands

#### Build Individual Programs
```bash
# Build ASIC monitor (current focus)
make build-monitor

# Build diagnostic tool
make build

# Build device probe
make build-probe

# Build enhanced probe v2
make build-probe-v2

# Build protocol discovery
make build-protocol-discover
```

#### Build All Programs
```bash
# Build all available programs
make build build-probe build-probe-v2 build-protocol-discover build-monitor
```

### Build Output
- **Location**: `bin/` directory
- **Naming**: `{program-name}-mips`
- **Architecture**: MIPS (softfloat)
- **Static Linking**: Enabled for monitor program

### Cross-Compilation Details
The Makefile handles cross-compilation for MIPS architecture:
```bash
GOOS=linux GOARCH=mips GOMIPS=softfloat
```

For the monitor program with USB support:
```bash
CGO_ENABLED=1 \
CC=$(SDK_ROOT)/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl/bin/mips-openwrt-linux-musl-gcc \
CGO_CFLAGS="-I$(SDK_ROOT)/staging_dir/target-mips_24kc_musl/usr/include" \
CGO_LDFLAGS="-L$(SDK_ROOT)/staging_dir/target-mips_24kc_musl/usr/lib -lusb-1.0 -static" \
GOOS=linux GOARCH=mips GOMIPS=softfloat \
go build -ldflags '-extldflags "-static"' -o bin/monitor-mips cmd/monitor/main.go
```

## 🚀 Deployment Process

### Current Focus: ASIC Monitor Deployment

There are two deployment methods for the ASIC monitor:

#### Method 1: USB-Based Deployment (Recommended)
```bash
# Deploy and run with USB direct access
./scripts/deploy-monitor-usb.sh
```

This method:
- Builds the monitor with USB support
- Deploys libusb-1.0 library to device
- Sets up proper library paths
- Runs monitor in USB mode with status dumping

#### Method 2: Standard Deployment
```bash
# Deploy and run standard monitor
./scripts/run-monitor.sh
```

This method:
- Uses standard device interface
- Handles kernel module management
- Runs monitor with device file access

### Deployment Steps (Manual Process)

#### 1. Build the Program
```bash
make build-monitor
```

#### 2. Deploy to Device
```bash
make deploy-monitor
```

#### 3. Run on Device
```bash
# SSH into device and run
sshpass -p '$DEVICE_PASSWORD' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    root@$DEVICE_IP '/tmp/monitor --dump-status --dump-interval 2'
```

### Deployment for Other Programs

#### Device Probe
```bash
# Build and deploy device probe
make deploy-probe

# Run on device
sshpass -p '$DEVICE_PASSWORD' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    root@$DEVICE_IP '/tmp/device-probe'
```

#### Protocol Discovery
```bash
# Build and deploy protocol discovery
make deploy-protocol-discover

# Run on device
sshpass -p '$DEVICE_PASSWORD' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    root@$DEVICE_IP '/tmp/protocol-discover'
```

## 🧪 Testing and Validation

### Automated Testing
```bash
# Run full diagnostic test
make test
```

### Manual Testing Workflow

#### 1. Verify Build
```bash
# Check binary exists and is executable
ls -lh bin/monitor-mips
file bin/monitor-mips
```

#### 2. Test Deployment
```bash
# Deploy and check if file exists on device
make deploy-monitor
sshpass -p '$DEVICE_PASSWORD' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    root@$DEVICE_IP 'ls -la /tmp/monitor'
```

#### 3. Run and Monitor
```bash
# Run with output capture
./scripts/run-monitor.sh

# Check logs
tail -f logs/monitor_*.log
```

### Validation Criteria

#### Successful USB Deployment
- ✅ USB device opened successfully
- ✅ Interface claimed successfully
- ✅ Data sent and received
- ✅ Status parsed and logged
- ✅ JSON dump entries created

#### Successful Standard Deployment
- ✅ Device opened successfully
- ✅ TxConfig packet sent
- ✅ TxTask packet sent
- ✅ RxStatus parsed
- ✅ Dump logs created


## 📊 Next Steps

### Immediate (Phase 1)
- [x] Build diagnostic tool
- [x] Automated deployment
- [ ] Run full diagnostics
- [ ] Analyze protocol from cgminer

### Short Term (Phase 2)
- [ ] Implement USB ASIC communication
- [ ] Test hash work submission
- [ ] Verify ASIC control

### Medium Term (Phase 3)
- [ ] Build salt chain generator
- [ ] Implement 10 Hz rotation
- [ ] Create authentication endpoint

### Long Term (Phase 4)
- [ ] Full HVRS implementation
- [ ] Threat detection
- [ ] Performance benchmarks
- [ ] Demo video

## 🔐 Security Architecture

### Moving Target Defense
- Salt rotation: 10 Hz (100ms intervals)
- Temporal tolerance: ±500ms
- Attack resistance: 17,823 years (quantum)

### Performance Expectations
| Metric | Value |
|--------|-------|
| Salt Generation | 1,000/sec |
| Auth Latency (p99) | <200ms |
| Power Consumption | 5-50W |
| Quantum Attack Time | 17,823 years |



## 🔧 Troubleshooting

### Common Issues

#### SSH Connection Problems
```bash
# Use legacy algorithms
ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    root@$DEVICE_IP
```

#### Build Failures
```bash
# Clean and rebuild
make clean
make build-monitor

# Check toolchain
ls -la toolchain/openwrt-sdk-*/
```

#### Device Access Issues
```bash
# Check device permissions
ssh root@$DEVICE_IP 'ls -la /dev/bitmain-asic'

# Check kernel modules
ssh root@$DEVICE_IP 'lsmod | grep bitmain'

# Restart if needed
ssh root@$DEVICE_IP 'rmmod bitmain_asic'
```

#### USB Library Issues
```bash
# Check libusb deployment
ssh root@$DEVICE_IP 'ls -la /tmp/libusb-1.0.so.0'

# Check library path
ssh root@$DEVICE_IP 'LD_LIBRARY_PATH=/tmp:/usr/lib ldd /tmp/monitor'
```

#### Go Version Mismatch Issues
If you encounter errors like "Go version mismatch" or "compiled packages for go1.24.1" when building, check your Go version manager (GVM) configuration:

```bash
# Check current Go version
go version

# Check GVM status
gvm list

# Ensure you have go1.24.11 installed (matches go.mod)
gvm install go1.24.11

# Switch to go1.24.11
gvm use go1.24.11

# Clear Go build cache
go clean -cache

# Rebuild
make clean
make build-monitor
```

The project requires Go 1.24.11 as specified in `go.mod`. If using GVM, ensure it's set to the correct version.

### Debug Commands

#### Device Diagnostics
```bash
# Check USB devices
ssh root@$DEVICE_IP 'lsusb | grep 4254'

# Check system info
ssh root@$DEVICE_IP 'cat /proc/cpuinfo'

# Check kernel logs
ssh root@$DEVICE_IP 'dmesg | tail -30'
```

#### Network Diagnostics
```bash
# Test connectivity
ping $DEVICE_IP

# Check SSH
ssh -v -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    root@$DEVICE_IP 'echo "SSH OK"'
```

## 📋 Development Best Practices

### Code Organization
- Keep program-specific code in `cmd/` directories
- Use `internal/` for shared libraries
- Maintain consistent naming conventions
- Document interfaces and protocols

### Build Management
- Always use Makefile targets, not direct `go build`
- Test builds on clean environments
- Verify cross-compilation output
- Check binary sizes and dependencies

### Deployment Safety
- Test deployments on non-production devices first
- Use version-specific binary names
- Maintain backup copies of working versions
- Document deployment parameters

### Logging and Monitoring
- Use structured logging with timestamps
- Capture both stdout and stderr
- Maintain log rotation for long-running processes
- Include device information in logs

### Version Control
- Commit Makefile changes with build updates
- Tag releases with corresponding binary versions
- Document breaking changes in commit messages
- Maintain changelog for deployment scripts

## 🔄 Development Cycle

### Typical Development Workflow

1. **Setup Environment**
   ```bash
   # Ensure toolchain is ready
   ./scripts/dev-toolchain-setup.sh
   ```

2. **Make Changes**
   ```bash
   # Edit source code
   vim cmd/monitor/main.go
   ```

3. **Build and Test Locally**
   ```bash
   # Build for target architecture
   make build-monitor
   
   # Verify binary
   ls -lh bin/monitor-mips
   ```

4. **Deploy and Test**
   ```bash
   # Deploy to device
   ./scripts/deploy-monitor-usb.sh
   
   # Monitor results
   tail -f logs/monitor-usb_*.log
   ```

5. **Validate Results**
   ```bash
   # Check for success indicators
   grep "USB device opened" logs/monitor-usb_*.log
   grep "Parsed RxStatus" logs/monitor-usb_*.log
   ```

6. **Iterate**
   ```bash
   # Clean and rebuild if needed
   make clean
   make build-monitor
   ```

### Quick Reference Commands

```bash
# Help - see all available targets
make help

# Clean build artifacts
make clean

# Build current focus program
make build-monitor

# Deploy current focus program (USB)
./scripts/deploy-monitor-usb.sh

# Deploy current focus program (standard)
./scripts/run-monitor.sh

# Check logs
ls -la logs/
tail -f logs/monitor*.log
```

## 📚 Additional Resources

- [Project README](README.md) - General project information
- [Protocol Documentation](docs/BITMAIN-PROTOCOL.md) - ASIC protocol details
- [Kernel Driver Info](docs/KERNEL_DRIVER.md) - Driver implementation
- [Toolchain Setup](docs/TOOLCHAIN-SETUP.md) - Development environment

---

**Last Updated**: December 21, 2024  
**Maintainer**: Hasher Development Team  
**Version**: 1.0
