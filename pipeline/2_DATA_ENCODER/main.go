package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"data-encoder/pkg/analyzer"
	"data-encoder/pkg/checkpoint"
	"data-encoder/pkg/embeddings"
	"data-encoder/pkg/mapper"
	"data-encoder/pkg/schema"
	"data-encoder/pkg/sliding"
	"data-encoder/pkg/tokenizer"

	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/reader"
	"github.com/xitongsys/parquet-go/writer"
)

// Config holds the application configuration
type Config struct {
	InputFile  string
	OutputFile string
	MapperSeed int64
	NumWorkers int

	// NEW: Sliding window configuration
	WindowSize   int // Default: 128 tokens
	WindowStride int // Default: 1 (no overlap)
	BatchSize    int // Default: 32 contexts per API call

	// NEW: Checkpoint and quota configuration
	QuotaLimit       int  // Maximum embeddings quota (default: 5000)
	EnableCheckpoint bool // Enable checkpoint/resume functionality
}

func main() {
	// Parse command line flags
	config := parseFlags()

	// Validate configuration
	if err := validateConfig(config); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Run the encoder
	if err := runEncoder(config); err != nil {
		log.Fatalf("Encoding failed: %v", err)
	}

	log.Println("✅ Encoding Complete. Ready for Evo-GRPO.")
}

// getAppDataDir returns the application data directory
func getAppDataDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(homeDir, ".local", "share", "data-encoder")
}

func parseFlags() *Config {
	config := &Config{}

	// Default paths - prioritize parquet with JSON fallback
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}
	defaultParquetInput := filepath.Join(homeDir, ".local", "share", "dataminer", "ai_knowledge_base.parquet")
	defaultJSONInput := filepath.Join(homeDir, ".local", "share", "dataminer", "backup", "json", "ai_knowledge_base.json")
	defaultOutput := filepath.Join(getAppDataDir(), "training_frames.parquet")

	flag.StringVar(&config.InputFile, "input", defaultParquetInput, "Input Parquet/JSON file path")
	flag.StringVar(&config.OutputFile, "output", defaultOutput, "Output Parquet file path")
	flag.Int64Var(&config.MapperSeed, "seed", 1337, "Random seed for mapper (for reproducibility)")
	flag.IntVar(&config.NumWorkers, "workers", 4, "Number of concurrent workers for Parquet writing")

	// NEW: Sliding window configuration
	flag.IntVar(&config.WindowSize, "window-size", 128, "Sliding window size in tokens")
	flag.IntVar(&config.WindowStride, "window-stride", 1, "Stride between sliding windows")
	flag.IntVar(&config.BatchSize, "batch-size", 32, "Batch size for embedding API calls")

	// NEW: Quota and checkpoint configuration
	flag.IntVar(&config.QuotaLimit, "quota", 5000, "Maximum embeddings quota per run")
	flag.BoolVar(&config.EnableCheckpoint, "checkpoint", true, "Enable checkpoint/resume functionality")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Data Encoder - Transform embeddings into hardware-ready Neural Frames\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nDefault input files (checked in order):\n")
		fmt.Fprintf(os.Stderr, "  1. %s (Parquet - primary)\n", defaultParquetInput)
		fmt.Fprintf(os.Stderr, "  2. %s (JSON - backup)\n", defaultJSONInput)
	}

	flag.Parse()

	return config
}

