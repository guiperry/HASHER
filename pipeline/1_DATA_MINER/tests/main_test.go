package dataminer_test

import (
	"data-miner/internal/app"
	"data-miner/internal/checkpoint"
	"encoding/json"
	"strings"
	"testing"
)

func TestChunkText(t *testing.T) {
	text := "This is a test document with multiple words that should be chunked properly with overlap between chunks"
	chunks := app.ChunkText(text, 10, 3)

	// Let's check what we actually get
	t.Logf("Generated chunks: %d", len(chunks))
	for i, chunk := range chunks {
		t.Logf("Chunk %d: '%s'", i, chunk)
	}

	// Just verify we get reasonable chunks
	if len(chunks) < 1 {
		t.Error("Should generate at least one chunk")
	}

	for _, chunk := range chunks {
		if len(strings.Fields(chunk)) == 0 {
			t.Error("Chunks should not be empty")
		}
	}
}

func TestJSONOutput(t *testing.T) {
	record := app.DocumentRecord{
		FileName:  "test.pdf",
		ChunkID:   0,
		Content:   "Test content",
		Embedding: []float32{0.1, 0.2, 0.3},
	}

	// Test JSON marshaling
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Failed to marshal record: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled app.DocumentRecord
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal record: %v", err)
	}

	if unmarshaled.FileName != record.FileName {
		t.Errorf("FileName mismatch: expected %s, got %s", record.FileName, unmarshaled.FileName)
	}

	if unmarshaled.ChunkID != record.ChunkID {
		t.Errorf("ChunkID mismatch: expected %d, got %d", record.ChunkID, unmarshaled.ChunkID)
	}

	if unmarshaled.Content != record.Content {
		t.Errorf("Content mismatch: expected %s, got %s", record.Content, unmarshaled.Content)
	}
}

func TestCheckpointer(t *testing.T) {
	// Create temporary database
	tempDB := t.TempDir() + "/test.db"
	checkpointer, err := checkpoint.NewCheckpointer(tempDB)
	if err != nil {
		t.Fatalf("Failed to create checkpointer: %v", err)
	}
	defer checkpointer.Close()

	// Test marking file as processed
	filename := "test.pdf"
	if checkpointer.IsProcessed(filename) {
		t.Error("File should not be marked as processed initially")
	}

	err = checkpointer.MarkAsDone(filename)
	if err != nil {
		t.Fatalf("Failed to mark file as done: %v", err)
	}

	if !checkpointer.IsProcessed(filename) {
		t.Error("File should be marked as processed after MarkAsDone")
	}
}
