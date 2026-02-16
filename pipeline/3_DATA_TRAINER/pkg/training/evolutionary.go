package training

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	mathrand "math/rand"
	"sort"
	"time"

	"github.com/lab/hasher/data-trainer/internal/config"
	"github.com/lab/hasher/data-trainer/pkg/simulator"
	"hasher/pkg/hashing/hardware"
)

const (
	SeedSize          = 32
	FeatureVectorSize = 12
	GroupSize         = 128
	MutationRateBase  = 10
)

type TrainingRecord struct {
	SourceFile    string     `json:"source_file"`
	ChunkID       int32      `json:"chunk_id"`
	WindowStart   int32      `json:"window_start"`
	TokenSequence []int32    `json:"token_sequence"`
	FeatureVector [12]uint32 `json:"feature_vector"`
	TargetToken   int32      `json:"target_token"`
	ContextHash   uint32     `json:"context_hash"`
	BestSeed      []byte     `json:"best_seed,omitempty"`
}

// BitcoinHeader constants for the "Camouflage" strategy
const (
	BitcoinVersion = 0x00000002 // Fixed version for Ghost Headers
	BitcoinBits    = 0x1d00ffff // Standard difficulty
)

// AsicSlots returns the 12 neural slots formatted for Bitcoin header assembly
// Slots 0-7 map to PrevBlockHash (Big-Endian)
// Slots 8-11 map to MerkleRoot (Big-Endian)
func (tr *TrainingRecord) AsicSlots() [12]uint32 {
	return tr.FeatureVector
}

// BuildBitcoinHeader constructs an 80-byte Bitcoin header from neural data
// This is the transient "Camouflage" header generated on-the-fly
func (tr *TrainingRecord) BuildBitcoinHeader(nonce uint32) []byte {
	header := make([]byte, 80)
	slots := tr.AsicSlots()

	// Bytes 0-3: Version (Little-Endian)
	binary.LittleEndian.PutUint32(header[0:4], BitcoinVersion)

	// Bytes 4-36: PrevBlockHash from Slots 0-7 (Big-Endian for SHA-256)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(header[4+(i*4):], slots[i])
	}

	// Bytes 36-68: MerkleRoot from Slots 8-11 + padding (Big-Endian)
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(header[36+(i*4):], slots[i+8])
	}
	// Remaining MerkleRoot bytes (12 bytes) padded with zeros
	for i := 16; i < 20; i++ {
		header[36+i*4] = 0
		header[37+i*4] = 0
		header[38+i*4] = 0
		header[39+i*4] = 0
	}

	// Bytes 68-72: Timestamp (Little-Endian)
	binary.LittleEndian.PutUint32(header[68:72], uint32(time.Now().Unix()))

	// Bytes 72-76: Bits (Little-Endian)
	binary.LittleEndian.PutUint32(header[72:76], BitcoinBits)

	// Bytes 76-80: Nonce (Little-Endian) - The target we're hunting for
	binary.LittleEndian.PutUint32(header[76:80], nonce)

	return header
}

// BuildHeaderBatch generates multiple Bitcoin headers for a population of candidate nonces
// Optimized for GPU batch processing - generates all headers at once to minimize memory allocations
func (tr *TrainingRecord) BuildHeaderBatch(nonces []uint32) [][]byte {
	headers := make([][]byte, len(nonces))
	slots := tr.AsicSlots()

	// Pre-compute static parts of header
	staticHeader := make([]byte, 76)

	// Bytes 0-3: Version (Little-Endian)
	binary.LittleEndian.PutUint32(staticHeader[0:4], BitcoinVersion)

	// Bytes 4-36: PrevBlockHash from Slots 0-7 (Big-Endian)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(staticHeader[4+(i*4):], slots[i])
	}

	// Bytes 36-68: MerkleRoot from Slots 8-11 + padding (Big-Endian)
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint32(staticHeader[36+(i*4):], slots[i+8])
	}
	// Remaining MerkleRoot bytes (12 bytes) padded with zeros
	for i := 16; i < 20; i++ {
		staticHeader[36+i*4] = 0
		staticHeader[37+i*4] = 0
		staticHeader[38+i*4] = 0
		staticHeader[39+i*4] = 0
	}

	// Bytes 68-72: Timestamp (Little-Endian) - use same timestamp for batch
	binary.LittleEndian.PutUint32(staticHeader[68:72], uint32(time.Now().Unix()))

	// Bytes 72-76: Bits (Little-Endian)
	binary.LittleEndian.PutUint32(staticHeader[72:76], BitcoinBits)

	// Generate headers with different nonces
	for i, nonce := range nonces {
		header := make([]byte, 80)
		copy(header[:76], staticHeader)
		binary.LittleEndian.PutUint32(header[76:80], nonce)
		headers[i] = header
	}

	return headers
}

