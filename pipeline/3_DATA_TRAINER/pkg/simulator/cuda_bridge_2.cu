#include <cuda_runtime.h>
#include <stdint.h>

// Forward declaration of the kernel we wrote earlier
__global__ void double_sha256_kernel(const uint32_t* d_headers, uint32_t* d_results, uint32_t target, int num_samples);

extern "C" {
    // This is the function cgo will call
    uint32_t run_cuda_search(const uint8_t* h_header, uint32_t target) {
        uint32_t *d_header, *d_result;
        uint32_t h_result = 0;

        // 1. Allocate GPU memory (In production, use a persistent pool for speed)
        cudaMalloc(&d_header, 80);
        cudaMalloc(&d_result, sizeof(uint32_t));
        cudaMemset(d_result, 0, sizeof(uint32_t));

        // 2. Copy the 80-byte "Camouflaged" header to GPU
        cudaMemcpy(d_header, h_header, 80, cudaMemcpyHostToDevice);

        // 3. Launch the 21-pass search (Simulating 1 candidate for this uBPF call)
        // In a group search, you'd launch a grid of nonces here.
        double_sha256_kernel<<<1, 1>>>(d_header, d_result, target, 1);

        // 4. Copy result back
        cudaMemcpy(&h_result, d_result, sizeof(uint32_t), cudaMemcpyDeviceToHost);

        // 5. Cleanup
        cudaFree(d_header);
        cudaFree(d_result);

        return h_result; // Returns the Nonce if found, or 0
    }
}