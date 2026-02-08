package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PaperData represents the structure for individual paper JSON files
type PaperData struct {
	FileName       string           `json:"filename"`
	Title          string           `json:"title,omitempty"`
	Authors        []string         `json:"authors,omitempty"`
	ArxivID        string           `json:"arxiv_id,omitempty"`
	Abstract       string           `json:"abstract,omitempty"`
	ProcessedAt    time.Time        `json:"processed_at"`
	Chunks         []DocumentRecord `json:"chunks"`
	ChunkCount     int              `json:"chunk_count"`
	WordCount      int              `json:"word_count"`
	FileSize       int64            `json:"file_size"`
	EmbeddingModel string           `json:"embedding_model"`
	PaperJSONFile  string           `json:"paper_json_file"`
}

// PaperManager handles creating and managing individual paper JSON files
type PaperManager struct {
	papersDir string
}

// NewPaperManager creates a new paper manager
func NewPaperManager(papersDir string) (*PaperManager, error) {
	if err := os.MkdirAll(papersDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create papers directory: %w", err)
	}
	return &PaperManager{papersDir: papersDir}, nil
}

// SavePaper saves a paper's data to its own JSON file
func (pm *PaperManager) SavePaper(paper *PaperData) error {
	// Generate a safe filename
	safeFilename := generateSafeFilename(paper.FileName, paper.Title, paper.ArxivID)
	jsonFile := filepath.Join(pm.papersDir, safeFilename+".json")

	// Add JSON file path to paper metadata
	paper.PaperJSONFile = jsonFile

	// Save to file
	data, err := json.MarshalIndent(paper, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal paper data: %w", err)
	}

	if err := os.WriteFile(jsonFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write paper JSON file: %w", err)
	}

	return nil
}

// LoadPaper loads a paper's data from its JSON file
func (pm *PaperManager) LoadPaper(filename string) (*PaperData, error) {
	jsonFile := filepath.Join(pm.papersDir, filename)
	if !strings.HasSuffix(filename, ".json") {
		jsonFile += ".json"
	}

	data, err := os.ReadFile(jsonFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read paper JSON file: %w", err)
	}

	var paper PaperData
	if err := json.Unmarshal(data, &paper); err != nil {
		return nil, fmt.Errorf("failed to unmarshal paper data: %w", err)
	}

	return &paper, nil
}

// ListPapers returns a list of all paper JSON files
func (pm *PaperManager) ListPapers() ([]string, error) {
	entries, err := os.ReadDir(pm.papersDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read papers directory: %w", err)
	}

	var papers []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			papers = append(papers, entry.Name())
		}
	}

	return papers, nil
}

// DeletePaper removes a paper's JSON file
func (pm *PaperManager) DeletePaper(filename string) error {
	if !strings.HasSuffix(filename, ".json") {
		filename += ".json"
	}
	jsonFile := filepath.Join(pm.papersDir, filename)

	if err := os.Remove(jsonFile); err != nil {
		return fmt.Errorf("failed to delete paper JSON file: %w", err)
	}

	return nil
}

// generateSafeFilename creates a safe filename from paper information
func generateSafeFilename(filename, title, arxivID string) string {
	// Try to use arXiv ID if available
	if arxivID != "" {
		// Clean up arXiv ID (replace slashes with dashes)
		cleanID := strings.ReplaceAll(arxivID, "/", "-")
		return cleanID
	}

	// Use title if available
	if title != "" {
		// Create a short, safe version of the title
		safeTitle := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			return '_'
		}, title)

		// Limit length and add timestamp for uniqueness
		if len(safeTitle) > 50 {
			safeTitle = safeTitle[:50]
		}
		return safeTitle + "_" + time.Now().Format("20060102-150405")
	}

	// Fallback to original filename without extension
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, base)
}

// UpdatePaperMetadata updates paper metadata after processing
func (pm *PaperManager) UpdatePaperMetadata(filename string, embeddingModel string, chunkCount, wordCount int, fileSize int64) error {
	// Load existing paper
	paper, err := pm.LoadPaper(filename)
	if err != nil {
		return err
	}

	// Update metadata
	paper.EmbeddingModel = embeddingModel
	paper.ChunkCount = chunkCount
	paper.WordCount = wordCount
	paper.FileSize = fileSize
	paper.ProcessedAt = time.Now()

	// Save updated paper
	return pm.SavePaper(paper)
}
