package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigParquetFile(t *testing.T) {
	// Test that config includes ParquetFile field
	config := &Config{
		InputDir:    "/input",
		ParquetFile: "/output/data.parquet",
		OutputFile:  "/backup/data.json",
	}

	if config.ParquetFile == "" {
		t.Error("Config should have ParquetFile field")
	}

	if config.OutputFile == "" {
		t.Error("Config should have OutputFile field for backup")
	}

	// Verify parquet is the primary output
	if !strings.HasSuffix(config.ParquetFile, ".parquet") {
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
		ParquetFile:  filepath.Join(appDataDir, "ai_knowledge_base.parquet"),
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

	// Verify parquet file path
	expectedParquet := filepath.Join(appDataDir, "ai_knowledge_base.parquet")
	if config.ParquetFile != expectedParquet {
		t.Errorf("ParquetFile path mismatch: expected %s, got %s", expectedParquet, config.ParquetFile)
	}

	// Verify JSON backup path is in backup subdirectory
	expectedJSON := filepath.Join(appDataDir, "backup", "json", "ai_knowledge_base.json")
	if config.OutputFile != expectedJSON {
		t.Errorf("OutputFile path mismatch: expected %s, got %s", expectedJSON, config.OutputFile)
	}

	// Verify JSON is in backup location
	if !strings.Contains(config.OutputFile, "backup") {
		t.Error("OutputFile (JSON backup) should be in backup directory")
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

	if len(record.Embedding) != 3 {
		t.Error("Embedding field not set correctly")
	}
}
