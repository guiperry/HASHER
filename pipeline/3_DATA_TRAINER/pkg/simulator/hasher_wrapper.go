package simulator

import (
	"encoding/binary"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/methods/cuda"
	"hasher/pkg/hashing/methods/software"
)

// HasherWrapper implements the HashSimulator interface using hasher's HashMethod
// This replaces the internal vhasher simulator with the production-grade hasher module
type HasherWrapper struct {
	hashMethod core.HashMethod
	config     *SimulatorConfig
	cache      map[string]uint32
	cacheMutex sync.RWMutex
	isRunning  bool
	mutex      sync.RWMutex
	methodType string // "auto", "software", "cuda"
}

// NewHasherWrapper creates a new HashSimulator wrapper using hasher's HashMethod
// It defaults to software implementation if no hardware is available
func NewHasherWrapper(config *SimulatorConfig) *HasherWrapper {
	if config == nil {
		config = &SimulatorConfig{
			DeviceType:     "software",
			MaxConcurrency: 100,
			TargetHashRate: 500000000,
			CacheSize:      10000,
			GPUDevice:      0,
			Timeout:        30,
		}
	}

	return &HasherWrapper{
		config:    config,
		cache:     make(map[string]uint32),
		isRunning: false,
	}
}

// Initialize sets up the HashMethod based on configuration
func (h *HasherWrapper) Initialize(config *SimulatorConfig) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if config != nil {
		h.config = config
		// Check if method type is specified in DeviceType
		if strings.Contains(config.DeviceType, "software") {
			h.methodType = "software"
		} else if strings.Contains(config.DeviceType, "cuda") {
			h.methodType = "cuda"
		} else {
			h.methodType = "auto"
		}
	}

	// Initialize hash method based on type
	if h.hashMethod == nil {
		var err error
		h.hashMethod, err = h.createHashMethod()
		if err != nil {
			return fmt.Errorf("failed to create hash method: %w", err)
		}

		if err := h.hashMethod.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize hash method: %w", err)
		}
	}

	h.isRunning = true
	return nil
}

// createHashMethod creates the appropriate hash method based on configuration
func (h *HasherWrapper) createHashMethod() (core.HashMethod, error) {
	switch h.methodType {
	case "cuda":
		// Directly create CUDA method
		cudaMethod := cuda.NewCudaMethod()
		if cudaMethod.IsAvailable() {
			return cudaMethod, nil
		}
		// Fall back to software if CUDA not available
		return software.NewSoftwareMethod(), nil

	case "software":
		return software.NewSoftwareMethod(), nil

	case "auto":
		// Auto-detect: try CUDA first, then software
		if h.isCUDAAvailable() {
			cudaMethod := cuda.NewCudaMethod()
			if cudaMethod.IsAvailable() {
				return cudaMethod, nil
			}
		}
		return software.NewSoftwareMethod(), nil

	default:
		return software.NewSoftwareMethod(), nil
	}
}

// isCUDAAvailable checks if CUDA is available on the system
func (h *HasherWrapper) isCUDAAvailable() bool {
	cmd := exec.Command("nvidia-smi")
	err := cmd.Run()
	return err == nil
}

// SetHashMethod allows injection of a specific HashMethod (e.g., ASIC, CUDA)
func (h *HasherWrapper) SetHashMethod(method core.HashMethod) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.hashMethod = method
}

// SetMethodType sets the hash method type (auto, software, cuda)
func (h *HasherWrapper) SetMethodType(methodType string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.methodType = methodType
}

// Shutdown cleans up the HashMethod
func (h *HasherWrapper) Shutdown() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.isRunning = false

	if h.hashMethod != nil {
		if err := h.hashMethod.Shutdown(); err != nil {
			return fmt.Errorf("failed to shutdown hash method: %w", err)
		}
	}

	h.cacheMutex.Lock()
	h.cache = make(map[string]uint32)
	h.cacheMutex.Unlock()

	return nil
}

