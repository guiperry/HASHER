#!/bin/bash
# Go-Spacy Automatic Installation Script
# Supports Linux, macOS, and Windows (via WSL/Git Bash)

set -e

PROJECT_NAME="go-spacy"
VERSION="1.0.0"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

fatal() {
    log_error "$1"
    exit 1
}

# Detect platform
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)

    case "$os" in
        linux*)
            PLATFORM_OS="linux"
            SHARED_EXT="so"
            ;;
        darwin*)
            PLATFORM_OS="darwin"
            SHARED_EXT="dylib"
            ;;
        mingw*|msys*|cygwin*)
            PLATFORM_OS="windows"
            SHARED_EXT="dll"
            ;;
        *)
            fatal "Unsupported operating system: $os"
            ;;
    esac

    case "$arch" in
        x86_64|amd64)
            PLATFORM_ARCH="amd64"
            ;;
        aarch64|arm64)
            PLATFORM_ARCH="arm64"
            ;;
        armv7l)
            PLATFORM_ARCH="arm"
            ;;
        *)
            fatal "Unsupported architecture: $arch"
            ;;
    esac

    log_info "Platform detected: $PLATFORM_OS/$PLATFORM_ARCH"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Detect package manager
detect_package_manager() {
    if command_exists apt-get; then
        PKG_MANAGER="apt"
    elif command_exists yum; then
        PKG_MANAGER="yum"
    elif command_exists dnf; then
        PKG_MANAGER="dnf"
    elif command_exists pacman; then
        PKG_MANAGER="pacman"
    elif command_exists brew; then
        PKG_MANAGER="brew"
    elif command_exists chocolatey || command_exists choco; then
        PKG_MANAGER="choco"
    else
        PKG_MANAGER="manual"
        log_warning "No package manager detected. Manual installation required."
    fi

    if [ "$PKG_MANAGER" != "manual" ]; then
        log_info "Package manager: $PKG_MANAGER"
    fi
}

# Install system dependencies
install_system_deps() {
    log_info "Installing system dependencies..."

    case "$PKG_MANAGER" in
        apt)
            sudo apt-get update
            sudo apt-get install -y build-essential python3 python3-dev python3-pip pkg-config
            ;;
        yum)
            sudo yum groupinstall -y "Development Tools"
            sudo yum install -y python3 python3-devel python3-pip pkgconfig
            ;;
        dnf)
            sudo dnf groupinstall -y "Development Tools"
            sudo dnf install -y python3 python3-devel python3-pip pkgconfig
            ;;
        pacman)
            sudo pacman -S --needed base-devel python python-pip pkgconf
            ;;
        brew)
            if ! command_exists gcc && ! command_exists clang; then
                xcode-select --install 2>/dev/null || true
            fi
            brew install python3 pkg-config
            ;;
        choco)
            choco install -y python3 pkgconfiglite
            choco install -y mingw
            ;;
        manual)
            log_warning "Please install the following manually:"
            log_warning "- C++ compiler (gcc/g++ or clang/clang++)"
            log_warning "- Python 3.7+ with development headers"
            log_warning "- pkg-config"
            log_warning "- pip (Python package manager)"
            ;;
    esac
}

# Verify system dependencies
verify_system_deps() {
    log_info "Verifying system dependencies..."

    # Check C++ compiler
    if command_exists g++; then
        CXX_COMPILER="g++"
    elif command_exists clang++; then
        CXX_COMPILER="clang++"
    elif command_exists cl; then
        CXX_COMPILER="cl"  # MSVC
    else
        fatal "No C++ compiler found. Please install gcc, clang, or MSVC."
    fi
    log_success "C++ compiler: $CXX_COMPILER"

    # Check Python
    if command_exists python3; then
        PYTHON_CMD="python3"
    elif command_exists python; then
        # Verify it's Python 3
        if python --version 2>&1 | grep -q "Python 3"; then
            PYTHON_CMD="python"
        else
            fatal "Python 3 is required, but found Python 2"
        fi
    else
        fatal "Python 3 is not installed"
    fi

    local python_version=$($PYTHON_CMD --version 2>&1)
    log_success "Python: $python_version"

    # Check pkg-config
    if command_exists pkg-config; then
        PKG_CONFIG_CMD="pkg-config"
        log_success "pkg-config available"
    else
        log_warning "pkg-config not found, falling back to python-config"
        PKG_CONFIG_CMD="fallback"
    fi
}

