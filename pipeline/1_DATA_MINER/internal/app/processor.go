package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dataminer/internal/arxiv"
	"dataminer/internal/checkpoint"
	"dataminer/internal/embedder"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
)

// ScanForPDFs scans the input directory for PDF files that haven't been processed
func ScanForPDFs(inputDir string, checkpointer *checkpoint.Checkpointer) ([]string, error) {
	var files []string

	fmt.Printf("🔍 Scanning directory: %s\n", inputDir)

	// First check if directory exists
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		fmt.Printf("❌ Directory does not exist: %s\n", inputDir)
		return files, nil
	}

	fmt.Printf("✅ Directory exists, starting walk...\n")

	// First pass: collect all PDF files
	var allPDFs []string
	fileCount := 0

	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("❌ Error walking path %s: %v\n", path, err)
			return err
		}

		fileCount++
		if fileCount%1000 == 0 {
			fmt.Printf("📊 Scanned %d files/directories...\n", fileCount)
		}

		if !d.IsDir() && strings.ToLower(filepath.Ext(path)) == ".pdf" {
			allPDFs = append(allPDFs, path)
		}
		return nil
	})

	if err != nil {
		return files, err
	}

	fmt.Printf("📊 Found %d total PDF files\n", len(allPDFs))

	// If no checkpointer provided, return all PDFs
	if checkpointer == nil {
		fmt.Printf("📝 No checkpoint filtering - returning all %d PDFs\n", len(allPDFs))
		return allPDFs, nil
	}

	// Second pass: filter out already processed files (batch database queries)
	fmt.Printf("🔍 Filtering processed files...\n")

	for i, path := range allPDFs {
		if i%100 == 0 {
			fmt.Printf("📊 Checking %d/%d files...\n", i+1, len(allPDFs))
		}

		filename := filepath.Base(path)

		// Check both legacy checkpoint system and new PDF metadata system
		isProcessed := checkpointer.IsProcessed(path)
		isPDFProcessed := checkpointer.IsPDFProcessed(filename)

		if !isProcessed && !isPDFProcessed {
			files = append(files, path)
		}
	}

	fmt.Printf("🎉 Scan completed. Found %d unprocessed PDFs out of %d total PDFs.\n", len(files), len(allPDFs))
	return files, nil
}

// ProcessDocuments orchestrates the processing of multiple PDF documents
func ProcessDocuments(config *Config, checkpointer *checkpoint.Checkpointer) error {
	// Initialize paper manager
	paperManager, err := NewPaperManager(config.DataDirs["papers"])
	if err != nil {
		return fmt.Errorf("failed to initialize paper manager: %w", err)
	}

	// Scan for PDF files
	files, err := ScanForPDFs(config.InputDir, checkpointer)
	if err != nil {
		return fmt.Errorf("failed to scan for PDFs: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No new PDF files to process.")
		return nil
	}

	// Initialize progress bar
	p := mpb.New(mpb.WithWidth(80))
	totalFiles := int64(len(files))
	bar := p.AddBar(totalFiles,
		mpb.PrependDecorators(
			decor.Name("Processing PDFs: "),
			decor.Percentage(decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			decor.OnComplete(decor.AverageETA(decor.ET_STYLE_GO), "done!"),
		),
	)

	// Setup concurrent processing
	jobs := make(chan string, config.NumWorkers*2)
	results := make(chan DocumentRecord, config.NumWorkers*2)
	var wg sync.WaitGroup

	// Start workers
	for w := 1; w <= config.NumWorkers; w++ {
		wg.Add(1)
		go embeddingWorker(w, jobs, results, &wg, config, bar, checkpointer, paperManager)
	}

	// Send jobs
	go func() {
		for _, file := range files {
			jobs <- file
		}
		close(jobs)
	}()

	// Monitor workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and write to output
	if err := writeOutput(config.ParquetFile, config.OutputFile, results); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	// Wait for progress bar to finish
	p.Wait()
	fmt.Printf("Pipeline complete! Processed %d PDF files.\n", len(files))
	fmt.Printf("  📊 Parquet (primary): %s\n", config.ParquetFile)
	fmt.Printf("  📄 JSON (backup): %s\n", config.OutputFile)

	return nil
}

// embeddingWorker processes PDF files and generates embeddings
func embeddingWorker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup, config *Config, bar *mpb.Bar, checkpointer *checkpoint.Checkpointer, paperManager *PaperManager) {
	defer wg.Done()

	// Create hybrid provider
	ollamaURL := strings.TrimSuffix(config.OllamaHost, "/") + "/api/embeddings"
	provider := embedder.NewHybridEmbeddingProvider(config.CloudflareEndpoint, ollamaURL, config.OllamaModel, config.CloudflareLimit)

	for path := range jobs {
		log.Printf("Worker %d processing: %s", id, path)

		// Extract text from PDF
		text, err := extractTextFromPDF(path)
		if err != nil {
			log.Printf("Worker %d failed to extract text from %s: %v", id, path, err)
			continue
		}

		if strings.TrimSpace(text) == "" {
			log.Printf("Worker %d: Empty text in %s, skipping", id, path)
			continue
		}

		// Get file info for metadata
		fileInfo, err := os.Stat(path)
		if err != nil {
			log.Printf("Worker %d failed to get file info for %s: %v", id, path, err)
			continue
		}

		// Split text into chunks
		chunks := ChunkText(text, config.ChunkSize, config.ChunkOverlap)

		if len(chunks) == 0 {
			log.Printf("Worker %d: No chunks generated from %s", id, path)
			continue
		}

		// Process chunks in batches for embedding API calls
		var allRecords []DocumentRecord
		for i := 0; i < len(chunks); i += config.BatchSize {
			end := i + config.BatchSize
			if end > len(chunks) {
				end = len(chunks)
			}

			batch := chunks[i:end]

			// Get embeddings for batch
			embeddings, err := getBatchEmbeddings(provider, batch)
			if err != nil {
				log.Printf("Worker %d failed to get embeddings for batch from %s: %v", id, path, err)
				continue
			}

			// Create DocumentRecord for each chunk
			for j, chunk := range batch {
				record := DocumentRecord{
					FileName:  path,
					ChunkID:   int32(i + j),
					Content:   chunk,
					Embedding: embeddings[j],
				}
				allRecords = append(allRecords, record)
				results <- record // Also send to main output
			}
		}

		// Create paper data structure
		paperData := &PaperData{
			FileName:       filepath.Base(path),
			ProcessedAt:    time.Now(),
			Chunks:         allRecords,
			ChunkCount:     len(allRecords),
			WordCount:      len(strings.Fields(text)),
			FileSize:       fileInfo.Size(),
			EmbeddingModel: config.OllamaModel,
		}

		// Save individual paper file
		if err := paperManager.SavePaper(paperData); err != nil {
			log.Printf("Worker %d failed to save paper data for %s: %v", id, path, err)
		} else {
			log.Printf("Worker %d saved paper data for %s", id, path)
		}

		// Add to processed PDFs metadata
		metadata := checkpoint.ProcessedPDFMetadata{
			FileName:      filepath.Base(path),
			ProcessedAt:   time.Now(),
			FileSize:      fileInfo.Size(),
			PaperJSONFile: paperData.PaperJSONFile,
		}
		if err := checkpointer.AddProcessedPDF(metadata); err != nil {
			log.Printf("Worker %d failed to add processed PDF metadata for %s: %v", id, path, err)
		}

		// Mark file as processed in legacy system
		if err := checkpointer.MarkAsDone(path); err != nil {
			log.Printf("Worker %d failed to mark %s as done: %v", id, path, err)
		}

		// Increment progress bar
		bar.Increment()
	}
}

