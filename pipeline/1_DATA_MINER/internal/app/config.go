package app

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"dataminer/internal/arxiv"

	"github.com/joho/godotenv"
)

// LoadEnv loads environment variables from .env file
func LoadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
}

// ParseFlags parses command line flags and returns configuration
func ParseFlags() *Config {
	LoadEnv()
	// Detect script execution mode first
	mode := detectScriptMode()

	// Get number of CPU cores for optimal defaults
	numWorkers := 8
	if runtime.GOOS != "windows" {
		if cmd := exec.Command("nproc"); cmd != nil {
			if output, err := cmd.Output(); err == nil {
				if n, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &numWorkers); err == nil && n > 0 {
					// Use all cores but cap at 16 to avoid excessive parallelism
					if numWorkers > 16 {
						numWorkers = 16
					}
				}
			}
		}
	}

	// Get application data directory
	appDataDir, err := GetAppDataDir()
	if err != nil {
		fmt.Printf("Warning: Could not create app data directory: %v\n", err)
		// Fallback to project root
		exePath, _ := os.Executable()
		projectRoot := filepath.Dir(exePath)
		appDataDir = filepath.Join(projectRoot, "data")
	}

	// Setup all necessary directories
	dirs, err := SetupDataDirectories(appDataDir)
	if err != nil {
		fmt.Printf("Warning: Could not setup data directories: %v\n", err)
		// Use fallback structure
		dirs = map[string]string{
			"checkpoints": filepath.Join(appDataDir, "checkpoints"),
			"papers":      filepath.Join(appDataDir, "papers"),
			"json":        filepath.Join(appDataDir, "json"),
			"documents":   filepath.Join(appDataDir, "documents"),
			"temp":        filepath.Join(appDataDir, "temp"),
		}
	}

	config := &Config{
		InputDir:     dirs["documents"],
		OutputFile:   filepath.Join(dirs["json"], "ai_knowledge_base.json"),
		NumWorkers:   numWorkers,
		ChunkSize:    150,
		ChunkOverlap: 25,
		OllamaModel:  "nomic-embed-text",
		OllamaHost:   "http://localhost:11434",
		CheckpointDB: filepath.Join(dirs["checkpoints"], "checkpoints.db"),
		BatchSize:    numWorkers,
		AppDataDir:   appDataDir,
		DataDirs:     dirs,

		// arXiv mining defaults
		EnableArxivMining:   false,
		ArxivCategories:     arxiv.GetRecommendedCategories(),
		ArxivMaxPapers:      50,
		ArxivRunInterval:    "24h",
		ArxivDownloadDelay:  2,
		ArxivBackgroundMode: false,
		ArxivSortBy:         "submittedDate",
		ArxivSortOrder:      "descending",

		// Script mode defaults
		ProductionMode: mode == "production",
		TestMode:       mode == "test",
		OptimizedMode:  mode == "optimized",
		HybridMode:     mode == "hybrid",
		NoArxivMode:    false,
		DryRun:         false,
	}

	config.CloudflareEndpoint = os.Getenv("CLOUDFLARE_EMBEDDINGS_WORKER")

	// Define flags
	flag.StringVar(&config.InputDir, "input", config.InputDir, "Directory containing PDF files")
	flag.StringVar(&config.OutputFile, "output", config.OutputFile, "Output JSON file path")
	flag.IntVar(&config.NumWorkers, "workers", config.NumWorkers, "Number of concurrent workers")
	flag.IntVar(&config.ChunkSize, "chunk-size", config.ChunkSize, "Words per chunk")
	flag.IntVar(&config.ChunkOverlap, "chunk-overlap", config.ChunkOverlap, "Words overlap between chunks")
	flag.StringVar(&config.OllamaModel, "model", config.OllamaModel, "Ollama embedding model")
	flag.StringVar(&config.OllamaHost, "host", config.OllamaHost, "Ollama API host")
	flag.StringVar(&config.CheckpointDB, "checkpoint", config.CheckpointDB, "Checkpoint database file")
	flag.IntVar(&config.BatchSize, "batch-size", config.BatchSize, "Batch size for embeddings")

	// arXiv mining flags
	flag.BoolVar(&config.EnableArxivMining, "arxiv-enable", config.EnableArxivMining, "Enable arXiv mining")

	categoriesStr := strings.Join(config.ArxivCategories, ",")
	flag.StringVar(&categoriesStr, "arxiv-categories", categoriesStr, "Comma-separated list of arXiv categories")

	flag.IntVar(&config.ArxivMaxPapers, "arxiv-max-papers", config.ArxivMaxPapers, "Maximum papers to download per category")
	flag.StringVar(&config.ArxivRunInterval, "arxiv-interval", config.ArxivRunInterval, "Run interval for background mode (e.g., 24h, 1h)")
	flag.IntVar(&config.ArxivDownloadDelay, "arxiv-delay", config.ArxivDownloadDelay, "Delay between downloads in seconds")
	flag.BoolVar(&config.ArxivBackgroundMode, "arxiv-background", config.ArxivBackgroundMode, "Run arXiv miner in background mode")
	flag.StringVar(&config.ArxivSortBy, "arxiv-sort-by", config.ArxivSortBy, "Sort by field (relevance, lastUpdatedDate, submittedDate)")
	flag.StringVar(&config.ArxivSortOrder, "arxiv-sort-order", config.ArxivSortOrder, "Sort order (ascending, descending)")

	// Script mode flags
	flag.BoolVar(&config.ProductionMode, "production", config.ProductionMode, "Run in production mode")
	flag.BoolVar(&config.TestMode, "test", config.TestMode, "Run in test mode")
	flag.BoolVar(&config.OptimizedMode, "optimized", config.OptimizedMode, "Run with CPU optimizations")
	flag.BoolVar(&config.HybridMode, "hybrid", config.HybridMode, "Run with hybrid embeddings")
	flag.BoolVar(&config.NoArxivMode, "no-arxiv", config.NoArxivMode, "Skip arXiv mining")
	flag.BoolVar(&config.DryRun, "dry-run", config.DryRun, "Show configuration without running")

	// Environment configuration
	flag.IntVar(&config.CPUOverride, "cpu-override", 0, "Override CPU core detection")
	flag.IntVar(&config.CloudflareLimit, "cloudflare-limit", 0, "Cloudflare daily request limit")
	flag.BoolVar(&config.GPUOverride, "gpu-override", false, "Force GPU usage")
	flag.BoolVar(&config.GPUOptimizations, "gpu-opt", false, "Enable GPU-specific optimizations")

	flag.Parse()

	// Parse categories string
	config.ArxivCategories = strings.Split(categoriesStr, ",")

	// Only parse additional arguments if they're not commands or modes
	if flag.NArg() > 0 {
		arg := flag.Arg(0)
		// Skip if it looks like a command or mode
		if !isCommandOrMode(arg) {
			config.ArxivCategories = strings.Split(arg, ",")
		}
	}

	// Apply script-specific configurations
	applyScriptConfiguration(config, mode)

	return config
}

