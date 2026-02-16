package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lab/hasher/data-trainer/pkg/training"
	"github.com/apache/arrow/go/arrow/ipc"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

const (
	DefaultChunkSize        = 1000
	DefaultProgressInterval = 5 * time.Second
	DefaultReadTimeout      = 30 * time.Second
	MaxRetries              = 3
)

// ParquetTrainingRecord represents the structure in the Parquet file
// Matches the TrainingFrame schema from 2_DATA_ENCODER
type ParquetTrainingRecord struct {
	// Metadata
	SourceFile string `parquet:"name=source_file, type=BYTE_ARRAY, convertedtype=UTF8"`
	ChunkID    int32  `parquet:"name=chunk_id, type=INT32"`

	// Window metadata
	WindowStart   int32 `parquet:"name=window_start, type=INT32"`
	WindowEnd     int32 `parquet:"name=window_end, type=INT32"`
	ContextLength int32 `parquet:"name=context_length, type=INT32"`

	// ASIC input slots (12 x 4 bytes = 48 bytes)
	AsicSlots0  int32 `parquet:"name=asic_slot_0, type=INT32"`
	AsicSlots1  int32 `parquet:"name=asic_slot_1, type=INT32"`
	AsicSlots2  int32 `parquet:"name=asic_slot_2, type=INT32"`
	AsicSlots3  int32 `parquet:"name=asic_slot_3, type=INT32"`
	AsicSlots4  int32 `parquet:"name=asic_slot_4, type=INT32"`
	AsicSlots5  int32 `parquet:"name=asic_slot_5, type=INT32"`
	AsicSlots6  int32 `parquet:"name=asic_slot_6, type=INT32"`
	AsicSlots7  int32 `parquet:"name=asic_slot_7, type=INT32"`
	AsicSlots8  int32 `parquet:"name=asic_slot_8, type=INT32"`
	AsicSlots9  int32 `parquet:"name=asic_slot_9, type=INT32"`
	AsicSlots10 int32 `parquet:"name=asic_slot_10, type=INT32"`
	AsicSlots11 int32 `parquet:"name=asic_slot_11, type=INT32"`

	// Target
	TargetTokenID int32 `parquet:"name=target_token_id, type=INT32"`

	// Seed (placeholder for Stage 3)
	BestSeed []byte `parquet:"name=best_seed, type=BYTE_ARRAY"`
}

