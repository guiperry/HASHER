package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/lab/hasher/data-trainer/pkg/training"
)

type StorageConfig struct {
	BasePath  string `json:"base_path"`
	LayerSize int    `json:"layer_size"`
}

type LayerMetadata struct {
	LayerID      int32        `json:"layer_id"`
	TokenCount   int          `json:"token_count"`
	TotalWeights int          `json:"total_weights"`
	CreatedAt    time.Time    `json:"created_at"`
	LastUpdated  time.Time    `json:"last_updated"`
	FitnessScore float64      `json:"fitness_score"`
	Generation   int32        `json:"generation"`
	TokenRanges  []TokenRange `json:"token_ranges"`
}

type TokenRange struct {
	StartToken int32 `json:"start_token"`
	EndToken   int32 `json:"end_token"`
	Count      int   `json:"count"`
}

type WeightQuery struct {
	TokenIDs    []int32 `json:"token_ids"`
	LayerID     int32   `json:"layer_id"`
	Generation  int32   `json:"generation"`
	MinFitness  float64 `json:"min_fitness"`
	MaxFitness  float64 `json:"max_fitness"`
	ContextHash uint32  `json:"context_hash"`
}

type WeightExport struct {
	LayerMetadata *LayerMetadata
	Weights       []training.WeightRecord
	ExportedAt    time.Time
}

type CSVStorage struct {
	basePath     string
	layerSize    int
	currentLayer int
	mutex        sync.RWMutex
}

func NewCSVStorage(basePath string, layerSize int) *CSVStorage {
	return &CSVStorage{
		basePath:  basePath,
		layerSize: layerSize,
	}
}

func (cs *CSVStorage) Initialize() error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if err := os.MkdirAll(cs.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}

	return nil
}

func (cs *CSVStorage) SaveWeights(weights []WeightRecord, layerID int32) error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if len(weights) == 0 {
		return fmt.Errorf("no weights to save")
	}

	layerPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d.csv", layerID))

	tmpPath := layerPath + ".tmp"
	defer os.Remove(tmpPath)

	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer file.Close()

	data, err := json.Marshal(weights)
	if err != nil {
		return fmt.Errorf("failed to marshal weights: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write weights: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	if err := os.Rename(tmpPath, layerPath); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	trainingWeights := make([]training.WeightRecord, len(weights))
	for i, w := range weights {
		trainingWeights[i] = training.WeightRecord{
			TokenID:      w.TokenID,
			BestSeed:     w.BestSeed,
			FitnessScore: w.FitnessScore,
			Generation:   w.Generation,
			ContextKey:   w.ContextKey,
		}
	}
	metadata := cs.createLayerMetadata(trainingWeights, layerID)
	if err := cs.saveLayerMetadata(metadata); err != nil {
		return fmt.Errorf("failed to save layer metadata: %w", err)
	}

	return nil
}

func (cs *CSVStorage) LoadWeights(query WeightQuery) ([]WeightRecord, error) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()

	layerPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d.json", query.LayerID))

	if _, err := os.Stat(layerPath); os.IsNotExist(err) {
		csvPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d.csv", query.LayerID))
		if _, err := os.Stat(csvPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("layer %d not found", query.LayerID)
		}
		layerPath = csvPath
	}

	data, err := os.ReadFile(layerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer file: %w", err)
	}

	var weights []training.WeightRecord
	if err := json.Unmarshal(data, &weights); err != nil {
		return nil, fmt.Errorf("failed to unmarshal weights: %w", err)
	}

	var results []WeightRecord
	for _, weight := range weights {
		storageWeight := WeightRecord{
			TokenID:      weight.TokenID,
			BestSeed:     weight.BestSeed,
			FitnessScore: weight.FitnessScore,
			Generation:   weight.Generation,
			ContextKey:   weight.ContextKey,
		}
		if cs.matchesQuery(storageWeight, query) {
			results = append(results, storageWeight)
		}
	}

	return results, nil
}

