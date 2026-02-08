package storage

import (
	"os"
	"testing"
)

func TestNewCSVStorage(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewCSVStorage(tempDir, 100)

	if storage == nil {
		t.Fatal("Expected non-nil storage")
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("Storage directory should exist after initialization")
	}
}

func TestCSVStorageSaveAndLoadWeights(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewCSVStorage(tempDir, 100)
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	weights := []WeightRecord{
		{
			TokenID:      42,
			BestSeed:     []byte{0x01, 0x02, 0x03, 0x04},
			FitnessScore: 0.95,
			Generation:   1,
			ContextKey:   12345,
		},
		{
			TokenID:      43,
			BestSeed:     []byte{0x05, 0x06, 0x07, 0x08},
			FitnessScore: 0.87,
			Generation:   2,
			ContextKey:   12346,
		},
	}

	layerID := int32(0)
	if err := storage.SaveWeights(weights, layerID); err != nil {
		t.Fatalf("Failed to save weights: %v", err)
	}

	query := WeightQuery{
		LayerID: layerID,
	}

	loadedWeights, err := storage.LoadWeights(query)
	if err != nil {
		t.Fatalf("Failed to load weights: %v", err)
	}

	if len(loadedWeights) != len(weights) {
		t.Errorf("Expected %d weights, got %d", len(weights), len(loadedWeights))
	}

	for i, original := range weights {
		if i >= len(loadedWeights) {
			break
		}
		loaded := loadedWeights[i]

		if loaded.TokenID != original.TokenID {
			t.Errorf("Expected TokenID %d, got %d", original.TokenID, loaded.TokenID)
		}

		if loaded.FitnessScore != original.FitnessScore {
			t.Errorf("Expected FitnessScore %f, got %f", original.FitnessScore, loaded.FitnessScore)
		}

		if loaded.Generation != original.Generation {
			t.Errorf("Expected Generation %d, got %d", original.Generation, loaded.Generation)
		}
	}
}

func TestCSVStorageListLayers(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewCSVStorage(tempDir, 100)
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	weights := []WeightRecord{
		{
			TokenID:      42,
			BestSeed:     []byte{0x01, 0x02, 0x03, 0x04},
			FitnessScore: 0.95,
			Generation:   1,
			ContextKey:   12345,
		},
	}

	for i := int32(0); i < 3; i++ {
		if err := storage.SaveWeights(weights, i); err != nil {
			t.Fatalf("Failed to save weights for layer %d: %v", i, err)
		}
	}

	layers, err := storage.ListLayers()
	if err != nil {
		t.Fatalf("Failed to list layers: %v", err)
	}

	if len(layers) != 3 {
		t.Errorf("Expected 3 layers, got %d", len(layers))
	}

	for i, layerID := range layers {
		if i == 0 && layerID != 0 {
			t.Errorf("Expected first layer ID to be 0, got %d", layerID)
		}
	}
}

func TestCSVStorageGetLayerMetadata(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewCSVStorage(tempDir, 100)
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	weights := []WeightRecord{
		{
			TokenID:      42,
			BestSeed:     []byte{0x01, 0x02, 0x03, 0x04},
			FitnessScore: 0.95,
			Generation:   1,
			ContextKey:   12345,
		},
		{
			TokenID:      43,
			BestSeed:     []byte{0x05, 0x06, 0x07, 0x08},
			FitnessScore: 0.87,
			Generation:   2,
			ContextKey:   12346,
		},
	}

	layerID := int32(0)
	if err := storage.SaveWeights(weights, layerID); err != nil {
		t.Fatalf("Failed to save weights: %v", err)
	}

	metadata, err := storage.GetLayerMetadata(layerID)
	if err != nil {
		t.Fatalf("Failed to get layer metadata: %v", err)
	}

	if metadata.LayerID != layerID {
		t.Errorf("Expected LayerID %d, got %d", layerID, metadata.LayerID)
	}

	if metadata.TokenCount != 2 {
		t.Errorf("Expected TokenCount 2, got %d", metadata.TokenCount)
	}

	if metadata.TotalWeights != 2 {
		t.Errorf("Expected TotalWeights 2, got %d", metadata.TotalWeights)
	}

	expectedAvgFitness := (0.95 + 0.87) / 2
	if metadata.FitnessScore < expectedAvgFitness-0.001 || metadata.FitnessScore > expectedAvgFitness+0.001 {
		t.Errorf("Expected FitnessScore %f, got %f", expectedAvgFitness, metadata.FitnessScore)
	}
}

