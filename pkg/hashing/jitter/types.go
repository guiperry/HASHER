// Package jitter implements the 21-pass temporal loop jitter mechanism
// for dynamic associative hashing in the HASHER architecture.
package jitter

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Default number of passes in the temporal loop
const DefaultPassCount = 21

// Domain Signature Constants (Slot 10)
const (
	DOMAIN_PROSE     uint32 = 0x1000
	DOMAIN_ACADEMIC  uint32 = 0x1100
	DOMAIN_MATH      uint32 = 0x2000
	DOMAIN_LOGIC     uint32 = 0x2100
	DOMAIN_CODE      uint32 = 0x3000
	DOMAIN_MARKUP    uint32 = 0x3100
	DOMAIN_FINANCIAL uint32 = 0x4000
	DOMAIN_TECHNICAL uint32 = 0x5000
	DOMAIN_DEBUG     uint32 = 0xFFFF
)

// JitterVector represents a single jitter value injected into the hash state
type JitterVector uint32

// HashState represents the current state during the 21-pass temporal loop
type HashState struct {
	// Current 80-byte Bitcoin-style header being hashed
	Header []byte

	// Current pass number (0-20)
	Pass int

	// Accumulated jitter vectors from previous passes
	JitterHistory []JitterVector

	// Target token ID we're trying to match
	TargetTokenID uint32

	// Current golden nonce candidate
	Nonce uint32
}

// NewHashState creates a new hash state for the temporal loop
func NewHashState(header []byte, targetTokenID uint32) *HashState {
	h := make([]byte, len(header))
	copy(h, header)
	return &HashState{
		Header:        h,
		Pass:          0,
		JitterHistory: make([]JitterVector, 0, DefaultPassCount),
		TargetTokenID: targetTokenID,
		Nonce:         0,
	}
}

// Clone creates a deep copy of the hash state
func (hs *HashState) Clone() *HashState {
	cloned := &HashState{
		Header:        make([]byte, len(hs.Header)),
		Pass:          hs.Pass,
		JitterHistory: make([]JitterVector, len(hs.JitterHistory)),
		TargetTokenID: hs.TargetTokenID,
		Nonce:         hs.Nonce,
	}
	copy(cloned.Header, hs.Header)
	copy(cloned.JitterHistory, hs.JitterHistory)
	return cloned
}

// ExtractLookupKey extracts the lookup key from a hash result
// Uses the first 4 bytes of the hash for the flash search
func ExtractLookupKey(hash [32]byte) uint32 {
	return binary.BigEndian.Uint32(hash[:4])
}

// XORJitterIntoHeader applies a jitter vector to the MerkleRoot section (bytes 36-68)
// The jitter is XORed into slots 8-11 of the header
func XORJitterIntoHeader(header []byte, jitter JitterVector) error {
	if len(header) != 80 {
		return fmt.Errorf("invalid header length: expected 80, got %d", len(header))
	}

	// MerkleRoot starts at byte 36, we affect bytes 36-52 (4 slots)
	// Apply jitter to each 4-byte slot in the MerkleRoot
	for i := 0; i < 4; i++ {
		offset := 36 + (i * 4)
		current := binary.BigEndian.Uint32(header[offset:])
		// Rotate jitter for each slot to distribute entropy
		rotatedJitter := uint32(jitter)<<(i*8) | uint32(jitter)>>(32-i*8)
		newValue := current ^ rotatedJitter
		binary.BigEndian.PutUint32(header[offset:], newValue)
	}

	return nil
}

// TemporalPassResult contains the result of a single pass through the temporal loop
type TemporalPassResult struct {
	// Hash output from this pass
	Hash [32]byte

	// Jitter vector applied in this pass (if any)
	AppliedJitter JitterVector

	// Whether jitter was found in the database
	JitterFound bool

	// Pass number (0-indexed)
	Pass int
}

// GoldenNonceResult represents the outcome of the 21-pass search
type GoldenNonceResult struct {
	// The discovered golden nonce
	Nonce uint32

	// Whether a valid golden nonce was found
	Found bool

	// Final hash after 21 passes
	FinalHash [32]byte

	// Full 32-byte seed result
	FullSeed []byte

	// Number of passes completed
	PassesCompleted int

	// Stability score (consistency across passes)
	Stability float64

	// Alignment score (how well final hash matches target)
	Alignment float64

	// All jitter vectors applied during the search
	JitterVectors []JitterVector

	// Metadata for debugging/analysis
	Metadata map[string]interface{}
}

