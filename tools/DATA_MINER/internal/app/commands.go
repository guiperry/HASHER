package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ScriptCommands maps script names to their Go implementations
var ScriptCommands = map[string]func() error{
	"run-workflow":  RunProductionMode,
	"test-workflow": RunTestMode,
	"run-optimized": RunOptimizedMode,
	"run-hybrid":    RunHybridMode,
	"run":           RunDefaultMode,
	"help":          ShowUsage,
	"list-modes":    func() error { ListAvailableModes(); return nil },
	"build":         BuildProject,
	"deps":          CheckDependencies,
	"clean":         CleanProject,
	"kill":          KillDataminer,
}

// RunScriptCommand executes a script command based on the provided name
func RunScriptCommand(command string) error {
	if handler, exists := ScriptCommands[command]; exists {
		return handler()
	}

	// Check if it's a valid execution mode
	if isValidMode(command) {
		return RunScriptMode(command, nil)
	}

	return fmt.Errorf("unknown command or mode: %s", command)
}

// RunDefaultMode runs the default neural processing mode (equivalent to run.sh)
func RunDefaultMode() error {
	fmt.Println("📊 Data Miner - Document Structuring Engine")
	fmt.Println("===========================================")

	// Set environment for default mode
	os.Setenv("DATAMINER_MODE", "neural")

	config := ParseFlags()

	// Validate dependencies
	if err := ValidateDependencies(); err != nil {
		return err
	}

	// Check if input directory exists
	if _, err := os.Stat(config.InputDir); os.IsNotExist(err) {
		fmt.Printf("❌ Input directory '%s' does not exist\n", config.InputDir)
		fmt.Printf("   Creating directory for you...\n")
		if err := os.MkdirAll(config.InputDir, 0755); err != nil {
			return fmt.Errorf("failed to create input directory: %w", err)
		}
		fmt.Printf("💡 Place PDF files in '%s' and run again\n", config.InputDir)
		return nil
	}

	// Count PDF files
	pdfCount, err := CountFiles(config.InputDir)
	if err != nil {
		return fmt.Errorf("failed to count PDF files: %w", err)
	}

	if pdfCount == 0 {
		fmt.Printf("❌ No PDF files found in '%s'\n", config.InputDir)
		fmt.Printf("   Please add some PDF files to process\n")
		return nil
	}

	fmt.Printf("📄 Found %d PDF files to process\n", pdfCount)
	printRunConfiguration(config)

	// Get project root for build check
	projectRoot, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to determine project root: %w", err)
	}

	// Build if necessary
	if err := ensureBuilt(projectRoot); err != nil {
		return err
	}

	return RunApplication(config)
}

// BuildProject builds the Go project
func BuildProject() error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to determine project root: %w", err)
	}

	fmt.Println("🔨 Building Data Miner...")

	cmd := exec.Command("go", "build", "-o", filepath.Join(projectRoot, "dataminer"), "./cmd/dataminer")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Println("✅ Build completed successfully!")
	return nil
}

// CheckDependencies validates all required dependencies
func CheckDependencies() error {
	fmt.Println("🔍 Checking dependencies...")

	if err := ValidateDependencies(); err != nil {
		fmt.Printf("❌ %v\n", err)
		return err
	}

	// Check if project has necessary directories
	projectRoot, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to determine project root: %w", err)
	}

	dirs := []string{
		filepath.Join(projectRoot, "data", "documents"),
		filepath.Join(projectRoot, "data", "json"),
		filepath.Join(projectRoot, "data", "checkpoints"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	fmt.Println("✅ Dependencies checked - Ollama check will be handled by the application")
	return nil
}

// CleanProject cleans build artifacts and temporary files
func CleanProject() error {
	fmt.Println("🧹 Cleaning project...")

	projectRoot, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to determine project root: %w", err)
	}

	// Remove binary
	binaryPath := filepath.Join(projectRoot, "dataminer")
	if _, err := os.Stat(binaryPath); err == nil {
		if err := os.Remove(binaryPath); err != nil {
			return fmt.Errorf("failed to remove binary: %w", err)
		}
		fmt.Println("🗑️  Removed binary")
	}

	// Remove temporary files
	tempFiles := []string{
		"/tmp/ollama_start.log",
		"/tmp/ollama_optimized.log",
		"/tmp/ollama_hybrid.log",
	}

	for _, tempFile := range tempFiles {
		if _, err := os.Stat(tempFile); err == nil {
			if err := os.Remove(tempFile); err != nil {
				fmt.Printf("Warning: failed to remove %s: %v\n", tempFile, err)
			} else {
				fmt.Printf("🗑️  Removed %s\n", tempFile)
			}
		}
	}

	fmt.Println("✅ Clean completed!")
	return nil
}

