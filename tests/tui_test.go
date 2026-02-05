package tests

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestTUICompiles verifies the code compiles without errors
func TestTUICompiles(t *testing.T) {
	// Change to root directory
	originalDir, err := os.Getwd()
	assert.NoError(t, err)
	err = os.Chdir("..")
	assert.NoError(t, err)
	defer os.Chdir(originalDir)

	cmd := exec.Command("go", "build", "./cmd/cli")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err = cmd.Start()
	assert.NoError(t, err)

	// Give it time to compile
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err = <-done:
		// Cleanup the executable
		os.Remove("hasher")
		assert.NoError(t, err, "Compilation failed: %s", errOut.String())
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		os.Remove("hasher")
		t.Fatal("Compilation timed out")
	}
}

// TestUIContent checks if the UI contains the expected elements when initialized
func TestUIContent(t *testing.T) {
	t.Skip("Skipping UI test because it requires a TTY")

	// Skip this test if running in a CI environment or if there's no existing config
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)

	// Try to find existing installation to test with
	var configPath string
	if strings.HasPrefix(homeDir, "/home/") {
		configPath = homeDir + "/.hasher-config"
	} else {
		// Skip test if we can't find typical config location
		t.Skip("Could not find typical config location")
	}

	// Check if we have an existing installation
	_, err = os.Stat(configPath)
	if err != nil {
		t.Skip("No existing config found, skipping UI test")
	}

	// Change to root directory
	originalDir, err := os.Getwd()
	assert.NoError(t, err)
	err = os.Chdir("..")
	assert.NoError(t, err)
	defer os.Chdir(originalDir)

	// Run the TUI and capture output
	cmd := exec.Command("go", "run", "./cmd/cli")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err = cmd.Start()
	assert.NoError(t, err)

	// Let it run for a moment
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		t.Log("Output:", out.String())
		t.Log("Errors:", errOut.String())
		// Check if UI contains expected elements
		assert.Contains(t, out.String(), "Hasher CLI Tool", "UI should display title")
		assert.Contains(t, out.String(), "Type your message here", "UI should display input placeholder")
		assert.Contains(t, out.String(), "Logs will appear here", "UI should display log section")
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
	}
}
