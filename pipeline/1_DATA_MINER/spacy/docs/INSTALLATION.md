# Installation and Setup Guide

This guide provides comprehensive instructions for setting up the Go-Spacy natural language processing library on various operating systems and environments.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Detailed Installation](#detailed-installation)
  - [System Dependencies](#system-dependencies)
  - [Python and Spacy Setup](#python-and-spacy-setup)
  - [Go Environment Setup](#go-environment-setup)
  - [Building the C++ Bridge](#building-the-c-bridge)
- [Platform-Specific Instructions](#platform-specific-instructions)
  - [Ubuntu/Debian](#ubuntudebian)
  - [macOS](#macos)
  - [Windows](#windows)
  - [Docker](#docker)
- [Verification](#verification)
- [Common Issues](#common-issues)
- [Advanced Configuration](#advanced-configuration)

## Prerequisites

Before installing Go-Spacy, ensure your system meets these requirements:

### Hardware Requirements
- **RAM**: Minimum 2GB, recommended 4GB+ (8GB+ for transformer models)
- **Storage**: 1-10GB depending on models installed
- **CPU**: x86_64 or arm64 architecture

### Software Requirements
- **Go**: Version 1.16 or higher
- **Python**: Version 3.7-3.11 (3.9+ recommended)
- **C++ Compiler**: GCC 7+ or Clang 7+
- **Make**: GNU Make or compatible
- **pkg-config**: For Python detection (essential for portability)
- **Git**: For cloning repository

## Quick Start

For users who want to get started immediately:

```bash
# 1. Install Python dependencies
pip install spacy
python -m spacy download en_core_web_sm

# 2. Clone and build the project
git clone https://github.com/am-sokolov/go-spacy.git
cd go-spacy
make clean && make

# 3. Test the installation
go test -v -run TestBasicFunctionality
```

If this works without errors, you're ready to use Go-Spacy! Skip to the [Verification](#verification) section.

## Detailed Installation

### System Dependencies

#### Ubuntu/Debian
```bash
# Update package list
sudo apt update

# Install build essentials and Python development headers
sudo apt install -y build-essential python3-dev python3-pip pkg-config

# Install Go (if not already installed)
wget https://golang.org/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

#### macOS
```bash
# Install Xcode command line tools
xcode-select --install

# Install Homebrew (if not already installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install dependencies including pkg-config for Python detection
brew install python3 go make pkg-config

# Ensure Python development headers are available
# (Usually included with Python3 from Homebrew)
```

#### Windows
For Windows users, we recommend using WSL2 (Windows Subsystem for Linux) for the best experience:

```powershell
# Enable WSL2 (run as Administrator in PowerShell)
wsl --install

# After restart, install Ubuntu in WSL2
wsl --install -d Ubuntu

# Follow Ubuntu instructions inside WSL2
```

Alternatively, you can use native Windows with proper toolchain:
- Install Go from https://golang.org/dl/
- Install Python from https://python.org/downloads/
- Install Microsoft C++ Build Tools or Visual Studio
- Install Git for Windows

### Python and Spacy Setup

#### 1. Create Virtual Environment (Recommended)
```bash
# Create virtual environment
python3 -m venv spacy-env
source spacy-env/bin/activate  # On Windows: spacy-env\Scripts\activate

# Upgrade pip
pip install --upgrade pip
```

#### 2. Install Spacy
```bash
# Install Spacy
pip install spacy

# Verify installation
python -c "import spacy; print('Spacy version:', spacy.__version__)"
```

#### 3. Download Language Models

**English Models:**
```bash
# Small model (13MB, no word vectors)
python -m spacy download en_core_web_sm

# Medium model (40MB, with word vectors) - Recommended
python -m spacy download en_core_web_md

# Large model (560MB, with word vectors)
python -m spacy download en_core_web_lg

# Transformer model (440MB, BERT-based)
python -m spacy download en_core_web_trf
```

**Other Languages:**
```bash
# German
python -m spacy download de_core_news_sm

# French
python -m spacy download fr_core_news_sm

# Spanish
python -m spacy download es_core_news_sm

# Italian
python -m spacy download it_core_news_sm

# Portuguese
python -m spacy download pt_core_news_sm

# Chinese
python -m spacy download zh_core_web_sm

# Japanese
python -m spacy download ja_core_news_sm
```

#### 4. Verify Models
```bash
# List installed models
python -m spacy info

# Test specific model
python -c "import spacy; nlp = spacy.load('en_core_web_sm'); print('Model loaded successfully')"
```

### Go Environment Setup

#### 1. Configure Go Environment
```bash
# Set Go environment variables (add to ~/.bashrc or ~/.zshrc)
export GOPATH=$HOME/go
export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin

# Verify Go installation
go version

# Enable CGO (required for this project)
export CGO_ENABLED=1
```

#### 2. Install Go Dependencies
The project uses CGO to interface with C++, so ensure CGO is properly configured:

```bash
# Test CGO
echo 'package main
import "C"
func main() {}' > cgo_test.go
go run cgo_test.go
rm cgo_test.go
```

### Building the C++ Bridge

#### 1. Clone Repository
```bash
# Clone the repository
git clone https://github.com/am-sokolov/go-spacy.git
cd go-spacy
```

#### 2. Build C++ Wrapper
```bash
# Clean any previous builds
make clean

# Build the C++ wrapper and shared library
make

# Verify build output
ls -la lib/
# Should show libspacy_wrapper.so (Linux) or libspacy_wrapper.dylib (macOS)
```

#### 3. Configure Library Path
```bash
# Add library path to environment (Linux)
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$(pwd)/lib

# Add library path to environment (macOS)
export DYLD_LIBRARY_PATH=$DYLD_LIBRARY_PATH:$(pwd)/lib

# Make permanent by adding to ~/.bashrc or ~/.zshrc
echo "export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$(pwd)/lib" >> ~/.bashrc
```

## Platform-Specific Instructions

### Ubuntu/Debian

Complete setup script for Ubuntu/Debian:

```bash
#!/bin/bash
set -e

echo "Installing Go-Spacy on Ubuntu/Debian..."

# Install system dependencies
sudo apt update
sudo apt install -y build-essential python3-dev python3-pip pkg-config git make

# Install Python dependencies
pip3 install --user spacy
python3 -m spacy download en_core_web_sm

# Install Go if not present
if ! command -v go &> /dev/null; then
    wget https://golang.org/dl/go1.21.0.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
fi

# Clone and build
git clone https://github.com/am-sokolov/go-spacy.git
cd go-spacy
make clean && make

# Set library path
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$(pwd)/lib
echo "export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$(pwd)/lib" >> ~/.bashrc

# Test installation
go test -v -run TestBasicFunctionality

echo "Installation complete!"
```

### macOS

Complete setup script for macOS:

```bash
#!/bin/bash
set -e

echo "Installing Go-Spacy on macOS..."

# Install Xcode command line tools
if ! command -v make &> /dev/null; then
    xcode-select --install
    echo "Please wait for Xcode command line tools to install, then run this script again."
    exit 1
fi

# Install Homebrew if not present
if ! command -v brew &> /dev/null; then
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

# Install dependencies including pkg-config for Python detection
brew install python3 go make pkg-config

# Install Python dependencies
pip3 install spacy
python3 -m spacy download en_core_web_sm

# Clone and build
git clone https://github.com/am-sokolov/go-spacy.git
cd go-spacy
make clean && make

# Set library path
export DYLD_LIBRARY_PATH=$DYLD_LIBRARY_PATH:$(pwd)/lib
echo "export DYLD_LIBRARY_PATH=$DYLD_LIBRARY_PATH:$(pwd)/lib" >> ~/.zshrc

# Test installation
go test -v -run TestBasicFunctionality

echo "Installation complete!"
```

### Windows

For Windows with WSL2:

1. **Install WSL2:**
   ```powershell
   # Run as Administrator in PowerShell
   wsl --install
   wsl --install -d Ubuntu
   ```

2. **Setup in WSL2:**
   ```bash
   # Inside WSL2 Ubuntu terminal
   sudo apt update
   sudo apt install -y build-essential python3-dev python3-pip

   # Follow Ubuntu instructions above
   ```

For native Windows (advanced users):

1. **Install Prerequisites:**
   - Download and install Go from https://golang.org/dl/
   - Download and install Python from https://python.org/downloads/
   - Install Microsoft C++ Build Tools
   - Install Git for Windows

2. **Build Process:**
   ```cmd
   # In Command Prompt or PowerShell
   set CGO_ENABLED=1
   pip install spacy
   python -m spacy download en_core_web_sm

   # Clone repository
   git clone https://github.com/am-sokolov/go-spacy.git
   cd go-spacy

   # Build (may require adjusting Makefile for Windows)
   make
   ```

### Docker

For containerized deployment:

```dockerfile
FROM ubuntu:22.04

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    python3 \
    python3-pip \
    python3-dev \
    pkg-config \
    git \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Install Go
RUN wget https://golang.org/dl/go1.21.0.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz \
    && rm go1.21.0.linux-amd64.tar.gz

ENV PATH=$PATH:/usr/local/go/bin
ENV CGO_ENABLED=1

# Install Python dependencies
RUN pip3 install spacy \
    && python3 -m spacy download en_core_web_sm

# Copy source code
COPY . /app
WORKDIR /app

# Build
RUN make clean && make

# Set library path
ENV LD_LIBRARY_PATH=/app/lib:$LD_LIBRARY_PATH

# Test
RUN go test -v -run TestBasicFunctionality
```

Build and run:
```bash
docker build -t go-spacy .
docker run -it go-spacy bash
```

## Verification

After installation, verify everything works correctly:

### 1. Basic Test
```bash
cd go-spacy  # Your project directory
go test -v -run TestBasicFunctionality
```

Expected output:
```
=== RUN   TestBasicFunctionality
--- PASS: TestBasicFunctionality (0.50s)
PASS
```

### 2. Model Test
```bash
go test -v -run TestModelAvailability
```

### 3. Feature Tests
```bash
# Test tokenization
go test -v -run TestTokenization

# Test entity recognition
go test -v -run TestEntityRecognition

# Test advanced features (requires medium/large model)
go test -v -run TestAdvancedFeatures
```

### 4. Manual Test
Create a test file `test_installation.go`:

```go
package main

import (
    "fmt"
    "log"
    "github.com/am-sokolov/go-spacy"
)

func main() {
    nlp, err := spacy.NewNLP("en_core_web_sm")
    if err != nil {
        log.Fatal("Failed to load model:", err)
    }
    defer nlp.Close()

    text := "Hello world! This is a test."

    tokens := nlp.Tokenize(text)
    fmt.Printf("Found %d tokens\n", len(tokens))

    entities := nlp.ExtractEntities("Apple Inc. was founded by Steve Jobs.")
    fmt.Printf("Found %d entities\n", len(entities))

    fmt.Println("Installation verified successfully!")
}
```

Run the test:
```bash
go run test_installation.go
```

## Common Issues

### 1. "Model not found" Error

**Problem:** Spacy model is not installed or not accessible.

**Solution:**
```bash
# Check installed models
python -m spacy info

# Install missing model
python -m spacy download en_core_web_sm

# Verify model installation
python -c "import spacy; spacy.load('en_core_web_sm')"
```

### 2. "CGO Build Failed" Error

**Problem:** CGO compilation issues.

**Solution:**
```bash
# Ensure CGO is enabled
export CGO_ENABLED=1

# Check compiler
gcc --version
g++ --version

# Install development headers and pkg-config (Ubuntu/Debian)
sudo apt install python3-dev pkg-config

# Install Xcode tools and pkg-config (macOS)
xcode-select --install
brew install pkg-config

# Verify pkg-config can find Python
pkg-config --cflags --libs python3
```

### 3. "Shared Library Not Found" Error

**Problem:** Runtime can't find the compiled library.

**Solution:**
```bash
# Check library exists
ls -la lib/

# Set library path (Linux)
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$(pwd)/lib

# Set library path (macOS)
export DYLD_LIBRARY_PATH=$DYLD_LIBRARY_PATH:$(pwd)/lib

# Make permanent
echo 'export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$PWD/lib' >> ~/.bashrc
source ~/.bashrc
```

### 4. Python Version Issues

**Problem:** Incompatible Python version.

**Solution:**
```bash
# Check Python version
python --version
python3 --version

# Install specific Python version (Ubuntu)
sudo apt install python3.9 python3.9-dev

# Use virtual environment with specific version
python3.9 -m venv spacy-env
source spacy-env/bin/activate
pip install spacy
```

### 5. Permission Issues

**Problem:** Permission denied errors during build.

**Solution:**
```bash
# Fix file permissions
chmod +x scripts/*.sh

# Use sudo for system installs only when necessary
sudo apt install build-essential

# Use user-level pip installs
pip install --user spacy
```

### 6. "pkg-config python3 not found" Error

**Problem:** pkg-config cannot find Python3 development libraries.

**Solution:**
```bash
# Install pkg-config (Ubuntu/Debian)
sudo apt install pkg-config

# Install pkg-config (macOS)
brew install pkg-config

# Verify pkg-config installation
pkg-config --version

# Check if python3.pc file exists
pkg-config --exists python3
echo $?  # Should return 0 if found

# Alternative: check python3-config fallback
python3-config --cflags --libs

# If pkg-config still fails, install python3-dev
sudo apt install python3-dev  # Ubuntu/Debian
brew reinstall python3        # macOS

# For custom Python installations, add to PKG_CONFIG_PATH
export PKG_CONFIG_PATH=$PKG_CONFIG_PATH:/usr/local/lib/pkgconfig
```

## Advanced Configuration

### Environment Variables

Key environment variables for customization:

```bash
# Go configuration
export CGO_ENABLED=1
export GO111MODULE=on
export GOPROXY=direct

# Python configuration
export PYTHONPATH=/path/to/custom/python/modules
export SPACY_DATA=/path/to/custom/spacy/models

# Library paths
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/custom/lib/path
export PKG_CONFIG_PATH=$PKG_CONFIG_PATH:/custom/pkgconfig

# Build configuration
export CC=gcc-9
export CXX=g++-9
export CFLAGS="-O3 -march=native"
export CXXFLAGS="-O3 -march=native"
```

### Custom Model Paths

If you need to use models from non-standard locations:

```bash
# Set custom model directory
export SPACY_DATA=/path/to/custom/models

# Link models to standard location
ln -s /path/to/custom/model ~/.local/lib/python3.9/site-packages/en_core_web_sm
```

### Performance Optimization

For production deployments:

```bash
# Optimize for performance
export CFLAGS="-O3 -march=native -DNDEBUG"
export CXXFLAGS="-O3 -march=native -DNDEBUG"

# Rebuild with optimizations
make clean && make

# Test performance improvements
go test -bench=. -benchmem
```

### Memory Configuration

For memory-constrained environments:

```bash
# Limit Go memory usage
export GOMEMLIMIT=1GB

# Use smaller models
python -m spacy download en_core_web_sm  # Instead of lg
```

### Debugging Setup

For development and debugging:

```bash
# Enable debug symbols
export CFLAGS="-g -O0"
export CXXFLAGS="-g -O0"

# Enable Go race detector
export GORACE="log_path=./race"

# Verbose build
make VERBOSE=1

# Debug tests
go test -v -race -run TestName
```

## Next Steps

After successful installation:

1. **Read the API Reference:** Check `docs/API_REFERENCE.md` for detailed API documentation
2. **Explore Examples:** Look at `examples/` directory for usage patterns
3. **Run Benchmarks:** Execute `go test -bench=.` to understand performance
4. **Multi-language Setup:** Install additional language models as needed
5. **Integration:** Integrate into your Go application following the usage examples

For additional help, see:
- [Troubleshooting Guide](TROUBLESHOOTING.md)
- [Performance Guide](PERFORMANCE.md)
- [Contributing Guide](CONTRIBUTING.md)
- [GitHub Issues](https://github.com/am-sokolov/go-spacy/issues)

## Support

If you encounter issues not covered in this guide:

1. Check the [GitHub Issues](https://github.com/am-sokolov/go-spacy/issues)
2. Search existing discussions and solutions
3. Create a new issue with:
   - Your operating system and version
   - Go version (`go version`)
   - Python version (`python --version`)
   - Spacy version (`python -c "import spacy; print(spacy.__version__)"`)
   - Complete error message
   - Steps to reproduce

The maintainers and community will help resolve your installation issues.