package hardware

import (
	"encoding/binary"
	"fmt"
	"time"
)

// BitcoinHeader represents the 80-byte Bitcoin block header structure
// This is used for the "Camouflage" strategy to trick BM1382 ASICs
type BitcoinHeader struct {
	Version    uint32    // 4 bytes: Block version
	PrevHash   [8]uint32 // 32 bytes: Previous block hash (our slots 0-7)
	MerkleRoot [8]uint32 // 32 bytes: Merkle root (our slots 8-11 + padding)
	Timestamp  uint32    // 4 bytes: Block timestamp
	Bits       uint32    // 4 bytes: Difficulty target
	Nonce      uint32    // 4 bytes: The nonce we're hunting for
}

// BitcoinMiningStats represents mining performance metrics
type BitcoinMiningStats struct {
	HashRate       float64       `json:"hash_rate"`
	ValidHeaders   int           `json:"valid_headers"`
	InvalidHeaders int           `json:"invalid_headers"`
	LastNonce      uint32        `json:"last_nonce"`
	LastHash       string        `json:"last_hash"`
	MiningTime     time.Duration `json:"mining_time"`
}

// constants for Bitcoin header construction
const (
	BitcoinVersion = 0x00000002
	BitcoinBits    = 0x1d00ffff
	MaxRetries     = 3
	TargetTimeout  = 30 * time.Second
)

// HardwarePrep provides utilities for preparing ASIC jobs with Bitcoin format
type HardwarePrep struct {
	enableCaching bool
	headerCache   map[string][]byte
	stats         *BitcoinMiningStats
	lastUpdate    time.Time
}

// NewHardwarePrep creates a new hardware preparation utility
func NewHardwarePrep(enableCache bool) *HardwarePrep {
	return &HardwarePrep{
		enableCaching: enableCache,
		headerCache:   make(map[string][]byte),
	}
}

// PrepareAsicJob creates an 80-byte Bitcoin header from neural slots and candidate nonce
// This implements the "Byte-Flipper" functionality from the refactor specification
func (hp *HardwarePrep) PrepareAsicJob(slots [12]uint32, candidateNonce uint32) []byte {
	// Create cache key for potential reuse
	cacheKey := hp.createCacheKey(slots, candidateNonce)
	if hp.enableCaching {
		if cached, exists := hp.headerCache[cacheKey]; exists {
			return cached
		}
	}

	header := make([]byte, 80)

	// Bytes 0-4: Version (Little-Endian)
	binary.LittleEndian.PutUint32(header[0:4], BitcoinVersion)

	// Bytes 4-36: Slots 0-7 -> PrevBlockHash (Big-Endian for SHA-256)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(header[4+(i*4):], slots[i])
	}

	// Bytes 36-68: Slots 8-11 + Padding -> MerkleRoot (Big-Endian)
	// Slots 8-11 occupy bytes 36-51 (4 slots * 4 bytes = 16 bytes)
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(header[36+(i*4):], slots[i+8])
	}
	// Pad remaining 16 bytes of MerkleRoot (bytes 52-67) with zeros
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(header[52+(i*4):], 0)
	}

	// Bytes 68-72: Timestamp (Little-Endian)
	binary.LittleEndian.PutUint32(header[68:72], uint32(time.Now().Unix()))

	// Bytes 72-76: Bits (Little-Endian)
	binary.LittleEndian.PutUint32(header[72:76], BitcoinBits)

	// Bytes 76-80: Candidate Nonce (Little-Endian)
	binary.LittleEndian.PutUint32(header[76:80], candidateNonce)

	// Cache the result if enabled
	if hp.enableCaching {
		hp.headerCache[cacheKey] = append([]byte(nil), header...)
	}

	return header
}

// PrepareAsicJobBatch creates multiple Bitcoin headers for batch processing
// Optimized for GPU processing and population evaluation
func (hp *HardwarePrep) PrepareAsicJobBatch(slots [12]uint32, candidateNonces []uint32) [][]byte {
	headers := make([][]byte, len(candidateNonces))

	// Pre-compute static header components (first 76 bytes)
	staticHeader := make([]byte, 76)

	// Version
	binary.LittleEndian.PutUint32(staticHeader[0:4], BitcoinVersion)

	// PrevBlockHash (slots 0-7, Big-Endian)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(staticHeader[4+(i*4):], slots[i])
	}

	// MerkleRoot (slots 8-11 + padding, Big-Endian)
	// Slots 8-11 occupy bytes 36-51
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(staticHeader[36+(i*4):], slots[i+8])
	}
	// Pad remaining 16 bytes (bytes 52-67) with zeros
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(staticHeader[52+(i*4):], 0)
	}

	// Timestamp (will be updated per header)
	binary.LittleEndian.PutUint32(staticHeader[68:72], uint32(time.Now().Unix()))

	// Bits
	binary.LittleEndian.PutUint32(staticHeader[72:76], BitcoinBits)

	// Generate headers with different nonces
	for i, nonce := range candidateNonces {
		header := make([]byte, 80)
		copy(header, staticHeader) // Copy static portion

		// Add nonce (bytes 76-80, Little-Endian)
		binary.LittleEndian.PutUint32(header[76:80], nonce)

		headers[i] = header
	}

	return headers
}