func validateConfig(config *Config) error {
	// Calculate default JSON backup path
	homeDir, _ := os.UserHomeDir()
	defaultJSONPath := filepath.Join(homeDir, ".local", "share", "dataminer", "backup", "json", "ai_knowledge_base.json")

	// Detect input file with priority: parquet -> json backup -> legacy json
	detectedInput, inputType, err := detectInputFile(config.InputFile, defaultJSONPath)
	if err != nil {
		return fmt.Errorf("no valid input file found: %w", err)
	}

	config.InputFile = detectedInput
	log.Printf("📁 Detected input file type: %s (%s)", filepath.Base(detectedInput), inputType)

	// Check if input file exists
	if _, err := os.Stat(config.InputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file does not exist: %s", config.InputFile)
	}

	// Ensure application data directory exists
	appDataDir := getAppDataDir()
	if err := os.MkdirAll(appDataDir, 0755); err != nil {
		return fmt.Errorf("failed to create application data directory: %w", err)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(config.OutputFile)
	if outputDir != "" && outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	return nil
}

// detectInputFile determines the appropriate input file based on priority:
// 1. Parquet file (primary)
// 2. JSON backup file (fallback)
// 3. Legacy JSON file (final fallback)
func detectInputFile(defaultParquetPath, defaultJSONPath string) (string, string, error) {
	// Priority 1: Check if default parquet file exists
	if _, err := os.Stat(defaultParquetPath); err == nil {
		return defaultParquetPath, "parquet", nil
	}

	// Priority 2: Check if JSON backup file exists
	if _, err := os.Stat(defaultJSONPath); err == nil {
		return defaultJSONPath, "json", nil
	}

	// Priority 3: Check for legacy JSON location
	legacyJSONPath := filepath.Join(filepath.Dir(defaultJSONPath), "..", "json", "ai_knowledge_base.json")
	if _, err := os.Stat(legacyJSONPath); err == nil {
		return legacyJSONPath, "json (legacy)", nil
	}

	return "", "", fmt.Errorf("no input file found in any location")
}

// detectFileType determines if a file is parquet or JSON based on extension and magic bytes
func detectFileType(filePath string) string {
	// Check file extension first
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".parquet" {
		return "parquet"
	}
	if ext == ".json" || ext == ".jsonl" {
		return "json"
	}

	// For files with no extension or unknown extension, check magic bytes
	file, err := os.Open(filePath)
	if err != nil {
		return "unknown"
	}
	defer file.Close()

	// Read first 4 bytes to check for parquet magic number "PAR1"
	header := make([]byte, 4)
	_, err = file.Read(header)
	if err != nil {
		return "unknown"
	}

	// Parquet files start with "PAR1" magic bytes
	if string(header) == "PAR1" {
		return "parquet"
	}

	return "json" // Default to JSON for text-based files
}

// readParquetFile reads a parquet file and returns DocumentRecord chunks
func readParquetFile(filePath string) ([]schema.DocumentRecord, error) {
	fr, err := local.NewLocalFileReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open parquet file: %w", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, new(schema.DocumentRecord), 2)
	if err != nil {
		return nil, fmt.Errorf("failed to create parquet reader: %w", err)
	}
	defer pr.ReadStop()

	numRows := pr.GetNumRows()
	if numRows == 0 {
		return nil, fmt.Errorf("parquet file contains no rows")
	}

	log.Printf("📖 Reading %d DocumentRecord chunks from parquet file...", numRows)

	// Read all records into memory (could be optimized for streaming)
	records := make([]schema.DocumentRecord, numRows)
	err = pr.Read(&records)
	if err != nil {
		return nil, fmt.Errorf("failed to read parquet records: %w", err)
	}

	log.Printf("✅ Successfully read %d DocumentRecord chunks", len(records))
	return records, nil
}

// performVarianceAnalysis samples documents to identify high-variance BGE dimensions
func performVarianceAnalysis(inputFile string) ([]int, error) {
	va := analyzer.NewVarianceAnalyzer()
	fileType := detectFileType(inputFile)

	// Sample up to 1000 records for variance analysis
	maxSamples := 1000
	sampleCount := 0

	if fileType == "parquet" {
		// Read parquet records
		records, err := readParquetFile(inputFile)
		if err != nil {
			log.Printf("⚠️  Failed to read parquet for variance analysis: %v", err)
		} else {
			for _, record := range records {
				if sampleCount >= maxSamples {
					break
				}
				// Sample the embedding for variance analysis
				if len(record.Embedding) > 0 {
					if err := va.Sample(record.Embedding); err == nil {
						sampleCount++
					}
				}
			}
		}
	}

	// Calculate variance and get indices
	if err := va.Calculate(); err != nil {
		log.Printf("⚠️  Variance calculation failed: %v, using defaults", err)
		return []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23}, nil
	}

	if sampleCount > 0 {
		log.Printf("📊 Variance analysis: sampled %d documents", sampleCount)
		stats, _ := va.GetStats()
		if stats != nil {
			log.Printf("📊 Top variance: %.6f, Mean variance: %.6f", stats.TopVariance, stats.MeanVariance)
		}
	} else {
		log.Printf("⚠️  No embeddings found for variance analysis, using defaults")
	}

	return va.GetSignalIndices(), nil
}