// extractTextFromPDF extracts text from a PDF file using pdftotext
func extractTextFromPDF(path string) (string, error) {
	// Use pdftotext command to extract text
	// pdftotext writes to stdout when '-' is specified as output file
	cmd := exec.Command("pdftotext", path, "-")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to extract text from PDF %s: %w (make sure pdftotext is installed)", path, err)
	}

	return string(output), nil
}

// ChunkText splits text into overlapping chunks
func ChunkText(text string, chunkSize, overlap int) []string {
	words := strings.Fields(text)
	var chunks []string

	log.Printf("Chunking text with %d words, chunk size %d, overlap %d", len(words), chunkSize, overlap)

	for i := 0; i < len(words); i += (chunkSize - overlap) {
		end := i + chunkSize
		if end > len(words) {
			end = len(words)
		}

		chunk := strings.Join(words[i:end], " ")

		// Skip very small chunks (less than 10 words)
		if len(strings.Fields(chunk)) < 10 {
			continue
		}

		// Limit chunk length to avoid timeouts (approx 2000 chars max)
		if len(chunk) > 2000 {
			// Truncate chunk
			chunkWords := strings.Fields(chunk)
			maxWords := 180 // approximate limit for ~2000 chars
			if len(chunkWords) > maxWords {
				chunk = strings.Join(chunkWords[:maxWords], " ")
			}
		}

		chunks = append(chunks, chunk)

		if end == len(words) {
			break
		}
	}

	log.Printf("Generated %d chunks from text", len(chunks))
	return chunks
}

// getBatchEmbeddings generates embeddings for a batch of text chunks
func getBatchEmbeddings(provider *embedder.HybridEmbeddingProvider, texts []string) ([][]float32, error) {
	// Get provider stats
	stats := provider.GetProviderStats()
	log.Printf("Embedding provider stats: %+v", stats)

	// Process batch using hybrid provider
	return provider.GetBatchEmbeddings(texts)
}

// writeOutput writes the document records to both Parquet (primary) and JSON (backup) files
func writeOutput(parquetPath string, jsonPath string, results <-chan DocumentRecord) error {
	// Write to Parquet first (primary format)
	if err := writeParquetOutput(parquetPath, results); err != nil {
		return fmt.Errorf("failed to write parquet output: %w", err)
	}

	// Then write to JSON as backup
	if err := writeJSONOutput(jsonPath, results); err != nil {
		return fmt.Errorf("failed to write json backup: %w", err)
	}

	return nil
}

// writeParquetOutput writes document records to a Parquet file with SNAPPY compression
func writeParquetOutput(path string, results <-chan DocumentRecord) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create parquet file writer
	fw, err := local.NewLocalFileWriter(path)
	if err != nil {
		return fmt.Errorf("failed to create parquet file writer: %w", err)
	}
	defer fw.Close()

	// Create parquet writer with SNAPPY compression
	pw, err := writer.NewParquetWriter(fw, new(DocumentRecord), 4)
	if err != nil {
		return fmt.Errorf("failed to create parquet writer: %w", err)
	}
	defer pw.WriteStop()

	// Set compression type to SNAPPY
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	// Write records
	for record := range results {
		if err := pw.Write(record); err != nil {
			return fmt.Errorf("failed to write record to parquet: %w", err)
		}
	}

	return nil
}

// writeJSONOutput writes document records to a JSON file as backup
func writeJSONOutput(path string, results <-chan DocumentRecord) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create json file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	first := true
	if _, err := file.WriteString("[\n"); err != nil {
		return err
	}

	for record := range results {
		if !first {
			if _, err := file.WriteString(",\n"); err != nil {
				return err
			}
		}
		first = false

		if err := encoder.Encode(record); err != nil {
			return err
		}
	}

	if _, err := file.WriteString("\n]"); err != nil {
		return err
	}

	return nil
}

