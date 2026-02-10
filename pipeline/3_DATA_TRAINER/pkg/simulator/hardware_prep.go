package simulator

import (
	"encoding/binary"
	"fmt"
	"sync"
)

// HardwarePrep provides utilities for preparing ASIC-compatible Bitcoin headers
// from neural network slots. This is the "camouflage" layer that transforms
// neural data into a format compatible with BM1382 ASIC miners.
type HardwarePrep struct {
	enableCaching bool
	cache         map[string][]byte
	cacheMutex    sync.RWMutex
}

// NewHardwarePrep creates a new HardwarePrep instance
// enableCaching: if true, caches headers to avoid recomputation
func NewHardwarePrep(enableCaching bool) *HardwarePrep {
	hp := &HardwarePrep{
		enableCaching: enableCaching,
		cache:         make(map[string][]byte),
	}
	return hp
}

// PrepareAsicJob creates an 80-byte Bitcoin header from 12 neural slots
// This implements the "Camouflage" strategy for BM1382 compatibility:
// - Slots 0-7 map to Previous Block Hash (bytes 4-35)
// - Slots 8-11 map to Merkle Root (bytes 36-51, partial)
// - Nonce goes in bytes 76-79
func (hp *HardwarePrep) PrepareAsicJob(slots [12]uint32, nonce uint32) []byte {
	// Create cache key if caching is enabled
	var cacheKey string
	if hp.enableCaching {
		cacheKey = hp.makeCacheKey(slots, nonce)
		hp.cacheMutex.RLock()
		if cached, exists := hp.cache[cacheKey]; exists {
			hp.cacheMutex.RUnlock()
			return cached
		}
		hp.cacheMutex.RUnlock()
	}

	// Create 80-byte Bitcoin header
	header := make([]byte, 80)

	// Bytes 0-3: Version (Little-Endian) - Bitcoin version 2
	binary.LittleEndian.PutUint32(header[0:4], 0x00000002)

	// Bytes 4-35: Previous Block Hash (Big-Endian)
	// Map slots 0-7 to these bytes
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(header[4+(i*4):8+(i*4)], slots[i])
	}

	// Bytes 36-67: Merkle Root (Big-Endian)
	// Map slots 8-11 to the first 16 bytes of Merkle Root
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(header[36+(i*4):40+(i*4)], slots[8+i])
	}
	// Remaining Merkle Root bytes (52-67) are left as zeros (padding)

	// Bytes 68-71: Timestamp (Little-Endian) - fixed for determinism
	binary.LittleEndian.PutUint32(header[68:72], 0x5D00C5A0)

	// Bytes 72-75: Bits/Difficulty (Little-Endian) - Difficulty 1
	binary.LittleEndian.PutUint32(header[72:76], 0x1d00ffff)

	// Bytes 76-79: Nonce (Little-Endian)
	binary.LittleEndian.PutUint32(header[76:80], nonce)

	// Cache the result if caching is enabled
	if hp.enableCaching {
		hp.cacheMutex.Lock()
		hp.cache[cacheKey] = header
		hp.cacheMutex.Unlock()
	}

	return header
}

// PrepareAsicJobBatch creates multiple Bitcoin headers for batch processing
func (hp *HardwarePrep) PrepareAsicJobBatch(slots [12]uint32, nonces []uint32) [][]byte {
	headers := make([][]byte, len(nonces))
	for i, nonce := range nonces {
		headers[i] = hp.PrepareAsicJob(slots, nonce)
	}
	return headers
}

// ExtractNonce extracts the nonce from an 80-byte Bitcoin header
func (hp *HardwarePrep) ExtractNonce(header []byte) uint32 {
	if len(header) < 80 {
		return 0
	}
	return binary.LittleEndian.Uint32(header[76:80])
}

// ExtractSlots extracts the 12 neural slots from an 80-byte Bitcoin header
func (hp *HardwarePrep) ExtractSlots(header []byte) [12]uint32 {
	var slots [12]uint32

	if len(header) < 80 {
		return slots
	}

	// Extract slots 0-7 from Previous Block Hash (bytes 4-35, Big-Endian)
	for i := 0; i < 8; i++ {
		slots[i] = binary.BigEndian.Uint32(header[4+(i*4) : 8+(i*4)])
	}

	// Extract slots 8-11 from Merkle Root (bytes 36-51, Big-Endian)
	for i := 0; i < 4; i++ {
		slots[8+i] = binary.BigEndian.Uint32(header[36+(i*4) : 40+(i*4)])
	}

	return slots
}

// ValidateHeader checks if a header has valid structure
func (hp *HardwarePrep) ValidateHeader(header []byte) bool {
	if len(header) != 80 {
		return false
	}

	// Check version is valid (should be 2 for our implementation)
	version := binary.LittleEndian.Uint32(header[0:4])
	if version != 0x00000002 {
		return false
	}

	// Check difficulty bits are valid (should be 0x1d00ffff for Difficulty 1)
	bits := binary.LittleEndian.Uint32(header[72:76])
	if bits != 0x1d00ffff {
		return false
	}

	return true
}

// GetCacheStats returns the current cache size and number of entries
func (hp *HardwarePrep) GetCacheStats() (int, int) {
	if !hp.enableCaching {
		return 0, 0
	}

	hp.cacheMutex.RLock()
	defer hp.cacheMutex.RUnlock()

	// Calculate approximate size (rough estimate)
	size := len(hp.cache) * 80 // Each header is 80 bytes
	return size, len(hp.cache)
}

// ClearCache clears the header cache
func (hp *HardwarePrep) ClearCache() {
	if !hp.enableCaching {
		return
	}

	hp.cacheMutex.Lock()
	defer hp.cacheMutex.Unlock()

	hp.cache = make(map[string][]byte)
}

// makeCacheKey creates a cache key from slots and nonce
func (hp *HardwarePrep) makeCacheKey(slots [12]uint32, nonce uint32) string {
	return fmt.Sprintf("%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x_%08x",
		slots[0], slots[1], slots[2], slots[3],
		slots[4], slots[5], slots[6], slots[7],
		slots[8], slots[9], slots[10], slots[11],
		nonce)
}
