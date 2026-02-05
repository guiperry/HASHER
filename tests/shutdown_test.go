package tests

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func testShutdown() bool {
	fmt.Println("=== Hasher CLI Shutdown Test ===")

	// Start application
	fmt.Println("\nStarting hasher CLI application...")
	cmd := exec.Command("./hasher-cli")
	err := cmd.Start()
	if err != nil {
		fmt.Printf("❌ Error starting application: %v\n", err)
		return false
	}

	// Give application time to start
	fmt.Println("Waiting for hasher CLI to start...")
	time.Sleep(3 * time.Second)

	// Test shutdown by sending SIGINT
	fmt.Println("\nTesting shutdown with SIGINT...")
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		fmt.Printf("❌ Error sending SIGINT: %v\n", err)
		cmd.Process.Kill()
		return false
	}

	// Give application time to shutdown
	time.Sleep(3 * time.Second)

	// Check if application is still running
	if isProcessRunning(cmd.Process.Pid) {
		fmt.Println("❌ Application failed to shut down")
		// Force kill application
		cmd.Process.Kill()
		time.Sleep(1 * time.Second)
		if isProcessRunning(cmd.Process.Pid) {
			fmt.Println("❌ Failed to force kill application")
		} else {
			fmt.Println("⚠️ Application force killed")
		}
		return false
	} else {
		fmt.Println("✅ Hasher CLI shut down successfully")
		return true
	}
}

func isProcessRunning(pid int) bool {
	// Check if process is still running by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 (doesn't actually kill the process)
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