func runEncoder(config *Config) error {
	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Handle signals in a goroutine
	go func() {
		sig := <-sigChan
		log.Printf("\n🛑 Received signal: %v. Initiating graceful shutdown...", sig)
		cancel()
	}()

	log.Printf("🔧 Initializing Data Encoder (seed=%d, workers=%d, window=%d, stride=%d, batch=%d, quota=%d)...",
		config.MapperSeed, config.NumWorkers, config.WindowSize, config.WindowStride, config.BatchSize, config.QuotaLimit)

	// 0. Initialize Checkpoint Manager (for resume capability and quota tracking)
	var cpManager *checkpoint.Manager
	if config.EnableCheckpoint {
		var err error
		cpManager, err = checkpoint.NewManager(config.OutputFile)
		if err != nil {
			log.Printf("⚠️  Failed to initialize checkpoint manager: %v", err)
			// Continue without checkpointing
			cpManager = nil
		} else {
			// Set quota limit from config
			cpManager.SetQuotaLimit(config.QuotaLimit)
			// Check if quota should be reset (new day)
			cpManager.ResetDailyQuota()
			log.Printf("✓ Checkpoint manager initialized: %s", cpManager.GetCheckpointPath())
		}
	}

	// 1. Initialize Services
	tk, err := tokenizer.New()
	if err != nil {
		return fmt.Errorf("failed to initialize tokenizer: %w", err)
	}

	mp := mapper.New(config.MapperSeed)
	ew := embeddings.NewWithBatchSize(config.BatchSize)

	log.Printf("✓ Mapper initialized with seed %d", mp.GetSeed())
	log.Printf("✓ Embeddings service initialized with batch size %d", config.BatchSize)
	log.Printf("✓ Sliding window: size=%d, stride=%d", config.WindowSize, config.WindowStride)

	// 1.5 Variance Analysis - Identify high-signal dimensions for BGE embeddings
	log.Printf("📊 Performing variance analysis on BGE embeddings...")
	varianceIndices, err := performVarianceAnalysis(config.InputFile)
	if err != nil {
		log.Printf("⚠️  Variance analysis failed: %v, using defaults", err)
		varianceIndices = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23}
	} else {
		log.Printf("✅ Variance analysis complete: using top 24 high-signal dimensions")
	}

	// Create variance-aware mapper
	varianceMapper := mapper.NewVarianceMapper(varianceIndices)
	log.Printf("✓ Variance-aware mapper initialized with %d signal indices", len(varianceIndices))

	// Check initial quota status
	if cpManager != nil {
		used, limit, remaining := cpManager.GetQuotaStatus()
		log.Printf("📊 Quota status: %d/%d used, %d remaining", used, limit, remaining)

		if remaining <= 0 {
			log.Printf("⚠️  Quota already exhausted (%d/%d). Nothing to process.", used, limit)
			return nil
		}
	}

	log.Printf("💾 Output will be saved to: %s", config.OutputFile)

	// 2. Setup Parquet Writer
	fw, err := local.NewLocalFileWriter(config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	pw, err := writer.NewParquetWriter(fw, new(schema.TrainingFrame), int64(config.NumWorkers))
	if err != nil {
		fw.Close()
		return fmt.Errorf("failed to create Parquet writer: %w", err)
	}
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	// Use WaitGroup to coordinate processing and cleanup
	var wg sync.WaitGroup
	var frameCount int64
	var processErr error
	var mu sync.Mutex

	// Start processing in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		// 3. Detect file type and read appropriate format
		fileType := detectFileType(config.InputFile)
		log.Printf("📖 Detected input file type: %s", fileType)

		if fileType == "parquet" {
			// Try to read as parquet first
			var records []schema.DocumentRecord
			records, err = readParquetFile(config.InputFile)
			if err != nil {
				// Parquet read failed, try fallback to JSON
				log.Printf("⚠️  Parquet read failed (%v), attempting JSON fallback...", err)
				frameCount, err = processJSONFallback(config.InputFile, mp, varianceMapper, tk, ew, pw, config, cpManager)
				if err != nil {
					mu.Lock()
					processErr = fmt.Errorf("both parquet and JSON processing failed: %w", err)
					mu.Unlock()
					return
				}
			} else {
				// Successfully read parquet, convert to MinedRecord format and process
				frameCount, err = processDocumentRecords(records, mp, varianceMapper, tk, ew, pw, config, cpManager)
				if err != nil {
					mu.Lock()
					processErr = fmt.Errorf("failed to process parquet records: %w", err)
					mu.Unlock()
					return
				}
			}
		} else {
			// Process as JSON format (array or JSONL)
			frameCount, err = processJSONFile(config.InputFile, mp, varianceMapper, tk, ew, pw, config, cpManager)
			if err != nil {
				mu.Lock()
				processErr = fmt.Errorf("failed to process JSON file: %w", err)
				mu.Unlock()
				return
			}
		}
	}()

	// Wait for either processing to complete or context to be cancelled
	waitChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		// Processing completed normally
	case <-ctx.Done():
		// Signal received, wait for goroutine to finish
		log.Printf("⏳ Waiting for processing to stop...")
		<-waitChan
	}

	// Save final checkpoint before cleanup
	if cpManager != nil {
		if err := cpManager.Save(); err != nil {
			log.Printf("⚠️  Failed to save final checkpoint: %v", err)
		}

		// Check if quota was exhausted
		if !cpManager.HasQuotaAvailable() {
			log.Printf("🛑 Processing stopped: quota exhausted")
		} else if ctx.Err() != nil {
			log.Printf("🛑 Processing stopped: interrupted by user")
		} else {
			log.Printf("✅ Processing completed - checkpoint saved for resume")
		}
	}

	// Explicitly flush and close the parquet writer
	log.Printf("💾 Flushing parquet writer...")
	if err := pw.WriteStop(); err != nil {
		fw.Close()
		return fmt.Errorf("failed to flush parquet writer: %w", err)
	}
	log.Printf("💾 Parquet writer flushed successfully")

	// Close the file writer
	if err := fw.Close(); err != nil {
		return fmt.Errorf("failed to close file writer: %w", err)
	}
	log.Printf("💾 File writer closed")

	// Check for processing errors
	mu.Lock()
	err = processErr
	mu.Unlock()
	if err != nil {
		return err
	}

	// Verify the output file exists and has content
	fileInfo, err := os.Stat(config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to stat output file: %w", err)
	}
	log.Printf("✅ Output file created: %s (%d bytes)", config.OutputFile, fileInfo.Size())

	log.Printf("📈 Total: %d training frames generated", frameCount)
	return nil
}

