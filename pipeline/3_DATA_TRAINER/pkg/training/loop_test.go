package training

import (
	"fmt"
	"testing"

	"github.com/lab/hasher/data-trainer/pkg/simulator"
)

func TestTrainingConvergence(t *testing.T) {
	// Use software simulator for this test
	simConfig := &simulator.SimulatorConfig{
		DeviceType:     "software",
		MaxConcurrency: 10,
		CacheSize:      1000,
	}
	sim := simulator.NewHasherWrapper(simConfig)
	if err := sim.Initialize(simConfig); err != nil {
		t.Fatalf("Failed to initialize simulator: %v", err)
	}
	defer sim.Shutdown()

	harness := NewEvolutionaryHarness(64) // Smaller population for fast test
	harness.SetDifficultyMask(0xF0000000) // 4 bits match (1 in 16)

	targetToken := int32(1234)
	tokenMap := map[int32]bool{targetToken: true}

	record := &TrainingRecord{
		TargetToken:   targetToken,
		TokenSequence: []int32{targetToken},
		FeatureVector: [12]uint32{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xA, 0xB, 0xC},
	}

	contextHash := uint32(12345)
	pop := NewSeedPopulation(targetToken, contextHash, 64)

	found := false
	for gen := 0; gen < 100; gen++ {
		results, err := harness.EvaluatePopulation(pop, record, tokenMap, sim)
		if err != nil {
			t.Fatalf("Generation %d failed: %v", gen, err)
		}

		eliteSeeds := harness.GetEliteSeeds(results)
		if len(eliteSeeds) > 0 {
			bestSeed := eliteSeeds[0]
			if harness.IsWinningSeed(bestSeed.HashOutput, uint32(targetToken)) {
				fmt.Printf("Found winning seed in generation %d! Hash: %08x, Target: %08x\n", 
					gen, bestSeed.HashOutput, uint32(targetToken))
				found = true
				break
			}
			
			if gen%10 == 0 {
				fmt.Printf("Gen %d: Best Hash %08x, Alignment %.4f\n", 
					gen, bestSeed.HashOutput, bestSeed.Alignment)
			}
		}

		pop.Seeds = harness.SelectAndMutate(results, pop.Seeds)
		pop.Generation++
	}

	if !found {
		t.Error("Failed to find 4-bit match in 100 generations (should be easy)")
	}
}