// SimulateHash performs deterministic hashing using the HashMethod
// Maps to: SHA256(seed || pass) with caching
func (h *HasherWrapper) SimulateHash(seed []byte, pass int) (uint32, error) {
	if !h.isRunning {
		return 0, fmt.Errorf("simulator is not running")
	}

	if h.hashMethod == nil {
		return 0, fmt.Errorf("hash method not initialized")
	}

	// Create cache key
	cacheKey := fmt.Sprintf("%x_%d", seed, pass)

	// Check cache
	h.cacheMutex.RLock()
	if cached, exists := h.cache[cacheKey]; exists {
		h.cacheMutex.RUnlock()
		return cached, nil
	}
	h.cacheMutex.RUnlock()

	// Combine seed with pass for deterministic hashing
	data := make([]byte, len(seed)+4)
	copy(data, seed)
	binary.LittleEndian.PutUint32(data[len(seed):], uint32(pass))

	// Use HashMethod for computation
	hash, err := h.hashMethod.ComputeHash(data)
	if err != nil {
		return 0, fmt.Errorf("hash computation failed: %w", err)
	}

	// Extract first 4 bytes as uint32 result (Big-Endian for simulator compatibility)
	result := binary.BigEndian.Uint32(hash[:4])

	// Cache result
	if len(h.cache) < h.config.CacheSize {
		h.cacheMutex.Lock()
		h.cache[cacheKey] = result
		h.cacheMutex.Unlock()
	}

	return result, nil
}

// SimulateBitcoinHeader returns first 4 bytes of Double-SHA256 as uint32
func (h *HasherWrapper) SimulateBitcoinHeader(header []byte) (uint32, error) {
	if !h.isRunning {
		return 0, fmt.Errorf("simulator is not running")
	}

	if h.hashMethod == nil {
		return 0, fmt.Errorf("hash method not initialized")
	}

	// Use MineHeader with a 0-0 range to just get the hash of the current header
	// or better, if the hashMethod supports a direct double-hash.
	// Most HashMethods implement double-hash internally for MineHeader.
	// But core.HashMethod doesn't have a direct "ComputeDoubleHash" exposed.
	// It has ComputeHash.

	hash1, err := h.hashMethod.ComputeHash(header)
	if err != nil {
		return 0, err
	}

	hash2, err := h.hashMethod.ComputeHash(hash1[:])
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(hash2[:4]), nil
}

// RecursiveMine performs the 21-pass temporal loop with associative jitter
// Returns the full 32-byte double-SHA256 hash of the final pass
func (h *HasherWrapper) RecursiveMine(header []byte, passes int) ([]byte, error) {
	if !h.isRunning {
		return nil, fmt.Errorf("simulator is not running")
	}

	if h.hashMethod == nil {
		return nil, fmt.Errorf("hash method not initialized")
	}

	// Delegate directly to the production-grade recursive mine implementation
	return h.hashMethod.ExecuteRecursiveMine(header, passes)
}

// ValidateSeed checks if seed produces target token in any pass
func (h *HasherWrapper) ValidateSeed(seed []byte, targetToken int32) (bool, error) {
	if !h.isRunning {
		return false, fmt.Errorf("simulator is not running")
	}

	// Check multiple passes
	for pass := 0; pass < 21; pass++ {
		nonce, err := h.SimulateHash(seed, pass)
		if err != nil {
			return false, err
		}

		if nonce == uint32(targetToken) {
			return true, nil
		}
	}

	return false, nil
}

// ProcessHeadersBatch processes multiple Bitcoin headers in parallel
func (h *HasherWrapper) ProcessHeadersBatch(headers [][]byte, targetTokenID uint32) ([]uint32, error) {
	if !h.isRunning {
		return nil, fmt.Errorf("simulator is not running")
	}

	results := make([]uint32, len(headers))
	for i, header := range headers {
		result, err := h.SimulateBitcoinHeader(header)
		if err != nil {
			continue
		}
		results[i] = result
	}

	return results, nil
}

// GetHashMethod returns the underlying HashMethod for advanced usage
func (h *HasherWrapper) GetHashMethod() core.HashMethod {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return h.hashMethod
}

// ClearCache clears the internal cache
func (h *HasherWrapper) ClearCache() {
	h.cacheMutex.Lock()
	defer h.cacheMutex.Unlock()
	h.cache = make(map[string]uint32)
}

// GetCacheSize returns current cache size
func (h *HasherWrapper) GetCacheSize() int {
	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()
	return len(h.cache)
}
