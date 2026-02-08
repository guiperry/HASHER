package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"data-encoder/pkg/analyzer"
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
	log.Printf("🔧 Initializing Data Encoder (seed=%d, workers=%d, window=%d, stride=%d, batch=%d)...",
		config.MapperSeed, config.NumWorkers, config.WindowSize, config.WindowStride, config.BatchSize)

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

	log.Printf("💾 Output will be saved to: %s", config.OutputFile)

	// 2. Setup Parquet Writer
	fw, err := local.NewLocalFileWriter(config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer fw.Close()

	pw, err := writer.NewParquetWriter(fw, new(schema.TrainingFrame), int64(config.NumWorkers))
	if err != nil {
		return fmt.Errorf("failed to create Parquet writer: %w", err)
	}
	pw.CompressionType = parquet.CompressionCodec_SNAPPY
	defer pw.WriteStop()

	// 3. Detect file type and read appropriate format
	fileType := detectFileType(config.InputFile)
	log.Printf("📖 Detected input file type: %s", fileType)

	var frameCount int64

	if fileType == "parquet" {
		// Try to read as parquet first
		var records []schema.DocumentRecord
		records, err = readParquetFile(config.InputFile)
		if err != nil {
			// Parquet read failed, try fallback to JSON
			log.Printf("⚠️  Parquet read failed (%v), attempting JSON fallback...", err)
			frameCount, err = processJSONFallback(config.InputFile, mp, varianceMapper, tk, ew, pw, config)
			if err != nil {
				return fmt.Errorf("both parquet and JSON processing failed: %w", err)
			}
		} else {
			// Successfully read parquet, convert to MinedRecord format and process
			frameCount, err = processDocumentRecords(records, mp, varianceMapper, tk, ew, pw, config)
			if err != nil {
				return fmt.Errorf("failed to process parquet records: %w", err)
			}
		}
	} else {
		// Process as JSON format (array or JSONL)
		frameCount, err = processJSONFile(config.InputFile, mp, varianceMapper, tk, ew, pw, config)
		if err != nil {
			return fmt.Errorf("failed to process JSON file: %w", err)
		}
	}

	log.Printf("📈 Total: %d training frames generated", frameCount)
	return nil
}

// processJSONFallback handles JSON processing when parquet fails
func processJSONFallback(filePath string, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config) (int64, error) {
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
		return processMinedRecords(&records, mp, vm, tk, ew, pw, config)
	} else {
		log.Printf("📋 Processing as JSONL format")
		return processJSONLRecords(contentStr, mp, vm, tk, ew, pw, config)
	}
}

// processJSONFile processes JSON files with proper error handling
func processJSONFile(filePath string, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config) (int64, error) {
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
		return processMinedRecords(&records, mp, vm, tk, ew, pw, config)
	} else {
		log.Printf("📋 Processing as JSONL format")
		return processJSONLRecords(contentStr, mp, vm, tk, ew, pw, config)
	}
}

// processDocumentRecords processes DocumentRecord chunks from parquet file
func processDocumentRecords(records []schema.DocumentRecord, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config) (int64, error) {
	var frameCount int64

	for i := range records {
		record := &records[i]

		// Convert DocumentRecord to MinedRecord format for processing
		minedRecord := schema.MinedRecord{
			FileName: record.FileName,
			ChunkID:  int(record.ChunkID),
			Content:  record.Content,
		}

		if err := processSingleRecordWithSlidingWindow(&minedRecord, &frameCount, mp, vm, tk, ew, pw, config); err != nil {
			log.Printf("⚠️  Error processing DocumentRecord %d: %v", i, err)
			continue
		}

		// Progress logging every 100 records
		if i%100 == 0 {
			log.Printf("📊 Processed %d DocumentRecords, generated %d frames...", i, frameCount)
		}
	}

	log.Printf("📈 Total: %d DocumentRecords processed, %d training frames generated", len(records), frameCount)
	return frameCount, nil
}

// processMinedRecords processes MinedRecord chunks from JSON array
func processMinedRecords(records *[]schema.MinedRecord, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config) (int64, error) {
	var frameCount int64

	for i := range *records {
		record := &(*records)[i]

		if err := processSingleRecordWithSlidingWindow(record, &frameCount, mp, vm, tk, ew, pw, config); err != nil {
			log.Printf("⚠️  Error processing record %d: %v", i, err)
			continue
		}

		// Progress logging every 100 records
		if i%100 == 0 {
			log.Printf("📊 Processed %d records, generated %d frames...", i, frameCount)
		}
	}

	log.Printf("📈 Total: %d records processed, %d training frames generated", len(*records), frameCount)
	return frameCount, nil
}

// processJSONLRecords processes JSONL format records
func processJSONLRecords(content string, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config) (int64, error) {
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

		// Process the record with sliding windows
		if err := processSingleRecordWithSlidingWindow(&record, &frameCount, mp, vm, tk, ew, pw, config); err != nil {
			log.Printf("⚠️  Error processing record %d: %v", recordCount, err)
			continue
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

func processSingleRecordWithSlidingWindow(record *schema.MinedRecord, frameCount *int64, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config) error {
	// 1. Tokenize the entire content once
	allTokens := tk.Encode(record.Content)
	if len(allTokens) < 2 {
		log.Printf("⚠️  Insufficient tokens (%d) for sliding window in record %d from %s", len(allTokens), record.ChunkID, record.FileName)
		return nil
	}

	// 2. Generate sliding windows
	sg := sliding.NewGenerator(config.WindowSize, config.WindowStride)
	windows := sg.GenerateWindows(allTokens)

	if len(windows) == 0 {
		log.Printf("⚠️  No windows generated for record %d from %s", record.ChunkID, record.FileName)
		return nil
	}

	// 3. Process windows in batches for API efficiency
	for batchStart := 0; batchStart < len(windows); batchStart += config.BatchSize {
		batchEnd := batchStart + config.BatchSize
		if batchEnd > len(windows) {
			batchEnd = len(windows)
		}
		batch := windows[batchStart:batchEnd]

		// 4. Extract context texts for batch embedding
		contextTexts := make([]string, len(batch))
		for i, window := range batch {
			contextTexts[i] = tk.Decode(window.ContextTokens)
		}

		// 5. Get batch embeddings from Cloudflare
		batchEmbeddings, err := ew.GetBatchEmbeddings(contextTexts)
		if err != nil {
			return fmt.Errorf("batch embedding failed for record %d: %w", record.ChunkID, err)
		}

		// 6. Process each window in the batch
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
				log.Printf("⚠️  Parquet write error: %v", err)
			} else {
				*frameCount++
			}
		}

		// Progress logging for large batches
		if len(windows) > config.BatchSize && batchStart%(config.BatchSize*2) == 0 {
			log.Printf("📊 Processed %d/%d windows for record %d, generated %d frames...",
				batchStart, len(windows), record.ChunkID, *frameCount)
		}
	}

	return nil
}

func processStreamingJSON(jsonFile *os.File, mp *mapper.Service, vm *mapper.VarianceMapper, tk *tokenizer.Service, ew *embeddings.Service, pw *writer.ParquetWriter, config *Config) error {
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
		if err := processSingleRecordWithSlidingWindow(&record, &frameCount, mp, vm, tk, ew, pw, config); err != nil {
			log.Printf("⚠️  Error processing record %d: %v", recordCount, err)
			continue
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
