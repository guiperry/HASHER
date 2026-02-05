# OpenWrt SDK Toolchain Setup Guide

This document explains how to set up the cross-compilation toolchain for building MIPS binaries that run on the Antminer S3.

## Quick Start

For new developers joining the project:

```bash
# Run the automated setup script
./scripts/dev-toolchain-setup.sh
```

This will:
1. Download OpenWrt SDK (~50MB)
2. Extract and configure it
3. Update package feeds
4. Compile libusb-1.0 with development libraries
5. Verify the toolchain is ready

**Time required:** ~5-10 minutes (depending on network speed)

## What Gets Installed

The setup script creates:

```
KNIRVHASHER/
├── openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64/
│   ├── staging_dir/
│   │   ├── toolchain-mips_24kc_gcc-7.5.0_musl/    # Cross-compiler
│   │   └── target-mips_24kc_musl/                  # Target sysroot
│   │       └── usr/
│   │           ├── include/libusb-1.0/            # Headers
│   │           └── lib/
│   │               ├── libusb-1.0.a               # Static library
│   │               └── libusb-1.0.so*             # Shared library
│   └── build_dir/                                  # Build artifacts
└── scripts/
    ├── dev-toolchain-setup.sh                      # This setup script
    └── build-mips-cgo.sh                           # Build helper (auto-created)
```

## System Requirements

### Operating System
- Linux (x86_64)
- Ubuntu 20.04+ recommended
- Other distros should work but may need package name adjustments

### Required Packages
```bash
sudo apt-get install wget tar make gcc g++ gawk unzip python3
```

The setup script will check for these and prompt you to install if missing.

### Disk Space
- SDK download: ~50MB
- Extracted SDK: ~250MB
- Total required: ~500MB (including build artifacts)

## Manual Setup Steps

If you prefer to set up manually or the automated script fails:

### 1. Download SDK
```bash
cd /home/gperry/Documents/GitHub/KNIRVHASHER
wget https://downloads.openwrt.org/releases/19.07.10/targets/ar71xx/generic/openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64.tar.xz
```

### 2. Extract
```bash
tar -xJf openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64.tar.xz
```

### 3. Update Feeds
```bash
cd openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64
./scripts/feeds update -a
```

### 4. Compile libusb
```bash
make package/libusb/compile V=s
```

This creates:
- Unstripped shared libraries (with symbols for linking)
- Static libraries
- Development headers

### 5. Verify
```bash
# Check that the library has symbols
nm -D staging_dir/target-mips_24kc_musl/usr/lib/libusb-1.0.so | grep libusb_init

# Should output something like:
# 000031d1 T libusb_init
```

## Building MIPS Binaries

### Using the Build Script

```bash
./scripts/build-mips-cgo.sh <output> <source>
```

Example:
```bash
./scripts/build-mips-cgo.sh bin/monitor-mips cmd/monitor/main.go
```

### Manual Build

```bash
# Set environment variables
export SDK_ROOT="/home/gperry/Documents/GitHub/KNIRVHASHER/openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64"
export CC="$SDK_ROOT/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl/bin/mips-openwrt-linux-musl-gcc"
export STAGING_DIR="$SDK_ROOT/staging_dir"
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=mips
export GOMIPS=softfloat
export CGO_CFLAGS="-I$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/include"
export CGO_LDFLAGS="-L$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/lib -lusb-1.0"

# Build
go build -o bin/monitor-mips cmd/monitor/main.go
```

## Verifying Built Binaries

### Check Architecture
```bash
file bin/monitor-mips
# Should show: ELF 32-bit MSB executable, MIPS, MIPS32 rel2 version 1 (SYSV)
```

### Check Dependencies
```bash
cd openwrt-sdk-*/staging_dir/toolchain-*/bin
./mips-openwrt-linux-musl-readelf -d /path/to/bin/monitor-mips | grep NEEDED
```

Expected output:
```
0x00000001 (NEEDED)    Shared library: [libusb-1.0.so.0]
0x00000001 (NEEDED)    Shared library: [libgcc_s.so.1]
0x00000001 (NEEDED)    Shared library: [libc.so]
```

