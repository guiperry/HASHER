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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"hasher/internal/analyzer"
	"hasher/internal/cli/embedded"
	"hasher/internal/cli/ui"
	"hasher/internal/config"
)

const (
	portFile = "/tmp/hasher-host.port"
)

// CLI configuration flags
var (
	monitorLogs = flag.Bool("monitor-logs", true, "enable server log monitoring")
)

// ServerState holds the hasher-host process state (shared between goroutines)
type ServerState struct {
	Cmd     *exec.Cmd
	Started bool
	Port    int
	Mu      sync.Mutex
}

func (s *ServerState) Get() (*exec.Cmd, bool, int) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.Cmd, s.Started, s.Port
}

func (s *ServerState) Set(cmd *exec.Cmd, started bool, port int) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Cmd = cmd
	s.Started = started
	s.Port = port
}

func main() {
	flag.Parse()

	// Shared server state (accessible from all goroutines)
	serverState := &ServerState{}

	// Initialize embedded binaries
	initEmbeddedBinaries()

	// Set up signal handler for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create log channel for server output
	logChan := make(chan string, 100)

	// Create UI model and pass log channel
	model := ui.NewModel()
	// Set ServerStarting to true immediately so initialization screen shows
	model.ServerStarting = true
	// Send initial message to show we're starting
	model.ServerLogs = append(model.ServerLogs, "Initializing...")

	// Start the Bubble Tea UI with alternate screen and mouse support
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())

	// Handle server shutdown with ASIC cleanup
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal.")
		cleanupASICDevice(model.Deployer)
		cmd, started, port := serverState.Get()
		shutdownHasherHost(cmd, started, port)
		os.Exit(0)
	}()

	// Start log listener and send log messages to program
	go func() {
		for log := range logChan {
			p.Send(ui.AppendLogMsg{Log: log})
		}
	}()

	// Start hasher-host in background - don't block UI
	go func() {
		hasherHostCmd, hasherHostStarted, hasherHostPort := startHasherHost(logChan)
		serverState.Set(hasherHostCmd, hasherHostStarted, hasherHostPort)

		// Update model with server info
		p.Send(ui.ServerCmdMsg{Cmd: hasherHostCmd})
		if hasherHostStarted {
			p.Send(ui.ServerReadyMsg{Ready: true, Starting: false, Port: hasherHostPort})
		}
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

// startHasherHost attempts to start the hasher-host orchestrator
func startHasherHost(logChan chan string) (*exec.Cmd, bool, int) {
	// Force extract a fresh hasher-host binary
	hostPath, err := embedded.GetHasherHostPathForce()
	if err != nil {
		logChan <- fmt.Sprintf("Failed to extract hasher-host: %v", err)
		return nil, false, 8080
	}

	// Get the binary directory for working directory
	binDir, err := embedded.GetBinDir()
	if err != nil {
		logChan <- fmt.Sprintf("Failed to get binary directory: %v", err)
		return nil, false, 8080
	}

	// Check if hasher-host is already running on common ports
	if port := findRunningHasherHost(); port > 0 {
		logChan <- fmt.Sprintf("Found existing hasher-host on port %d.", port)
		return nil, true, port
	}

	logChan <- fmt.Sprintf("Starting hasher-host from %s...", hostPath)

	// Build hasher-host arguments
	var args []string

	// Check if device configuration is available
	deviceConfig, err := config.LoadDeviceConfig()
	if err == nil && deviceConfig.IP != "" {
		// Device is configured, use it
		args = append(args, "--device="+deviceConfig.IP)
		args = append(args, "--discover=false")
		args = append(args, "--force-redeploy=true")
		logChan <- fmt.Sprintf("Using configured device %s (discovery disabled, force-redeploy enabled)", deviceConfig.IP)
	} else {
		// No device configuration, enable discovery for auto-detection
		args = append(args, "--discover=true")
		args = append(args, "--auto-deploy=true")
		logChan <- "No device configuration found - enabling network discovery and auto-deployment"
	}

	// Start hasher-host with configured arguments
	cmd := exec.Command(hostPath, args...)
	cmd.Dir = binDir

	// Create pipes to capture output and forward to the UI
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		logChan <- fmt.Sprintf("Error starting hasher-host: %v", err)
		return nil, false, 8081
	}

	logChan <- fmt.Sprintf("hasher-host started with PID %d", cmd.Process.Pid)

	// Goroutine to forward stdout to the log channel
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdoutPipe.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				logChan <- string(buf[:n])
			}
		}
	}()

	// Goroutine to forward stderr to the log channel
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderrPipe.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				logChan <- string(buf[:n])
			}
		}
	}()

	// --- NEW LOGIC: Wait for port file and health check ---
	startTime := time.Now()
	timeout := 5 * time.Minute // Extended timeout for server deployment
	var actualPort int

	for time.Since(startTime) < timeout {
		portBytes, err := os.ReadFile(portFile)
		if err == nil {
			port, err := strconv.Atoi(strings.TrimSpace(string(portBytes)))
			if err == nil {
				actualPort = port
				// Now that we have a port, check the health endpoint
				if isHasherHostRunning(actualPort) {
					logChan <- fmt.Sprintf("hasher-host is ready on port %d!", actualPort)
					return cmd, true, actualPort
				}
			}
		}
		// Wait a bit before retrying
		time.Sleep(500 * time.Millisecond)
	}

	logChan <- "Error: hasher-host startup timed out. Continuing without orchestrator."

	// Attempt to kill the process we started, since it's not healthy
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
	return cmd, false, 8080 // default port
}

// findRunningHasherHost checks if hasher-host is already running on any port and returns the port
func findRunningHasherHost() int {
	// Common ports to check
	ports := []int{8080, 8081, 8082, 8083, 8084, 8085, 8008, 9000}
	client := &http.Client{Timeout: 2 * time.Second}

	for _, port := range ports {
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/health", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return port
			}
		}
	}
	return 0
}

// isHasherHostRunning checks if hasher-host API is responding on a specific port
func isHasherHostRunning(port int) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/health", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
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
