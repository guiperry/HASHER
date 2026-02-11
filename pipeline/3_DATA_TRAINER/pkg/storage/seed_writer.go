package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// JSONSeedWriter handles writing best seeds back to JSON with precise ASIC slot matching
type JSONSeedWriter struct {
	sourceFile    string
	outputFile    string
	mu            sync.Mutex
	pendingWrites map[string][]byte // composite key (asic_slots) -> best_seed
}

// NewJSONSeedWriter creates a new JSONSeedWriter
func NewJSONSeedWriter(sourceFile, outputFile string) *JSONSeedWriter {
	return &JSONSeedWriter{
		sourceFile:    sourceFile,
		outputFile:    outputFile,
		pendingWrites: make(map[string][]byte),
	}
}

// generateAsicKey creates a unique key for a frame based on its 12 ASIC slots
func (sw *JSONSeedWriter) generateAsicKey(slots [12]uint32) string {
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d",
		slots[0], slots[1], slots[2], slots[3],
		slots[4], slots[5], slots[6], slots[7],
		slots[8], slots[9], slots[10], slots[11])
}

// AddSeedWrite queues a best seed using ASIC slots as the unique identifier
func (sw *JSONSeedWriter) AddSeedWrite(slots [12]uint32, bestSeed []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	key := sw.generateAsicKey(slots)
	fmt.Printf("[DEBUG] SeedWriter: Queuing seed for ASIC key (Slot0=%d)\n", slots[0])
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

	fmt.Printf("[DEBUG] SeedWriter: Writing back %d pending seeds\n", len(sw.pendingWrites))

	// 1. Read existing file
	data, err := os.ReadFile(sw.sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source JSON file: %w", err)
	}

	var records []map[string]interface{}
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	updated := 0
	for i := range records {
		record := records[i]
		
		// Extract ASIC slots to form the key
		var slots [12]uint32
		foundAll := true
		for j := 0; j < 12; j++ {
			key := fmt.Sprintf("asic_slot_%d", j)
			val, ok := record[key].(float64)
			if !ok {
				// Try PascalCase
				key = fmt.Sprintf("AsicSlots%d", j)
				val, ok = record[key].(float64)
			}
			
			if ok {
				slots[j] = uint32(val)
			} else {
				foundAll = false
				break
			}
		}

		if !foundAll {
			continue
		}

		key := sw.generateAsicKey(slots)
		if seed, ok := sw.pendingWrites[key]; ok {
			// Update the record
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

	// 3. Write back
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

// DualSeedWriter writes to both JSON and Arrow
type DualSeedWriter struct {
	jsonWriter  *JSONSeedWriter
	arrowWriter *ArrowSeedWriter
}

// NewDualSeedWriter creates a new DualSeedWriter
func NewDualSeedWriter(dataPath string) *DualSeedWriter {
	framesDir := filepath.Join(dataPath, "frames")
	jsonSource := filepath.Join(framesDir, "training_frames.json")
	jsonOutput := filepath.Join(framesDir, "training_frames_with_seeds.json")
	arrowSource := filepath.Join(framesDir, "training_frames.arrow")
	arrowOutput := filepath.Join(framesDir, "training_frames_with_seeds.arrow")

	return &DualSeedWriter{
		jsonWriter:  NewJSONSeedWriter(jsonSource, jsonOutput),
		arrowWriter: NewArrowSeedWriter(arrowSource, arrowOutput),
	}
}

// AddSeedWrite redirects to both writers using ASIC slots
func (dsw *DualSeedWriter) AddSeedWrite(slots [12]uint32, bestSeed []byte) error {
	if err := dsw.jsonWriter.AddSeedWrite(slots, bestSeed); err != nil {
		return err
	}
	return dsw.arrowWriter.AddSeedWrite(slots, bestSeed)
}

// WriteBack commits pending writes to both outputs
func (dsw *DualSeedWriter) WriteBack() error {
	if err := dsw.jsonWriter.WriteBack(); err != nil {
		return err
	}
	return dsw.arrowWriter.WriteBack()
}

// GetOutputFile returns the primary output file path
func (dsw *DualSeedWriter) GetOutputFile() string {
	return dsw.jsonWriter.GetOutputFile()
}
