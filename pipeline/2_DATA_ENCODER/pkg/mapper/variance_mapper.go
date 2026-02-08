package mapper

import "fmt"

// BGE_SIGNAL_INDICES contains the 24 indices with highest variance from BGE-Base model analysis
// These indices represent the most "informative" dimensions in the embedding space
// By focusing on high-variance dimensions, we maximize the signal in the 12-slot ASIC frame
// TODO: Run variance_analyzer utility on your dataset to generate these indices
var BGE_SIGNAL_INDICES = []int{
	// Default placeholder indices - REPLACE with actual output from variance_analyzer
	// Example format: {12, 45, 89, 112, 203, 301, 333, 412, 506, 555, 612, 700,
	//                 14, 48, 92, 115, 205, 304, 336, 415, 509, 558, 615, 703}
	// These should be replaced with real variance-analyzed indices from your data
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
}

// VarianceMapper provides high-signal embedding to ASIC slots mapping
type VarianceMapper struct {
	signalIndices []int
}

// NewVarianceMapper creates a new VarianceMapper with configurable signal indices
func NewVarianceMapper(signalIndices []int) *VarianceMapper {
	if len(signalIndices) != 24 {
		panic(fmt.Sprintf("VarianceMapper requires exactly 24 signal indices, got %d", len(signalIndices)))
	}
	return &VarianceMapper{
		signalIndices: signalIndices,
	}
}

// NewDefaultVarianceMapper creates a mapper with default BGE signal indices
func NewDefaultVarianceMapper() *VarianceMapper {
	indices := make([]int, len(BGE_SIGNAL_INDICES))
	copy(indices, BGE_SIGNAL_INDICES)
	return &VarianceMapper{
		signalIndices: indices,
	}
}

// MapToSlots converts a 768-dimensional embedding to 12 ASIC-ready uint32 slots
// Uses only the high-variance dimensions for maximum signal preservation
func (v *VarianceMapper) MapToSlots(embedding []float32) [12]uint32 {
	if len(embedding) != 768 {
		panic(fmt.Sprintf("Embedding must be 768 dimensions, got %d", len(embedding)))
	}

	var slots [12]uint32
	for i := 0; i < 12; i++ {
		// Extract two high-variance dimensions per slot
		idx1 := v.signalIndices[i*2]
		idx2 := v.signalIndices[i*2+1]

		v1 := embedding[idx1]
		v2 := embedding[idx2]

		// Quantize float32 [-1.0, 1.0] to uint16 [0, 65535]
		// BGE embeddings are L2 normalized, typically in range [-1, 1]
		q1 := quantizeFloatToUint16(v1)
		q2 := quantizeFloatToUint16(v2)

		// Pack two uint16 values into one uint32 (16 bits each)
		slots[i] = (uint32(q1) << 16) | uint32(q2)
	}
	return slots
}

// quantizeFloatToUint16 converts a float32 in range [-1.0, 1.0] to uint16 [0, 65535]
func quantizeFloatToUint16(val float32) uint16 {
	// Clamp to valid range
	if val < -1.0 {
		val = -1.0
	}
	if val > 1.0 {
		val = 1.0
	}

	// Scale from [-1.0, 1.0] to [0, 65535]
	// Formula: (val + 1.0) / 2.0 * 65535
	scaled := (val + 1.0) / 2.0 * 65535.0

	return uint16(scaled + 0.5) // Round to nearest
}

// MapToSlotsRaw provides raw uint32 values for direct hardware access
// Returns slots as-is without the high-signal filtering
func MapToSlotsRaw(embedding []float32) [12]uint32 {
	var mapper = NewDefaultVarianceMapper()
	return mapper.MapToSlots(embedding)
}
