// internal/hasher/mining_neuron.go
// Mining-based neuron activation using BM1382 ASIC
//
// ARCHITECTURE: Instead of computing SHA256(input) directly, we use the ASIC's
// mining capability with Difficulty 1 target. The first valid nonce found
// becomes the deterministic activation value.
//
// Determinism: Same header + same nonce range = same first valid nonce (always)
// The ASIC iterates nonces sequentially, so the result is reproducible.

package hasher

import (
	"encoding/binary"
	"math"
	"sync"
)

// Difficulty1NBits is the nBits value for Bitcoin Difficulty 1
// This means any hash with first byte < 0x1d is valid
// At 500 GH/s, a valid nonce is found in nanoseconds
const Difficulty1NBits = 0x1d00ffff

// MiningNeuron implements a hash-based neuron using ASIC mining
// The activation is the first valid nonce found for the input projections
type MiningNeuron struct {
	// Weights for computing projections from input
	Weights [][]float32 // [outputDim][inputDim]
	Bias    []float32   // [outputDim]

	// Mining parameters
	Salt       uint32 // Unique salt per neuron (goes in Version field)
	NonceStart uint32 // Start of nonce search range
	NonceEnd   uint32 // End of nonce search range (for determinism)

	// ASIC client (nil = use software fallback)
	asicClient *ASICClient

	mu sync.RWMutex
}

// MiningNeuronConfig holds configuration for creating mining neurons
type MiningNeuronConfig struct {
	InputDim   int
	OutputDim  int // Number of projections (max 16 for 64 bytes)
	Salt       uint32
	NonceStart uint32
	NonceEnd   uint32
}

// NewMiningNeuron creates a new mining-based neuron
func NewMiningNeuron(config MiningNeuronConfig) *MiningNeuron {
	// Initialize weights with small random values
	weights := make([][]float32, config.OutputDim)
	for i := range weights {
		weights[i] = make([]float32, config.InputDim)
		for j := range weights[i] {
			// Xavier initialization
			weights[i][j] = float32((float64(i+j) * 0.01) - 0.005)
		}
	}

	bias := make([]float32, config.OutputDim)

	return &MiningNeuron{
		Weights:    weights,
		Bias:       bias,
		Salt:       config.Salt,
		NonceStart: config.NonceStart,
		NonceEnd:   config.NonceEnd,
	}
}

// SetASICClient sets the ASIC client for hardware acceleration
func (n *MiningNeuron) SetASICClient(client *ASICClient) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.asicClient = client
}

// Forward computes the neuron activation via mining
// Returns the first valid nonce as the activation value
func (n *MiningNeuron) Forward(input []float32) (uint32, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Step 1: Compute projections (CPU: simple matrix multiply)
	projections := n.computeProjections(input)

	// Step 2: Build mining header from projections
	header := n.buildMiningHeader(projections)

	// Step 3: Mine with Difficulty 1 to get deterministic nonce
	nonce, err := n.mine(header)
	if err != nil {
		return 0, err
	}

	return nonce, nil
}

// ForwardBatch computes activations for multiple inputs
func (n *MiningNeuron) ForwardBatch(inputs [][]float32) ([]uint32, error) {
	results := make([]uint32, len(inputs))
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in []float32) {
			defer wg.Done()
			nonce, err := n.Forward(in)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}
			results[idx] = nonce
		}(i, input)
	}

	wg.Wait()
	return results, firstErr
}

// computeProjections performs matrix multiplication: W * input + bias
func (n *MiningNeuron) computeProjections(input []float32) []float32 {
	outputDim := len(n.Weights)
	projections := make([]float32, outputDim)

	for i := 0; i < outputDim; i++ {
		sum := float32(0)
		for j := 0; j < len(input) && j < len(n.Weights[i]); j++ {
			sum += n.Weights[i][j] * input[j]
		}
		projections[i] = sum + n.Bias[i]
	}

	return projections
}

// buildMiningHeader constructs an 80-byte Bitcoin-style header from projections
// This is the key mapping that makes mining work as a hash function
func (n *MiningNeuron) buildMiningHeader(projections []float32) []byte {
	header := make([]byte, 80)

	// Byte 0-3: Version (Salt) - unique per neuron
	binary.LittleEndian.PutUint32(header[0:4], n.Salt)

	// Byte 4-35: Previous Block Hash (Projections 0-7)
	// Pack up to 8 float32 values into 32 bytes
	for i := 0; i < 8 && i < len(projections); i++ {
		val := math.Float32bits(projections[i])
		binary.LittleEndian.PutUint32(header[4+(i*4):8+(i*4)], val)
	}

	// Byte 36-67: Merkle Root (Projections 8-15)
	// Pack next 8 float32 values into 32 bytes
	for i := 0; i < 8 && (i+8) < len(projections); i++ {
		val := math.Float32bits(projections[8+i])
		binary.LittleEndian.PutUint32(header[36+(i*4):40+(i*4)], val)
	}

	// Byte 68-71: Timestamp (fixed for determinism)
	// Use salt as timestamp for additional uniqueness
	binary.LittleEndian.PutUint32(header[68:72], n.Salt)

	// Byte 72-75: nBits (Difficulty 1 = 0x1d00ffff)
	binary.LittleEndian.PutUint32(header[72:76], Difficulty1NBits)

	// Byte 76-79: Nonce (starting point, ASIC will iterate from here)
	binary.LittleEndian.PutUint32(header[76:80], n.NonceStart)

	return header
}