func TestCSVStorageDeleteLayer(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewCSVStorage(tempDir, 100)
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	weights := []WeightRecord{
		{
			TokenID:      42,
			BestSeed:     []byte{0x01, 0x02, 0x03, 0x04},
			FitnessScore: 0.95,
			Generation:   1,
			ContextKey:   12345,
		},
	}

	layerID := int32(0)
	if err := storage.SaveWeights(weights, layerID); err != nil {
		t.Fatalf("Failed to save weights: %v", err)
	}

	if _, err := storage.GetLayerMetadata(layerID); err != nil {
		t.Fatalf("Layer should exist before deletion: %v", err)
	}

	if err := storage.DeleteLayer(layerID); err != nil {
		t.Fatalf("Failed to delete layer: %v", err)
	}

	if _, err := storage.GetLayerMetadata(layerID); err == nil {
		t.Error("Layer should not exist after deletion")
	}
}

func TestCSVStorageWeightQuery(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewCSVStorage(tempDir, 100)
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	weights := []WeightRecord{
		{
			TokenID:      42,
			BestSeed:     []byte{0x01, 0x02, 0x03, 0x04},
			FitnessScore: 0.95,
			Generation:   1,
			ContextKey:   12345,
		},
		{
			TokenID:      43,
			BestSeed:     []byte{0x05, 0x06, 0x07, 0x08},
			FitnessScore: 0.87,
			Generation:   2,
			ContextKey:   12346,
		},
		{
			TokenID:      44,
			BestSeed:     []byte{0x09, 0x0A, 0x0B, 0x0C},
			FitnessScore: 0.92,
			Generation:   1,
			ContextKey:   12347,
		},
	}

	layerID := int32(0)
	if err := storage.SaveWeights(weights, layerID); err != nil {
		t.Fatalf("Failed to save weights: %v", err)
	}

	query := WeightQuery{
		LayerID:    layerID,
		TokenIDs:   []int32{42, 44},
		MinFitness: 0.9,
	}

	filteredWeights, err := storage.LoadWeights(query)
	if err != nil {
		t.Fatalf("Failed to load weights with query: %v", err)
	}

	if len(filteredWeights) != 2 {
		t.Errorf("Expected 2 filtered weights, got %d", len(filteredWeights))
	}

	for _, weight := range filteredWeights {
		if weight.TokenID != 42 && weight.TokenID != 44 {
			t.Errorf("Unexpected TokenID %d in filtered results", weight.TokenID)
		}

		if weight.FitnessScore < 0.9 {
			t.Errorf("TokenID %d has fitness %f below minimum 0.9", weight.TokenID, weight.FitnessScore)
		}
	}
}

func TestCSVStorageGetStorageStats(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewCSVStorage(tempDir, 100)
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	stats := storage.GetStorageStats()
	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}

	if totalLayers, ok := stats["total_layers"]; ok {
		if totalLayers.(int) != 0 {
			t.Errorf("Expected 0 total layers initially, got %v", totalLayers)
		}
	}

	if basePath, ok := stats["base_path"]; ok {
		if basePath.(string) != tempDir {
			t.Errorf("Expected base path %s, got %v", tempDir, basePath)
		}
	}

	weights := []WeightRecord{
		{
			TokenID:      42,
			BestSeed:     []byte{0x01, 0x02, 0x03, 0x04},
			FitnessScore: 0.95,
			Generation:   1,
			ContextKey:   12345,
		},
	}

	if err := storage.SaveWeights(weights, 0); err != nil {
		t.Fatalf("Failed to save weights: %v", err)
	}

	stats = storage.GetStorageStats()
	if totalLayers, ok := stats["total_layers"]; ok {
		if totalLayers.(int) != 1 {
			t.Errorf("Expected 1 total layer after saving, got %v", totalLayers)
		}
	}
}

func BenchmarkCSVStorageSaveWeights(b *testing.B) {
	tempDir := b.TempDir()
	storage := NewCSVStorage(tempDir, 100)
	if err := storage.Initialize(); err != nil {
		b.Fatalf("Failed to initialize storage: %v", err)
	}

	weights := make([]WeightRecord, 100)
	for i := range weights {
		weights[i] = WeightRecord{
			TokenID:      int32(i),
			BestSeed:     []byte{byte(i), byte(i + 1), byte(i + 2), byte(i + 3)},
			FitnessScore: 0.95,
			Generation:   1,
			ContextKey:   uint32(i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		layerID := int32(i % 10)
		if err := storage.SaveWeights(weights, layerID); err != nil {
			b.Fatalf("Failed to save weights: %v", err)
		}
	}
}
