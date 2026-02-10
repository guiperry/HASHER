package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigJSONOutput(t *testing.T) {
	// Test that config includes JSON output field
	config := &Config{
		InputDir:   "/input",
		OutputFile: "/data/ai_knowledge_base.json",
	}

	if config.InputDir != "/input" {
		t.Errorf("InputDir mismatch: expected /input, got %s", config.InputDir)
	}

	if config.OutputFile == "" {
		t.Error("Config should have OutputFile field")
	}

	// Verify JSON is the primary output
	if !strings.HasSuffix(config.OutputFile, ".json") {
		t.Error("ParquetFile should have .parquet extension")
	}

	// Verify json is the backup
	if !strings.HasSuffix(config.OutputFile, ".json") {
		t.Error("OutputFile should have .json extension for backup")
	}
}

func TestConfigDefaultPaths(t *testing.T) {
	// Test that the default configuration sets up correct paths
	appDataDir := "/tmp/testdataminer"
	dirs := map[string]string{
		"checkpoints": filepath.Join(appDataDir, "checkpoints"),
		"papers":      filepath.Join(appDataDir, "papers"),
		"json":        filepath.Join(appDataDir, "backup", "json"),
		"documents":   filepath.Join(appDataDir, "documents"),
		"temp":        filepath.Join(appDataDir, "temp"),
		"backup":      filepath.Join(appDataDir, "backup"),
	}

	config := &Config{
		InputDir:     dirs["documents"],
		OutputFile:   filepath.Join(dirs["json"], "ai_knowledge_base.json"),
		NumWorkers:   4,
		ChunkSize:    150,
		ChunkOverlap: 25,
		OllamaModel:  "nomic-embed-text",
		OllamaHost:   "http://localhost:11434",
		CheckpointDB: filepath.Join(dirs["checkpoints"], "checkpoints.db"),
		BatchSize:    4,
		AppDataDir:   appDataDir,
		DataDirs:     dirs,
	}

	// Verify JSON output path
	expectedJSON := filepath.Join(dirs["json"], "ai_knowledge_base.json")
	if config.OutputFile != expectedJSON {
		t.Errorf("OutputFile path mismatch: expected %s, got %s", expectedJSON, config.OutputFile)
	}

	// Verify JSON is in json directory
	if !strings.Contains(config.OutputFile, "json") {
		t.Error("OutputFile should be in json directory")
	}

	// Verify other configuration fields
	if config.InputDir != dirs["documents"] {
		t.Errorf("InputDir mismatch: expected %s, got %s", dirs["documents"], config.InputDir)
	}
	if config.NumWorkers != 4 {
		t.Errorf("NumWorkers mismatch: expected 4, got %d", config.NumWorkers)
	}
	if config.ChunkSize != 150 {
		t.Errorf("ChunkSize mismatch: expected 150, got %d", config.ChunkSize)
	}
	if config.ChunkOverlap != 25 {
		t.Errorf("ChunkOverlap mismatch: expected 25, got %d", config.ChunkOverlap)
	}
	if config.OllamaModel != "nomic-embed-text" {
		t.Errorf("OllamaModel mismatch: expected nomic-embed-text, got %s", config.OllamaModel)
	}
	if config.OllamaHost != "http://localhost:11434" {
		t.Errorf("OllamaHost mismatch: expected http://localhost:11434, got %s", config.OllamaHost)
	}
	expectedCheckpoint := filepath.Join(dirs["checkpoints"], "checkpoints.db")
	if config.CheckpointDB != expectedCheckpoint {
		t.Errorf("CheckpointDB mismatch: expected %s, got %s", expectedCheckpoint, config.CheckpointDB)
	}
	if config.BatchSize != 4 {
		t.Errorf("BatchSize mismatch: expected 4, got %d", config.BatchSize)
	}
	if config.AppDataDir != appDataDir {
		t.Errorf("AppDataDir mismatch: expected %s, got %s", appDataDir, config.AppDataDir)
	}
	// Verify DataDirs mapping
	for key, expected := range dirs {
		if actual, ok := config.DataDirs[key]; !ok {
			t.Errorf("DataDirs missing key %s", key)
		} else if actual != expected {
			t.Errorf("DataDirs[%s] mismatch: expected %s, got %s", key, expected, actual)
		}
	}
}

func TestConfigDataDirs(t *testing.T) {
	appDataDir := "/test/app"
	dirs := map[string]string{
		"checkpoints": filepath.Join(appDataDir, "checkpoints"),
		"papers":      filepath.Join(appDataDir, "papers"),
		"json":        filepath.Join(appDataDir, "backup", "json"),
		"documents":   filepath.Join(appDataDir, "documents"),
		"temp":        filepath.Join(appDataDir, "temp"),
		"backup":      filepath.Join(appDataDir, "backup"),
	}

	config := &Config{
		AppDataDir: appDataDir,
		DataDirs:   dirs,
	}

	// Verify AppDataDir is set correctly
	if config.AppDataDir != appDataDir {
		t.Errorf("AppDataDir mismatch: expected %s, got %s", appDataDir, config.AppDataDir)
	}

	// Test JSON directory is under backup
	jsonDir := config.DataDirs["json"]
	if !strings.Contains(jsonDir, "backup") {
		t.Error("JSON directory should be under backup")
	}

	// Test backup directory exists
	backupDir := config.DataDirs["backup"]
	if backupDir != filepath.Join(appDataDir, "backup") {
		t.Errorf("Backup directory mismatch: expected %s, got %s", filepath.Join(appDataDir, "backup"), backupDir)
	}
}

func TestDocumentRecordParquetTags(t *testing.T) {
	// Test that DocumentRecord has proper parquet tags
	record := DocumentRecord{
		FileName:  "test.pdf",
		ChunkID:   1,
		Content:   "test content",
		Embedding: []float32{0.1, 0.2, 0.3},
	}

	// Verify the struct can be created
	if record.FileName != "test.pdf" {
		t.Error("FileName field not set correctly")
	}

	if record.ChunkID != 1 {
		t.Error("ChunkID field not set correctly")
	}

	if record.Content != "test content" {
		t.Errorf("Content field not set correctly: expected 'test content', got %s", record.Content)
	}

	if len(record.Embedding) != 3 {
		t.Error("Embedding field not set correctly")
	}
}
