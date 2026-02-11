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
	pendingWrites map[string][]byte // composite key (asic_slots + target_token_id) -> best_seed
	cachedRecords []map[string]interface{}
}

// NewJSONSeedWriter creates a new JSONSeedWriter
func NewJSONSeedWriter(sourceFile, outputFile string) *JSONSeedWriter {
	return &JSONSeedWriter{
		sourceFile:    sourceFile,
		outputFile:    outputFile,
		pendingWrites: make(map[string][]byte),
	}
}

// generateAsicKey creates a unique key based on 12 ASIC slots and target token ID
func (sw *JSONSeedWriter) generateAsicKey(slots [12]uint32, targetTokenID int32) string {
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d",
		slots[0], slots[1], slots[2], slots[3],
		slots[4], slots[5], slots[6], slots[7],
		slots[8], slots[9], slots[10], slots[11],
		targetTokenID)
}

// AddSeedWrite queues a best seed using ASIC slots + target token as the unique identifier
func (sw *JSONSeedWriter) AddSeedWrite(slots [12]uint32, targetTokenID int32, bestSeed []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	key := sw.generateAsicKey(slots, targetTokenID)
	fmt.Printf("[DEBUG] JSONSeedWriter: Queuing win for Token %d, Slot0: %d\n", targetTokenID, slots[0])
	sw.pendingWrites[key] = bestSeed
	return nil
}

// WriteBack commits all pending writes to the output JSON file incrementally
func (sw *JSONSeedWriter) WriteBack() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.pendingWrites) == 0 {
		return nil
	}

	// 1. Initialize cache if empty
	if sw.cachedRecords == nil {
		// Prefer output file if it exists (incremental update)
		source := sw.outputFile
		if _, err := os.Stat(source); os.IsNotExist(err) {
			source = sw.sourceFile
		}
		
		fmt.Printf("[DEBUG] JSONSeedWriter: Initializing cache from %s\n", filepath.Base(source))
		data, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("failed to read JSON source: %w", err)
		}

		if err := json.Unmarshal(data, &sw.cachedRecords); err != nil {
			return fmt.Errorf("failed to unmarshal JSON source: %w", err)
		}
	}

	// 2. Update cached records
	updated := 0
	for i := range sw.cachedRecords {
		record := sw.cachedRecords[i]
		
		// Extract ASIC slots
		var slots [12]uint32
		foundSlots := true
		for j := 0; j < 12; j++ {
			key := fmt.Sprintf("asic_slot_%d", j)
			valRaw, ok := record[key]
			if !ok {
				key = fmt.Sprintf("AsicSlots%d", j)
				valRaw, ok = record[key]
			}
			
			if ok {
				if valFloat, ok := valRaw.(float64); ok {
					slots[j] = uint32(int32(valFloat))
				} else {
					foundSlots = false
					break
				}
			} else {
				foundSlots = false
				break
			}
		}

		if !foundSlots {
			continue
		}

		// Extract TargetTokenID
		targetIDRaw, ok := record["target_token_id"].(float64)
		if !ok {
			targetIDRaw, _ = record["TargetTokenID"].(float64)
		}
		targetTokenID := int32(targetIDRaw)

		key := sw.generateAsicKey(slots, targetTokenID)
		if seed, ok := sw.pendingWrites[key]; ok {
			fmt.Printf("[DEBUG] JSONSeedWriter: MATCH FOUND for Token %d, Slot0: %d\n", targetTokenID, slots[0])
			
			// Use original casing if present, default to snake_case
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

	// 3. Persist to output file
	newData, err := json.MarshalIndent(sw.cachedRecords, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal output JSON: %w", err)
	}

	if err := os.WriteFile(sw.outputFile, newData, 0644); err != nil {
		return fmt.Errorf("failed to write output JSON: %w", err)
	}

	// Clear pending writes but keep cachedRecords for next WriteBack
	sw.pendingWrites = make(map[string][]byte)
	fmt.Printf("Successfully updated %d records in %s (total: %d)\n", updated, filepath.Base(sw.outputFile), len(sw.cachedRecords))

	return nil
}

// GetOutputFile returns the output file path
func (sw *JSONSeedWriter) GetOutputFile() string {
	return sw.outputFile
}

// DualSeedWriter handles dual persistence to JSON and Arrow
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

// AddSeedWrite queues a win
func (dsw *DualSeedWriter) AddSeedWrite(slots [12]uint32, targetTokenID int32, bestSeed []byte) error {
	if err := dsw.jsonWriter.AddSeedWrite(slots, targetTokenID, bestSeed); err != nil {
		return err
	}
	return dsw.arrowWriter.AddSeedWrite(slots, targetTokenID, bestSeed)
}

// WriteBack commits all pending wins
func (dsw *DualSeedWriter) WriteBack() error {
	if err := dsw.jsonWriter.WriteBack(); err != nil {
		return err
	}
	return dsw.arrowWriter.WriteBack()
}

// GetOutputFile returns the primary JSON output file path
func (dsw *DualSeedWriter) GetOutputFile() string {
	return dsw.jsonWriter.GetOutputFile()
}
