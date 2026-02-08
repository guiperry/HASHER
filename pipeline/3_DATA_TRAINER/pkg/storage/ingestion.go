package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	trainingpkg "github.com/lab/hasher/data-trainer/pkg/training"
)

type DataIngestor struct {
	basePath      string
	currentFile   string
	fileIndex     int
	totalRecords  int
	currentRecord *trainingpkg.TrainingRecord
}

type ParquetReader struct {
	reader io.Reader
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
}

func NewDataIngestor(basePath string) *DataIngestor {
	return &DataIngestor{
		basePath:     basePath,
		currentFile:  "",
		fileIndex:    -1,
		totalRecords: 0,
	}
}

func (di *DataIngestor) GetAvailableFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(di.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
	return files, nil
}

func (di *DataIngestor) GetNextFile() (string, error) {
	files, err := di.GetAvailableFiles()
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no parquet files found in %s", di.basePath)
	}

	di.fileIndex++
	if di.fileIndex >= len(files) {
		return "", fmt.Errorf("no more files to process")
	}

	di.currentFile = files[di.fileIndex]
	return di.currentFile, nil
}

func (di *DataIngestor) HasMoreFiles() bool {
	files, err := di.GetAvailableFiles()
	if err != nil {
		return false
	}

	return di.fileIndex+1 < len(files)
}

func (di *DataIngestor) ReadTrainingRecords(filePath string) ([]*trainingpkg.TrainingRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var records []*trainingpkg.TrainingRecord

	for decoder.More() {
		var recordData map[string]interface{}
		if err := decoder.Decode(&recordData); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		record, err := di.parseTrainingRecord(recordData)
		if err != nil {
			continue
		}

		records = append(records, record)
	}

	return records, nil
}

func (di *DataIngestor) parseTrainingRecord(data map[string]interface{}) (*trainingpkg.TrainingRecord, error) {
	record := &trainingpkg.TrainingRecord{}

	// Parse token sequence
	if tokenSeq, ok := data["token_sequence"].([]interface{}); ok {
		for _, token := range tokenSeq {
			if tokenFloat, ok := token.(float64); ok {
				record.TokenSequence = append(record.TokenSequence, int32(tokenFloat))
			} else if tokenInt, ok := token.(int); ok {
				record.TokenSequence = append(record.TokenSequence, int32(tokenInt))
			}
		}
	}

	// Parse feature vector
	if featureVec, ok := data["feature_vector"].([]interface{}); ok && len(featureVec) == 12 {
		for i, feature := range featureVec {
			if featureFloat, ok := feature.(float64); ok {
				record.FeatureVector[i] = uint32(featureFloat)
			} else if featureInt, ok := feature.(int); ok {
				record.FeatureVector[i] = uint32(featureInt)
			}
		}
	}

	// Parse target token
	if targetToken, ok := data["target_token"].(float64); ok {
		record.TargetToken = int32(targetToken)
	} else if targetToken, ok := data["target_token"].(int); ok {
		record.TargetToken = int32(targetToken)
	}

	// Parse context hash
	if contextHash, ok := data["context_hash"].(float64); ok {
		record.ContextHash = uint32(contextHash)
	} else if contextHash, ok := data["context_hash"].(int); ok {
		record.ContextHash = uint32(contextHash)
	}

	// Validate record
	if !record.Validate() {
		return nil, fmt.Errorf("invalid training record")
	}

	return record, nil
}

func (di *DataIngestor) ProcessAllFiles(progressChan chan<- *IngestionStats) ([]*trainingpkg.TrainingRecord, error) {
	var allRecords []*trainingpkg.TrainingRecord
	var stats IngestionStats

	stats.StartTime = time.Now()
	availableFiles, err := di.GetAvailableFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get available files: %w", err)
	}

	stats.TotalFiles = len(availableFiles)
	stats.RecordsPerFile = make([]int64, 0, len(availableFiles))

	for di.HasMoreFiles() {
		filePath, err := di.GetNextFile()
		if err != nil {
			return nil, fmt.Errorf("failed to get next file: %w", err)
		}

		records, err := di.ReadTrainingRecords(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		allRecords = append(allRecords, records...)

		stats.ProcessedFiles++
		stats.ProcessedRecords = stats.ProcessedRecords + int64(len(records))
		stats.RecordsPerFile = append(stats.RecordsPerFile, int64(len(records)))
		stats.CurrentFile = filepath.Base(filePath)
		stats.LastUpdateTime = time.Now()

		if progressChan != nil {
			statsCopy := stats
			progressChan <- &statsCopy
		}
	}

	stats.TotalRecords = stats.ProcessedRecords
	stats.CurrentFile = ""
	stats.LastUpdateTime = time.Now()

	if progressChan != nil {
		progressChan <- &stats
		close(progressChan)
	}

	return allRecords, nil
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
	files, err := di.GetAvailableFiles()
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

func (di *DataIngestor) ValidateTrainingData(records []*trainingpkg.TrainingRecord) (*ValidationReport, error) {
	if len(records) == 0 {
		return &ValidationReport{Valid: true, Message: "No records to validate"}, nil
	}

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
				report.Issues = append(report.Issues, "Missing token sequence")
			}

			if record.FeatureVector == [12]uint32{} {
				report.ZeroFeatureVectors++
				report.Issues = append(report.Issues, "Zero feature vector")
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
				report.Issues = append(report.Issues, "Missing feature vector values")
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
		report.Message = "Validation failed: " + strings.Join(report.Issues, "; ")
	} else {
		report.Message = "All records valid"
	}

	return report, nil
}

func (di *DataIngestor) CreateSampleOutput(records []*trainingpkg.TrainingRecord, outputPath string, limit int) error {
	if len(records) == 0 {
		return fmt.Errorf("no records to sample")
	}

	sampleSize := limit
	if sampleSize > len(records) {
		sampleSize = len(records)
	}

	sampleRecords := records[:sampleSize]

	outputData := map[string]interface{}{
		"sample_size":   sampleSize,
		"total_records": len(records),
		"records":       sampleRecords,
		"generated_at":  time.Now().Unix(),
		"sample_ratio":  float64(sampleSize) / float64(len(records)),
	}

	data, err := json.MarshalIndent(outputData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sample data: %w", err)
	}

	return os.WriteFile(outputPath, data, 0644)
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
		return fmt.Sprintf("✅ Validation passed: %d records processed", vr.TotalRecords)
	} else {
		return fmt.Sprintf("❌ Validation failed: %s", vr.Message)
	}
}