// ValidateDependencies checks if all required dependencies are available
func ValidateDependencies() error {
	// Check if pdftotext is available
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return fmt.Errorf("pdftotext not found. Please install:\n   Ubuntu/Debian: sudo apt-get install poppler-utils\n   macOS: brew install poppler\n   Other: https://poppler.freedesktop.org/")
	}

	// Check if ollama is available (optional, will be started if not running)
	if _, err := exec.LookPath("ollama"); err != nil {
		log.Printf("Warning: ollama not found in PATH. The application will try to start it if needed.")
	}

	return nil
}

// CountFiles counts the number of PDF files in a directory
func CountFiles(inputDir string) (int, error) {
	count := 0
	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.ToLower(filepath.Ext(path)) == ".pdf" {
			count++
		}
		return nil
	})
	return count, err
}

// CreateDirectories creates the necessary directories for processing
func CreateDirectories(config *Config) error {
	dirs := []string{
		config.InputDir,
		filepath.Dir(config.OutputFile),
		filepath.Dir(config.CheckpointDB),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// SessionStats tracks statistics for the current session only
type SessionStats struct {
	CloudflareUsed      int
	CloudflareRemaining int
}

// RunContinuousWorkflow runs an integrated workflow that loops until quota is hit
func RunContinuousWorkflow(ctx context.Context, config *Config, statsManager *StatsManager) error {
	fmt.Printf("🔄 Starting Continuous Workflow\n")
	fmt.Printf("================================\n")

	// Only start Ollama if Cloudflare is not configured
	if config.CloudflareEndpoint == "" {
		fmt.Println("⚠️  No Cloudflare endpoint configured, checking Ollama...")
		if err := CheckOrStartOllama(config.OllamaHost, config.OllamaModel); err != nil {
			return fmt.Errorf("failed to start Ollama: %w", err)
		}
	} else {
		fmt.Printf("☁️  Using Cloudflare embeddings: %s\n", config.CloudflareEndpoint)
	}

	// Initialize session stats (cloudflare tracking only)
	sessionStats := &SessionStats{}

	// Create the provider for quota tracking
	maxDaily := config.CloudflareLimit
	if maxDaily == 0 {
		maxDaily = 5000 // Default if not set
	}

	// Get previous quota usage from stats
	persistentStats := statsManager.GetCurrentStats()
	previousQuotaUsed := persistentStats["cloudflare_quota_used"].(int)

	// Create hybrid provider to track quota, initializing with previous usage
	ollamaFullURL := strings.TrimSuffix(config.OllamaHost, "/") + "/api/embeddings"

	provider := embedder.NewHybridEmbeddingProvider(
		config.CloudflareEndpoint,
		ollamaFullURL,
		config.OllamaModel,
		maxDaily,
	)

	// Initialize provider with previous quota usage
	if previousQuotaUsed > 0 {
		provider.RequestTracker.SetRequests(previousQuotaUsed)
		fmt.Printf("🌐 Restored previous Cloudflare quota usage: %d/%d\n", previousQuotaUsed, maxDaily)
	}

	// Main workflow loop
	loopCount := 0
	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			fmt.Printf("\n🛑 Workflow cancelled by user\n")
			fmt.Println("💾 Saving final statistics...")
			if err := statsManager.Save(); err != nil {
				fmt.Printf("❌ Warning: failed to save stats: %v\n", err)
			}
			return ctx.Err()
		default:
		}

		loopCount++
		fmt.Printf("\n📍 Workflow Iteration %d\n", loopCount)
		fmt.Printf("============================\n")

		// Get current quota status
		providerStats := provider.GetProviderStats()

		// Handle both int and float64 types for quota values
		if usedFloat, ok := providerStats["cloudflare_quota_used"].(float64); ok {
			sessionStats.CloudflareUsed = int(usedFloat)
		} else if usedInt, ok := providerStats["cloudflare_quota_used"].(int); ok {
			sessionStats.CloudflareUsed = usedInt
		}

		if remainingFloat, ok := providerStats["remaining_quota"].(float64); ok {
			sessionStats.CloudflareRemaining = int(remainingFloat)
		} else if remainingInt, ok := providerStats["remaining_quota"].(int); ok {
			sessionStats.CloudflareRemaining = remainingInt
		}

		// Get persistent stats for display
		persistentStats := statsManager.GetCurrentStats()

		fmt.Printf("📊 Current Status:\n")
		fmt.Printf("  🌐 Cloudflare Quota: %d/%d used (%d remaining)\n",
			sessionStats.CloudflareUsed, maxDaily, sessionStats.CloudflareRemaining)
		fmt.Printf("  📄 Papers Downloaded: %d (session: %d)\n",
			persistentStats["daily_papers_downloaded"], persistentStats["daily_papers_downloaded"])
		fmt.Printf("  🧠 Papers Processed: %d (session: %d)\n",
			persistentStats["daily_papers_processed"], persistentStats["daily_papers_processed"])
		fmt.Printf("  📈 Embeddings Generated: %d (session: %d)\n",
			persistentStats["daily_embeddings_generated"], persistentStats["daily_embeddings_generated"])

		// Update stats manager with current cloudflare usage from provider
		used, max, _ := provider.RequestTracker.GetStats()
		statsManager.RecordCloudflareUsage(used, max)
		sessionStats.CloudflareUsed = used
		sessionStats.CloudflareRemaining = max - used

		// Check if quota is exhausted or nearly exhausted
		if sessionStats.CloudflareRemaining <= 10 {
			fmt.Printf("\n⚠️  Cloudflare quota nearly exhausted (%d remaining)!\n", sessionStats.CloudflareRemaining)
			return handleQuotaExhausted(config, statsManager, provider, maxDaily)
		}

		// Check if we have sufficient quota for another batch
		batchSize := estimateNextBatchSize(sessionStats.CloudflareRemaining)
		if batchSize <= 0 {
			fmt.Printf("\n⚠️  Insufficient quota for another batch!\n")
			return handleQuotaExhausted(config, statsManager, provider, maxDaily)
		}

		// Initialize counters for this iteration
		var papersDownloaded, papersProcessed, embeddingsGenerated int

		// First, check if we have existing papers to process
		fmt.Printf("🔍 Step 1: Scanning for existing papers...\n")
		existingFiles, err := ScanForPDFs(config.InputDir, nil)
		if err != nil {
			return fmt.Errorf("failed to scan for existing PDFs: %w", err)
		}

		hasExistingPapers := len(existingFiles) > 0
		fmt.Printf("📊 Found %d existing unprocessed papers\n", len(existingFiles))

		// PHASE 1: Neural Processing (if we have existing papers)
		if hasExistingPapers {
			fmt.Printf("\n🧠 PHASE 1: Neural Processing (Existing Papers)\n")
			fmt.Printf("===============================================\n")
			fmt.Printf("🎯 Target: Process %d existing papers\n", len(existingFiles))
			fmt.Printf("📊 Current quota: %d embeddings remaining\n", sessionStats.CloudflareRemaining)

			papersProcessed, embeddingsGenerated, err = runNeuralProcessingPhase(ctx, config, provider, sessionStats, statsManager)
			if err != nil {
				return fmt.Errorf("neural processing phase failed: %w", err)
			}

			// Record in stats manager
			statsManager.RecordWorkflowLoop(0, papersProcessed, embeddingsGenerated)
			// Sync quota from provider and save
			used, max, _ := provider.RequestTracker.GetStats()
			statsManager.RecordCloudflareUsage(used, max)
			statsManager.Save()

			fmt.Printf("\n🎉 PHASE 1 COMPLETED\n")
			fmt.Printf("===================\n")
			fmt.Printf("🧠 Papers processed: %d\n", papersProcessed)
			fmt.Printf("📈 Embeddings generated: %d\n", embeddingsGenerated)
		}

		// PHASE 2: ArXiv Mining (if enabled and we need more papers)
		if config.EnableArxivMining && sessionStats.CloudflareRemaining > 100 {
			fmt.Printf("\n📚 PHASE 2: ArXiv Mining\n")
			fmt.Printf("========================\n")
			fmt.Printf("🎯 Target: Download up to %d papers\n", batchSize)
			fmt.Printf("📊 Current quota: %d embeddings remaining\n", sessionStats.CloudflareRemaining)

			papersDownloaded, err = runArxivMiningPhase(config, sessionStats, batchSize)
			if err != nil {
				return fmt.Errorf("arXiv mining phase failed: %w", err)
			}

			// Record downloads in stats manager
			if papersDownloaded > 0 {
				statsManager.RecordWorkflowLoop(papersDownloaded, 0, 0)
				statsManager.Save()
			}

			fmt.Printf("\n🎉 PHASE 2 COMPLETED\n")
			fmt.Printf("===================\n")
			fmt.Printf("📥 Papers downloaded: %d\n", papersDownloaded)
			fmt.Printf("🌐 Quota remaining: %d embeddings\n", sessionStats.CloudflareRemaining)

			// If we downloaded new papers, immediately process them
			if papersDownloaded > 0 {
				fmt.Printf("\n🧠 PHASE 3: Neural Processing (New Papers)\n")
				fmt.Printf("========================================\n")
				fmt.Printf("🎯 Target: Process %d newly downloaded papers\n", papersDownloaded)

				newPapersProcessed, newEmbeddingsGenerated, err := runNeuralProcessingPhase(ctx, config, provider, sessionStats, statsManager)
				if err != nil {
					return fmt.Errorf("neural processing of new papers failed: %w", err)
				}

				// Record in stats manager
				statsManager.RecordWorkflowLoop(0, newPapersProcessed, newEmbeddingsGenerated)
				statsManager.Save()

				fmt.Printf("\n🎉 PHASE 3 COMPLETED\n")
				fmt.Printf("===================\n")
				fmt.Printf("🧠 New papers processed: %d\n", newPapersProcessed)
				fmt.Printf("📈 New embeddings generated: %d\n", newEmbeddingsGenerated)
			}
		} else if !config.EnableArxivMining {
			fmt.Printf("\n📝 ArXiv mining disabled - only processing existing papers\n")
		} else {
			fmt.Printf("\n⚠️  Skipping arXiv mining - insufficient quota (%d embeddings remaining)\n", sessionStats.CloudflareRemaining)
		}

		fmt.Printf("\n✅ Workflow Iteration %d completed successfully\n", loopCount)

		// Create summary based on what actually happened
		var summaryParts []string
		if hasExistingPapers {
			summaryParts = append(summaryParts, fmt.Sprintf("%d existing processed", papersProcessed))
		}
		if papersDownloaded > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("%d downloaded", papersDownloaded))
		}
		if papersDownloaded > 0 && papersProcessed > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("%d embeddings", embeddingsGenerated))
		}

		summary := strings.Join(summaryParts, ", ")
		fmt.Printf("📊 Iteration Summary: %s\n", summary)

		// Save stats after each iteration
		statsManager.Save()

		// Brief pause between iterations
		if sessionStats.CloudflareRemaining > 0 {
			fmt.Printf("⏳ Waiting 5 seconds before next iteration...\n")
			time.Sleep(5 * time.Second)
		}
	}
}

