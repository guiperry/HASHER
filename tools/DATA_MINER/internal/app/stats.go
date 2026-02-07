package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PersistentStats represents stats that persist between application runs
type PersistentStats struct {
	mu sync.RWMutex

	// Daily tracking
	LastResetDate string `json:"last_reset_date"`

	// Cumulative counters (never reset)
	TotalPapersDownloaded    int `json:"total_papers_downloaded"`
	TotalPapersProcessed     int `json:"total_papers_processed"`
	TotalEmbeddingsGenerated int `json:"total_embeddings_generated"`
	TotalWorkflowLoops       int `json:"total_workflow_loops"`

	// Daily counters (reset daily)
	DailyPapersDownloaded    int `json:"daily_papers_downloaded"`
	DailyPapersProcessed     int `json:"daily_papers_processed"`
	DailyEmbeddingsGenerated int `json:"daily_embeddings_generated"`
	DailyWorkflowLoops       int `json:"daily_workflow_loops"`

	// Cloudflare quota tracking
	CloudflareQuotaUsed     int    `json:"cloudflare_quota_used"`
	CloudflareQuotaMax      int    `json:"cloudflare_quota_max"`
	CloudflareLastResetDate string `json:"cloudflare_last_reset_date"`

	// File path for persistence
	statsFile string
}

// StatsManager handles persistent statistics
type StatsManager struct {
	stats *PersistentStats
}

// NewStatsManager creates a new stats manager
func NewStatsManager(appDataDir string) (*StatsManager, error) {
	statsFile := filepath.Join(appDataDir, "stats.json")

	manager := &StatsManager{
		stats: &PersistentStats{
			statsFile:               statsFile,
			CloudflareQuotaMax:      5000, // Default quota
			CloudflareLastResetDate: time.Now().Format("2006-01-02"),
			LastResetDate:           time.Now().Format("2006-01-02"),
		},
	}

	// Try to load existing stats
	if err := manager.Load(); err != nil {
		// If file doesn't exist, that's OK - start fresh
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load stats: %w", err)
		}
	}

	// Check if we need to reset daily counters
	manager.checkAndResetDaily()

	return manager, nil
}