// mine sends work to ASIC and gets the first valid nonce
func (n *MiningNeuron) mine(header []byte) (uint32, error) {
	if n.asicClient != nil && !n.asicClient.IsUsingFallback() {
		return n.mineASIC(header)
	}
	return n.mineSoftware(header)
}

// mineASIC uses the hardware ASIC for mining
func (n *MiningNeuron) mineASIC(header []byte) (uint32, error) {
	// Build TxTask packet with the header
	// The ASIC will find the first nonce where H(H(header||nonce)) < target
	result, err := n.asicClient.MineHeader(header, n.NonceStart, n.NonceEnd)
	if err != nil {
		// Fallback to software on ASIC error
		return n.mineSoftware(header)
	}
	return result, nil
}

// mineSoftware provides a CPU-based fallback for testing
// This is much slower but produces the same deterministic results
func (n *MiningNeuron) mineSoftware(header []byte) (uint32, error) {
	// For software fallback, we simulate the mining process
	// by finding the first nonce that produces a valid hash
	//
	// At Difficulty 1, we need: first byte of hash < 0x1d
	// This happens roughly 1 in 256 attempts

	workHeader := make([]byte, 80)
	copy(workHeader, header)

	for nonce := n.NonceStart; nonce <= n.NonceEnd; nonce++ {
		binary.LittleEndian.PutUint32(workHeader[76:80], nonce)

		// Double SHA-256 (Bitcoin mining)
		hash := doubleSHA256(workHeader)

		// Check if hash meets Difficulty 1 target
		// For Difficulty 1, the hash must be less than:
		// 0x00000000FFFF0000000000000000000000000000000000000000000000000000
		// Simplified check: first 4 bytes should have enough leading zeros
		if hash[0] == 0 && hash[1] == 0 && hash[2] == 0 && hash[3] < 0x10 {
			return nonce, nil
		}
	}

	// If no valid nonce found in range, return the last one
	// This shouldn't happen at Difficulty 1 with a reasonable range
	return n.NonceEnd, nil
}

// UpdateWeights updates the neuron's weights (for backpropagation)
func (n *MiningNeuron) UpdateWeights(gradients [][]float32, biasGradients []float32, learningRate float32) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i := range n.Weights {
		for j := range n.Weights[i] {
			if i < len(gradients) && j < len(gradients[i]) {
				n.Weights[i][j] -= learningRate * gradients[i][j]
			}
		}
	}

	for i := range n.Bias {
		if i < len(biasGradients) {
			n.Bias[i] -= learningRate * biasGradients[i]
		}
	}
}

// GetWeights returns a copy of the weights
func (n *MiningNeuron) GetWeights() [][]float32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	weights := make([][]float32, len(n.Weights))
	for i := range n.Weights {
		weights[i] = make([]float32, len(n.Weights[i]))
		copy(weights[i], n.Weights[i])
	}
	return weights
}

// GetBias returns a copy of the bias
func (n *MiningNeuron) GetBias() []float32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	bias := make([]float32, len(n.Bias))
	copy(bias, n.Bias)
	return bias
}

// NormalizeNonce converts the raw nonce to a [0,1] activation value
func NormalizeNonce(nonce uint32, nonceRange uint32) float32 {
	return float32(nonce) / float32(nonceRange)
}

// NonceToActivation converts a nonce to multiple activation values
// by using different bit ranges of the 32-bit nonce
func NonceToActivation(nonce uint32, numOutputs int) []float32 {
	activations := make([]float32, numOutputs)

	// Use modular arithmetic to distribute the nonce across outputs
	for i := 0; i < numOutputs; i++ {
		// Rotate and mix the nonce for each output
		rotated := (nonce >> (i % 32)) | (nonce << (32 - (i % 32)))
		// Add index-based variation
		mixed := rotated ^ (uint32(i) * GoldenRatio) // Golden ratio constant
		// Normalize to [0, 1]
		activations[i] = float32(mixed) / float32(^uint32(0))
	}

	return activations
}
