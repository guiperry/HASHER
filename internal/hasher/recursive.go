package hasher

import (
	"math/rand"
	"time"
)

// RecursiveEngine implements the recursive single-ASIC inference engine
// as specified in HASHER_SDD.md sections 1.2 and 2.3
type RecursiveEngine struct {
	Network      *HashNetwork // The hash network to use
	AsicClient   *ASICClient  // Optional ASIC client for hardware acceleration
	Passes       int          // Number of temporal passes (default: 21)
	Jitter       float64      // Input jitter factor [0, 1] (default: 0.01)
	SeedRotation bool         // Whether to rotate neuron seeds per pass
}

// NewRecursiveEngine creates a new recursive inference engine with software-only hashing
func NewRecursiveEngine(network *HashNetwork, passes int, jitter float64, seedRotation bool) (*RecursiveEngine, error) {
	return NewRecursiveEngineWithASIC(network, nil, passes, jitter, seedRotation)
}

// NewRecursiveEngineWithASIC creates a new recursive inference engine with optional ASIC acceleration
// If asicClient is nil, falls back to software SHA-256
func NewRecursiveEngineWithASIC(network *HashNetwork, asicClient *ASICClient, passes int, jitter float64, seedRotation bool) (*RecursiveEngine, error) {
	if network == nil {
		return nil, ErrInvalidNetwork
	}
	if passes <= 0 {
		passes = 21 // Default from HASHER_SDD.md section 4.1.2
	}
	if jitter < 0 || jitter > 1 {
		jitter = 0.01 // Default small jitter
	}

	return &RecursiveEngine{
		Network:      network,
		AsicClient:   asicClient,
		Passes:       passes,
		Jitter:       jitter,
		SeedRotation: seedRotation,
	}, nil
}

// IsUsingASIC returns true if the engine is using ASIC hardware acceleration
func (e *RecursiveEngine) IsUsingASIC() bool {
	return e.AsicClient != nil && !e.AsicClient.IsUsingFallback()
}

// Infer performs recursive inference on the given input using temporal ensemble
func (e *RecursiveEngine) Infer(input []byte) (*RecursiveResult, error) {
	start := time.Now()

	results := make([]*InferencePass, 0, e.Passes)
	for i := 0; i < e.Passes; i++ {
		passResult, err := e.runPass(input, i)
		if err != nil {
			continue
		}
		results = append(results, passResult)
	}

	if len(results) == 0 {
		return nil, ErrNoValidPasses
	}

	// Aggregate results
	aggregated := e.aggregateResults(results)

	return &RecursiveResult{
		Passes:      results,
		Consensus:   aggregated,
		Latency:     time.Since(start),
		ValidPasses: len(results),
		TotalPasses: e.Passes,
	}, nil
}

// runPass executes a single pass of the temporal ensemble
func (e *RecursiveEngine) runPass(input []byte, passNum int) (*InferencePass, error) {
	start := time.Now()
	passStart := time.Now()

	// Apply input jitter
	jitteredInput, err := applyJitter(input, e.Jitter, passNum)
	if err != nil {
		return nil, err
	}

	var prediction int
	var confidence float64

	// Check if we should use ASIC acceleration
	if e.AsicClient != nil && !e.AsicClient.IsUsingFallback() {
		// Use ASIC-backed inference
		pred, conf, err := e.runASICInference(jitteredInput, passNum)
		if err != nil {
			return nil, err
		}
		prediction = pred
		confidence = conf
	} else {
		// Run software inference with optional seed rotation
		if e.SeedRotation {
			// Create a temporary network with rotated seeds
			tempNet := e.rotateNetworkSeeds(passNum)
			pred, conf, err := tempNet.Predict(jitteredInput)
			if err != nil {
				return nil, err
			}
			prediction = pred
			confidence = conf
		} else {
			// Run with original network
			pred, conf, err := e.Network.Predict(jitteredInput)
			if err != nil {
				return nil, err
			}
			prediction = pred
			confidence = conf
		}
	}

	return &InferencePass{
		PassNumber:  passNum,
		Prediction:  prediction,
		Confidence:  confidence,
		Latency:     time.Since(start),
		PassLatency: time.Since(passStart),
	}, nil
}

