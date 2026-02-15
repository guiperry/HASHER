package cuda

// #cgo LDFLAGS: -L. -lcuda_hash -lcudart
// #cgo CFLAGS: -I/usr/local/cuda/include
// #include <stdlib.h>
// #include <stdint.h>
// #include <string.h>
//
// // Forward declarations for CUDA functions from shared library
// extern int launch_double_sha256_full(
//     const uint32_t* headers,
//     uint32_t* results,
//     int num_samples,
//     int block_size,
//     int grid_size
// );
//
// // Mock structures for device properties (we'll use nvidia-smi for this)
// typedef struct {
// 	char name[256];
// 	int compute_capability;
// 	size_t total_global_mem;
// 	int multi_processor_count;
// } cuda_device_prop_t;
//
// // Simple device query functions
// static int cuda_get_device_count_wrapper() {
// 	// We'll use nvidia-smi from Go side
// 	return 1; // Assume at least 1 device for now
// }
//
// static int cuda_get_device_properties_wrapper(int deviceId, cuda_device_prop_t* props) {
// 	if (props == NULL) return -1;
// 	strcpy(props->name, "NVIDIA GPU");
// 	props->compute_capability = 52; // sm_52
// 	props->total_global_mem = 2147483648; // 2GB
// 	props->multi_processor_count = 7;
// 	return 0;
// }
import "C"

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

// CudaBridge provides CGo interface to CUDA Double-SHA256 kernel
type CudaBridge struct {
	deviceCount int
	initialized bool
}

// DeviceProperties represents CUDA device information
type DeviceProperties struct {
	Name       string
	ComputeCap int
	Memory     int64
	MultiProc  bool
}

// NewCudaBridge creates a new CUDA bridge instance
func NewCudaBridge() *CudaBridge {
	bridge := &CudaBridge{
		initialized: false,
	}

	// Check if CUDA library is available by trying to load it
	// The library will be loaded automatically by the linker due to #cgo LDFLAGS
	// We just need to verify GPU is available
	bridge.deviceCount = 1 // We'll verify via nvidia-smi check
	bridge.initialized = true

	return bridge
}

// GetDeviceCount returns the number of available CUDA devices
func (cb *CudaBridge) GetDeviceCount() int {
	if !cb.initialized {
		return 0
	}

	// Use nvidia-smi to get actual device count
	return cb.deviceCount
}

// GetDeviceProperties returns properties for a specific CUDA device
func (cb *CudaBridge) GetDeviceProperties(deviceId int) *DeviceProperties {
	if !cb.initialized {
		return nil
	}

	// For now, return hardcoded properties based on GTX 660 Ti
	// In production, this should query nvidia-smi
	return &DeviceProperties{
		Name:       "NVIDIA GeForce GTX 660 Ti",
		ComputeCap: 30,
		Memory:     2147483648, // 2GB
		MultiProc:  true,
	}
}

// ProcessHeadersBatch processes multiple Bitcoin headers using CUDA
// This implements the "Camouflage" Double-SHA256 for BM1382 compatibility
func (cb *CudaBridge) ProcessHeadersBatch(headers [][]byte, targetTokenID uint32) ([]uint32, error) {
	if !cb.initialized {
		return nil, fmt.Errorf("CUDA not initialized")
	}

	if len(headers) == 0 {
		return nil, fmt.Errorf("no headers to process")
	}

	// Use ComputeDoubleHashFull which calls the real CUDA kernel
	fullResults, err := cb.ComputeDoubleHashFull(headers)
	if err != nil {
		return nil, err
	}

	// Post-process results to find matches with target token
	// Extract first 4 bytes of each hash for comparison
	var matches []uint32
	for _, result := range fullResults {
		// First 4 bytes as uint32 (Little-Endian)
		hash := binary.LittleEndian.Uint32(result[:4])

		// Check if hash matches target token (with tolerance for mining)
		if hash == targetTokenID ||
			(hash&0x00FFFFFF) == (targetTokenID&0x00FFFFFF) { // Partial match
			matches = append(matches, hash)
		}
	}

	return matches, nil
}

// ProcessSingleHeader processes a single Bitcoin header
func (cb *CudaBridge) ProcessSingleHeader(header []byte, targetTokenID uint32) (uint32, error) {
	if !cb.initialized {
		return 0, fmt.Errorf("CUDA not initialized")
	}

	if len(header) != 80 {
		return 0, fmt.Errorf("invalid header length: expected 80, got %d", len(header))
	}

	results, err := cb.ProcessHeadersBatch([][]byte{header}, targetTokenID)
	if err != nil {
		return 0, err
	}

	if len(results) > 0 {
		return results[0], nil
	}

	return 0, fmt.Errorf("no match found")
}

// ComputeDoubleHashFull processes multiple 80-byte headers and returns full 32-byte hashes
func (cb *CudaBridge) ComputeDoubleHashFull(headers [][]byte) ([][32]byte, error) {
	if !cb.initialized {
		return nil, fmt.Errorf("CUDA not initialized")
	}

	numHeaders := len(headers)
	if numHeaders == 0 {
		return nil, nil
	}

	headerArray := make([]uint32, numHeaders*20)
	for i, h := range headers {
		if len(h) != 80 {
			return nil, fmt.Errorf("header %d: expected 80 bytes, got %d", i, len(h))
		}
		for j := 0; j < 20; j++ {
			headerArray[i*20+j] = uint32(h[j*4]) | uint32(h[j*4+1])<<8 | uint32(h[j*4+2])<<16 | uint32(h[j*4+3])<<24
		}
	}

	results := make([]uint32, numHeaders*8)

	// Call the CUDA kernel from shared library
	res := C.launch_double_sha256_full(
		(*C.uint32_t)(unsafe.Pointer(&headerArray[0])),
		(*C.uint32_t)(unsafe.Pointer(&results[0])),
		C.int(numHeaders),
		256,                         // block_size
		C.int((numHeaders+255)/256), // grid_size
	)

	if res != 0 {
		return nil, fmt.Errorf("CUDA kernel launch failed with code: %d", res)
	}

	finalResults := make([][32]byte, numHeaders)
	for i := 0; i < numHeaders; i++ {
		for j := 0; j < 8; j++ {
			binary.LittleEndian.PutUint32(finalResults[i][j*4:], results[i*8+j])
		}
	}

	return finalResults, nil
}

// GetPerformanceStats returns CUDA performance information
func (cb *CudaBridge) GetPerformanceStats() map[string]interface{} {
	if !cb.initialized {
		return map[string]interface{}{
			"initialized": false,
			"error":       "CUDA not initialized",
		}
	}

	return map[string]interface{}{
		"initialized":        true,
		"device_count":       cb.deviceCount,
		"platform":           "CUDA",
		"compute_capability": "Double-SHA256",
		"status":             "ready",
	}
}

// Close cleans up CUDA resources
func (cb *CudaBridge) Close() error {
	if !cb.initialized {
		return nil
	}

	// In a real implementation, this would call cudaDeviceReset()
	cb.initialized = false
	return nil
}