// Load reads stats from the JSON file
func (sm *StatsManager) Load() error {
	sm.stats.mu.Lock()
	defer sm.stats.mu.Unlock()

	data, err := os.ReadFile(sm.stats.statsFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, sm.stats); err != nil {
		return fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	// Ensure statsFile field is preserved
	sm.stats.statsFile = filepath.Dir(sm.stats.statsFile) + "/stats.json"

	return nil
}

// Save writes stats to the JSON file
func (sm *StatsManager) Save() error {
	sm.stats.mu.RLock()
	defer sm.stats.mu.RUnlock()

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(sm.stats.statsFile), 0755); err != nil {
		return fmt.Errorf("failed to create stats directory: %w", err)
	}

	data, err := json.MarshalIndent(sm.stats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	if err := os.WriteFile(sm.stats.statsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write stats file: %w", err)
	}

	return nil
}

// checkAndResetDaily checks if daily counters need to be reset
func (sm *StatsManager) checkAndResetDaily() {
	today := time.Now().Format("2006-01-02")

	sm.stats.mu.Lock()
	defer sm.stats.mu.Unlock()

	// Check if it's a new day
	if sm.stats.LastResetDate != today {
		// Reset daily counters
		sm.stats.DailyPapersDownloaded = 0
		sm.stats.DailyPapersProcessed = 0
		sm.stats.DailyEmbeddingsGenerated = 0
		sm.stats.DailyWorkflowLoops = 0
		sm.stats.LastResetDate = today

		// Also reset Cloudflare quota if it's a new day
		if sm.stats.CloudflareLastResetDate != today {
			sm.stats.CloudflareQuotaUsed = 0
			sm.stats.CloudflareLastResetDate = today
		}
	}
}

// RecordWorkflowLoop records a completed workflow loop
func (sm *StatsManager) RecordWorkflowLoop(papersDownloaded, papersProcessed, embeddingsGenerated int) {
	sm.stats.mu.Lock()
	defer sm.stats.mu.Unlock()

	// Update cumulative counters
	sm.stats.TotalWorkflowLoops++
	sm.stats.TotalPapersDownloaded += papersDownloaded
	sm.stats.TotalPapersProcessed += papersProcessed
	sm.stats.TotalEmbeddingsGenerated += embeddingsGenerated

	// Update daily counters
	sm.stats.DailyWorkflowLoops++
	sm.stats.DailyPapersDownloaded += papersDownloaded
	sm.stats.DailyPapersProcessed += papersProcessed
	sm.stats.DailyEmbeddingsGenerated += embeddingsGenerated
}

// RecordCloudflareUsage records Cloudflare quota usage
func (sm *StatsManager) RecordCloudflareUsage(used, max int) {
	sm.stats.mu.Lock()
	defer sm.stats.mu.Unlock()

	sm.stats.CloudflareQuotaUsed = used
	sm.stats.CloudflareQuotaMax = max
	sm.stats.CloudflareLastResetDate = time.Now().Format("2006-01-02")
}

// GetCurrentStats returns the current stats for display
func (sm *StatsManager) GetCurrentStats() map[string]interface{} {
	sm.stats.mu.RLock()
	defer sm.stats.mu.RUnlock()

	return map[string]interface{}{
		"total_papers_downloaded":    sm.stats.TotalPapersDownloaded,
		"total_papers_processed":     sm.stats.TotalPapersProcessed,
		"total_embeddings_generated": sm.stats.TotalEmbeddingsGenerated,
		"total_workflow_loops":       sm.stats.TotalWorkflowLoops,
		"daily_papers_downloaded":    sm.stats.DailyPapersDownloaded,
		"daily_papers_processed":     sm.stats.DailyPapersProcessed,
		"daily_embeddings_generated": sm.stats.DailyEmbeddingsGenerated,
		"daily_workflow_loops":       sm.stats.DailyWorkflowLoops,
		"cloudflare_quota_used":      sm.stats.CloudflareQuotaUsed,
		"cloudflare_quota_max":       sm.stats.CloudflareQuotaMax,
		"cloudflare_remaining":       sm.stats.CloudflareQuotaMax - sm.stats.CloudflareQuotaUsed,
		"last_reset_date":            sm.stats.LastResetDate,
	}
}

// PrintInitialStatus prints the status when the application starts
func (sm *StatsManager) PrintInitialStatus() {
	stats := sm.GetCurrentStats()

	fmt.Printf("\n📊 Application Status (Session Start)\n")
	fmt.Printf("=====================================\n")
	fmt.Printf("🌐 Cloudflare Quota: %d/%d used (%d remaining)\n",
		stats["cloudflare_quota_used"],
		stats["cloudflare_quota_max"],
		stats["cloudflare_remaining"])
	fmt.Printf("📄 Papers Downloaded: %d\n", stats["daily_papers_downloaded"])
	fmt.Printf("🧠 Papers Processed: %d\n", stats["daily_papers_processed"])
	fmt.Printf("📈 Embeddings Generated: %d\n", stats["daily_embeddings_generated"])
	fmt.Printf("🔄 Workflow Loops: %d\n", stats["daily_workflow_loops"])
	fmt.Printf("=====================================\n")
	fmt.Printf("📊 Totals (All Time):\n")
	fmt.Printf("   Papers Downloaded: %d\n", stats["total_papers_downloaded"])
	fmt.Printf("   Papers Processed: %d\n", stats["total_papers_processed"])
	fmt.Printf("   Embeddings Generated: %d\n", stats["total_embeddings_generated"])
	fmt.Printf("   Workflow Loops: %d\n", stats["total_workflow_loops"])
	fmt.Printf("=====================================\n\n")
}

// Close saves the stats before shutdown
func (sm *StatsManager) Close() error {
	return sm.Save()
}
