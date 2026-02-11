package simulator

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/methods/software"
)

// ... (existing struct)

// RecursiveMine performs the 21-pass temporal loop with associative jitter
func (h *HasherWrapper) RecursiveMine(header []byte, passes int) (uint32, error) {
	if !h.isRunning {
		return 0, fmt.Errorf("simulator is not running")
	}

	if len(header) != 80 {
		return 0, fmt.Errorf("invalid header length")
	}

	// Working copy of header
	currentHeader := make([]byte, 80)
	copy(currentHeader, header)

	var lastHash uint32

	// Connect to Jitter RPC Server
	conn, err := net.Dial("unix", "/tmp/jitter.sock")
	if err != nil {
		// Fallback to non-jitter mining if server not available
		return h.SimulateBitcoinHeader(header)
	}
	defer conn.Close()

	for i := 0; i < passes; i++ {
		// 1. Pass i: Hash current state
		hash, err := h.SimulateBitcoinHeader(currentHeader)
		if err != nil {
			return 0, err
		}
		lastHash = hash

		// 2. RPC: Get jitter from Host (Dimension Shift: Search 0, Retrieve 1)
		jitter, err := h.getJitterRPC(conn, hash)
		if err != nil {
			continue // Skip jitter if RPC fails
		}

		// 3. Inject: XOR jitter into Merkle Root slots (e.g. slot 8 = bytes 36-39)
		// We rotate through the 4 Merkle Root slots
		slotIdx := i % 4
		offset := 36 + (slotIdx * 4)
		
		existing := binary.BigEndian.Uint32(currentHeader[offset : offset+4])
		binary.BigEndian.PutUint32(currentHeader[offset:offset+4], existing^jitter)
	}

	return lastHash, nil
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

// SimulateBitcoinHeader performs Double-SHA256 on 80-byte Bitcoin header
// Uses HashMethod.MineHeader for hardware acceleration when available
func (h *HasherWrapper) SimulateBitcoinHeader(header []byte) (uint32, error) {
	if !h.isRunning {
		return 0, fmt.Errorf("simulator is not running")
	}

	if h.hashMethod == nil {
		return 0, fmt.Errorf("hash method not initialized")
	}

	if len(header) != 80 {
		return 0, fmt.Errorf("invalid Bitcoin header length: expected 80 bytes, got %d", len(header))
	}

	// Create cache key
	cacheKey := fmt.Sprintf("btc_%x", header[:16])

	// Check cache
	h.cacheMutex.RLock()
	if cached, exists := h.cache[cacheKey]; exists {
		h.cacheMutex.RUnlock()
		return cached, nil
	}
	h.cacheMutex.RUnlock()

	// Use MineHeader for Bitcoin-style mining
	// This will use ASIC hardware if available, otherwise software fallback
	nonceStart := binary.LittleEndian.Uint32(header[76:80])
	nonceEnd := nonceStart + 0xFFFFFFFF

	result, err := h.hashMethod.MineHeader(header, nonceStart, nonceEnd)
	if err != nil {
		return 0, fmt.Errorf("bitcoin header mining failed: %w", err)
	}

	// Cache result
	if len(h.cache) < h.config.CacheSize {
		h.cacheMutex.Lock()
		h.cache[cacheKey] = result
		h.cacheMutex.Unlock()
	}

	return result, nil
}

// ValidateSeed checks if seed produces target token in any pass
func (h *HasherWrapper) ValidateSeed(seed []byte, targetToken int32) (bool, error) {
	if !h.isRunning {
		return false, fmt.Errorf("simulator is not running")
	}

	// Check multiple passes (as in original vhasher)
	for pass := 0; pass < 21; pass++ {
		nonce, err := h.SimulateHash(seed, pass)
		if err != nil {
			return false, err
		}

		if nonce == uint32(targetToken) {
			return true, nil
		}
	}

	// Check final pass specifically
	finalNonce, err := h.SimulateHash(seed, 20)
	if err != nil {
		return false, err
	}

	return finalNonce == uint32(targetToken), nil
}

// ProcessHeadersBatch processes multiple Bitcoin headers in parallel
// Optimized for batch processing using HashMethod.ComputeBatch
func (h *HasherWrapper) ProcessHeadersBatch(headers [][]byte, targetTokenID uint32) ([]uint32, error) {
	if !h.isRunning {
		return nil, fmt.Errorf("simulator is not running")
	}

	if h.hashMethod == nil {
		return nil, fmt.Errorf("hash method not initialized")
	}

	results := make([]uint32, len(headers))

	// Use batch processing if available
	// Note: HashMethod.ComputeBatch expects data to hash, not headers to mine
	// For mining, we process individually but could parallelize
	for i, header := range headers {
		result, err := h.SimulateBitcoinHeader(header)
		if err != nil {
			// Continue processing other headers
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
