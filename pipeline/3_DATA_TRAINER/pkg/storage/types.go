package storage

type WeightRecord struct {
	TokenID      int32   `json:"token_id"`
	BestSeed     string  `json:"best_seed"`
	FitnessScore float64 `json:"fitness_score"`
	Generation   int32   `json:"generation"`
	ContextKey   uint32  `json:"context_key"`
}

// JSONTrainingRecord represents the structure in the JSON file
type JSONTrainingRecord struct {
	// Metadata
	SourceFile string `json:"source_file"`
	ChunkID    int32  `json:"chunk_id"`

	// Window metadata
	WindowStart   int32 `json:"window_start"`
	WindowEnd     int32 `json:"window_end"`
	ContextLength int32 `json:"context_length"`

	// ASIC input slots (12 x 4 bytes = 48 bytes)
	AsicSlots0  int32 `json:"asic_slot_0"`
	AsicSlots1  int32 `json:"asic_slot_1"`
	AsicSlots2  int32 `json:"asic_slot_2"`
	AsicSlots3  int32 `json:"asic_slot_3"`
	AsicSlots4  int32 `json:"asic_slot_4"`
	AsicSlots5  int32 `json:"asic_slot_5"`
	AsicSlots6  int32 `json:"asic_slot_6"`
	AsicSlots7  int32 `json:"asic_slot_7"`
	AsicSlots8  int32 `json:"asic_slot_8"`
	AsicSlots9  int32 `json:"asic_slot_9"`
	AsicSlots10 int32 `json:"asic_slot_10"`
	AsicSlots11 int32 `json:"asic_slot_11"`

	// Target
	TargetTokenID int32 `json:"target_token_id"`

	// Seed (placeholder for Stage 3)
	BestSeed string `json:"best_seed"`
}
