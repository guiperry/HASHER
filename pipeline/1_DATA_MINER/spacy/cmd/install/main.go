// Package main provides an automatic installer for go-spacy that can be called during go get
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	fmt.Println("🚀 Go-Spacy Automatic Installer")
	fmt.Printf("📋 Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	// Get the directory where this program is running from
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}

	// Find the root directory of the go-spacy project
	rootDir := findProjectRoot(dir)
	if rootDir == "" {
		fmt.Printf("❌ Could not find go-spacy project root\n")
		os.Exit(1)
	}

	fmt.Printf("📁 Project root: %s\n", rootDir)

	// Change to project root
	if err := os.Chdir(rootDir); err != nil {
		fmt.Printf("❌ Failed to change to project root: %v\n", err)
		os.Exit(1)
	}

	// Check if library already exists
	libPath := getLibraryPath()
	if _, err := os.Stat(libPath); err == nil {
		fmt.Printf("✅ Library already exists: %s\n", libPath)
		fmt.Printf("💡 Use 'go generate' to rebuild if needed\n")
		return
	}

	fmt.Printf("🔨 Building C++ wrapper library...\n")

	// Try to run the automatic build
	if err := runAutomaticBuild(); err != nil {
		fmt.Printf("❌ Automatic build failed: %v\n", err)
		fmt.Printf("💡 Manual build instructions:\n")
		printManualInstructions()
		os.Exit(1)
	}

	fmt.Printf("✅ Build completed successfully!\n")
	fmt.Printf("📦 Library: %s\n", libPath)
}

// findProjectRoot searches up the directory tree for go.mod or other project indicators
func findProjectRoot(start string) string {
	dir := start
	for {
		// Check for go.mod
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		// Check for cpp directory
		if _, err := os.Stat(filepath.Join(dir, "cpp")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "cpp", "spacy_wrapper.cpp")); err == nil {
				return dir
			}
		}

		// Check for spacy.go
		if _, err := os.Stat(filepath.Join(dir, "spacy.go")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}
	return ""
}

// getLibraryPath returns the expected path for the built library
func getLibraryPath() string {
	var libName string
	switch runtime.GOOS {
	case "windows":
		libName = "libspacy_wrapper.dll"
	case "darwin":
		libName = "libspacy_wrapper.dylib"
	default:
		libName = "libspacy_wrapper.so"
	}
	return filepath.Join("lib", libName)
}

// runAutomaticBuild attempts to build the library automatically
func runAutomaticBuild() error {
	// Try different build methods in order of preference

	// Method 1: Use build.go if available
	if _, err := os.Stat("build.go"); err == nil {
		fmt.Printf("🔧 Using build.go...\n")
		cmd := exec.Command("go", "run", "build.go")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
		fmt.Printf("⚠️  build.go failed, trying other methods...\n")
	}

	// Method 2: Use platform-specific script
	script := getInstallScript()
	if script != "" {
		if _, err := os.Stat(script); err == nil {
			fmt.Printf("🔧 Using install script: %s...\n", script)
			if err := runScript(script); err == nil {
				return nil
			}
			fmt.Printf("⚠️  Install script failed, trying other methods...\n")
		}
	}

	// Method 3: Use Makefile if available
	if _, err := os.Stat("Makefile"); err == nil {
		fmt.Printf("🔧 Using Makefile...\n")
		if runtime.GOOS == "windows" {
			// Try mingw32-make first, then make
			for _, makeCmd := range []string{"mingw32-make", "make"} {
				cmd := exec.Command(makeCmd, "lib") /* #nosec G204 -- makeCmd is from a fixed whitelist */
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err == nil {
					return nil
				}
			}
		} else {
			cmd := exec.Command("make", "lib")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		fmt.Printf("⚠️  Makefile build failed\n")
	}

	// Method 4: Try go generate
	fmt.Printf("🔧 Trying go generate...\n")
	cmd := exec.Command("go", "generate", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		return nil
	}

	return fmt.Errorf("all build methods failed")
}

// getInstallScript returns the appropriate install script for the platform
func getInstallScript() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join("scripts", "install.ps1")
	default:
		return filepath.Join("scripts", "install.sh")
	}
}

// runScript executes the platform-appropriate install script
func runScript(script string) error {
	switch runtime.GOOS {
	case "windows":
		// Try PowerShell first, then fallback
		for _, shell := range []string{"powershell", "pwsh"} {
			cmd := exec.Command(shell, "-ExecutionPolicy", "Bypass", "-File", script) /* #nosec G204 -- shell is from a fixed whitelist */
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		return fmt.Errorf("failed to run PowerShell script")
	default:
		// Unix-like systems
		cmd := exec.Command("bash", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
}

// printManualInstructions shows manual build instructions
func printManualInstructions() {
	fmt.Printf("\n📋 Manual Build Instructions:\n")
	fmt.Printf("=============================\n\n")

	switch runtime.GOOS {
	case "windows":
		fmt.Printf("Windows:\n")
		fmt.Printf("1. Install build tools:\n")
		fmt.Printf("   - Install MinGW-w64 or Visual Studio Build Tools\n")
		fmt.Printf("   - Install Python 3.7+ with development headers\n")
		fmt.Printf("2. Install Python dependencies:\n")
		fmt.Printf("   python -m pip install spacy\n")
		fmt.Printf("   python -m spacy download en_core_web_sm\n")
		fmt.Printf("3. Run the install script:\n")
		fmt.Printf("   powershell -ExecutionPolicy Bypass -File scripts\\install.ps1\n")
		fmt.Printf("4. Or build manually:\n")
		fmt.Printf("   make lib\n")

	case "darwin":
		fmt.Printf("macOS:\n")
		fmt.Printf("1. Install Xcode command line tools:\n")
		fmt.Printf("   xcode-select --install\n")
		fmt.Printf("2. Install dependencies:\n")
		fmt.Printf("   brew install python3 pkg-config\n")
		fmt.Printf("   pip3 install spacy\n")
		fmt.Printf("   python3 -m spacy download en_core_web_sm\n")
		fmt.Printf("3. Run the install script:\n")
		fmt.Printf("   bash scripts/install.sh\n")
		fmt.Printf("4. Or build manually:\n")
		fmt.Printf("   make lib\n")

	default:
		fmt.Printf("Linux:\n")
		fmt.Printf("1. Install build tools:\n")
		fmt.Printf("   sudo apt install build-essential python3-dev pkg-config  # Ubuntu/Debian\n")
		fmt.Printf("   sudo yum groupinstall \"Development Tools\" python3-devel pkgconfig  # CentOS/RHEL\n")
		fmt.Printf("2. Install Python dependencies:\n")
		fmt.Printf("   pip3 install --user spacy\n")
		fmt.Printf("   python3 -m spacy download en_core_web_sm\n")
		fmt.Printf("3. Run the install script:\n")
		fmt.Printf("   bash scripts/install.sh\n")
		fmt.Printf("4. Or build manually:\n")
		fmt.Printf("   make lib\n")
	}

	fmt.Printf("\n📖 For more information, see:\n")
	fmt.Printf("   - README.md\n")
	fmt.Printf("   - docs/INSTALLATION.md\n")
	fmt.Printf("   - https://github.com/am-sokolov/go-spacy\n")
}
