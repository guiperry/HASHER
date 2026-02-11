package simulator

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"hasher/pkg/hashing/core"
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

// Initialize sets up the HashMethod (defaults to software if not specified)
func (h *HasherWrapper) Initialize(config *SimulatorConfig) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if config != nil {
		h.config = config
	}

	// Default to software method if no specific method is configured
	if h.hashMethod == nil {
		h.hashMethod = software.NewSoftwareMethod()
		if err := h.hashMethod.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize software hash method: %w", err)
		}
	}

	h.isRunning = true
	return nil
}

// SetHashMethod allows injection of a specific HashMethod (e.g., ASIC, CUDA)
func (h *HasherWrapper) SetHashMethod(method core.HashMethod) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.hashMethod = method
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

	// Extract first 4 bytes as uint32 result
	result := binary.LittleEndian.Uint32(hash[:4])

	// Cache result
	if len(h.cache) < h.config.CacheSize {
		h.cacheMutex.Lock()
		h.cache[cacheKey] = result
		h.cacheMutex.Unlock()
	}

	return result, nil
}

// SimulateBitcoinHeaderFull performs Double-SHA256 on 80-byte Bitcoin header and returns full 32 bytes
func (h *HasherWrapper) SimulateBitcoinHeaderFull(header []byte) ([]byte, error) {
	if !h.isRunning {
		return nil, fmt.Errorf("simulator is not running")
	}

	if h.hashMethod == nil {
		return nil, fmt.Errorf("hash method not initialized")
	}

	if len(header) != 80 {
		return nil, fmt.Errorf("invalid Bitcoin header length: expected 80 bytes, got %d", len(header))
	}

	// Double SHA-256 of the header
	hash1, err := h.hashMethod.ComputeHash(header)
	if err != nil {
		return nil, err
	}
	
	hash2, err := h.hashMethod.ComputeHash(hash1[:])
	if err != nil {
		return nil, err
	}

	return hash2[:], nil
}

// SimulateBitcoinHeader returns first 4 bytes of Double-SHA256 as uint32
func (h *HasherWrapper) SimulateBitcoinHeader(header []byte) (uint32, error) {
	hashFull, err := h.SimulateBitcoinHeaderFull(header)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(hashFull[:4]), nil
}

// RecursiveMine performs the 21-pass temporal loop with associative jitter
// Returns the full 32-byte double-SHA256 hash of the final pass
func (h *HasherWrapper) RecursiveMine(header []byte, passes int) ([]byte, error) {
	if !h.isRunning {
		return nil, fmt.Errorf("simulator is not running")
	}

	// Working copy of header
	currentHeader := make([]byte, 80)
	copy(currentHeader, header)

	var lastHashFull []byte

	// Connect to Jitter RPC Server
	conn, err := net.Dial("unix", "/tmp/jitter.sock")
	if err != nil {
		// Fallback to non-jitter mining if server not available
		return h.SimulateBitcoinHeaderFull(header)
	}
	defer conn.Close()

	for i := 0; i < passes; i++ {
		// 1. Pass i: Hash current state
		hashFull, err := h.SimulateBitcoinHeaderFull(currentHeader)
		if err != nil {
			return nil, err
		}
		lastHashFull = hashFull

		// Extract uint32 for Jitter RPC
		hash32 := binary.LittleEndian.Uint32(hashFull[:4])

		// 2. RPC: Get jitter from Host (Dimension Shift: Search 0, Retrieve 1)
		jitter, err := h.getJitterRPC(conn, hash32)
		if err != nil {
			continue // Skip jitter if RPC fails
		}

		// 3. Inject: XOR jitter into Merkle Root slots (e.g. slot 8 = bytes 36-39)
		// We rotate through the 4 Merkle Root slots (bytes 36-51)
		slotIdx := i % 4
		offset := 36 + (slotIdx * 4)
		
		existing := binary.BigEndian.Uint32(currentHeader[offset : offset+4])
		binary.BigEndian.PutUint32(currentHeader[offset:offset+4], existing^jitter)
	}

	return lastHashFull, nil
}

func (h *HasherWrapper) getJitterRPC(conn net.Conn, hash uint32) (uint32, error) {
	// Send 4-byte hash
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, hash)
	if _, err := conn.Write(buf); err != nil {
		return 0, err
	}

	// Read 4-byte jitter
	if _, err := conn.Read(buf); err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(buf), nil
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
