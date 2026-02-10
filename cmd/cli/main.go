// Hasher: Neural Inference Engine Powered by SHA-256 ASICs
// Copyright (C) 2026  Guillermo Perry
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"hasher/internal/analyzer"
	"hasher/internal/cli/embedded"
	"hasher/internal/cli/ui"
)

// CLI configuration flags
var (
	monitorLogs = flag.Bool("monitor-logs", true, "enable server log monitoring")
)

func main() {
	flag.Parse()

	// Initialize embedded binaries
	initEmbeddedBinaries()

	// Set up signal handler for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create UI model
	model := ui.NewModel()

	// Check if hasher-host is already running and update model state
	model.CheckExistingHasherHost()

	// Start the Bubble Tea UI with alternate screen and mouse support
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())

	// Handle server shutdown with ASIC cleanup
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal.")
		cleanupASICDevice(model.Deployer)
		// Shutdown hasher-host if it was started via the UI
		if model.ServerCmd != nil && model.ServerCmd.Process != nil {
			shutdownHasherHost(model.ServerCmd, true, 8080)
		}
		os.Exit(0)
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		cleanupASICDevice(model.Deployer)
		os.Exit(1)
	}

	// Ensure cleanup when exiting normally
	cleanupASICDevice(model.Deployer)
}

// initEmbeddedBinaries extracts embedded binaries to app data directory
func initEmbeddedBinaries() {
	binDir, err := embedded.GetBinDir()
	if err != nil {
		fmt.Printf("Warning: Could not determine binary directory: %v\n", err)
		return
	}

	// Check for embedded binaries
	binaries := embedded.CheckEmbeddedBinaries()
	embeddedCount := 0
	for _, b := range binaries {
		if b.Embedded {
			embeddedCount++
		}
	}

	if embeddedCount > 0 {
		if err := embedded.EnsureExtracted(); err != nil {
			// Silently continue - will show error in UI if needed
		}
	}

	// Copy .env file to bin directory if found
	copyEnvFileToBinDir(binDir)
}

// copyEnvFileToBinDir copies the .env file to the binary directory
func copyEnvFileToBinDir(binDir string) {
	// Find .env file (check CWD first, then walk up)
	envPath := findEnvFile()
	if envPath == "" {
		// Try looking in the project directory (where the CLI is being run from)
		if cwd, err := os.Getwd(); err == nil {
			envPath = filepath.Join(cwd, ".env")
			if _, err := os.Stat(envPath); err != nil {
				envPath = ""
			}
		}
	}

	if envPath == "" {
		// Silently continue - will show error in UI if needed
		return
	}

	// Copy to bin directory
	destPath := filepath.Join(binDir, ".env")
	srcData, err := os.ReadFile(envPath)
	if err != nil {
		return
	}

	_ = os.WriteFile(destPath, srcData, 0644)
}

// findEnvFile finds .env file in CWD or parent directories
func findEnvFile() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Check CWD first
	envPath := filepath.Join(cwd, ".env")
	if _, err := os.Stat(envPath); err == nil {
		return envPath
	}

	// Walk up looking for go.mod and .env
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			envPath = filepath.Join(cwd, ".env")
			if _, err := os.Stat(envPath); err == nil {
				return envPath
			}
			break
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}

	return ""
}

// cleanupASICDevice removes deployed binaries from the ASIC device
func cleanupASICDevice(deployer *analyzer.Deployer) {
	if deployer == nil {
		return
	}

	device := deployer.GetActiveDevice()
	if device == nil {
		return
	}

	fmt.Printf("Cleaning up binaries from ASIC device %s...\n", device.IPAddress)

	// Try to connect and cleanup
	if err := deployer.Connect(); err != nil {
		fmt.Printf("Could not connect to device for cleanup: %v\n", err)
		return
	}
	defer deployer.Disconnect()

	// Cleanup removes hasher-server and any temporary files
	if err := deployer.Cleanup(); err != nil {
		fmt.Printf("Cleanup warning: %v\n", err)
	} else {
		fmt.Println("ASIC device cleanup complete.")
	}
}

// shutdownHasherHost gracefully shuts down the hasher-host process
func shutdownHasherHost(cmd *exec.Cmd, started bool, port int) {
	if !started {
		return
	}

	fmt.Println("Shutting down hasher-host...")

	// If we started the process, we can wait for it to exit.
	if cmd != nil && cmd.Process != nil {
		// Attempt graceful shutdown via API. Fire and forget is okay.
		shutdownURL := fmt.Sprintf("http://localhost:%d/api/v1/shutdown", port)
		http.Post(shutdownURL, "application/json", nil)

		// Wait for process to terminate with timeout
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-done:
			fmt.Println("hasher-host shut down successfully.")
		case <-time.After(15 * time.Second): // Increased timeout
			fmt.Println("hasher-host shutdown timeout, force killing...")
			cmd.Process.Kill()
		}
	} else if port > 0 { // It was running, but we didn't start it. We can still ask it to shut down.
		shutdownURL := fmt.Sprintf("http://localhost:%d/api/v1/shutdown", port)
		resp, err := http.Post(shutdownURL, "application/json", nil)
		if err == nil {
			resp.Body.Close()
			fmt.Println("Shutdown request sent to pre-existing hasher-host.")
		}
	}
}
