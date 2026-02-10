package jitter

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/bits"
)

// JitterEngine executes the 21-pass temporal loop for dynamic associative hashing
type JitterEngine struct {
	// Flash searcher for jitter lookups
	searcher *FlashSearcher

	// Configuration
	config *JitterConfig

	// Hash method for performing SHA-256 operations
	hashMethod HashMethod
}

// HashMethod defines the interface for performing hash operations
type HashMethod interface {
	// ComputeHash computes a single SHA-256 hash
	ComputeHash(data []byte) ([32]byte, error)

	// ComputeDoubleHash computes a double SHA-256 hash
	ComputeDoubleHash(data []byte) ([32]byte, error)
}

// SoftwareHashMethod implements HashMethod using pure Go
type SoftwareHashMethod struct{}

// ComputeHash computes SHA-256 using crypto/sha256
func (s *SoftwareHashMethod) ComputeHash(data []byte) ([32]byte, error) {
	return sha256.Sum256(data), nil
}

// ComputeDoubleHash computes double SHA-256
func (s *SoftwareHashMethod) ComputeDoubleHash(data []byte) ([32]byte, error) {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second, nil
}

// NewJitterEngine creates a new jitter engine with the given configuration
func NewJitterEngine(config *JitterConfig) *JitterEngine {
	if config == nil {
		config = DefaultJitterConfig()
	}

	return &JitterEngine{
		searcher:   NewFlashSearcher(config),
		config:     config,
		hashMethod: &SoftwareHashMethod{},
	}
}

// NewJitterEngineWithSearcher creates a jitter engine with a pre-configured searcher
func NewJitterEngineWithSearcher(searcher *FlashSearcher, config *JitterConfig) *JitterEngine {
	if config == nil {
		config = DefaultJitterConfig()
	}

	return &JitterEngine{
		searcher:   searcher,
		config:     config,
		hashMethod: &SoftwareHashMethod{},
	}
}

// SetHashMethod sets a custom hash method (e.g., for CUDA or ASIC acceleration)
func (je *JitterEngine) SetHashMethod(method HashMethod) {
	je.hashMethod = method
}

// GetSearcher returns the flash searcher for external use
func (je *JitterEngine) GetSearcher() *FlashSearcher {
	return je.searcher
}

// Execute21PassLoop runs the complete 21-pass temporal loop
// This is the core jitter mechanism that creates dynamic associative hashes
func (je *JitterEngine) Execute21PassLoop(header []byte, targetTokenID uint32) (*GoldenNonceResult, error) {
	if len(header) != 80 {
		return nil, fmt.Errorf("invalid header length: expected 80, got %d", len(header))
	}

	state := NewHashState(header, targetTokenID)
	passResults := make([]TemporalPassResult, 0, je.config.PassCount)
	appliedJitters := make([]JitterVector, 0, je.config.PassCount)

	// Execute each pass
	for pass := 0; pass < je.config.PassCount; pass++ {
		state.Pass = pass

		// Step 1: Hash the current state
		hash, err := je.hashMethod.ComputeDoubleHash(state.Header)
		if err != nil {
			return nil, fmt.Errorf("hash computation failed at pass %d: %w", pass, err)
		}

		// Step 2: Extract lookup key from hash
		lookupKey := ExtractLookupKey(hash)

		// Step 3: Flash search for jitter vector
		var jitter JitterVector
		var found bool

		if je.config.EnableFlashSearch {
			jitter, found = je.searcher.Search(lookupKey)
			if !found {
				jitter = je.searcher.GenerateDefaultJitter(lookupKey)
			}
		} else {
			// Generate deterministic jitter without database
			jitter = je.searcher.GenerateDefaultJitter(lookupKey)
			found = false
		}

		// Step 4: Apply jitter to header (XOR into MerkleRoot)
		err = XORJitterIntoHeader(state.Header, jitter)
		if err != nil {
			return nil, fmt.Errorf("jitter application failed at pass %d: %w", pass, err)
		}

		// Record the result
		passResults = append(passResults, TemporalPassResult{
			Hash:          hash,
			AppliedJitter: jitter,
			JitterFound:   found,
			Pass:          pass,
		})

		appliedJitters = append(appliedJitters, jitter)
		state.JitterHistory = appliedJitters

		if je.config.Verbose {
			fmt.Printf("[Pass %2d] Hash: %08x, Jitter: %08x (found: %v)\n",
				pass, lookupKey, jitter, found)
		}
	}

	// Compute final hash after all passes
	finalHash, err := je.hashMethod.ComputeDoubleHash(state.Header)
	if err != nil {
		return nil, fmt.Errorf("final hash computation failed: %w", err)
	}

	// Extract golden nonce from final hash
	goldenNonce := binary.BigEndian.Uint32(finalHash[:4])

	// Calculate metrics
	stability := ComputeStability(appliedJitters)
	alignment := ComputeAlignment(finalHash, targetTokenID)

	result := &GoldenNonceResult{
		Nonce:           goldenNonce,
		Found:           alignment >= 0.95, // Consider found if alignment is high
		FinalHash:       finalHash,
		PassesCompleted: je.config.PassCount,
		Stability:       stability,
		Alignment:       alignment,
		JitterVectors:   appliedJitters,
		Metadata: map[string]interface{}{
			"target_token_id": targetTokenID,
			"pass_results":    passResults,
		},
	}

	return result, nil
}

