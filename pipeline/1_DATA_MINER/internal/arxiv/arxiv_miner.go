package arxiv

import (
	"data-miner/internal/checkpoint"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PDFDownloader handles downloading PDFs from arXiv
type PDFDownloader struct {
	HTTPClient  *http.Client
	DownloadDir string
	MaxRetries  int
	RetryDelay  time.Duration
	UserAgent   string
}

// NewPDFDownloader creates a new PDF downloader
func NewPDFDownloader(downloadDir string) *PDFDownloader {
	return &PDFDownloader{
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		DownloadDir: downloadDir,
		MaxRetries:  3,
		RetryDelay:  2 * time.Second,
		UserAgent:   "Neural-Miner/1.0 (arXiv PDF Downloader)",
	}
}

// DownloadPDF downloads a PDF from the given URL
func (d *PDFDownloader) DownloadPDF(pdfURL, arxivID string) (string, error) {
	// Create download directory if it doesn't exist
	if err := os.MkdirAll(d.DownloadDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create download directory: %w", err)
	}

	// Generate filename
	filename := d.generateFilename(arxivID)
	filepath := filepath.Join(d.DownloadDir, filename)

	// Check if file already exists
	if _, err := os.Stat(filepath); err == nil {
		return filepath, nil // File already exists
	}

	// Download with retries
	var lastErr error
	for attempt := 0; attempt < d.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(d.RetryDelay * time.Duration(attempt))
		}

		err := d.downloadWithRetry(pdfURL, filepath)
		if err == nil {
			return filepath, nil
		}
		lastErr = err
	}

	return "", fmt.Errorf("failed to download PDF after %d attempts: %w", d.MaxRetries, lastErr)
}