// processJSONFallback handles JSON processing when parquet fails
func processJSONFallback(filePath string, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config, cpManager *checkpoint.Manager) (int64, error) {
	log.Printf("🔄 Attempting JSON processing from: %s", filePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read JSON file: %w", err)
	}

	contentStr := string(content)
	trimmed := strings.TrimSpace(contentStr)
	isJSONArray := strings.HasPrefix(trimmed, "[")

	if isJSONArray {
		fixedContent, needsFix := fixJSONContent(contentStr)
		if needsFix {
			log.Printf("🔧 Fixed JSON array structure")
			if writeErr := os.WriteFile(filePath, []byte(fixedContent), 0644); writeErr != nil {
				log.Printf("⚠️  Failed to update fixed file: %v", writeErr)
			}
			contentStr = fixedContent
		}

		var records []schema.MinedRecord
		if err := json.Unmarshal([]byte(contentStr), &records); err != nil {
			return 0, fmt.Errorf("failed to parse JSON array: %w", err)
		}

		log.Printf("📋 Detected JSON array format with %d records", len(records))
		return processMinedRecords(&records, mp, vm, tk, ew, pw, config, cpManager)
	} else {
		log.Printf("📋 Processing as JSONL format")
		return processJSONLRecords(contentStr, mp, vm, tk, ew, pw, config, cpManager)
	}
}

