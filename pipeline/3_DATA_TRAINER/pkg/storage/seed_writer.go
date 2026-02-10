package storage

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
	"github.com/xitongsys/parquet-go/writer"
)

// SeedWriter handles writing best seeds back to the source training_frames.parquet file
type SeedWriter struct {
	sourceFile    string
	tempFile      string
	backupFile    string
	mu            sync.RWMutex
	pendingWrites map[int32][]byte // token_id -> best_seed mapping
}

// NewSeedWriter creates a new SeedWriter for the given source file
func NewSeedWriter(sourceFile string) *SeedWriter {
	return &SeedWriter{
		sourceFile:    sourceFile,
		tempFile:      sourceFile + ".tmp",
		backupFile:    sourceFile + ".backup." + fmt.Sprintf("%d", time.Now().Unix()),
		pendingWrites: make(map[int32][]byte),
	}
}

// AddSeedWrite queues a best seed to be written back for the given token
func (sw *SeedWriter) AddSeedWrite(tokenID int32, bestSeed []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(bestSeed) == 0 {
		return fmt.Errorf("cannot write empty seed for token %d", tokenID)
	}

	sw.pendingWrites[tokenID] = make([]byte, len(bestSeed))
	copy(sw.pendingWrites[tokenID], bestSeed)

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

// WriteBack commits all pending writes to the source parquet file
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

	// Create backup of original file
	if err := sw.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read original file and update with best seeds
	if err := sw.updateParquetFile(); err != nil {
		// Attempt to restore from backup
		if restoreErr := sw.restoreFromBackup(); restoreErr != nil {
			return fmt.Errorf("failed to update file and restore backup: update_error=%w, restore_error=%w", err, restoreErr)
		}
		return fmt.Errorf("failed to update parquet file (backup restored): %w", err)
	}

	// Clear pending writes after successful write
	sw.pendingWrites = make(map[int32][]byte)

	// Clean up backup file
	os.Remove(sw.backupFile)

	return nil
}

// createBackup creates a backup of the source file
func (sw *SeedWriter) createBackup() error {
	source, err := os.Open(sw.sourceFile)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	backup, err := os.Create(sw.backupFile)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer backup.Close()

	buf := make([]byte, 64*1024) // 64KB buffer
	for {
		n, err := source.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read source file: %w", err)
		}

		if _, err := backup.Write(buf[:n]); err != nil {
			return fmt.Errorf("failed to write backup file: %w", err)
		}
	}

	return nil
}

// restoreFromBackup restores the source file from backup
func (sw *SeedWriter) restoreFromBackup() error {
	backup, err := os.Open(sw.backupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer backup.Close()

	source, err := os.Create(sw.sourceFile)
	if err != nil {
		return fmt.Errorf("failed to recreate source file: %w", err)
	}
	defer source.Close()

	buf := make([]byte, 64*1024) // 64KB buffer
	for {
		n, err := backup.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read backup file: %w", err)
		}

		if _, err := source.Write(buf[:n]); err != nil {
			return fmt.Errorf("failed to restore source file: %w", err)
		}
	}

	return nil
}

// updateParquetFile reads the original parquet file and writes a new one with updated best seeds
func (sw *SeedWriter) updateParquetFile() error {
	// Open original file for reading
	fr, err := local.NewLocalFileReader(sw.sourceFile)
	if err != nil {
		return fmt.Errorf("failed to open parquet file for reading: %w", err)
	}
	defer fr.Close()

	// Create parquet reader
	pr, err := reader.NewParquetReader(fr, new(ParquetTrainingRecord), 4)
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

	// Create parquet writer
	pw, err := writer.NewParquetWriter(fw, new(ParquetTrainingRecord), 4)
	if err != nil {
		return fmt.Errorf("failed to create parquet writer: %w", err)
	}
	defer pw.WriteStop()

	// Read all records and update best seeds
	parquetRecords := make([]ParquetTrainingRecord, numRows)
	if err := pr.Read(&parquetRecords); err != nil {
		return fmt.Errorf("failed to read parquet records: %w", err)
	}

	// Update records with pending best seeds
	updatedCount := 0
	for i := range parquetRecords {
		record := &parquetRecords[i]
		if bestSeed, exists := sw.pendingWrites[record.TargetTokenID]; exists {
			// Convert byte seed to hex string for storage
			record.BestSeed = hex.EncodeToString(bestSeed)
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

	// Replace original file with temporary file
	if err := os.Remove(sw.sourceFile); err != nil {
		return fmt.Errorf("failed to remove original file: %w", err)
	}

	if err := os.Rename(sw.tempFile, sw.sourceFile); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// GetSourceFile returns the source file path
func (sw *SeedWriter) GetSourceFile() string {
	return sw.sourceFile
}

// GetPendingWrites returns a copy of pending writes (for debugging)
func (sw *SeedWriter) GetPendingWrites() map[int32][]byte {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	result := make(map[int32][]byte)
	for token, seed := range sw.pendingWrites {
		result[token] = make([]byte, len(seed))
		copy(result[token], seed)
	}

	return result
}
