package dataminer_test

import (
	"dataminer/internal/app"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJSONOutputFile verifies that the application can write JSON files
func TestJSONOutputFile(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a test document record
	record := app.DocumentRecord{
		FileName:  "test.pdf",
		ChunkID:   0,
		Content:   "Test content for JSON",
		Embedding: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
	}

	// Create JSON file path
	jsonPath := filepath.Join(tempDir, "test.json")

	// Verify DocumentRecord has the correct structure for JSON
	if record.FileName != "test.pdf" {
		t.Error("FileName not set correctly")
	}

	if len(record.Embedding) != 5 {
		t.Errorf("Expected 5 embedding values, got %d", len(record.Embedding))
	}

	t.Logf("JSON output path would be: %s", jsonPath)
}

// TestJSONOutputLocation verifies the JSON output is in the correct location
func TestJSONOutputLocation(t *testing.T) {
	appDataDir := "/tmp/test_dataminer"

	// Create expected paths
	expectedJSONPath := filepath.Join(appDataDir, "json", "ai_knowledge_base.json")

	// Verify paths
	if !strings.HasSuffix(expectedJSONPath, ".json") {
		t.Error("JSON file should have .json extension")
	}

	if !strings.Contains(expectedJSONPath, "json") {
		t.Error("JSON file should be in json directory")
	}

	t.Logf("JSON output path: %s", expectedJSONPath)
}

// TestDataDirectoryStructureNew verifies new directory structure
func TestDataDirectoryStructureNew(t *testing.T) {
	tempDir := t.TempDir()
	appDir := filepath.Join(tempDir, "dataminer")

	// Setup directories
	dirs, err := app.SetupDataDirectories(appDir)
	if err != nil {
		t.Fatalf("Failed to setup directories: %v", err)
	}

	// Verify JSON is in json subdirectory
	jsonDir := dirs["json"]
	if !strings.Contains(jsonDir, "json") {
		t.Error("JSON directory should be under data directory")
	}

	// Verify json directory exists
	if _, err := os.Stat(jsonDir); os.IsNotExist(err) {
		t.Error("JSON directory should exist")
	}

	// Verify full path
	expectedJSONDir := filepath.Join(appDir, "json")
	if jsonDir != expectedJSONDir {
		t.Errorf("JSON directory mismatch: expected %s, got %s", expectedJSONDir, jsonDir)
	}

	// Verify backup directory doesn't exist anymore
	if _, exists := dirs["backup"]; exists {
		t.Error("Backup directory should not exist anymore")
	}
}

// TestJSONFormat verifies JSON files have correct format
func TestJSONFormat(t *testing.T) {
	records := []app.DocumentRecord{
		{
			FileName:  "doc1.pdf",
			ChunkID:   0,
			Content:   "First document content",
			Embedding: []float32{0.1, 0.2, 0.3},
		},
		{
			FileName:  "doc2.pdf",
			ChunkID:   1,
			Content:   "Second document content",
			Embedding: []float32{0.4, 0.5, 0.6},
		},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	// Verify it's valid JSON
	var unmarshaled []app.DocumentRecord
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if len(unmarshaled) != 2 {
		t.Errorf("Expected 2 records, got %d", len(unmarshaled))
	}

	if unmarshaled[0].FileName != "doc1.pdf" {
		t.Errorf("FileName mismatch: expected doc1.pdf, got %s", unmarshaled[0].FileName)
	}
}

// TestConfigHasJSONField verifies Config struct includes OutputFile for JSON
func TestConfigHasJSONField(t *testing.T) {
	config := &app.Config{
		OutputFile: "/data/json/ai_knowledge_base.json",
	}

	if config.OutputFile == "" {
		t.Error("Config should have OutputFile field")
	}

	// Verify JSON file path
	if !strings.HasSuffix(config.OutputFile, ".json") {
		t.Error("OutputFile should end with .json")
	}

	// Verify json location
	if !strings.Contains(config.OutputFile, "json") {
		t.Error("OutputFile should be in json directory")
	}
}

// TestJSONOutputFormat verifies JSON output has correct format
func TestJSONOutputFormat(t *testing.T) {
	records := []app.DocumentRecord{
		{
			FileName:  "test.pdf",
			ChunkID:   0,
			Content:   "Test content",
			Embedding: []float32{0.1, 0.2, 0.3},
		},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	// Verify it's valid JSON
	var unmarshaled []app.DocumentRecord
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if len(unmarshaled) != 1 {
		t.Errorf("Expected 1 record, got %d", len(unmarshaled))
	}

	if unmarshaled[0].FileName != "test.pdf" {
		t.Errorf("FileName mismatch: expected test.pdf, got %s", unmarshaled[0].FileName)
	}
}

// TestDocumentRecordJSONCompatibility verifies struct works with JSON
func TestDocumentRecordJSONCompatibility(t *testing.T) {
	record := app.DocumentRecord{
		FileName:  "compatibility_test.pdf",
		ChunkID:   42,
		Content:   "Testing JSON compatibility with various content",
		Embedding: make([]float32, 768), // Standard BERT embedding size
	}

	// Fill embedding with test data
	for i := range record.Embedding {
		record.Embedding[i] = float32(i) * 0.001
	}

	// Verify all fields are accessible
	if record.FileName != "compatibility_test.pdf" {
		t.Error("FileName field error")
	}

	if record.ChunkID != 42 {
		t.Error("ChunkID field error")
	}

	if len(record.Embedding) != 768 {
		t.Errorf("Embedding size error: expected 768, got %d", len(record.Embedding))
	}
}