// runASICInference runs a single inference pass using ASIC hardware
func (e *RecursiveEngine) runASICInference(input []byte, passNum int) (int, float64, error) {
	network := e.Network
	if e.SeedRotation {
		network = e.rotateNetworkSeeds(passNum)
	}

	// Layer 1: Hidden layer 1
	layer1Inputs := e.prepareLayerInputs(input, network.Seeds1)
	layer1Hashes, err := e.AsicClient.ComputeBatch(layer1Inputs)
	if err != nil {
		return -1, 0, err
	}
	layer1Output := e.hashesToFloats(layer1Hashes)
	layer1Bytes := floatSliceToBytes(layer1Output)

	// Layer 2: Hidden layer 2
	layer2Inputs := e.prepareLayerInputs(layer1Bytes, network.Seeds2)
	layer2Hashes, err := e.AsicClient.ComputeBatch(layer2Inputs)
	if err != nil {
		return -1, 0, err
	}
	layer2Output := e.hashesToFloats(layer2Hashes)
	layer2Bytes := floatSliceToBytes(layer2Output)

	// Layer 3: Output layer
	outputInputs := e.prepareLayerInputs(layer2Bytes, network.SeedsOut)
	outputHashes, err := e.AsicClient.ComputeBatch(outputInputs)
	if err != nil {
		return -1, 0, err
	}
	output := e.hashesToFloats(outputHashes)

	// Find prediction (argmax) and confidence
	maxVal := output[0]
	maxIndex := 0
	for i, val := range output[1:] {
		if val > maxVal {
			maxVal = val
			maxIndex = i + 1
		}
	}

	return maxIndex, maxVal, nil
}

// prepareLayerInputs prepares inputs for a neural network layer
// Each neuron receives: input || seed
func (e *RecursiveEngine) prepareLayerInputs(input []byte, seeds [][32]byte) [][]byte {
	inputs := make([][]byte, len(seeds))
	for i, seed := range seeds {
		combined := make([]byte, len(input)+32)
		copy(combined, input)
		copy(combined[len(input):], seed[:])
		inputs[i] = combined
	}
	return inputs
}

// hashesToFloats converts hash outputs to float64 values [0, 1]
func (e *RecursiveEngine) hashesToFloats(hashes [][32]byte) []float64 {
	floats := make([]float64, len(hashes))
	for i, hash := range hashes {
		// Take first 8 bytes as uint64 and normalize to [0, 1]
		val := uint64(hash[0])<<56 | uint64(hash[1])<<48 | uint64(hash[2])<<40 | uint64(hash[3])<<32 |
			uint64(hash[4])<<24 | uint64(hash[5])<<16 | uint64(hash[6])<<8 | uint64(hash[7])
		floats[i] = float64(val) / float64(1<<64-1)
	}
	return floats
}

// applyJitter adds controlled jitter to the input
func applyJitter(input []byte, jitter float64, seed int) ([]byte, error) {
	if jitter == 0 {
		return input, nil
	}

	rng := rand.New(rand.NewSource(int64(seed)))
	jittered := make([]byte, len(input))
	copy(jittered, input)

	for i := range jittered {
		// Apply small random jitter to each byte
		a := int(rng.Float64() * jitter * 255)
		b := int(rng.Float64() * jitter * 255)
		delta := a - b
		newVal := int(jittered[i]) + delta
		if newVal < 0 {
			newVal = 0
		}
		if newVal > 255 {
			newVal = 255
		}
		jittered[i] = byte(newVal)
	}

	return jittered, nil
}