// ShowUsage displays usage information
func ShowUsage() error {
	fmt.Println("Data Miner - Document Structuring Engine")
	fmt.Println("=========================================")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  dataminer [command|mode] [flags]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  run-workflow    Run production workflow")
	fmt.Println("  test-workflow   Run test workflow")
	fmt.Println("  run-optimized   Run with optimizations")
	fmt.Println("  run-hybrid      Run with hybrid embeddings")
	fmt.Println("  run             Run default neural processing")
	fmt.Println("  build           Build the project")
	fmt.Println("  deps            Check dependencies")
	fmt.Println("  clean           Clean build artifacts")
	fmt.Println("  kill            Kill all running dataminer processes")
	fmt.Println("  help            Show this help")
	fmt.Println("  list-modes      List available modes")
	fmt.Println("")
	fmt.Println("Execution Modes:")
	fmt.Println("  production      Production workflow mode")
	fmt.Println("  test            Test mode with limited scope")
	fmt.Println("  optimized       CPU/GPU optimized mode")
	fmt.Println("  hybrid          Hybrid embeddings mode")
	fmt.Println("  neural          Neural processing only")
	fmt.Println("  arxiv           ArXiv mining only")
	fmt.Println("  workflow        Integrated workflow")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  -input <dir>              Input directory (default: ./data/documents)")
	fmt.Println("  -output <file>            Output file (default: ./data/json/ai_knowledge_base.json)")
	fmt.Println("  -workers <num>            Number of workers (default: auto-detect)")
	fmt.Println("  -chunk-size <num>         Text chunk size (default: 150)")
	fmt.Println("  -chunk-overlap <num>      Chunk overlap (default: 25)")
	fmt.Println("  -model <name>             Ollama model (default: nomic-embed-text)")
	fmt.Println("  -host <url>               Ollama host (default: http://localhost:11434)")
	fmt.Println("  -batch-size <num>         Embedding batch size (default: workers)")
	fmt.Println("")
	fmt.Println("ArXiv Flags:")
	fmt.Println("  -arxiv-enable             Enable arXiv mining")
	fmt.Println("  -arxiv-categories <list>   Comma-separated categories")
	fmt.Println("  -arxiv-max-papers <num>   Max papers per category")
	fmt.Println("  -arxiv-delay <seconds>    Download delay")
	fmt.Println("  -arxiv-background         Run in background mode")
	fmt.Println("")
	fmt.Println("Script Mode Flags:")
	fmt.Println("  -production                Production mode")
	fmt.Println("  -test                      Test mode")
	fmt.Println("  -optimized                 Optimized mode")
	fmt.Println("  -hybrid                    Hybrid mode")
	fmt.Println("  -no-arxiv                  Skip arXiv mining")
	fmt.Println("  -dry-run                   Show configuration only")
	fmt.Println("")
	fmt.Println("Environment Variables:")
	fmt.Println("  DATAMINER_MODE             Set execution mode")
	fmt.Println("  CLOUDFLARE_EMBEDDINGS_URL  Cloudflare embeddings URL")
	fmt.Println("  CLOUDFLARE_DAILY_LIMIT     Cloudflare daily request limit")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  dataminer run-workflow                    # Production workflow")
	fmt.Println("  dataminer test-workflow                   # Test workflow")
	fmt.Println("  dataminer -production -arxiv-enable      # Production with arXiv")
	fmt.Println("  dataminer -input /path/to/pdfs -output out.json  # Custom paths")
	fmt.Println("  DATAMINER_MODE=production dataminer      # Environment mode")

	return nil
}

