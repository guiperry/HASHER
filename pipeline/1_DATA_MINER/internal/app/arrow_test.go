package app

import (
	"os"
	"testing"
	"reflect"
)

func TestArrowNLPSerialization(t *testing.T) {
	tempFile := "test_nlp_metadata.arrow"
	defer os.Remove(tempFile)

	originalRecords := []DocumentRecord{
		{
			FileName:     "test.pdf",
			ChunkID:      1,
			Content:      "Linguistics test.",
			Embedding:    []float32{0.1, 0.2, 0.3},
			Tokens:       []string{"Linguistics", "test", "."},
			TokenOffsets: []int32{0, 12, 16},
			POSTags:      []uint8{0x01, 0x01, 0x0F},
			Tenses:       []uint8{0, 0, 0},
			DepHashes:    []uint32{123, 456, 789},
		},
	}

	// Write to Arrow
	err := WriteDocumentRecordsToArrowIPC(tempFile, originalRecords)
	if err != nil {
		t.Fatalf("Failed to write Arrow IPC: %v", err)
	}

	// Read back from Arrow
	readRecords, err := ReadDocumentRecordsFromArrowIPC(tempFile)
	if err != nil {
		t.Fatalf("Failed to read Arrow IPC: %v", err)
	}

	if len(readRecords) != len(originalRecords) {
		t.Fatalf("Record count mismatch: got %d, want %d", len(readRecords), len(originalRecords))
	}

	// Compare fields
	r := readRecords[0]
	o := originalRecords[0]

	if !reflect.DeepEqual(r.Tokens, o.Tokens) {
		t.Errorf("Tokens mismatch: %v vs %v", r.Tokens, o.Tokens)
	}
	if !reflect.DeepEqual(r.TokenOffsets, o.TokenOffsets) {
		t.Errorf("Offsets mismatch: %v vs %v", r.TokenOffsets, o.TokenOffsets)
	}
	if !reflect.DeepEqual(r.POSTags, o.POSTags) {
		t.Errorf("POS tags mismatch: %v vs %v", r.POSTags, o.POSTags)
	}
	if !reflect.DeepEqual(r.DepHashes, o.DepHashes) {
		t.Errorf("Dep hashes mismatch: %v vs %v", r.DepHashes, o.DepHashes)
	}
}
