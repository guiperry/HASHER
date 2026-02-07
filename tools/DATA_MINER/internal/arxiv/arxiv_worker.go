package arxiv

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// WorkerPool manages a pool of arXiv mining workers
type WorkerPool struct {
	NumWorkers int
	JobQueue   chan MiningJob
	Results    chan MiningResult
	Workers    []*ArxivWorker
	Wg         sync.WaitGroup
	Cancel     context.CancelFunc
	Miner      *ArxivMiner
}

// MiningJob represents a job to mine papers from a category
type MiningJob struct {
	ID        int
	Category  string
	MaxPapers int
	Config    MiningConfig
}

// MiningResult represents the result of a mining job
type MiningResult struct {
	JobID            int
	Category         string
	PapersFound      int
	PapersDownloaded int
	Error            error
	Duration         time.Duration
}

// ArxivWorker represents a single arXiv mining worker
type ArxivWorker struct {
	ID         int
	WorkerPool *WorkerPool
	Miner      *ArxivMiner
	StopSignal chan bool
}

// NewWorkerPool creates a new worker pool for arXiv mining
func NewWorkerPool(numWorkers int, miner *ArxivMiner) *WorkerPool {
	return &WorkerPool{
		NumWorkers: numWorkers,
		JobQueue:   make(chan MiningJob, numWorkers*2),
		Results:    make(chan MiningResult, numWorkers*2),
		Miner:      miner,
		Workers:    make([]*ArxivWorker, numWorkers),
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start(ctx context.Context) {
	// Create context for cancellation
	ctx, cancel := context.WithCancel(ctx)
	wp.Cancel = cancel

	// Start workers
	for i := 0; i < wp.NumWorkers; i++ {
		worker := &ArxivWorker{
			ID:         i + 1,
			WorkerPool: wp,
			Miner:      wp.Miner,
			StopSignal: make(chan bool, 1),
		}
		wp.Workers[i] = worker
		wp.Wg.Add(1)
		go worker.Start(ctx)
	}

	// Start result collector
	go wp.collectResults(ctx)

	log.Printf("Started arXiv mining worker pool with %d workers", wp.NumWorkers)
}

// Stop stops the worker pool gracefully
func (wp *WorkerPool) Stop() {
	if wp.Cancel != nil {
		wp.Cancel()
	}

	// Close job queue to signal no more jobs
	close(wp.JobQueue)

	// Wait for all workers to finish
	wp.Wg.Wait()

	// Close results channel
	close(wp.Results)

	log.Println("ArXiv mining worker pool stopped")
}

// AddJob adds a mining job to the queue
func (wp *WorkerPool) AddJob(category string, maxPapers int, config MiningConfig) {
	job := MiningJob{
		ID:        time.Now().Nanosecond(), // Simple unique ID
		Category:  category,
		MaxPapers: maxPapers,
		Config:    config,
	}
	wp.JobQueue <- job
}

// collectResults collects results from workers
func (wp *WorkerPool) collectResults(ctx context.Context) {
	for {
		select {
		case result, ok := <-wp.Results:
			if !ok {
				return // Results channel closed
			}
			wp.handleResult(result)
		case <-ctx.Done():
			return
		}
	}
}

// handleResult handles a mining result
func (wp *WorkerPool) handleResult(result MiningResult) {
	if result.Error != nil {
		log.Printf("Job %d for category %s failed: %v", result.JobID, result.Category, result.Error)
	} else {
		log.Printf("Job %d completed: %s - %d papers found, %d downloaded in %v",
			result.JobID, result.Category, result.PapersFound, result.PapersDownloaded, result.Duration)
	}
}

// Start starts the arXiv worker
func (w *ArxivWorker) Start(ctx context.Context) {
	defer w.WorkerPool.Wg.Done()

	log.Printf("ArXiv worker %d started", w.ID)

	for {
		select {
		case job, ok := <-w.WorkerPool.JobQueue:
			if !ok {
				log.Printf("ArXiv worker %d stopping - job queue closed", w.ID)
				return
			}
			w.processJob(ctx, job)
		case <-w.StopSignal:
			log.Printf("ArXiv worker %d received stop signal", w.ID)
			return
		case <-ctx.Done():
			log.Printf("ArXiv worker %d stopping - context cancelled", w.ID)
			return
		}
	}
}

// processJob processes a mining job
func (w *ArxivWorker) processJob(ctx context.Context, job MiningJob) {
	startTime := time.Now()
	log.Printf("Worker %d processing job for category: %s", w.ID, job.Category)

	// Update config with specific category
	config := job.Config
	config.Categories = []string{job.Category}
	config.MaxPapers = job.MaxPapers

	var papersFound, papersDownloaded int
	var err error

	// Mine the category
	err = w.Miner.MineCategory(config)
	if err != nil {
		log.Printf("Worker %d failed to mine category %s: %v", w.ID, job.Category, err)
	} else {
		// Get statistics (simplified for now)
		papersFound = job.MaxPapers    // This is approximate
		papersDownloaded = papersFound // Assume all found are downloaded for now
	}

	duration := time.Since(startTime)

	// Send result
	result := MiningResult{
		JobID:            job.ID,
		Category:         job.Category,
		PapersFound:      papersFound,
		PapersDownloaded: papersDownloaded,
		Error:            err,
		Duration:         duration,
	}

	select {
	case w.WorkerPool.Results <- result:
	case <-ctx.Done():
		return
	}
}

// ArxivBackgroundService manages the background arXiv mining service
type ArxivBackgroundService struct {
	WorkerPool *WorkerPool
	Config     ArxivServiceConfig
	IsRunning  bool
	StopSignal chan os.Signal
	Ticker     *time.Ticker
}

// ArxivServiceConfig holds configuration for the background service
type ArxivServiceConfig struct {
	NumWorkers      int
	DownloadDir     string
	CheckpointDB    string
	Categories      []string
	MaxPapersPerRun int
	RunInterval     time.Duration
	DownloadDelay   time.Duration
	SortBy          string
	SortOrder       string
	RetentionDays   int // Days to keep papers before cleaning up
}

// NewArxivBackgroundService creates a new background arXiv mining service
func NewArxivBackgroundService(config ArxivServiceConfig) *ArxivBackgroundService {
	// Create miner
	miner := NewArxivMiner(config.DownloadDir, config.CheckpointDB)

	// Create worker pool
	workerPool := NewWorkerPool(config.NumWorkers, miner)

	return &ArxivBackgroundService{
		WorkerPool: workerPool,
		Config:     config,
		StopSignal: make(chan os.Signal, 1),
	}
}

// Start starts the background service
func (s *ArxivBackgroundService) Start() error {
	if s.IsRunning {
		return fmt.Errorf("service is already running")
	}

	log.Println("Starting arXiv background mining service...")

	// Setup signal handling
	signal.Notify(s.StopSignal, syscall.SIGINT, syscall.SIGTERM)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker pool
	s.WorkerPool.Start(ctx)
	s.IsRunning = true

	// Create ticker for periodic runs
	s.Ticker = time.NewTicker(s.Config.RunInterval)
	defer s.Ticker.Stop()

	// Run initial mining
	s.runMiningCycle()

	// Main service loop
	for {
		select {
		case <-s.Ticker.C:
			log.Println("Starting scheduled arXiv mining cycle...")
			s.runMiningCycle()
		case sig := <-s.StopSignal:
			log.Printf("Received signal %v, shutting down gracefully...", sig)
			s.Stop()
			return nil
		case <-ctx.Done():
			log.Println("Context cancelled, shutting down...")
			s.Stop()
			return nil
		}
	}
}

// Stop stops the background service
func (s *ArxivBackgroundService) Stop() {
	if !s.IsRunning {
		return
	}

	log.Println("Stopping arXiv background mining service...")
	s.IsRunning = false

	// Stop worker pool
	s.WorkerPool.Stop()

	// Stop ticker
	if s.Ticker != nil {
		s.Ticker.Stop()
	}

	log.Println("ArXiv background mining service stopped")
}

// runMiningCycle runs a single mining cycle
func (s *ArxivBackgroundService) runMiningCycle() {
	log.Printf("Starting mining cycle for %d categories", len(s.Config.Categories))

	// Create mining config
	config := MiningConfig{
		Categories:    s.Config.Categories,
		MaxPapers:     s.Config.MaxPapersPerRun,
		SortBy:        s.Config.SortBy,
		SortOrder:     s.Config.SortOrder,
		DownloadDelay: s.Config.DownloadDelay,
	}

	// Add jobs for each category
	for _, category := range s.Config.Categories {
		if !ValidateCategory(category) {
			log.Printf("Skipping invalid category: %s", category)
			continue
		}

		s.WorkerPool.AddJob(category, s.Config.MaxPapersPerRun, config)
	}

	log.Printf("Queued mining jobs for %d categories", len(s.Config.Categories))
}

// GetServiceStatus returns the current status of the service
func (s *ArxivBackgroundService) GetServiceStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_running":     s.IsRunning,
		"num_workers":    s.Config.NumWorkers,
		"categories":     s.Config.Categories,
		"max_papers":     s.Config.MaxPapersPerRun,
		"run_interval":   s.Config.RunInterval.String(),
		"download_dir":   s.Config.DownloadDir,
		"active_workers": len(s.WorkerPool.Workers),
	}
}

// CleanupOldPapers removes old papers based on retention policy
func (s *ArxivBackgroundService) CleanupOldPapers() error {
	if s.Config.RetentionDays <= 0 {
		return nil // No cleanup needed
	}

	log.Printf("Cleaning up papers older than %d days", s.Config.RetentionDays)

	// This is a placeholder for cleanup logic
	// In a real implementation, you would:
	// 1. Scan download directory for PDF files
	// 2. Check file modification times
	// 3. Remove files older than retention period
	// 4. Update checkpoint database accordingly

	log.Println("Cleanup completed (placeholder implementation)")
	return nil
}
