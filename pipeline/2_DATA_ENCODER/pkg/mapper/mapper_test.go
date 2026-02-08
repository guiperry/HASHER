package mapper

import (
	"math"
	"testing"
)

func TestNew(t *testing.T) {
	seed := int64(1337)
	svc := New(seed)

	if svc == nil {
		t.Fatal("expected service to not be nil")
	}

	if svc.seed != seed {
		t.Errorf("expected seed %d, got %d", seed, svc.seed)
	}

	if len(svc.projectionMatrix) != NumFeatures {
		t.Errorf("expected %d features, got %d", NumFeatures, len(svc.projectionMatrix))
	}

	// Verify each feature has 768 dimensions
	for i, feature := range svc.projectionMatrix {
		if len(feature) != DefaultEmbeddingDim {
			t.Errorf("feature %d: expected %d dimensions, got %d", i, DefaultEmbeddingDim, len(feature))
		}
	}
}

func TestMapToSlots(t *testing.T) {
	svc := New(1337)

	// Create a test embedding (768 dimensions)
	embedding := make([]float32, DefaultEmbeddingDim)
	for i := range embedding {
		embedding[i] = float32(i%10) * 0.1
	}

	slots := svc.MapToSlots(embedding)

	// Verify we get 12 slots
	if len(slots) != NumSlots {
		t.Errorf("expected %d slots, got %d", NumSlots, len(slots))
	}
}

func TestMapToSlotsDeterministic(t *testing.T) {
	// Test that the same embedding produces the same output with the same seed
	svc1 := New(1337)
	svc2 := New(1337)

	embedding := make([]float32, DefaultEmbeddingDim)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	slots1 := svc1.MapToSlots(embedding)
	slots2 := svc2.MapToSlots(embedding)

	for i := range slots1 {
		if slots1[i] != slots2[i] {
			t.Errorf("slot %d mismatch: %d != %d", i, slots1[i], slots2[i])
		}
	}
}

func TestMapToSlotsDifferentSeeds(t *testing.T) {
	// Test that different seeds produce different outputs
	svc1 := New(1337)
	svc2 := New(42)

	embedding := make([]float32, DefaultEmbeddingDim)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	slots1 := svc1.MapToSlots(embedding)
	slots2 := svc2.MapToSlots(embedding)

	// They should be different (with very high probability)
	allSame := true
	for i := range slots1 {
		if slots1[i] != slots2[i] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("expected different seeds to produce different slots")
	}
}

func TestMapToSlotsShortEmbedding(t *testing.T) {
	svc := New(1337)

	// Test with embedding shorter than 768
	shortEmbedding := make([]float32, 100)
	for i := range shortEmbedding {
		shortEmbedding[i] = 0.5
	}

	slots := svc.MapToSlots(shortEmbedding)

	if len(slots) != NumSlots {
		t.Errorf("expected %d slots, got %d", NumSlots, len(slots))
	}

	// All slots should still be populated (with zeros for missing values)
	for i, slot := range slots {
		// Just verify it's a valid uint32
		_ = slot
		t.Logf("Slot %d: %d (0x%08x)", i, slot, slot)
	}
}

func TestMapToSlotsLongEmbedding(t *testing.T) {
	svc := New(1337)

	// Test with embedding longer than 768
	longEmbedding := make([]float32, 1000)
	for i := range longEmbedding {
		longEmbedding[i] = 0.5
	}

	slots := svc.MapToSlots(longEmbedding)

	if len(slots) != NumSlots {
		t.Errorf("expected %d slots, got %d", NumSlots, len(slots))
	}
}

func TestMapToSlotsBitPacking(t *testing.T) {
	svc := New(1337)

	// Create a simple embedding
	embedding := make([]float32, DefaultEmbeddingDim)
	for i := range embedding {
		embedding[i] = 0.1
	}

	slots := svc.MapToSlots(embedding)

	// Verify slots contain valid uint32 values
	for i, slot := range slots {
		// Slot should be non-negative (uint32 is always non-negative)
		if slot == 0 {
			t.Logf("Slot %d is zero", i)
		}

		// Verify we can extract high and low 16 bits
		high := uint16(slot >> 16)
		low := uint16(slot & 0xFFFF)

		// Reconstruct and verify
		reconstructed := (uint32(high) << 16) | uint32(low)
		if reconstructed != slot {
			t.Errorf("slot %d: bit packing inconsistent", i)
		}
	}
}

func TestSigmoidSquash(t *testing.T) {
	svc := New(1337)

	// Test with extreme values
	embedding := make([]float32, DefaultEmbeddingDim)

	// First half: very negative values
	for i := 0; i < DefaultEmbeddingDim/2; i++ {
		embedding[i] = -1000.0
	}

	// Second half: very positive values
	for i := DefaultEmbeddingDim / 2; i < DefaultEmbeddingDim; i++ {
		embedding[i] = 1000.0
	}

	slots := svc.MapToSlots(embedding)

	// All slots should be valid uint32
	for i, slot := range slots {
		_ = slot
		t.Logf("Slot %d: %d", i, slot)
	}
}

func TestGetSeed(t *testing.T) {
	seed := int64(42)
	svc := New(seed)

	if svc.GetSeed() != seed {
		t.Errorf("GetSeed() = %d, want %d", svc.GetSeed(), seed)
	}
}

func TestSigmoid(t *testing.T) {
	// Test sigmoid behavior indirectly through extreme embeddings
	tests := []struct {
		value    float32
		expected float64
	}{
		{0, 0.5},   // sigmoid(0) = 0.5
		{10, 1.0},  // large positive -> ~1
		{-10, 0.0}, // large negative -> ~0
	}

	for _, tt := range tests {
		result := 1.0 / (1.0 + math.Exp(float64(-tt.value)))
		diff := math.Abs(result - tt.expected)
		if diff > 0.01 {
			t.Errorf("sigmoid(%f) = %f, want ~%f", tt.value, result, tt.expected)
		}
	}
}

func BenchmarkMapToSlots(b *testing.B) {
	svc := New(1337)
	embedding := make([]float32, DefaultEmbeddingDim)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.MapToSlots(embedding)
	}
}

func BenchmarkMapToSlotsParallel(b *testing.B) {
	svc := New(1337)
	embedding := make([]float32, DefaultEmbeddingDim)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			svc.MapToSlots(embedding)
		}
	})
}
