package checkpoint

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Checkpoint tracks processing progress and quota usage
type Checkpoint struct {
	mu sync.RWMutex

	// Quota tracking
	QuotaLimit    int  `json:"quota_limit"`
	QuotaUsed     int  `json:"quota_used"`
	QuotaExceeded bool `json:"quota_exceeded"`

	// Progress tracking
	LastProcessedFile   string `json:"last_processed_file"`
	LastProcessedChunk  int32  `json:"last_processed_chunk"`
	LastProcessedWindow int32  `json:"last_processed_window"`

	// Statistics
	RecordsProcessed int64 `json:"records_processed"`
	FramesGenerated  int64 `json:"frames_generated"`
	LastUpdated      int64 `json:"last_updated"`
}

// Manager handles checkpoint operations
type Manager struct {
	checkpointPath string
	checkpoint     *Checkpoint
}

// NewManager creates a new checkpoint manager
func NewManager(outputFile string) (*Manager, error) {
	// Store checkpoint next to output file
	checkpointPath := outputFile + ".checkpoint.json"

	mgr := &Manager{
		checkpointPath: checkpointPath,
		checkpoint: &Checkpoint{
			QuotaLimit:  5000, // Default: 5000 embeddings
			QuotaUsed:   0,
			LastUpdated: time.Now().Unix(),
		},
	}

	// Try to load existing checkpoint
	if err := mgr.Load(); err != nil {
		// No existing checkpoint or error loading - start fresh
		fmt.Printf("📋 Starting fresh checkpoint (quota: %d embeddings)\n", mgr.checkpoint.QuotaLimit)
	} else {
		fmt.Printf("📋 Resumed checkpoint: %d/%d embeddings used, %d records processed\n",
			mgr.checkpoint.QuotaUsed, mgr.checkpoint.QuotaLimit, mgr.checkpoint.RecordsProcessed)
	}

	return mgr, nil
}

// Load reads checkpoint from disk
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.checkpointPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no checkpoint file found")
		}
		return fmt.Errorf("failed to read checkpoint: %w", err)
	}

	m.checkpoint.mu.Lock()
	defer m.checkpoint.mu.Unlock()

	if err := json.Unmarshal(data, m.checkpoint); err != nil {
		return fmt.Errorf("failed to parse checkpoint: %w", err)
	}

	return nil
}

// Save writes checkpoint to disk atomically
func (m *Manager) Save() error {
	m.checkpoint.mu.RLock()
	m.checkpoint.LastUpdated = time.Now().Unix()
	data, err := json.MarshalIndent(m.checkpoint, "", "  ")
	m.checkpoint.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Write to temp file then rename for atomicity
	tempPath := m.checkpointPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}

	if err := os.Rename(tempPath, m.checkpointPath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to rename checkpoint: %w", err)
	}

	return nil
}

// UseQuota checks if quota is available and increments usage
func (m *Manager) UseQuota(amount int) bool {
	m.checkpoint.mu.Lock()
	defer m.checkpoint.mu.Unlock()

	if m.checkpoint.QuotaUsed+amount > m.checkpoint.QuotaLimit {
		m.checkpoint.QuotaExceeded = true
		return false
	}

	m.checkpoint.QuotaUsed += amount
	return true
}

// HasQuotaAvailable checks if there's quota remaining
func (m *Manager) HasQuotaAvailable() bool {
	m.checkpoint.mu.RLock()
	defer m.checkpoint.mu.RUnlock()

	return m.checkpoint.QuotaUsed < m.checkpoint.QuotaLimit && !m.checkpoint.QuotaExceeded
}

// GetQuotaStatus returns current quota status
func (m *Manager) GetQuotaStatus() (used, limit int, remaining int) {
	m.checkpoint.mu.RLock()
	defer m.checkpoint.mu.RUnlock()

	return m.checkpoint.QuotaUsed, m.checkpoint.QuotaLimit, m.checkpoint.QuotaLimit - m.checkpoint.QuotaUsed
}

// UpdateProgress updates the last processed position
func (m *Manager) UpdateProgress(file string, chunkID, windowStart int32) {
	m.checkpoint.mu.Lock()
	defer m.checkpoint.mu.Unlock()

	m.checkpoint.LastProcessedFile = file
	m.checkpoint.LastProcessedChunk = chunkID
	m.checkpoint.LastProcessedWindow = windowStart
	m.checkpoint.LastUpdated = time.Now().Unix()
}

// IncrementStats increments processing statistics
func (m *Manager) IncrementStats(records, frames int64) {
	m.checkpoint.mu.Lock()
	defer m.checkpoint.mu.Unlock()

	m.checkpoint.RecordsProcessed += records
	m.checkpoint.FramesGenerated += frames
	m.checkpoint.LastUpdated = time.Now().Unix()
}

// ShouldSkipRecord checks if a record has already been processed
func (m *Manager) ShouldSkipRecord(file string, chunkID, windowStart int32) bool {
	m.checkpoint.mu.RLock()
	defer m.checkpoint.mu.RUnlock()

	// Simple comparison - skip if this record is before or at the checkpoint
	if file < m.checkpoint.LastProcessedFile {
		return true
	}
	if file == m.checkpoint.LastProcessedFile {
		if chunkID < m.checkpoint.LastProcessedChunk {
			return true
		}
		if chunkID == m.checkpoint.LastProcessedChunk && windowStart < m.checkpoint.LastProcessedWindow {
			return true
		}
	}
	return false
}

// GetCheckpoint returns a copy of the current checkpoint
func (m *Manager) GetCheckpoint() Checkpoint {
	m.checkpoint.mu.RLock()
	defer m.checkpoint.mu.RUnlock()

	return *m.checkpoint
}

// SetQuotaLimit sets the quota limit (useful for testing or overrides)
func (m *Manager) SetQuotaLimit(limit int) {
	m.checkpoint.mu.Lock()
	defer m.checkpoint.mu.Unlock()

	m.checkpoint.QuotaLimit = limit
}

// ResetDailyQuota resets the quota usage if it's a new day
func (m *Manager) ResetDailyQuota() bool {
	m.checkpoint.mu.Lock()
	defer m.checkpoint.mu.Unlock()

	now := time.Now()
	lastUpdate := time.Unix(m.checkpoint.LastUpdated, 0)

	// Check if it's a different day
	if now.Year() != lastUpdate.Year() || now.YearDay() != lastUpdate.YearDay() {
		m.checkpoint.QuotaUsed = 0
		m.checkpoint.QuotaExceeded = false
		m.checkpoint.LastUpdated = now.Unix()
		log.Printf("🌅 New day detected - quota reset to 0/%d", m.checkpoint.QuotaLimit)
		return true
	}

	return false
}

// GetCheckpointPath returns the path to the checkpoint file
func (m *Manager) GetCheckpointPath() string {
	return m.checkpointPath
}

// EnsureDir ensures the checkpoint directory exists
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}