// processJSONFile processes JSON files with proper error handling
func processJSONFile(filePath string, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config, cpManager *checkpoint.Manager) (int64, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read input file: %w", err)
	}

	contentStr := string(content)
	trimmed := strings.TrimSpace(contentStr)
	isJSONArray := strings.HasPrefix(trimmed, "[")

	if isJSONArray {
		fixedContent, needsFix := fixJSONContent(contentStr)
		if needsFix {
			log.Printf("🔧 Fixed JSON array structure")
			if writeErr := os.WriteFile(filePath, []byte(fixedContent), 0644); writeErr != nil {
				log.Printf("⚠️  Failed to update fixed file: %v", writeErr)
			}
			contentStr = fixedContent
		}

		var records []schema.MinedRecord
		if err := json.Unmarshal([]byte(contentStr), &records); err != nil {
			return 0, fmt.Errorf("failed to parse JSON array: %w", err)
		}

		log.Printf("📋 Detected JSON array format with %d records", len(records))
		return processMinedRecords(&records, mp, vm, tk, ew, pw, config, cpManager)
	} else {
		log.Printf("📋 Processing as JSONL format")
		return processJSONLRecords(contentStr, mp, vm, tk, ew, pw, config, cpManager)
	}
}

// processDocumentRecords processes DocumentRecord chunks from parquet file
func processDocumentRecords(records []schema.DocumentRecord, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config, cpManager *checkpoint.Manager) (int64, error) {
	var frameCount int64
	var recordCount int64
	log.Printf("[BATCH] Starting to process %d DocumentRecords...", len(records))

	for i := range records {
		record := &records[i]

		// Check if we should skip this record (already processed)
		if cpManager != nil && cpManager.ShouldSkipRecord(record.FileName, record.ChunkID, 0) {
			log.Printf("[BATCH] Skipping already processed record %d/%d: %s (chunk %d)",
				i+1, len(records), record.FileName, record.ChunkID)
			recordCount++
			continue
		}

		log.Printf("[BATCH] Processing record %d/%d: %s (chunk %d, content: %d chars)",
			i+1, len(records), record.FileName, record.ChunkID, len(record.Content))

		// Convert DocumentRecord to MinedRecord format for processing
		minedRecord := schema.MinedRecord{
			FileName: record.FileName,
			ChunkID:  int(record.ChunkID),
			Content:  record.Content,
		}

		if err := processSingleRecordWithSlidingWindow(&minedRecord, &frameCount, mp, vm, tk, ew, pw, config, cpManager); err != nil {
			return 0, fmt.Errorf("failed to process DocumentRecord %d: %w", i, err)
		}

		recordCount++
		log.Printf("[BATCH] Completed record %d/%d, total frames: %d", i+1, len(records), frameCount)

		// Update checkpoint every 10 records
		if cpManager != nil && i%10 == 0 {
			cpManager.UpdateProgress(record.FileName, record.ChunkID, 0)
			cpManager.IncrementStats(1, frameCount-cpManager.GetCheckpoint().FramesGenerated)
			if err := cpManager.Save(); err != nil {
				log.Printf("⚠️  Failed to save checkpoint: %v", err)
			}
		}

		// Flush parquet writer every 50 frames to ensure data is written to disk
		if frameCount%50 == 0 {
			log.Printf("[BATCH] Flushing parquet writer (frame count: %d)...", frameCount)
			if err := pw.Flush(true); err != nil {
				log.Printf("⚠️  Failed to flush parquet writer: %v", err)
			}
		}

		// Progress logging every 10 records (more frequent)
		if i%10 == 0 {
			log.Printf("📊 Processed %d/%d DocumentRecords, generated %d frames...", i, len(records), frameCount)
		}
	}

	log.Printf("📈 Total: %d DocumentRecords processed, %d training frames generated", recordCount, frameCount)
	return frameCount, nil
}

