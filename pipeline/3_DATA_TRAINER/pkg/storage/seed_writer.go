package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
	"github.com/xitongsys/parquet-go/writer"
)

// SeedTrainingRecord matches the schema from 2_DATA_ENCODER/pkg/schema/output.go
type SeedTrainingRecord struct {
	SourceFile    string `parquet:"name=source_file, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	ChunkID       int32  `parquet:"name=chunk_id, type=INT32"`
	WindowStart   int32  `parquet:"name=window_start, type=INT32"`
	WindowEnd     int32  `parquet:"name=window_end, type=INT32"`
	ContextLength int32  `parquet:"name=context_length, type=INT32"`
	// ASIC input slots (12 x 4 bytes = 48 bytes)
	AsicSlots0    int32  `parquet:"name=asic_slot_0, type=INT32"`
	AsicSlots1    int32  `parquet:"name=asic_slot_1, type=INT32"`
	AsicSlots2    int32  `parquet:"name=asic_slot_2, type=INT32"`
	AsicSlots3    int32  `parquet:"name=asic_slot_3, type=INT32"`
	AsicSlots4    int32  `parquet:"name=asic_slot_4, type=INT32"`
	AsicSlots5    int32  `parquet:"name=asic_slot_5, type=INT32"`
	AsicSlots6    int32  `parquet:"name=asic_slot_6, type=INT32"`
	AsicSlots7    int32  `parquet:"name=asic_slot_7, type=INT32"`
	AsicSlots8    int32  `parquet:"name=asic_slot_8, type=INT32"`
	AsicSlots9    int32  `parquet:"name=asic_slot_9, type=INT32"`
	AsicSlots10   int32  `parquet:"name=asic_slot_10, type=INT32"`
	AsicSlots11   int32  `parquet:"name=asic_slot_11, type=INT32"`
	TargetTokenID int32  `parquet:"name=target_token_id, type=INT32"`
	BestSeed      string `parquet:"name=best_seed, type=BYTE_ARRAY, convertedtype=UTF8"`
}

// SeedWriter handles writing best seeds to a new training_frames_with_seeds.parquet file
// Note: Parquet files are immutable, so we create a new file with updated seeds
type SeedWriter struct {
	sourceFile    string
	outputFile    string
	tempFile      string
	mu            sync.RWMutex
	pendingWrites map[int32]string // token_id -> best_seed mapping
}

// NewSeedWriter creates a new SeedWriter for the given source file
func NewSeedWriter(sourceFile string) *SeedWriter {
	// Output to data-trainer's data directory instead of trying to modify source
	outputFile := filepath.Join("data", "training_frames_with_seeds.parquet")
	return &SeedWriter{
		sourceFile:    sourceFile,
		outputFile:    outputFile,
		tempFile:      outputFile + ".tmp",
		pendingWrites: make(map[int32]string),
	}
}

// AddSeedWrite queues a best seed to be written back for the given token
func (sw *SeedWriter) AddSeedWrite(tokenID int32, bestSeed []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed for token %d", tokenID)
	}

	sw.pendingWrites[tokenID] = string(bestSeed)

	return nil
}

// HasPendingWrite returns true if there are pending writes for the given token
func (sw *SeedWriter) HasPendingWrite(tokenID int32) bool {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	_, exists := sw.pendingWrites[tokenID]
	return exists
}

// GetPendingWriteCount returns the number of pending writes
func (sw *SeedWriter) GetPendingWriteCount() int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	return len(sw.pendingWrites)
}

// WriteBack commits all pending writes to a new parquet file with seeds
func (sw *SeedWriter) WriteBack() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.pendingWrites) == 0 {
		return nil // Nothing to write
	}

	// Verify source file exists
	if _, err := os.Stat(sw.sourceFile); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist: %s", sw.sourceFile)
	}

	// Read original file and create new file with best seeds
	if err := sw.createUpdatedParquetFile(); err != nil {
		return fmt.Errorf("failed to create updated parquet file: %w", err)
	}

	// Clear pending writes after successful write
	sw.pendingWrites = make(map[int32]string)

	fmt.Printf("Successfully wrote updated training data to: %s\n", sw.outputFile)
	return nil
}

// createUpdatedParquetFile reads the original parquet file and creates a new one with updated best seeds
func (sw *SeedWriter) createUpdatedParquetFile() error {
	// Ensure data directory exists
	if err := os.MkdirAll(filepath.Dir(sw.outputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Open original file for reading
	fr, err := local.NewLocalFileReader(sw.sourceFile)
	if err != nil {
		return fmt.Errorf("failed to open parquet file for reading: %w", err)
	}
	defer fr.Close()

	// Create parquet reader using the correct schema
	pr, err := reader.NewParquetReader(fr, new(SeedTrainingRecord), 4)
	if err != nil {
		return fmt.Errorf("failed to create parquet reader: %w", err)
	}
	defer pr.ReadStop()

	numRows := pr.GetNumRows()
	if numRows == 0 {
		return fmt.Errorf("source parquet file is empty")
	}

	// Create temporary file for writing
	fw, err := local.NewLocalFileWriter(sw.tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temporary file writer: %w", err)
	}
	defer fw.Close()

	// Create parquet writer using the correct schema
	pw, err := writer.NewParquetWriter(fw, new(SeedTrainingRecord), 4)
	if err != nil {
		return fmt.Errorf("failed to create parquet writer: %w", err)
	}
	defer pw.WriteStop()

	// Read all records and update best seeds
	parquetRecords := make([]SeedTrainingRecord, numRows)
	if err := pr.Read(&parquetRecords); err != nil {
		return fmt.Errorf("failed to read parquet records: %w", err)
	}

	// Update records with pending best seeds
	updatedCount := 0
	for i := range parquetRecords {
		record := &parquetRecords[i]
		if bestSeed, exists := sw.pendingWrites[record.TargetTokenID]; exists {
			// Update seed directly as string (matches schema)
			record.BestSeed = bestSeed
			updatedCount++
		}

		// Write the record to temporary file
		if err := pw.Write(record); err != nil {
			return fmt.Errorf("failed to write record %d: %w", i, err)
		}
	}

	// Close writer properly
	if err := pw.WriteStop(); err != nil {
		return fmt.Errorf("failed to stop parquet writer: %w", err)
	}

	// Replace temporary file with final output file
	if err := os.Remove(sw.tempFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove temporary file: %w", err)
	}

	if err := os.Rename(sw.tempFile, sw.outputFile); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	fmt.Printf("Updated %d records with best seeds\n", updatedCount)
	return nil
}

// GetOutputFile returns the output file path
func (sw *SeedWriter) GetOutputFile() string {
	return sw.outputFile
}

// GetPendingWrites returns a copy of pending writes (for debugging)
func (sw *SeedWriter) GetPendingWrites() map[int32]string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	result := make(map[int32]string)
	for token, seed := range sw.pendingWrites {
		result[token] = seed
	}

	return result
}