## Troubleshooting

### "undefined reference to libusb_*"

This means the linker can't find libusb symbols. Likely causes:

1. **Library was stripped:** The library in staging_dir must have symbols
   ```bash
   # Check if library has symbols
   nm -D staging_dir/target-mips_24kc_musl/usr/lib/libusb-1.0.so | grep libusb_init
   ```

   If no output, rebuild libusb:
   ```bash
   cd openwrt-sdk-*
   make package/libusb/clean
   make package/libusb/compile V=s
   ```

2. **Wrong library path:** Check CGO_LDFLAGS points to the correct location

### "cannot find -lusb-1.0"

The linker can't find the library file at all.

```bash
# Verify library exists
ls -la openwrt-sdk-*/staging_dir/target-mips_24kc_musl/usr/lib/libusb*
```

If missing, run: `make package/libusb/compile V=s`

### Feed Update Times Out

If `./scripts/feeds update -a` hangs or is very slow:

1. Check network connectivity
2. Try updating feeds individually:
   ```bash
   ./scripts/feeds update base
   ./scripts/feeds update packages
   ```

### Build Errors "staging_dir not set"

The OpenWrt toolchain requires the STAGING_DIR environment variable:

```bash
export STAGING_DIR="$(pwd)/staging_dir"
```

This is automatically set by `build-mips-cgo.sh`.

## Architecture Details

### Target Platform
- **Device:** Bitmain Antminer S3
- **SoC:** Atheros AR9330
- **CPU:** MIPS 24Kc @ 400MHz
- **Architecture:** MIPS32 Release 2
- **Endianness:** Big-endian (MSB)
- **ABI:** o32
- **FPU:** Software float (softfloat)
- **C Library:** musl libc

### Why OpenWrt SDK?

The Antminer runs a customized OpenWrt firmware. Using the OpenWrt SDK ensures:

1. **Binary compatibility** - Same toolchain version as the device firmware
2. **C library compatibility** - musl libc matches the device
3. **Correct MIPS variant** - 24Kc with softfloat, not other MIPS variants
4. **Package integration** - Access to OpenWrt's pre-built packages

### CGO Requirements

For Go programs that use C libraries (like gousb → libusb):

- **CGO_ENABLED=1** - Enable CGO
- **Cross-compiler** - MIPS-targeted GCC
- **Sysroot** - Headers and libraries for target system
- **Proper linking** - Libraries must be unstripped for static analysis

## Advanced Topics

### Building Other Packages

The SDK can build any OpenWrt package:

```bash
cd openwrt-sdk-*
./scripts/feeds install <package-name>
make package/<package-name>/compile V=s
```

Compiled packages appear in: `bin/packages/mips_24kc/base/`

### Cross-Compiling C/C++ Code

```bash
export CC="$SDK_ROOT/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl/bin/mips-openwrt-linux-musl-gcc"
export CXX="$SDK_ROOT/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl/bin/mips-openwrt-linux-musl-g++"
export STAGING_DIR="$SDK_ROOT/staging_dir"

$CC -o hello hello.c
```

### Inspecting Binaries

```bash
# Get binary info
file <binary>

# Check dependencies
readelf -d <binary>

# Dump symbols
nm -D <binary>

# Check architecture details
readelf -h <binary>
```

## References

- [OpenWrt SDK Documentation](https://openwrt.org/docs/guide-developer/toolchain/using_the_sdk)
- [MIPS Architecture](https://en.wikipedia.org/wiki/MIPS_architecture)
- [musl libc](https://musl.libc.org/)
- [Go CGO Documentation](https://pkg.go.dev/cmd/cgo)

## Support

If you encounter issues not covered here:

1. Check build logs in `/tmp/libusb-build.log`
2. Verify Go version supports CGO cross-compilation (1.5+)
3. Ensure STAGING_DIR is set before building
4. Try a clean rebuild: `make package/libusb/clean && make package/libusb/compile`

For project-specific issues, see `CLAUDE.md` in the project root.