// detectScriptMode determines which script mode is being used based on flags or environment
func detectScriptMode() string {
	// Check for mode-specific environment variables or arguments
	if os.Getenv("DATAMINER_MODE") != "" {
		return os.Getenv("DATAMINER_MODE")
	}

	// Check command line arguments for mode indicators
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-production":
			return "production"
		case "-test":
			return "test"
		case "-optimized", "--optimized":
			return "optimized"
		case "-hybrid", "--hybrid":
			return "hybrid"
		}
	}

	return "default"
}

// applyScriptConfiguration applies configuration based on the detected script mode
func applyScriptConfiguration(config *Config, mode string) {
	switch mode {
	case "production":
		// Production workflow configuration
		if config.NumWorkers == 8 { // default
			if runtime.GOOS != "windows" {
				if cmd := exec.Command("nproc"); cmd != nil {
					if output, err := cmd.Output(); err == nil {
						if n, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &config.NumWorkers); err == nil && n > 0 {
							if config.NumWorkers > 16 {
								config.NumWorkers = 16
							}
						}
					}
				}
			}
		}
		config.ChunkSize = 150
		config.ChunkOverlap = 25
		config.EnableArxivMining = true
		config.ArxivMaxPapers = 50
		config.ArxivDownloadDelay = 2

	case "test":
		// Test workflow configuration
		config.NumWorkers = 4
		config.ChunkSize = 150
		config.ChunkOverlap = 25
		config.EnableArxivMining = true
		config.ArxivMaxPapers = 5
		config.ArxivDownloadDelay = 1
		config.CloudflareLimit = 100

	case "optimized":
		// Optimized neural processing configuration
		config.ChunkSize = 100
		config.ChunkOverlap = 25
		config.BatchSize = 8
		// Enable GPU optimizations if available
		if config.GPUOverride || isGPUAvailable() {
			config.GPUOptimizations = true
		}

	case "hybrid":
		// Hybrid embeddings configuration
		config.ChunkSize = 100
		config.ChunkOverlap = 25
		config.BatchSize = 4
		// Hybrid mode uses default Cloudflare limit unless overridden
		if config.CloudflareLimit == 0 {
			config.CloudflareLimit = 5000
		}

	default:
		// Default configuration is already set above
	}
}

