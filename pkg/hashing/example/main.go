package main

import (
	"fmt"
	"log"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/factory"
)

func main() {
	fmt.Println("HASHER Hashing Methods Example")
	fmt.Println("==============================")

	// Example 1: Default configuration (production mode)
	fmt.Println("\n1. Default Production Configuration:")
	defaultConfig := factory.DefaultHashMethodConfig()
	runHashingExample(defaultConfig, "Production")

	// Example 2: Training configuration
	fmt.Println("\n2. Training Configuration:")
	trainingConfig := factory.TrainingHashMethodConfig()
	runHashingExample(trainingConfig, "Training")

	// Example 3: Custom configuration
	fmt.Println("\n3. Custom Configuration:")
	customConfig := &factory.HashMethodConfig{
		PreferredOrder: []string{
			"software", // Force software-only for testing
			"cuda",
			"asic",
			"ubpf",
			"ebpf",
		},
		EnableFallback: true,
		TrainingMode:   false,
	}
	runHashingExample(customConfig, "Custom Software-Only")
}

func runHashingExample(config *factory.HashMethodConfig, mode string) {
	// Create factory with configuration
	fact := factory.NewHashMethodFactory(config)

	// Get detection report
	report := fact.GetDetectionReport()

	fmt.Printf("\n%s Mode - Detection Results:\n", mode)
	for _, method := range report.Methods {
		status := "❌ UNAVAILABLE"
		if method.Available {
			status = "✅ AVAILABLE"
		}
		fmt.Printf("  %-20s %s - %s\n", method.Name, status, method.Description)

		if method.Capabilities != nil {
			caps := method.Capabilities
			fmt.Printf("    Hash Rate: %d H/s\n", caps.HashRate)
			fmt.Printf("    Hardware: %t\n", caps.IsHardware)
			fmt.Printf("    Production: %t\n", caps.ProductionReady)
			if caps.TrainingOptimized {
				fmt.Printf("    Training Optimized: Yes\n")
			}
			if caps.HardwareInfo != nil {
				fmt.Printf("    Device: %s (%s)\n", caps.HardwareInfo.DevicePath, caps.HardwareInfo.ConnectionType)
			}
			if !method.Available && caps.Reason != "" {
				fmt.Printf("    Reason: %s\n", caps.Reason)
			}
		}
		fmt.Println()
	}

	// Get best method and demonstrate usage
	bestMethod := fact.GetBestMethod()
	if bestMethod != nil {
		fmt.Printf("Best Method Selected: %s\n", bestMethod.Name())

		// Initialize the best method
		if err := bestMethod.Initialize(); err != nil {
			log.Printf("Failed to initialize best method: %v\n", err)
			return
		}

		// Demonstrate hashing
		demonstrateHashing(bestMethod)

		// Cleanup
		if err := bestMethod.Shutdown(); err != nil {
			log.Printf("Error shutting down method: %v\n", err)
		}
	} else {
		fmt.Println("No hashing methods available!")
	}
}

func demonstrateHashing(method core.HashMethod) {
	fmt.Println("\nHashing Demonstration:")
	fmt.Println("=====================")

	// Test single hash
	testData := []byte("Hello, HASHER!")
	hash, err := method.ComputeHash(testData)
	if err != nil {
		fmt.Printf("Single hash failed: %v\n", err)
	} else {
		fmt.Printf("Single Hash: %x\n", hash[:8]) // Show first 8 bytes
	}

	// Test batch hashing
	batchData := [][]byte{
		[]byte("Test data 1"),
		[]byte("Test data 2"),
		[]byte("Test data 3"),
	}
	batchHashes, err := method.ComputeBatch(batchData)
	if err != nil {
		fmt.Printf("Batch hash failed: %v\n", err)
	} else {
		fmt.Printf("Batch Hashes:\n")
		for i, h := range batchHashes {
			fmt.Printf("  [%d]: %x\n", i+1, h[:8])
		}
	}

	// Test Bitcoin header mining (if method supports it)
	caps := method.GetCapabilities()
	if caps.ProductionReady || caps.TrainingOptimized {
		// Create a simple Bitcoin header
		header := make([]byte, 80)
		// Set version (bytes 0-3)
		header[0] = 0x02
		// Set timestamp (bytes 68-71)
		// Set difficulty (bytes 72-75)
		// Nonce will be set by mining function

		nonce, err := method.MineHeader(header, 0, 1000)
		if err != nil {
			fmt.Printf("Mining failed: %v\n", err)
		} else {
			fmt.Printf("Mining Result: Nonce = %d\n", nonce)
		}
	}
}
