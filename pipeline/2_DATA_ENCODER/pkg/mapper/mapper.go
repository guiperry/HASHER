package mapper

import (
	"math"
	"math/rand"
)

const (
	// DefaultEmbeddingDim is the expected embedding dimension
	DefaultEmbeddingDim = 768
	// NumFeatures is the number of semantic features to extract
	NumFeatures = 24
	// NumSlots is the number of hardware slots
	NumSlots = 12
)

// Service handles feature mapping from embeddings to hardware slots
type Service struct {
	projectionMatrix [][]float32
	seed             int64
}

// New creates a new mapper service with a deterministic projection matrix
func New(seed int64) *Service {
	m := &Service{
		projectionMatrix: make([][]float32, NumFeatures),
		seed:             seed,
	}

	// Deterministic random projection
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < NumFeatures; i++ {
		m.projectionMatrix[i] = make([]float32, DefaultEmbeddingDim)
		for j := 0; j < DefaultEmbeddingDim; j++ {
			m.projectionMatrix[i][j] = r.Float32()*2 - 1
		}
	}
	return m
}

// MapToSlots converts an embedding vector to 12 hardware slots
func (s *Service) MapToSlots(embedding []float32) [12]uint32 {
	var slots [12]uint32
	intermediate := make([]int16, NumFeatures)

	// 1. Project & Quantize to int16
	for i := 0; i < NumFeatures; i++ {
		var sum float32
		for j := 0; j < DefaultEmbeddingDim; j++ {
			// Safety check for embedding length
			if j < len(embedding) {
				sum += embedding[j] * s.projectionMatrix[i][j]
			}
		}
		// Sigmoid squash to fit int16 range
		normalized := 1.0 / (1.0 + math.Exp(float64(-sum)))
		intermediate[i] = int16((normalized * 65535) - 32768)
	}

	// 2. Bit Packing (2x int16 -> 1x uint32)
	for i := 0; i < NumSlots; i++ {
		high := uint32(uint16(intermediate[i*2]))
		low := uint32(uint16(intermediate[i*2+1]))
		slots[i] = (high << 16) | low
	}

	return slots
}

// GetSeed returns the seed used for the projection matrix
func (s *Service) GetSeed() int64 {
	return s.seed
}
