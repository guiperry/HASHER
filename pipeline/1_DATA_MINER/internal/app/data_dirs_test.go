package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetAppDataDir(t *testing.T) {
	appDir, err := GetAppDataDir()
	if err != nil {
		t.Fatalf("Failed to get app data directory: %v", err)
	}

	// Check that the directory was created
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Errorf("App data directory should exist: %s", appDir)
	}

	// Check that the path contains 'dataminer'
	if !containsSubstring(appDir, "dataminer") {
		t.Errorf("App data directory should contain 'dataminer': %s", appDir)
	}

	t.Logf("App data directory: %s", appDir)
}

func TestSetupDataDirectories(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	appDir := filepath.Join(tempDir, "dataminer")

	dirs, err := SetupDataDirectories(appDir)
	if err != nil {
		t.Fatalf("Failed to setup data directories: %v", err)
	}

	// Check that all expected directories are created
	expectedDirs := []string{"checkpoints", "papers", "json", "documents", "temp", "backup"}
	for _, dirName := range expectedDirs {
		dirPath, exists := dirs[dirName]
		if !exists {
			t.Errorf("Directory '%s' should be in the returned map", dirName)
			continue
		}

		// Check that the directory exists
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("Directory should exist: %s", dirPath)
		}

		// Special check: json should be under backup
		if dirName == "json" && !containsSubstring(dirPath, "backup") {
			t.Errorf("JSON directory should be under backup: %s", dirPath)
		}
	}

	// Verify json is in backup subdirectory
	jsonPath := dirs["json"]
	expectedJSONPath := filepath.Join(appDir, "backup", "json")
	if jsonPath != expectedJSONPath {
		t.Errorf("JSON path mismatch: expected %s, got %s", expectedJSONPath, jsonPath)
	}
}

func TestSetupDataDirectoriesJSONBackupLocation(t *testing.T) {
	tempDir := t.TempDir()
	appDir := filepath.Join(tempDir, "testapp")

	dirs, err := SetupDataDirectories(appDir)
	if err != nil {
		t.Fatalf("Failed to setup data directories: %v", err)
	}

	// Verify the json directory is in backup/json
	jsonDir := dirs["json"]
	expectedJSONDir := filepath.Join(appDir, "backup", "json")

	if jsonDir != expectedJSONDir {
		t.Errorf("JSON directory should be at %s, got %s", expectedJSONDir, jsonDir)
	}

	// Verify it exists
	if info, err := os.Stat(jsonDir); err != nil {
		t.Errorf("JSON directory should exist: %v", err)
	} else if !info.IsDir() {
		t.Errorf("JSON path should be a directory")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestOSSpecificPaths(t *testing.T) {
	tempDir := t.TempDir()

	// Test based on OS
	switch runtime.GOOS {
	case "windows":
		// On Windows, should use APPDATA or USERPROFILE
		os.Setenv("APPDATA", tempDir)
		appDir, err := GetAppDataDir()
		if err != nil {
			t.Fatalf("Failed to get app data dir on Windows: %v", err)
		}
		if !containsSubstring(appDir, tempDir) {
			t.Errorf("Windows app dir should use APPDATA: %s", appDir)
		}

	case "darwin":
		// On macOS, should use ~/Library/Application Support
		os.Setenv("HOME", tempDir)
		appDir, err := GetAppDataDir()
		if err != nil {
			t.Fatalf("Failed to get app data dir on macOS: %v", err)
		}
		expectedPath := filepath.Join(tempDir, "Library", "Application Support", "dataminer")
		if appDir != expectedPath {
			t.Errorf("macOS app dir mismatch: expected %s, got %s", expectedPath, appDir)
		}

	default: // linux
		// On Linux, should use XDG_DATA_HOME or ~/.local/share
		os.Setenv("HOME", tempDir)
		os.Setenv("XDG_DATA_HOME", "")
		appDir, err := GetAppDataDir()
		if err != nil {
			t.Fatalf("Failed to get app data dir on Linux: %v", err)
		}
		expectedPath := filepath.Join(tempDir, ".local", "share", "dataminer")
		if appDir != expectedPath {
			t.Errorf("Linux app dir mismatch: expected %s, got %s", expectedPath, appDir)
		}
	}
}

func TestSetupDataDirectoriesCreatesAllDirs(t *testing.T) {
	tempDir := t.TempDir()
	appDir := filepath.Join(tempDir, "freshapp")

	// Ensure the directory doesn't exist yet
	os.RemoveAll(appDir)

	dirs, err := SetupDataDirectories(appDir)
	if err != nil {
		t.Fatalf("Failed to setup directories: %v", err)
	}

	// Verify all directories were created
	for name, path := range dirs {
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Directory %s (%s) should exist: %v", name, path, err)
		} else if !info.IsDir() {
			t.Errorf("Path %s (%s) should be a directory", name, path)
		}
	}
}