// handleQuotaExhausted handles the quota exceeded scenario with interactive prompts
func handleQuotaExhausted(config *Config, statsManager *StatsManager, provider *embedder.HybridEmbeddingProvider, maxDaily int) error {
	// Get current persistent stats
	persistentStats := statsManager.GetCurrentStats()

	fmt.Printf("\n🎯 Workflow Statistics:\n")
	fmt.Printf("===================\n")
	fmt.Printf("🔄 Daily Workflow Loops: %d\n", persistentStats["daily_workflow_loops"])
	fmt.Printf("📄 Daily Papers Downloaded: %d\n", persistentStats["daily_papers_downloaded"])
	fmt.Printf("🧠 Daily Papers Processed: %d\n", persistentStats["daily_papers_processed"])
	fmt.Printf("📈 Daily Embeddings: %d\n", persistentStats["daily_embeddings_generated"])
	fmt.Printf("🌐 Cloudflare Quota Used: %d/%d\n", persistentStats["cloudflare_quota_used"], maxDaily)
	fmt.Printf("===================\n")
	fmt.Printf("📊 Totals (All Time):\n")
	fmt.Printf("   Total Papers Downloaded: %d\n", persistentStats["total_papers_downloaded"])
	fmt.Printf("   Total Papers Processed: %d\n", persistentStats["total_papers_processed"])
	fmt.Printf("   Total Embeddings: %d\n", persistentStats["total_embeddings_generated"])
	fmt.Printf("===================\n")

	for {
		fmt.Printf("\n❓ Cloudflare quota is nearly exhausted. What would you like to do?\n")
		fmt.Printf("1. 🛑 Stop for today (recommended)\n")
		fmt.Printf("2. 🤖 Continue with Ollama embeddings only\n")
		fmt.Printf("3. 🔄 Continue with mixed (Ollama + remaining Cloudflare)\n")
		fmt.Printf("4. 📊 Show detailed statistics\n")
		fmt.Printf("5. ❌ Exit completely\n")
		fmt.Printf("\nChoice (1-5): ")

		choice, err := readUserInput()
		if err != nil {
			return fmt.Errorf("failed to read user input: %w", err)
		}

		switch strings.TrimSpace(choice) {
		case "1":
			fmt.Printf("\n🛑 Stopping workflow for today.\n")
			fmt.Printf("💡 Tip: Run again tomorrow to continue with fresh Cloudflare quota.\n")
			return nil

		case "2":
			fmt.Printf("\n🤖 Continuing with Ollama embeddings only...\n")
			return continueWithOllamaOnly(config, statsManager)

		case "3":
			fmt.Printf("\n🔄 Continuing with mixed embeddings...\n")
			return continueWithMixedEmbeddings(config, statsManager, provider)

		case "4":
			printDetailedStats(statsManager, maxDaily)

		case "5":
			fmt.Printf("\n❌ Exiting workflow.\n")
			return fmt.Errorf("user chose to exit")

		default:
			fmt.Printf("❌ Invalid choice. Please enter 1-5.\n")
		}
	}
}

