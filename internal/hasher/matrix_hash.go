package hasher

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"hasher/internal/driver/device" // Import the EBPFDriver package
)

// MatrixHashNeuron implements learnable hash-based neural operations
type MatrixHashNeuron struct {
	Seed       [32]byte // Encoded weight matrix and bias
	InputDim   int
	OutputDim  int
	Activation string      // "hash", "tanh", "sigmoid"
	Decoded    bool        // Whether weights are decoded
	Weights    [][]float32 // Decoded weight matrix
	Bias       []float32   // Decoded bias vector
	AsicDriver       *device.EBPFDriver     // ASIC driver for hardware acceleration
	SoftwareEmulator *SoftwareASICEmulator // Software emulator for algorithmic testing
}

// NewMatrixHashNeuron creates a new hash-based neuron
func NewMatrixHashNeuron(inputDim, outputDim int, activation string, driver *device.EBPFDriver, emulator *SoftwareASICEmulator) *MatrixHashNeuron {
	// Initialize with random seed (will be overwritten during training)
	seed := [32]byte{}
	// randBytes(seed[:]) // Will remove this dummy func later

	return &MatrixHashNeuron{
		Seed:             seed,
		InputDim:         inputDim,
		OutputDim:        outputDim,
		Activation:       activation,
		Decoded:          false,
		AsicDriver:       driver,
		SoftwareEmulator: emulator,
	}
}

// Forward pass with hash-based activation
func (n *MatrixHashNeuron) Forward(input []float32) []float32 {
	if !n.Decoded {
		n.decodeWeights()
	}

	// Matrix multiplication: output = W·input + b
	output := make([]float32, n.OutputDim)
	for i := 0; i < n.OutputDim; i++ {
		sum := n.Bias[i]
		for j := 0; j < n.InputDim; j++ {
			sum += n.Weights[i][j] * input[j]
		}

		// Apply hash-based activation
		output[i] = n.hashActivation(sum, input, i)
	}

	return output
}

// hashActivation applies hash-based non-linearity
func (n *MatrixHashNeuron) hashActivation(value float32, input []float32, neuronIndex int) float32 {
	switch n.Activation {
	case "hash":
		// Construct the 80-byte Bitcoin block header
		// This is a simplified implementation based on HASHER_SDD_UPDATE.md
		// In a full implementation, this would involve careful packing of
		// 'value', 'input', 'neuronIndex', and 'n.Seed' into the header fields.

		header := make([]byte, 80)

		// Example: Pack value into version field (first 4 bytes)
		binary.LittleEndian.PutUint32(header[0:4], math.Float32bits(value))

		// Example: Pack neuronIndex into a part of the header (e.g., bytes 4-8)
		binary.LittleEndian.PutUint32(header[4:8], uint32(neuronIndex))

		// Example: Pack input into remaining parts of the header
		// This is a simplified example. Real packing would depend on the LSH projections.
		// We need to ensure we don't write beyond the 80-byte limit.
		inputBytesLen := len(input) * 4
		if inputBytesLen > 80-8-len(n.Seed) { // 8 bytes used by value and neuronIndex, rest for seed
			inputBytesLen = 80 - 8 - len(n.Seed)
		}
		
		inputBytes := make([]byte, inputBytesLen)
		for i := 0; i < len(input) && (i*4+4) <= inputBytesLen; i++ {
			binary.LittleEndian.PutUint32(inputBytes[i*4:i*4+4], math.Float32bits(input[i]))
		}
		copy(header[8:], inputBytes) // Copy inputBytes

		// Pad with seed (simplified)
		copy(header[len(header)-len(n.Seed):], n.Seed[:])

		// Fixed Difficulty Bits (0x1d00ffff = Difficulty 1)
		binary.LittleEndian.PutUint32(header[72:76], 0x1d00ffff)

		// Send header to ASIC and get nonce
		nonce, err := uint32(0), error(nil)
		if n.AsicDriver != nil {
			nonce, err = n.AsicDriver.ComputeNonceBucket(header)
		} else {
			err = fmt.Errorf("ASIC driver not initialized")
		}
		
		if err != nil {
			fmt.Printf("ASIC nonce computation failed (%v). Attempting software emulation...\n", err)
			if n.SoftwareEmulator != nil {
				nonce, err = n.SoftwareEmulator.ComputeNonce(header)
				if err != nil {
					fmt.Printf("Software emulation failed: %v. Falling back to dummy value.\n", err)
					return float32(neuronIndex) * 0.01 // Fallback if both fail
				}
			} else {
				fmt.Printf("Software emulator not initialized. Falling back to dummy value.\n")
				return float32(neuronIndex) * 0.01 // Fallback if no emulator
			}
		}

		// Convert nonce (uint32) to float32 [0, 1]
		return float32(nonce) / float32(math.MaxUint32)

	case "tanh":
		return float32(math.Tanh(float64(value)))

	case "sigmoid":
		return float32(1.0 / (1.0 + math.Exp(-float64(value))))

	default:
		return value
	}
}

// decodeWeights decodes the weight matrix from seed
func (n *MatrixHashNeuron) decodeWeights() {
	// For now, use simple decoding - can be enhanced with factorization
	n.Weights = make([][]float32, n.OutputDim)
	n.Bias = make([]float32, n.OutputDim)

	// Use seed to generate deterministic weights
	for i := 0; i < n.OutputDim; i++ {
		n.Weights[i] = make([]float32, n.InputDim)
		for j := 0; j < n.InputDim; j++ {
			// Generate weight from seed + position
			data := append(n.Seed[:], byte(i), byte(j))
			hash := sha256.Sum256(data)
			val := binary.BigEndian.Uint64(hash[:8])
			// Convert to [-1, 1] range
			n.Weights[i][j] = (float32(val)/float32(^uint64(0)))*2 - 1
		}

		// Generate bias from seed + output index
		data := append(n.Seed[:], byte(i), 0xFF)
		hash := sha256.Sum256(data)
		val := binary.BigEndian.Uint64(hash[:8])
		n.Bias[i] = (float32(val)/float32(^uint64(0)))*2 - 1
	}

	n.Decoded = true
}

// UpdateWeights updates the neuron's weights and re-encodes to seed
func (n *MatrixHashNeuron) UpdateWeights(newWeights [][]float32, newBias []float32) {
	n.Weights = newWeights
	n.Bias = newBias
	n.Seed = encodeWeightsToSeed(newWeights, newBias)
	n.Decoded = true
}

// encodeWeightsToSeed encodes weight matrix and bias into 32-byte seed
func encodeWeightsToSeed(weights [][]float32, bias []float32) [32]byte {
	// Simple encoding using hash of weights
	data := make([]byte, 0)
	for i := range weights {
		for j := range weights[i] {
			bits := math.Float32bits(weights[i][j])
			data = append(data, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
		}
	}
	for _, v := range bias {
		bits := math.Float32bits(v)
		data = append(data, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
	}

	// Hash the weights to get 32-byte seed
	hash := sha256.Sum256(data)
	var seed [32]byte
	copy(seed[:], hash[:])
	return seed
}


