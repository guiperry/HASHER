package app

import (
	"testing"
)

func TestMapPOSTag(t *testing.T) {
	tests := []struct {
		pos      string
		expected uint8
	}{
		{"NOUN", 0x01},
		{"VERB", 0x02},
		{"ADJ", 0x03},
		{"PROPN", 0x06},
		{"PUNCT", 0x0F},
		{"UNKNOWN", 0x00},
	}

	for _, tt := range tests {
		got := MapPOSTag(tt.pos)
		if got != tt.expected {
			t.Errorf("MapPOSTag(%s) = 0x%02X; want 0x%02X", tt.pos, got, tt.expected)
		}
	}
}

func TestHashDependency(t *testing.T) {
	dep1 := "nsubj"
	dep2 := "nsubj"
	dep3 := "dobj"

	h1 := HashDependency(dep1)
	h2 := HashDependency(dep2)
	h3 := HashDependency(dep3)

	if h1 != h2 {
		t.Errorf("HashDependency(%s) should be deterministic", dep1)
	}
	if h1 == h3 {
		t.Errorf("HashDependency(%s) and HashDependency(%s) should differ", dep1, dep3)
	}
}

func TestNLPBridge_ProcessText(t *testing.T) {
	// Note: This requires the SpaCy model to be installed in the environment
	bridge, err := NewNLPBridge()
	if err != nil {
		t.Skipf("Skipping SpaCy integration test: %v", err)
	}
	defer bridge.Close()

	text := "The quick brown fox jumps."
	words, offsets, posTags, _, depHashes := bridge.ProcessText(text)

	if len(words) == 0 {
		t.Fatal("Expected tokens, got zero")
	}

	if len(words) != len(offsets) || len(words) != len(posTags) || len(words) != len(depHashes) {
		t.Errorf("Slice length mismatch: words=%d, offsets=%d, pos=%d, dep=%d",
			len(words), len(offsets), len(posTags), len(depHashes))
	}

	// Verify the first word "The"
	if words[0] != "The" {
		t.Errorf("Expected first word 'The', got '%s'", words[0])
	}
	if posTags[0] != int(MapPOSTag("DET")) {
		t.Errorf("Expected DET tag for 'The', got 0x%02X", posTags[0])
	}
}