// continueWithOllamaOnly continues processing with Ollama embeddings only
func continueWithOllamaOnly(config *Config, statsManager *StatsManager) error {
	fmt.Printf("🤖 Switching to Ollama-only mode...\n")

	// Disable Cloudflare embeddings
	os.Setenv("CLOUDFLARE_EMBEDDINGS_URL", "")

	// Initialize checkpoint system
	checkpointer, err := checkpoint.NewCheckpointer(config.CheckpointDB)
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint system: %w", err)
	}
	defer checkpointer.Close()

	// Continue processing with Ollama only
	fmt.Printf("🧠 Processing remaining papers with Ollama embeddings...\n")

	papersProcessed, err := ProcessDocumentsWithOllamaOnly(config, checkpointer)
	if err != nil {
		return fmt.Errorf("Ollama processing failed: %w", err)
	}

	// Record in stats manager
	statsManager.RecordWorkflowLoop(0, papersProcessed, 0)
	statsManager.Save()

	fmt.Printf("✅ Processed %d additional papers with Ollama\n", papersProcessed)

	// Get updated stats
	persistentStats := statsManager.GetCurrentStats()
	fmt.Printf("\n🎯 Final Statistics:\n")
	fmt.Printf("===================\n")
	fmt.Printf("🔄 Daily Workflow Loops: %d\n", persistentStats["daily_workflow_loops"])
	fmt.Printf("📄 Daily Papers Downloaded: %d\n", persistentStats["daily_papers_downloaded"])
	fmt.Printf("🧠 Daily Papers Processed: %d\n", persistentStats["daily_papers_processed"])
	fmt.Printf("📈 Daily Embeddings: %d\n", persistentStats["daily_embeddings_generated"])
	fmt.Printf("🤖 Final batch used Ollama embeddings\n")

	return nil
}

