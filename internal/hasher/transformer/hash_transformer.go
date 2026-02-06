package transformer

import (
	"hasher/internal/hasher"
	"math"

	"hasher/internal/driver/device"
)

// HasherTransformer implements transformer architecture with hash-based layers
type HasherTransformer struct {
	Config        *TransformerConfig
	Embeddings    []*hasher.MatrixHashNeuron
	PositionalEnc []*hasher.MatrixHashNeuron
	Attention     []*HasherAttentionBlock
	FeedForward   []*hasher.MatrixHashNeuron
	Norm          []*hasher.MatrixHashNeuron
	SeedEncoder   *hasher.MatrixSeedEncoder
	Surrogate     *hasher.SurrogateGradient
	OutputLayer   *hasher.MatrixHashNeuron // Cached output projection layer
	AsicDriver       *device.EBPFDriver     // ASIC driver for hardware acceleration
	SoftwareEmulator *hasher.SoftwareASICEmulator // Software emulator for algorithmic testing
}

// HasherAttentionBlock implements self-attention with hash operations
type HasherAttentionBlock struct {
	QueryWeights  *hasher.MatrixHashNeuron
	KeyWeights    *hasher.MatrixHashNeuron
	ValueWeights  *hasher.MatrixHashNeuron
	OutputWeights *hasher.MatrixHashNeuron
	NumHeads      int
	HeadDim       int
	SeedEncoder   *hasher.MatrixSeedEncoder
	Surrogate     *hasher.SurrogateGradient
}

// TransformerConfig defines model architecture
type TransformerConfig struct {
	VocabSize    int
	EmbedDim     int
	NumLayers    int
	NumHeads     int
	ContextLen   int
	DropoutRate  float32
	FFNHiddenDim int
	Activation   string // "hash", "tanh", "sigmoid"
}

// NewHasherTransformer creates a new hash-based transformer
func NewHasherTransformer(config *TransformerConfig, driver *device.EBPFDriver, emulator *hasher.SoftwareASICEmulator) *HasherTransformer {
	seedEncoder := hasher.NewMatrixSeedEncoder()
	surrogate := hasher.NewSurrogateGradient("ste") // Straight-through estimator

	model := &HasherTransformer{
		Config:           config,
		SeedEncoder:      seedEncoder,
		Surrogate:        surrogate,
		AsicDriver:       driver,
		SoftwareEmulator: emulator,
	}

	// Initialize embeddings
	model.Embeddings = make([]*hasher.MatrixHashNeuron, config.VocabSize)
	for i := 0; i < config.VocabSize; i++ {
		model.Embeddings[i] = hasher.NewMatrixHashNeuron(1, config.EmbedDim, config.Activation, driver, emulator)
	}

	// Initialize positional encoding (hash-based)
	model.PositionalEnc = make([]*hasher.MatrixHashNeuron, config.ContextLen)
	for i := 0; i < config.ContextLen; i++ {
		model.PositionalEnc[i] = hasher.NewMatrixHashNeuron(1, config.EmbedDim, config.Activation, driver, emulator)
	}

	// Initialize transformer layers
	model.Attention = make([]*HasherAttentionBlock, config.NumLayers)
	model.FeedForward = make([]*hasher.MatrixHashNeuron, config.NumLayers)
	model.Norm = make([]*hasher.MatrixHashNeuron, config.NumLayers*2) // Pre-norm and post-norm

	for i := 0; i < config.NumLayers; i++ {
		// Self-attention block
		model.Attention[i] = NewHasherAttentionBlock(config, seedEncoder, surrogate, driver, emulator)

		// Feed-forward network
		model.FeedForward[i] = hasher.NewMatrixHashNeuron(
			config.EmbedDim,
			config.FFNHiddenDim,
			config.Activation,
			driver,
			emulator,
		)

		// Layer normalization (pre and post)
		model.Norm[i*2] = hasher.NewMatrixHashNeuron(
			config.EmbedDim,
			config.EmbedDim,
			"hash",
			driver,
			emulator,
		)
		model.Norm[i*2+1] = hasher.NewMatrixHashNeuron(
			config.EmbedDim,
			config.EmbedDim,
			"hash",
			driver,
			emulator,
		)
	}

	return model
}

