# Automatic Build System

Go-Spacy includes a comprehensive automatic build system that handles cross-platform compilation and dependency management. This system is designed to work seamlessly with `go get` and provide multiple build methods for different environments.

## Table of Contents

- [Overview](#overview)
- [Build Methods](#build-methods)
- [Platform Support](#platform-support)
- [Installation Process](#installation-process)
- [Troubleshooting](#troubleshooting)
- [Manual Build](#manual-build)
- [Development](#development)

## Overview

The automatic build system consists of several components that work together to ensure reliable package installation:

1. **`go generate` Integration** - Triggers automatic builds during development
2. **Cross-platform Build Scripts** - Shell/PowerShell scripts for different operating systems
3. **Go Build Tool** - Pure Go implementation for maximum compatibility
4. **Makefile Fallback** - Traditional build system for advanced users
5. **Docker Support** - Containerized builds for consistent environments

## Build Methods

The system tries multiple build methods in order of reliability:

### 1. Go Generate (`build.go`)
```bash
go generate github.com/am-sokolov/go-spacy
```

**Features:**
- ✅ Pure Go implementation
- ✅ Cross-platform support (Linux, macOS, Windows)
- ✅ Automatic dependency detection
- ✅ Python configuration detection (pkg-config/python-config)
- ✅ Comprehensive error reporting

**Process:**
1. Detects platform and architecture
2. Validates system dependencies (C++ compiler, Python, Spacy)
3. Configures build environment
4. Compiles C++ wrapper with proper flags
5. Links shared library with Python
6. Validates build artifacts

### 2. Platform-Specific Scripts

#### Linux/macOS (`scripts/install.sh`)
```bash
bash scripts/install.sh
```

**Features:**
- ✅ Automatic package manager detection (apt, yum, dnf, pacman, brew)
- ✅ System dependency installation
- ✅ Python environment setup
- ✅ Spacy model downloading
- ✅ Build validation and testing

#### Windows (`scripts/install.ps1`)
```powershell
powershell -ExecutionPolicy Bypass -File scripts/install.ps1
```

**Features:**
- ✅ Chocolatey integration for dependency management
- ✅ MinGW-w64/MSVC compiler support
- ✅ User-level installation (no admin required)
- ✅ Python installer automation
- ✅ Comprehensive error handling

### 3. Makefile (Advanced Users)
```bash
make lib
```

**Features:**
- ✅ Traditional Unix build system
- ✅ Optimized builds with custom flags
- ✅ Development targets (debug, release, profiling)
- ✅ Cross-compilation support

### 4. Installer Command
```bash
go run github.com/am-sokolov/go-spacy/cmd/install
```

**Features:**
- ✅ Standalone installer
- ✅ Multiple fallback methods
- ✅ Detailed troubleshooting guidance
- ✅ Project root detection

## Platform Support

### Linux

**Supported Distributions:**
- Ubuntu 18.04+ (LTS recommended)
- Debian 10+
- CentOS/RHEL 7+
- Fedora 30+
- Arch Linux
- Alpine Linux (with build tools)

**Package Managers:**
- `apt` (Ubuntu/Debian)
- `yum` (CentOS/RHEL 7)
- `dnf` (CentOS/RHEL 8+, Fedora)
- `pacman` (Arch Linux)

**Build Tools:**
- GCC 7+ or Clang 7+
- `pkg-config`
- `python3-dev` or equivalent

### macOS

**Supported Versions:**
- macOS 10.15+ (Catalina and later)
- Both Intel (x86_64) and Apple Silicon (arm64)

**Requirements:**
- Xcode Command Line Tools
- Homebrew (recommended)
- Python 3.7+ (system or Homebrew)

**Build Tools:**
- Clang (from Xcode)
- `pkg-config` (via Homebrew)

### Windows

**Supported Versions:**
- Windows 10 20H2+
- Windows 11
- Windows Server 2019+

**Build Environments:**
- MinGW-w64 (recommended for auto-install)
- Visual Studio Build Tools 2019+
- WSL2 with Linux build tools

**Package Managers:**
- Chocolatey (automatic installation)
- Manual installation fallback

## Installation Process

### Automatic Installation

The recommended way to install Go-Spacy:

```bash
go get github.com/am-sokolov/go-spacy
```

**What happens automatically:**
1. Go downloads the package source
2. `go generate` trigger activates automatic build
3. System dependencies are validated
4. C++ wrapper is compiled and linked
5. Python integration is tested
6. Library is ready for use

### Manual Trigger

If automatic installation fails:

```bash
# Method 1: Go generate
go generate github.com/am-sokolov/go-spacy

# Method 2: Platform script
cd $GOPATH/src/github.com/am-sokolov/go-spacy
bash scripts/install.sh  # Linux/macOS
# or
powershell scripts/install.ps1  # Windows

# Method 3: Make
make lib

# Method 4: Pure Go installer
go run github.com/am-sokolov/go-spacy/cmd/install
```

### Docker Installation

For containerized environments:

```bash
# Using included Dockerfile
docker build -t go-spacy .
docker run -it go-spacy

# Using Docker Compose
docker-compose up --build
```

## Troubleshooting

### Common Issues

#### 1. Missing C++ Compiler

**Error:** `C++ compiler not found`

**Solution:**
```bash
# Ubuntu/Debian
sudo apt install build-essential

# CentOS/RHEL
sudo yum groupinstall "Development Tools"

# macOS
xcode-select --install

# Windows
# Install via script or manually install MinGW-w64/Visual Studio
```

#### 2. Python Not Found

**Error:** `Python not found` or `Python 3 is required`

**Solution:**
```bash
# Linux
sudo apt install python3 python3-dev python3-pip

# macOS
brew install python3

# Windows
# Script will auto-install or download from python.org
```

#### 3. Spacy Not Installed

**Error:** `Spacy not installed`

**Solution:**
```bash
pip3 install spacy
python3 -m spacy download en_core_web_sm
```

#### 4. pkg-config Issues

**Error:** `pkg-config python3 not found`

**Solution:**
```bash
# Linux
sudo apt install pkg-config

# macOS
brew install pkg-config

# Windows
choco install pkgconfiglite
```

#### 5. Library Not Found at Runtime

**Error:** `Library not loaded` or `shared library not found`

**Solution:**
```bash
# Linux
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/path/to/go-spacy/lib

# macOS
export DYLD_LIBRARY_PATH=$DYLD_LIBRARY_PATH:/path/to/go-spacy/lib

# Windows
set PATH=%PATH%;C:\path\to\go-spacy\lib
```

### Environment-Specific Issues

#### Python Version Mismatches

If you have multiple Python versions:

```bash
# Force specific Python version
export PYTHON_VERSION=3.9
./scripts/install.sh

# Or use pyenv/conda
pyenv local 3.9.7
conda activate myenv
```

#### Cross-compilation

For cross-platform builds:

```bash
# Linux to Windows
GOOS=windows GOARCH=amd64 make lib-windows

# macOS Universal Binary
make lib-universal
```

### Debug Mode

Enable verbose output:

```bash
# Go build
VERBOSE=1 go run build.go

# Shell script
DEBUG=1 bash scripts/install.sh

# Make
make V=1 lib
```

## Manual Build

For complete manual control:

### Step 1: Install Dependencies

```bash
# System packages (example for Ubuntu)
sudo apt update
sudo apt install -y build-essential python3-dev pkg-config

# Python packages
pip3 install --user spacy
python3 -m spacy download en_core_web_sm
```

### Step 2: Configure Build

```bash
# Get Python configuration
PYTHON_CFLAGS=$(pkg-config --cflags python3)
PYTHON_LIBS=$(pkg-config --libs python3)

# Or fallback
PYTHON_CFLAGS=$(python3-config --cflags)
PYTHON_LIBS=$(python3-config --ldflags)
```

### Step 3: Compile

```bash
# Create directories
mkdir -p build lib

# Compile C++ wrapper
g++ -Wall -Wextra -fPIC -std=c++17 -Iinclude -O3 -DNDEBUG \
    $PYTHON_CFLAGS \
    -c cpp/spacy_wrapper.cpp -o build/spacy_wrapper.o

# Link shared library
g++ -shared -o lib/libspacy_wrapper.so \
    build/spacy_wrapper.o \
    $PYTHON_LIBS
```

### Step 4: Test

```bash
# Set library path
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$(pwd)/lib

# Test Python integration
python3 -c "import spacy; nlp = spacy.load('en_core_web_sm'); print('OK')"

# Test Go integration
go test -v -run TestInitialization
```

## Development

### Adding New Build Methods

To add support for new build methods:

1. **Update `build.go`** - Add new platform detection
2. **Create platform script** - Add to `scripts/` directory
3. **Update Makefile** - Add new targets
4. **Test thoroughly** - Validate on target platform
5. **Update documentation** - Document new method

### Build Configuration

Key configuration files:

- **`build.go`** - Main build logic and cross-platform support
- **`scripts/install.sh`** - Unix installation script
- **`scripts/install.ps1`** - Windows PowerShell script
- **`Makefile`** - Traditional build system
- **`Dockerfile`** - Container build definition
- **`docker-compose.yml`** - Container orchestration

### Testing Build System

```bash
# Test all build methods
make test-build-methods

# Test specific platform
make test-linux
make test-macos
make test-windows

# Test in Docker
make test-docker

# Clean and rebuild
make clean-all && make test-full
```

### Contributing

When contributing to the build system:

1. **Test on multiple platforms** - Ensure compatibility
2. **Add error handling** - Provide clear error messages
3. **Update documentation** - Keep this file current
4. **Add tests** - Verify functionality
5. **Consider edge cases** - Handle unusual environments

## Performance Notes

### Build Times

Typical build times on different systems:

- **Linux (GitHub Actions)**: 30-60 seconds
- **macOS (GitHub Actions)**: 45-90 seconds
- **Windows (GitHub Actions)**: 2-5 minutes
- **Local Development**: 10-30 seconds (after first build)

### Optimization

The build system includes several optimizations:

- **Incremental builds** - Only rebuild when sources change
- **Parallel compilation** - Use multiple CPU cores where possible
- **Caching** - Reuse dependencies and intermediate files
- **Minimal dependencies** - Only install what's needed

For faster development builds:

```bash
# Debug mode (faster compilation)
make BUILD_MODE=debug lib

# Skip validation steps
SKIP_VALIDATION=1 go run build.go

# Use cached dependencies
make lib-fast
```

---

For more information, see:
- [Installation Guide](INSTALLATION.md)
- [CI/CD Documentation](CICD_DEPLOYMENT.md)
- [API Reference](API_REFERENCE.md)
- [Project README](../README.md)