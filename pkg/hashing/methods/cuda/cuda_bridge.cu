// Compatibility macros for GCC 11+ with older CUDA versions
// Must be defined before including any system headers
#if defined(__GNUC__) && __GNUC__ >= 11
#include <sys/cdefs.h>
#undef __attr_dealloc
#define __attr_dealloc(f, i)
#undef __attr_dealloc_free
#define __attr_dealloc_free
#undef __attr_dealloc_fclose
#define __attr_dealloc_fclose
#endif

#include <cuda_runtime.h>
#include <stdint.h>

// Standard SHA-256 Constants
__device__ const uint32_t K[64] = {
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1,
    0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116,
    0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f,
    0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
};

__device__ __forceinline__ uint32_t rotr(uint32_t x, uint32_t n) {
    return (x >> n) | (x << (32 - n));
}

__device__ __forceinline__ uint32_t sigma0(uint32_t x) {
    return rotr(x, 7) ^ rotr(x, 18) ^ (x >> 3);
}

__device__ __forceinline__ uint32_t sigma1(uint32_t x) {
    return rotr(x, 17) ^ rotr(x, 19) ^ (x >> 10);
}

__device__ __forceinline__ uint32_t Sigma0(uint32_t x) {
    return rotr(x, 2) ^ rotr(x, 13) ^ rotr(x, 22);
}

__device__ __forceinline__ uint32_t Sigma1(uint32_t x) {
    return rotr(x, 6) ^ rotr(x, 11) ^ rotr(x, 25);
}

__device__ __forceinline__ uint32_t Ch(uint32_t x, uint32_t y, uint32_t z) {
    return (x & y) ^ (~x & z);
}

__device__ __forceinline__ uint32_t Maj(uint32_t x, uint32_t y, uint32_t z) {
    return (x & y) ^ (x & z) ^ (y & z);
}

__device__ void sha256_transform(uint32_t state[8], const uint32_t data[16]) {
    uint32_t w[64];
    for (int i = 0; i < 16; i++) w[i] = data[i];
    for (int i = 16; i < 64; i++) w[i] = sigma1(w[i-2]) + w[i-7] + sigma0(w[i-15]) + w[i-16];

    uint32_t a = state[0], b = state[1], c = state[2], d = state[3];
    uint32_t e = state[4], f = state[5], g = state[6], h = state[7];

    for (int i = 0; i < 64; i++) {
        uint32_t t1 = h + Sigma1(e) + Ch(e, f, g) + K[i] + w[i];
        uint32_t t2 = Sigma0(a) + Maj(a, b, c);
        h = g; g = f; f = e; e = d + t1;
        d = c; c = b; b = a; a = t1 + t2;
    }

    state[0] += a; state[1] += b; state[2] += c; state[3] += d;
    state[4] += e; state[5] += f; state[6] += g; state[7] += h;
}

__global__ void double_sha256_full_kernel(const uint32_t* d_headers, uint32_t* d_results, int num_samples) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx >= num_samples) return;

    const uint32_t* header = &d_headers[idx * 20];

    // ROUND 1: Hash the 80-byte header
    uint32_t state[8] = {0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19};
    
    // Process first block (64 bytes)
    sha256_transform(state, &header[0]);
    
    // Process second block (remaining 16 bytes + padding)
    uint32_t second_block[16] = {0};
    for (int i = 0; i < 4; i++) second_block[i] = header[16 + i];
    second_block[4] = 0x80000000;
    second_block[15] = 640; // 80 bytes * 8 bits/byte
    sha256_transform(state, second_block);

    // ROUND 2: Hash the 32-byte result of Round 1
    uint32_t final_state[8] = {0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19};
    uint32_t final_block[16] = {0};
    for (int i = 0; i < 8; i++) final_block[i] = state[i];
    final_block[8] = 0x80000000;
    final_block[15] = 256; // 32 bytes * 8 bits/byte
    sha256_transform(final_state, final_block);

    for (int i = 0; i < 8; i++) d_results[idx * 8 + i] = final_state[i];
}

extern "C" {
    int launch_double_sha256_full(const uint32_t* headers, uint32_t* results, int num_samples, int block_size, int grid_size) {
        uint32_t *d_headers, *d_results;
        cudaError_t err;

        err = cudaMalloc((void**)&d_headers, num_samples * 20 * sizeof(uint32_t));
        if (err != cudaSuccess) return (int)err;

        err = cudaMalloc((void**)&d_results, num_samples * 8 * sizeof(uint32_t));
        if (err != cudaSuccess) {
            cudaFree(d_headers);
            return (int)err;
        }
        
        cudaMemcpy(d_headers, headers, num_samples * 20 * sizeof(uint32_t), cudaMemcpyHostToDevice);
        double_sha256_full_kernel<<<grid_size, block_size>>>(d_headers, d_results, num_samples);
        cudaMemcpy(results, d_results, num_samples * 8 * sizeof(uint32_t), cudaMemcpyDeviceToHost);
        
        cudaFree(d_headers);
        cudaFree(d_results);
        
        err = cudaGetLastError();
        return (int)err;
    }
}
