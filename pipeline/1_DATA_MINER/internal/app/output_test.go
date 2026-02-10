package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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

func TestWriteOutput(t *testing.T) {
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
	err := writeOutput(jsonPath, results)
	if err != nil {
		t.Fatalf("Failed to write output: %v", err)
	}

	// Verify JSON file exists
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("JSON file should exist")
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