// continueWithMixedEmbeddings continues with both providers (using remaining quota)
func continueWithMixedEmbeddings(config *Config, statsManager *StatsManager, provider *embedder.HybridEmbeddingProvider) error {
	// Get current quota info
	providerStats := provider.GetProviderStats()
	remainingQuota := 0
	if remainingFloat, ok := providerStats["remaining_quota"].(float64); ok {
		remainingQuota = int(remainingFloat)
	} else if remainingInt, ok := providerStats["remaining_quota"].(int); ok {
		remainingQuota = remainingInt
	}

	fmt.Printf("🔄 Continuing with mixed embeddings (using remaining %d Cloudflare quota)...\n", remainingQuota)

	// Initialize checkpoint system
	checkpointer, err := checkpoint.NewCheckpointer(config.CheckpointDB)
	if err != nil {
		return fmt.Errorf("failed to initialize checkpoint system: %w", err)
	}
	defer checkpointer.Close()

	// Continue processing with mixed mode
	fmt.Printf("🧠 Processing remaining papers with mixed embeddings...\n")

	papersProcessed, embeddingsGenerated, err := ProcessDocumentsWithMixedOnly(config, checkpointer, provider)
	if err != nil {
		return fmt.Errorf("mixed processing failed: %w", err)
	}

	// Record in stats manager
	statsManager.RecordWorkflowLoop(0, papersProcessed, embeddingsGenerated)
	statsManager.Save()

	fmt.Printf("✅ Processed %d additional papers, generated %d embeddings\n", papersProcessed, embeddingsGenerated)

	// Get updated stats
	persistentStats := statsManager.GetCurrentStats()
	fmt.Printf("\n🎯 Final Statistics:\n")
	fmt.Printf("===================\n")
	fmt.Printf("🔄 Daily Workflow Loops: %d\n", persistentStats["daily_workflow_loops"])
	fmt.Printf("📄 Daily Papers Downloaded: %d\n", persistentStats["daily_papers_downloaded"])
	fmt.Printf("🧠 Daily Papers Processed: %d\n", persistentStats["daily_papers_processed"])
	fmt.Printf("📈 Daily Embeddings: %d\n", persistentStats["daily_embeddings_generated"])
	fmt.Printf("🌐 Cloudflare Quota Used: %d/%d\n", persistentStats["cloudflare_quota_used"], persistentStats["cloudflare_quota_max"])

	return nil
}

// Helper functions

func readUserInput() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func estimateNextBatchSize(remainingQuota int) int {
	// Estimate how many embeddings we might need for the next batch
	// This is a rough estimate - actual usage will depend on paper length
	estimatedEmbeddingsPerPaper := 30 // More conservative estimate
	maxPapers := remainingQuota / estimatedEmbeddingsPerPaper

	// Allow at least 1 paper if we have any quota
	if maxPapers < 1 && remainingQuota >= 10 {
		maxPapers = 1
	}

	return maxPapers
}

func runArxivMiningPhase(config *Config, sessionStats *SessionStats, maxPapers int) (int, error) {
	fmt.Printf("📥 Starting arXiv mining for up to %d papers...\n", maxPapers)

	// Calculate how many papers we can download based on remaining quota
	maxPapers = min(maxPapers, config.ArxivMaxPapers)

	// Count existing papers before mining
	checkpointer, err := checkpoint.NewCheckpointer(config.CheckpointDB)
	if err != nil {
		return 0, fmt.Errorf("failed to initialize checkpoint system: %w", err)
	}
	defer checkpointer.Close()

	filesBefore, err := ScanForPDFs(config.InputDir, checkpointer)
	if err != nil {
		return 0, fmt.Errorf("failed to scan existing PDFs: %w", err)
	}

	papersBefore := len(filesBefore)
	fmt.Printf("📊 Found %d existing papers before mining\n", papersBefore)

	// Create miner with adjusted max papers
	miner := arxiv.NewArxivMiner(config.InputDir, config.CheckpointDB)

	miningConfig := arxiv.MiningConfig{
		Categories:    config.ArxivCategories,
		MaxPapers:     maxPapers,
		SortBy:        config.ArxivSortBy,
		SortOrder:     config.ArxivSortOrder,
		DownloadDelay: time.Duration(config.ArxivDownloadDelay) * time.Second,
	}

	fmt.Printf("📋 Mining configuration:\n")
	fmt.Printf("  Categories: %v\n", config.ArxivCategories)
	fmt.Printf("  Target papers: %d\n", maxPapers)
	fmt.Printf("  Sort by: %s %s\n", config.ArxivSortBy, config.ArxivSortOrder)
	fmt.Printf("  Download delay: %ds\n", config.ArxivDownloadDelay)

	if err := miner.MineCategory(miningConfig); err != nil {
		return 0, fmt.Errorf("arXiv mining failed: %w", err)
	}

	// Count papers after mining to get actual number downloaded
	filesAfter, err := ScanForPDFs(config.InputDir, checkpointer)
	if err != nil {
		return 0, fmt.Errorf("failed to scan PDFs after mining: %w", err)
	}

	papersAfter := len(filesAfter)
	papersDownloaded := papersAfter - papersBefore

	fmt.Printf("📊 Mining results: %d papers downloaded (%d total)\n", papersDownloaded, papersAfter)

	return papersDownloaded, nil
}