// isGPUAvailable checks if GPU is available for processing
func isGPUAvailable() bool {
	// Check if nvidia-smi is available and can detect GPU
	if cmd := exec.Command("nvidia-smi"); cmd != nil {
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

// GetScriptSpecificFlags returns mode-specific flag configurations
func GetScriptSpecificFlags(mode string) map[string]interface{} {
	flags := make(map[string]interface{})

	switch mode {
	case "production":
		flags["workers"] = "auto"
		flags["chunk-size"] = 150
		flags["chunk-overlap"] = 25
		flags["arxiv-enable"] = true
		flags["arxiv-max-papers"] = 50
		flags["arxiv-delay"] = 2

	case "test":
		flags["workers"] = 4
		flags["chunk-size"] = 150
		flags["chunk-overlap"] = 25
		flags["arxiv-enable"] = true
		flags["arxiv-max-papers"] = 5
		flags["arxiv-delay"] = 1
		flags["cloudflare-limit"] = 100

	case "optimized":
		flags["chunk-size"] = 100
		flags["chunk-overlap"] = 25
		flags["batch-size"] = 8

	case "hybrid":
		flags["chunk-size"] = 100
		flags["chunk-overlap"] = 25
		flags["batch-size"] = 4
		flags["cloudflare-limit"] = 5000
	}

	return flags
}

// ValidateConfiguration checks if the configuration is valid
func ValidateConfiguration(config *Config) error {
	// Validate arXiv categories
	for _, category := range config.ArxivCategories {
		if !arxiv.ValidateCategory(category) {
			return fmt.Errorf("invalid arXiv category: %s", category)
		}
	}

	// Validate file paths
	if config.InputDir == "" {
		return fmt.Errorf("input directory cannot be empty")
	}

	if config.OutputFile == "" {
		return fmt.Errorf("output file path cannot be empty")
	}

	// Validate numeric values
	if config.NumWorkers <= 0 {
		return fmt.Errorf("number of workers must be positive")
	}

	if config.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive")
	}

	if config.BatchSize <= 0 {
		return fmt.Errorf("batch size must be positive")
	}

	return nil
}

// PrintConfiguration prints the current configuration in a readable format
func PrintConfiguration(config *Config) {
	fmt.Println("📋 Configuration:")
	fmt.Printf("  📂 Input Directory: %s\n", config.InputDir)
	fmt.Printf("  📄 JSON Output: %s\n", config.OutputFile)
	fmt.Printf("  🔧 Workers: %d\n", config.NumWorkers)
	fmt.Printf("  📝 Chunk Size: %d words\n", config.ChunkSize)
	fmt.Printf("  🔄 Chunk Overlap: %d words\n", config.ChunkOverlap)
	fmt.Printf("  🤖 Model: %s\n", config.OllamaModel)
	fmt.Printf("  🌐 Host: %s\n", config.OllamaHost)
	fmt.Printf("  📦 Batch Size: %d\n", config.BatchSize)

	fmt.Println("\n  📚 arXiv Mining:")
	if config.EnableArxivMining {
		fmt.Printf("    ✅ Enabled\n")
		fmt.Printf("    🏷️  Categories: %v\n", config.ArxivCategories)
		fmt.Printf("    📄 Max Papers: %d\n", config.ArxivMaxPapers)
		fmt.Printf("    ⏱️  Delay: %ds\n", config.ArxivDownloadDelay)
		fmt.Printf("    🔄 Background Mode: %t\n", config.ArxivBackgroundMode)
	} else {
		fmt.Printf("    ❌ Disabled\n")
	}

	if config.CloudflareLimit > 0 {
		fmt.Printf("\n  🌐 Cloudflare Daily Limit: %d requests\n", config.CloudflareLimit)
	}

	if config.GPUOptimizations {
		fmt.Printf("\n  🚀 GPU Optimizations: Enabled\n")
	}

	if config.DryRun {
		fmt.Printf("\n  🔍 Dry Run: Enabled\n")
	}
}

// isCommandOrMode checks if a string is a command or mode name
func isCommandOrMode(arg string) bool {
	// Check script commands
	commands := []string{
		"run-workflow", "test-workflow", "run-optimized", "run-hybrid",
		"run", "help", "list-modes", "build", "deps", "clean",
	}
	for _, cmd := range commands {
		if arg == cmd {
			return true
		}
	}

	// Check execution modes
	validModes := []string{
		"production", "test", "optimized", "hybrid",
		"neural", "arxiv", "workflow", "default",
	}
	for _, mode := range validModes {
		if arg == mode {
			return true
		}
	}

	return false
}
