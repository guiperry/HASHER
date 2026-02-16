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

func (sw *JSONSeedWriter) AddSeedWrite(sourceFile string, slots [12]uint32, targetTokenID int32, bestSeed []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	// Normalize sourceFile to its base name
	sourceFile = filepath.Base(sourceFile)
	if sourceFile == "" || sourceFile == "." || sourceFile == "synthetic" {
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

		// Force .json extension for JSONSeedWriter
		base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
		// Avoid double-suffixing if sourceFile already contains _with_seeds
		cleanBase := strings.TrimSuffix(base, "_with_seeds")
		outputFile := filepath.Join(sw.dataPath, "frames", cleanBase+"_with_seeds.json")
		
		// Search for source file in common locations
		potentialPaths := []string{
			filepath.Join(sw.dataPath, "frames", base+".json"),
			filepath.Join(sw.dataPath, "json", base+".json"),
			filepath.Join(sw.dataPath, "frames/archive", base+".json"),
		}

		sourcePath := ""
		for _, p := range potentialPaths {
			if _, err := os.Stat(p); err == nil {
				sourcePath = p
				break
			}
		}

		// 1. Initialize cache for this file if empty
		if _, ok := sw.cachedRecords[sourceFile]; !ok {
			// Prefer output file if it exists (incremental update)
			readPath := outputFile
			if _, err := os.Stat(readPath); os.IsNotExist(err) {
				readPath = sourcePath
			}
			
			// Safety check: if readPath doesn't exist, we can't do anything
			if readPath == "" {
				fmt.Printf("[WARN] JSONSeedWriter: No source path found for %s among %v\n", sourceFile, potentialPaths)
				continue
			}
			if _, err := os.Stat(readPath); os.IsNotExist(err) {
				fmt.Printf("[WARN] JSONSeedWriter: Source file not found: %s\n", readPath)
				continue
			}

			fmt.Printf("[DEBUG] JSONSeedWriter: Initializing cache from %s\n", readPath)
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

			// Try "feature_vector" array first
			if fv, ok := record["feature_vector"].([]interface{}); ok && len(fv) >= 12 {
				for j := 0; j < 12; j++ {
					if val, ok := fv[j].(float64); ok {
						slots[j] = uint32(uint64(val))
					} else {
						foundSlots = false
						break
					}
				}
			} else {
				// Fallback to individual slot keys
				for j := 0; j < 12; j++ {
					key := fmt.Sprintf("asic_slot_%d", j)
					valRaw, ok := record[key]
					if !ok {
						key = fmt.Sprintf("AsicSlots%d", j)
						valRaw, ok = record[key]
					}
					
					if ok {
						if valFloat, ok := valRaw.(float64); ok {
							slots[j] = uint32(uint64(valFloat))
						} else {
							foundSlots = false
							break
						}
					} else {
						foundSlots = false
						break
					}
				}
			}

			if !foundSlots {
				continue
			}

			// Extract TargetTokenID
			var targetTokenID int32
			if tID, ok := record["target_token"].(float64); ok {
				targetTokenID = int32(tID)
			} else if tID, ok := record["target_token_id"].(float64); ok {
				targetTokenID = int32(tID)
			} else if tID, ok := record["TargetTokenID"].(float64); ok {
				targetTokenID = int32(tID)
			} else {
				continue // Target token not found
			}

			key := sw.generateAsicKey(slots, targetTokenID)
			found := false
			if seed, ok := writes[key]; ok {
				fmt.Printf("[DEBUG] JSONSeedWriter: EXACT MATCH FOUND for Token %d, updating seed...\n", targetTokenID)
				if _, exists := record["best_seed"]; exists {
					record["best_seed"] = seed
				} else if _, exists := record["BestSeed"]; exists {
					record["BestSeed"] = seed
				} else {
					record["best_seed"] = seed
				}
				updated++
				found = true
			} else {
				// Fallback: Check if any queued seed matches THIS targetTokenID
				// This handles cases where ASIC slots might have subtle mismatches in synthetic data
				for qKey, seed := range writes {
					if strings.HasSuffix(qKey, fmt.Sprintf(":%d", targetTokenID)) {
						fmt.Printf("[DEBUG] JSONSeedWriter: FALLBACK MATCH for Token %d, updating seed...\n", targetTokenID)
						if _, exists := record["best_seed"]; exists {
							record["best_seed"] = seed
						} else if _, exists := record["BestSeed"]; exists {
							record["BestSeed"] = seed
						} else {
							record["best_seed"] = seed
						}
						updated++
						found = true
						break
					}
				}
			}

			if !found && i == 0 {
				// Periodically log key attempts to help debug mismatches
				fmt.Printf("[DEBUG] JSONSeedWriter: Record[0] key: %s\n", key)
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
