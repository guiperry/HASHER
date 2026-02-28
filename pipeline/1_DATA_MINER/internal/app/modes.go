package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RunApplication is the main entry point - runs the orchestrator with continuous workflow
func RunApplication(config *Config) error {
	// Validate configuration
	if err := ValidateConfiguration(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create necessary directories
	if err := CreateDirectories(config); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Handle dry run mode
	if config.DryRun {
		PrintConfiguration(config)
		fmt.Println("🔍 Dry run completed - no processing performed")
		return nil
	}

	// Handle demo mode: generate hello world training frames and exit
	if config.DemoMode {
		return RunDemoFrameGenerator(config)
	}

	// Enable optimized mode by default
	if !config.GPUOverride {
		config.GPUOptimizations = true
	}

	// Run the orchestrator - continuous workflow is now the default
	fmt.Println("🚀 Starting Data Miner with continuous workflow...")
	return RunOrchestrator(config)
}

// RunScriptMode executes the application with simplified configuration
func RunScriptMode(mode string, args []string) error {
	// Set mode environment variable
	os.Setenv("DATAMINER_MODE", mode)

	// Parse flags
	config := ParseFlags()

	// Apply mode-specific configurations
	switch mode {
	case "production":
		if config.CloudflareLimit > 0 {
			os.Setenv("CLOUDFLARE_DAILY_LIMIT", fmt.Sprintf("%d", config.CloudflareLimit))
		}
	case "test":
		os.Setenv("CLOUDFLARE_DAILY_LIMIT", "100")
	case "optimized":
		config.GPUOptimizations = true
	case "hybrid":
		if config.CloudflareLimit == 0 {
			config.CloudflareLimit = 5000
		}
		os.Setenv("CLOUDFLARE_DAILY_LIMIT", fmt.Sprintf("%d", config.CloudflareLimit))
	}

	// Run the application with the configured mode
	return RunApplication(config)
}

// RunProductionMode runs the production workflow
func RunProductionMode() error {
	return RunScriptMode("production", nil)
}

// RunTestMode runs the test workflow
func RunTestMode() error {
	return RunScriptMode("test", nil)
}

// RunOptimizedMode runs with CPU/GPU optimizations
func RunOptimizedMode() error {
	return RunScriptMode("optimized", nil)
}

// RunHybridMode runs with hybrid embeddings
func RunHybridMode() error {
	return RunScriptMode("hybrid", nil)
}

// PrintModeInfo prints information about the current execution mode
func PrintModeInfo(mode string) {
	switch mode {
	case "production":
		fmt.Println("🏭 Production Mode: Full workflow with GOAT dataset as default source")
	case "test":
		fmt.Println("🧪 Test Mode: Limited scope for testing functionality")
	case "optimized":
		fmt.Println("⚡ Optimized Mode: CPU/GPU optimizations enabled for maximum performance")
	case "hybrid":
		fmt.Println("🌐 Hybrid Mode: Hybrid embeddings (Cloudflare + Ollama fallback)")
	case "neural":
		fmt.Println("🧠 Neural Mode: Neural processing of existing PDFs only")
	case "arxiv":
		fmt.Println("📚 ArXiv Mode: ArXiv mining and PDF processing")
	case "workflow":
		fmt.Println("🔄 Workflow Mode: Integrated processing")
	default:
		fmt.Println("🔧 Default Mode: Standard neural processing")
	}
}

// demoTrainingFrame mirrors the trainer's expected JSON structure for demo frames.
type demoTrainingFrame struct {
	SourceFile    string  `json:"source_file"`
	ChunkID       int     `json:"chunk_id"`
	WindowStart   int     `json:"window_start"`
	TokenSequence []int   `json:"token_sequence"`
	TargetToken   int     `json:"target_token"`
	FeatureVector []int64 `json:"feature_vector"`
	ContextHash   int     `json:"context_hash"`
}

// RunDemoFrameGenerator generates hello world training frames.
// It first tries to run the Python script at the known location; if unavailable,
// it generates the same frames directly in Go.
func RunDemoFrameGenerator(config *Config) error {
	fmt.Println("🎮 Demo Mode: Generating hello world training frames...")

	outputDir := filepath.Join(config.AppDataDir, "frames")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create frames directory: %w", err)
	}
	outputFile := filepath.Join(outputDir, "training_frames.json")

	// Try running the Python script first.
	scriptCandidates := []string{
		filepath.Join("pipeline", "1_DATA_MINER", "scripts", "generate_hello_world.py"),
		filepath.Join("..", "scripts", "generate_hello_world.py"),
	}
	for _, script := range scriptCandidates {
		if _, err := os.Stat(script); err == nil {
			fmt.Printf("📜 Running Python script: %s\n", script)
			cmd := exec.Command("python3", script)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				fmt.Println("✅ Demo frames generated via Python script")
				return nil
			}
			// Python failed — fall through to Go fallback
			fmt.Println("⚠️  Python script failed, generating frames in Go...")
			break
		}
	}

	// Go fallback: generate the same hello world frames directly.
	frames := buildHelloWorldFrames()
	data, err := json.MarshalIndent(frames, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal demo frames: %w", err)
	}
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write demo frames: %w", err)
	}
	fmt.Printf("✅ Demo frames written to %s (%d frames)\n", outputFile, len(frames))
	return nil
}