// ComputeStability calculates the stability score from jitter history
// Higher score means more consistent jitter patterns
func ComputeStability(jitters []JitterVector) float64 {
	if len(jitters) < 2 {
		return 1.0 // Perfect stability with no history
	}

	// Calculate variance in jitter values
	var sum uint64
	for _, j := range jitters {
		sum += uint64(j)
	}
	mean := float64(sum) / float64(len(jitters))

	var variance float64
	for _, j := range jitters {
		diff := float64(j) - mean
		variance += diff * diff
	}
	variance /= float64(len(jitters))

	// Convert variance to stability (0-1 range)
	// Lower variance = higher stability
	maxVariance := 1 << 30 // Expected maximum variance for uint32
	stability := 1.0 - (variance / float64(maxVariance))
	if stability < 0 {
		stability = 0
	}
	if stability > 1 {
		stability = 1
	}

	return stability
}

// ComputeAlignment calculates how well the final hash matches the target
// Returns a value between 0 and 1
func ComputeAlignment(finalHash [32]byte, targetTokenID uint32) float64 {
	// Use first 4 bytes of hash as the golden nonce
	goldenNonce := binary.BigEndian.Uint32(finalHash[:4])

	if goldenNonce == targetTokenID {
		return 1.0
	}

	// Calculate bit-wise similarity
	xor := goldenNonce ^ targetTokenID
	matchingBits := 0
	for i := 0; i < 32; i++ {
		if (xor>>i)&1 == 0 {
			matchingBits++
		}
	}

	return float64(matchingBits) / 32.0
}

// JitterConfig contains configuration for the jitter system
type JitterConfig struct {
	// Number of passes in the temporal loop
	PassCount int

	// Default jitter value when no association found
	DefaultJitter JitterVector

	// Whether to use database lookups for jitter
	EnableFlashSearch bool

	// Path to the jitter RPC socket
	JitterSocketPath string

	// Cache size for jitter table
	JitterCacheSize int

	// Enable detailed logging
	Verbose bool
}

// DefaultJitterConfig returns default jitter configuration
func DefaultJitterConfig() *JitterConfig {
	return &JitterConfig{
		PassCount:         DefaultPassCount,
		DefaultJitter:     JitterVector(0xDEADBEEF),
		EnableFlashSearch: true,
		JitterSocketPath:  "/tmp/jitter.sock",
		JitterCacheSize:   10000,
		Verbose:           false,
	}
}

// TrainingFrame represents a training sample for the evolutionary process
type TrainingFrame struct {
	// Source file information
	SourceFile string `json:"source_file"`

	// Chunk and window identifiers
	ChunkID       int32 `json:"chunk_id"`
	WindowStart   int32 `json:"window_start"`
	WindowEnd     int32 `json:"window_end"`
	ContextLength int32 `json:"context_length"`

	// 12 ASIC slots (neural feature vector)
	AsicSlots [12]uint32 `json:"asic_slots"`

	// Original token sequence for semantic matching
	TokenSequence []int `json:"token_sequence,omitempty"`

	// Target token to predict
	TargetTokenID int32 `json:"target_token_id"`

	// Best seed found during training (32-byte Golden Nonce)
	BestSeed []byte `json:"best_seed,omitempty"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ToBitcoinHeader converts a TrainingFrame to an 80-byte Bitcoin header
func (tf *TrainingFrame) ToBitcoinHeader() []byte {
	header := make([]byte, 80)

	// Version (4 bytes, little-endian)
	binary.LittleEndian.PutUint32(header[0:4], 0x00000002)

	// Previous block hash from slots 0-7 (32 bytes, big-endian)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(header[4+(i*4):], tf.AsicSlots[i])
	}

	// Merkle root from slots 8-11 + padding (32 bytes, big-endian)
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(header[36+(i*4):], tf.AsicSlots[i+8])
	}
	// Remaining 16 bytes of Merkle root are zero (padding)

	// Timestamp (4 bytes, little-endian) - current time
	binary.LittleEndian.PutUint32(header[68:72], uint32(0))

	// Bits (4 bytes, little-endian) - standard difficulty
	binary.LittleEndian.PutUint32(header[72:76], 0x1d00ffff)

	// Nonce (4 bytes, little-endian) - will be set during search
	binary.LittleEndian.PutUint32(header[76:80], 0)

	return header
}

// ComputeDoubleSHA256 computes the double SHA-256 hash of data
func ComputeDoubleSHA256(data []byte) [32]byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second
}
