package storage

import (
	"encoding/json"
)

type WeightRecord struct {
	TokenID      int32   `json:"token_id"`
	BestSeed     []byte  `json:"best_seed"`
	FitnessScore float64 `json:"fitness_score"`
	Generation   int32   `json:"generation"`
	ContextKey   uint32  `json:"context_key"`
}

// JSONTrainingRecord represents the structure in the JSON file
// Using PascalCase to match the existing training_frames.json format
type JSONTrainingRecord struct {
	// Metadata
	SourceFile string
	ChunkID    int32
	WindowStart int32
	WindowEnd   int32
	ContextLength int32

	// ASIC input slots (12 x 4 bytes = 48 bytes)
	AsicSlots0  int32
	AsicSlots1  int32
	AsicSlots2  int32
	AsicSlots3  int32
	AsicSlots4  int32
	AsicSlots5  int32
	AsicSlots6  int32
	AsicSlots7  int32
	AsicSlots8  int32
	AsicSlots9  int32
	AsicSlots10 int32
	AsicSlots11 int32

	// Target
	TargetTokenID int32

	// Seed (placeholder for Stage 3)
	BestSeed []byte
}

func (jtr *JSONTrainingRecord) UnmarshalJSON(data []byte) error {
	var aux map[string]interface{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Helper to get string
	getString := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := aux[k]; ok {
				if s, ok := v.(string); ok {
					return s
				}
			}
		}
		return ""
	}

	// Helper to get int32
	getInt32 := func(keys ...string) int32 {
		for _, k := range keys {
			if v, ok := aux[k]; ok {
				switch val := v.(type) {
				case float64:
					return int32(val)
				case int:
					return int32(val)
				}
			}
		}
		return 0
	}

	// Helper to get byte slice (string base64 or raw string)
	getBytes := func(keys ...string) []byte {
		for _, k := range keys {
			if v, ok := aux[k]; ok {
				if s, ok := v.(string); ok {
					return []byte(s)
				}
			}
		}
		return nil
	}

	jtr.SourceFile = getString("source_file", "SourceFile")
	jtr.ChunkID = getInt32("chunk_id", "ChunkID")
	jtr.WindowStart = getInt32("window_start", "WindowStart")
	jtr.WindowEnd = getInt32("window_end", "WindowEnd")
	jtr.ContextLength = getInt32("context_length", "ContextLength")

	jtr.AsicSlots0 = getInt32("asic_slot_0", "AsicSlots0")
	jtr.AsicSlots1 = getInt32("asic_slot_1", "AsicSlots1")
	jtr.AsicSlots2 = getInt32("asic_slot_2", "AsicSlots2")
	jtr.AsicSlots3 = getInt32("asic_slot_3", "AsicSlots3")
	jtr.AsicSlots4 = getInt32("asic_slot_4", "AsicSlots4")
	jtr.AsicSlots5 = getInt32("asic_slot_5", "AsicSlots5")
	jtr.AsicSlots6 = getInt32("asic_slot_6", "AsicSlots6")
	jtr.AsicSlots7 = getInt32("asic_slot_7", "AsicSlots7")
	jtr.AsicSlots8 = getInt32("asic_slot_8", "AsicSlots8")
	jtr.AsicSlots9 = getInt32("asic_slot_9", "AsicSlots9")
	jtr.AsicSlots10 = getInt32("asic_slot_10", "AsicSlots10")
	jtr.AsicSlots11 = getInt32("asic_slot_11", "AsicSlots11")

	jtr.TargetTokenID = getInt32("target_token_id", "TargetTokenID")
	jtr.BestSeed = getBytes("best_seed", "BestSeed")

	return nil
}

// Helper methods to convert between different formats
func (jtr *JSONTrainingRecord) GetTargetToken() int32 {
	return jtr.TargetTokenID
}

func (jtr *JSONTrainingRecord) SetTargetToken(token int32) {
	jtr.TargetTokenID = token
}

func (jtr *JSONTrainingRecord) GetAsicSlots() [12]int32 {
	return [12]int32{
		jtr.AsicSlots0, jtr.AsicSlots1, jtr.AsicSlots2, jtr.AsicSlots3,
		jtr.AsicSlots4, jtr.AsicSlots5, jtr.AsicSlots6, jtr.AsicSlots7,
		jtr.AsicSlots8, jtr.AsicSlots9, jtr.AsicSlots10, jtr.AsicSlots11,
	}
}

func (jtr *JSONTrainingRecord) SetAsicSlots(slots [12]int32) {
	jtr.AsicSlots0, jtr.AsicSlots1, jtr.AsicSlots2, jtr.AsicSlots3 = slots[0], slots[1], slots[2], slots[3]
	jtr.AsicSlots4, jtr.AsicSlots5, jtr.AsicSlots6, jtr.AsicSlots7 = slots[4], slots[5], slots[6], slots[7]
	jtr.AsicSlots8, jtr.AsicSlots9, jtr.AsicSlots10, jtr.AsicSlots11 = slots[8], slots[9], slots[10], slots[11]
}
