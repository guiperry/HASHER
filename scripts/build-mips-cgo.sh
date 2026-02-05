#!/bin/bash
# Build script for MIPS cross-compilation with CGO
#
# Usage:
#   ./build-mips-cgo.sh <output-binary> <source.go> [static]
#
# Examples:
#   ./build-mips-cgo.sh bin/monitor-mips cmd/monitor/main.go
#   ./build-mips-cgo.sh bin/monitor-mips cmd/monitor/main.go static

set -e

# OpenWrt SDK paths
SDK_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/toolchain/openwrt-sdk-19.07.10-ar71xx-generic_gcc-7.5.0_musl.Linux-x86_64"
TOOLCHAIN="$SDK_ROOT/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl"
TARGET_ROOT="$SDK_ROOT/staging_dir/target-mips_24kc_musl"

# Cross-compiler
export CC="$TOOLCHAIN/bin/mips-openwrt-linux-musl-gcc"
export CXX="$TOOLCHAIN/bin/mips-openwrt-linux-musl-g++"
export AR="$TOOLCHAIN/bin/mips-openwrt-linux-musl-ar"

# OpenWrt staging directory (needed by toolchain)
export STAGING_DIR="$SDK_ROOT/staging_dir"

# CGO flags
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=mips
export GOMIPS=softfloat

# Include paths
export CGO_CFLAGS="-I$TARGET_ROOT/usr/include"

# Check if static build requested
STATIC_BUILD="${3:-}"
BUILD_FLAGS=""

if [ "$STATIC_BUILD" = "static" ]; then
    echo "🔧 MIPS Cross-Compilation Environment (STATIC BUILD)"
    echo "======================================================"
    # Static linking flags
    export CGO_LDFLAGS="-L$TARGET_ROOT/usr/lib -L$TOOLCHAIN/lib --sysroot=$TARGET_ROOT -static -lusb-1.0 -lpthread"
    BUILD_FLAGS="-ldflags '-extldflags \"-static\"'"
else
    echo "🔧 MIPS Cross-Compilation Environment (DYNAMIC BUILD)"
    echo "======================================================="
    # Dynamic linking flags
    export CGO_LDFLAGS="-L$TARGET_ROOT/usr/lib -L$TOOLCHAIN/lib -lusb-1.0 --sysroot=$TARGET_ROOT -Wl,-rpath-link=$TARGET_ROOT/usr/lib -Wl,--dynamic-linker=/lib/ld-musl-mips-sf.so.1"
fi

echo "CC: $CC"
echo "Target: MIPS 24Kc (softfloat)"
echo "Sysroot: $TARGET_ROOT"
echo "Linking: ${STATIC_BUILD:-dynamic}"
echo ""

# Build the specified Go program
if [ -z "$1" ]; then
    echo "Usage: $0 <output-binary> <source.go> [static]"
    echo "Examples:"
    echo "  $0 bin/monitor-mips cmd/monitor/main.go"
    echo "  $0 bin/monitor-mips cmd/monitor/main.go static"
    exit 1
fi

OUTPUT="$1"
SOURCE="$2"

echo "📦 Building: $SOURCE"
echo "📤 Output: $OUTPUT"
echo ""

if [ "$STATIC_BUILD" = "static" ]; then
    go build -ldflags '-extldflags "-static"' -o "$OUTPUT" "$SOURCE"
else
    go build -o "$OUTPUT" "$SOURCE"
fi

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ Build successful!"
    ls -lh "$OUTPUT"

    # Show linking info
    echo ""
    echo "Binary info:"
    file "$OUTPUT" | sed 's/^/  /'

    if [ "$STATIC_BUILD" = "static" ]; then
        echo "  Linking: statically linked (no external libraries needed)"
    else
        echo "  Linking: dynamically linked"
        if command -v readelf &> /dev/null; then
            echo ""
            echo "Required libraries:"
            readelf -d "$OUTPUT" | grep NEEDED | sed 's/^/  /'
        fi
    fi
else
    echo ""
    echo "❌ Build failed"
    exit 1
fi
