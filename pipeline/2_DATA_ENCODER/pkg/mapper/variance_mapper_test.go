package mapper

import (
	"testing"
)

func TestVarianceMapperBasic(t *testing.T) {
	// Create a test embedding (768 dimensions)
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = float32(i%10) / 10.0 // Values 0.0 to 0.9
	}

	vm := NewDefaultVarianceMapper()
	slots := vm.MapToSlots(embedding)

	// Verify we get 12 slots
	if len(slots) != 12 {
		t.Errorf("expected 12 slots, got %d", len(slots))
	}

	// Verify slots are not all zero (some should have values)
	nonZero := 0
	for _, slot := range slots {
		if slot != 0 {
			nonZero++
		}
	}

	if nonZero == 0 {
		t.Error("expected some non-zero slots")
	}
}

func TestVarianceMapperDeterministic(t *testing.T) {
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = float32(i * 17 % 100 / 100.0) // Deterministic pattern
	}

	vm := NewDefaultVarianceMapper()
	slots1 := vm.MapToSlots(embedding)
	slots2 := vm.MapToSlots(embedding)

	// Same input should produce same output
	for i := range slots1 {
		if slots1[i] != slots2[i] {
			t.Errorf("slots not deterministic at index %d: %d != %d", i, slots1[i], slots2[i])
		}
	}
}

func TestVarianceMapperCustomIndices(t *testing.T) {
	// Use first 24 indices as signal indices
	customIndices := make([]int, 24)
	for i := range customIndices {
		customIndices[i] = i
	}

	vm := NewVarianceMapper(customIndices)

	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = 0.5 // All same values
	}

	slots := vm.MapToSlots(embedding)

	// With all same input values, all slots should have similar values
	for i, slot := range slots {
		if slot == 0 {
			t.Errorf("slot %d should not be zero", i)
		}
	}
}

func TestVarianceMapperPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with wrong number of indices")
		}
	}()

	// Should panic with wrong number of indices
	badIndices := make([]int, 10)
	NewVarianceMapper(badIndices)
}

func TestVarianceMapperEmbeddingSize(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with wrong embedding size")
		}
	}()

	vm := NewDefaultVarianceMapper()
	smallEmbedding := make([]float32, 100)

	vm.MapToSlots(smallEmbedding)
}