type SeedResult struct {
	SeedID     uint32  `json:"seed_id"`
	Seed       []byte  `json:"seed"`
	HashOutput uint32  `json:"hash_output"` // Store the actual hash for bit-matching
	Reward     float64 `json:"reward"`
	Advantage  float64 `json:"advantage"`
	Alignment  float64 `json:"alignment"`
	Stability  float64 `json:"stability"`
	Format     float64 `json:"format"`
}

type SeedPopulation struct {
	Seeds       map[uint32][]byte `json:"seeds"`
	Generation  int32             `json:"generation"`
	Fitness     float64           `json:"fitness"`
	TargetToken int32             `json:"target_token"`
	ContextHash uint32            `json:"context_hash"`
}

type WeightRecord struct {
	TokenID      int32   `json:"token_id"`
	BestSeed     []byte  `json:"best_seed"`
	FitnessScore float64 `json:"fitness_score"`
	Generation   int32   `json:"generation"`
	ContextKey   uint32  `json:"context_key"`
}

type CheckpointEntry struct {
	TokenID      int32   `json:"token_id"`
	SeedHash     []byte  `json:"seed_hash"`
	BestSeed     []byte  `json:"best_seed"`
	FitnessScore float64 `json:"fitness_score"`
	LastUpdated  int64   `json:"last_updated"`
}

type TrainingConfig struct {
	PopulationSize  int     `json:"population_size"`
	MaxGenerations  int     `json:"max_generations"`
	EliteRatio      float64 `json:"elite_ratio"`
	MutationRate    float64 `json:"mutation_rate"`
	TargetFitness   float64 `json:"target_fitness"`
	ValidationSplit float64 `json:"validation_split"`
}

func NewSeedPopulation(targetToken int32, contextHash uint32, size int) *SeedPopulation {
	population := &SeedPopulation{
		Seeds:       make(map[uint32][]byte),
		Generation:  0,
		Fitness:     0.0,
		TargetToken: targetToken,
		ContextHash: contextHash,
	}

	for i := 0; i < size; i++ {
		seed := make([]byte, SeedSize)
		rand.Read(seed)
		population.Seeds[uint32(i)] = seed
	}

	return population
}

// GenerateCandidateNonces creates a slice of candidate nonces for Bitcoin mining
// Optimized for the Evolutionary GRPO process
func (sp *SeedPopulation) GenerateCandidateNonces(count int) []uint32 {
	nonces := make([]uint32, count)

	// Generate diverse nonces using different strategies
	quarter := count / 4

	// Strategy 1: Sequential nonces (deterministic exploration)
	for i := 0; i < quarter; i++ {
		nonces[i] = uint32(int(sp.Generation)*1000 + i)
	}

	// Strategy 2: Random nonces (stochastic exploration)
	for i := quarter; i < 2*quarter; i++ {
		nonces[i] = mathrand.Uint32()
	}

	// Strategy 3: Targeted nonces based on current best performers
	if len(sp.Seeds) > 0 {
		bestSeedID := sp.getBestSeedID()
		for i := 2 * quarter; i < 3*quarter; i++ {
			// Mutate around best performer
			mutation := uint32(mathrand.Intn(1000) - 500) // +/- 500 range
			nonces[i] = bestSeedID + mutation
		}
	} else {
		// Fallback to random if no seeds yet
		for i := 2 * quarter; i < 3*quarter; i++ {
			nonces[i] = mathrand.Uint32()
		}
	}

	// Strategy 4: Hash-derived nonces (cryptographic exploration)
	for i := 3 * quarter; i < count; i++ {
		seed := make([]byte, SeedSize)
		rand.Read(seed)
		// Use first 4 bytes of SHA-256 hash of random seed
		hasher := sha256.New()
		hasher.Write(seed)
		hash := hasher.Sum(nil)
		nonces[i] = binary.LittleEndian.Uint32(hash[:4])
	}

	return nonces
}

