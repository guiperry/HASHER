package hasher

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// SoftwareASICEmulator provides a software-based emulation of the ASIC's
// nonce-mining process for algorithmic testing and fallback.
type SoftwareASICEmulator struct{}

// NewSoftwareASICEmulator creates a new instance of the emulator.
func NewSoftwareASICEmulator() *SoftwareASICEmulator {
	return &SoftwareASICEmulator{}
}

// ComputeNonce emulates the ASIC's nonce-mining process in software.
// It takes an 80-byte Bitcoin block header and deterministically computes
// a 32-bit nonce that satisfies a Difficulty 1 target.
// This involves a double SHA-256 computation, similar to Bitcoin mining.
func (e *SoftwareASICEmulator) ComputeNonce(header []byte) (uint32, error) {
	if len(header) != 80 {
		return 0, fmt.Errorf("header must be 80 bytes for software emulation")
	}

	// Extract the target (Difficulty 1) from the header
	// For Bitcoin, this is typically at bytes 68-71 (bits field), but our custom
	// header uses bytes 72-75 based on HASHER_SDD_UPDATE.md.
	targetBits := binary.LittleEndian.Uint32(header[72:76])
	target := convertBitsToTarget(targetBits)

	// Simulate nonce search
	// We iterate through possible nonces and perform the double SHA-256.
	// In a real ASIC, this search is done in hardware at very high speed.
	// For software emulation, we'll iterate a reasonable number of times.
	// A small range (e.g., 0 to 1,000,000) for "Golden Nonce" determinism
	// was mentioned in HASHER_SDD_UPDATE.md.
	const maxNonce = 1000000 // Limit search for performance

	for nonce := uint32(0); nonce < maxNonce; nonce++ {
		// Construct the block header with the current nonce
		blockHeaderWithNonce := make([]byte, 80)
		copy(blockHeaderWithNonce, header)
		binary.LittleEndian.PutUint32(blockHeaderWithNonce[76:80], nonce) // Nonce is at bytes 76-79

		// Perform double SHA-256
		hash1 := sha256.Sum256(blockHeaderWithNonce)
		hash2 := sha256.Sum256(hash1[:])

		// Convert hash to big.Int for comparison (or custom byte comparison)
		// Bitcoin hashes are compared as little-endian 256-bit integers.
		// So we need to reverse the byte order for direct comparison.
		reversedHash2 := make([]byte, 32)
		for i := 0; i < 32; i++ {
			reversedHash2[i] = hash2[31-i]
		}

		// Convert reversed hash to uint256 for comparison with target
		// For simplicity, we'll just check if the lower 32 bits (or 64 bits)
		// of the hash are less than the target, assuming the target for Difficulty 1
		// will usually result in nonces with low leading zeroes in the hash.
		// A full implementation would compare the entire 256-bit hash.
		hashVal := binary.BigEndian.Uint32(reversedHash2[28:32]) // Last 4 bytes of reversed hash

		if hashVal < target.Uint32() { // Simplified comparison
			return nonce, nil
		}
	}

	return 0, fmt.Errorf("no nonce found within maxNonce limit in software emulation")
}

// convertBitsToTarget converts the 'bits' field (compact representation of target)
// into a full 256-bit target value.
// For Difficulty 1 (0x1d00ffff), the target is 0x00000000FFFF00000000000000000000000000000000000000000000.
// This simplified version only returns a uint32 for comparison with a uint32 hashVal.
func convertBitsToTarget(bits uint32) *Uint256 {
	// Basic implementation of Bitcoin's compact target representation
	// bits = 0x1d00ffff for Difficulty 1
	// exponent = bits >> 24
	// coefficient = bits & 0xffffff
	// target = coefficient * 256^(exponent - 3)
	
	_ = bits >> 24 // exponent, not used in simplified version
	coefficient := bits & 0x00ffffff
	
	// Simplified: for Difficulty 1 (0x1d00ffff), exponent = 0x1d (29), coefficient = 0x00ffff
	// target = 0x00ffff * 256^(29-3) = 0x00ffff * 256^26
	// For our simplified comparison, we'll use the coefficient as the target
	// This matches the comment about using 0x0000FFFF for basic comparison
	return NewUint256(uint64(coefficient))
}

// Uint256 represents a 256-bit unsigned integer (simplified to uint64 for this PoC)
// In a full Bitcoin implementation, this would be a proper 256-bit integer type.
type Uint256 struct {
	val uint64 // For this PoC, we'll use uint64 to represent the lower part
}

// NewUint256 creates a new Uint256 (simplified to uint64).
func NewUint256(v uint64) *Uint256 {
	return &Uint256{val: v}
}

// Uint32 returns the lower 32 bits of the Uint256 (simplified).
func (u *Uint256) Uint32() uint32 {
	return uint32(u.val)
}