func runNeuralProcessingPhase(ctx context.Context, config *Config, provider *embedder.HybridEmbeddingProvider, sessionStats *SessionStats, statsManager *StatsManager) (int, int, error) {
	fmt.Printf("🔄 Starting neural processing phase...\n")

	// Initialize checkpoint system
	checkpointer, err := checkpoint.NewCheckpointer(config.CheckpointDB)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to initialize checkpoint system: %w", err)
	}
	defer checkpointer.Close()

	// Scan for PDF files
	files, err := ScanForPDFs(config.InputDir, checkpointer)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to scan for PDFs: %w", err)
	}

	if len(files) == 0 {
		fmt.Printf("ℹ️  No PDF files found to process\n")
		return 0, 0, nil
	}

	fmt.Printf("📄 Found %d PDF files to process\n", len(files))

	// Process all available files
	maxFilesToProcess := len(files)
	fmt.Printf("📊 Processing all %d available files\n", maxFilesToProcess)

	// Estimate embeddings needed and limit papers if necessary
	maxEmbeddings := sessionStats.CloudflareRemaining
	if maxEmbeddings <= 0 {
		maxEmbeddings = 100 // fallback
	}

	maxPapers := maxEmbeddings / 50
	if len(files) > maxPapers {
		fmt.Printf("📊 Limiting processing to %d papers due to quota constraints\n", maxPapers)
		files = files[:maxPapers]
	}

	// Create parquet file (primary output)
	fmt.Printf("📝 Creating parquet file: %s\n", config.ParquetFile)
	if err := os.MkdirAll(filepath.Dir(config.ParquetFile), 0755); err != nil {
		return 0, 0, fmt.Errorf("failed to create parquet directory: %w", err)
	}
	parquetFile, err := local.NewLocalFileWriter(config.ParquetFile)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create parquet file: %w", err)
	}
	defer parquetFile.Close()

	parquetWriter, err := writer.NewParquetWriter(parquetFile, new(DocumentRecord), 4)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create parquet writer: %w", err)
	}
	defer parquetWriter.WriteStop()
	parquetWriter.CompressionType = parquet.CompressionCodec_SNAPPY

	// Create JSON backup file
	fmt.Printf("📝 Creating JSON backup: %s\n", config.OutputFile)
	if err := os.MkdirAll(filepath.Dir(config.OutputFile), 0755); err != nil {
		return 0, 0, fmt.Errorf("failed to create json directory: %w", err)
	}
	jsonFile, err := os.Create(config.OutputFile)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create json file: %w", err)
	}
	defer jsonFile.Close()

	// Write JSON array opening
	if _, err := jsonFile.WriteString("[\n"); err != nil {
		return 0, 0, fmt.Errorf("failed to write JSON opening: %w", err)
	}

	totalEmbeddingsGenerated := 0
	papersProcessed := 0
	firstRecord := true

	// Process files with detailed logging and timeouts
	for i, file := range files {
		// Check for cancellation
		select {
		case <-ctx.Done():
			fmt.Printf("\n🛑 Processing cancelled by user\n")
			return papersProcessed, totalEmbeddingsGenerated, ctx.Err()
		default:
		}

		fmt.Printf("📖 Processing file %d/%d: %s\n", i+1, len(files), filepath.Base(file))

		// Extract and process one file at a time to track quota
		fmt.Printf("🔍 Extracting text from %s...\n", filepath.Base(file))
		text, err := extractTextFromPDF(file)
		if err != nil {
			fmt.Printf("❌ Failed to extract text from %s: %v\n", filepath.Base(file), err)
			continue // Skip files that can't be processed
		}

		fmt.Printf("✅ Successfully extracted %d characters from %s\n", len(text), filepath.Base(file))

		chunks := ChunkText(text, config.ChunkSize, config.ChunkOverlap)
		if len(chunks) == 0 {
			fmt.Printf("⚠️  No chunks generated from %s\n", filepath.Base(file))
			continue
		}

		fmt.Printf("📝 Generated %d chunks from %s\n", len(chunks), filepath.Base(file))

		// Process chunks with quota tracking and timeouts
		fileEmbeddings := 0
		for j, chunk := range chunks {
			// Check for cancellation
			select {
			case <-ctx.Done():
				fmt.Printf("\n🛑 Processing cancelled by user during chunk processing\n")
				// Note: stats will be saved by the calling function
				return papersProcessed, totalEmbeddingsGenerated, ctx.Err()
			default:
			}

			fmt.Printf("🔧 Processing chunk %d/%d (length: %d)...\n", j+1, len(chunks), len(chunk))

			// Check if we have quota left
			providerStats := provider.GetProviderStats()
			var remaining int
			if remainingFloat, ok := providerStats["remaining_quota"].(float64); ok {
				remaining = int(remainingFloat)
			} else if remainingInt, ok := providerStats["remaining_quota"].(int); ok {
				remaining = remainingInt
			}

			if remaining <= 0 {
				fmt.Printf("⚠️  Quota exhausted, stopping processing\n")
				break
			}

			fmt.Printf("🌐 Getting embedding for chunk %d...\n", j+1)

			// Add timeout for embedding generation
			embeddingChan := make(chan []float32, 1)
			errChan := make(chan error, 1)

			go func() {
				embedding, err := provider.GetEmbedding(chunk)
				if err != nil {
					errChan <- err
				} else {
					embeddingChan <- embedding
				}
			}()

			// Wait for embedding with timeout and cancellation
			var embedding []float32
			select {
			case emb := <-embeddingChan:
				embedding = emb
				fmt.Printf("✅ Generated embedding for chunk %d (length: %d)\n", j+1, len(embedding))
				fileEmbeddings++

				// Sync quota to stats manager after each successful embedding
				used, max, _ := provider.RequestTracker.GetStats()
				statsManager.RecordCloudflareUsage(used, max)
			case err := <-errChan:
				fmt.Printf("❌ Failed to generate embedding for chunk %d: %v\n", j+1, err)
				continue // Skip chunks that fail
			case <-ctx.Done():
				fmt.Printf("\n🛑 Embedding generation cancelled by user\n")
				// Note: stats will be saved by the calling function
				return papersProcessed, totalEmbeddingsGenerated, ctx.Err()
			case <-time.After(60 * time.Second):
				fmt.Printf("⏰ TIMEOUT: Failed to generate embedding for chunk %d after 60 seconds\n", j+1)
				continue // Skip chunks that timeout
			}

			// Create DocumentRecord for both parquet and JSON
			record := DocumentRecord{
				FileName:  file,
				ChunkID:   int32(j),
				Content:   chunk,
				Embedding: embedding,
			}

			// Write to Parquet (primary)
			if err := parquetWriter.Write(record); err != nil {
				fmt.Printf("⚠️  Failed to write parquet record: %v\n", err)
				continue
			}

			// Write record to JSON backup
			if !firstRecord {
				if _, err := jsonFile.WriteString(",\n"); err != nil {
					fmt.Printf("⚠️  Failed to write JSON separator: %v\n", err)
					continue
				}
			}

			// Create JSON record
			jsonRecord := fmt.Sprintf(`{
    "file_name": "%s",
    "chunk_id": %d,
    "content": %s,
    "embedding": [%s]
  }`,
				file,
				j,
				fmt.Sprintf("%q", chunk),
				formatEmbeddingArray(embedding),
			)

			if _, err := jsonFile.WriteString(jsonRecord); err != nil {
				fmt.Printf("⚠️  Failed to write JSON record: %v\n", err)
				continue
			}

			firstRecord = false
		}

		totalEmbeddingsGenerated += fileEmbeddings
		papersProcessed++

		fmt.Printf("✅ Completed file %s: %d embeddings generated and saved\n", filepath.Base(file), fileEmbeddings)

		// Mark file as processed
		if err := checkpointer.MarkAsDone(file); err != nil {
			fmt.Printf("⚠️  Failed to mark %s as processed: %v\n", filepath.Base(file), err)
		}

		// Check if we're out of quota
		currentProviderStats := provider.GetProviderStats()
		var remaining int
		if remainingFloat, ok := currentProviderStats["remaining_quota"].(float64); ok {
			remaining = int(remainingFloat)
		} else if remainingInt, ok := currentProviderStats["remaining_quota"].(int); ok {
			remaining = remainingInt
		}
		if remaining <= 0 {
			fmt.Printf("⚠️  Quota exhausted after processing %d papers\n", papersProcessed)
			break
		}

		// Brief pause between files to prevent overwhelming
		fmt.Printf("⏳ Pausing 1 second before next file...\n")
		time.Sleep(1 * time.Second)
	}

	// Close JSON array
	if _, err := jsonFile.WriteString("\n]"); err != nil {
		return 0, 0, fmt.Errorf("failed to write JSON closing: %w", err)
	}

	fmt.Printf("🎉 Neural processing phase completed: %d papers, %d embeddings\n", papersProcessed, totalEmbeddingsGenerated)
	fmt.Printf("  📊 Parquet (primary): %s\n", config.ParquetFile)
	fmt.Printf("  📄 JSON (backup): %s\n", config.OutputFile)
	return papersProcessed, totalEmbeddingsGenerated, nil
}

