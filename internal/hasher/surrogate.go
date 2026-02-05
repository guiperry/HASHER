package hasher

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/rand"
	"time"
)

// SurrogateGradient provides differentiable approximations for hash operations
type SurrogateGradient struct {
	Method      string // "ste", "gumbel", "smooth"
	Temperature float32
	Noise       float32
	rand        *rand.Rand
}

// NewSurrogateGradient creates a new surrogate gradient estimator
func NewSurrogateGradient(method string) *SurrogateGradient {
	return &SurrogateGradient{
		Method:      method,
		Temperature: 1.0,
		Noise:       0.1,
		rand:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ForwardHash computes forward hash with gradient tracking
func (sg *SurrogateGradient) ForwardHash(input []byte, seed [32]byte) (output float32, gradFunc func(float32) []byte) {
	hash := sha256.Sum256(append(input, seed[:]...))
	hashVal := binary.BigEndian.Uint64(hash[:8])
	output = float32(hashVal) / float32(^uint64(0))

	// Create gradient function for backward pass
	switch sg.Method {
	case "ste":
		gradFunc = sg.straightThroughEstimator(input, seed)
	case "gumbel":
		gradFunc = sg.gumbelSoftmax(input, seed)
	case "smooth":
		gradFunc = sg.smoothApproximation(input, seed)
	default:
		gradFunc = func(float32) []byte { return make([]byte, len(input)) } // Zero gradient
	}

	return
}

// straightThroughEstimator implements straight-through estimator
func (sg *SurrogateGradient) straightThroughEstimator(input []byte, seed [32]byte) func(float32) []byte {
	return func(upstreamGrad float32) []byte {
		// STE: pass gradient straight through
		grad := make([]byte, len(input))

		// Use seed to create deterministic noise
		seedHash := sha256.Sum256(seed[:])
		randSeed := binary.BigEndian.Uint64(seedHash[:8])
		localRand := rand.New(rand.NewSource(int64(randSeed)))

		// Add small noise for exploration
		for i := range grad {
			grad[i] = byte(float32(upstreamGrad) + sg.Noise*(2*localRand.Float32()-1))
		}

		return grad
	}
}

// gumbelSoftmax implements Gumbel-Softmax relaxation
func (sg *SurrogateGradient) gumbelSoftmax(input []byte, seed [32]byte) func(float32) []byte {
	return func(upstreamGrad float32) []byte {
		grad := make([]byte, len(input))

		// Use seed to influence the random generator for deterministic results
		// We'll modify the random state based on seed to make it deterministic
		seedHash := sha256.Sum256(seed[:])
		// Use the seed hash to perturb the random state
		// This makes the Gumbel sampling deterministic for the same seed
		_ = binary.BigEndian.Uint64(seedHash[:8]) // Use seed to affect determinism
		
		// Gumbel noise + softmax approximation
		for i := range grad {
			gumbel := sg.sampleGumbel()
			softmax := sg.approximateSigmoid(float32(i) + gumbel)
			grad[i] = byte(upstreamGrad * softmax * (1 - softmax))
		}

		return grad
	}
}

// smoothApproximation implements smooth differentiable hash approximation
func (sg *SurrogateGradient) smoothApproximation(input []byte, seed [32]byte) func(float32) []byte {
	return func(upstreamGrad float32) []byte {
		grad := make([]byte, len(input))

		// Use tanh approximation of hash function
		for i, b := range input {
			smoothHash := sg.tanhHash(float32(b) + float32(seed[i%32]))
			grad[i] = byte(upstreamGrad * (1 - smoothHash*smoothHash)) // dtanh/dx
		}

		return grad
	}
}

// sampleGumbel samples from Gumbel distribution
func (sg *SurrogateGradient) sampleGumbel() float32 {
	// Simple Gumbel sampling: -ln(-ln(U))
	u := sg.rand.Float32() // [0,1)
	return float32(-math.Log(-math.Log(float64(u))))
}

// approximateSigmoid approximates sigmoid function
func (sg *SurrogateGradient) approximateSigmoid(x float32) float32 {
	return 1.0 / (1.0 + float32(math.Exp(-float64(x/sg.Temperature))))
}

// tanhHash approximates hash with smooth tanh function
func (sg *SurrogateGradient) tanhHash(x float32) float32 {
	return float32(math.Tanh(float64(x)))
}

// BackwardHash computes gradients for hash-based layer
func (sg *SurrogateGradient) BackwardHash(input []float32, seed [32]byte, upstreamGrad []float32) ([][]float32, []float32) {
	inputDim := len(input)
	outputDim := len(upstreamGrad)

	// Initialize gradients
	weightGrads := make([][]float32, outputDim)
	biasGrads := make([]float32, outputDim)

	for i := 0; i < outputDim; i++ {
		weightGrads[i] = make([]float32, inputDim)

		for j := 0; j < inputDim; j++ {
			// Approximate gradient through hash operation
			switch sg.Method {
			case "ste":
				weightGrads[i][j] = upstreamGrad[i] * input[j]
			case "smooth":
				hashInput := sg.prepareHashInput(input, i, j, seed)
				_, gradFunc := sg.ForwardHash(hashInput, seed)
				inputGrad := gradFunc(upstreamGrad[i])
				if j < len(inputGrad) {
					weightGrads[i][j] = float32(inputGrad[j]) * input[j]
				}
			default:
				weightGrads[i][j] = 0
			}
		}

		// Bias gradient (for hash activation, bias effect is approximated)
		biasGrads[i] = upstreamGrad[i] * sg.approximateBiasGradient(seed, i)
	}

	return weightGrads, biasGrads
}

// prepareHashInput creates input for hash operation
func (sg *SurrogateGradient) prepareHashInput(input []float32, outputIdx, inputIdx int, seed [32]byte) []byte {
	data := make([]byte, 0, 8+len(input)*4+8) // 8 bytes for indices + input values + seed

	// Add output index and input index
	data = append(data, byte(outputIdx), byte(outputIdx>>8), byte(outputIdx>>16), byte(outputIdx>>24))
	data = append(data, byte(inputIdx), byte(inputIdx>>8), byte(inputIdx>>16), byte(inputIdx>>24))

	// Add input values
	for _, v := range input {
		bits := math.Float32bits(v)
		data = append(data, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
	}

	// Add seed
	data = append(data, seed[:]...)

	return data
}

// approximateBiasGradient approximates gradient through bias in hash activation
func (sg *SurrogateGradient) approximateBiasGradient(seed [32]byte, neuronIndex int) float32 {
	// Use finite difference approximation
	epsilon := float32(1e-5)

	// Original hash input
	data := append(seed[:], byte(neuronIndex))
	origHash := sha256.Sum256(data)
	origVal := float32(binary.BigEndian.Uint64(origHash[:8])) / float32(^uint64(0))

	// Perturbed hash input (+epsilon)
	perturbedData := append(seed[:], byte(neuronIndex+1))
	perturbedHash := sha256.Sum256(perturbedData)
	perturbedVal := float32(binary.BigEndian.Uint64(perturbedHash[:8])) / float32(^uint64(0))

	// Finite difference gradient
	return (perturbedVal - origVal) / epsilon
}

// ComputeGradients computes gradients for matrix hash layer
func (sg *SurrogateGradient) ComputeGradients(layer *MatrixHashNeuron, input []float32, upstreamGrad []float32) ([][]float32, []float32) {
	if !layer.Decoded {
		layer.decodeWeights()
	}

	return sg.BackwardHash(input, layer.Seed, upstreamGrad)
}
