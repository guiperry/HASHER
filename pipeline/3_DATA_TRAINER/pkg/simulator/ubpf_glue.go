package simulator

import (
	"fmt"

	"hasher/pkg/hashing/jitter"
)

// FlashSearcher is our high-speed associative memory interface
// This now wraps the actual jitter.FlashSearcher from the pkg/hashing/jitter package
type FlashSearcher struct {
	// The actual flash searcher from the jitter package
	searcher *jitter.FlashSearcher
}

var globalSearcher *FlashSearcher

// NewFlashSearcher creates a new flash searcher wrapper
func NewFlashSearcher(config *jitter.JitterConfig) *FlashSearcher {
	return &FlashSearcher{
		searcher: jitter.NewFlashSearcher(config),
	}
}

// LoadJitterFromParquet loads jitter vectors from a parquet file
// This now uses the actual implementation from the jitter package
func LoadJitterFromParquet(filename string) map[uint32]uint32 {
	fmt.Printf("[FlashSearch] Loading jitter table from: %s\n", filename)

	// Use the actual implementation
	jitterTable := jitter.LoadJitterFromParquet(filename)

	// Convert JitterVector to uint32 for compatibility
	table := make(map[uint32]uint32, len(jitterTable))
	for k, v := range jitterTable {
		table[k] = uint32(v)
	}

	return table
}

// Search performs a flash lookup for a jitter vector
func (fs *FlashSearcher) Search(hashKey uint32) (uint32, bool) {
	jitter, found := fs.searcher.Search(hashKey)
	return uint32(jitter), found
}

// LoadJitterTable loads a jitter table into the searcher
func (fs *FlashSearcher) LoadJitterTable(table map[uint32]uint32) {
	// Convert uint32 to JitterVector
	jitterTable := make(map[uint32]jitter.JitterVector, len(table))
	for k, v := range table {
		jitterTable[k] = jitter.JitterVector(v)
	}

	fs.searcher.LoadJitterTable(jitterTable)
}

// GetStats returns search statistics
func (fs *FlashSearcher) GetStats() map[string]interface{} {
	stats := fs.searcher.GetStats()
	return map[string]interface{}{
		"hits":         stats.Hits,
		"misses":       stats.Misses,
		"cache_hits":   stats.CacheHits,
		"cache_misses": stats.CacheMisses,
		"table_size":   fs.searcher.Size(),
	}
}

// TrainingFrame is a local alias for compatibility (matches schema.TrainingFrame)
type TrainingFrame struct {
	// Placeholder for training frame structure
	SourceFile    string
	ChunkID       int32
	WindowStart   int32
	WindowEnd     int32
	ContextLength int32
	AsicSlots0    int32
	AsicSlots1    int32
	AsicSlots2    int32
	AsicSlots3    int32
	AsicSlots4    int32
	AsicSlots5    int32
	AsicSlots6    int32
	AsicSlots7    int32
	AsicSlots8    int32
	AsicSlots9    int32
	AsicSlots10   int32
	AsicSlots11   int32
	TargetTokenID int32
	BestSeed      uint32 // This is what we're trying to optimize
}

// ToBitcoinHeader converts a TrainingFrame to an 80-byte Bitcoin header
func (tf *TrainingFrame) ToBitcoinHeader() []byte {
	// Convert ASIC slots to uint32 array
	slots := [12]uint32{
		uint32(tf.AsicSlots0), uint32(tf.AsicSlots1), uint32(tf.AsicSlots2),
		uint32(tf.AsicSlots3), uint32(tf.AsicSlots4), uint32(tf.AsicSlots5),
		uint32(tf.AsicSlots6), uint32(tf.AsicSlots7), uint32(tf.AsicSlots8),
		uint32(tf.AsicSlots9), uint32(tf.AsicSlots10), uint32(tf.AsicSlots11),
	}

	// Use the jitter package's Bitcoin header construction
	trainingFrame := &jitter.TrainingFrame{
		AsicSlots:     slots,
		TargetTokenID: tf.TargetTokenID,
	}

	return trainingFrame.ToBitcoinHeader()
}

// RunEvolutionaryPass runs the evolutionary training pass with 21-pass jitter
func RunEvolutionaryPass(bytecode []byte, trainingBatch []TrainingFrame) {
	fmt.Printf("[Evolutionary] Running pass with %d training frames\n", len(trainingBatch))

	// 1. Setup the jitter engine
	jitterConfig := jitter.DefaultJitterConfig()
	jitterEngine := jitter.NewJitterEngine(jitterConfig)

	// 2. Load the associative memory (jitter vectors)
	searcher := NewFlashSearcher(jitterConfig)
	jitterTable := LoadJitterFromParquet("weights.parquet")
	searcher.LoadJitterTable(jitterTable)

	// 3. Process each training frame with 21-pass temporal loop
	for i, frame := range trainingBatch {
		fmt.Printf("[Frame %d] Processing with target token: %d\n", i, frame.TargetTokenID)

		// Convert frame to Bitcoin header
		header := frame.ToBitcoinHeader()

		// Execute 21-pass temporal loop
		result, err := jitterEngine.Execute21PassLoop(header, uint32(frame.TargetTokenID))
		if err != nil {
			fmt.Printf("[Frame %d] Error: %v\n", i, err)
			continue
		}

		// Store the best seed found
		frame.BestSeed = result.Nonce

		fmt.Printf("[Frame %d] Golden nonce: %d (alignment: %.3f, stability: %.3f)\n",
			i, result.Nonce, result.Alignment, result.Stability)
	}

	// 4. Report statistics
	stats := searcher.GetStats()
	fmt.Printf("[FlashSearch] Stats - Hits: %d, Misses: %d, Cache hits: %d, Table size: %d\n",
		stats["hits"], stats["misses"], stats["cache_hits"], stats["table_size"])
}