// buildHelloWorldFrames creates a minimal hello-world conversation dataset.
// Token IDs use cl100k_base values.
func buildHelloWorldFrames() []demoTrainingFrame {
	// Feature vector: [token_sequence_packed, target, zeros...]
	// The trainer uses these 12 slots as the ASIC input header.
	makeFeatureVector := func(tok, target int) []int64 {
		fv := make([]int64, 12)
		fv[0] = int64(tok)
		fv[1] = int64(target)
		return fv
	}

	return []demoTrainingFrame{
		// Pattern 1: Basic "Hello" (9906) -> " world" (1917)
		{SourceFile: "demo.txt", ChunkID: 1, WindowStart: 0,
			TokenSequence: []int{9906}, TargetToken: 1917,
			FeatureVector: makeFeatureVector(9906, 1917)},
		// Pattern 2: " world" (1917) -> "!" (0)
		{SourceFile: "demo.txt", ChunkID: 2, WindowStart: 1,
			TokenSequence: []int{1917}, TargetToken: 0,
			FeatureVector: makeFeatureVector(1917, 0)},
		// Pattern 3: "world" (no space, 14957) -> "!" (0)
		{SourceFile: "demo.txt", ChunkID: 3, WindowStart: 0,
			TokenSequence: []int{14957}, TargetToken: 0,
			FeatureVector: makeFeatureVector(14957, 0)},
		// Pattern 4: "Hasher" (6504, 261) -> " world" (1917)
		{SourceFile: "demo.txt", ChunkID: 4, WindowStart: 0,
			TokenSequence: []int{6504, 261}, TargetToken: 1917,
			FeatureVector: makeFeatureVector(261, 1917)},
		// Pattern 5: "hasher" (8460, 261) -> " world" (1917)
		{SourceFile: "demo.txt", ChunkID: 5, WindowStart: 0,
			TokenSequence: []int{8460, 261}, TargetToken: 1917,
			FeatureVector: makeFeatureVector(261, 1917)},
		// Pattern 6: "What" (3923) -> " is" (374)
		{SourceFile: "demo.txt", ChunkID: 6, WindowStart: 0,
			TokenSequence: []int{3923}, TargetToken: 374,
			FeatureVector: makeFeatureVector(3923, 374)},
		// Pattern 7: " is" (374) -> " your" (701)
		{SourceFile: "demo.txt", ChunkID: 7, WindowStart: 1,
			TokenSequence: []int{374}, TargetToken: 701,
			FeatureVector: makeFeatureVector(374, 701)},
		// Pattern 8: " your" (701) -> " name" (836)
		{SourceFile: "demo.txt", ChunkID: 8, WindowStart: 2,
			TokenSequence: []int{701}, TargetToken: 836,
			FeatureVector: makeFeatureVector(701, 836)},
		// Pattern 9: " name" (836) -> "?" (30)
		{SourceFile: "demo.txt", ChunkID: 9, WindowStart: 3,
			TokenSequence: []int{836}, TargetToken: 30,
			FeatureVector: makeFeatureVector(836, 30)},
		// Pattern 10: "Whats" (59175) -> " your" (701)
		{SourceFile: "demo.txt", ChunkID: 10, WindowStart: 0,
			TokenSequence: []int{59175}, TargetToken: 701,
			FeatureVector: makeFeatureVector(59175, 701)},
	}
}

// ListAvailableModes lists all available execution modes
func ListAvailableModes() {
	fmt.Println("Available execution modes:")
	fmt.Println("  production   - Full production workflow")
	fmt.Println("  test         - Test workflow with limited scope")
	fmt.Println("  optimized    - CPU/GPU optimized processing")
	fmt.Println("  hybrid       - Hybrid embeddings (Cloudflare + Ollama)")
	fmt.Println("  neural       - Neural processing only")
	fmt.Println("  arxiv        - ArXiv mining only")
	fmt.Println("  workflow     - Integrated workflow")
	fmt.Println("")
	fmt.Println("Script modes can be activated using:")
	fmt.Println("  -production, -test, -optimized, -hybrid flags")
	fmt.Println("  Or by setting DATAMINER_MODE environment variable")
}
