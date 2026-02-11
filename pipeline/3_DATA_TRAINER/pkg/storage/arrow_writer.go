package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/lab/hasher/data-trainer/pkg/training"
)

// ArrowSeedWriter handles incremental updates to Arrow files
type ArrowSeedWriter struct {
	sourceFile    string
	outputFile    string
	mu            sync.Mutex
	pendingWrites map[string][]byte
	cachedRecords []*training.TrainingRecord
}

// NewArrowSeedWriter creates a new ArrowSeedWriter
func NewArrowSeedWriter(sourceFile, outputFile string) *ArrowSeedWriter {
	return &ArrowSeedWriter{
		sourceFile:    sourceFile,
		outputFile:    outputFile,
		pendingWrites: make(map[string][]byte),
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
func (aw *ArrowSeedWriter) AddSeedWrite(slots [12]uint32, targetTokenID int32, bestSeed []byte) error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	key := aw.generateAsicKey(slots, targetTokenID)
	aw.pendingWrites[key] = bestSeed
	return nil
}

// WriteBack commits all pending wins incrementally
func (aw *ArrowSeedWriter) WriteBack() error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	if len(aw.pendingWrites) == 0 {
		return nil
	}

	// 1. Initialize cache if empty
	if aw.cachedRecords == nil {
		source := aw.outputFile
		if _, err := os.Stat(source); os.IsNotExist(err) {
			source = aw.sourceFile
		}
		
		fmt.Printf("[DEBUG] ArrowSeedWriter: Initializing cache from %s\n", filepath.Base(source))
		records, err := ReadTrainingRecordsFromArrowIPC(source)
		if err != nil {
			return fmt.Errorf("failed to read Arrow source: %w", err)
		}
		aw.cachedRecords = records
	}

	// 2. Update records
	updated := 0
	for _, record := range aw.cachedRecords {
		key := aw.generateAsicKey(record.FeatureVector, record.TargetToken)

		if seed, ok := aw.pendingWrites[key]; ok {
			record.BestSeed = seed
			updated++
		}
	}

	// 3. Write back
	if err := WriteTrainingRecordsToArrowIPC(aw.outputFile, aw.cachedRecords); err != nil {
		return fmt.Errorf("failed to write output Arrow file: %w", err)
	}

	// Clear pending writes but keep cache
	aw.pendingWrites = make(map[string][]byte)
	fmt.Printf("Successfully updated %d records in %s (total: %d)\n", updated, filepath.Base(aw.outputFile), len(aw.cachedRecords))

	return nil
}

// GetOutputFile returns the output file path
func (aw *ArrowSeedWriter) GetOutputFile() string {
	return aw.outputFile
}