// processMinedRecords processes MinedRecord chunks from JSON array
func processMinedRecords(records *[]schema.MinedRecord, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config, cpManager *checkpoint.Manager) (int64, error) {
	var frameCount int64
	var processedCount int64

	for i := range *records {
		record := &(*records)[i]

		// Check if we should skip this record
		if cpManager != nil && cpManager.ShouldSkipRecord(record.FileName, int32(record.ChunkID), 0) {
			log.Printf("[BATCH] Skipping already processed record %d/%d: %s", i+1, len(*records), record.FileName)
			processedCount++
			continue
		}

		if err := processSingleRecordWithSlidingWindow(record, &frameCount, mp, vm, tk, ew, pw, config, cpManager); err != nil {
			return 0, fmt.Errorf("failed to process record %d: %w", i, err)
		}

		processedCount++

		// Update checkpoint every 10 records
		if cpManager != nil && i%10 == 0 {
			cpManager.UpdateProgress(record.FileName, int32(record.ChunkID), 0)
			cpManager.IncrementStats(1, frameCount-cpManager.GetCheckpoint().FramesGenerated)
			if err := cpManager.Save(); err != nil {
				log.Printf("⚠️  Failed to save checkpoint: %v", err)
			}
		}

		// Progress logging every 100 records
		if i%100 == 0 {
			log.Printf("📊 Processed %d records, generated %d frames...", i, frameCount)
		}
	}

	log.Printf("📈 Total: %d records processed, %d training frames generated", processedCount, frameCount)
	return frameCount, nil
}

// processJSONLRecords processes JSONL format records
func processJSONLRecords(content string, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config, cpManager *checkpoint.Manager) (int64, error) {
	var recordCount, frameCount, fixedCount int64
	scanner := bufio.NewScanner(strings.NewReader(content))
	var updatedLines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Try to parse the JSON record
		var record schema.MinedRecord
		err := json.Unmarshal([]byte(line), &record)

		if err != nil {
			// Try to fix the JSON
			fixedJSON := fixJSONString(line)
			if parseErr := json.Unmarshal([]byte(fixedJSON), &record); parseErr != nil {
				if recordCount%10 == 0 || recordCount < 10 {
					log.Printf("⚠️  Skipping unfixable record %d: %v", recordCount, err)
				}
				continue
			}

			// Use the fixed version for file update
			updatedLines = append(updatedLines, fixedJSON)
			fixedCount++
			if fixedCount <= 10 {
				log.Printf("🔧 Fixed record %d", recordCount)
			}
		} else {
			updatedLines = append(updatedLines, line)
		}

		recordCount++

		// Check if we should skip this record
		if cpManager != nil && cpManager.ShouldSkipRecord(record.FileName, int32(record.ChunkID), 0) {
			log.Printf("[BATCH] Skipping already processed record %d: %s", recordCount, record.FileName)
			continue
		}

		// Process the record with sliding windows
		if err := processSingleRecordWithSlidingWindow(&record, &frameCount, mp, vm, tk, ew, pw, config, cpManager); err != nil {
			return 0, fmt.Errorf("failed to process record %d: %w", recordCount, err)
		}

		// Update checkpoint every 10 records
		if cpManager != nil && recordCount%10 == 0 {
			cpManager.UpdateProgress(record.FileName, int32(record.ChunkID), 0)
			cpManager.IncrementStats(1, frameCount-cpManager.GetCheckpoint().FramesGenerated)
			if err := cpManager.Save(); err != nil {
				log.Printf("⚠️  Failed to save checkpoint: %v", err)
			}
		}

		// Progress logging every 100 records
		if recordCount%100 == 0 {
			log.Printf("📊 Processed %d records, generated %d frames (fixed: %d)...", recordCount, frameCount, fixedCount)
		}
	}

	// Update file with fixed content if any records were fixed
	if fixedCount > 0 {
		fixedContent := strings.Join(updatedLines, "\n") + "\n"
		if writeErr := os.WriteFile(config.InputFile, []byte(fixedContent), 0644); writeErr != nil {
			log.Printf("⚠️  Failed to update fixed file: %v", writeErr)
		} else {
			log.Printf("🔧 Updated file with %d fixed records", fixedCount)
		}
	}

	log.Printf("📈 Total: %d records processed, %d training frames generated", recordCount, frameCount)
	if fixedCount > 0 {
		log.Printf("🔧 JSON issues fixed and updated: %d records", fixedCount)
	}

	return frameCount, nil
}

func fixJSONContent(content string) (string, bool) {
	original := content
	fixed := fixJSONString(content)

	// Check if JSON array is properly closed
	trimmed := strings.TrimSpace(fixed)
	if strings.HasPrefix(trimmed, "[") && !strings.HasSuffix(trimmed, "]") {
		// Add missing closing bracket
		fixed = fixed + "\n]"
	}

	return fixed, original != fixed
}

