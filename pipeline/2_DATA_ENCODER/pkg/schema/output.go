package schema

// TrainingFrame represents the output schema for Parquet files
type TrainingFrame struct {
	// 1. Metadata for traceability
	SourceFile string `parquet:"name=source_file, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	ChunkID    int32  `parquet:"name=chunk_id, type=INT32"`

	// 2. Window Metadata (NEW: Sliding window information)
	WindowStart   int32 `parquet:"name=window_start, type=INT32"`   // Start token position
	WindowEnd     int32 `parquet:"name=window_end, type=INT32"`     // End token position (exclusive)
	ContextLength int32 `parquet:"name=context_length, type=INT32"` // Number of context tokens used

	// 3. The Input (What the ASIC sees)
	// 12 slots * 4 bytes = 48 bytes total
	AsicSlots0  int32 `parquet:"name=asic_slot_0, type=INT32"`
	AsicSlots1  int32 `parquet:"name=asic_slot_1, type=INT32"`
	AsicSlots2  int32 `parquet:"name=asic_slot_2, type=INT32"`
	AsicSlots3  int32 `parquet:"name=asic_slot_3, type=INT32"`
	AsicSlots4  int32 `parquet:"name=asic_slot_4, type=INT32"`
	AsicSlots5  int32 `parquet:"name=asic_slot_5, type=INT32"`
	AsicSlots6  int32 `parquet:"name=asic_slot_6, type=INT32"`
	AsicSlots7  int32 `parquet:"name=asic_slot_7, type=INT32"`
	AsicSlots8  int32 `parquet:"name=asic_slot_8, type=INT32"`
	AsicSlots9  int32 `parquet:"name=asic_slot_9, type=INT32"`
	AsicSlots10 int32 `parquet:"name=asic_slot_10, type=INT32"`
	AsicSlots11 int32 `parquet:"name=asic_slot_11, type=INT32"`

	// 4. The Target (What the ASIC must predict)
	// This is the "Golden Nonce" we are hunting for
	TargetTokenID int32 `parquet:"name=target_token_id, type=INT32"`

	// 5. Seed Persistence (Placeholder for Stage 3)
	// The Data Trainer will fill this. 32 bytes = 256 bits (SHA-256)
	// Using string to ensure compatibility with parquet serialization
	BestSeed string `parquet:"name=best_seed, type=BYTE_ARRAY"`
}

// SetAsicSlots sets all 12 ASIC slots from an array
func (tf *TrainingFrame) SetAsicSlots(slots [12]uint32) {
	tf.AsicSlots0 = int32(slots[0])
	tf.AsicSlots1 = int32(slots[1])
	tf.AsicSlots2 = int32(slots[2])
	tf.AsicSlots3 = int32(slots[3])
	tf.AsicSlots4 = int32(slots[4])
	tf.AsicSlots5 = int32(slots[5])
	tf.AsicSlots6 = int32(slots[6])
	tf.AsicSlots7 = int32(slots[7])
	tf.AsicSlots8 = int32(slots[8])
	tf.AsicSlots9 = int32(slots[9])
	tf.AsicSlots10 = int32(slots[10])
	tf.AsicSlots11 = int32(slots[11])
}

// GetAsicSlots returns all 12 ASIC slots as an array
func (tf *TrainingFrame) GetAsicSlots() [12]uint32 {
	return [12]uint32{
		uint32(tf.AsicSlots0), uint32(tf.AsicSlots1), uint32(tf.AsicSlots2), uint32(tf.AsicSlots3),
		uint32(tf.AsicSlots4), uint32(tf.AsicSlots5), uint32(tf.AsicSlots6), uint32(tf.AsicSlots7),
		uint32(tf.AsicSlots8), uint32(tf.AsicSlots9), uint32(tf.AsicSlots10), uint32(tf.AsicSlots11),
	}
}
