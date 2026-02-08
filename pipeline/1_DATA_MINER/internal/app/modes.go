package app

import (
	"fmt"
	"os"
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
		fmt.Println("🏭 Production Mode: Full workflow with optimal defaults for production deployment")
	case "test":
		fmt.Println("🧪 Test Mode: Limited scope for testing functionality")
	case "optimized":
		fmt.Println("⚡ Optimized Mode: CPU/GPU optimizations enabled for maximum performance")
	case "hybrid":
		fmt.Println("🌐 Hybrid Mode: Hybrid embeddings (Cloudflare + Ollama fallback)")
	case "neural":
		fmt.Println("🧠 Neural Mode: Neural processing of existing PDFs only")
	case "arxiv":
		fmt.Println("📚 ArXiv Mode: ArXiv mining only")
	case "workflow":
		fmt.Println("🔄 Workflow Mode: Integrated arXiv + neural processing")
	default:
		fmt.Println("🔧 Default Mode: Standard neural processing")
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
