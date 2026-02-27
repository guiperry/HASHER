package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"data-miner/internal/arxiv"
	"data-miner/internal/checkpoint"
	"data-miner/internal/embedder"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
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
	if err := writeOutput(config.OutputFile, results); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	// Wait for progress bar to finish
	p.Wait()
	fmt.Printf("Pipeline complete! Processed %d PDF files.\n", len(files))
	fmt.Printf("  📄 JSON Output: %s\n", config.OutputFile)

	return nil
}

// embeddingWorker processes PDF files and generates embeddings
func embeddingWorker(id int, jobs <-chan string, results chan<- DocumentRecord, wg *sync.WaitGroup, config *Config, bar *mpb.Bar, checkpointer *checkpoint.Checkpointer, paperManager *PaperManager) {
	defer wg.Done()

	// Create hybrid provider
	ollamaURL := strings.TrimSuffix(config.OllamaHost, "/") + "/api/embeddings"
	provider := embedder.NewHybridEmbeddingProvider(config.CloudflareEndpoint, ollamaURL, config.OllamaModel, config.CloudflareLimit)

	// Initialize NLP Bridge
	nlpBridge, err := NewNLPBridge()
	if err != nil {
		log.Printf("Worker %d failed to initialize NLP Bridge: %v", id, err)
	} else {
		defer nlpBridge.Close()
	}

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
				var tokens []string
				var offsets []int32
				var posTags []int
				var tenses []int
				var depHashes []uint32

				if nlpBridge != nil {
					tokens, offsets, posTags, tenses, depHashes = nlpBridge.ProcessText(chunk)
				}

				record := DocumentRecord{
					FileName:     path,
					ChunkID:      int32(i + j),
					Content:      chunk,
					Embedding:    embeddings[j],
					Tokens:       tokens,
					TokenOffsets: offsets,
					POSTags:      posTags,
					Tenses:       tenses,
					DepHashes:    depHashes,
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

// ChunkTextByParagraph splits text into paragraphs or sentences
func ChunkTextByParagraph(text string) []string {
	// First split by double newlines for paragraphs
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// If a paragraph is too long, we could further split it by sentences,
		// but for now let's just use paragraphs as the base unit.
		if len(strings.Fields(p)) < 5 {
			continue // Skip very short fragments
		}

		chunks = append(chunks, p)
	}

	// If we didn't find many paragraphs, try single newlines
	if len(chunks) < 5 {
		lines := strings.Split(text, "\n")
		chunks = nil
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if len(strings.Fields(l)) > 10 {
				chunks = append(chunks, l)
			}
		}
	}

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

// writeOutput writes the document records to JSON file and Arrow IPC stream
func writeOutput(jsonPath string, results <-chan DocumentRecord) error {
	// Collect all results first
	var records []DocumentRecord
	for record := range results {
		records = append(records, record)
	}

	// Write to JSON
	if err := writeJSONOutputFromSlice(jsonPath, records); err != nil {
		return fmt.Errorf("failed to write json output: %w", err)
	}

	// Write to Arrow IPC stream
	arrowPath := replaceFileExtension(jsonPath, ".arrow")
	if err := WriteDocumentRecordsToArrowIPC(arrowPath, records); err != nil {
		return fmt.Errorf("failed to write Arrow IPC output: %w", err)
	}

	return nil
}

// writeJSONOutputFromSlice writes document records from a slice to a JSON file
func writeJSONOutputFromSlice(path string, records []DocumentRecord) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create json file: %w", err)
	}
	defer file.Close()

	first := true
	if _, err := file.WriteString("["); err != nil {
		return err
	}

	for _, record := range records {
		if !first {
			if _, err := file.WriteString(","); err != nil {
				return err
			}
		}
		first = false

		recordBytes, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := file.Write(recordBytes); err != nil {
			return err
		}
	}

	if _, err := file.WriteString("]"); err != nil {
		return err
	}

	return nil
}

// replaceFileExtension replaces the file extension in a path
func replaceFileExtension(path, newExt string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + newExt
}