// NewHasherAttentionBlock creates a new hash-based attention block
func NewHasherAttentionBlock(config *TransformerConfig, seedEncoder *hasher.MatrixSeedEncoder, surrogate *hasher.SurrogateGradient, driver *device.EBPFDriver, emulator *hasher.SoftwareASICEmulator) *HasherAttentionBlock {
	headDim := config.EmbedDim / config.NumHeads

	return &HasherAttentionBlock{
		QueryWeights:  hasher.NewMatrixHashNeuron(config.EmbedDim, config.EmbedDim, config.Activation, driver, emulator),
		KeyWeights:    hasher.NewMatrixHashNeuron(config.EmbedDim, config.EmbedDim, config.Activation, driver, emulator),
		ValueWeights:  hasher.NewMatrixHashNeuron(config.EmbedDim, config.EmbedDim, config.Activation, driver, emulator),
		OutputWeights: hasher.NewMatrixHashNeuron(config.EmbedDim, config.EmbedDim, config.Activation, driver, emulator),
		NumHeads:      config.NumHeads,
		HeadDim:       headDim,
		SeedEncoder:   seedEncoder,
		Surrogate:     surrogate,
	}
}

// Forward performs forward pass through the transformer
func (ht *HasherTransformer) Forward(tokenIDs []int) []float32 {
	seqLen := len(tokenIDs)
	embedDim := ht.Config.EmbedDim

	// Handle empty context - return zero embedding
	if seqLen == 0 {
		return make([]float32, embedDim)
	}

	// Embedding lookup
	embeddings := make([][]float32, seqLen)
	for i, tokenID := range tokenIDs {
		input := []float32{float32(tokenID)}
		embeddings[i] = ht.Embeddings[tokenID].Forward(input)
	}

	// Add positional encoding
	for i := 0; i < seqLen; i++ {
		posInput := []float32{float32(i)}
		posEnc := ht.PositionalEnc[i].Forward(posInput)

		// Add positional encoding to embeddings
		for j := 0; j < embedDim; j++ {
			embeddings[i][j] += posEnc[j]
		}
	}

	// Process through transformer layers
	hidden := embeddings
	for i := 0; i < ht.Config.NumLayers; i++ {
		hidden = ht.forwardLayer(hidden, i)
	}

	// Global average pooling for final representation
	finalEmbed := make([]float32, embedDim)
	for j := 0; j < embedDim; j++ {
		sum := float32(0)
		for i := 0; i < seqLen; i++ {
			sum += hidden[i][j]
		}
		finalEmbed[j] = sum / float32(seqLen)
	}

	return finalEmbed
}

// forwardLayer processes a single transformer layer
func (ht *HasherTransformer) forwardLayer(hidden [][]float32, layerIdx int) [][]float32 {
	seqLen := len(hidden)
	embedDim := ht.Config.EmbedDim

	// Pre-layer normalization
	hidden = ht.applyLayerNorm(hidden, layerIdx*2)

	// Self-attention
	attention := ht.Attention[layerIdx].Forward(hidden)

	// Residual connection + post-layer norm
	for i := 0; i < seqLen; i++ {
		for j := 0; j < embedDim; j++ {
			hidden[i][j] += attention[i][j]
		}
	}
	hidden = ht.applyLayerNorm(hidden, layerIdx*2+1)

	// Feed-forward network
	ffnOutput := make([][]float32, seqLen)
	for i := 0; i < seqLen; i++ {
		ffnOutput[i] = ht.FeedForward[layerIdx].Forward(hidden[i])
	}

	// Residual connection
	for i := 0; i < seqLen; i++ {
		for j := 0; j < embedDim; j++ {
			hidden[i][j] += ffnOutput[i][j]
		}
	}

	return hidden
}

// applyLayerNorm applies hash-based layer normalization
func (ht *HasherTransformer) applyLayerNorm(hidden [][]float32, normIdx int) [][]float32 {
	seqLen := len(hidden)
	embedDim := ht.Config.EmbedDim

	// Compute mean and variance for each position
	for i := 0; i < seqLen; i++ {
		// Calculate mean
		mean := float32(0)
		for j := 0; j < embedDim; j++ {
			mean += hidden[i][j]
		}
		mean /= float32(embedDim)

		// Calculate variance
		variance := float32(0)
		for j := 0; j < embedDim; j++ {
			diff := hidden[i][j] - mean
			variance += diff * diff
		}
		variance /= float32(embedDim)
		stddev := float32(math.Sqrt(float64(variance)))

		// Normalize the entire sequence at once using hash-based layer norm
		normalizedInput := make([]float32, embedDim)
		for j := 0; j < embedDim; j++ {
			normalizedInput[j] = (hidden[i][j] - mean) / (stddev + 1e-6)
		}
		normalizedOutput := ht.Norm[normIdx].Forward(normalizedInput)
		for j := 0; j < embedDim; j++ {
			hidden[i][j] = normalizedOutput[j]*(stddev+1e-6) + mean
		}
	}

	return hidden
}

