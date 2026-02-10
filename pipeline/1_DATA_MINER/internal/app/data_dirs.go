package app

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetAppDataDir returns the OS-specific application data directory
func GetAppDataDir() (string, error) {
	var basePath string

	switch runtime.GOOS {
	case "windows":
		// %APPDATA% on Windows
		if appData := os.Getenv("APPDATA"); appData != "" {
			basePath = appData
		} else {
			// Fallback to user profile
			if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
				basePath = filepath.Join(userProfile, "AppData", "Roaming")
			} else {
				// Ultimate fallback
				basePath = filepath.Join(os.TempDir(), "hasher", "data")
			}
		}
	case "darwin":
		// ~/Library/Application Support on macOS
		if home := os.Getenv("HOME"); home != "" {
			basePath = filepath.Join(home, "Library", "Application Support")
		} else {
			basePath = os.TempDir()
		}
	default: // linux, unix, etc.
		// ~/.local/share on XDG Base Directory Specification
		if home := os.Getenv("HOME"); home != "" {
			if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
				basePath = xdgData
			} else {
				basePath = filepath.Join(home, ".local", "share")
			}
		} else {
			basePath = os.TempDir()
		}
	}

	appDir := filepath.Join(basePath, "hasher", "data")

	// Ensure the directory exists
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	return appDir, nil
}

// SetupDataDirectories creates all necessary data directories within the app data directory
func SetupDataDirectories(appDataDir string) (map[string]string, error) {
	dirs := map[string]string{
		"checkpoints": filepath.Join(appDataDir, "checkpoints"),
		"papers":      filepath.Join(appDataDir, "papers"),
		"json":        filepath.Join(appDataDir, "json"),
		"documents":   filepath.Join(appDataDir, "documents"),
		"temp":        filepath.Join(appDataDir, "temp"),
		"backup":      filepath.Join(appDataDir, "backup"),
	}

	// Create all directories
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	return dirs, nil
}