// getBestSeedID finds the seed ID with highest fitness (placeholder)
func (sp *SeedPopulation) getBestSeedID() uint32 {
	var bestID uint32
	for id := range sp.Seeds {
		bestID = id
		break
	}
	return bestID
}

// GetSeedIDs returns all seed IDs in the population
func (sp *SeedPopulation) GetSeedIDs() []uint32 {
	ids := make([]uint32, 0, len(sp.Seeds))
	for id := range sp.Seeds {
		ids = append(ids, id)
	}
	return ids
}

func (tr *TrainingRecord) Validate() bool {
	if len(tr.TokenSequence) == 0 {
		return false
	}

	if tr.TargetToken <= 0 {
		return false
	}

	// Check if FeatureVector is all zeros
	if tr.FeatureVector == [12]uint32{} {
		return false
	}

	// Allow zeros in FeatureVector - they are valid ASIC slot values
	return true
}

func GenerateRandomSeed() []byte {
	seed := make([]byte, SeedSize)
	rand.Read(seed)
	return seed
}

type RewardCalculator struct {
	TokenMap        map[int32]bool
	AlignmentWeight float64
	StabilityWeight float64
	FormatWeight    float64
}

type EvolutionaryHarness struct {
	PopulationSize int
	EliteRatio     float64
	MutationRate   float64
	RewardCalc     *RewardCalculator
	rand           *mathrand.Rand
	DifficultyMask uint32 // For partial difficulty validation (e.g., 0xFFFF0000 for 16-bit match)
	StaticMidstate bool   // Freeze jitter for first N generations
	Generation     int    // Track current generation for midstate logic
	Epoch          int    // Track current epoch for DDS
}

func NewEvolutionaryHarness(populationSize int) *EvolutionaryHarness {
	// Default difficulty mask uses centralized constant (12 bits = 0xFFF00000)
	// This ensures consistency across the entire codebase
	var defaultMask uint32
	switch config.DefaultDifficultyBits {
	case 8:
		defaultMask = 0xFF000000
	case 12:
		defaultMask = 0xFFF00000
	case 16:
		defaultMask = 0xFFFF0000
	case 20:
		defaultMask = 0xFFFFF000
	case 24:
		defaultMask = 0xFFFFFF00
	case 28:
		defaultMask = 0xFFFFFFF0
	case 32:
		defaultMask = 0xFFFFFFFF
	default:
		defaultMask = 0xFFF00000 // Fallback to 12 bits
	}

	return &EvolutionaryHarness{
		PopulationSize: populationSize,
		EliteRatio:     0.25,
		MutationRate:   0.05,
		RewardCalc: &RewardCalculator{
			TokenMap:        make(map[int32]bool),
			AlignmentWeight: 0.6,
			StabilityWeight: 0.3,
			FormatWeight:    0.1,
		},
		rand:           mathrand.New(mathrand.NewSource(time.Now().UnixNano())),
		DifficultyMask: defaultMask, // Uses centralized DefaultDifficultyBits constant
		StaticMidstate: true,
		Generation:     0,
		Epoch:          1,
	}
}

