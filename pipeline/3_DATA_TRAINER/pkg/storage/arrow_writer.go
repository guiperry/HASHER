package storage

import (
	"fmt"
	"path/filepath"
	"sync"
)

// ArrowSeedWriter handles writing best seeds back to Arrow IPC files
type ArrowSeedWriter struct {
	sourceFile    string
	outputFile    string
	mu            sync.Mutex
	pendingWrites map[string][]byte
}

// NewArrowSeedWriter creates a new ArrowSeedWriter
func NewArrowSeedWriter(sourceFile, outputFile string) *ArrowSeedWriter {
	return &ArrowSeedWriter{
		sourceFile:    sourceFile,
		outputFile:    outputFile,
		pendingWrites: make(map[string][]byte),
	}
}

// generateKey creates a unique key for a specific training frame
func (aw *ArrowSeedWriter) generateKey(sourceFile string, chunkID int32, windowStart int32) string {
	return fmt.Sprintf("%s:%d:%d", filepath.Base(sourceFile), chunkID, windowStart)
}

// AddSeedWrite queues a best seed to be written back
func (aw *ArrowSeedWriter) AddSeedWrite(sourceFile string, chunkID int32, windowStart int32, bestSeed []byte) error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed")
	}

	key := aw.generateKey(sourceFile, chunkID, windowStart)
	aw.pendingWrites[key] = bestSeed
	return nil
}

// WriteBack writes all pending seeds back to the output Arrow file
func (aw *ArrowSeedWriter) WriteBack() error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	if len(aw.pendingWrites) == 0 {
		return nil
	}

	// 1. Read existing records
	records, err := ReadTrainingRecordsFromArrowIPC(aw.sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source Arrow file: %w", err)
	}

	updated := 0
	for _, record := range records {
		key := aw.generateKey(record.SourceFile, record.ChunkID, record.WindowStart)

		if seed, ok := aw.pendingWrites[key]; ok {
			record.BestSeed = seed
			updated++
		}
	}

	// 2. Write back to new file
	if err := WriteTrainingRecordsToArrowIPC(aw.outputFile, records); err != nil {
		return fmt.Errorf("failed to write output Arrow file: %w", err)
	}

	// Clear pending writes
	aw.pendingWrites = make(map[string][]byte)
	fmt.Printf("Successfully updated %d records in %s (total: %d)\n", updated, aw.outputFile, len(records))

	return nil
}

// GetOutputFile returns the output file path
func (aw *ArrowSeedWriter) GetOutputFile() string {
	return aw.outputFile
}