type DataIngestor struct {
	basePath         string
	currentFile      string
	fileIndex        int
	totalRecords     int64
	processedRecords int64
	currentRecord    *training.TrainingRecord
	checkpointMgr    *CheckpointManager
	logger           IngestionLogger

	// Configuration
	chunkSize        int
	progressInterval time.Duration
	readTimeout      time.Duration

	// File list cache
	filesCache     []string
	filesCacheTime time.Time
	filesCacheMu   sync.RWMutex

	// Progress tracking
	progressMu   sync.RWMutex
	lastProgress time.Time
	currentStats *IngestionStats

	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

type IngestionLogger interface {
	Info(format string, args ...interface{})
	Debug(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

type defaultLogger struct{}

func (d *defaultLogger) Info(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}
func (d *defaultLogger) Debug(format string, args ...interface{}) {
	fmt.Printf("[DEBUG] "+format+"\n", args...)
}
func (d *defaultLogger) Warn(format string, args ...interface{}) {
	fmt.Printf("[WARN] "+format+"\n", args...)
}
func (d *defaultLogger) Error(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
}

type IngestionStats struct {
	TotalFiles       int       `json:"total_files"`
	TotalRecords     int64     `json:"total_records"`
	ProcessedFiles   int       `json:"processed_files"`
	ProcessedRecords int64     `json:"processed_records"`
	CurrentFile      string    `json:"current_file"`
	StartTime        time.Time `json:"start_time"`
	LastUpdateTime   time.Time `json:"last_update_time"`
	RecordsPerFile   []int64   `json:"records_per_file"`

	// Progress tracking
	CurrentChunk     int     `json:"current_chunk"`
	TotalChunks      int     `json:"total_chunks"`
	Phase            string  `json:"phase"`
	PercentComplete  float64 `json:"percent_complete"`
	ETA              string  `json:"eta"`
	RecordsPerSecond float64 `json:"records_per_second"`
}

type IngestionCheckpoint struct {
	FileIndex      int      `json:"file_index"`
	RecordOffset   int64    `json:"record_offset"`
	ProcessedFiles []string `json:"processed_files"`
	TotalRecords   int64    `json:"total_records"`
	Timestamp      int64    `json:"timestamp"`
}

type ProgressBar struct {
	width     int
	current   int64
	total     int64
	startTime time.Time
}

func NewProgressBar(width int, total int64) *ProgressBar {
	return &ProgressBar{
		width:     width,
		total:     total,
		startTime: time.Now(),
	}
}

func (pb *ProgressBar) Update(current int64) string {
	pb.current = current
	if pb.total == 0 {
		return "[>          ] 0%"
	}

	percent := float64(current) / float64(pb.total)
	filled := int(percent * float64(pb.width))
	empty := pb.width - filled

	bar := strings.Repeat("=", filled) + strings.Repeat("-", empty)
	percentage := int(percent * 100)

	// Calculate ETA
	elapsed := time.Since(pb.startTime)
	var eta string
	if current > 0 && percent < 1.0 {
		rate := float64(current) / elapsed.Seconds()
		remaining := float64(pb.total-current) / rate
		eta = time.Duration(remaining * float64(time.Second)).Round(time.Second).String()
	} else {
		eta = "N/A"
	}

	return fmt.Sprintf("[%s] %d%% (%d/%d) ETA: %s", bar, percentage, current, pb.total, eta)
}

func NewDataIngestor(basePath string) *DataIngestor {
	ctx, cancel := context.WithCancel(context.Background())
	return &DataIngestor{
		basePath:         basePath,
		currentFile:      "",
		fileIndex:        -1,
		totalRecords:     0,
		processedRecords: 0,
		logger:           &defaultLogger{},
		chunkSize:        DefaultChunkSize,
		progressInterval: DefaultProgressInterval,
		readTimeout:      DefaultReadTimeout,
		lastProgress:     time.Now(),
		currentStats:     &IngestionStats{},
		filesCache:       nil,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// JSONDataIngestor is similar to DataIngestor but works with JSON files
type JSONDataIngestor struct {
	*DataIngestor
}

// NewJSONDataIngestor creates a new JSONDataIngestor for JSON files
func NewJSONDataIngestor(basePath string) *JSONDataIngestor {
	di := NewDataIngestor(basePath)
	return &JSONDataIngestor{DataIngestor: di}
}

func (di *DataIngestor) SetLogger(logger IngestionLogger) {
	di.logger = logger
}

func (di *DataIngestor) SetCheckpointManager(cm *CheckpointManager) {
	di.checkpointMgr = cm
}

func (di *DataIngestor) SetChunkSize(size int) {
	di.chunkSize = size
}

// getAvailableFiles returns cached file list if available, otherwise scans directory
func (di *DataIngestor) getAvailableFiles() ([]string, error) {
	// Check cache first
	di.filesCacheMu.RLock()
	if di.filesCache != nil && time.Since(di.filesCacheTime) < 5*time.Second {
		files := di.filesCache
		di.filesCacheMu.RUnlock()
		return files, nil
	}
	di.filesCacheMu.RUnlock()

	// Scan directory
	var files []string
	err := filepath.Walk(di.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking despite errors
		}
		if info.IsDir() {
			return nil
		}
		name := strings.ToLower(info.Name())
		if strings.Contains(name, "_with_seeds") {
			return nil
		}
		if strings.HasSuffix(name, ".parquet") {
			files = append(files, path)
		} else if strings.HasSuffix(name, ".json") && !strings.Contains(name, ".checkpoint.") {
			files = append(files, path)
		} else if strings.HasSuffix(name, ".arrow") {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan directory %s: %w", di.basePath, err)
	}

	sort.Strings(files)

	// Update cache
	di.filesCacheMu.Lock()
	di.filesCache = files
	di.filesCacheTime = time.Now()
	di.filesCacheMu.Unlock()

	return files, nil
}

func (di *DataIngestor) GetAvailableFiles() ([]string, error) {
	return di.getAvailableFiles()
}

func (di *DataIngestor) GetNextFile() (string, error) {
	files, err := di.getAvailableFiles()
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no parquet or json files found in %s", di.basePath)
	}

	di.fileIndex++
	if di.fileIndex >= len(files) {
		return "", fmt.Errorf("no more files to process (processed %d of %d)", di.fileIndex, len(files))
	}

	di.currentFile = files[di.fileIndex]
	return di.currentFile, nil
}

func (di *DataIngestor) HasMoreFiles() bool {
	files, err := di.getAvailableFiles()
	if err != nil {
		return false
	}
	return di.fileIndex+1 < len(files)
}

// GetTotalRecords estimates total records across all files (for progress bar)
func (di *DataIngestor) GetTotalRecords() (int64, error) {
	files, err := di.getAvailableFiles()
	if err != nil {
		return 0, err
	}

	var total int64
	for _, file := range files {
		count, err := di.countRecordsInFile(file)
		if err != nil {
			di.logger.Debug("Failed to count records in %s: %v", filepath.Base(file), err)
			continue
		}
		total += count
	}

	return total, nil
}

func (di *DataIngestor) countRecordsInFile(filePath string) (int64, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".json" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return 0, err
		}
		var records []interface{}
		if err := json.Unmarshal(data, &records); err != nil {
			return 0, err
		}
		return int64(len(records)), nil
	} else if ext == ".arrow" {
		file, err := os.Open(filePath)
		if err != nil {
			return 0, err
		}
		defer file.Close()
		r, err := ipc.NewReader(file)
		if err != nil {
			return 0, err
		}
		defer r.Release()
		var total int64
		for r.Next() {
			batch := r.Record()
			total += batch.NumRows()
		}
		return total, nil
	}

	fr, err := local.NewLocalFileReader(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file reader: %w", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, new(ParquetTrainingRecord), 4)
	if err != nil {
		return 0, fmt.Errorf("failed to create parquet reader: %w", err)
	}
	defer pr.ReadStop()

	return pr.GetNumRows(), nil
}

func (di *DataIngestor) ReadTrainingRecords(filePath string) ([]*training.TrainingRecord, error) {
	di.logger.Debug("Opening file: %s", filepath.Base(filePath))

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	di.logger.Debug("File size: %d bytes (%s)", fileInfo.Size(), formatBytes(fileInfo.Size()))

	// Use timeout context
	ctx, cancel := context.WithTimeout(di.ctx, di.readTimeout)
	defer cancel()

	done := make(chan struct {
		records []*training.TrainingRecord
		err     error
	}, 1)

	go func() {
		var records []*training.TrainingRecord
		var err error

		if strings.HasSuffix(strings.ToLower(filePath), ".json") {
			records, err = di.readJSONFile(filePath)
		} else if strings.HasSuffix(strings.ToLower(filePath), ".arrow") {
			records, err = di.readArrowFile(filePath)
		} else {
			records, err = di.readParquetFile(filePath)
		}

		done <- struct {
			records []*training.TrainingRecord
			err     error
		}{records, err}
	}()

	select {
	case result := <-done:
		return result.records, result.err
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout reading file %s after %v", filePath, di.readTimeout)
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (di *DataIngestor) readJSONFile(filePath string) ([]*training.TrainingRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON file: %w", err)
	}
	defer file.Close()

	var jsonRecords []JSONTrainingRecord
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&jsonRecords); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	di.logger.Info("Successfully loaded %d JSON records from %s", len(jsonRecords), filepath.Base(filePath))

	// Convert JSON records to training records
	var records []*training.TrainingRecord
	fileName := filepath.Base(filePath)
	for _, jsonRec := range jsonRecords {
		// Override record's internal source file with the actual filename on disk
		jsonRec.SourceFile = fileName
		record := di.convertJSONRecord(&jsonRec)
		if record != nil {
			records = append(records, record)
		}
	}

	di.logger.Info("Successfully converted %d JSON records to training format", len(records))
	return records, nil
}

func (di *DataIngestor) readParquetFile(filePath string) ([]*training.TrainingRecord, error) {
	// Use Python to read parquet and convert to CSV that Go can read easily
	tempCSV := filePath + ".temp.csv"
	defer os.Remove(tempCSV)

	// Python script to convert parquet to CSV
	pythonScript := fmt.Sprintf(`
import pandas as pd
import sys

try:
    df = pd.read_parquet('%s')
    # Convert to simple CSV format
    df.to_csv('%s', index=False)
    print(f"SUCCESS: {len(df)} rows converted")
except Exception as e:
    print(f"ERROR: {e}")
    sys.exit(1)
`, filePath, tempCSV)

	// Execute Python conversion
	output, err := exec.Command("python3", "-c", pythonScript).Output()
	if err != nil {
		return nil, fmt.Errorf("Python conversion failed: %w", err)
	}

	if !strings.Contains(string(output), "SUCCESS") {
		return nil, fmt.Errorf("Python conversion error: %s", string(output))
	}

	// Now read the CSV file
	return di.readCSVAndConvert(tempCSV, filepath.Base(filePath))
}

// readCSVAndConvert reads CSV file and converts to training records
func (di *DataIngestor) readCSVAndConvert(csvPath string, originalFileName string) ([]*training.TrainingRecord, error) {
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	// Simple CSV parsing
	var records []*training.TrainingRecord
	lineCount := 0
	scanner := bufio.NewScanner(file)

	// Skip header line
	if scanner.Scan() {
		lineCount++
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse CSV line manually (simple approach)
		fields := strings.Split(line, ",")
		if len(fields) >= 15 { // We have at least source_file, chunk_id, asic_slots_0-11, target_token_id
			record := &training.TrainingRecord{
				SourceFile: originalFileName,
			}

			// Parse the fields we can access directly
			// This is a simplified approach for demo
			if len(fields) >= 15 {
				// target_token_id is at position 14 (after source_file and 12 asic slots)
				if targetID, err := strconv.ParseInt(strings.Trim(fields[14], "\""), 32, 64); err == nil {
					record.TargetToken = int32(targetID)
				}

				// Map ASIC slots (fields 2-13)
				if len(fields) >= 14 {
					record.FeatureVector = [12]uint32{}
					for i := 0; i < 12 && i+2 < len(fields); i++ {
						if val, err := strconv.ParseInt(strings.Trim(fields[i+2], "\""), 32, 64); err == nil {
							record.FeatureVector[i] = uint32(val)
						}
					}
				}

				// Set a default context hash for now
				record.ContextHash = 0

				if record.Validate() {
					records = append(records, record)
				}
			}
		}

		lineCount++
		if lineCount%1000 == 0 {
			di.logger.Debug("Converted %d CSV records to training format", lineCount)
		}
	}

	di.logger.Info("Successfully converted %d CSV records to training format", len(records))
	return records, scanner.Err()
}

func (di *DataIngestor) convertJSONRecord(jr *JSONTrainingRecord) *training.TrainingRecord {
	// Skip records that already have a best seed
	if len(jr.BestSeed) > 0 {
		di.logger.Debug("Skipping already trained JSON record for token %d", jr.TargetTokenID)
		return nil
	}

	record := &training.TrainingRecord{
		SourceFile:    jr.SourceFile,
		ChunkID:       jr.ChunkID,
		WindowStart:   jr.WindowStart,
		TargetToken:   jr.TargetTokenID,
		TokenSequence: jr.TokenSequence,
		ContextHash:   uint32(jr.ChunkID), // Using ChunkID as context identifier
	}

	// Map ASIC slots to FeatureVector
	record.FeatureVector = [12]uint32{
		uint32(jr.AsicSlots0), uint32(jr.AsicSlots1), uint32(jr.AsicSlots2), uint32(jr.AsicSlots3),
		uint32(jr.AsicSlots4), uint32(jr.AsicSlots5), uint32(jr.AsicSlots6), uint32(jr.AsicSlots7),
		uint32(jr.AsicSlots8), uint32(jr.AsicSlots9), uint32(jr.AsicSlots10), uint32(jr.AsicSlots11),
	}

	// Validate record
	if !record.Validate() {
		return nil
	}

	return record
}

func (di *DataIngestor) convertParquetRecord(pr *ParquetTrainingRecord, fileName string) *training.TrainingRecord {
	// Skip records that already have a best seed
	if len(pr.BestSeed) > 0 {
		di.logger.Debug("Skipping already trained Parquet record for token %d", pr.TargetTokenID)
		return nil
	}

	record := &training.TrainingRecord{
		SourceFile:    fileName,
		ChunkID:       pr.ChunkID,
		WindowStart:   pr.WindowStart,
		TargetToken:   pr.TargetTokenID,
		ContextHash:   uint32(pr.ChunkID), // Using ChunkID as context identifier
	}

	// Map ASIC slots to FeatureVector
	record.FeatureVector = [12]uint32{
		uint32(pr.AsicSlots0), uint32(pr.AsicSlots1), uint32(pr.AsicSlots2), uint32(pr.AsicSlots3),
		uint32(pr.AsicSlots4), uint32(pr.AsicSlots5), uint32(pr.AsicSlots6), uint32(pr.AsicSlots7),
		uint32(pr.AsicSlots8), uint32(pr.AsicSlots9), uint32(pr.AsicSlots10), uint32(pr.AsicSlots11),
	}

	// Validate record
	if !record.Validate() {
		return nil
	}

	return record
}

// ReadTrainingRecordsChunked reads records in chunks with checkpoint support
// Uses sequential reading without seeking to avoid "invalid argument" errors
func (di *DataIngestor) ReadTrainingRecordsChunked(filePath string, chunkOffset int64) ([]*training.TrainingRecord, bool, error) {
	// For simplicity and to avoid seek errors, we'll read the entire file
	// but only return the requested chunk. This is less efficient but more reliable.
	// In production, you'd want to use a different approach or fix the parquet reader.

	fr, err := local.NewLocalFileReader(filePath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to open parquet file: %w", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, new(ParquetTrainingRecord), 4)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create parquet reader: %w", err)
	}
	defer pr.ReadStop()

	numRows := pr.GetNumRows()
	if chunkOffset >= numRows {
		return nil, true, nil // All chunks processed
	}

	// Read records sequentially until we reach the desired chunk
	var records []*training.TrainingRecord
	batchSize := int64(di.chunkSize)
	remaining := numRows - chunkOffset
	if remaining < batchSize {
		batchSize = remaining
	}

	// Read all remaining records in one batch (skip seeking by reading sequentially)
	// This is a workaround for the seek error
	parquetRecords := make([]ParquetTrainingRecord, numRows)
	if err = pr.Read(&parquetRecords); err != nil {
		return nil, false, fmt.Errorf("error reading records: %w", err)
	}

	// Extract only the records for this chunk
	startIdx := chunkOffset
	endIdx := chunkOffset + batchSize
	if endIdx > int64(len(parquetRecords)) {
		endIdx = int64(len(parquetRecords))
	}

	fileName := filepath.Base(filePath)
	for i := startIdx; i < endIdx; i++ {
		record := di.convertParquetRecord(&parquetRecords[i], fileName)
		if record != nil {
			records = append(records, record)
		}
	}

	hasMore := chunkOffset+batchSize < numRows

	return records, hasMore, nil
}

// ProcessAllFilesWithProgress processes all files with a progress callback
func (di *DataIngestor) ProcessAllFilesWithProgress(progressCallback func(*IngestionStats)) ([]*training.TrainingRecord, error) {
	var allRecords []*training.TrainingRecord

	di.logger.Info("Starting data ingestion from %s", di.basePath)

	// Get available files once
	availableFiles, err := di.getAvailableFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get available files: %w", err)
	}

	if len(availableFiles) == 0 {
		return nil, fmt.Errorf("no parquet or json files found in %s", di.basePath)
	}

	di.logger.Info("Ingesting from files: %v", availableFiles)
	di.logger.Info("Found %d parquet/json file(s)", len(availableFiles))

	// Get total records estimate for progress bar
	totalRecords, err := di.GetTotalRecords()
	if err != nil {
		di.logger.Debug("Failed to estimate total records: %v", err)
		totalRecords = 0
	} else {
		di.logger.Debug("Estimated total records: %d", totalRecords)
	}

	// Initialize stats
	stats := &IngestionStats{
		StartTime:      time.Now(),
		TotalFiles:     len(availableFiles),
		TotalRecords:   totalRecords,
		RecordsPerFile: make([]int64, 0),
		Phase:          "ingesting",
	}

	// Create progress bar
	progressBar := NewProgressBar(40, totalRecords)

	// Process each file
	for di.HasMoreFiles() {
		select {
		case <-di.ctx.Done():
			return allRecords, di.ctx.Err()
		default:
		}

		filePath, err := di.GetNextFile()
		if err != nil {
			di.logger.Error("Failed to get next file: %v", err)
			break
		}

		stats.CurrentFile = filepath.Base(filePath)
		di.logger.Info("Processing file %d/%d: %s", stats.ProcessedFiles+1, stats.TotalFiles, stats.CurrentFile)

		// Process file with chunked reading
		fileRecords, err := di.processFileWithCheckpoints(filePath, stats, progressCallback)
		if err != nil {
			di.logger.Error("Failed to process file %s: %v", filepath.Base(filePath), err)
			continue
		}

		allRecords = append(allRecords, fileRecords...)
		stats.ProcessedFiles++
		stats.ProcessedRecords = int64(len(allRecords))
		stats.RecordsPerFile = append(stats.RecordsPerFile, int64(len(fileRecords)))
		stats.LastUpdateTime = time.Now()

		// Calculate progress
		if totalRecords > 0 {
			stats.PercentComplete = float64(stats.ProcessedRecords) / float64(totalRecords) * 100
			elapsed := time.Since(stats.StartTime)
			if stats.ProcessedRecords > 0 {
				stats.RecordsPerSecond = float64(stats.ProcessedRecords) / elapsed.Seconds()
				remaining := float64(totalRecords-stats.ProcessedRecords) / stats.RecordsPerSecond
				stats.ETA = time.Duration(remaining * float64(time.Second)).Round(time.Second).String()
			}
		}

		di.logger.Info("Progress: %s", progressBar.Update(stats.ProcessedRecords))

		if progressCallback != nil {
			statsCopy := *stats
			progressCallback(&statsCopy)
		}
	}

	// Final stats
	stats.Phase = "complete"
	stats.LastUpdateTime = time.Now()
	di.logger.Info("Data ingestion complete: %d file(s), %d records in %s",
		stats.ProcessedFiles, len(allRecords), time.Since(stats.StartTime).Round(time.Second))

	if progressCallback != nil {
		progressCallback(stats)
	}

	return allRecords, nil
}

func (di *DataIngestor) processFileWithCheckpoints(filePath string, stats *IngestionStats, progressCallback func(*IngestionStats)) ([]*training.TrainingRecord, error) {
	// Simplified approach: read entire file at once to avoid seek issues
	// In production with very large files, you'd want a more sophisticated approach
	_ = stats
	_ = progressCallback
	records, err := di.ReadTrainingRecords(filePath)
	if err != nil {
		return nil, err
	}

	return records, nil
}

// ProcessAllFiles is the original method - now delegates to ProcessAllFilesWithProgress
func (di *DataIngestor) ProcessAllFiles(progressChan chan<- *IngestionStats) ([]*training.TrainingRecord, error) {
	var callback func(*IngestionStats)
	if progressChan != nil {
		callback = func(stats *IngestionStats) {
			select {
			case progressChan <- stats:
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	records, err := di.ProcessAllFilesWithProgress(callback)

	if progressChan != nil {
		close(progressChan)
	}

	return records, err
}

func (di *DataIngestor) GetFileStats(filePath string) (*IngestionStats, error) {
	records, err := di.ReadTrainingRecords(filePath)
	if err != nil {
		return nil, err
	}

	stats := &IngestionStats{
		TotalFiles:       1,
		TotalRecords:     int64(len(records)),
		ProcessedFiles:   1,
		ProcessedRecords: int64(len(records)),
		CurrentFile:      filepath.Base(filePath),
		StartTime:        time.Now(),
		LastUpdateTime:   time.Now(),
		RecordsPerFile:   []int64{int64(len(records))},
	}

	return stats, nil
}

func (di *DataIngestor) GetIngestionStats() *IngestionStats {
	files, err := di.getAvailableFiles()
	if err != nil {
		return &IngestionStats{}
	}

	return &IngestionStats{
		TotalFiles:       len(files),
		TotalRecords:     0,
		ProcessedFiles:   0,
		ProcessedRecords: 0,
		CurrentFile:      di.currentFile,
		StartTime:        time.Now(),
		LastUpdateTime:   time.Now(),
		RecordsPerFile:   make([]int64, 0),
	}
}

func (di *DataIngestor) ValidateTrainingData(records []*training.TrainingRecord) (*ValidationReport, error) {
	if len(records) == 0 {
		return &ValidationReport{Valid: true, Message: "No records to validate"}, nil
	}

	di.logger.Debug("Validating %d training records...", len(records))

	report := &ValidationReport{
		TotalRecords:       int64(len(records)),
		ValidRecords:       0,
		InvalidRecords:     0,
		MissingTokens:      0,
		ZeroFeatureVectors: 0,
		MissingFeatureVecs: 0,
		Valid:              true,
		Issues:             make([]string, 0),
	}

	tokenMap := make(map[int32]bool)

	for _, record := range records {
		if record.Validate() {
			report.ValidRecords++
			tokenMap[record.TargetToken] = true
		} else {
			report.InvalidRecords++

			if len(record.TokenSequence) == 0 {
				report.MissingTokens++
			}

			if record.FeatureVector == [12]uint32{} {
				report.ZeroFeatureVectors++
			}

			hasAllFeatures := true
			for _, v := range record.FeatureVector {
				if v == 0 {
					hasAllFeatures = false
					break
				}
			}
			if !hasAllFeatures {
				report.MissingFeatureVecs++
			}
		}
	}

	report.UniqueTokens = len(tokenMap)
	report.TokenDistribution = make(map[int32]int)
	for _, record := range records {
		if record.Validate() {
			report.TokenDistribution[record.TargetToken]++
		}
	}

	report.Valid = report.InvalidRecords == 0
	if !report.Valid {
		report.Message = fmt.Sprintf("%d/%d invalid records", report.InvalidRecords, report.TotalRecords)
	} else {
		report.Message = fmt.Sprintf("all %d records valid", report.TotalRecords)
	}

	return report, nil
}

func (di *DataIngestor) CreateSampleOutput(records []*training.TrainingRecord, outputPath string, limit int) error {
	if len(records) == 0 {
		return fmt.Errorf("no records to sample")
	}

	sampleSize := limit
	if sampleSize > len(records) {
		sampleSize = len(records)
	}

	_ = records[:sampleSize]
	di.logger.Debug("Creating sample output with %d records", sampleSize)
	di.logger.Debug("Sample output would be written to: %s", outputPath)
	return nil
}

func (di *DataIngestor) Cancel() {
	di.cancel()
}

type ValidationReport struct {
	TotalRecords       int64         `json:"total_records"`
	ValidRecords       int           `json:"valid_records"`
	InvalidRecords     int           `json:"invalid_records"`
	UniqueTokens       int           `json:"unique_tokens"`
	MissingTokens      int           `json:"missing_tokens"`
	ZeroFeatureVectors int           `json:"zero_feature_vectors"`
	MissingFeatureVecs int           `json:"missing_feature_vecs"`
	TokenDistribution  map[int32]int `json:"token_distribution"`
	Valid              bool          `json:"valid"`
	Message            string        `json:"message"`
	Issues             []string      `json:"issues,omitempty"`
}

func (vr *ValidationReport) GetSummary() string {
	if vr.Valid {
		return fmt.Sprintf("Validation passed: %d records, %d unique tokens", vr.TotalRecords, vr.UniqueTokens)
	} else {
		return fmt.Sprintf("Validation failed: %d/%d invalid records", vr.InvalidRecords, vr.TotalRecords)
	}
}
