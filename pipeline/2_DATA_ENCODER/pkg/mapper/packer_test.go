package mapper

import (
	"testing"
)

func TestTensorPacker_PackFrame(t *testing.T) {
	// 24 mock signal indices
	indices := make([]int, 24)
	for i := range indices {
		indices[i] = i
	}
	
	tp := NewTensorPacker(indices)
	
	embedding := make([]float32, 768)
	// Set specific values for variance dimensions
	embedding[0] = 1.0  // Max
	embedding[1] = -1.0 // Min
	
	pos := uint8(0x06)    // PROPN
	tense := uint8(0x02)  // Present/Verb mode
	depHash := uint32(0xDEADBEEF)
	memory := []uint32{0x11111111, 0x22222222, 0x33333333}
	tokenPos := uint16(42)

	slots := tp.PackFrame(embedding, pos, tense, depHash, memory, tokenPos)

	// Verify Slot 0 (Identity): Should contain quantized embedding[0] and embedding[1]
	// 1.0 -> 0xFFFF, -1.0 -> 0x0000
	expectedSlot0 := (uint32(0xFFFF) << 16) | uint32(0x0000)
	if slots[0] != expectedSlot0 {
		t.Errorf("Slot 0 (Identity) = 0x%08X; want 0x%08X", slots[0], expectedSlot0)
	}

	// Verify Slot 4 (Grammar): pos | (tense << 8)
	expectedSlot4 := uint32(pos) | (uint32(tense) << 8)
	if slots[4] != expectedSlot4 {
		t.Errorf("Slot 4 (Grammar) = 0x%08X; want 0x%08X", slots[4], expectedSlot4)
	}

	// Verify Slot 5 (Syntax): depHash
	if slots[5] != depHash {
		t.Errorf("Slot 5 (Syntax) = 0x%08X; want 0x%08X", slots[5], depHash)
	}

	// Verify Slot 6-8 (Memory)
	if slots[6] != memory[0] || slots[7] != memory[1] || slots[8] != memory[2] {
		t.Errorf("Memory slots mismatch: 0x%08X, 0x%08X, 0x%08X", slots[6], slots[7], slots[8])
	}

	// Verify Slot 11 (Lock): tokenPos
	if slots[11] != uint32(tokenPos) {
		t.Errorf("Slot 11 (Lock) = 0x%08X; want 0x%08X", slots[11], uint32(tokenPos))
	}
}
