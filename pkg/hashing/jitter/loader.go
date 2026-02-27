package jitter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadFromJSONFile reads a JSON file containing an array of TrainingFrame records
// and appends them into the FlashSearcher's knowledge base.
// Returns the number of frames loaded.
func LoadFromJSONFile(fs *FlashSearcher, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}

	// Support both a top-level array and a wrapped object {"frames": [...]}
	var frames []TrainingFrame
	if err := json.Unmarshal(data, &frames); err != nil {
		// Try wrapped form
		var wrapped struct {
			Frames []TrainingFrame `json:"frames"`
		}
		if err2 := json.Unmarshal(data, &wrapped); err2 != nil {
			return 0, fmt.Errorf("parse %s: %w", path, err)
		}
		frames = wrapped.Frames
	}

	if len(frames) == 0 {
		return 0, nil
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
