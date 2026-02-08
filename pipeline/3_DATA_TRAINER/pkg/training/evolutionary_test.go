package training

import (
	"testing"
)

func TestNewSeedPopulation(t *testing.T) {
	targetToken := int32(42)
	contextHash := uint32(12345)
	size := 10

	population := NewSeedPopulation(targetToken, contextHash, size)

	if population == nil {
		t.Fatal("Expected non-nil population")
	}

	if population.TargetToken != targetToken {
		t.Errorf("Expected TargetToken %d, got %d", targetToken, population.TargetToken)
	}

	if population.ContextHash != contextHash {
		t.Errorf("Expected ContextHash %d, got %d", contextHash, population.ContextHash)
	}

	if len(population.Seeds) != size {
		t.Errorf("Expected %d seeds, got %d", size, len(population.Seeds))
	}

	for seedID, seed := range population.Seeds {
		if len(seed) != SeedSize {
			t.Errorf("Expected seed size %d, got %d for seed ID %d", SeedSize, len(seed), seedID)
		}
	}
}

func TestTrainingRecordValidate(t *testing.T) {
	tests := []struct {
		name     string
		record   TrainingRecord
		expected bool
	}{
		{
			name: "valid record",
			record: TrainingRecord{
				TokenSequence: []int32{1, 2, 3},
				FeatureVector: [12]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
				TargetToken:   42,
				ContextHash:   12345,
			},
			expected: true,
		},
		{
			name: "empty token sequence",
			record: TrainingRecord{
				TokenSequence: []int32{},
				FeatureVector: [12]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
				TargetToken:   42,
				ContextHash:   12345,
			},
			expected: false,
		},
		{
			name: "zero target token",
			record: TrainingRecord{
				TokenSequence: []int32{1, 2, 3},
				FeatureVector: [12]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
				TargetToken:   0,
				ContextHash:   12345,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.record.Validate()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGenerateRandomSeed(t *testing.T) {
	seed := GenerateRandomSeed()

	if len(seed) != SeedSize {
		t.Errorf("Expected seed size %d, got %d", SeedSize, len(seed))
	}

	seed2 := GenerateRandomSeed()
	if len(seed2) != SeedSize {
		t.Errorf("Expected seed size %d, got %d", SeedSize, len(seed2))
	}

	different := false
	for i := range seed {
		if seed[i] != seed2[i] {
			different = true
			break
		}
	}

	if !different {
		t.Error("Seeds should be different")
	}
}

func TestNewEvolutionaryHarness(t *testing.T) {
	populationSize := 128
	harness := NewEvolutionaryHarness(populationSize)

	if harness == nil {
		t.Fatal("Expected non-nil harness")
	}

	if harness.PopulationSize != populationSize {
		t.Errorf("Expected PopulationSize %d, got %d", populationSize, harness.PopulationSize)
	}

	if harness.EliteRatio != 0.25 {
		t.Errorf("Expected EliteRatio 0.25, got %f", harness.EliteRatio)
	}

	if harness.MutationRate != 0.05 {
		t.Errorf("Expected MutationRate 0.05, got %f", harness.MutationRate)
	}
}

func TestEvolutionaryHarnessCalculateAdvantage(t *testing.T) {
	harness := NewEvolutionaryHarness(10)

	results := []SeedResult{
		{SeedID: 1, Reward: 0.8},
		{SeedID: 2, Reward: 0.6},
		{SeedID: 3, Reward: 0.4},
		{SeedID: 4, Reward: 0.2},
	}

	results = harness.CalculateAdvantage(results)

	nonZeroAdvantages := 0
	for _, result := range results {
		if result.Advantage != 0 {
			nonZeroAdvantages++
		}
	}

	if nonZeroAdvantages == 0 {
		t.Error("Expected at least some non-zero advantages")
	}
}

func TestEvolutionaryHarnessBitwiseMutation(t *testing.T) {
	harness := NewEvolutionaryHarness(10)

	originalSeed := []byte{0x00, 0x00, 0x00, 0x00}

	mutatedSeed := harness.BitwiseMutation(originalSeed, 1.0)

	if len(mutatedSeed) != len(originalSeed) {
		t.Errorf("Expected mutated seed size %d, got %d", len(originalSeed), len(mutatedSeed))
	}

	different := false
	for i := range originalSeed {
		if originalSeed[i] != mutatedSeed[i] {
			different = true
			break
		}
	}

	if !different {
		t.Error("Expected mutation to change at least one bit")
	}
}

func TestEvolutionaryHarnessSelectAndMutate(t *testing.T) {
	harness := NewEvolutionaryHarness(10)

	currentSeeds := map[uint32][]byte{
		1: {0x01, 0x02, 0x03, 0x04},
		2: {0x05, 0x06, 0x07, 0x08},
		3: {0x09, 0x0A, 0x0B, 0x0C},
		4: {0x0D, 0x0E, 0x0F, 0x10},
	}

	results := []SeedResult{
		{SeedID: 1, Advantage: 2.0},
		{SeedID: 2, Advantage: 1.5},
		{SeedID: 3, Advantage: 0.5},
		{SeedID: 4, Advantage: -1.0},
	}

	newGeneration := harness.SelectAndMutate(results, currentSeeds)

	if len(newGeneration) > harness.PopulationSize { // Should not exceed population size
		t.Errorf("Expected new generation size <= %d, got %d", harness.PopulationSize, len(newGeneration))
	}

	preservedElite := 0
	for _, result := range results {
		if result.Advantage > 0 && result.SeedID == 1 || result.SeedID == 2 {
			if _, exists := newGeneration[result.SeedID]; exists {
				preservedElite++
			}
		}
	}
	if preservedElite == 0 {
		t.Error("Expected at least one elite seed to be preserved")
	}
}

func TestEvolutionaryHarnessGetEliteSeeds(t *testing.T) {
	harness := NewEvolutionaryHarness(10)

	results := []SeedResult{
		{SeedID: 1, Advantage: 2.0},
		{SeedID: 2, Advantage: 1.5},
		{SeedID: 3, Advantage: 0.5},
		{SeedID: 4, Advantage: -1.0},
	}

	eliteSeeds := harness.GetEliteSeeds(results)

	expectedEliteCount := int(float64(len(results)) * harness.EliteRatio)
	if expectedEliteCount == 0 {
		expectedEliteCount = 1
	}

	if len(eliteSeeds) != expectedEliteCount {
		t.Errorf("Expected %d elite seeds, got %d", expectedEliteCount, len(eliteSeeds))
	}

	if len(eliteSeeds) > 1 && eliteSeeds[0].Advantage < eliteSeeds[1].Advantage {
		t.Error("Elite seeds should be sorted by advantage in descending order")
	}
}

func TestHammingDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        uint32
		b        uint32
		expected int
	}{
		{"same values", 0x12345678, 0x12345678, 0},
		{"different values", 0x00000000, 0xFFFFFFFF, 32},
		{"one bit difference", 0x00000000, 0x00000001, 1},
		{"multiple bits", 0x10101010, 0x01010101, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hammingDistance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected distance %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestComputeContextHash(t *testing.T) {
	tests := []struct {
		name          string
		tokenSequence []int32
		windowSize    int
		expectedZero  bool
	}{
		{
			name:          "empty sequence",
			tokenSequence: []int32{},
			windowSize:    5,
			expectedZero:  true,
		},
		{
			name:          "single token",
			tokenSequence: []int32{42},
			windowSize:    5,
			expectedZero:  false,
		},
		{
			name:          "multiple tokens",
			tokenSequence: []int32{1, 2, 3, 4, 5},
			windowSize:    5,
			expectedZero:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeContextHash(tt.tokenSequence, tt.windowSize)
			isZero := result == 0
			if isZero != tt.expectedZero {
				t.Errorf("Expected zero=%v, got %d", tt.expectedZero, result)
			}
		})
	}
}

func BenchmarkNewSeedPopulation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewSeedPopulation(42, 12345, 100)
	}
}

func BenchmarkGenerateRandomSeed(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateRandomSeed()
	}
}

func BenchmarkComputeContextHash(b *testing.B) {
	tokens := make([]int32, 100)
	for i := range tokens {
		tokens[i] = int32(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ComputeContextHash(tokens, 5)
	}
}