// writeJSONOutput writes document records to a JSON file
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

	first := true
	if _, err := file.WriteString("["); err != nil {
		return err
	}

	for record := range results {
		if !first {
			if _, err := file.WriteString(","); err != nil {
				return err
			}
		}
		first = false

		recordBytes, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := file.Write(recordBytes); err != nil {
			return err
		}
	}

	if _, err := file.WriteString("]"); err != nil {
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

	// 1. Ensure Ollama is running — always required as the fallback for embeddings
	if err := CheckOrStartOllama(config.OllamaHost, config.OllamaModel); err != nil {
		fmt.Printf("⚠️  Warning: Failed to ensure Ollama is running: %v\n", err)
		fmt.Printf("    Cloudflare will be used for embeddings; Ollama fallback unavailable\n")
	}

	// 2. Try to start OpenCode server as a secondary provider (skip in Goat+Cloudflare mode)
	if !config.GoatMode || config.CloudflareEndpoint == "" {
		fmt.Println("🚀 Ensuring OpenCode server is running on port 5500...")
		if !IsOpenCodeRunning() {
			go func() {
				exec.Command("opencode", "serve", "--port", "5500").Start()
			}()
			// Give it a moment to start
			time.Sleep(2 * time.Second)
		} else {
			fmt.Println("✅ OpenCode server is already running")
		}
	}

	// 3. Smart model selection for Ollama (Embedding)
	hasEmbedModel, _ := GetOllamaModel(config.OllamaHost, config.OllamaModel)
	if !hasEmbedModel {
		fmt.Printf("📥 Embedding model %s not found. Pulling...\n", config.OllamaModel)
		if err := PullOllamaModel(config.OllamaHost, config.OllamaModel); err != nil {
			fmt.Printf("⚠️  Warning: Failed to pull embedding model %s: %v\n", config.OllamaModel, err)
		}
	} else {
		fmt.Printf("✅ Embedding model %s is ready\n", config.OllamaModel)
	}

	// 4. Smart model selection for Ollama (Generation)
	fmt.Printf("🤖 Checking Ollama generation model: %s...\n", config.OllamaGenModel)
	hasGenModel, err := GetOllamaModel(config.OllamaHost, config.OllamaGenModel)
	if err != nil {
		fmt.Printf("⚠️  Warning: Failed to check Ollama models: %v\n", err)
	}

	if !hasGenModel {
		fmt.Printf("ℹ️  Model %s not found. Checking for ANY available model...\n", config.OllamaGenModel)
		availableModels, _ := GetOllamaModels(config.OllamaHost)
		if len(availableModels) > 0 {
			// Find a good fallback
			foundFallback := false
			for _, m := range availableModels {
				// Prioritize llama, mistral, or any non-embedding model
				if !strings.Contains(strings.ToLower(m), "embed") {
					fmt.Printf("✅ Found fallback model: %s. Using it for generation.\n", m)
					config.OllamaGenModel = m
					foundFallback = true
					break
				}
			}
			if !foundFallback {
				config.OllamaGenModel = availableModels[0]
				fmt.Printf("✅ Using first available model: %s\n", config.OllamaGenModel)
			}
		} else {
			// No models at all, must pull if opencode isn't running
			if !IsOpenCodeRunning() {
				fmt.Printf("📥 No models found in Ollama and OpenCode not running. Pulling %s...\n", config.OllamaGenModel)
				if err := PullOllamaModel(config.OllamaHost, config.OllamaGenModel); err != nil {
					fmt.Printf("❌ Failed to pull model %s: %v\n", config.OllamaGenModel, err)
				}
			} else {
				fmt.Printf("ℹ️  No models in Ollama, but OpenCode is running. Will use OpenCode for generation.\n")
			}
		}
	} else {
		fmt.Printf("✅ Ollama generation model %s is ready\n", config.OllamaGenModel)
	}

	if config.CloudflareEndpoint != "" {
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

		var papersDownloaded, papersProcessed, embeddingsGenerated int

		// PHASE 0: GOAT Mining (Hugging Face)
		if config.GoatMode {
			fmt.Printf("\n🐐 PHASE 0: GOAT Mining (Hugging Face)\n")
			fmt.Printf("========================================\n")
			
			papersProcessed, embeddingsGenerated, err = RunGoatMiningPhase(ctx, config, provider, statsManager)
			if err != nil {
				return fmt.Errorf("GOAT mining phase failed: %w", err)
			}

			// Record in stats manager
			statsManager.RecordWorkflowLoop(0, papersProcessed, embeddingsGenerated)
			statsManager.Save()

			fmt.Printf("\n🎉 GOAT PHASE COMPLETED\n")
			fmt.Printf("======================\n")
			fmt.Printf("🐐 Records processed: %d\n", papersProcessed)
			fmt.Printf("📈 Embeddings generated: %d\n", embeddingsGenerated)
			
			// In GOAT mode, we might want to exit after one successful run if not continuous
			if !config.EnableArxivMining {
				fmt.Println("🏁 GOAT mode completed. Exiting workflow.")
				return nil
			}
		}

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

func runArxivMiningPhase(config *Config, _ *SessionStats, maxPapers int) (int, error) {
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
	fmt.Printf("🔄 Starting neural processing phase with Alpaca transformation...\n")

	// Initialize NLP Bridge
	nlpBridge, err := NewNLPBridge()
	if err != nil {
		fmt.Printf("⚠️  Failed to initialize NLP Bridge: %v\n", err)
	} else {
		defer nlpBridge.Close()
	}

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

	// Create JSON output file for all results (as requested, not splitting by paper)
	alpacaJsonPath := strings.TrimSuffix(config.OutputFile, ".json") + "_alpaca.json"
	fmt.Printf("📝 Creating Alpaca JSON output: %s\n", alpacaJsonPath)
	if err := os.MkdirAll(filepath.Dir(alpacaJsonPath), 0755); err != nil {
		return 0, 0, fmt.Errorf("failed to create json directory: %w", err)
	}
	jsonFile, err := os.Create(alpacaJsonPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create json file: %w", err)
	}
	defer jsonFile.Close()

	// Write JSON array opening
	if _, err := jsonFile.WriteString("[\n"); err != nil {
		return 0, 0, fmt.Errorf("failed to write JSON opening: %w", err)
	}

	var allAlpacaRecords []AlpacaDocumentRecord
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

		chunks := ChunkTextByParagraph(text)
		if len(chunks) == 0 {
			fmt.Printf("⚠️  No paragraphs found in %s, falling back to standard chunking\n", filepath.Base(file))
			chunks = ChunkText(text, config.ChunkSize, config.ChunkOverlap)
		}

		if len(chunks) == 0 {
			fmt.Printf("⚠️  Still no chunks generated from %s\n", filepath.Base(file))
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
				return papersProcessed, totalEmbeddingsGenerated, ctx.Err()
			default:
			}

			fmt.Printf("🔧 Processing chunk %d/%d (length: %d)...\n", j+1, len(chunks), len(chunk))

			// 1. Transform to Alpaca record
			fmt.Printf("🤖 Transforming to Alpaca record using %s...\n", config.OllamaGenModel)
			alpaca, err := GenerateAlpacaRecord(ctx, config.OllamaHost, config.OllamaGenModel, chunk, nlpBridge)
			if err != nil {
				fmt.Printf("❌ Failed to generate Alpaca record for chunk %d: %v\n", j+1, err)
				continue
			}

			// 2. Extract NLP metadata
			var tokens []string
			var offsets []int32
			var posTags []int
			var tenses []int
			var depHashes []uint32

			// Use the full interaction text for NLP metadata to ensure token alignment
			content := fmt.Sprintf("Instruction: %s\nInput: %s\nOutput: %s", alpaca.Instruction, alpaca.Input, alpaca.Output)

			if nlpBridge != nil {
				tokens, offsets, posTags, tenses, depHashes = nlpBridge.ProcessText(content)
			}

			// 3. Get embedding for the Alpaca record
			// We embed Instruction + Input + Output to capture the full semantic meaning
			embedText := content
			
			// Check if we have quota left
			providerStats := provider.GetProviderStats()
			var remaining int
			if remainingFloat, ok := providerStats["remaining_quota"].(float64); ok {
				remaining = int(remainingFloat)
			} else if remainingInt, ok := providerStats["remaining_quota"].(int); ok {
				remaining = remainingInt
			}

			if remaining <= 0 && config.CloudflareEndpoint != "" {
				fmt.Printf("⚠️  Cloudflare quota exhausted, falling back to Ollama or stopping\n")
			}

			fmt.Printf("🌐 Getting embedding for Alpaca record %d...\n", j+1)

			// Add timeout for embedding generation
			embeddingChan := make(chan []float32, 1)
			errChan := make(chan error, 1)

			go func() {
				embedding, err := provider.GetEmbedding(embedText)
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
				fmt.Printf("✅ Generated embedding for chunk %d\n", j+1)
				fileEmbeddings++

				// Sync quota to stats manager after each successful embedding
				used, max, _ := provider.RequestTracker.GetStats()
				statsManager.RecordCloudflareUsage(used, max)
			case err := <-errChan:
				fmt.Printf("❌ Failed to generate embedding for chunk %d: %v\n", j+1, err)
				continue // Skip chunks that fail
			case <-ctx.Done():
				return papersProcessed, totalEmbeddingsGenerated, ctx.Err()
			case <-time.After(60 * time.Second):
				fmt.Printf("⏰ TIMEOUT: Failed to generate embedding for chunk %d after 60 seconds\n", j+1)
				continue
			}

			// 4. Create record and write to JSON backup
			if !firstRecord {
				if _, err := jsonFile.WriteString(","); err != nil {
					fmt.Printf("⚠️  Failed to write JSON separator: %v\n", err)
					continue
				}
			}

			alpacaRecord := AlpacaDocumentRecord{
				AlpacaRecord: *alpaca,
				FileName:     file,
				ChunkID:      int32(j),
				Content:      content,
				Embedding:    embedding,
				Tokens:       tokens,
				TokenOffsets: offsets,
				POSTags:      posTags,
				Tenses:       tenses,
				DepHashes:    depHashes,
			}
			
			allAlpacaRecords = append(allAlpacaRecords, alpacaRecord)

			recordBytes, err := formatAlpacaRecord(alpacaRecord)
			if err != nil {
				fmt.Printf("⚠️  Failed to format JSON record: %v\n", err)
				continue
			}

			if _, err := jsonFile.Write(recordBytes); err != nil {
				fmt.Printf("⚠️  Failed to write JSON record: %v\n", err)
				continue
			}

			firstRecord = false
		}

		totalEmbeddingsGenerated += fileEmbeddings
		papersProcessed++

		fmt.Printf("✅ Completed file %s: %d Alpaca records generated\n", filepath.Base(file), fileEmbeddings)

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
		if remaining <= 0 && config.CloudflareEndpoint != "" {
			fmt.Printf("⚠️  Quota exhausted after processing %d papers\n", papersProcessed)
			break
		}

		// Brief pause between files
		time.Sleep(1 * time.Second)
	}

	// Close JSON array
	if _, err := jsonFile.WriteString("\n]"); err != nil {
		return 0, 0, fmt.Errorf("failed to write JSON closing: %w", err)
	}

	// 5. Write to Arrow output
	arrowPath := strings.TrimSuffix(config.OutputFile, ".json") + "_alpaca.arrow"
	fmt.Printf("📝 Writing Alpaca Arrow IPC output: %s\n", arrowPath)
	if err := WriteAlpacaDocumentRecordsToArrowIPC(arrowPath, allAlpacaRecords); err != nil {
		fmt.Printf("❌ Failed to write Arrow IPC output: %v\n", err)
	}

	fmt.Printf("🎉 Neural processing phase completed: %d papers, %d Alpaca records\n", papersProcessed, totalEmbeddingsGenerated)
	return papersProcessed, totalEmbeddingsGenerated, nil
}

