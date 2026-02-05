#!/bin/bash
# Development Toolchain Setup for Hasher POC
#
# This script sets up the OpenWrt SDK cross-compilation environment
# for building Go applications with CGO support targeting MIPS 24Kc
# architecture (Bitmain Antminer S3).
#
# What this script does:
# 1. Downloads OpenWrt SDK for ar71xx (MIPS 24Kc)
# 2. Extracts and configures the SDK
# 3. Updates package feeds
# 4. Compiles libusb-1.0 with proper development libraries
# 5. Verifies the toolchain is ready for use
#
# Usage: ./scripts/dev-toolchain-setup.sh

set -e  # Exit on error

# Configuration
SDK_VERSION="19.07.10"
SDK_TARGET="ar71xx-generic"
SDK_GCC="gcc-7.5.0"
SDK_LIBC="musl"
SDK_HOST="Linux-x86_64"
SDK_NAME="openwrt-sdk-${SDK_VERSION}-${SDK_TARGET}_${SDK_GCC}_${SDK_LIBC}.${SDK_HOST}"
SDK_URL="https://downloads.openwrt.org/releases/${SDK_VERSION}/targets/ar71xx/generic/${SDK_NAME}.tar.xz"

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SDK_ROOT="$PROJECT_ROOT/toolchain/$SDK_NAME"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Helper functions
print_header() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${BLUE}$1${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

print_step() {
    echo -e "${GREEN}✓${NC} $1"
}

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

check_dependencies() {
    print_header "Checking Dependencies"

    local deps=(wget tar make gcc g++ gawk unzip python3)
    local missing=()

    for dep in "${deps[@]}"; do
        if command -v "$dep" &> /dev/null; then
            print_step "$dep installed"
        else
            missing+=("$dep")
            print_error "$dep not found"
        fi
    done

    if [ ${#missing[@]} -ne 0 ]; then
        echo ""
        print_error "Missing dependencies: ${missing[*]}"
        echo ""
        echo "Install with:"
        echo "  sudo apt-get install ${missing[*]}"
        exit 1
    fi

    print_step "All dependencies satisfied"
}

download_sdk() {
    print_header "Downloading OpenWrt SDK"

    mkdir -p "$PROJECT_ROOT/toolchain"
    cd "$PROJECT_ROOT/toolchain"

    if [ -f "${SDK_NAME}.tar.xz" ]; then
        print_info "SDK tarball already exists"
        print_step "Using cached ${SDK_NAME}.tar.xz"
    else
        print_info "Downloading from: $SDK_URL"
        print_info "This may take a few minutes (file is ~50MB)..."
        echo ""

        if wget -c "$SDK_URL"; then
            print_step "Download complete"
        else
            print_error "Download failed"
            exit 1
        fi
    fi

    # Verify file exists and has content
    if [ ! -s "${SDK_NAME}.tar.xz" ]; then
        print_error "SDK tarball is empty or missing"
        exit 1
    fi

    local size=$(du -h "${SDK_NAME}.tar.xz" | cut -f1)
    print_info "SDK tarball size: $size"
}

extract_sdk() {
    print_header "Extracting OpenWrt SDK"

    cd "$PROJECT_ROOT/toolchain"

    if [ -d "$SDK_ROOT" ]; then
        print_warn "SDK directory already exists"
        read -p "Remove and re-extract? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            print_info "Removing existing SDK directory..."
            rm -rf "$SDK_ROOT"
        else
            print_info "Using existing SDK directory"
            return 0
        fi
    fi

    print_info "Extracting ${SDK_NAME}.tar.xz..."
    if tar -xJf "${SDK_NAME}.tar.xz"; then
        print_step "Extraction complete"
    else
        print_error "Extraction failed"
        exit 1
    fi

    # Verify directory structure
    if [ ! -d "$SDK_ROOT/staging_dir" ]; then
        print_error "SDK structure invalid (missing staging_dir)"
        exit 1
    fi

    print_step "SDK directory structure verified"
}

update_feeds() {
    print_header "Updating Package Feeds"

    cd "$SDK_ROOT"

    print_info "This step fetches package definitions from OpenWrt repositories"
    print_info "It may take 2-5 minutes depending on network speed..."
    echo ""

    # Update feeds with timeout
    if timeout 300 ./scripts/feeds update -a 2>&1 | while IFS= read -r line; do
        # Show feed updates but suppress excessive output
        if [[ "$line" =~ "Updating feed" ]] || [[ "$line" =~ "Create index" ]]; then
            echo "  $line"
        fi
    done; then
        print_step "Package feeds updated successfully"
    else
        exit_code=$?
        if [ $exit_code -eq 124 ]; then
            print_error "Feed update timed out (> 5 minutes)"
            print_warn "You may need to check your network connection"
        else
            print_error "Feed update failed with exit code: $exit_code"
        fi
        exit 1
    fi

    # Verify feeds were created
    if [ -f "feeds/base.index" ] && [ -f "feeds/packages.index" ]; then
        print_step "Feed indexes created successfully"
    else
        print_warn "Some feed indexes may be missing"
    fi
}

compile_libusb() {
    print_header "Compiling libusb for MIPS"

    cd "$SDK_ROOT"

    print_info "Building libusb-1.0 with development libraries..."
    print_info "This creates unstripped libraries needed for CGO linking"
    print_info "Build time: ~30 seconds"
    echo ""

    # Clean any previous builds
    if [ -d "build_dir/target-mips_24kc_musl/libusb-1.0.22" ]; then
        print_info "Cleaning previous build..."
        make package/libusb/clean > /dev/null 2>&1 || true
    fi

    # Compile libusb
    print_info "Running: make package/libusb/compile V=s"
    if make package/libusb/compile V=s > /tmp/libusb-build.log 2>&1; then
        print_step "libusb compilation successful"
    else
        print_error "libusb compilation failed"
        echo ""
        echo "Last 50 lines of build log:"
        tail -50 /tmp/libusb-build.log
        exit 1
    fi

    # Verify libraries were created
    print_info "Verifying build artifacts..."

    local lib_static="$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/lib/libusb-1.0.a"
    local lib_shared="$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/lib/libusb-1.0.so"
    local lib_header="$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/include/libusb-1.0/libusb.h"

    if [ -f "$lib_static" ]; then
        local size=$(du -h "$lib_static" | cut -f1)
        print_step "Static library: libusb-1.0.a ($size)"
    else
        print_error "Static library not found"
        exit 1
    fi

    if [ -L "$lib_shared" ]; then
        print_step "Shared library: libusb-1.0.so (symlink)"
    else
        print_error "Shared library not found"
        exit 1
    fi

    if [ -f "$lib_header" ]; then
        print_step "Header file: libusb-1.0/libusb.h"
    else
        print_error "Header file not found"
        exit 1
    fi

    # Verify library has symbols (not stripped)
    print_info "Checking library symbols..."
    if nm -D "$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/lib/libusb-1.0.so.0.1.0" | grep -q "libusb_init"; then
        print_step "Library symbols verified (unstripped, suitable for linking)"
    else
        print_error "Library appears to be stripped (cannot link)"
        exit 1
    fi
}

verify_toolchain() {
    print_header "Verifying Toolchain"

    local toolchain="$SDK_ROOT/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl"
    local target="$SDK_ROOT/staging_dir/target-mips_24kc_musl"

    print_info "Checking toolchain components..."

    # Check cross-compiler
    if [ -x "$toolchain/bin/mips-openwrt-linux-musl-gcc" ]; then
        local gcc_version=$("$toolchain/bin/mips-openwrt-linux-musl-gcc" --version | head -1)
        print_step "Cross-compiler: $gcc_version"
    else
        print_error "Cross-compiler not found"
        exit 1
    fi

    # Check target sysroot
    if [ -d "$target/usr/lib" ] && [ -d "$target/usr/include" ]; then
        print_step "Target sysroot: Ready"
    else
        print_error "Target sysroot incomplete"
        exit 1
    fi

    # Check required libraries
    local required_libs=(
        "$target/usr/lib/libusb-1.0.so"
        "$target/usr/lib/libc.so"
    )

    for lib in "${required_libs[@]}"; do
        if [ -e "$lib" ]; then
            print_step "Found: $(basename "$lib")"
        else
            print_error "Missing: $(basename "$lib")"
            exit 1
        fi
    done

    print_step "Toolchain verification complete"
}

create_build_script() {
    print_header "Creating Build Helper Script"

    local build_script="$PROJECT_ROOT/scripts/build-mips-cgo.sh"

    if [ -f "$build_script" ]; then
        print_info "Build script already exists: $build_script"
        print_step "Skipping creation"
        return 0
    fi

    cat > "$build_script" << 'EOF'
#!/bin/bash
# Build script for MIPS cross-compilation with CGO

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

# Include paths and library paths with sysroot
export CGO_CFLAGS="-I$TARGET_ROOT/usr/include"
export CGO_LDFLAGS="-L$TARGET_ROOT/usr/lib -L$TOOLCHAIN/lib -lusb-1.0 --sysroot=$TARGET_ROOT -Wl,-rpath-link=$TARGET_ROOT/usr/lib -Wl,--dynamic-linker=/lib/ld-musl-mips-sf.so.1"

echo "🔧 MIPS Cross-Compilation Environment"
echo "======================================"
echo "CC: $CC"
echo "Target: MIPS 24Kc (softfloat)"
echo "Sysroot: $TARGET_ROOT"
echo ""

# Build the specified Go program
if [ -z "$1" ]; then
    echo "Usage: $0 <output-binary> <source.go>"
    echo "Example: $0 bin/monitor-mips cmd/monitor/main.go"
    exit 1
fi

OUTPUT="$1"
SOURCE="$2"

echo "📦 Building: $SOURCE"
echo "📤 Output: $OUTPUT"
echo ""

go build -o "$OUTPUT" "$SOURCE"

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ Build successful!"
    ls -lh "$OUTPUT"
else
    echo ""
    echo "❌ Build failed"
    exit 1
fi
EOF

    chmod +x "$build_script"
    print_step "Created: $build_script"
}

print_summary() {
    print_header "Setup Complete!"

    echo -e "${GREEN}✓ OpenWrt SDK ready at:${NC}"
    echo "  $SDK_ROOT"
    echo ""

    echo -e "${GREEN}✓ Toolchain components:${NC}"
    echo "  • Cross-compiler: mips-openwrt-linux-musl-gcc 7.5.0"
    echo "  • C library: musl"
    echo "  • Target: MIPS 24Kc (softfloat)"
    echo "  • Architecture: ar71xx-generic"
    echo ""

    echo -e "${GREEN}✓ Development libraries:${NC}"
    echo "  • libusb-1.0 (headers + static + shared)"
    echo "  • Standard C library"
    echo ""

    echo -e "${BLUE}📖 Next Steps:${NC}"
    echo ""
    echo "1. Build your MIPS binary:"
    echo "   ./scripts/build-mips-cgo.sh bin/monitor-mips cmd/monitor/main.go"
    echo ""
    echo "2. Deploy and test on Antminer:"
    echo "   ./scripts/deploy-monitor-usb.sh"
    echo ""
    echo "3. For manual builds, use:"
    echo "   export CC=\"$SDK_ROOT/staging_dir/toolchain-mips_24kc_gcc-7.5.0_musl/bin/mips-openwrt-linux-musl-gcc\""
    echo "   export CGO_ENABLED=1 GOOS=linux GOARCH=mips GOMIPS=softfloat"
    echo "   export CGO_LDFLAGS=\"-L$SDK_ROOT/staging_dir/target-mips_24kc_musl/usr/lib -lusb-1.0\""
    echo ""

    echo -e "${YELLOW}💡 Tips:${NC}"
    echo "  • The SDK is ~250MB and includes full source for packages"
    echo "  • Rebuild libusb if needed: cd $SDK_NAME && make package/libusb/compile"
    echo "  • See build logs in: /tmp/libusb-build.log"
    echo ""
}

# Main execution
main() {
    clear
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║                                                            ║"
    echo "║     Hasher POC - Toolchain Setup                   ║"
    echo "║     OpenWrt SDK for MIPS Cross-Compilation                ║"
    echo "║                                                            ║"
    echo "╚════════════════════════════════════════════════════════════╝"

    print_info "Project root: $PROJECT_ROOT"
    print_info "SDK will be installed to: $SDK_ROOT"
    echo ""

    read -p "Continue with setup? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Setup cancelled"
        exit 0
    fi

    check_dependencies
    download_sdk
    extract_sdk
    update_feeds
    compile_libusb
    verify_toolchain
    create_build_script
    print_summary

    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}✓ Toolchain setup completed successfully!${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# Run main function
main "$@"
