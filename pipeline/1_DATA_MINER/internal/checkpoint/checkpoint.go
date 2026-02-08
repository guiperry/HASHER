package checkpoint

import (
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

// ProcessedPDFMetadata stores metadata about processed PDF files
type ProcessedPDFMetadata struct {
	FileName      string    `json:"filename"`
	Title         string    `json:"title,omitempty"`
	Authors       []string  `json:"authors,omitempty"`
	ArxivID       string    `json:"arxiv_id,omitempty"`
	ProcessedAt   time.Time `json:"processed_at"`
	FileSize      int64     `json:"file_size"`
	PaperJSONFile string    `json:"paper_json_file"`
}

// Checkpointer handles file processing checkpoints using bbolt
type Checkpointer struct {
	db *bbolt.DB
}

// NewCheckpointer creates a new checkpointer with the given database path
func NewCheckpointer(dbPath string) (*Checkpointer, error) {
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open checkpoint database: %w", err)
	}

	// Create buckets for processed files and metadata
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("ProcessedFiles"))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte("ProcessedPDFs"))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	return &Checkpointer{db: db}, nil
}

// Close closes the underlying database
func (c *Checkpointer) Close() error {
	return c.db.Close()
}

// IsProcessed returns true if the given filename has already been processed
func (c *Checkpointer) IsProcessed(filename string) bool {
	var exists bool
	c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedFiles"))
		v := b.Get([]byte(filename))
		exists = (v != nil)
		return nil
	})
	return exists
}

// MarkAsDone marks the given filename as processed
func (c *Checkpointer) MarkAsDone(filename string) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedFiles"))
		return b.Put([]byte(filename), []byte("completed"))
	})
}

// AddProcessedPDF adds metadata for a processed PDF file
func (c *Checkpointer) AddProcessedPDF(metadata ProcessedPDFMetadata) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedPDFs"))
		data, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		return b.Put([]byte(metadata.FileName), data)
	})
}

// IsPDFProcessed checks if a PDF has been fully processed
func (c *Checkpointer) IsPDFProcessed(filename string) bool {
	var exists bool
	c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedPDFs"))
		v := b.Get([]byte(filename))
		exists = (v != nil)
		return nil
	})
	return exists
}

// GetProcessedPDFMetadata retrieves metadata for a processed PDF
func (c *Checkpointer) GetProcessedPDFMetadata(filename string) (*ProcessedPDFMetadata, error) {
	var metadata *ProcessedPDFMetadata
	err := c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedPDFs"))
		data := b.Get([]byte(filename))
		if data == nil {
			return fmt.Errorf("PDF metadata not found: %s", filename)
		}
		return json.Unmarshal(data, &metadata)
	})
	return metadata, err
}

// GetAllProcessedPDFs returns metadata for all processed PDFs
func (c *Checkpointer) GetAllProcessedPDFs() ([]ProcessedPDFMetadata, error) {
	var pdfs []ProcessedPDFMetadata
	err := c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedPDFs"))
		return b.ForEach(func(k, v []byte) error {
			var metadata ProcessedPDFMetadata
			if err := json.Unmarshal(v, &metadata); err != nil {
				return err
			}
			pdfs = append(pdfs, metadata)
			return nil
		})
	})
	return pdfs, err
}

// RemoveProcessedPDF removes a PDF from the processed list
func (c *Checkpointer) RemoveProcessedPDF(filename string) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedPDFs"))
		return b.Delete([]byte(filename))
	})
}
