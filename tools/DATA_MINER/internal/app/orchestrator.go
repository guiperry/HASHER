package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Orchestrator coordinates the continuous workflow with graceful shutdown
type Orchestrator struct {
	config       *Config
	statsManager *StatsManager
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator(config *Config) (*Orchestrator, error) {
	// Get app data directory for stats
	appDataDir, err := GetAppDataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get app data directory: %w", err)
	}

	// Create stats manager
	statsManager, err := NewStatsManager(appDataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create stats manager: %w", err)
	}

	return &Orchestrator{
		config:       config,
		statsManager: statsManager,
	}, nil
}

// Run executes the workflow with graceful shutdown handling
func (o *Orchestrator) Run() error {
	// Ensure stats are saved on exit
	defer o.statsManager.Close()

	// Print initial status
	o.statsManager.PrintInitialStatus()

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run workflow in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- o.runWorkflow(ctx)
	}()

	// Wait for completion or interruption
	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("workflow failed: %w", err)
		}
		return nil

	case sig := <-sigChan:
		fmt.Printf("\n⚠️  Received signal %v, initiating graceful shutdown...\n", sig)

		// Save stats immediately before cancelling
		fmt.Println("💾 Saving statistics...")
		if err := o.statsManager.Save(); err != nil {
			fmt.Printf("❌ Warning: failed to save stats: %v\n", err)
		} else {
			fmt.Println("✅ Statistics saved successfully")
		}

		cancel()

		// Wait for graceful shutdown
		select {
		case err := <-errChan:
			if err != nil {
				fmt.Printf("Workflow shut down with error: %v\n", err)
			} else {
				fmt.Println("✅ Workflow shut down gracefully")
			}
		case <-time.After(30 * time.Second):
			fmt.Println("❌ Shutdown timeout, forcing exit")
		}

		return fmt.Errorf("workflow interrupted by signal %v", sig)
	}
}

// runWorkflow executes the workflow
func (o *Orchestrator) runWorkflow(ctx context.Context) error {
	// Enable optimized mode by default if not explicitly disabled
	if o.config.GPUOptimizations {
		ConfigureOllamaEnvironment(true, o.config.GPUOverride, o.config.NumWorkers)
	}

	// Continuous loop is now the default behavior
	// Run the continuous workflow that loops until quota is hit
	return RunContinuousWorkflow(ctx, o.config, o.statsManager)
}

// RunOrchestrator is a convenience function to run the orchestrator
func RunOrchestrator(config *Config) error {
	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		return err
	}
	return orchestrator.Run()
}