// downloadWithRetry performs the actual download
func (d *PDFDownloader) downloadWithRetry(pdfURL, filepath string) error {
	// Create HTTP request
	req, err := http.NewRequest("GET", pdfURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", d.UserAgent)

	// Make request
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Create file
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy content
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(filepath) // Clean up partial file
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// generateFilename generates a filename for the PDF
func (d *PDFDownloader) generateFilename(arxivID string) string {
	// Sanitize arxivID for filename
	sanitized := strings.ReplaceAll(arxivID, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	return fmt.Sprintf("%s.pdf", sanitized)
}

// ArxivMiner coordinates the mining process
type ArxivMiner struct {
	Client       *ArxivClient
	Downloader   *PDFDownloader
	Checkpointer *checkpoint.Checkpointer
}

// NewArxivMiner creates a new arXiv miner
func NewArxivMiner(downloadDir string, checkpointDB string) *ArxivMiner {
	checkpointer, err := checkpoint.NewCheckpointer(checkpointDB)
	if err != nil {
		log.Printf("Warning: failed to initialize checkpointer: %v", err)
		// Continue without checkpointing for now
		checkpointer = nil
	}

	return &ArxivMiner{
		Client:       NewArxivClient(),
		Downloader:   NewPDFDownloader(downloadDir),
		Checkpointer: checkpointer,
	}
}

// MiningConfig holds configuration for the mining process
type MiningConfig struct {
	Categories    []string
	MaxPapers     int
	SortBy        string
	SortOrder     string
	StartDate     string
	EndDate       string
	DownloadDelay time.Duration
}

// MineCategory downloads papers from specific categories with global limit
func (m *ArxivMiner) MineCategory(config MiningConfig) error {
	totalDownloaded := 0
	remainingPapers := config.MaxPapers

	log.Printf("Starting mining with global limit: %d papers total", config.MaxPapers)

	for _, category := range config.Categories {
		if remainingPapers <= 0 {
			log.Printf("Reached global limit of %d papers, stopping", config.MaxPapers)
			break
		}

		log.Printf("Mining category: %s (remaining: %d)", category, remainingPapers)

		// Search for papers in category (request more than we need to account for already processed)
		searchLimit := remainingPapers
		if searchLimit < 10 {
			searchLimit = 10 // Get more to filter out already processed
		}

		feed, err := m.Client.SearchByCategory(category, searchLimit)
		if err != nil {
			log.Printf("Failed to search category %s: %v", category, err)
			continue
		}

		log.Printf("Found %d papers in category %s", len(feed.Entries), category)

		// Download papers until we hit the global limit
		categoryDownloaded := 0
		for i, entry := range feed.Entries {
			if remainingPapers <= 0 {
				log.Printf("Reached global limit, stopping downloads")
				break
			}

			paperID := entry.GetArxivID()
			if paperID == "" {
				log.Printf("Skipping entry %d: no arXiv ID found", i)
				continue
			}

			// Check if already processed
			if m.Checkpointer != nil && m.Checkpointer.IsProcessed(paperID) {
				log.Printf("Skipping already processed paper: %s", paperID)
				continue
			}

			pdfURL := entry.GetPDFURL()
			if pdfURL == "" {
				log.Printf("No PDF URL found for paper: %s", paperID)
				continue
			}

			log.Printf("Downloading PDF for paper: %s (%s) [%d/%d]",
				entry.Title, paperID, totalDownloaded+1, config.MaxPapers)

			// Download PDF
			filepath, err := m.Downloader.DownloadPDF(pdfURL, paperID)
			if err != nil {
				log.Printf("Failed to download PDF for %s: %v", paperID, err)
				continue
			}

			log.Printf("Successfully downloaded: %s", filepath)

			// Mark as processed
			if m.Checkpointer != nil {
				if err := m.Checkpointer.MarkAsDone(paperID); err != nil {
					log.Printf("Failed to mark paper %s as processed: %v", paperID, err)
				}
			}

			// Update counters
			totalDownloaded++
			remainingPapers--
			categoryDownloaded++

			// Add delay between downloads to be respectful
			if config.DownloadDelay > 0 {
				time.Sleep(config.DownloadDelay)
			}
		}

		log.Printf("Downloaded %d papers from category %s (total: %d/%d)",
			categoryDownloaded, category, totalDownloaded, config.MaxPapers)

		// If we've downloaded enough papers, stop
		if totalDownloaded >= config.MaxPapers {
			log.Printf("Successfully downloaded target of %d papers", config.MaxPapers)
			break
		}
	}

	log.Printf("Mining completed. Total papers downloaded: %d/%d", totalDownloaded, config.MaxPapers)
	return nil
}

// MineRecentPapers mines papers from multiple categories with date filtering
func (m *ArxivMiner) MineRecentPapers(config MiningConfig) error {
	// Build search query with date range if specified
	query := ""
	if len(config.Categories) == 1 {
		query = fmt.Sprintf("cat:%s", config.Categories[0])
	} else {
		// Multiple categories - use OR logic
		categoryQueries := make([]string, len(config.Categories))
		for i, cat := range config.Categories {
			categoryQueries[i] = fmt.Sprintf("cat:%s", cat)
		}
		query = fmt.Sprintf("(%s)", strings.Join(categoryQueries, "+OR+"))
	}

	// Add date filter if specified
	if config.StartDate != "" && config.EndDate != "" {
		dateFilter := fmt.Sprintf("submittedDate:[%s+TO+%s]", config.StartDate, config.EndDate)
		if query != "" {
			query = fmt.Sprintf("%s+AND+%s", query, dateFilter)
		} else {
			query = dateFilter
		}
	}

	searchReq := SearchRequest{
		Query:      query,
		MaxResults: config.MaxPapers,
		Start:      0,
		SortBy:     config.SortBy,
		SortOrder:  config.SortOrder,
	}

	feed, err := m.Client.Search(searchReq)
	if err != nil {
		return fmt.Errorf("failed to search recent papers: %w", err)
	}

	log.Printf("Found %d papers matching criteria", len(feed.Entries))

	// Download papers
	for i, entry := range feed.Entries {
		paperID := entry.GetArxivID()
		if paperID == "" {
			log.Printf("Skipping entry %d: no arXiv ID found", i)
			continue
		}

		if m.Checkpointer != nil && m.Checkpointer.IsProcessed(paperID) {
			log.Printf("Skipping already processed paper: %s", paperID)
			continue
		}

		pdfURL := entry.GetPDFURL()
		if pdfURL == "" {
			log.Printf("No PDF URL found for paper: %s", paperID)
			continue
		}

		log.Printf("Downloading: %s (%s)", entry.Title, paperID)

		filepath, err := m.Downloader.DownloadPDF(pdfURL, paperID)
		if err != nil {
			log.Printf("Failed to download PDF for %s: %v", paperID, err)
			continue
		}

		log.Printf("Successfully downloaded: %s", filepath)

		if m.Checkpointer != nil {
			if err := m.Checkpointer.MarkAsDone(paperID); err != nil {
				log.Printf("Failed to mark paper %s as processed: %v", paperID, err)
			}
		}

		if config.DownloadDelay > 0 {
			time.Sleep(config.DownloadDelay)
		}
	}

	return nil
}

// GetMiningStats returns statistics about the mining process
func (m *ArxivMiner) GetMiningStats() (map[string]int, error) {
	stats := make(map[string]int)

	// This would require extending the checkpointer to provide statistics
	// For now, return empty stats
	return stats, nil
}

// ValidateCategory checks if a category is valid
func ValidateCategory(category string) bool {
	categories := GetCategoryMapping()
	_, exists := categories[category]
	return exists
}

// GetRecommendedCategories returns recommended categories for ML/AI research
func GetRecommendedCategories() []string {
	return []string{
		"cs.AI",           // Artificial Intelligence
		"cs.LG",           // Machine Learning
		"cs.CV",           // Computer Vision
		"cs.CL",           // Natural Language Processing
		"cs.NE",           // Neural and Evolutionary Computing
		"stat.ML",         // Machine Learning (Statistics perspective)
		"stat.AP",         // Applications of Statistics
		"math.NA",         // Numerical Analysis
		"physics.comp-ph", // Computational Physics
		"q-bio.NC",        // Neurons and Cognition
		"q-bio.QM",        // Quantitative Methods for Biology
	}
}