// UpdateDifficulty scales the difficulty mask based on the current epoch
// Starts at DefaultDifficultyBits and increases over epochs
func (eh *EvolutionaryHarness) UpdateDifficulty(epoch int) {
	eh.Epoch = epoch

	// Base bits at epoch 1 = DefaultDifficultyBits (default: 12)
	// Target bits at epoch 10 = 24
	// Increment: ~1.33 bits per epoch

	targetBits := config.DefaultDifficultyBits + int(float64(epoch-1)*1.33)
	if targetBits > config.MaxDifficultyBits {
		targetBits = config.MaxDifficultyBits
	}
	if targetBits < config.MinDifficultyBits {
		targetBits = config.MinDifficultyBits
	}

	// Create mask using lookup to avoid overflow issues
	var mask uint32
	switch targetBits {
	case 8:
		mask = 0xFF000000
	case 9:
		mask = 0xFF800000
	case 10:
		mask = 0xFFC00000
	case 11:
		mask = 0xFFE00000
	case 12:
		mask = 0xFFF00000
	case 13:
		mask = 0xFFF80000
	case 14:
		mask = 0xFFFC0000
	case 15:
		mask = 0xFFFE0000
	case 16:
		mask = 0xFFFF0000
	case 17:
		mask = 0xFFFF8000
	case 18:
		mask = 0xFFFFC000
	case 19:
		mask = 0xFFFFE000
	case 20:
		mask = 0xFFFFF000
	case 21:
		mask = 0xFFFFF800
	case 22:
		mask = 0xFFFFFC00
	case 23:
		mask = 0xFFFFFE00
	case 24:
		mask = 0xFFFFFF00
	case 25:
		mask = 0xFFFFFF80
	case 26:
		mask = 0xFFFFFFC0
	case 27:
		mask = 0xFFFFFFE0
	case 28:
		mask = 0xFFFFFFF0
	case 29:
		mask = 0xFFFFFFF8
	case 30:
		mask = 0xFFFFFFFC
	case 31:
		mask = 0xFFFFFFFE
	case 32:
		mask = 0xFFFFFFFF
	default:
		mask = 0xFFF00000 // Default to 12 bits
	}

	eh.DifficultyMask = mask
	fmt.Printf("[DDS] Epoch %d: Difficulty set to %d bits (Mask: 0x%08X)\n", epoch, targetBits, eh.DifficultyMask)
}

// SetDifficultyMask sets the difficulty mask for partial validation
func (eh *EvolutionaryHarness) SetDifficultyMask(mask uint32) {
	eh.DifficultyMask = mask
}

// IsWinningSeed checks if hash matches target within difficulty mask
func (eh *EvolutionaryHarness) IsWinningSeed(hash uint32, target uint32) bool {
	return (hash & eh.DifficultyMask) == (target & eh.DifficultyMask)
}

func (eh *EvolutionaryHarness) calculateAlignmentReward(goldenNonce uint32, targetToken int32, tokenMap map[int32]bool) float64 {
	// Exact match gets full reward
	if goldenNonce == uint32(targetToken) {
		return 1.0
	}

	// Prefix matching: reward based on leading bits that match (like ASIC difficulty)
	// XOR gives us the bits that differ, LeadingZeros32 tells us how many MSBs match
	diff := goldenNonce ^ uint32(targetToken)
	matchingBits := bits.LeadingZeros32(diff)

	// Calculate how many bits past the difficulty threshold
	difficultyBits := 32 - bits.Len32(eh.DifficultyMask)
	bitsAboveThreshold := matchingBits - difficultyBits

	// REWARD SCHEME: Clear separation between "winning" and "not winning"
	if matchingBits >= int(difficultyBits) {
		// Passing the difficulty threshold - give high reward that increases with margin
		// 16 bits on 16-bit difficulty = 0.85 (clear winner)
		// 24 bits on 16-bit difficulty = 1.0 (exceptional)
		baseReward := 0.85
		if bitsAboveThreshold > 0 {
			additional := float64(bitsAboveThreshold) / float64(32-difficultyBits) * 0.15
			return baseReward + additional
		}
		return baseReward
	}

	// Below difficulty threshold - much lower rewards to create clear gradient
	if matchingBits >= 12 {
		// Close but not passing - give partial reward
		return 0.3 + 0.3*float64(matchingBits-12)/4.0
	} else if matchingBits >= 8 {
		// Moderate prefix - smaller reward
		return 0.1 + 0.2*float64(matchingBits-8)/4.0
	} else if matchingBits >= 4 {
		// Weak prefix - tiny reward
		return 0.02 + 0.08*float64(matchingBits-4)/4.0
	}

	// No meaningful prefix - essentially noise
	return float64(matchingBits) / 32.0 * 0.02
}

func (eh *EvolutionaryHarness) calculateFormatReward(goldenNonce uint32, tokenToken int32, tokenMap map[int32]bool) float64 {
	// Reward if nonce resolves to any valid token in the map
	if tokenMap[int32(goldenNonce)] {
		return 1.0
	}
	return 0.0
}

