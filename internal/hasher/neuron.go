package hasher

import (
	"encoding/binary"
	"crypto/sha256"
)

// HashNeuron represents a single hash neuron in the neural network
// as specified in HASHER_SDD.md section 4.1.2
type HashNeuron struct {
	Seed       [32]byte     // Cryptographic seed (the "weight")
	OutputMode string       // "float" | "binary" | "signed"
}

// NewHashNeuron creates a new hash neuron with the given seed and output mode
func NewHashNeuron(seed [32]byte, outputMode string) *HashNeuron {
	if outputMode == "" {
		outputMode = "float"
	}
	return &HashNeuron{
		Seed:       seed,
		OutputMode: outputMode,
	}
}

// Forward pass implementation for hash neuron
// as specified in HASHER_SDD.md section 4.1.2
func (n *HashNeuron) Forward(input []byte) float64 {
	// Step 1: Concatenate input with seed
	combined := append(input, n.Seed[:]...)
	
	// Step 2: Compute SHA-256
	hash := sha256.Sum256(combined)  // 32 bytes output
	
	// Step 3: Convert to float64 [0, 1]
	// Take first 8 bytes as uint64
	val := binary.BigEndian.Uint64(hash[0:8])
	
	// Normalize to [0, 1]
	return float64(val) / float64(1<<64 - 1)
}

// String representation of the neuron
func (n *HashNeuron) String() string {
	return "HashNeuron"
}
