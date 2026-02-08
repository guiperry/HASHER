package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

func TestWriteParquetOutput(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	parquetPath := filepath.Join(tempDir, "test.parquet")

	// Create a channel with test data
	results := make(chan DocumentRecord, 3)
	results <- DocumentRecord{
		FileName:  "test1.pdf",
		ChunkID:   0,
		Content:   "Test content 1",
		Embedding: []float32{0.1, 0.2, 0.3},
	}
	results <- DocumentRecord{
		FileName:  "test2.pdf",
		ChunkID:   1,
		Content:   "Test content 2",
		Embedding: []float32{0.4, 0.5, 0.6},
	}
	results <- DocumentRecord{
		FileName:  "test3.pdf",
		ChunkID:   2,
		Content:   "Test content 3",
		Embedding: []float32{0.7, 0.8, 0.9},
	}
	close(results)

	// Write to parquet
	err := writeParquetOutput(parquetPath, results)
	if err != nil {
		t.Fatalf("Failed to write parquet output: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
		t.Fatalf("Parquet file should exist: %s", parquetPath)
	}

	// Read back the parquet file
	fr, err := local.NewLocalFileReader(parquetPath)
	if err != nil {
		t.Fatalf("Failed to open parquet file for reading: %v", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, new(DocumentRecord), 4)
	if err != nil {
		t.Fatalf("Failed to create parquet reader: %v", err)
	}
	defer pr.ReadStop()

	// Check number of rows
	numRows := pr.GetNumRows()
	if numRows != 3 {
		t.Errorf("Expected 3 rows, got %d", numRows)
	}

	// Read records
	records := make([]DocumentRecord, 3)
	if err := pr.Read(&records); err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}

	// Verify first record
	if records[0].FileName != "test1.pdf" {
		t.Errorf("FileName mismatch: expected test1.pdf, got %s", records[0].FileName)
	}
	if records[0].ChunkID != 0 {
		t.Errorf("ChunkID mismatch: expected 0, got %d", records[0].ChunkID)
	}
	if records[0].Content != "Test content 1" {
		t.Errorf("Content mismatch: expected 'Test content 1', got %s", records[0].Content)
	}
	if len(records[0].Embedding) != 3 {
		t.Errorf("Embedding length mismatch: expected 3, got %d", len(records[0].Embedding))
	}
}

func TestWriteJSONOutput(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "test.json")

	// Create a channel with test data
	results := make(chan DocumentRecord, 2)
	results <- DocumentRecord{
		FileName:  "test1.pdf",
		ChunkID:   0,
		Content:   "Test content 1",
		Embedding: []float32{0.1, 0.2, 0.3},
	}
	results <- DocumentRecord{
		FileName:  "test2.pdf",
		ChunkID:   1,
		Content:   "Test content 2",
		Embedding: []float32{0.4, 0.5, 0.6},
	}
	close(results)

	// Write to JSON
	err := writeJSONOutput(jsonPath, results)
	if err != nil {
		t.Fatalf("Failed to write JSON output: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Fatalf("JSON file should exist: %s", jsonPath)
	}

	// Read and verify JSON content
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	// Parse JSON
	var records []DocumentRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify number of records
	if len(records) != 2 {
		t.Errorf("Expected 2 records, got %d", len(records))
	}

	// Verify first record
	if records[0].FileName != "test1.pdf" {
		t.Errorf("FileName mismatch: expected test1.pdf, got %s", records[0].FileName)
	}
	if records[0].ChunkID != 0 {
		t.Errorf("ChunkID mismatch: expected 0, got %d", records[0].ChunkID)
	}
}

func TestWriteOutputBothFormats(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	parquetPath := filepath.Join(tempDir, "test.parquet")
	jsonPath := filepath.Join(tempDir, "test.json")

	// Create a channel with test data
	results := make(chan DocumentRecord, 2)
	results <- DocumentRecord{
		FileName:  "test1.pdf",
		ChunkID:   0,
		Content:   "Test content 1",
		Embedding: []float32{0.1, 0.2, 0.3},
	}
	results <- DocumentRecord{
		FileName:  "test2.pdf",
		ChunkID:   1,
		Content:   "Test content 2",
		Embedding: []float32{0.4, 0.5, 0.6},
	}
	close(results)

	// Write to both formats
	err := writeOutput(parquetPath, jsonPath, results)
	if err != nil {
		t.Fatalf("Failed to write output: %v", err)
	}

	// Verify parquet file exists
	if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
		t.Error("Parquet file should exist")
	}

	// Verify JSON file exists
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("JSON file should exist")
	}
}

