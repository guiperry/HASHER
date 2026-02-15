package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// JSONSeedWriter handles writing best seeds back to JSON with precise ASIC slot matching
type JSONSeedWriter struct {
	dataPath      string
	mu            sync.Mutex
	pendingWrites map[string]map[string][]byte // sourceFile -> (compositeKey -> best_seed)
	cachedRecords map[string][]map[string]interface{}
}

// NewJSONSeedWriter creates a new JSONSeedWriter
func NewJSONSeedWriter(dataPath string) *JSONSeedWriter {
	return &JSONSeedWriter{
		dataPath:      dataPath,
		pendingWrites: make(map[string]map[string][]byte),
		cachedRecords: make(map[string][]map[string]interface{}),
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
func (sw *JSONSeedWriter) AddSeedWrite(sourceFile string, slots [12]uint32, targetTokenID int32, bestSeed []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	// Use generic name if source is empty
	if sourceFile == "" {
		sourceFile = "training_frames.json"
	}

	if _, ok := sw.pendingWrites[sourceFile]; !ok {
		sw.pendingWrites[sourceFile] = make(map[string][]byte)
	}

	key := sw.generateAsicKey(slots, targetTokenID)
	fmt.Printf("[DEBUG] JSONSeedWriter: Queuing win for Token %d in %s\n", targetTokenID, sourceFile)
	sw.pendingWrites[sourceFile][key] = bestSeed
	return nil
}

// WriteBack commits all pending writes to the appropriate output JSON files
func (sw *JSONSeedWriter) WriteBack() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.pendingWrites) == 0 {
		return nil
	}

	for sourceFile, writes := range sw.pendingWrites {
		if len(writes) == 0 {
			continue
		}

		// Determine output file path
		ext := filepath.Ext(sourceFile)
		base := strings.TrimSuffix(filepath.Base(sourceFile), ext)
		outputFile := filepath.Join(sw.dataPath, "frames", base+"_with_seeds"+ext)
		
		// If sourceFile is a full path, use it directly to find the source
		sourcePath := sourceFile
		if !filepath.IsAbs(sourcePath) {
			sourcePath = filepath.Join(sw.dataPath, "frames", sourceFile)
		}

		// 1. Initialize cache for this file if empty
		if _, ok := sw.cachedRecords[sourceFile]; !ok {
			// Prefer output file if it exists (incremental update)
			readPath := outputFile
			if _, err := os.Stat(readPath); os.IsNotExist(err) {
				readPath = sourcePath
			}
			
			// Safety check: if readPath doesn't exist, we can't do anything
			if _, err := os.Stat(readPath); os.IsNotExist(err) {
				fmt.Printf("[WARN] JSONSeedWriter: Source file not found: %s\n", readPath)
				continue
			}

			fmt.Printf("[DEBUG] JSONSeedWriter: Initializing cache from %s\n", filepath.Base(readPath))
			data, err := os.ReadFile(readPath)
			if err != nil {
				return fmt.Errorf("failed to read JSON source %s: %w", readPath, err)
			}

			var records []map[string]interface{}
			if err := json.Unmarshal(data, &records); err != nil {
				return fmt.Errorf("failed to unmarshal JSON source %s: %w", readPath, err)
			}
			sw.cachedRecords[sourceFile] = records
		}

		// 2. Update cached records
		records := sw.cachedRecords[sourceFile]
		updated := 0
		for i := range records {
			record := records[i]
			
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
			if seed, ok := writes[key]; ok {
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
		newData, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output JSON: %w", err)
		}

		if err := os.WriteFile(outputFile, newData, 0644); err != nil {
			return fmt.Errorf("failed to write output JSON %s: %w", outputFile, err)
		}

		fmt.Printf("Successfully updated %d records in %s (total: %d)\n", updated, filepath.Base(outputFile), len(records))
		
		// Clear writes for this file
		delete(sw.pendingWrites, sourceFile)
	}

	return nil
}

// DualSeedWriter handles dual persistence to JSON and Arrow
type DualSeedWriter struct {
	jsonWriter  *JSONSeedWriter
	arrowWriter *ArrowSeedWriter
}

// NewDualSeedWriter creates a new DualSeedWriter
func NewDualSeedWriter(dataPath string) *DualSeedWriter {
	return &DualSeedWriter{
		jsonWriter:  NewJSONSeedWriter(dataPath),
		arrowWriter: NewArrowSeedWriter(dataPath),
	}
}

// AddSeedWrite queues a win
func (dsw *DualSeedWriter) AddSeedWrite(sourceFile string, slots [12]uint32, targetTokenID int32, bestSeed []byte) error {
	if err := dsw.jsonWriter.AddSeedWrite(sourceFile, slots, targetTokenID, bestSeed); err != nil {
		return err
	}
	return dsw.arrowWriter.AddSeedWrite(sourceFile, slots, targetTokenID, bestSeed)
}

// WriteBack commits all pending wins
func (dsw *DualSeedWriter) WriteBack() error {
	if err := dsw.jsonWriter.WriteBack(); err != nil {
		return err
	}
	return dsw.arrowWriter.WriteBack()
}
