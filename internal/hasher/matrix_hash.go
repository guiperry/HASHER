package hasher

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
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
}

// NewMatrixHashNeuron creates a new hash-based neuron
func NewMatrixHashNeuron(inputDim, outputDim int, activation string) *MatrixHashNeuron {
	// Initialize with random seed (will be overwritten during training)
	seed := [32]byte{}
	randBytes(seed[:])

	return &MatrixHashNeuron{
		Seed:       seed,
		InputDim:   inputDim,
		OutputDim:  outputDim,
		Activation: activation,
		Decoded:    false,
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
		// Create hash input: value + input slice + neuron index
		data := make([]byte, 4+len(input)*4+4)
		binary.BigEndian.PutUint32(data[0:4], math.Float32bits(value))
		for i, v := range input {
			binary.BigEndian.PutUint32(data[4+i*4:4+i*4+4], math.Float32bits(v))
		}
		binary.BigEndian.PutUint32(data[len(data)-4:], uint32(neuronIndex))

		// Add seed for uniqueness
		data = append(data, n.Seed[:]...)

		// Compute SHA-256
		hash := sha256.Sum256(data)

		// Convert to float32 [0,1]
		hashVal := binary.BigEndian.Uint64(hash[:8])
		return float32(hashVal) / float32(^uint64(0))

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

// Simple random byte generator
func randBytes(b []byte) {
	for i := range b {
		b[i] = byte(i * 7 % 256)
	}
}