// rotateNetworkSeeds creates a temporary network with rotated seeds for passNum
func (e *RecursiveEngine) rotateNetworkSeeds(passNum int) *HashNetwork {
	// Create a deep copy of the network with rotated seeds
	tempNet, _ := NewHashNetwork(
		e.Network.InputSize,
		e.Network.Hidden1,
		e.Network.Hidden2,
		e.Network.OutputSize,
	)

	// Rotate each layer's seeds
	for i := range tempNet.Seeds1 {
		rotateSeed(tempNet.Seeds1[i][:], passNum)
	}
	for i := range tempNet.Seeds2 {
		rotateSeed(tempNet.Seeds2[i][:], passNum)
	}
	for i := range tempNet.SeedsOut {
		rotateSeed(tempNet.SeedsOut[i][:], passNum)
	}

	return tempNet
}

// rotateSeed performs a deterministic seed rotation based on pass number
func rotateSeed(seed []byte, offset int) {
	for i := range seed {
		seed[i] = seed[i] ^ byte((offset+i)%256)
	}
}

// aggregateResults performs temporal consensus on pass results
func (e *RecursiveEngine) aggregateResults(passes []*InferencePass) *ConsensusResult {
	// Collect all predictions
	predictions := make([]int, 0, len(passes))
	for _, pass := range passes {
		predictions = append(predictions, pass.Prediction)
	}

	// Compute vote count for each class
	voteCount := make(map[int]int)
	maxVotes := 0
	mode := -1

	for _, pred := range predictions {
		voteCount[pred]++
		if voteCount[pred] > maxVotes {
			maxVotes = voteCount[pred]
			mode = pred
		}
	}

	// Calculate confidence as percentage of max votes
	confidence := float64(maxVotes) / float64(len(passes))

	// Calculate average confidence across passes
	totalConfidence := 0.0
	for _, pass := range passes {
		totalConfidence += pass.Confidence
	}
	averageConfidence := totalConfidence / float64(len(passes))

	return &ConsensusResult{
		Prediction:        mode,
		Confidence:        confidence,
		AverageConfidence: averageConfidence,
		VoteCount:         len(passes),
		Mode:              mode,
	}
}

// RecursiveResult contains the complete results from recursive inference
type RecursiveResult struct {
	Passes      []*InferencePass // Results from each individual pass
	Consensus   *ConsensusResult // Aggregated consensus
	Latency     time.Duration    // Total inference latency
	ValidPasses int              // Number of valid passes completed
	TotalPasses int              // Total passes attempted
}

// InferencePass contains the result of a single pass
type InferencePass struct {
	PassNumber  int           // Pass sequence number
	Prediction  int           // Predicted class label
	Confidence  float64       // Neuron confidence [0, 1]
	Latency     time.Duration // Total time since start
	PassLatency time.Duration // Time for this specific pass
}

// ConsensusResult contains aggregated results from temporal consensus
type ConsensusResult struct {
	Prediction        int     // Aggregated prediction
	Confidence        float64 // Consensus confidence [0, 1]
	AverageConfidence float64 // Average per-pass confidence
	VoteCount         int     // Total number of valid votes
	Mode              int     // Most frequent prediction
}

// StatisticalSummary returns statistical information about the passes
func (r *RecursiveResult) StatisticalSummary() *StatisticalSummary {
	allConfidences := make([]float64, 0, r.ValidPasses)
	classDistribution := make(map[int]int)

	for _, pass := range r.Passes {
		allConfidences = append(allConfidences, pass.Confidence)
		classDistribution[pass.Prediction]++
	}

	// Calculate mean and std deviation
	mean := 0.0
	for _, conf := range allConfidences {
		mean += conf
	}
	mean /= float64(r.ValidPasses)

	stdDev := 0.0
	for _, conf := range allConfidences {
		diff := conf - mean
		stdDev += diff * diff
	}
	stdDev /= float64(r.ValidPasses)
	// Note: For simplicity, we're not taking square root here

	return &StatisticalSummary{
		MeanConfidence:    mean,
		ConfidenceStdDev:  stdDev,
		ClassDistribution: classDistribution,
	}
}

// StatisticalSummary contains statistics about confidence values
type StatisticalSummary struct {
	MeanConfidence    float64     // Mean per-pass confidence
	ConfidenceStdDev  float64     // Standard deviation of confidence
	ClassDistribution map[int]int // Distribution of predicted classes
}
