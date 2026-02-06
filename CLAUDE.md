# Hasher Proof of Concept

## Project Overview

Hasher transforms obsolete Bitcoin mining hardware (Antminer S2/S3) into a novel machine learning inference system by using SHA-256 ASIC chips as computational primitives for neural network operations. The primary architecture virtualizes a multi-node ensemble into a time-series process on a **single ASIC device**, combining this temporal ensemble learning with formal logical reasoning to achieve robust, explainable, and maximally cost-effective AI inference.

**Hardware:** Bitmain Antminer S3
- CPU: Atheros AR9330 (MIPS 24Kc @ 400MHz)
- RAM: 61MB
- ASIC: 32x BM1382 chips (~500 GH/s SHA-256 hashing)
- Interface: USB (not GPIO/SPI as originally planned)
- Device: `/dev/bitmain-asic` (character device, major 10, minor 60)

**Network:**
- IP: 192.168.12.151
- User: root
- Password: ********* (hardcoded in scripts)

## Architecture Notes

### Original vs Current Design

**Original design** (from architecture conversation):
- ARM + PRU (Programmable Real-time Unit) architecture
- 200 Hz target rotation via PRU GPIO control
- Direct memory-mapped hardware access

**Current reality** (Antminer S3):
- MIPS + USB architecture
- ~10 Hz target rotation limit (USB latency)
- Device driver interface via `/dev/bitmain-asic`

The USB bottleneck significantly reduces the target rotation rate, but the system remains quantum-resistant due to high hash velocity. Each target still requires ~2^48 hashes to reverse, cycling every 100ms.

### Device Communication

CGMiner communicates with the ASICs through `/dev/bitmain-asic` using:
- Baud rate: 115200
- 32 chips in 8 chains
- Target frequency: 250 MHz
- Voltage: 0x0982 (0.982V)

Command format: `--bitmain-options 115200:32:8:16:250:0982`

**Current challenge:** The device remains locked even after stopping CGMiner. The kernel driver may maintain an exclusive lock. Need to investigate:
- Kernel module reload (`rmmod`/`insmod`)
- Alternative device access methods
- CGMiner source code for IOCTL commands

## Build System

### Cross-Compilation

All tools must be compiled for MIPS architecture:

```bash
GOOS=linux GOARCH=mips GOMIPS=softfloat go build -o <output> <source>
```

### Make Targets

```bash
make build         # Build diagnostic tool (MIPS)
make build-probe   # Build device probe tool (MIPS)
make deploy        # Build and deploy diagnostics
make deploy-probe  # Build and deploy probe tool
make test          # Build, deploy, and run diagnostics
make clean         # Clean build artifacts
```

### SSH Requirements

The Antminer uses legacy SSH algorithms:

```bash
ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 \
    -o HostKeyAlgorithms=+ssh-rsa \
    -o StrictHostKeyChecking=no \
    root@192.168.12.151
```

SSH config is set up at `~/.ssh/config` with host alias `antminer`.

All deployment scripts use `sshpass` with hardcoded password for automation.

## Project Structure

```
KNIRVHASHER/
├── cmd/
│   ├── asic-test/main.go       # Comprehensive diagnostic tool
│   ├── device-probe/main.go     # Device access probe
│   └── device-provision/      # Future: provisioning tool
├── scripts/
│   ├── deploy.sh                # Deploy diagnostic tool
│   └── run-probe.sh             # Build, deploy, and run probe
├── Makefile                     # Build automation
├── README.md                    # User-facing documentation
├── probe_*.txt                  # Probe output files (gitignored)
└── diagnostics_*.txt            # Diagnostic output files (gitignored)
```

## Current Status

### ✅ Completed

1. **Network discovery and SSH access** - Successfully connected to Antminer
2. **Cross-compilation setup** - MIPS build pipeline working
3. **Diagnostic tool** - 6-phase system analysis complete
4. **Device probe tool** - Created to test direct device access
5. **Automated deployment** - Scripts with password automation
6. **Initial diagnostics** - Confirmed hardware configuration

### 🔄 In Progress

1. **Device access** - `/dev/bitmain-asic` is "busy" even after stopping CGMiner
   - CGMiner stops successfully (verified via `pgrep`)
   - Device exists with correct permissions
   - Both O_RDWR and O_RDONLY fail with "device or resource busy"
   - Likely kernel driver holds exclusive lock

### 📋 Next Steps

1. Resolve device busy lock:
   - Try `rmmod bitmain_asic` (or similar module name)
   - Check `lsmod` for loaded modules
   - Review `/proc/devices` for driver info
   - May need to modify kernel driver or find alternative access method

2. Protocol discovery:
   - Once device access achieved, capture binary data
   - Reverse-engineer command protocol
   - Identify IOCTL commands for ASIC control

3. ASIC control library:
   - Implement Go library for device communication
   - Support for chain initialization, target setting, result polling

4. Salt chain generator:
   - Implement HVRS algorithm
   - Build proof-of-concept authentication flow

## MIPS Compatibility Notes

### Syscall Differences

Standard Go syscall wrappers may not be available for MIPS. Use raw syscalls:

```go
// Instead of: flags, err := syscall.FcntlInt(...)
// Use:
flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFL, 0)
if errno != 0 {
    return errno
}
```

### USB Device ID

The ASIC chips appear as USB device `4254:4153` (hex for "BT-AS" = BiTmain ASIC).

## Performance Expectations

- **Hash rate:** ~500 GH/s (32 chips @ 250 MHz)
- **Salt generation:** ~10 Hz target rotation (USB limited)
- **Security margin:** ~2^48 hashes per target = ~5 minutes @ 500 GH/s
- **Quantum resistance:** Target rotates every 100ms, quantum computer needs years

While slower than the original 200 Hz PRU design, the 10 Hz USB-based approach still provides effective quantum resistance through high-velocity salt rotation.

## Development Workflow

1. **Make changes** to Go code
2. **Build for MIPS:** `make build-probe` (or `make build`)
3. **Deploy:** `make deploy-probe` (automatically uses sshpass)
4. **Run on device:** Script captures output to timestamped file
5. **Analyze results:** Read output file in project directory

All build artifacts and output files are gitignored.

## Key Files to Understand

- `cmd/device-probe/main.go` - Start here to understand device access approach
- `Makefile` - Build and deployment automation
- `scripts/run-probe.sh` - Full workflow automation example
- `probe_20251218_013640.txt` - Most recent probe results showing device busy issue

## Debugging Tips

1. **SSH issues:** Check legacy algorithm options in commands/scripts
2. **Build failures:** Verify GOOS/GOARCH/GOMIPS settings for MIPS
3. **Device access:** Ensure CGMiner is stopped and check kernel modules
4. **Missing dependencies:** `sshpass` required for automated deployment

## Security Note

**Password is hardcoded** in multiple locations (Makefile, scripts, code). This is acceptable for a local proof-of-concept but would need proper secret management for production use.