// simulateBitcoinHeader attempts to use Bitcoin header simulation if available
// Falls back to regular hash simulation if not available
func (eh *EvolutionaryHarness) simulateBitcoinHeader(header []byte, sim simulator.HashSimulator) (uint32, error) {
	// Use the interface method directly
	result, err := sim.SimulateBitcoinHeader(header)
	if err == nil {
		return result, nil
	}

	// Fallback: use regular SHA-256 simulation
	return sim.SimulateHash(header[:32], 20) // Use first 32 bytes as "seed"
}

// calculateStabilityReward measures convergence behavior across hash passes
func (eh *EvolutionaryHarness) calculateStabilityReward(seed []byte, sim simulator.HashSimulator) float64 {
	// Simulate multiple passes to measure stability
	passes := make([]uint32, 5) // Last 5 passes
	for i := 0; i < 5; i++ {
		pass := 16 + i // Passes 16-20
		output, err := sim.SimulateHash(seed, pass)
		if err != nil {
			return 0.0
		}
		passes[i] = output
	}

	// Calculate variance across passes (lower = more stable)
	var mean uint32
	for _, pass := range passes {
		mean += pass
	}
	mean /= 5

	var variance float64
	for _, pass := range passes {
		diff := float64(pass) - float64(mean)
		variance += diff * diff
	}
	variance /= 5

	// Convert variance to stability reward (lower variance = higher reward)
	maxVariance := 1000000.0 // Maximum expected variance
	stability := 1.0 - (variance / maxVariance)
	if stability < 0 {
		stability = 0
	}

	return stability
}

func (eh *EvolutionaryHarness) CalculateReward(seed []byte, targetToken int32, tokenMap map[int32]bool, sim simulator.HashSimulator) (*SeedResult, error) {
	result := &SeedResult{
		Seed: make([]byte, len(seed)),
	}
	copy(result.Seed, seed)

	// Extract nonce from seed (last 4 bytes of 32-byte seed)
	nonce := binary.LittleEndian.Uint32(seed[len(seed)-4:])

	// For Bitcoin compatibility, we need to construct a temporary header
	// In real implementation, this would use the full TrainingRecord
	tempSlots := [12]uint32{
		0x12345678, 0x23456789, 0x34567890, 0x45678901, // Placeholder slots 0-7
		0x56789012, 0x67890123, 0x78901234, 0x89012345, // Placeholder slots 8-11
		0x90123456, 0x01234567, 0x12345678, 0x23456789,
	}

	// Create temporary hardware prep
	hwPrep := hardware.NewHardwarePrep(false)

	// Build Bitcoin header with the nonce from this seed
	bitcoinHeader := hwPrep.PrepareAsicJob(tempSlots, nonce)

	// Perform Double-SHA256 using the Bitcoin header method
	// This replicates the BM1382's hard-wired behavior
	finalHash, err := eh.simulateBitcoinHeader(bitcoinHeader, sim)
	if err != nil {
		return nil, fmt.Errorf("bitcoin header simulation failed: %w", err)
	}

	// The golden nonce is the first 4 bytes of the final Double-SHA256
	goldenNonce := finalHash

	// Calculate alignment reward (primary - matches target)
	alignmentReward := eh.calculateAlignmentReward(goldenNonce, targetToken, tokenMap)

	// Calculate stability reward (convergence patterns)
	stabilityReward := eh.calculateStabilityReward(seed, sim)

	// Calculate format reward (valid token in map)
	formatReward := eh.calculateFormatReward(goldenNonce, targetToken, tokenMap)

	// Bonus reward for exact target match
	exactMatchBonus := 0.0
	if goldenNonce == uint32(targetToken) {
		exactMatchBonus = 0.5 // Significant bonus for exact match
	}

	result.HashOutput = goldenNonce // Store hash for bit-matching
	result.Alignment = alignmentReward
	result.Stability = stabilityReward
	result.Format = formatReward
	result.Reward = alignmentReward + stabilityReward + formatReward + exactMatchBonus

	return result, nil
}