func processSingleRecordWithSlidingWindow(record *schema.MinedRecord, frameCount *int64, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config, cpManager *checkpoint.Manager) error {
	log.Printf("[PROCESS] Starting record %d from %s (content length: %d chars)", record.ChunkID, record.FileName, len(record.Content))

	// Check quota availability before processing
	if cpManager != nil && !cpManager.HasQuotaAvailable() {
		log.Printf("🛑 Quota exhausted (%d/%d embeddings used). Stopping processing.",
			cpManager.GetCheckpoint().QuotaUsed, cpManager.GetCheckpoint().QuotaLimit)
		return fmt.Errorf("quota exhausted")
	}

	// 1. Tokenize the entire content once
	log.Printf("[PROCESS] Step 1/6: Tokenizing content...")
	allTokens := tk.Encode(record.Content)
	log.Printf("[PROCESS] Tokenized to %d tokens", len(allTokens))

	if len(allTokens) < 2 {
		log.Printf("⚠️  Insufficient tokens (%d) for sliding window in record %d from %s", len(allTokens), record.ChunkID, record.FileName)
		return nil
	}

	// 2. Generate sliding windows
	log.Printf("[PROCESS] Step 2/6: Generating sliding windows (size=%d, stride=%d)...", config.WindowSize, config.WindowStride)
	sg := sliding.NewGenerator(config.WindowSize, config.WindowStride)
	windows := sg.GenerateWindows(allTokens)
	log.Printf("[PROCESS] Generated %d sliding windows", len(windows))

	if len(windows) == 0 {
		log.Printf("⚠️  No windows generated for record %d from %s", record.ChunkID, record.FileName)
		return nil
	}

	// 3. Process windows in batches for API efficiency
	log.Printf("[PROCESS] Step 3/6: Processing %d windows in batches of %d...", len(windows), config.BatchSize)
	for batchStart := 0; batchStart < len(windows); batchStart += config.BatchSize {
		batchEnd := batchStart + config.BatchSize
		if batchEnd > len(windows) {
			batchEnd = len(windows)
		}
		batch := windows[batchStart:batchEnd]
		batchSize := len(batch)
		log.Printf("[PROCESS]   Processing batch %d-%d of %d windows...", batchStart, batchEnd, len(windows))

		// Check quota before this batch
		if cpManager != nil {
			if !cpManager.UseQuota(batchSize) {
				used, limit, _ := cpManager.GetQuotaStatus()
				log.Printf("🛑 Quota exhausted before batch (need %d, have %d/%d). Stopping.",
					batchSize, used, limit)
				return fmt.Errorf("quota exhausted")
			}
			used, limit, remaining := cpManager.GetQuotaStatus()
			log.Printf("📊 Quota: %d/%d used, %d remaining (using %d for this batch)",
				used, limit, remaining, batchSize)
		}

		// 4. Extract context texts for batch embedding
		log.Printf("[PROCESS] Step 4/6: Extracting %d context texts...", len(batch))
		contextTexts := make([]string, len(batch))
		for i, window := range batch {
			contextTexts[i] = tk.Decode(window.ContextTokens)
		}
		log.Printf("[PROCESS]   Extracted %d context texts (avg length: %d chars)", len(contextTexts), len(contextTexts[0]))

		// 5. Get batch embeddings from Cloudflare
		log.Printf("[PROCESS] Step 5/6: Requesting embeddings from endpoint...")
		batchEmbeddings, err := ew.GetBatchEmbeddings(contextTexts)
		if err != nil {
			log.Printf("[PROCESS] ❌ Embedding request FAILED: %v", err)
			return fmt.Errorf("batch embedding failed for record %d: %w", record.ChunkID, err)
		}
		log.Printf("[PROCESS]   Received %d embeddings (dimension: %d)", len(batchEmbeddings), len(batchEmbeddings[0]))

		// 6. Process each window in the batch
		log.Printf("[PROCESS] Step 6/6: Writing %d frames to parquet...", len(batch))
		for i, window := range batch {
			// Map embedding to ASIC slots
			asicSlots := mp.MapToSlots(batchEmbeddings[i])

			// Create training frame with sliding window metadata
			frame := schema.TrainingFrame{
				SourceFile:    record.FileName,
				ChunkID:       int32(record.ChunkID),
				WindowStart:   int32(window.StartPos),
				WindowEnd:     int32(window.EndPos),
				ContextLength: int32(len(window.ContextTokens)),
				TargetTokenID: int32(window.TargetToken),
				BestSeed:      "",
			}
			frame.SetAsicSlots(asicSlots)

			if err := pw.Write(frame); err != nil {
				log.Printf("[PROCESS] ❌ Parquet write error: %v", err)
				return fmt.Errorf("failed to write frame to parquet: %w", err)
			}
			*frameCount++
		}
		log.Printf("[PROCESS]   Wrote %d frames (total: %d)", len(batch), *frameCount)

		// Progress logging for large batches
		if len(windows) > config.BatchSize && batchStart%(config.BatchSize*2) == 0 {
			log.Printf("📊 Processed %d/%d windows for record %d, generated %d frames...",
				batchStart, len(windows), record.ChunkID, *frameCount)
		}
	}

	log.Printf("[PROCESS] ✅ Completed record %d: %d windows -> %d frames", record.ChunkID, len(windows), *frameCount)
	return nil
}

