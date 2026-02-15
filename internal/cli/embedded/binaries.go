// internal/cli/embedded/binaries.go
// Package embedded provides embedded hasher binaries that are extracted at runtime
package embedded

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// NOTE: Binaries are optionally embedded at build time.
// Build the binaries first using:
//   make build-host         # For hasher-host (native)
//
// The Makefile should place binaries in: internal/cli/embedded/bin/
// If binaries aren't present, the CLI will look for pre-compiled binaries
// in the app data directory or prompt to compile them.

//go:embed all:bin/*
var embeddedBinaries embed.FS

// BinaryInfo contains metadata about an embedded binary
type BinaryInfo struct {
	Name        string // Binary name (e.g., "hasher-host")
	Description string // Human-readable description
	TargetOS    string // Target OS (linux, darwin, windows)
	TargetArch  string // Target architecture (amd64, mips, arm64)
	Embedded    bool   // Whether binary is embedded
}

// AvailableBinaries lists all binaries that should be embedded
var AvailableBinaries = []BinaryInfo{
	{
		Name:        "hasher-host",
		Description: "Hasher orchestrator for Native OS",
		TargetOS:    "linux",
		TargetArch:  "amd64",
	},
	{
		Name:        "data-miner",
		Description: "Data Miner - Document structuring and PDF processing pipeline",
		TargetOS:    "linux",
		TargetArch:  "amd64",
	},
	{
		Name:        "data-encoder",
		Description: "Data Encoder - Tokenization and embedding generation pipeline",
		TargetOS:    "linux",
		TargetArch:  "amd64",
	},
	{
		Name:        "data-trainer",
		Description: "Data Trainer - Model training and neural network optimization pipeline",
		TargetOS:    "linux",
		TargetArch:  "amd64",
	},
}

// GetAppDataDir returns the OS-specific application data directory
func GetAppDataDir() (string, error) {
	var baseDir string

	switch runtime.GOOS {
	case "linux":
		// XDG_DATA_HOME or ~/.local/share
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			baseDir = xdg
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home directory: %w", err)
			}
			baseDir = filepath.Join(home, ".local", "share")
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(home, "Library", "Application Support")
	case "windows":
		baseDir = os.Getenv("LOCALAPPDATA")
		if baseDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home directory: %w", err)
			}
			baseDir = filepath.Join(home, "AppData", "Local")
		}
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = home
	}

	appDir := filepath.Join(baseDir, "hasher")
	return appDir, nil
}

// GetBinDir returns the directory where binaries should be extracted
func GetBinDir() (string, error) {
	appDir, err := GetAppDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(appDir, "bin"), nil
}

// IsEmbedded checks if a binary is embedded in the CLI
func IsEmbedded(name string) bool {
	_, err := embeddedBinaries.ReadFile(filepath.Join("bin", name))
	return err == nil
}

// GetEmbedded returns the embedded binary content
func GetEmbedded(name string) ([]byte, error) {
	return embeddedBinaries.ReadFile(filepath.Join("bin", name))
}

// ListEmbedded returns a list of all embedded binary names
func ListEmbedded() ([]string, error) {
	var names []string

	entries, err := embeddedBinaries.ReadDir("bin")
	if err != nil {
		// No binaries embedded - this is OK during development
		return names, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}

	return names, nil
}

// ExtractBinary extracts an embedded binary to the bin directory
func ExtractBinary(name string) (string, error) {
	binDir, err := GetBinDir()
	if err != nil {
		return "", fmt.Errorf("failed to get bin directory: %w", err)
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Read embedded binary
	data, err := GetEmbedded(name)
	if err != nil {
		return "", fmt.Errorf("binary %s not embedded: %w", name, err)
	}

	// Write to destination
	destPath := filepath.Join(binDir, name)
	if err := os.WriteFile(destPath, data, 0755); err != nil {
		return "", fmt.Errorf("failed to write binary: %w", err)
	}

	return destPath, nil
}

// ForceExtractBinary extracts the binary even if it already exists
func ForceExtractBinary(name string) (string, error) {
	binDir, err := GetBinDir()
	if err != nil {
		return "", fmt.Errorf("failed to get bin directory: %w", err)
	}

	// Ensure bin directory exists
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Read embedded binary
	data, err := GetEmbedded(name)
	if err != nil {
		return "", fmt.Errorf("binary %s not embedded: %w", name, err)
	}

	// Write to destination (overwrite existing)
	destPath := filepath.Join(binDir, name)
	if err := os.WriteFile(destPath, data, 0755); err != nil {
		return "", fmt.Errorf("failed to write binary: %w", err)
	}

	return destPath, nil
}

// ExtractAll extracts all embedded binaries to the bin directory
func ExtractAll() (map[string]string, error) {
	extracted := make(map[string]string)

	names, err := ListEmbedded()
	if err != nil {
		return extracted, err
	}

	for _, name := range names {
		path, err := ExtractBinary(name)
		if err != nil {
			return extracted, fmt.Errorf("failed to extract %s: %w", name, err)
		}
		extracted[name] = path
	}

	return extracted, nil
}

// GetBinaryPath returns the path to a binary, extracting if necessary
func GetBinaryPath(name string) (string, error) {
	binDir, err := GetBinDir()
	if err != nil {
		return "", err
	}

	destPath := filepath.Join(binDir, name)

	// Check if already extracted
	if info, err := os.Stat(destPath); err == nil {
		// File exists, verify it's executable
		if info.Mode()&0111 != 0 {
			return destPath, nil
		}
		// Make executable
		if err := os.Chmod(destPath, 0755); err != nil {
			return "", fmt.Errorf("failed to make binary executable: %w", err)
		}
		return destPath, nil
	}

	// Not extracted yet, extract now
	return ExtractBinary(name)
}

// GetHasherHostPath returns the path to hasher-host for the current platform
func GetHasherHostPath() (string, error) {
	name := "hasher-host"
	return GetBinaryPath(name)
}

// GetHasherHostPathForce returns the path to hasher-host, always extracting a fresh copy
func GetHasherHostPathForce() (string, error) {
	name := "hasher-host"
	return ForceExtractBinary(name)
}

// CheckEmbeddedBinaries checks which binaries are embedded and returns status
func CheckEmbeddedBinaries() []BinaryInfo {
	result := make([]BinaryInfo, len(AvailableBinaries))
	copy(result, AvailableBinaries)

	for i := range result {
		result[i].Embedded = IsEmbedded(result[i].Name)
	}

	return result
}

// EnsureExtracted ensures all required binaries are extracted
func EnsureExtracted() error {
	binDir, err := GetBinDir()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	names, err := ListEmbedded()
	if err != nil {
		return err
	}

	for _, name := range names {
		destPath := filepath.Join(binDir, name)

		// Check if already exists and is valid
		if info, err := os.Stat(destPath); err == nil {
			// Check if same size as embedded
			embeddedData, _ := GetEmbedded(name)
			if info.Size() == int64(len(embeddedData)) {
				continue // Already extracted
			}
		}

		// Extract
		if _, err := ExtractBinary(name); err != nil {
			return fmt.Errorf("failed to extract %s: %w", name, err)
		}
	}

	return nil
}

// WalkEmbedded walks through all embedded files
func WalkEmbedded(fn func(path string, d fs.DirEntry, err error) error) error {
	return fs.WalkDir(embeddedBinaries, "bin", fn)
}
