package schema

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMinedRecordJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected MinedRecord
		wantErr  bool
	}{
		{
			name: "valid record",
			json: `{"file_name":"test.txt","chunk_id":1,"content":"Hello World"}`,
			expected: MinedRecord{
				FileName: "test.txt",
				ChunkID:  1,
				Content:  "Hello World",
			},
			wantErr: false,
		},
		{
			name: "valid record with embedding (backward compatibility)",
			// Embedding field is now ignored for backward compatibility
			json: `{"file_name":"test.txt","chunk_id":1,"content":"Hello World","embedding":[0.1,0.2,0.3]}`,
			expected: MinedRecord{
				FileName: "test.txt",
				ChunkID:  1,
				Content:  "Hello World",
				// Embedding field is not populated - generated on-demand via sliding windows
			},
			wantErr: false,
		},
		{
			name:     "invalid json",
			json:     `{"file_name":}`,
			expected: MinedRecord{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var record MinedRecord
			err := json.Unmarshal([]byte(tt.json), &record)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if record.FileName != tt.expected.FileName {
				t.Errorf("FileName = %v, want %v", record.FileName, tt.expected.FileName)
			}
			if record.ChunkID != tt.expected.ChunkID {
				t.Errorf("ChunkID = %v, want %v", record.ChunkID, tt.expected.ChunkID)
			}
			if record.Content != tt.expected.Content {
				t.Errorf("Content = %v, want %v", record.Content, tt.expected.Content)
			}
		})
	}
}

func TestMinedRecordRoundTrip(t *testing.T) {
	record := MinedRecord{
		FileName: "test.txt",
		ChunkID:  42,
		Content:  "Test text for sliding window processing",
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded MinedRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.FileName != record.FileName {
		t.Errorf("FileName mismatch: got %v, want %v", decoded.FileName, record.FileName)
	}
	if decoded.ChunkID != record.ChunkID {
		t.Errorf("ChunkID mismatch: got %v, want %v", decoded.ChunkID, record.ChunkID)
	}
	if decoded.Content != record.Content {
		t.Errorf("Content mismatch: got %v, want %v", decoded.Content, record.Content)
	}
}

func TestTrainingFrameStructure(t *testing.T) {
	frame := TrainingFrame{
		SourceFile:    "test.go",
		ChunkID:       1,
		WindowStart:   0,
		WindowEnd:     10,
		ContextLength: 10,
		TargetTokenID: 42,
		BestSeed:      nil,
	}
	frame.SetAsicSlots([12]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})

	// Verify the frame structure
	slots := frame.GetAsicSlots()
	if len(slots) != 12 {
		t.Errorf("expected 12 ASIC slots, got %d", len(slots))
	}

	if frame.SourceFile != "test.go" {
		t.Errorf("expected SourceFile to be 'test.go', got %s", frame.SourceFile)
	}

	if frame.ChunkID != 1 {
		t.Errorf("expected ChunkID to be 1, got %d", frame.ChunkID)
	}

	if frame.WindowStart != 0 {
		t.Errorf("expected WindowStart to be 0, got %d", frame.WindowStart)
	}

	if frame.WindowEnd != 10 {
		t.Errorf("expected WindowEnd to be 10, got %d", frame.WindowEnd)
	}

	if frame.ContextLength != 10 {
		t.Errorf("expected ContextLength to be 10, got %d", frame.ContextLength)
	}

	if frame.TargetTokenID != 42 {
		t.Errorf("expected TargetTokenID to be 42, got %d", frame.TargetTokenID)
	}

	// Verify all slots are set correctly
	for i, slot := range slots {
		expected := uint32(i + 1)
		if slot != expected {
			t.Errorf("slot %d: expected %d, got %d", i, expected, slot)
		}
	}
}

func TestTrainingFrameWithSeed(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	frame := TrainingFrame{
		SourceFile:    "seeded.txt",
		ChunkID:       99,
		WindowStart:   50,
		WindowEnd:     128,
		ContextLength: 78,
		TargetTokenID: 12345,
		BestSeed:      seed,
	}
	frame.SetAsicSlots([12]uint32{})

	if len(frame.BestSeed) != 32 {
		t.Errorf("expected BestSeed to be 32 bytes, got %d", len(frame.BestSeed))
	}

	for i, v := range frame.BestSeed {
		if byte(v) != byte(i) {
			t.Errorf("expected BestSeed[%d] to be %d, got %d", i, i, v)
		}
	}

	if frame.WindowStart != 50 {
		t.Errorf("expected WindowStart to be 50, got %d", frame.WindowStart)
	}

	if frame.WindowEnd != 128 {
		t.Errorf("expected WindowEnd to be 128, got %d", frame.WindowEnd)
	}
}

func TestTrainingFrameASICSslots(t *testing.T) {
	// Test setting and getting ASIC slots
	slots := [12]uint32{
		0x12345678, 0x9ABCDEF0, 0x11223344, 0x55667788,
		0x99AABBCC, 0xDDEEFF00, 0x11223344, 0x55667788,
		0x99AABBCC, 0xDDEEFF00, 0x12345678, 0x9ABCDEF0,
	}

	frame := TrainingFrame{}
	frame.SetAsicSlots(slots)

	retrievedSlots := frame.GetAsicSlots()

	for i, slot := range retrievedSlots {
		if slot != slots[i] {
			t.Errorf("ASIC slot %d mismatch: got 0x%08X, want 0x%08X", i, slot, slots[i])
		}
	}
}

func TestTrainingFrameRoundTrip(t *testing.T) {
	// Test that we can set and retrieve all fields correctly
	frame := TrainingFrame{
		SourceFile:    "round_trip_test.txt",
		ChunkID:       123,
		WindowStart:   0,
		WindowEnd:     64,
		ContextLength: 64,
		TargetTokenID: 45678,
		BestSeed:      []byte("test_seed_data"),
	}
	frame.SetAsicSlots([12]uint32{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
	})

	// Verify source file
	if frame.SourceFile != "round_trip_test.txt" {
		t.Errorf("SourceFile mismatch: got %s, want %s", frame.SourceFile, "round_trip_test.txt")
	}

	// Verify chunk ID
	if frame.ChunkID != 123 {
		t.Errorf("ChunkID mismatch: got %d, want %d", frame.ChunkID, 123)
	}

	// Verify window metadata
	if frame.WindowStart != 0 {
		t.Errorf("WindowStart mismatch: got %d, want %d", frame.WindowStart, 0)
	}

	if frame.WindowEnd != 64 {
		t.Errorf("WindowEnd mismatch: got %d, want %d", frame.WindowEnd, 64)
	}

	if frame.ContextLength != 64 {
		t.Errorf("ContextLength mismatch: got %d, want %d", frame.ContextLength, 64)
	}

	// Verify target token
	if frame.TargetTokenID != 45678 {
		t.Errorf("TargetTokenID mismatch: got %d, want %d", frame.TargetTokenID, 45678)
	}

	// Verify seed
	if !bytes.Equal(frame.BestSeed, []byte("test_seed_data")) {
		t.Errorf("BestSeed mismatch: got %s, want %s", frame.BestSeed, "test_seed_data")
	}

	// Verify ASIC slots
	slots := frame.GetAsicSlots()
	for i, slot := range slots {
		expected := uint32(i + 1)
		if slot != expected {
			t.Errorf("ASIC slot %d mismatch: got %d, want %d", i, slot, expected)
		}
	}
}