func TestWriteParquetOutputEmptyData(t *testing.T) {
	tempDir := t.TempDir()
	parquetPath := filepath.Join(tempDir, "empty.parquet")

	// Create empty channel
	results := make(chan DocumentRecord)
	close(results)

	// Write empty data
	err := writeParquetOutput(parquetPath, results)
	if err != nil {
		t.Fatalf("Failed to write empty parquet: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
		t.Error("Empty parquet file should still exist")
	}
}

func TestWriteJSONOutputEmptyData(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "empty.json")

	// Create empty channel
	results := make(chan DocumentRecord)
	close(results)

	// Write empty data
	err := writeJSONOutput(jsonPath, results)
	if err != nil {
		t.Fatalf("Failed to write empty JSON: %v", err)
	}

	// Verify file exists and contains empty array
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read empty JSON: %v", err)
	}

	content := string(data)
	// Empty JSON array with newlines: opening bracket + newline, then newline + closing bracket
	if content != "[\n\n]" {
		t.Errorf("Empty JSON format mismatch. Expected '[\\n\\n]', got: %q", content)
	}
}

func TestWriteParquetOutputLargeEmbedding(t *testing.T) {
	tempDir := t.TempDir()
	parquetPath := filepath.Join(tempDir, "large.parquet")

	// Create large embedding (768 dimensions like BERT)
	largeEmbedding := make([]float32, 768)
	for i := range largeEmbedding {
		largeEmbedding[i] = float32(i) * 0.001
	}

	results := make(chan DocumentRecord, 1)
	results <- DocumentRecord{
		FileName:  "large.pdf",
		ChunkID:   0,
		Content:   "Large embedding test",
		Embedding: largeEmbedding,
	}
	close(results)

	err := writeParquetOutput(parquetPath, results)
	if err != nil {
		t.Fatalf("Failed to write parquet with large embedding: %v", err)
	}

	// Read back and verify
	fr, err := local.NewLocalFileReader(parquetPath)
	if err != nil {
		t.Fatalf("Failed to read parquet: %v", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, new(DocumentRecord), 4)
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}
	defer pr.ReadStop()

	records := make([]DocumentRecord, 1)
	if err := pr.Read(&records); err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}

	if len(records[0].Embedding) != 768 {
		t.Errorf("Expected 768 embedding dimensions, got %d", len(records[0].Embedding))
	}
}

func TestWriteParquetCreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	// Use a subdirectory that doesn't exist yet
	parquetPath := filepath.Join(tempDir, "subdir1", "subdir2", "test.parquet")

	results := make(chan DocumentRecord, 1)
	results <- DocumentRecord{
		FileName:  "test.pdf",
		ChunkID:   0,
		Content:   "Test",
		Embedding: []float32{0.1},
	}
	close(results)

	err := writeParquetOutput(parquetPath, results)
	if err != nil {
		t.Fatalf("Failed to write parquet with nested directories: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(parquetPath); os.IsNotExist(err) {
		t.Error("Parquet file should exist in nested directory")
	}
}

func TestWriteJSONCreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	// Use a subdirectory that doesn't exist yet
	jsonPath := filepath.Join(tempDir, "subdir1", "subdir2", "test.json")

	results := make(chan DocumentRecord, 1)
	results <- DocumentRecord{
		FileName:  "test.pdf",
		ChunkID:   0,
		Content:   "Test",
		Embedding: []float32{0.1},
	}
	close(results)

	err := writeJSONOutput(jsonPath, results)
	if err != nil {
		t.Fatalf("Failed to write JSON with nested directories: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("JSON file should exist in nested directory")
	}
}
