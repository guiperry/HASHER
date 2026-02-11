package storage

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
	SourceFile string `json:"SourceFile"`
	ChunkID    int32  `json:"ChunkID"`

	// Window metadata
	WindowStart   int32 `json:"WindowStart"`
	WindowEnd     int32 `json:"WindowEnd"`
	ContextLength int32 `json:"ContextLength"`

	// ASIC input slots (12 x 4 bytes = 48 bytes)
	AsicSlots0  int32 `json:"AsicSlots0"`
	AsicSlots1  int32 `json:"AsicSlots1"`
	AsicSlots2  int32 `json:"AsicSlots2"`
	AsicSlots3  int32 `json:"AsicSlots3"`
	AsicSlots4  int32 `json:"AsicSlots4"`
	AsicSlots5  int32 `json:"AsicSlots5"`
	AsicSlots6  int32 `json:"AsicSlots6"`
	AsicSlots7  int32 `json:"AsicSlots7"`
	AsicSlots8  int32 `json:"AsicSlots8"`
	AsicSlots9  int32 `json:"AsicSlots9"`
	AsicSlots10 int32 `json:"AsicSlots10"`
	AsicSlots11 int32 `json:"AsicSlots11"`

	// Target
	TargetTokenID int32 `json:"TargetTokenID"`

	// Seed (placeholder for Stage 3)
	BestSeed []byte `json:"BestSeed,omitempty"`
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