func (cs *CSVStorage) matchesQuery(weight WeightRecord, query WeightQuery) bool {
	if len(query.TokenIDs) > 0 {
		found := false
		for _, tokenID := range query.TokenIDs {
			if weight.TokenID == tokenID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if query.Generation > 0 && weight.Generation != query.Generation {
		return false
	}

	if query.MinFitness > 0 && weight.FitnessScore < query.MinFitness {
		return false
	}

	if query.MaxFitness > 0 && weight.FitnessScore > query.MaxFitness {
		return false
	}

	if query.ContextHash > 0 && weight.ContextKey != query.ContextHash {
		return false
	}

	return true
}

func (cs *CSVStorage) createLayerMetadata(weights []training.WeightRecord, layerID int32) *LayerMetadata {
	if len(weights) == 0 {
		return &LayerMetadata{
			LayerID:     layerID,
			CreatedAt:   time.Now(),
			LastUpdated: time.Now(),
		}
	}

	tokenMap := make(map[int32]int)
	var totalFitness float64
	var minToken, maxToken int32 = weights[0].TokenID, weights[0].TokenID
	generation := weights[0].Generation

	for _, weight := range weights {
		tokenMap[weight.TokenID]++
		totalFitness += weight.FitnessScore

		if weight.TokenID < minToken {
			minToken = weight.TokenID
		}
		if weight.TokenID > maxToken {
			maxToken = weight.TokenID
		}

		if weight.Generation > generation {
			generation = weight.Generation
		}
	}

	var tokenRanges []TokenRange
	for token, count := range tokenMap {
		tokenRanges = append(tokenRanges, TokenRange{
			StartToken: token,
			EndToken:   token,
			Count:      count,
		})
	}

	return &LayerMetadata{
		LayerID:      layerID,
		TokenCount:   len(tokenMap),
		TotalWeights: len(weights),
		CreatedAt:    time.Now(),
		LastUpdated:  time.Now(),
		FitnessScore: totalFitness / float64(len(weights)),
		Generation:   generation,
		TokenRanges:  tokenRanges,
	}
}

func (cs *CSVStorage) saveLayerMetadata(metadata *LayerMetadata) error {
	metadataPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d_metadata.json", metadata.LayerID))

	file, err := os.Create(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("failed to encode metadata: %w", err)
	}

	return nil
}

func (cs *CSVStorage) GetLayerMetadata(layerID int32) (*LayerMetadata, error) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()

	metadataPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d_metadata.json", layerID))

	file, err := os.Open(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open metadata file: %w", err)
	}
	defer file.Close()

	var metadata LayerMetadata
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode metadata: %w", err)
	}

	return &metadata, nil
}

func (cs *CSVStorage) ListLayers() ([]int32, error) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()

	entries, err := os.ReadDir(cs.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var layers []int32
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		var layerID int32
		if n, err := fmt.Sscanf(entry.Name(), "layer_%d.csv", &layerID); err == nil && n == 1 {
			layers = append(layers, layerID)
		} else if n, err := fmt.Sscanf(entry.Name(), "layer_%d.json", &layerID); err == nil && n == 1 {
			layers = append(layers, layerID)
		}
	}

	sort.Slice(layers, func(i, j int) bool {
		return layers[i] < layers[j]
	})

	return layers, nil
}

func (cs *CSVStorage) ExportWeights(layerID int32) (*WeightExport, error) {
	weights, err := cs.LoadWeights(WeightQuery{LayerID: layerID})
	if err != nil {
		return nil, fmt.Errorf("failed to load weights: %w", err)
	}

	metadata, err := cs.GetLayerMetadata(layerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get layer metadata: %w", err)
	}

	trainingWeights := make([]training.WeightRecord, len(weights))
	for i, w := range weights {
		trainingWeights[i] = training.WeightRecord{
			TokenID:      w.TokenID,
			BestSeed:     w.BestSeed,
			FitnessScore: w.FitnessScore,
			Generation:   w.Generation,
			ContextKey:   w.ContextKey,
		}
	}
	return &WeightExport{
		LayerMetadata: metadata,
		Weights:       trainingWeights,
		ExportedAt:    time.Now(),
	}, nil
}

func (cs *CSVStorage) DeleteLayer(layerID int32) error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	layerPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d.json", layerID))
	metadataPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d_metadata.json", layerID))
	csvPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d.csv", layerID))

	for _, path := range []string{layerPath, metadataPath, csvPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete file %s: %w", path, err)
		}
	}

	return nil
}

func (cs *CSVStorage) GetStorageStats() map[string]interface{} {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()

	layers, err := cs.ListLayers()
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	var totalSize int64
	var totalWeights int
	var totalTokens int

	for _, layerID := range layers {
		jsonPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d.json", layerID))
		csvPath := filepath.Join(cs.basePath, fmt.Sprintf("layer_%d.csv", layerID))

		for _, path := range []string{jsonPath, csvPath} {
			if info, err := os.Stat(path); err == nil {
				totalSize += info.Size()
			}
		}

		metadata, err := cs.GetLayerMetadata(layerID)
		if err == nil {
			totalWeights += metadata.TotalWeights
			totalTokens += metadata.TokenCount
		}
	}

	return map[string]interface{}{
		"total_layers":  len(layers),
		"total_size":    totalSize,
		"total_weights": totalWeights,
		"total_tokens":  totalTokens,
		"base_path":     cs.basePath,
		"layer_size":    cs.layerSize,
	}
}
