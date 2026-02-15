//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	projectName = "go-spacy"
	version     = "1.0.0"
)

var (
	buildDir   = "build"
	libDir     = "lib"
	cppSrcFile = filepath.Join("cpp", "spacy_wrapper.cpp")
	headerFile = filepath.Join("include", "spacy_wrapper.h")
)

// BuildConfig holds platform-specific build configuration
type BuildConfig struct {
	OS              string
	Arch            string
	SharedExt       string
	ObjectExt       string
	CC              string
	CXX             string
	PythonConfig    string
	LibraryName     string
	LibraryPath     string
	ObjectFile      string
	InstallNameFlag string
	PkgConfigCmd    []string
}

func main() {
	fmt.Printf("🚀 Starting automatic build for %s v%s\n", projectName, version)
	fmt.Printf("📋 Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	config, err := detectPlatform()
	if err != nil {
		fatal("Failed to detect platform configuration: %v", err)
	}

	if err := validateDependencies(config); err != nil {
		fatal("Dependency validation failed: %v", err)
	}

	if err := setupDirectories(); err != nil {
		fatal("Failed to setup directories: %v", err)
	}

	if err := buildCppWrapper(config); err != nil {
		fatal("Failed to build C++ wrapper: %v", err)
	}

	if err := validateBuild(config); err != nil {
		fatal("Build validation failed: %v", err)
	}

	fmt.Printf("✅ Build completed successfully!\n")
	fmt.Printf("📦 Library: %s\n", config.LibraryPath)
}

func detectPlatform() (*BuildConfig, error) {
	config := &BuildConfig{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	switch runtime.GOOS {
	case "linux":
		config.SharedExt = "so"
		config.ObjectExt = "o"
		                config.CC = detectCompiler([]string{"gcc", "clang", "cc"})
		                config.CXX = detectCompiler([]string{"g++", "clang++", "c++"})
		                config.InstallNameFlag = "" // Linux doesn't need install name
		                config.PkgConfigCmd = []string{"pkg-config", "python3-embed"}
		
		        case "darwin":
		
		config.SharedExt = "dylib"
		config.ObjectExt = "o"
		config.CC = detectCompiler([]string{"clang", "gcc", "cc"})
		config.CXX = detectCompiler([]string{"clang++", "g++", "c++"})
		config.InstallNameFlag = "-install_name"
		config.PkgConfigCmd = []string{"pkg-config", "python3"}

	case "windows":
		config.SharedExt = "dll"
		config.ObjectExt = "obj"
		config.CC = detectCompiler([]string{"gcc", "clang", "cl"})
		config.CXX = detectCompiler([]string{"g++", "clang++", "cl"})
		config.InstallNameFlag = "" // Windows doesn't need install name
		config.PkgConfigCmd = []string{"pkg-config", "python3"}

	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	config.LibraryName = fmt.Sprintf("libspacy_wrapper.%s", config.SharedExt)
	config.LibraryPath = filepath.Join(libDir, config.LibraryName)
	config.ObjectFile = filepath.Join(buildDir, fmt.Sprintf("spacy_wrapper.%s", config.ObjectExt))

	// Detect Python configuration
	config.PythonConfig = detectPythonConfig()

	return config, nil
}

func detectCompiler(compilers []string) string {
	for _, compiler := range compilers {
		if _, err := exec.LookPath(compiler); err == nil {
			return compiler
		}
	}
	return compilers[0] // Fallback to first option
}

func detectPythonConfig() string {
	// On macOS, prefer python-config to avoid version mismatches
	if runtime.GOOS == "darwin" {
		pythonConfigs := []string{"python3-config", "python-config"}
		for _, cmd := range pythonConfigs {
			if _, err := exec.LookPath(cmd); err == nil {
				return cmd
			}
		}
	}

	// Try pkg-config (most portable on Linux)
	if _, err := exec.LookPath("pkg-config"); err == nil {
		if exec.Command("pkg-config", "--exists", "python3-embed").Run() == nil {
			return "pkg-config"
		}
		if exec.Command("pkg-config", "--exists", "python3").Run() == nil {
			return "pkg-config"
		}
	}

	// Fallback to python-config
	pythonConfigs := []string{"python3-config", "python-config"}
	for _, cmd := range pythonConfigs {
		if _, err := exec.LookPath(cmd); err == nil {
			return cmd
		}
	}

	return "python3-config" // Default fallback
}

func validateDependencies(config *BuildConfig) error {
	fmt.Printf("🔍 Validating dependencies...\n")

	// Check C++ compiler
	if _, err := exec.LookPath(config.CXX); err != nil {
		return fmt.Errorf("C++ compiler not found: %s. Please install build tools", config.CXX)
	}
	fmt.Printf("✓ C++ compiler: %s\n", config.CXX)

	// Check Python
	pythonCmd := "python3"
	if runtime.GOOS == "windows" {
		pythonCmd = "python"
	}

	if _, err := exec.LookPath(pythonCmd); err != nil {
		return fmt.Errorf("Python not found. Please install Python 3.7+")
	}

	// Check Python version
	out, err := exec.Command(pythonCmd, "--version").Output()
	if err != nil {
		return fmt.Errorf("failed to get Python version: %v", err)
	}
	fmt.Printf("✓ Python: %s", strings.TrimSpace(string(out)))

	// Check Spacy installation
	err = exec.Command(pythonCmd, "-c", "import spacy; print(f'Spacy {spacy.__version__}')").Run()
	if err != nil {
		return fmt.Errorf("Spacy not installed. Please run: pip install spacy")
	}
	fmt.Printf("✓ Spacy is installed\n")

	// Check for required Spacy model
	err = exec.Command(pythonCmd, "-c", "import spacy; spacy.load('en_core_web_sm')").Run()
	if err != nil {
		fmt.Printf("⚠️  English model not found. Attempting to download...\n")
		downloadCmd := exec.Command(pythonCmd, "-m", "spacy", "download", "en_core_web_sm")
		downloadCmd.Stdout = os.Stdout
		downloadCmd.Stderr = os.Stderr
		if err := downloadCmd.Run(); err != nil {
			return fmt.Errorf("failed to download Spacy model. Please run: python -m spacy download en_core_web_sm")
		}
		fmt.Printf("✓ English model downloaded successfully\n")
	} else {
		fmt.Printf("✓ English model available\n")
	}

	// Check Python configuration
	if err := validatePythonConfig(config); err != nil {
		return fmt.Errorf("Python configuration error: %v", err)
	}

	return nil
}

func validatePythonConfig(config *BuildConfig) error {
	var cmd *exec.Cmd

	if config.PythonConfig == "pkg-config" {
		// Test pkg-config
		cmd = exec.Command("pkg-config", "--cflags", "python3")
	} else {
		// Test python-config
		cmd = exec.Command(config.PythonConfig, "--cflags")
	}

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get Python compile flags using %s: %v", config.PythonConfig, err)
	}

	if len(strings.TrimSpace(string(out))) == 0 {
		return fmt.Errorf("empty Python compile flags from %s", config.PythonConfig)
	}

	fmt.Printf("✓ Python configuration: %s\n", config.PythonConfig)
	return nil
}

func setupDirectories() error {
	fmt.Printf("📁 Setting up directories...\n")

	dirs := []string{buildDir, libDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	fmt.Printf("✓ Directories created: %s\n", strings.Join(dirs, ", "))
	return nil
}

func buildCppWrapper(config *BuildConfig) error {
	fmt.Printf("🔨 Building C++ wrapper...\n")

	// Get Python flags
	pythonCFlags, err := getPythonFlags(config, "--cflags")
	if err != nil {
		return fmt.Errorf("failed to get Python compile flags: %v", err)
	}

	pythonLdFlags, err := getPythonFlags(config, "--libs")
	if err != nil {
		// Try --ldflags for python-config
		pythonLdFlags, err = getPythonFlags(config, "--ldflags")
		if err != nil {
			return fmt.Errorf("failed to get Python link flags: %v", err)
		}
	}

	// Compile C++ source to object file
	compileArgs := []string{
		"-Wall", "-Wextra", "-fPIC", "-std=c++17",
		"-I" + filepath.Join("include"),
		"-O3", "-DNDEBUG",
	}

	// Add architecture-specific flags
	if runtime.GOARCH == "amd64" {
		compileArgs = append(compileArgs, "-march=x86-64")
	} else if runtime.GOARCH == "arm64" && runtime.GOOS == "darwin" {
		// Use default for Apple Silicon
	} else if runtime.GOARCH == "arm64" {
		compileArgs = append(compileArgs, "-march=armv8-a")
	}

	// Add Python flags
	compileArgs = append(compileArgs, strings.Fields(pythonCFlags)...)
	compileArgs = append(compileArgs, "-c", cppSrcFile, "-o", config.ObjectFile)

	fmt.Printf("🔧 Compiling: %s %s\n", config.CXX, strings.Join(compileArgs, " "))
	compileCmd := exec.Command(config.CXX, compileArgs...)
	compileCmd.Stdout = os.Stdout
	compileCmd.Stderr = os.Stderr
	if err := compileCmd.Run(); err != nil {
		return fmt.Errorf("compilation failed: %v", err)
	}

	// Link shared library
	linkArgs := []string{"-shared", "-o", config.LibraryPath, config.ObjectFile}

	// Add Python link flags
	linkArgs = append(linkArgs, strings.Fields(pythonLdFlags)...)

	// Add explicit Python library on macOS if not already present
	if runtime.GOOS == "darwin" && !strings.Contains(pythonLdFlags, "-lpython") {
		// Use runtime Python version to ensure compatibility
		pythonVersion, err := exec.Command("python3", "-c", "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')").Output()
		if err == nil && len(pythonVersion) > 0 {
			pythonLibVersion := strings.TrimSpace(string(pythonVersion))
			if pythonLibVersion != "" {
				linkArgs = append(linkArgs, "-lpython"+pythonLibVersion)
			}
		}
	}

	// Add platform-specific flags
	if config.InstallNameFlag != "" {
		linkArgs = append(linkArgs, config.InstallNameFlag, "@rpath/"+config.LibraryName)
	}

	// Windows-specific flags
	if runtime.GOOS == "windows" {
		linkArgs = append(linkArgs, "-Wl,--out-implib,"+config.LibraryPath+".a")
	}

	fmt.Printf("🔗 Linking: %s %s\n", config.CXX, strings.Join(linkArgs, " "))
	linkCmd := exec.Command(config.CXX, linkArgs...)
	linkCmd.Stdout = os.Stdout
	linkCmd.Stderr = os.Stderr
	if err := linkCmd.Run(); err != nil {
		return fmt.Errorf("linking failed: %v", err)
	}

	// Fix install name on macOS for local testing
	if runtime.GOOS == "darwin" {
		absPath, _ := filepath.Abs(config.LibraryPath)
		installNameCmd := exec.Command("install_name_tool", "-id", absPath, config.LibraryPath)
		installNameCmd.Run() // Don't fail if this doesn't work
	}

	return nil
}

func getPythonFlags(config *BuildConfig, flagType string) (string, error) {
	var cmd *exec.Cmd

	// On macOS, prefer python-config to avoid version mismatches
	if runtime.GOOS == "darwin" {
		pythonConfigCmd := detectPythonConfig()
		if pythonConfigCmd != "pkg-config" {
			if flagType == "--libs" {
				cmd = exec.Command(pythonConfigCmd, "--ldflags")
			} else {
				cmd = exec.Command(pythonConfigCmd, flagType)
			}
		} else {
			// Fallback to pkg-config if python-config not available
			if flagType == "--cflags" {
				cmd = exec.Command("pkg-config", "--cflags", "python3-embed")
			} else if flagType == "--libs" {
				cmd = exec.Command("pkg-config", "--libs", "python3-embed")
			} else {
				return "", fmt.Errorf("unsupported flag type for pkg-config: %s", flagType)
			}
		}
	} else {
		// On other platforms, use the configured method
		if config.PythonConfig == "pkg-config" {
			if flagType == "--cflags" {
				cmd = exec.Command("pkg-config", "--cflags", "python3-embed")
			} else if flagType == "--libs" {
				// pkg-config often doesn't provide complete linking info, fallback to python-config
				pythonConfigCmd := detectPythonConfig()
				if pythonConfigCmd != "pkg-config" {
					if flagType == "--libs" {
						cmd = exec.Command(pythonConfigCmd, "--ldflags")
					} else {
						cmd = exec.Command(pythonConfigCmd, flagType)
					}
				} else {
					cmd = exec.Command("pkg-config", "--libs", "python3-embed")
				}
			} else {
				return "", fmt.Errorf("unsupported flag type for pkg-config: %s", flagType)
			}
		} else {
			// For python-config, --libs might not exist, use --ldflags
			if flagType == "--libs" {
				cmd = exec.Command(config.PythonConfig, "--ldflags")
			} else {
				cmd = exec.Command(config.PythonConfig, flagType)
			}
		}
	}

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func validateBuild(config *BuildConfig) error {
	fmt.Printf("✅ Validating build...\n")

	// Check if library file exists
	if _, err := os.Stat(config.LibraryPath); err != nil {
		return fmt.Errorf("library file not found: %s", config.LibraryPath)
	}

	// Check file size (should be reasonable, not empty)
	info, err := os.Stat(config.LibraryPath)
	if err != nil {
		return fmt.Errorf("failed to get library file info: %v", err)
	}

	if info.Size() < 1000 { // Less than 1KB is suspicious
		return fmt.Errorf("library file too small (%d bytes), build may have failed", info.Size())
	}

	fmt.Printf("✓ Library built successfully: %s (%.1f KB)\n",
		config.LibraryPath, float64(info.Size())/1024)

	return nil
}

func fatal(format string, args ...interface{}) {
	fmt.Printf("❌ "+format+"\n", args...)
	os.Exit(1)
}

func askUserConfirmation(question string) bool {
	fmt.Printf("%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
