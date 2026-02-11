package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// JSONTrainingRecord is used for JSON matching during write-back
type JSONTrainingRecordWithMetadata struct {
	SourceFile    string `json:"source_file"`
	ChunkID       int32  `json:"chunk_id"`
	WindowStart   int32  `json:"window_start"`
	TargetTokenID int32  `json:"target_token_id"`
	BestSeed      []byte `json:"best_seed,omitempty"`
}

// JSONSeedWriter handles writing best seeds back to JSON with precise frame matching
type JSONSeedWriter struct {
	sourceFile    string
	outputFile    string
	mu            sync.Mutex
	pendingWrites map[string][]byte // composite key (file+chunk+window) -> best_seed
}

// NewJSONSeedWriter creates a new JSONSeedWriter
func NewJSONSeedWriter(sourceFile, outputFile string) *JSONSeedWriter {
	return &JSONSeedWriter{
		sourceFile:    sourceFile,
		outputFile:    outputFile,
		pendingWrites: make(map[string][]byte),
	}
}

// generateKey creates a unique key for a specific training frame
func (sw *JSONSeedWriter) generateKey(sourceFile string, chunkID int32, windowStart int32) string {
	return fmt.Sprintf("%s:%d:%d", filepath.Base(sourceFile), chunkID, windowStart)
}

// AddSeedWrite queues a best seed to be written back with full metadata context
func (sw *JSONSeedWriter) AddSeedWrite(sourceFile string, chunkID int32, windowStart int32, bestSeed []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	key := sw.generateKey(sourceFile, chunkID, windowStart)
	sw.pendingWrites[key] = bestSeed
	return nil
}

// WriteBack writes all pending seeds back to the output JSON file
func (sw *JSONSeedWriter) WriteBack() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.pendingWrites) == 0 {
		return nil
	}

	// 1. Read existing file (ingestion source)
	data, err := os.ReadFile(sw.sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source JSON file: %w", err)
	}

	// Unmarshal into a generic map or slice of maps to preserve all fields
	var records []map[string]interface{}
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	updated := 0
	for i := range records {
		record := records[i]
		
		// Extract metadata for key generation - support both snake_case and PascalCase
		sourceFile, _ := record["source_file"].(string)
		if sourceFile == "" {
			sourceFile, _ = record["SourceFile"].(string)
		}

		chunkIDRaw, ok := record["chunk_id"].(float64)
		if !ok {
			chunkIDRaw, _ = record["ChunkID"].(float64)
		}

		windowStartRaw, ok := record["window_start"].(float64)
		if !ok {
			windowStartRaw, _ = record["WindowStart"].(float64)
		}
		
		key := sw.generateKey(sourceFile, int32(chunkIDRaw), int32(windowStartRaw))
		
		if seed, ok := sw.pendingWrites[key]; ok {
			// Update the record - use original case if present, otherwise snake_case
			if _, exists := record["best_seed"]; exists {
				record["best_seed"] = seed
			} else if _, exists := record["BestSeed"]; exists {
				record["BestSeed"] = seed
			} else {
				record["best_seed"] = seed
			}
			updated++
		}
	}

	// 3. Write back with indentation
	newData, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(sw.outputFile, newData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	// Clear pending writes
	sw.pendingWrites = make(map[string][]byte)
	fmt.Printf("Successfully updated %d records in %s (total: %d)\n", updated, sw.outputFile, len(records))

	return nil
}

// GetOutputFile returns the output file path
func (sw *JSONSeedWriter) GetOutputFile() string {
	return sw.outputFile
}

// DualSeedWriter is now a wrapper around JSONSeedWriter as Parquet is deprecated
type DualSeedWriter struct {
	jsonWriter *JSONSeedWriter
}

// NewDualSeedWriter creates a new DualSeedWriter that primary targets JSON
func NewDualSeedWriter(dataPath string) *DualSeedWriter {
	framesDir := filepath.Join(dataPath, "frames")
	jsonSource := filepath.Join(framesDir, "training_frames.json")
	jsonOutput := filepath.Join(framesDir, "training_frames_with_seeds.json")

	return &DualSeedWriter{
		jsonWriter: NewJSONSeedWriter(jsonSource, jsonOutput),
	}
}

// AddSeedWrite redirects to JSON writer with full metadata
func (dsw *DualSeedWriter) AddSeedWrite(sourceFile string, chunkID int32, windowStart int32, bestSeed []byte) error {
	return dsw.jsonWriter.AddSeedWrite(sourceFile, chunkID, windowStart, bestSeed)
}

// WriteBack commits pending writes to the JSON output
func (dsw *DualSeedWriter) WriteBack() error {
	return dsw.jsonWriter.WriteBack()
}

// GetOutputFile returns the primary output file path
func (dsw *DualSeedWriter) GetOutputFile() string {
	return dsw.jsonWriter.GetOutputFile()
}
