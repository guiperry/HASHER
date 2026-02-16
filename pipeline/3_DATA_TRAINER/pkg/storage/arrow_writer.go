package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lab/hasher/data-trainer/pkg/training"
)

// ArrowSeedWriter handles incremental updates to Arrow files
type ArrowSeedWriter struct {
	dataPath      string
	mu            sync.Mutex
	pendingWrites map[string]map[string][]byte
	cachedRecords map[string][]*training.TrainingRecord
}

// NewArrowSeedWriter creates a new ArrowSeedWriter
func NewArrowSeedWriter(dataPath string) *ArrowSeedWriter {
	return &ArrowSeedWriter{
		dataPath:      dataPath,
		pendingWrites: make(map[string]map[string][]byte),
		cachedRecords: make(map[string][]*training.TrainingRecord),
	}
}

// generateAsicKey creates a unique key based on 12 ASIC slots and target token ID
func (aw *ArrowSeedWriter) generateAsicKey(slots [12]uint32, targetTokenID int32) string {
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d:%d",
		slots[0], slots[1], slots[2], slots[3],
		slots[4], slots[5], slots[6], slots[7],
		slots[8], slots[9], slots[10], slots[11],
		targetTokenID)
}

// AddSeedWrite queues a win
func (aw *ArrowSeedWriter) AddSeedWrite(sourceFile string, slots [12]uint32, targetTokenID int32, bestSeed []byte) error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	// Normalize sourceFile to its base name
	sourceFile = filepath.Base(sourceFile)
	if sourceFile == "" || sourceFile == "." || sourceFile == "synthetic" {
		sourceFile = "training_frames.arrow"
	}

	if _, ok := aw.pendingWrites[sourceFile]; !ok {
		aw.pendingWrites[sourceFile] = make(map[string][]byte)
	}

	key := aw.generateAsicKey(slots, targetTokenID)
	aw.pendingWrites[sourceFile][key] = bestSeed
	return nil
}

// WriteBack commits all pending wins incrementally
func (aw *ArrowSeedWriter) WriteBack() error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	if len(aw.pendingWrites) == 0 {
		return nil
	}

	for sourceFile, writes := range aw.pendingWrites {
		if len(writes) == 0 {
			continue
		}

		// Force .arrow extension for ArrowSeedWriter
		base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
		// Avoid double-suffixing
		cleanBase := strings.TrimSuffix(base, "_with_seeds")
		outputFile := filepath.Join(aw.dataPath, "frames", cleanBase+"_with_seeds.arrow")
		
		// Search for source file in common locations
		potentialPaths := []string{
			filepath.Join(aw.dataPath, "frames", base+".arrow"),
			filepath.Join(aw.dataPath, "json", base+".arrow"),
			filepath.Join(aw.dataPath, "frames/archive", base+".arrow"),
		}

		sourcePath := ""
		for _, p := range potentialPaths {
			if _, err := os.Stat(p); err == nil {
				sourcePath = p
				break
			}
		}

		// 1. Initialize cache for this file if empty
		if _, ok := aw.cachedRecords[sourceFile]; !ok {
			// Prefer output file if it exists (incremental update)
			readPath := outputFile
			if _, err := os.Stat(readPath); os.IsNotExist(err) {
				readPath = sourcePath
			}
			
			// Safety check
			if readPath == "" {
				fmt.Printf("[WARN] ArrowSeedWriter: No source path found for %s among %v\n", sourceFile, potentialPaths)
				continue
			}
			if _, err := os.Stat(readPath); os.IsNotExist(err) {
				fmt.Printf("[WARN] ArrowSeedWriter: Source file not found: %s\n", readPath)
				continue
			}

			fmt.Printf("[DEBUG] ArrowSeedWriter: Initializing cache from %s\n", readPath)
			records, err := ReadTrainingRecordsFromArrowIPC(readPath)
			if err != nil {
				return fmt.Errorf("failed to read Arrow source %s: %w", readPath, err)
			}
			aw.cachedRecords[sourceFile] = records
		}

		// 2. Update records
		records := aw.cachedRecords[sourceFile]
		updated := 0
		for _, record := range records {
			key := aw.generateAsicKey(record.FeatureVector, record.TargetToken)

			if seed, ok := writes[key]; ok {
				record.BestSeed = seed
				updated++
			} else {
				// Fallback matching by TargetTokenID
				for qKey, seed := range writes {
					if strings.HasSuffix(qKey, fmt.Sprintf(":%d", record.TargetToken)) {
						record.BestSeed = seed
						updated++
						break
					}
				}
			}
		}

		// 3. Write back
		if err := WriteTrainingRecordsToArrowIPC(outputFile, records); err != nil {
			return fmt.Errorf("failed to write output Arrow file %s: %w", outputFile, err)
		}

		fmt.Printf("Successfully updated %d records in %s (total: %d)\n", updated, filepath.Base(outputFile), len(records))
		
		// Clear writes for this file
		delete(aw.pendingWrites, sourceFile)
	}

	return nil
}
