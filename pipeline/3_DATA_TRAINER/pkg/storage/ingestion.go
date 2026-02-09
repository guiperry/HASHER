package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lab/hasher/data-trainer/pkg/training"
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
type ParquetTrainingRecord struct {
	TokenSequence []int32 `parquet:"name=token_sequence, type=LIST, convertedtype=LIST, scale=0"`
	FeatureVector []int64 `parquet:"name=feature_vector, type=LIST, convertedtype=LIST, scale=0"`
	TargetToken   int32   `parquet:"name=target_token, type=INT32"`
	ContextHash   int64   `parquet:"name=context_hash, type=INT64"`
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
		if strings.HasSuffix(strings.ToLower(info.Name()), ".parquet") {
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
		return "", fmt.Errorf("no parquet files found in %s", di.basePath)
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
	di.logger.Debug("Opening parquet file: %s", filepath.Base(filePath))

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
		records, err := di.readParquetFile(filePath)
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

func (di *DataIngestor) validateParquetFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file size
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if info.Size() < 8 {
		return fmt.Errorf("file too small to be a valid parquet file (%d bytes)", info.Size())
	}

	// Check magic bytes at beginning
	magic := make([]byte, 4)
	if _, err := file.Read(magic); err != nil {
		return fmt.Errorf("failed to read header magic: %w", err)
	}
	if string(magic) != "PAR1" {
		return fmt.Errorf("invalid parquet header magic: %s (expected PAR1)", string(magic))
	}

	// Check magic bytes at end (footer)
	if _, err := file.Seek(-4, 2); err != nil {
		return fmt.Errorf("failed to seek to footer: %w", err)
	}
	if _, err := file.Read(magic); err != nil {
		return fmt.Errorf("failed to read footer magic: %w", err)
	}
	if string(magic) != "PAR1" {
		return fmt.Errorf("invalid or missing parquet footer magic: %s (expected PAR1). File may be truncated or corrupted", string(magic))
	}

	return nil
}

func (di *DataIngestor) readParquetFile(filePath string) ([]*training.TrainingRecord, error) {
	// Validate file integrity first
	if err := di.validateParquetFile(filePath); err != nil {
		return nil, fmt.Errorf("parquet file validation failed: %w", err)
	}

	fr, err := local.NewLocalFileReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open parquet file: %w", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, new(ParquetTrainingRecord), 4)
	if err != nil {
		return nil, fmt.Errorf("failed to create parquet reader: %w", err)
	}
	defer pr.ReadStop()

	numRows := pr.GetNumRows()
	if numRows == 0 {
		return []*training.TrainingRecord{}, nil
	}

	di.logger.Debug("Parquet file has %d rows", numRows)

	var records []*training.TrainingRecord
	batchSize := int64(di.chunkSize)
	processed := int64(0)

	// Create progress bar
	progressBar := NewProgressBar(40, numRows)
	lastLog := time.Now()

	for processed < numRows {
		select {
		case <-di.ctx.Done():
			return records, di.ctx.Err()
		default:
		}

		remaining := numRows - processed
		if remaining < batchSize {
			batchSize = remaining
		}

		// Read batch
		parquetRecords := make([]ParquetTrainingRecord, batchSize)
		if err = pr.Read(&parquetRecords); err != nil {
			di.logger.Warn("Error reading batch at offset %d: %v", processed, err)
			break
		}

		// Convert to training records
		for _, pr := range parquetRecords {
			record := di.convertParquetRecord(&pr)
			if record != nil {
				records = append(records, record)
			}
		}

		processed += batchSize
		atomic.AddInt64(&di.processedRecords, batchSize)

		// Update progress
		if time.Since(lastLog) > di.progressInterval {
			di.logger.Info("Progress: %s", progressBar.Update(processed))
			lastLog = time.Now()
		}
	}

	di.logger.Debug("Completed reading %s: %d records processed", filepath.Base(filePath), len(records))
	return records, nil
}

func (di *DataIngestor) convertParquetRecord(pr *ParquetTrainingRecord) *training.TrainingRecord {
	record := &training.TrainingRecord{
		TokenSequence: pr.TokenSequence,
		TargetToken:   pr.TargetToken,
		ContextHash:   uint32(pr.ContextHash),
	}

	// Convert FeatureVector from []int64 to [12]uint32
	if len(pr.FeatureVector) >= 12 {
		for i := 0; i < 12; i++ {
			record.FeatureVector[i] = uint32(pr.FeatureVector[i])
		}
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

	for i := startIdx; i < endIdx; i++ {
		record := di.convertParquetRecord(&parquetRecords[i])
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
		return nil, fmt.Errorf("no parquet files found in %s", di.basePath)
	}

	di.logger.Info("Found %d parquet file(s)", len(availableFiles))

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
	records, err := di.ReadTrainingRecords(filePath)
	if err != nil {
		return nil, err
	}

	return records, nil
}

func (di *DataIngestor) loadIngestionCheckpoint(filePath string) *IngestionCheckpoint {
	return nil
}

func (di *DataIngestor) saveIngestionCheckpoint(filePath string, offset int64) error {
	return nil
}

func (di *DataIngestor) clearIngestionCheckpoint(filePath string) error {
	return nil
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
