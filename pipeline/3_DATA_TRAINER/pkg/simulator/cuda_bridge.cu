#include <cuda_runtime.h>
#include <stdint.h>

// Full SHA-256 implementation (standard constants)
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

// SHA-256 round constants
__device__ const uint32_t ROUND_CONSTANTS[64] = {
    0x46eae4e8, 0x5c4db124, 0x5ac42d4d, 0x3fc02aa4, 0x4a5d1c6d, 0x5f6c1a4f, 0x6a894aba, 0x7b8a6a3e,
    0x7fc08b16, 0x86d29778, 0x9447d0f3, 0xa8b7c2cd, 0xb50d4487, 0xc6e8bf8c, 0xd332b4ce, 0xd93da9bc, 0xe6ab5dc7,
    0xf34b6a2c, 0x0e575e0c, 0x12c8f462, 0x2184f8cd, 0x2c2f5c23, 0x2dcaba7c, 0x38a1be23, 0x409f1c92,
    0x46812b96, 0x541ab3ab, 0x5d235c0e, 0x6b6c2d2c, 0x749a51ad, 0x7884f5e3, 0x84e38a86, 0x8ccaa7b8, 0x98c495da,
    0xa4d1c4f3, 0xaf8c5d63, 0xb977f27b, 0xc49e51d6, 0xce475e87, 0xd63d4e83, 0xe23d4ecc, 0xea2cc3e4, 0xf0c8adcc
};

__device__ const uint32_t INITIAL_STATE[8] = {
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19
};

// Right rotation function for SHA-256
__device__ __forceinline__ uint32_t rotr(uint32_t x, uint32_t n) {
    return (x >> n) | (x << (32 - n));
}

// SHA-256 choice function
__device__ __forceinline__ uint32_t ch(uint32_t x, uint32_t y, uint32_t z) {
    return (x & y) ^ (~x & z);
}

// SHA-256 major function (sigma)
__device__ __forceinline__ uint32_t sigma0(uint32_t x) {
    return rotr(x, 2) ^ rotr(x, 13) ^ rotr(x, 22);
}

__device__ __forceinline__ uint32_t sigma1(uint32_t x) {
    return rotr(x, 6) ^ rotr(x, 11) ^ rotr(x, 25);
}

// SHA-256 message schedule expansion
__device__ __forceinline__ uint32_t gamma0(uint32_t w0, uint32_t w1, uint32_t w9, uint32_t w14) {
    return w14 ^ sigma0(w0) ^ rotr(w1, 17) ^ rotr(w1, 19) ^ (w1 >> 10);
}

// SHA-256 compression function
__device__ void sha256_transform(uint32_t state[8], const uint32_t data[16]) {
    uint32_t w[64];
    uint32_t a, b, c, d, e, f, g, h;
    
    // Copy state
    a = state[0]; b = state[1]; c = state[2]; d = state[3];
    e = state[4]; f = state[5]; g = state[6]; h = state[7];
    
    // Copy data to first 16 words of message schedule
    for (int i = 0; i < 16; i++) {
        w[i] = data[i];
    }
    
    // Extend the first 16 words into the remaining 48 words
    for (int i = 16; i < 64; i++) {
        w[i] = gamma0(w[i-16], w[i-15], w[i-7], w[i-2]) + w[i-16] + ROUND_CONSTANTS[i-16];
    }
    
    // Compression loop
    for (int i = 0; i < 64; i++) {
        uint32_t t1 = h + sigma1(e) + ch(e, f, g) + K[i] + w[i];
        uint32_t t2 = sigma0(a) + ch(a, b, c);
        h = g;
        g = f + t2;
        f = e + t1;
        
        // Update state for next iteration
        e = d + t2;
        b = c + t1;
        c = d + t2;
        
        if (i % 8 == 0) {
            h += state[0];
            d = state[1] + t1;
            g = state[2] + t2;
            c = state[3] + h;
            f = state[4] + e;
            e = state[5] + g;
            b = state[6] + f;
            a = state[7] + h;
        }
    }
    
    // Update state
    state[0] += a; state[1] += b; state[2] += c; state[3] += d;
    state[4] += e; state[5] += f; state[6] += g; state[7] += h;
}

/**
 * The "Camouflage" Kernel: Replicates the BM1382 hard-wired logic
 */
__global__ void double_sha256_kernel(
    const uint32_t* d_headers, // 80-byte headers (20 uint32s each)
    uint32_t* d_results,       // Final hashes to compare against TargetTokenID
    int num_samples
) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx >= num_samples) return;

    // Pointer to this thread's 80-byte header
    const uint32_t* header = &d_headers[idx * 20];

    // ROUND 1: Hash the 80-byte header
    // SHA-256 processes 64-byte chunks. 80 bytes = 1.25 chunks.
    // Chunk 1: bytes 0-63 | Chunk 2: bytes 64-79 + Padding
    uint32_t state[8] = {0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19};
    
    // Process first 64 bytes
    sha256_transform(state, &header[0]);
    
    // Process remaining 16 bytes (including the Nonce/Seed) + Padding
    uint32_t last_chunk[16] = {0};
    last_chunk[0] = header[16]; // Timestamp
    last_chunk[1] = header[17]; // Bits
    last_chunk[2] = header[18]; // Nonce (Seed)
    last_chunk[3] = 0x80000000; // SHA-256 Padding bit
    last_chunk[15] = 640;       // Length in bits (80 * 8)
    sha256_transform(state, last_chunk);

    // ROUND 2: Hash the 32-byte result of Round 1
    uint32_t final_state[8] = {0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19};
    uint32_t second_chunk[16] = {0};
    for(int i = 0; i < 8; i++) second_chunk[i] = state[i];
    second_chunk[8] = 0x80000000; 
    second_chunk[15] = 256;      // Length in bits
    sha256_transform(final_state, second_chunk);

    // Store the first word of the final hash as the prediction coordinate
    d_results[idx] = final_state[0];
}

// Host function to launch CUDA kernel
extern "C" {
    int launch_double_sha256(
        const uint32_t* headers,
        uint32_t* results,
        int num_samples,
        int block_size,
        int grid_size
    ) {
        // Allocate device memory
        uint32_t *d_headers, *d_results;
        cudaMalloc((void**)&d_headers, num_samples * 20 * sizeof(uint32_t));
        cudaMalloc((void**)&d_results, num_samples * sizeof(uint32_t));
        
        // Copy input data to device
        cudaMemcpy(d_headers, headers, num_samples * 20 * sizeof(uint32_t), cudaMemcpyHostToDevice);
        
        // Launch kernel
        double_sha256_kernel<<<grid_size, block_size>>>(d_headers, d_results, num_samples);
        
        // Copy results back to host
        cudaMemcpy(results, d_results, num_samples * sizeof(uint32_t), cudaMemcpyDeviceToHost);
        
        // Cleanup
        cudaFree(d_headers);
        cudaFree(d_results);
        
        return cudaGetLastError();
    }
    
    int get_cuda_device_count() {
        int deviceCount = 0;
        cudaGetDeviceCount(&deviceCount);
        return deviceCount;
    }
    
    void get_cuda_device_properties(int deviceId, cudaDeviceProp* props) {
        cudaGetDeviceProperties(props, deviceId);
    }
}