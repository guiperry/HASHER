package mapper

// TensorPacker orchestrates bit-level placement of metadata into the 12-slot format
type TensorPacker struct {
	signalIndices []int
}

// NewTensorPacker creates a new packer with variance-selected indices for slots 0-3
func NewTensorPacker(signalIndices []int) *TensorPacker {
	return &TensorPacker{
		signalIndices: signalIndices,
	}
}

// PackFrame creates the 12-slot bitmask tensor according to the HASHER-MAPPER spec
func (p *TensorPacker) PackFrame(
	embedding []float32,
	pos uint8,
	tense uint8,
	depHash uint32,
	lastHeaders []uint32,
	tokenPos uint16,
) [12]uint32 {
	var slots [12]uint32

	// Slots 0-3: Identity (Core BGE Dimensions - Variance-Selected)
	// We need 4 uint32 values. If we have 24 signal indices, we can pack 8 of them?
	// The doc says "4 primary slots" in Section 2, and "Slot 0-3" in Section 4.
	// If each slot is 32 bits, and we have 4 slots, that's 128 bits.
	// If we use 8 float32 dimensions quantized to uint16, we get 8 * 16 = 128 bits.
	for i := 0; i < 4; i++ {
		idx1 := p.signalIndices[i*2]
		idx2 := p.signalIndices[i*2+1]
		
		q1 := quantizeFloatToUint16(embedding[idx1])
		q2 := quantizeFloatToUint16(embedding[idx2])
		
		slots[i] = (uint32(q1) << 16) | uint32(q2)
	}

	// Slot 4: Grammar (POS Tag ID, Tense / Mood ID)
	// Bits 0-7: POS Tag ID
	// Bits 8-15: Tense / Mood ID
	slots[4] = uint32(pos) | (uint32(tense) << 8)

	// Slot 5: Syntax (Dependency Link Hash)
	slots[5] = depHash

	// Slots 6-8: Memory (Recursive Summary - Last 10 Headers XORed)
	// We'll take the first 3 values from the recursive summary if available
	for i := 0; i < 3 && i < len(lastHeaders); i++ {
		slots[6+i] = lastHeaders[i]
	}

	// Slot 9: Intent (Flags: Question, Command, Code)
	// Placeholder logic for now
	slots[9] = 0 // Will be populated by future intent classifier

	// Slot 10: Reserved / Unspecified
	slots[10] = 0

	// Slot 11: Lock (Token Position Index)
	// Bits 0-15: Token Position Index
	slots[11] = uint32(tokenPos)

	return slots
}

func quantizeFloatToUint16(val float32) uint16 {
	if val < -1.0 { val = -1.0 }
	if val > 1.0 { val = 1.0 }
	scaled := (val + 1.0) / 2.0 * 65535.0
	return uint16(scaled + 0.5)
}