// CalculateBitMatchAdvantage calculates advantage based on bit-match scores (GRPO style)
// This provides a gradient for evolution to follow despite SHA-256 avalanche effect
func (eh *EvolutionaryHarness) CalculateBitMatchAdvantage(results []SeedResult, targetToken int32) []SeedResult {
	if len(results) == 0 {
		return results
	}

	// Calculate bit-match scores for each result
	bitScores := make([]float64, len(results))
	var totalScore float64

	for i, res := range results {
		// Count leading bits that match the target
		diff := res.HashOutput ^ uint32(targetToken)
		matchingBits := bits.LeadingZeros32(diff)
		// Normalize to 0-1 range
		bitScores[i] = float64(matchingBits) / 32.0
		totalScore += bitScores[i]
	}

	mean := totalScore / float64(len(results))

	var varianceSum float64
	for _, score := range bitScores {
		varianceSum += math.Pow(score-mean, 2)
	}
	stdDev := math.Sqrt(varianceSum / float64(len(results)))

	// Calculate advantage based on bit-match scores
	for i := range results {
		if stdDev > 0 {
			results[i].Advantage = (bitScores[i] - mean) / stdDev
		} else {
			results[i].Advantage = bitScores[i] - mean
		}
		// Boost advantage for seeds that pass difficulty mask
		if eh.IsWinningSeed(results[i].HashOutput, uint32(targetToken)) {
			results[i].Advantage += 2.0 // Significant boost for passing difficulty
		}
	}

	return results
}

// Legacy advantage calculation (kept for compatibility)
func (eh *EvolutionaryHarness) CalculateAdvantage(results []SeedResult) []SeedResult {
	if len(results) == 0 {
		return results
	}

	var totalReward float64
	for _, res := range results {
		totalReward += res.Reward
	}
	mean := totalReward / float64(len(results))

	var varianceSum float64
	for _, res := range results {
		varianceSum += math.Pow(res.Reward-mean, 2)
	}
	stdDev := math.Sqrt(varianceSum / float64(len(results)))

	for i := range results {
		if stdDev > 0 {
			results[i].Advantage = (results[i].Reward - mean) / stdDev
		} else {
			results[i].Advantage = results[i].Reward - mean
		}
	}

	return results
}

func (eh *EvolutionaryHarness) SelectAndMutate(results []SeedResult, currentSeeds map[uint32][]byte) map[uint32][]byte {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Advantage > results[j].Advantage
	})

	newGeneration := make(map[uint32][]byte)
	topCount := int(float64(len(results)) * eh.EliteRatio)

	for i := 0; i < len(results); i++ {
		seedID := results[i].SeedID
		if _, exists := currentSeeds[seedID]; exists {
			if i < topCount {
				originalSeed := currentSeeds[seedID]
				newGeneration[seedID] = originalSeed

				if len(newGeneration) < eh.PopulationSize {
					mutatedID := uint32(eh.rand.Intn(1000000))
					mutatedSeed := eh.BitwiseMutation(originalSeed, results[i].Advantage)
					newGeneration[mutatedID] = mutatedSeed
				}
			}
		}
	}

	for len(newGeneration) < eh.PopulationSize {
		seedID := uint32(eh.rand.Intn(1000000))
		newGeneration[seedID] = GenerateRandomSeed()
	}

	return newGeneration
}

// BitcoinAwareMutate performs mutation optimized for Bitcoin nonce hunting
// Focuses on mutating the last 4 bytes (nonce) more aggressively
func (eh *EvolutionaryHarness) BitcoinAwareMutate(seed []byte, advantage float64) []byte {
	mutated := make([]byte, len(seed))
	copy(mutated, seed)

	mutationRate := MutationRateBase / (math.Abs(advantage) + 1)
	if mutationRate < 1 {
		mutationRate = 1
	}
	if mutationRate > 10 {
		mutationRate = 10
	}

	// For Bitcoin compatibility, focus mutations on nonce (last 4 bytes)
	nonceMutationRate := int(mutationRate * 0.7) // 70% focus on nonce
	seedMutationRate := int(mutationRate * 0.3)  // 30% on rest of seed

	// Mutate nonce bytes (last 4 bytes) more aggressively
	for i := 0; i < nonceMutationRate; i++ {
		byteIdx := len(mutated) - 4 - eh.rand.Intn(4) // Last 4 bytes
		bitIdx := uint(eh.rand.Intn(8))
		mutated[byteIdx] ^= (1 << bitIdx)
	}

	// Mutate remaining bytes normally
	for i := 0; i < seedMutationRate; i++ {
		byteIdx := eh.rand.Intn(len(mutated) - 4) // Exclude nonce bytes
		bitIdx := uint(eh.rand.Intn(8))
		mutated[byteIdx] ^= (1 << bitIdx)
	}

	return mutated
}