# Install Python dependencies
install_python_deps() {
    log_info "Installing Python dependencies..."

    # Upgrade pip and install setuptools/wheel for Python 3.12+
    $PYTHON_CMD -m pip install --upgrade pip setuptools wheel

    # Install spacy
    # Use --user flag only if not in virtual env or CI
    if [ -n "$GITHUB_ACTIONS" ] || [ -n "$VIRTUAL_ENV" ] || [ -n "$CI" ]; then
        $PYTHON_CMD -m pip install spacy
    else
        $PYTHON_CMD -m pip install --user spacy
    fi

    # Download English model
    log_info "Downloading English language model..."
    $PYTHON_CMD -m spacy download en_core_web_sm

    # Verify installation
    $PYTHON_CMD -c "import spacy; nlp = spacy.load('en_core_web_sm'); print('✅ Spacy installation verified')"
    log_success "Spacy and model installed successfully"
}

# Get Python configuration
get_python_config() {
    log_info "Detecting Python configuration..."

    # On macOS, prefer python-config to avoid version mismatches
    if [ "$PLATFORM_OS" = "darwin" ]; then
        if command_exists python3-config; then
            PYTHON_CONFIG_CMD="python3-config"
            PYTHON_CFLAGS=$($PYTHON_CONFIG_CMD --cflags)
            PYTHON_LIBS=$($PYTHON_CONFIG_CMD --ldflags)
            log_success "Using $PYTHON_CONFIG_CMD for Python detection"
            return
        elif command_exists python-config; then
            PYTHON_CONFIG_CMD="python-config"
            PYTHON_CFLAGS=$($PYTHON_CONFIG_CMD --cflags)
            PYTHON_LIBS=$($PYTHON_CONFIG_CMD --ldflags)
            log_success "Using $PYTHON_CONFIG_CMD for Python detection"
            return
        fi
    fi

    # On other platforms, try pkg-config first
    if [ "$PKG_CONFIG_CMD" = "pkg-config" ] && pkg-config --exists python3; then
        PYTHON_CFLAGS=$(pkg-config --cflags python3)
        PYTHON_LIBS=$(pkg-config --libs python3)
        log_success "Using pkg-config for Python detection"
    else
        # Fallback to python-config
        if command_exists python3-config; then
            PYTHON_CONFIG_CMD="python3-config"
        elif command_exists python-config; then
            PYTHON_CONFIG_CMD="python-config"
        else
            fatal "Neither pkg-config nor python-config available"
        fi

        PYTHON_CFLAGS=$($PYTHON_CONFIG_CMD --cflags 2>/dev/null)
        # For proper linking, we need both ldflags and libs with --embed flag
        PYTHON_LIBS=$($PYTHON_CONFIG_CMD --ldflags --embed 2>/dev/null || $PYTHON_CONFIG_CMD --ldflags 2>/dev/null)
        # On some systems, we also need --libs
        PYTHON_LIBS="$PYTHON_LIBS $($PYTHON_CONFIG_CMD --libs --embed 2>/dev/null || $PYTHON_CONFIG_CMD --libs 2>/dev/null || echo '')"
        log_success "Using $PYTHON_CONFIG_CMD for Python detection"
    fi

    if [ -z "$PYTHON_CFLAGS" ]; then
        fatal "Failed to get Python compile flags"
    fi
}

