package simulator

import (
	"testing"
	"time"
)

func TestNewvHasherSimulator(t *testing.T) {
	config := &SimulatorConfig{
		DeviceType:     "vhasher",
		MaxConcurrency: 100,
		TargetHashRate: 500000000,
		CacheSize:      10000,
		GPUDevice:      0,
		Timeout:        30,
	}

	sim := NewvHasherSimulator(config)
	if sim == nil {
		t.Fatal("Expected non-nil simulator")
	}

	if err := sim.Initialize(config); err != nil {
		t.Fatalf("Failed to initialize simulator: %v", err)
	}

	defer sim.Shutdown()
}

func TestVHasherSimulatorSimulateHash(t *testing.T) {
	sim := NewvHasherSimulator(nil)
	if err := sim.Initialize(nil); err != nil {
		t.Fatalf("Failed to initialize simulator: %v", err)
	}
	defer sim.Shutdown()

	seed := []byte{0x01, 0x02, 0x03, 0x04}

	result1, err := sim.SimulateHash(seed, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result2, err := sim.SimulateHash(seed, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result1 != result2 {
		t.Error("Same seed and pass should produce same result")
	}

	result3, err := sim.SimulateHash(seed, 1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result2 == result3 {
		t.Error("Different passes should produce different results")
	}
}

func TestVHasherSimulatorValidateSeed(t *testing.T) {
	sim := NewvHasherSimulator(nil)
	if err := sim.Initialize(nil); err != nil {
		t.Fatalf("Failed to initialize simulator: %v", err)
	}
	defer sim.Shutdown()

	seed := []byte{0x01, 0x02, 0x03, 0x04}
	targetToken := int32(0x04030201)

	valid, err := sim.ValidateSeed(seed, targetToken)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if valid {
		t.Error("Seed should not be valid for this target token")
	}

	hashResult, err := sim.SimulateHash(seed, 20)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	valid, err = sim.ValidateSeed(seed, int32(hashResult))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !valid {
		t.Error("Seed should be valid for its own hash result")
	}
}

func TestVHasherSimulatorCaching(t *testing.T) {
	sim := NewvHasherSimulator(nil)
	if err := sim.Initialize(nil); err != nil {
		t.Fatalf("Failed to initialize simulator: %v", err)
	}
	defer sim.Shutdown()

	seed := []byte{0x01, 0x02, 0x03, 0x04}

	start := time.Now()
	result1, err := sim.SimulateHash(seed, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	firstCallTime := time.Since(start)

	start = time.Now()
	result2, err := sim.SimulateHash(seed, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	secondCallTime := time.Since(start)

	if result1 != result2 {
		t.Error("Cached results should be identical")
	}

	if secondCallTime > firstCallTime {
		t.Error("Cached call should be faster than first call")
	}
}

func TestVHasherSimulatorGetDeviceStats(t *testing.T) {
	sim := NewvHasherSimulator(nil)
	if err := sim.Initialize(nil); err != nil {
		t.Fatalf("Failed to initialize simulator: %v", err)
	}
	defer sim.Shutdown()

	seed := []byte{0x01, 0x02, 0x03, 0x04}

	initialStats, err := sim.GetDeviceStats()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for i := 0; i < 10; i++ {
		_, err := sim.SimulateHash(seed, i)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	finalStats, err := sim.GetDeviceStats()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if finalStats.TotalHashes <= initialStats.TotalHashes {
		t.Error("Total hashes should increase after operations")
	}

	if finalStats.DeviceTemp < initialStats.DeviceTemp {
		t.Error("Device temperature should not decrease after operations")
	}
}

func BenchmarkVHasherSimulatorSimulateHash(b *testing.B) {
	sim := NewvHasherSimulator(nil)
	if err := sim.Initialize(nil); err != nil {
		b.Fatalf("Failed to initialize simulator: %v", err)
	}
	defer sim.Shutdown()

	seed := []byte{0x01, 0x02, 0x03, 0x04}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sim.SimulateHash(seed, i%21)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}