// Execute21PassLoopBatch processes multiple headers in batch
// Optimized for GPU acceleration when available
func (je *JitterEngine) Execute21PassLoopBatch(headers [][]byte, targetTokenID uint32) ([]*GoldenNonceResult, error) {
	results := make([]*GoldenNonceResult, len(headers))

	for i, header := range headers {
		result, err := je.Execute21PassLoop(header, targetTokenID)
		if err != nil {
			return nil, fmt.Errorf("batch processing failed at index %d: %w", i, err)
		}
		results[i] = result
	}

	return results, nil
}

// HuntGoldenNonce searches for the golden nonce using evolutionary optimization
// This implements the GRPO-style search described in the specification
func (je *JitterEngine) HuntGoldenNonce(
	baseHeader []byte,
	targetTokenID uint32,
	candidateNonces []uint32,
) (*GoldenNonceResult, error) {
	if len(baseHeader) != 80 {
		return nil, fmt.Errorf("invalid base header length: expected 80, got %d", len(baseHeader))
	}

	var bestResult *GoldenNonceResult
	bestAlignment := 0.0

	// Evaluate each candidate nonce
	for _, nonce := range candidateNonces {
		// Create header with this nonce
		header := make([]byte, 80)
		copy(header, baseHeader)
		binary.LittleEndian.PutUint32(header[76:80], nonce)

		// Execute 21-pass loop
		result, err := je.Execute21PassLoop(header, targetTokenID)
		if err != nil {
			continue // Skip failed evaluations
		}

		// Track best result
		if result.Alignment > bestAlignment {
			bestAlignment = result.Alignment
			bestResult = result
			bestResult.Nonce = nonce

			// Early exit if we found a perfect match
			if result.Alignment >= 1.0 {
				break
			}
		}
	}

	if bestResult == nil {
		return nil, fmt.Errorf("no valid result found")
	}

	return bestResult, nil
}

// HuntGoldenNonceBatch searches for golden nonces across multiple candidate populations
// Returns the best nonce for each training frame
func (je *JitterEngine) HuntGoldenNonceBatch(
	frames []TrainingFrame,
	getCandidates func(frame *TrainingFrame) []uint32,
) (map[int]*GoldenNonceResult, error) {
	results := make(map[int]*GoldenNonceResult)

	for i, frame := range frames {
		// Generate candidate nonces
		candidates := getCandidates(&frame)

		// Convert frame to header
		header := frame.ToBitcoinHeader()

		// Hunt for golden nonce
		result, err := je.HuntGoldenNonce(header, uint32(frame.TargetTokenID), candidates)
		if err != nil {
			continue // Skip failed frames
		}

		results[i] = result
		frame.BestSeed = result.Nonce
	}

	return results, nil
}

// ComputePassReward calculates the reward for a single pass
// Used for fine-grained reward shaping during training
func (je *JitterEngine) ComputePassReward(
	passResult *TemporalPassResult,
	targetTokenID uint32,
) float64 {
	// Extract hash output
	hashOutput := ExtractLookupKey(passResult.Hash)

	// Calculate bit-wise similarity to target
	xor := hashOutput ^ targetTokenID
	matchingBits := bits.LeadingZeros32(xor)

	// Base reward from bit matching
	reward := float64(matchingBits) / 32.0

	// Bonus if jitter was found in database (associative memory hit)
	if passResult.JitterFound {
		reward += 0.1
	}

	return reward
}

// GetConfig returns the current jitter configuration
func (je *JitterEngine) GetConfig() *JitterConfig {
	return je.config
}

// UpdateConfig updates the jitter configuration
func (je *JitterEngine) UpdateConfig(config *JitterConfig) {
	je.config = config
	je.searcher.config = config
}

// Reset clears the jitter history and resets the engine state
func (je *JitterEngine) Reset() {
	je.searcher.ResetStats()
}

// GetStatistics returns comprehensive statistics about the jitter engine
func (je *JitterEngine) GetStatistics() map[string]interface{} {
	searchStats := je.searcher.GetStats()

	return map[string]interface{}{
		"pass_count":        je.config.PassCount,
		"flash_search":      je.config.EnableFlashSearch,
		"jitter_table_size": je.searcher.Size(),
		"search_hits":       searchStats.Hits,
		"search_misses":     searchStats.Misses,
		"cache_hits":        searchStats.CacheHits,
		"cache_misses":      searchStats.CacheMisses,
	}
}