# Build C++ wrapper
build_cpp_wrapper() {
    log_info "Building C++ wrapper..."

    # Create directories
    mkdir -p build lib

    # Compile object file
    local cpp_file="cpp/spacy_wrapper.cpp"
    local obj_file="build/spacy_wrapper.o"
    local lib_file="lib/libspacy_wrapper.$SHARED_EXT"

    if [ ! -f "$cpp_file" ]; then
        fatal "Source file not found: $cpp_file"
    fi

    # Compilation flags
    local compile_flags="-Wall -Wextra -fPIC -std=c++17 -Iinclude -O3 -DNDEBUG"

    # Add architecture-specific optimizations
    if [ "$PLATFORM_ARCH" = "amd64" ]; then
        compile_flags="$compile_flags -march=x86-64"
    elif [ "$PLATFORM_ARCH" = "arm64" ] && [ "$PLATFORM_OS" != "darwin" ]; then
        compile_flags="$compile_flags -march=armv8-a"
    fi

    log_info "Compiling C++ source..."
    $CXX_COMPILER $compile_flags $PYTHON_CFLAGS -c "$cpp_file" -o "$obj_file"

    # Linking flags
    local link_flags="-shared"

    # Platform-specific linking
    case "$PLATFORM_OS" in
        darwin)
            link_flags="$link_flags -install_name @rpath/libspacy_wrapper.dylib"
            # Add explicit Python library if not already present
            if ! echo "$PYTHON_LIBS" | grep -q "\-lpython"; then
                local python_version=$($PYTHON_CMD -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')")
                if [ -n "$python_version" ]; then
                    PYTHON_LIBS="$PYTHON_LIBS -lpython$python_version"
                fi
            fi
            ;;
        windows)
            link_flags="$link_flags -Wl,--out-implib,lib/libspacy_wrapper.dll.a"
            ;;
    esac

    log_info "Linking shared library..."
    $CXX_COMPILER $link_flags -o "$lib_file" "$obj_file" $PYTHON_LIBS

    # Fix install name on macOS
    if [ "$PLATFORM_OS" = "darwin" ] && command_exists install_name_tool; then
        local abs_path=$(pwd)/$lib_file
        install_name_tool -id "$abs_path" "$lib_file" 2>/dev/null || true
    fi

    # Verify build
    if [ ! -f "$lib_file" ]; then
        fatal "Failed to build shared library"
    fi

    local lib_size=$(stat -f%z "$lib_file" 2>/dev/null || stat -c%s "$lib_file" 2>/dev/null || echo "0")
    if [ "$lib_size" -lt 1000 ]; then
        fatal "Library file too small ($lib_size bytes), build failed"
    fi

    log_success "Library built successfully: $lib_file ($(( lib_size / 1024 )) KB)"
}

# Test build
test_build() {
    log_info "Testing build..."

    # Set library path
    case "$PLATFORM_OS" in
        darwin)
            export DYLD_LIBRARY_PATH="$(pwd)/lib:$DYLD_LIBRARY_PATH"
            ;;
        linux)
            export LD_LIBRARY_PATH="$(pwd)/lib:$LD_LIBRARY_PATH"
            ;;
        windows)
            export PATH="$(pwd)/lib:$PATH"
            ;;
    esac

    # Test Python integration
    $PYTHON_CMD -c "
import spacy
try:
    nlp = spacy.load('en_core_web_sm')
    doc = nlp('Hello world')
    print('✅ Python integration test passed')
except Exception as e:
    print(f'❌ Python integration test failed: {e}')
    exit(1)
"

    log_success "Build test completed successfully"
}

# Main installation function
main() {
    echo "🚀 Go-Spacy Automatic Installation"
    echo "=================================="
    echo "Version: $VERSION"
    echo "Platform: $(uname -s) $(uname -m)"
    echo ""

    detect_platform
    detect_package_manager

    # Check if we need to install dependencies
    if ! command_exists "$CXX_COMPILER" 2>/dev/null || ! command_exists python3 && ! command_exists python; then
        log_info "Installing system dependencies..."
        install_system_deps
    fi

    verify_system_deps

    # Check if Python dependencies are installed
    if ! $PYTHON_CMD -c "import spacy" 2>/dev/null; then
        install_python_deps
    else
        log_success "Spacy already installed"
    fi

    get_python_config
    build_cpp_wrapper
    test_build

    echo ""
    log_success "🎉 Installation completed successfully!"
    echo ""
    echo "You can now use go-spacy in your Go projects:"
    echo "  go get github.com/am-sokolov/go-spacy"
    echo ""
    echo "Library path for runtime:"
    case "$PLATFORM_OS" in
        darwin)
            echo "  export DYLD_LIBRARY_PATH=\$DYLD_LIBRARY_PATH:$(pwd)/lib"
            ;;
        linux)
            echo "  export LD_LIBRARY_PATH=\$LD_LIBRARY_PATH:$(pwd)/lib"
            ;;
        windows)
            echo "  export PATH=\$PATH:$(pwd)/lib"
            ;;
    esac
}

# Run main function
main "$@"