func processStreamingJSON(jsonFile *os.File, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config, cpManager *checkpoint.Manager) error {
	decoder := json.NewDecoder(jsonFile)
	decoder.DisallowUnknownFields()

	var recordCount, frameCount, fixedCount int64

	// Processing Loop
	log.Printf("🚀 Starting encoding from %s...", config.InputFile)

	for decoder.More() {
		var record schema.MinedRecord
		if err := decoder.Decode(&record); err != nil {
			// Try to fix JSON errors by reading raw and fixing
			if jsonErr := fixAndRetryRecord(decoder, &record, config, &fixedCount); jsonErr != nil {
				if recordCount%10 == 0 || recordCount < 10 {
					log.Printf("⚠️  Skipping bad record %d: %v", recordCount, jsonErr)
				}
				continue
			}
		}

		recordCount++

		// Process with sliding windows
		if err := processSingleRecordWithSlidingWindow(&record, &frameCount, mp, vm, tk, ew, pw, config, cpManager); err != nil {
			return fmt.Errorf("failed to process record %d: %w", recordCount, err)
		}

		// Progress logging every 100 records
		if recordCount%100 == 0 {
			log.Printf("📊 Processed %d records, generated %d frames (fixed: %d)...", recordCount, frameCount, fixedCount)
		}
	}

	log.Printf("📈 Total: %d records processed, %d training frames generated", recordCount, frameCount)
	if fixedCount > 0 {
		log.Printf("🔧 JSON issues fixed: %d records", fixedCount)
	}

	return nil
}

func fixAndRetryRecord(decoder *json.Decoder, record *schema.MinedRecord, config *Config, fixedCount *int64) error {
	// Read the raw JSON bytes and try to fix
	var rawRecord map[string]interface{}
	if err := decoder.Decode(&rawRecord); err != nil {
		return fmt.Errorf("failed to read raw record: %w", err)
	}

	// Convert back to JSON and fix common issues
	jsonBytes, err := json.Marshal(rawRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal raw record: %w", err)
	}

	fixedJSON := fixJSONString(string(jsonBytes))

	// Try to parse the fixed JSON
	if err := json.Unmarshal([]byte(fixedJSON), record); err != nil {
		return fmt.Errorf("failed to parse fixed record: %w", err)
	}

	*fixedCount++
	return nil
}

func fixJSONString(jsonStr string) string {
	// Step 1: Fix invalid \x escape sequences by removing them completely
	invalidEscapeRegex := regexp.MustCompile(`\\x[0-9a-fA-F]{2}`)
	fixed := invalidEscapeRegex.ReplaceAllString(jsonStr, "")

	// Step 2: Remove control characters that break JSON
	controlCharRegex := regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F]`)
	fixed = controlCharRegex.ReplaceAllString(fixed, "")

	// Step 3: Fix invalid escape sequences by removing the backslash entirely
	// This catches \i, \ , etc. which are not valid JSON escapes
	invalidBackslashRegex := regexp.MustCompile(`\\[^"\\bfnrt/]`)
	fixed = invalidBackslashRegex.ReplaceAllString(fixed, "")

	return fixed
}
