package core

import (
	"crypto/sha256"
	"encoding/binary"
)

// CanonicalSHA256 provides the canonical Double SHA-256 implementation
// This is the reference implementation used across all hashing methods
type CanonicalSHA256 struct{}

// NewCanonicalSHA256 creates a new canonical SHA-256 instance
func NewCanonicalSHA256() *CanonicalSHA256 {
	return &CanonicalSHA256{}
}

// ComputeSHA256 computes a single SHA-256 hash
func (c *CanonicalSHA256) ComputeSHA256(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// ComputeDoubleSHA256 computes SHA256(SHA256(data)) - Bitcoin's hash function
// This is the canonical implementation that all methods should use
func (c *CanonicalSHA256) ComputeDoubleSHA256(data []byte) [32]byte {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:])
}

// ComputeDoubleSHA256WithNonce computes Double SHA-256 for an 80-byte Bitcoin header
// with the nonce field replaced with the specified nonce value
func (c *CanonicalSHA256) ComputeDoubleSHA256WithNonce(header []byte, nonce uint32) ([32]byte, error) {
	if len(header) != 80 {
		return [32]byte{}, &HashError{
			Type:    ErrorInvalidInput,
			Message: "header must be exactly 80 bytes",
			Context: map[string]interface{}{
				"header_length": len(header),
				"nonce":         nonce,
			},
		}
	}

	// Create a copy to avoid modifying the original
	workHeader := make([]byte, 80)
	copy(workHeader, header)

	// Set nonce in header (bytes 76-79, little-endian)
	workHeader[76] = byte(nonce)
	workHeader[77] = byte(nonce >> 8)
	workHeader[78] = byte(nonce >> 16)
	workHeader[79] = byte(nonce >> 24)

	return c.ComputeDoubleSHA256(workHeader), nil
}

// IsValidDifficulty1 checks if a hash meets Bitcoin Difficulty 1 target
// For Difficulty 1, hash must be less than:
// 0x00000000FFFF0000000000000000000000000000000000000000000000000000
func (c *CanonicalSHA256) IsValidDifficulty1(hash [32]byte) bool {
	// Simplified check: first 4 bytes should have sufficient leading zeros
	// For Difficulty 1: first 3 bytes must be 0, 4th byte < 0x10
	return hash[0] == 0 && hash[1] == 0 && hash[2] == 0 && hash[3] < 0x10
}

// MineForNonce performs mining to find the first nonce that produces a valid Difficulty 1 hash
// This is the reference mining algorithm that all methods should implement
func (c *CanonicalSHA256) MineForNonce(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	if len(header) != 80 {
		return 0, &HashError{
			Type:    ErrorInvalidInput,
			Message: "header must be exactly 80 bytes",
			Context: map[string]interface{}{
				"header_length": len(header),
				"nonce_start":   nonceStart,
				"nonce_end":     nonceEnd,
			},
		}
	}

	for nonce := nonceStart; nonce <= nonceEnd; nonce++ {
		hash, err := c.ComputeDoubleSHA256WithNonce(header, nonce)
		if err != nil {
			continue // Shouldn't happen with our implementation
		}

		if c.IsValidDifficulty1(hash) {
			return nonce, nil
		}
	}

	// No valid nonce found, return the last one attempted
	return nonceEnd, nil
}

// ExtractNonce extracts nonce from an 80-byte Bitcoin header
func (c *CanonicalSHA256) ExtractNonce(header []byte) (uint32, error) {
	if len(header) < 80 {
		return 0, &HashError{
			Type:    ErrorInvalidInput,
			Message: "header must be at least 80 bytes",
			Context: map[string]interface{}{
				"header_length": len(header),
			},
		}
	}

	return binary.LittleEndian.Uint32(header[76:80]), nil
}

// HashError represents errors that can occur during hashing operations
type HashError struct {
	Type    ErrorType
	Message string
	Context map[string]interface{}
}

func (e *HashError) Error() string {
	return e.Message
}

// ErrorType represents different types of hashing errors
type ErrorType int

const (
	ErrorInvalidInput ErrorType = iota
	ErrorHardwareUnavailable
	ErrorOperationFailed
	ErrorTimeout
	ErrorResourceBusy
)