// Helper functions

func getProjectRoot() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return ".", err
	}

	exeDir := filepath.Dir(exePath)
	// If running from cmd/dataminer directory, project root is parent
	if filepath.Base(exeDir) == "dataminer" && filepath.Base(filepath.Dir(exeDir)) == "cmd" {
		return filepath.Dir(filepath.Dir(exeDir)), nil
	}
	return exeDir, nil
}

func getDefaultWorkers() int {
	if runtime.GOOS != "windows" {
		if cmd := exec.Command("nproc"); cmd != nil {
			if output, err := cmd.Output(); err == nil {
				var workers int
				if n, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &workers); err == nil && n > 0 {
					// Cap at reasonable limit
					if workers > 16 {
						workers = 16
					}
					return workers
				}
			}
		}
	}
	return 4 // fallback
}

func ensureBuilt(projectRoot string) error {
	binaryPath := filepath.Join(projectRoot, "dataminer")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		fmt.Println("🔨 Dataminer binary not found, building...")
		return BuildProject()
	}
	return nil
}

func printRunConfiguration(config *Config) {
	fmt.Println("🔧 Configuration:")
	fmt.Printf("   Input Directory: %s\n", config.InputDir)
	fmt.Printf("   Output File: %s\n", config.OutputFile)
	fmt.Printf("   Workers: %d\n", config.NumWorkers)
	fmt.Printf("   Chunk Size: %d words\n", config.ChunkSize)
	fmt.Printf("   Chunk Overlap: %d words\n", config.ChunkOverlap)
	fmt.Printf("   Model: %s\n", config.OllamaModel)
	fmt.Printf("   Host: %s\n", config.OllamaHost)
	fmt.Printf("   Batch Size: %d\n", config.BatchSize)
	fmt.Println("")
}

func isValidMode(mode string) bool {
	validModes := []string{
		"production", "test", "optimized", "hybrid",
		"neural", "arxiv", "workflow", "default",
	}
	for _, m := range validModes {
		if m == mode {
			return true
		}
	}
	return false
}

// KillDataminer kills all running dataminer processes
func KillDataminer() error {
	fmt.Println("🛑 Killing all dataminer processes...")

	// Get the project root to find the kill script
	projectRoot, err := getProjectRoot()
	if err != nil {
		projectRoot = "."
	}

	killScript := filepath.Join(projectRoot, "scripts", "kill_dataminer.sh")

	// Check if the script exists
	if _, err := os.Stat(killScript); os.IsNotExist(err) {
		// Fallback: try to kill directly
		fmt.Println("⚠️  Kill script not found, using fallback method...")
		return killDataminerFallback()
	}

	// Run the kill script
	cmd := exec.Command("bash", killScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run kill script: %w", err)
	}

	return nil
}

// killDataminerFallback kills dataminer processes directly without the script
func killDataminerFallback() error {
	// Try graceful shutdown first
	fmt.Println("🛑 Sending SIGTERM to dataminer processes...")
	exec.Command("pkill", "-TERM", "-f", "dataminer").Run()

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Check if any are still running
	cmd := exec.Command("pgrep", "-f", "dataminer")
	output, _ := cmd.Output()

	if len(output) > 0 {
		fmt.Println("⚠️  Processes still running, sending SIGKILL...")
		exec.Command("pkill", "-KILL", "-f", "dataminer").Run()
	}

	fmt.Println("✅ Done")
	return nil
}
