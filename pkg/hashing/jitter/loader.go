package jitter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// rawFrame is an intermediate struct that accepts both the trainer's field names
// (feature_vector, target_token) and the canonical names (asic_slots, target_token_id).
// This bridges the gap between the Data Trainer output format and the inference engine.
type rawFrame struct {
	SourceFile  string  `json:"source_file"`
	ChunkID     int32   `json:"chunk_id"`
	WindowStart int32   `json:"window_start"`
	WindowEnd   int32   `json:"window_end"`

	// Trainer writes "feature_vector"; canonical form is "asic_slots".
	FeatureVector [12]uint32 `json:"feature_vector"`
	AsicSlots     [12]uint32 `json:"asic_slots"`

	TokenSequence []int32 `json:"token_sequence"`

	// Trainer writes "target_token"; canonical form is "target_token_id".
	TargetToken   int32  `json:"target_token"`
	TargetTokenID int32  `json:"target_token_id"`

	BestSeed []byte `json:"best_seed,omitempty"`
}

// toTrainingFrame converts a rawFrame to a TrainingFrame, preferring the
// canonical field names but falling back to the trainer's field names.
func (r rawFrame) toTrainingFrame() TrainingFrame {
	slots := r.AsicSlots
	// If canonical slots are all zero but feature_vector is non-zero, use it.
	allZero := true
	for _, v := range slots {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		slots = r.FeatureVector
	}

	targetID := r.TargetTokenID
	if targetID == 0 && r.TargetToken != 0 {
		targetID = r.TargetToken
	}

	tokenSeq := make([]int, len(r.TokenSequence))
	for i, t := range r.TokenSequence {
		tokenSeq[i] = int(t)
	}

	return TrainingFrame{
		SourceFile:    r.SourceFile,
		ChunkID:       r.ChunkID,
		WindowStart:   r.WindowStart,
		WindowEnd:     r.WindowEnd,
		AsicSlots:     slots,
		TokenSequence: tokenSeq,
		TargetTokenID: targetID,
		BestSeed:      r.BestSeed,
	}
}

// LoadFromJSONFile reads a JSON file containing an array of TrainingFrame records
// and appends them into the FlashSearcher's knowledge base.
// Handles both the canonical field names (asic_slots, target_token_id) and the
// Data Trainer output field names (feature_vector, target_token).
// Returns the number of frames loaded.
func LoadFromJSONFile(fs *FlashSearcher, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}

	// Unmarshal through rawFrame to handle both field name variants.
	var raw []rawFrame
	if err := json.Unmarshal(data, &raw); err != nil {
		// Try wrapped object form: {"frames": [...]}
		var wrapped struct {
			Frames []rawFrame `json:"frames"`
		}
		if err2 := json.Unmarshal(data, &wrapped); err2 != nil {
			return 0, fmt.Errorf("parse %s: %w", path, err)
		}
		raw = wrapped.Frames
	}

	if len(raw) == 0 {
		return 0, nil
	}

	frames := make([]TrainingFrame, len(raw))
	for i, r := range raw {
		frames[i] = r.toTrainingFrame()
	}

	// Merge into the existing knowledge base (rebuild indices over the full set)
	fs.mu.Lock()
	combined := make([]TrainingFrame, len(fs.knowledgeBase)+len(frames))
	copy(combined, fs.knowledgeBase)
	copy(combined[len(fs.knowledgeBase):], frames)
	fs.mu.Unlock()

	fs.BuildFromTrainingData(combined)
	return len(frames), nil
}

// LoadFromDirectory scans dir for JSON files matching the pattern
// "*_with_seeds.json" (trainer output) and "training_frames.json" (demo output),
// loads each into fs, and returns the total number of frames loaded.
//
// Files that cannot be read are logged and skipped; the first fatal error is returned.
func LoadFromDirectory(fs *FlashSearcher, dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read dir %s: %w", dir, err)
	}

	total := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		// Accept trainer output (*_with_seeds.json), demo output (training_frames.json),
		// and any generic training_*.json files.
		if !strings.HasSuffix(name, "_with_seeds.json") &&
			name != "training_frames.json" &&
			!strings.HasPrefix(name, "training_") {
			continue
		}

		fullPath := filepath.Join(dir, name)
		n, err := LoadFromJSONFile(fs, fullPath)
		if err != nil {
			// Log and skip; don't abort the entire load
			fmt.Printf("[loader] warning: skipping %s: %v\n", name, err)
			continue
		}
		if n > 0 {
			fmt.Printf("[loader] loaded %d frames from %s\n", n, name)
			total += n
		}
	}

	return total, nil
}
