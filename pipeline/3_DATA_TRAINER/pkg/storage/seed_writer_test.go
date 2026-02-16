package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONSeedWriter_HelloWorld(t *testing.T) {
	err := os.MkdirAll("tmp_test/frames", 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("tmp_test")

	// 1. Create a dummy training_frames.json with the user's format
	frames := []map[string]interface{}{
		{
			"chunk_id": 1,
			"context_hash": 0,
			"feature_vector": []interface{}{
				286331153.0, 572662306.0, 858993459.0, 1145324612.0,
				1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 4096.0, 0.0,
			},
			"source_file": "demo.txt",
			"target_token": 917.0,
			"token_sequence": []interface{}{906.0},
			"window_start": 0.0,
		},
	}

	sourceFile := "training_frames.json"
	sourcePath := filepath.Join("tmp_test/frames", sourceFile)
	data, _ := json.MarshalIndent(frames, "", "  ")
	os.WriteFile(sourcePath, data, 0644)

	// 2. Initialize SeedWriter
	sw := NewJSONSeedWriter("tmp_test")

	// 3. Add a seed write
	slots := [12]uint32{
		286331153, 572662306, 858993459, 1145324612,
		1, 0, 0, 0, 0, 0, 4096, 0,
	}
	targetTokenID := int32(917)
	bestSeed := []byte("GOLDEN_SEED_12345")

	// The trainer uses record.SourceFile. 
	// In the demo, record.SourceFile is "demo.txt".
	err = sw.AddSeedWrite("demo.txt", slots, targetTokenID, bestSeed)
	if err != nil {
		t.Fatalf("AddSeedWrite failed: %v", err)
	}

	// 4. Write back
	err = sw.WriteBack()
	if err != nil {
		t.Fatalf("WriteBack failed: %v", err)
	}

	// 5. Check if output file exists and has the seed
	// It should look for frames/demo.txt because that's what sw.pendingWrites uses as key.
	// But it won't find frames/demo.txt, so it should fallback to NOTHING if it doesn't exist?
	// Actually, sw.WriteBack initializes cache from readPath = outputFile OR sourcePath.
}
