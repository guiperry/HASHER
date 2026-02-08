#!/bin/bash
set -e

echo "--- HASHER Local Toolchain Setup (CUDA 10.2) ---"

# 1. Build the REAL uBPF (Userspace VM)

# #1. Clone the repository
if [ ! -d "ubpf" ]; then
    git clone https://github.com/iovisor/ubpf
fi
cd ubpf

# #2. Initialize submodules (CRITICAL - required for dependencies)
git submodule update --init --recursive

# #3. Prepare system dependencies (Linux only)
./scripts/build-libbpf.sh

# #4. Build using CMake
cmake -S . -B build -DCMAKE_BUILD_TYPE=Release

# #5. Compile
cmake --build build --config Release

# #6. Install (optional - installs to system directories)
sudo cmake --build build --target install
sudo ldconfig

# 2. Compile the CUDA Bridge specifically for 10.2
# -Xcompiler -fPIC is vital for the .so to be loaded by Go
nvcc -O3 -Xcompiler -fPIC -shared \
     -o pkg/simulator/libcuda_bridge.so \
     pkg/simulator/cuda_bridge.cu

# 3. Compile the eBPF Kernel to Bytecode
# We use 'generic' target because uBPF is a virtual architecture
clang -O2 -target bpf -c pkg/simulator/neural_kernel_ubpf.c -o pkg/simulator/neural_kernel_ubpf.o
clang -O2 -target bpf -c pkg/simulator/neural_kernel.c -o pkg/simulator/neural_kernel.o

# 4. Build the Go Trainer
go build -o bin/data-trainer ./cmd/trainer/

echo "[✓] Toolchain Ready. libubpf.so and libcuda_bridge.so are linked."