// GenerateNoncesForPopulation creates optimized nonces for Bitcoin mining
// Uses multiple strategies to explore nonce space effectively
func (eh *EvolutionaryHarness) GenerateNoncesForPopulation(population *SeedPopulation, count int) []uint32 {
	nonces := make([]uint32, count)

	// Strategy 1: Sequential nonces (deterministic exploration)
	sequentialCount := count / 4
	for i := 0; i < sequentialCount; i++ {
		nonces[i] = uint32(int(population.Generation)*10000 + i)
	}

	// Strategy 2: Random nonces (stochastic exploration)
	randomStart := sequentialCount
	randomEnd := sequentialCount + count/4
	for i := randomStart; i < randomEnd && i < count; i++ {
		nonces[i] = eh.rand.Uint32()
	}

	// Strategy 3: Targeted nonces based on current best performers
	targetedStart := randomEnd
	targetedEnd := targetedStart + count/4
	bestNonce := eh.findBestNonce(population)
	for i := targetedStart; i < targetedEnd && i < count; i++ {
		mutation := uint32(eh.rand.Intn(10000) - 5000) // +/- 5000 range
		nonces[i] = bestNonce + mutation
	}

	// Strategy 4: Cryptographic nonces
	cryptoStart := targetedEnd
	for i := cryptoStart; i < count; i++ {
		nonces[i] = eh.generateCryptoNonce()
	}

	return nonces
}

// findBestNonce finds the nonce from the best performing seed
func (eh *EvolutionaryHarness) findBestNonce(population *SeedPopulation) uint32 {
	bestNonce := uint32(0)
	for _, seed := range population.Seeds {
		if len(seed) >= 4 {
			bestNonce = binary.LittleEndian.Uint32(seed[len(seed)-4:])
			break
		}
	}
	return bestNonce
}

// generateCryptoNonce creates a cryptographically-derived nonce
func (eh *EvolutionaryHarness) generateCryptoNonce() uint32 {
	seed := make([]byte, 32)
	rand.Read(seed)
	hasher := sha256.New()
	hasher.Write(seed)
	hash := hasher.Sum(nil)
	return binary.LittleEndian.Uint32(hash[:4])
}

func (eh *EvolutionaryHarness) BitwiseMutation(seed []byte, advantage float64) []byte {
	mutated := make([]byte, len(seed))
	copy(mutated, seed)

	mutationRate := MutationRateBase / (math.Abs(advantage) + 1)
	if mutationRate < 1 {
		mutationRate = 1
	}
	if mutationRate > 10 {
		mutationRate = 10
	}

	for i := 0; i < int(mutationRate); i++ {
		byteIdx := eh.rand.Intn(len(mutated))
		bitIdx := uint(eh.rand.Intn(8))
		mutated[byteIdx] ^= (1 << bitIdx)
	}

	return mutated
}

func (eh *EvolutionaryHarness) EvaluatePopulation(population *SeedPopulation, record *TrainingRecord, tokenMap map[int32]bool, sim simulator.HashSimulator) ([]SeedResult, error) {
	var results []SeedResult

	// Extract nonces from seeds for batch processing
	seedIDs := make([]uint32, 0, len(population.Seeds))
	candidateNonces := make([]uint32, 0, len(population.Seeds))

	for seedID, seed := range population.Seeds {
		seedIDs = append(seedIDs, seedID)
		// Extract nonce from last 4 bytes of seed
		nonce := binary.LittleEndian.Uint32(seed[len(seed)-4:])
		candidateNonces = append(candidateNonces, nonce)
	}

	results, err := eh.EvaluatePopulationBatch(population, record, tokenMap, sim, seedIDs, candidateNonces)
	if err != nil {
		return nil, err
	}

	// Calculate population fitness
	var totalFitness float64
	for _, res := range results {
		if res.Advantage > 0 {
			totalFitness += res.Reward
		}
	}
	if len(results) > 0 {
		population.Fitness = totalFitness / float64(len(results))
	}

	return results, nil
}

