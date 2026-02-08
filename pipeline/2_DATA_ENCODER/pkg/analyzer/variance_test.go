package analyzer

import (
	"os"
	"testing"
)

func TestVarianceAnalyzerBasic(t *testing.T) {
	va := NewVarianceAnalyzer()

	// Add some test embeddings
	for i := 0; i < 100; i++ {
		embedding := make([]float32, 768)
		for j := range embedding {
			// Create variance: some dimensions vary more than others
			if j < 384 {
				embedding[j] = float32(i) * 0.01 // High variance
			} else {
				embedding[j] = 0.1 + float32(i%2)*0.01 // Low variance
			}
		}

		if err := va.Sample(embedding); err != nil {
			t.Fatalf("sample failed: %v", err)
		}
	}

	// Calculate variance
	if err := va.Calculate(); err != nil {
		t.Fatalf("calculate failed: %v", err)
	}

	// Get signal indices
	indices := va.GetSignalIndices()
	if len(indices) != 24 {
		t.Errorf("expected 24 indices, got %d", len(indices))
	}

	// First indices should be from the high-variance region (0-383)
	highVarianceCount := 0
	for _, idx := range indices {
		if idx < 384 {
			highVarianceCount++
		}
	}

	// Most indices should be from high-variance region
	if highVarianceCount < 20 {
		t.Errorf("expected most indices from high-variance region, got %d/%d", highVarianceCount, len(indices))
	}
}

func TestVarianceAnalyzerNoSamples(t *testing.T) {
	va := NewVarianceAnalyzer()

	// Should return defaults without samples
	indices := va.GetSignalIndices()
	if len(indices) != 24 {
		t.Errorf("expected 24 indices, got %d", len(indices))
	}
}

func TestVarianceAnalyzerStats(t *testing.T) {
	va := NewVarianceAnalyzer()

	// Add uniform embeddings (all same value)
	for i := 0; i < 50; i++ {
		embedding := make([]float32, 768)
		for j := range embedding {
			embedding[j] = 0.5
		}
		va.Sample(embedding)
	}

	va.Calculate()

	stats, err := va.GetStats()
	if err != nil {
		t.Fatalf("getStats failed: %v", err)
	}

	if stats.SampleCount != 50 {
		t.Errorf("expected 50 samples, got %d", stats.SampleCount)
	}

	// Variance should be near zero for uniform data
	if stats.MeanVariance > 0.001 {
		t.Errorf("expected near-zero variance for uniform data, got %f", stats.MeanVariance)
	}
}

func TestVarianceAnalyzerSampleFromRecord(t *testing.T) {
	va := NewVarianceAnalyzer()

	// Test with []float32
	embedding1 := make([]float32, 768)
	for i := range embedding1 {
		embedding1[i] = float32(i % 10)
	}

	if err := va.SampleFromRecord(embedding1); err != nil {
		t.Fatalf("sampleFromRecord []float32 failed: %v", err)
	}

	// Test with []interface{}
	embedding2 := make([]interface{}, 768)
	for i := range embedding2 {
		embedding2[i] = float64(i % 10)
	}

	if err := va.SampleFromRecord(embedding2); err != nil {
		t.Fatalf("sampleFromRecord []interface{} failed: %v", err)
	}

	if va.sampleCount != 2 {
		t.Errorf("expected 2 samples, got %d", va.sampleCount)
	}
}

func TestFindTopVarianceIndices(t *testing.T) {
	// Create variances with known pattern
	variances := make([]float32, 768)
	for i := range variances {
		if i < 100 {
			variances[i] = float32(100 - i) // Decreasing: 100, 99, ..., 1
		} else {
			variances[i] = float32(i - 99) // Increasing: 1, 2, ..., 648
		}
	}

	indices := FindTopVarianceIndices(variances, 10)

	// With this pattern:
	// - Indices 0-99 have variance 100, 99, ..., 1
	// - Indices 100-767 have variance 1, 2, ..., 668
	// So highest variance values are in the high index range (668 > 100)
	highIndexCount := 0
	for _, idx := range indices {
		if idx >= 100 {
			highIndexCount++
		}
	}

	// Most indices should be from high index region (100-767) since they have higher variance
	if highIndexCount < 8 {
		t.Errorf("expected most indices from high index region (100-767), got %d/%d from that region", highIndexCount, len(indices))
	}

	// Verify indices are unique
	seen := make(map[int]bool)
	for _, idx := range indices {
		if seen[idx] {
			t.Errorf("duplicate index found: %d", idx)
		}
		seen[idx] = true
	}
}

func TestVarianceAnalyzerSaveLoad(t *testing.T) {
	// Create analyzer with samples
	va1 := NewVarianceAnalyzer()
	for i := 0; i < 20; i++ {
		embedding := make([]float32, 768)
		for j := range embedding {
			embedding[j] = float32(i*j) * 0.001
		}
		va1.Sample(embedding)
	}
	va1.Calculate()

	// Save to temp file
	tmpFile := "/tmp/test_variance_indices.json"
	if err := va1.SaveToFile(tmpFile, "test-model"); err != nil {
		t.Fatalf("saveToFile failed: %v", err)
	}
	defer os.Remove(tmpFile)

	// Load into new analyzer
	va2 := NewVarianceAnalyzer()
	if err := va2.LoadFromFile(tmpFile); err != nil {
		t.Fatalf("loadFromFile failed: %v", err)
	}

	// Verify indices match
	indices1 := va1.GetSignalIndices()
	indices2 := va2.GetSignalIndices()

	for i := range indices1 {
		if indices1[i] != indices2[i] {
			t.Errorf("indices mismatch at %d: %d != %d", i, indices1[i], indices2[i])
		}
	}
}

func TestVarianceAnalyzerSaveLoadErrors(t *testing.T) {
	va := NewVarianceAnalyzer()

	// Should fail to load non-existent file
	err := va.LoadFromFile("/nonexistent/file.json")
	if err == nil {
		t.Error("expected error loading non-existent file")
	}

	// Calculate should fail without samples
	va2 := NewVarianceAnalyzer()
	err = va2.Calculate()
	if err == nil {
		t.Error("expected error calculating without samples")
	}
}