// ForwardAttention implements self-attention forward pass
func (hab *HasherAttentionBlock) Forward(hidden [][]float32) [][]float32 {
	seqLen := len(hidden)
	if seqLen == 0 {
		return make([][]float32, 0)
	}
	embedDim := len(hidden[0])
	headDim := hab.HeadDim
	numHeads := hab.NumHeads

	// Multi-head attention computation
	output := make([][]float32, seqLen)
	for i := 0; i < seqLen; i++ {
		output[i] = make([]float32, embedDim)

		for head := 0; head < numHeads; head++ {
			// Compute queries, keys, values
			q := hab.QueryWeights.Forward(hidden[i])
			k := hab.KeyWeights.Forward(hidden[i])
			v := hab.ValueWeights.Forward(hidden[i])

			// Head output (simplified attention)
			headStart := head * headDim

			// Dot product attention (simplified)
			for j := 0; j < headDim; j++ {
				if headStart+j < embedDim && headStart+j < len(q) {
					output[i][headStart+j] = q[headStart+j] * k[headStart+j] * v[headStart+j]
				}
			}
		}

		// Final output projection
		output[i] = hab.OutputWeights.Forward(output[i])
	}

	return output
}

// Backward performs backward pass with surrogate gradients
func (ht *HasherTransformer) Backward(upstreamGrad []float32, learningRate float32) {
	// Simplified backward pass - would need full implementation for training
	// This is a placeholder showing the concept

	for i := range ht.FeedForward {
		// Compute gradients for feed-forward layer
		grads, biasGrads := ht.Surrogate.ComputeGradients(
			ht.FeedForward[i],
			[]float32{0}, // Simplified input
			upstreamGrad,
		)

		// Update weights (simplified)
		ht.updateLayerWeights(ht.FeedForward[i], grads, biasGrads, learningRate)
	}

	// Similar updates for attention layers and layer norm
}

// updateLayerWeights updates layer weights using gradients
func (ht *HasherTransformer) updateLayerWeights(layer *hasher.MatrixHashNeuron, weightGrads [][]float32, biasGrads []float32, learningRate float32) {
	// Get current weights
	weights, bias := ht.SeedEncoder.DecodeMatrix(layer.Seed, layer.InputDim, layer.OutputDim)

	// Update weights with gradients
	for i := range weights {
		for j := range weights[i] {
			weights[i][j] -= learningRate * weightGrads[i][j]
		}
	}
	for i := range bias {
		bias[i] -= learningRate * biasGrads[i]
	}

	// Re-encode updated weights to seed
	layer.UpdateWeights(weights, bias)
}

// GenerateToken generates next token using hash-based transformer, returning the chosen token ID and all token scores.
func (ht *HasherTransformer) GenerateToken(context []int, temperature float32) (int, []float32) {
	// Forward pass
	hidden := ht.Forward(context)

	// Initialize output layer if not exists (lazy initialization)
	if ht.OutputLayer == nil {
		ht.OutputLayer = hasher.NewMatrixHashNeuron(len(hidden)+1, ht.Config.VocabSize, "hash", ht.AsicDriver, ht.SoftwareEmulator)
	}

	// Create input with bias token
	fullInput := make([]float32, len(hidden)+1)
	copy(fullInput, hidden)
	fullInput[len(hidden)] = 0.0 // Add bias

	// Project to vocabulary using matrix multiplication instead of per-token loops
	outputs := ht.OutputLayer.Forward(fullInput)

	// Extract the first output (simplified - for now use first row)
	tokenScores := make([]float32, ht.Config.VocabSize)
	for i := 0; i < ht.Config.VocabSize && i < len(outputs); i++ {
		tokenScores[i] = outputs[i]
	}
	for i := len(outputs); i < ht.Config.VocabSize; i++ {
		tokenScores[i] = float32(0) // Fill remaining vocab with zeros
	}

	// Apply temperature and sample
	if temperature > 0 {
		// Softmax with temperature
		maxScore := float32(-math.MaxFloat32)
		for _, score := range tokenScores {
			if score > maxScore {
				maxScore = score
			}
		}

		// Compute softmax probabilities
		probs := make([]float32, len(tokenScores))
		sum := float32(0)
		for i, score := range tokenScores {
			probs[i] = float32(math.Exp(float64((score - maxScore) / temperature)))
			sum += probs[i]
		}
		for i := range probs {
			probs[i] /= sum
		}

		// Sample from distribution
		// For now, use simple deterministic selection
		// In production, use proper random sampling
		randVal := float32(0.5) // Placeholder for deterministic behavior
		cumsum := float32(0)
		for i, prob := range probs {
			cumsum += prob
			if randVal < cumsum {
				return i, tokenScores
			}
		}
		return 0, tokenScores // Fallback
	}

	// Argmax for temperature = 0
	maxIdx := 0
	maxScore := tokenScores[0]
	for i, score := range tokenScores {
		if score > maxScore {
			maxScore = score
			maxIdx = i
		}
	}
	return maxIdx, tokenScores
}