// Helper function to format embedding array for JSON
func formatEmbeddingArray(embedding []float32) string {
	if len(embedding) == 0 {
		return ""
	}

	var parts []string
	for _, v := range embedding {
		parts = append(parts, fmt.Sprintf("%.6f", v))
	}
	return strings.Join(parts, ", ")
}

func ProcessDocumentsWithOllamaOnly(config *Config, checkpointer *checkpoint.Checkpointer) (int, error) {
	// Temporarily disable Cloudflare to force Ollama usage
	originalURL := os.Getenv("CLOUDFLARE_EMBEDDINGS_URL")
	os.Setenv("CLOUDFLARE_EMBEDDINGS_URL", "")
	defer func() {
		os.Setenv("CLOUDFLARE_EMBEDDINGS_URL", originalURL)
	}()

	err := ProcessDocuments(config, checkpointer)
	if err != nil {
		return 0, err
	}

	// Count processed files
	files, err := ScanForPDFs(config.InputDir, checkpointer)
	if err != nil {
		return 0, err
	}

	return len(files), nil
}

func ProcessDocumentsWithMixedOnly(config *Config, checkpointer *checkpoint.Checkpointer, provider *embedder.HybridEmbeddingProvider) (int, int, error) {
	// Scan for PDF files
	files, err := ScanForPDFs(config.InputDir, checkpointer)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to scan for PDFs: %w", err)
	}

	if len(files) == 0 {
		return 0, 0, nil
	}

	totalEmbeddings := 0

	for _, file := range files {
		text, err := extractTextFromPDF(file)
		if err != nil {
			continue
		}

		chunks := ChunkText(text, config.ChunkSize, config.ChunkOverlap)
		if len(chunks) == 0 {
			continue
		}

		// Process all chunks (provider will handle quota)
		for _, chunk := range chunks {
			_, err := provider.GetEmbedding(chunk)
			if err != nil {
				continue
			}
			totalEmbeddings++
		}

		checkpointer.MarkAsDone(file)
	}

	return len(files), totalEmbeddings, nil
}

func printDetailedStats(statsManager *StatsManager, maxDaily int) {
	stats := statsManager.GetCurrentStats()

	fmt.Printf("\n📊 Detailed Workflow Statistics:\n")
	fmt.Printf("================================\n")
	fmt.Printf("🔄 Daily Workflow Loops: %d\n", stats["daily_workflow_loops"])
	fmt.Printf("📄 Daily Papers Downloaded: %d\n", stats["daily_papers_downloaded"])
	fmt.Printf("🧠 Daily Papers Processed: %d\n", stats["daily_papers_processed"])
	fmt.Printf("📈 Daily Embeddings Generated: %d\n", stats["daily_embeddings_generated"])
	fmt.Printf("🌐 Cloudflare Quota: %d/%d (%.1f%% used)\n",
		stats["cloudflare_quota_used"], maxDaily,
		float64(stats["cloudflare_quota_used"].(int))/float64(maxDaily)*100)
	fmt.Printf("================================\n")
	fmt.Printf("📊 Totals (All Time):\n")
	fmt.Printf("   Total Papers Downloaded: %d\n", stats["total_papers_downloaded"])
	fmt.Printf("   Total Papers Processed: %d\n", stats["total_papers_processed"])
	fmt.Printf("   Total Embeddings: %d\n", stats["total_embeddings_generated"])
	fmt.Printf("   Total Workflow Loops: %d\n", stats["total_workflow_loops"])
	fmt.Printf("================================\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
