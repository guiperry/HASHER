package training

import (
	"crypto/rand"
	"math"
	mathrand "math/rand"
	"sort"
	"time"

	"github.com/lab/hasher/data-trainer/pkg/simulator"
)

const (
	SeedSize          = 32
	FeatureVectorSize = 12
	GroupSize         = 128
	MutationRateBase  = 10
)

type TrainingRecord struct {
	TokenSequence []int32    `json:"token_sequence"`
	FeatureVector [12]uint32 `json:"feature_vector"`
	TargetToken   int32      `json:"target_token"`
	ContextHash   uint32     `json:"context_hash"`
}

type SeedResult struct {
	SeedID    uint32  `json:"seed_id"`
	Seed      []byte  `json:"seed"`
	Reward    float64 `json:"reward"`
	Advantage float64 `json:"advantage"`
	Alignment float64 `json:"alignment"`
	Stability float64 `json:"stability"`
	Format    float64 `json:"format"`
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

func (tr *TrainingRecord) Validate() bool {
	if len(tr.TokenSequence) == 0 {
		return false
	}
	if tr.TargetToken <= 0 {
		return false
	}
	if tr.FeatureVector != [12]uint32{} {
		for _, v := range tr.FeatureVector {
			if v == 0 {
				return false
			}
		}
		return true
	}
	return false
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
}

func NewEvolutionaryHarness(populationSize int) *EvolutionaryHarness {
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
		rand: mathrand.New(mathrand.NewSource(time.Now().UnixNano())),
	}
}

func (eh *EvolutionaryHarness) calculateAlignmentReward(goldenNonce uint32, targetToken int32, tokenMap map[int32]bool) float64 {
	// Exact match gets full reward
	if goldenNonce == uint32(targetToken) {
		return 1.0
	}

	// Partial match or related token gets partial reward
	if tokenMap[int32(goldenNonce)] {
		// Check if it's in the same semantic range (simple heuristic)
		diff := int32(targetToken) - int32(goldenNonce)
		if diff >= -100 && diff <= 100 {
			return 0.7 // Close semantic match
		}
		return 0.3 // Same token map but different range
	}

	return 0.0
}

func (eh *EvolutionaryHarness) calculateFormatReward(goldenNonce uint32, tokenToken int32, tokenMap map[int32]bool) float64 {
	// Reward if nonce resolves to any valid token in the map
	if tokenMap[int32(goldenNonce)] {
		return 1.0
	}
	return 0.0
}

func (eh *EvolutionaryHarness) CalculateReward(seed []byte, targetToken int32, tokenMap map[int32]bool, sim simulator.HashSimulator) (*SeedResult, error) {
	result := &SeedResult{
		Seed: make([]byte, len(seed)),
	}
	copy(result.Seed, seed)

	// Get all 21 passes for comprehensive analysis
	passes := make([]uint32, 21)
	for i := 0; i < 21; i++ {
		output, err := sim.SimulateHash(seed, i)
		if err != nil {
			return nil, err
		}
		passes[i] = output
	}

	goldenNonce := passes[20] // Pass 20 is the golden nonce

	// Calculate alignment reward (primary - matches target)
	alignmentReward := eh.calculateAlignmentReward(goldenNonce, targetToken, tokenMap)

	// Calculate stability reward (convergence in later passes)
	var stabilityReward float64
	if len(passes) >= 20 {
		// Measure stability in the final few passes
		var totalStability float64
		var comparisons int

		for i := 17; i < 20; i++ {
			// Compare each pass with the final pass
			dist := hammingDistance(passes[i], passes[20])
			totalStability += float64(dist)
			comparisons++
		}

		if comparisons > 0 {
			// Lower average distance = more stable
			avgDist := totalStability / float64(comparisons)
			stabilityReward = 1.0 - (avgDist / 32.0)
		} else {
			stabilityReward = 0.0 // No stability data available
		}
	}

	// Calculate format reward (valid token in map)
	formatReward := eh.calculateFormatReward(goldenNonce, targetToken, tokenMap)

	// Bonus reward for finding target early (in earlier passes)
	earlyFindBonus := 0.0
	for i := 0; i < 19; i++ {
		if passes[i] == uint32(targetToken) {
			// Found target earlier = better
			earlyFindBonus = 0.2 * (19.0 - float64(i)/19.0) // Bonus for finding earlier
			break
		}
	}

	result.Alignment = alignmentReward
	result.Stability = stabilityReward
	result.Format = formatReward
	result.Reward = alignmentReward + stabilityReward + formatReward + earlyFindBonus

	return result, nil
}

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

func (eh *EvolutionaryHarness) EvaluatePopulation(population *SeedPopulation, targetToken int32, tokenMap map[int32]bool, sim simulator.HashSimulator) ([]SeedResult, error) {
	var results []SeedResult

	for seedID, seed := range population.Seeds {
		result, err := eh.CalculateReward(seed, targetToken, tokenMap, sim)
		if err != nil {
			continue
		}
		result.SeedID = seedID
		results = append(results, *result)
	}

	results = eh.CalculateAdvantage(results)

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