// createCacheKey generates a unique key for header caching
func (hp *HardwarePrep) createCacheKey(slots [12]uint32, nonce uint32) string {
	return fmt.Sprintf("%x:%x", slots, nonce)
}

// ClearCache clears the header cache
func (hp *HardwarePrep) ClearCache() {
	if hp.enableCaching {
		hp.headerCache = make(map[string][]byte)
	}
}

// GetCacheStats returns cache statistics
func (hp *HardwarePrep) GetCacheStats() (int, int) {
	if !hp.enableCaching {
		return 0, 0
	}
	return len(hp.headerCache), len(hp.headerCache) * 80
}

// ValidateHeader performs basic validation on Bitcoin header format
func (hp *HardwarePrep) ValidateHeader(header []byte) bool {
	if len(header) != 80 {
		return false
	}

	// Check version field
	version := binary.LittleEndian.Uint32(header[0:4])
	if version != BitcoinVersion {
		return false
	}

	// Check bits field
	bits := binary.LittleEndian.Uint32(header[72:76])
	if bits != BitcoinBits {
		return false
	}

	return true
}

// ExtractNonce extracts nonce from a Bitcoin header
func (hp *HardwarePrep) ExtractNonce(header []byte) uint32 {
	if len(header) < 80 {
		return 0
	}
	return binary.LittleEndian.Uint32(header[76:80])
}

// ExtractSlots extracts 12 neural slots from a Bitcoin header
// This is the reverse operation of PrepareAsicJob
func (hp *HardwarePrep) ExtractSlots(header []byte) [12]uint32 {
	var slots [12]uint32

	if len(header) < 80 {
		return slots
	}

	// Extract slots 0-7 from PrevBlockHash (convert from Big-Endian to uint32)
	for i := 0; i < 8; i++ {
		slots[i] = binary.BigEndian.Uint32(header[4+(i*4):])
	}

	// Extract slots 8-11 from MerkleRoot (convert from Big-Endian to uint32)
	for i := 0; i < 4; i++ {
		slots[i+8] = binary.BigEndian.Uint32(header[36+(i*4):])
	}

	return slots
}

// UpdateStats updates mining performance statistics
func (hp *HardwarePrep) UpdateStats(hashRate float64, validHeaders, invalidHeaders int, lastNonce uint32, lastHash string) {
	if hp.stats == nil {
		hp.stats = &BitcoinMiningStats{}
	}

	hp.stats.HashRate = hashRate
	hp.stats.ValidHeaders = validHeaders
	hp.stats.InvalidHeaders = invalidHeaders
	hp.stats.LastNonce = lastNonce
	hp.stats.LastHash = lastHash
	hp.stats.MiningTime = time.Since(hp.lastUpdate)
	hp.lastUpdate = time.Now()
}

// GetStats returns current mining statistics
func (hp *HardwarePrep) GetStats() *BitcoinMiningStats {
	if hp.stats == nil {
		return &BitcoinMiningStats{}
	}

	// Return copy of stats to prevent external modification
	stats := *hp.stats
	return &stats
}

// CalculateHashPerformance calculates hash rate and efficiency metrics
func (hp *HardwarePrep) CalculateHashPerformance(startTime time.Time, headersProcessed int) (float64, float64) {
	elapsed := time.Since(startTime).Seconds()
	if elapsed > 0 {
		hashRate := float64(headersProcessed) / elapsed
		efficiency := float64(headersProcessed) / float64(headersProcessed+hp.stats.InvalidHeaders) * 100
		return hashRate, efficiency
	}
	return 0, 0
}

// ValidateSlots checks if neural slots are valid for Bitcoin header construction
func ValidateSlots(slots [12]uint32) bool {
	// Check if all slots are non-zero
	nonZeroCount := 0
	for _, slot := range slots {
		if slot != 0 {
			nonZeroCount++
		}
	}

	// Require at least 8 non-zero slots for meaningful Bitcoin header
	return nonZeroCount >= 8
}