// formatAlpacaRecord formats an AlpacaDocumentRecord with pretty-printed keys but compact arrays
func formatAlpacaRecord(r AlpacaDocumentRecord) ([]byte, error) {
	instruction, _ := json.Marshal(r.Instruction)
	input, _ := json.Marshal(r.Input)
	output, _ := json.Marshal(r.Output)
	fileName, _ := json.Marshal(r.FileName)
	content, _ := json.Marshal(r.Content)
	embedding, _ := json.Marshal(r.Embedding)
	tokens, _ := json.Marshal(r.Tokens)
	offsets, _ := json.Marshal(r.TokenOffsets)
	posTags, _ := json.Marshal(r.POSTags)
	tenses, _ := json.Marshal(r.Tenses)
	depHashes, _ := json.Marshal(r.DepHashes)

	s := fmt.Sprintf(`  {
    "instruction": %s,
    "input": %s,
    "output": %s,
    "file_name": %s,
    "chunk_id": %d,
    "content": %s,
    "embedding": %s,
    "tokens": %s,
    "token_offsets": %s,
    "pos_tags": %s,
    "tenses": %s,
    "dep_hashes": %s
  }`, instruction, input, output, fileName, r.ChunkID, content, embedding, tokens, offsets, posTags, tenses, depHashes)

	return []byte(s), nil
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

// RunGoatMiningPhase fetches the GOAT dataset from Hugging Face and processes it
func RunGoatMiningPhase(ctx context.Context, config *Config, provider *embedder.HybridEmbeddingProvider, statsManager *StatsManager) (int, int, error) {
	fmt.Printf("🐐 Starting GOAT Mining Phase (Hugging Face)\n")
	fmt.Printf("===========================================\n")

	// Initialize NLP Bridge
	nlpBridge, err := NewNLPBridge()
	if err != nil {
		fmt.Printf("⚠️  Failed to initialize NLP Bridge: %v\n", err)
	} else {
		defer nlpBridge.Close()
	}

	// Hugging Face Datasets Server API URL
	offset := 0
	length := 100
	
	hfURL := fmt.Sprintf("https://datasets-server.huggingface.co/rows?dataset=stallone%%2Fgoat&config=completion&split=train&offset=%d&length=%d", offset, length)
	
	fmt.Printf("🌐 Fetching GOAT dataset from: %s\n", hfURL)
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(hfURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch GOAT dataset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("Hugging Face API returned status %d", resp.StatusCode)
	}

	var hfResponse struct {
		Rows []struct {
			Row struct {
				Input  string `json:"input"`
				Output string `json:"output"`
				DocID  string `json:"doc_id"`
			} `json:"row"`
		} `json:"rows"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&hfResponse); err != nil {
		return 0, 0, fmt.Errorf("failed to decode GOAT response: %w", err)
	}

	fmt.Printf("📊 Received %d rows from GOAT dataset\n", len(hfResponse.Rows))

	// Create JSON output file
	alpacaJsonPath := strings.TrimSuffix(config.OutputFile, ".json") + "_goat_alpaca.json"
	fmt.Printf("📝 Creating GOAT Alpaca JSON output: %s\n", alpacaJsonPath)
	if err := os.MkdirAll(filepath.Dir(alpacaJsonPath), 0755); err != nil {
		return 0, 0, fmt.Errorf("failed to create json directory: %w", err)
	}
	
	jsonFile, err := os.Create(alpacaJsonPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create json file: %w", err)
	}
	defer jsonFile.Close()

	jsonFile.WriteString("[\n")

	var allAlpacaRecords []AlpacaDocumentRecord
	totalEmbeddingsGenerated := 0
	recordsProcessed := 0
	firstRecord := true

	for i, hfRow := range hfResponse.Rows {
		select {
		case <-ctx.Done():
			return recordsProcessed, totalEmbeddingsGenerated, ctx.Err()
		default:
		}

		row := hfRow.Row
		fmt.Printf("🔢 Processing record %d/%d (ID: %s)\n", i+1, len(hfResponse.Rows), row.DocID)

		// Map to Alpaca structure
		alpaca := &AlpacaRecord{
			Instruction: "Solve the following arithmetic problem.",
			Input:       row.Input,
			Output:      row.Output,
		}

		// Extract NLP metadata
		var tokens []string
		var offsets []int32
		var posTags []int
		var tenses []int
		var depHashes []uint32

		// Use the full interaction text for NLP metadata to ensure token alignment
		content := fmt.Sprintf("Instruction: %s\nInput: %s\nOutput: %s", alpaca.Instruction, alpaca.Input, alpaca.Output)

		if nlpBridge != nil {
			tokens, offsets, posTags, tenses, depHashes = nlpBridge.ProcessText(content)
		}

		// Get embedding
		embedText := content
		
		fmt.Printf("🌐 Getting embedding for record %d...\n", i+1)
		embedding, err := provider.GetEmbedding(embedText)
		if err != nil {
			fmt.Printf("❌ Failed to generate embedding for record %d: %v\n", i+1, err)
			continue
		}

		// Create full record
		alpacaRecord := AlpacaDocumentRecord{
			AlpacaRecord: *alpaca,
			FileName:     fmt.Sprintf("hf://stallone/goat/%s", row.DocID),
			ChunkID:      int32(i),
			Content:      content,
			Embedding:    embedding,
			Tokens:       tokens,
			TokenOffsets: offsets,
			POSTags:      posTags,
			Tenses:       tenses,
			DepHashes:    depHashes,
		}

		allAlpacaRecords = append(allAlpacaRecords, alpacaRecord)

		// Write to JSON
		if !firstRecord {
			jsonFile.WriteString(",")
		}
		recordBytes, _ := formatAlpacaRecord(alpacaRecord)
		jsonFile.Write(recordBytes)
		firstRecord = false

		totalEmbeddingsGenerated++
		recordsProcessed++

		// Sync quota
		used, max, _ := provider.RequestTracker.GetStats()
		statsManager.RecordCloudflareUsage(used, max)
	}

	jsonFile.WriteString("\n]")
	
	// Write to Arrow
	arrowPath := strings.TrimSuffix(config.OutputFile, ".json") + "_goat_alpaca.arrow"
	fmt.Printf("📝 Writing GOAT Alpaca Arrow IPC output: %s\n", arrowPath)
	if err := WriteAlpacaDocumentRecordsToArrowIPC(arrowPath, allAlpacaRecords); err != nil {
		fmt.Printf("❌ Failed to write Arrow IPC output: %v\n", err)
	}

	fmt.Printf("🎉 GOAT Mining completed: %d records processed\n", recordsProcessed)
	return recordsProcessed, totalEmbeddingsGenerated, nil
}
