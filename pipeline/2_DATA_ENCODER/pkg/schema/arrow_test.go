package schema

import (
	"os"
	"testing"
)

func TestTrainingFrameArrowIO(t *testing.T) {
	tempFile := "test_training_frames.arrow"
	defer os.Remove(tempFile)

	frames := []TrainingFrame{
		{
			SourceFile:    "input.pdf",
			ChunkID:       0,
			WindowStart:   0,
			WindowEnd:     128,
			ContextLength: 128,
			TargetTokenID: 50256,
			AsicSlots0:    100,
			AsicSlots11:   42,
		},
	}

	// Write frames to Arrow IPC
	err := WriteTrainingFramesToArrowIPC(tempFile, frames)
	if err != nil {
		t.Fatalf("Failed to write training frames to Arrow: %v", err)
	}

	// Read frames back
	readFrames, err := ReadTrainingFramesFromArrowIPC(tempFile)
	if err != nil {
		t.Fatalf("Failed to read training frames from Arrow: %v", err)
	}

	if len(readFrames) != 1 {
		t.Fatalf("Expected 1 frame, got %d", len(readFrames))
	}

	if readFrames[0].TargetTokenID != 50256 {
		t.Errorf("TargetTokenID mismatch: got %d, want 50256", readFrames[0].TargetTokenID)
	}
	
	if readFrames[0].AsicSlots11 != 42 {
		t.Errorf("AsicSlot11 mismatch: got %d, want 42", readFrames[0].AsicSlots11)
	}
}