// EvaluatePopulationBatch processes multiple seeds efficiently using Bitcoin headers
// Optimized for GPU batch processing and BM1382 "Camouflage" strategy
func (eh *EvolutionaryHarness) EvaluatePopulationBatch(
	population *SeedPopulation,
	record *TrainingRecord,
	tokenMap map[int32]bool,
	sim simulator.HashSimulator,
	seedIDs []uint32,
	candidateNonces []uint32,
) ([]SeedResult, error) {
	results := make([]SeedResult, len(seedIDs))

	// The record's FeatureVector contains the contextual data for the ASIC job
	slots := record.FeatureVector

	// Create hardware prep for batch processing
	hwPrep := hardware.NewHardwarePrep(true) // Enable caching for performance

	// Generate Bitcoin headers for all candidate nonces in batch
	headers := hwPrep.PrepareAsicJobBatch(slots, candidateNonces)

	// Evaluate each header
	for i, header := range headers {
		// Reconstruct seed from original seeds
		seed := population.Seeds[seedIDs[i]]

		result := &SeedResult{
			Seed:   make([]byte, len(seed)),
			SeedID: seedIDs[i],
		}
		copy(result.Seed, seed)

		// Perform 21-pass temporal loop using RecursiveMine (Pathfinder logic)
		// This invokes the Jitter RPC for associative nudging
		finalHash, err := sim.RecursiveMine(header, 21)
		if err != nil {
			eh.logError("RecursiveMine failed for seed %d: %v", seedIDs[i], err)
			result.Reward = 0.0
			results[i] = *result
			continue
		}

		// The user wants the Double-SHA256 hashed result to be used as the final Seed
		result.Seed = make([]byte, len(finalHash))
		copy(result.Seed, finalHash)

		// The golden nonce is first 4 bytes of final Double-SHA256 for bit-matching
		goldenNonce := binary.BigEndian.Uint32(finalHash[:4])

		// Store hash output for bit-matching
		result.HashOutput = goldenNonce

		// Calculate rewards
		result.Alignment = eh.calculateAlignmentReward(goldenNonce, record.TargetToken, tokenMap)
		result.Stability = eh.calculateStabilityReward(seed, sim)
		result.Format = eh.calculateFormatReward(goldenNonce, record.TargetToken, tokenMap)

		// Bonus for exact target match
		exactMatchBonus := 0.0
		if goldenNonce == uint32(record.TargetToken) {
			exactMatchBonus = 0.5 // Significant bonus for exact match
		}

		result.Reward = result.Alignment + result.Stability + result.Format + exactMatchBonus
		results[i] = *result
	}

	// Calculate advantages using bit-match scores (provides gradient despite SHA-256 avalanche)
	results = eh.CalculateBitMatchAdvantage(results, record.TargetToken)

	return results, nil
}

// logError helper for logging (placeholder - would use actual logger)
func (eh *EvolutionaryHarness) logError(format string, args ...interface{}) {
	// In real implementation, this would use proper logging
	fmt.Printf("ERROR: "+format+"\n", args...)
}

func (eh *EvolutionaryHarness) GetEliteSeeds(results []SeedResult) []SeedResult {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Advantage > results[j].Advantage
	})

	eliteCount := int(float64(len(results)) * eh.EliteRatio)
	if eliteCount == 0 && len(results) > 0 {
		eliteCount = 1
	}

	return results[:eliteCount]
}

func hammingDistance(a, b uint32) int {
	xor := a ^ b
	distance := 0
	for xor != 0 {
		distance += int(xor & 1)
		xor >>= 1
	}
	return distance
}

func ComputeContextHash(tokenSequence []int32, windowSize int) uint32 {
	if len(tokenSequence) == 0 {
		return 0
	}

	start := len(tokenSequence) - windowSize
	if start < 0 {
		start = 0
	}

	hash := make([]byte, 0, 4)
	for i := start; i < len(tokenSequence); i++ {
		hash = append(hash, byte(tokenSequence[i]), byte(tokenSequence[i]>>8), byte(tokenSequence[i]>>16), byte(tokenSequence[i]>>24))
	}

	if len(hash) < 4 {
		hash = append(hash, 0, 0, 0, 0)
	}

	return uint32(hash[0]) | uint32(hash[1])<<8 | uint32(hash[2])<<16 | uint32(hash[3])<<24
}
