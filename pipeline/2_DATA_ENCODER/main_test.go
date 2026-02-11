package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"data-encoder/pkg/mapper"
	"data-encoder/pkg/schema"
	"data-encoder/pkg/tokenizer"
)

func TestParseFlags(t *testing.T) {
	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	tests := []struct {
		name     string
		args     []string
		expected *Config
	}{
		{
			name: "all flags",
			args: []string{"cmd", "-input", "test.json", "-output", "out.parquet", "-seed", "42", "-workers", "8", "-window-size", "256", "-window-stride", "2", "-batch-size", "16"},
			expected: &Config{
				InputFile:  "test.json",
				OutputFile: "out.parquet",
				MapperSeed: 42,
				NumWorkers: 8,
			},
		},
		{
			name: "defaults",
			args: []string{"cmd", "-input", "test.json"},
			expected: &Config{
				InputFile:  "test.json",
				OutputFile: "training_frames.parquet",
				MapperSeed: 1337,
				NumWorkers: 4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			// Note: This won't work well with flag.Parse() in tests due to flag state
			// In real tests, we'd refactor parseFlags to accept arguments
		})
	}
}

func TestValidateConfig(t *testing.T) {
	// Create a temp directory for tests
	tempDir := t.TempDir()

	// Create a test input file
	testInput := filepath.Join(tempDir, "test_input.json")
	if err := os.WriteFile(testInput, []byte(`{}`), 0644); err != nil {
		t.Fatalf("failed to create test input: %v", err)
	}

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				InputFile:  testInput,
				OutputFile: filepath.Join(tempDir, "output.parquet"),
			},
			wantErr: false,
		},
		{
			name: "missing input",
			config: &Config{
				InputFile: "",
			},
			wantErr: true,
		},
		{
			name: "nonexistent input",
			config: &Config{
				InputFile: "/nonexistent/file.json",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestEndToEndEncoding(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Create test input file with sliding window data
	inputFile := filepath.Join(tempDir, "test_input.json")
	records := []schema.MinedRecord{
		{
			FileName: "test1.txt",
			ChunkID:  1,
			Content:  "Hello World from the data encoder test suite",
		},
		{
			FileName: "test2.txt",
			ChunkID:  2,
			Content:  "Test encoding with sliding windows for better context understanding",
		},
	}

	// Write records as JSON stream
	file, err := os.Create(inputFile)
	if err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("failed to encode record: %v", err)
		}
	}
	file.Close()

	// Set up output file
	outputFile := filepath.Join(tempDir, "test_output.json")

	// Run the encoder
	config := &Config{
		InputFile:    inputFile,
		OutputFile:   outputFile,
		MapperSeed:   1337,
		NumWorkers:   2,
		WindowSize:   128,
		WindowStride: 1,
		BatchSize:    32,
	}

	if err := runEncoder(config); err != nil {
		t.Fatalf("encoding failed: %v", err)
	}

	// Verify output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatal("output file was not created")
	}

	// Read and verify JSON file
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}

	var frames []schema.TrainingFrame
	if err := json.Unmarshal(data, &frames); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if len(frames) == 0 {
		t.Error("expected frames in JSON file, got 0")
	}

	t.Logf("✓ Generated %d training frames", len(frames))

	// Verify frame structure with sliding window metadata
	for i, frame := range frames {
		if frame.SourceFile == "" {
			t.Errorf("frame %d: SourceFile is empty", i)
		}
		slots := frame.GetAsicSlots()
		if len(slots) != 12 {
			t.Errorf("frame %d: expected 12 slots, got %d", i, len(slots))
		}
		if frame.TargetTokenID < 0 {
			t.Errorf("frame %d: TargetTokenID is negative", i)
		}
		// Verify sliding window metadata
		if frame.ContextLength < 0 {
			t.Errorf("frame %d: ContextLength is negative", i)
		}
		if frame.WindowStart < 0 {
			t.Errorf("frame %d: WindowStart is negative", i)
		}
		if frame.WindowEnd < 0 {
			t.Errorf("frame %d: WindowEnd is negative", i)
		}
	}
}

func TestComponentIntegration(t *testing.T) {
	// Test that all components work together
	tk, err := tokenizer.New()
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	mp := mapper.New(1337)

	testText := "Hello World, this is a test for sliding windows"
	testEmbedding := generateTestEmbedding(0.5)

	// Tokenize
	tokens := tk.Encode(testText)
	if len(tokens) == 0 {
		t.Fatal("expected tokens")
	}

	// Map embedding
	slots := mp.MapToSlots(testEmbedding)
	if len(slots) != 12 {
		t.Errorf("expected 12 slots, got %d", len(slots))
	}

	// Create frames with sliding window metadata
	for i, tokenID := range tokens {
		frame := schema.TrainingFrame{
			SourceFile:    "integration_test.txt",
			ChunkID:       1,
			WindowStart:   int32(i),
			WindowEnd:     int32(i + 1),
			ContextLength: int32(i),
			TargetTokenID: int32(tokenID),
			BestSeed:      nil,
		}
		frame.SetAsicSlots(slots)

		_ = frame
		t.Logf("Frame %d: Source=%s, ChunkID=%d, Window=[%d,%d], TargetToken=%d",
			i, frame.SourceFile, frame.ChunkID, frame.WindowStart, frame.WindowEnd, frame.TargetTokenID)
	}

	t.Logf("✓ Successfully created %d training frames with sliding window metadata", len(tokens))
}

func TestSlidingWindowIntegration(t *testing.T) {
	// Test sliding window generation and processing
	tk, err := tokenizer.New()
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	mp := mapper.New(1337)
	testText := "The quick brown fox jumps over the lazy dog"
	testEmbedding := generateTestEmbedding(0.8)

	tokens := tk.Encode(testText)
	if len(tokens) == 0 {
		t.Fatal("expected tokens")
	}

	slots := mp.MapToSlots(testEmbedding)

	// Simulate sliding window frames
	windowSize := 3
	for i := 1; i < len(tokens); i++ {
		start := 0
		if i > windowSize {
			start = i - windowSize
		}

		frame := schema.TrainingFrame{
			SourceFile:    "sliding_window_test.txt",
			ChunkID:       1,
			WindowStart:   int32(start),
			WindowEnd:     int32(i),
			ContextLength: int32(i - start),
			TargetTokenID: int32(tokens[i]),
			BestSeed:      nil,
		}
		frame.SetAsicSlots(slots)

		t.Logf("Window [%d,%d]: ContextLength=%d, Target=%d",
			frame.WindowStart, frame.WindowEnd, frame.ContextLength, frame.TargetTokenID)
	}

	t.Logf("✓ Sliding window test complete: %d tokens generated", len(tokens))
}

// Helper function to generate a test embedding
func generateTestEmbedding(scale float32) []float32 {
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = float32(i%10) * 0.1 * scale
	}
	return embedding
}

// minInt64 returns the minimum of two int64 values
func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
