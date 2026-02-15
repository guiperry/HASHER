# Go-Spacy Automatic Installation Script for Windows
# Supports Windows 10/11 with PowerShell 5.1+

param(
    [switch]$Force,
    [switch]$NoUserInstall,
    [string]$PythonVersion = "3.9"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

# Constants
$PROJECT_NAME = "go-spacy"
$VERSION = "1.0.0"

# Colors for output (if supported)
$colors = @{
    Red = "Red"
    Green = "Green"
    Yellow = "Yellow"
    Blue = "Blue"
    White = "White"
}

function Write-ColorOutput {
    param([string]$Message, [string]$Color = "White")

    if ($Host.UI.RawUI.BackgroundColor -ne $null) {
        Write-Host $Message -ForegroundColor $colors[$Color]
    } else {
        Write-Output $Message
    }
}

function Write-Info {
    param([string]$Message)
    Write-ColorOutput "ℹ️  $Message" "Blue"
}

function Write-Success {
    param([string]$Message)
    Write-ColorOutput "✅ $Message" "Green"
}

function Write-Warning {
    param([string]$Message)
    Write-ColorOutput "⚠️  $Message" "Yellow"
}

function Write-Error {
    param([string]$Message)
    Write-ColorOutput "❌ $Message" "Red"
}

function Test-Command {
    param([string]$Command)

    try {
        if (Get-Command $Command -ErrorAction SilentlyContinue) {
            return $true
        }
        return $false
    } catch {
        return $false
    }
}

function Test-IsAdmin {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Install-Chocolatey {
    if (!(Test-Command "choco")) {
        Write-Info "Installing Chocolatey package manager..."
        Set-ExecutionPolicy Bypass -Scope Process -Force
        [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
        iex ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))

        # Refresh environment
        $env:PATH = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

        if (Test-Command "choco") {
            Write-Success "Chocolatey installed successfully"
        } else {
            throw "Chocolatey installation failed"
        }
    } else {
        Write-Success "Chocolatey is already installed"
    }
}

function Install-BuildTools {
    Write-Info "Installing build tools..."

    # Check for existing C++ compiler
    $hasCompiler = $false

    if (Test-Command "cl") {
        Write-Success "Found MSVC compiler (cl.exe)"
        $global:CXX_COMPILER = "cl"
        $hasCompiler = $true
    } elseif (Test-Command "g++") {
        Write-Success "Found GCC compiler (g++.exe)"
        $global:CXX_COMPILER = "g++"
        $hasCompiler = $true
    } elseif (Test-Command "clang++") {
        Write-Success "Found Clang compiler (clang++.exe)"
        $global:CXX_COMPILER = "clang++"
        $hasCompiler = $true
    }

    if (!$hasCompiler -or $Force) {
        if (Test-IsAdmin) {
            Write-Info "Installing MinGW-w64 via Chocolatey..."
            choco install -y mingw
        } else {
            Write-Warning "No administrator privileges. Attempting user-level installation..."

            # Try to install MinGW-w64 manually
            $mingwUrl = "https://github.com/niXman/mingw-builds-binaries/releases/download/12.2.0-rt_v10-rev2/x86_64-12.2.0-release-posix-seh-ucrt-rt_v10-rev2.7z"
            $mingwZip = "$env:TEMP\mingw.7z"
            $mingwDir = "$env:LOCALAPPDATA\mingw64"

            Write-Info "Downloading MinGW-w64..."
            Invoke-WebRequest -Uri $mingwUrl -OutFile $mingwZip

            # Extract using Windows built-in extraction (requires Windows 10+)
            Write-Info "Extracting MinGW-w64..."
            if (Test-Command "7z") {
                & 7z x "$mingwZip" -o"$env:LOCALAPPDATA" -y
            } else {
                # Fallback: try with PowerShell 5.1+ Expand-Archive
                try {
                    Expand-Archive -Path $mingwZip -DestinationPath $env:LOCALAPPDATA -Force
                } catch {
                    Write-Warning "Could not extract MinGW-w64 automatically. Please extract manually to $mingwDir"
                    return
                }
            }

            # Add to PATH
            $newPath = "$mingwDir\bin"
            if (Test-Path $newPath) {
                $env:PATH = "$newPath;$env:PATH"
                [Environment]::SetEnvironmentVariable("PATH", "$newPath;$([Environment]::GetEnvironmentVariable('PATH', 'User'))", "User")
                Write-Success "MinGW-w64 installed to $mingwDir"
            }
        }

        # Refresh PATH and check again
        $env:PATH = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

        if (Test-Command "g++") {
            $global:CXX_COMPILER = "g++"
            Write-Success "C++ compiler installed successfully"
        } else {
            throw "Failed to install C++ compiler"
        }
    }
}

function Install-Python {
    Write-Info "Checking Python installation..."

    $pythonCommands = @("python", "python3", "py")
    $pythonFound = $false

    foreach ($cmd in $pythonCommands) {
        if (Test-Command $cmd) {
            try {
                $version = & $cmd --version 2>&1
                if ($version -match "Python 3\.\d+") {
                    $global:PYTHON_CMD = $cmd
                    Write-Success "Found Python: $version"
                    $pythonFound = $true
                    break
                }
            } catch {
                continue
            }
        }
    }

    if (!$pythonFound -or $Force) {
        Write-Info "Installing Python..."

        if (Test-IsAdmin) {
            choco install -y python3 --params "/InstallDir:C:\Python3"
        } else {
            # Download and install Python from python.org
            $pythonUrl = "https://www.python.org/ftp/python/3.11.7/python-3.11.7-amd64.exe"
            $pythonInstaller = "$env:TEMP\python-installer.exe"

            Write-Info "Downloading Python installer..."
            Invoke-WebRequest -Uri $pythonUrl -OutFile $pythonInstaller

            Write-Info "Installing Python (user-level installation)..."
            Start-Process -FilePath $pythonInstaller -ArgumentList "/quiet", "InstallAllUsers=0", "PrependPath=1", "Include_test=0" -Wait

            Remove-Item $pythonInstaller -Force
        }

        # Refresh environment
        $env:PATH = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

        # Check installation
        if (Test-Command "python") {
            $global:PYTHON_CMD = "python"
            Write-Success "Python installed successfully"
        } elseif (Test-Command "py") {
            $global:PYTHON_CMD = "py"
            Write-Success "Python installed successfully"
        } else {
            throw "Python installation failed"
        }
    }

    # Verify Python version
    $pythonVersion = & $global:PYTHON_CMD --version 2>&1
    if ($pythonVersion -notmatch "Python 3\.[789]|Python 3\.1[0-9]") {
        Write-Warning "Python version may be too old: $pythonVersion"
        Write-Warning "Python 3.7+ is recommended"
    }
}

function Install-PythonPackages {
    Write-Info "Installing Python packages..."

    # Upgrade pip and install setuptools/wheel for Python 3.12+
    Write-Info "Upgrading pip and installing build tools..."
    if ($env:GITHUB_ACTIONS -or $env:CI) {
        & $global:PYTHON_CMD -m pip install --upgrade pip setuptools wheel
    } else {
        & $global:PYTHON_CMD -m pip install --user --upgrade pip setuptools wheel
    }

    # Install spacy
    Write-Info "Installing spacy..."
    if ($env:GITHUB_ACTIONS -or $env:CI) {
        & $global:PYTHON_CMD -m pip install spacy
    } else {
        & $global:PYTHON_CMD -m pip install --user spacy
    }

    # Download language model
    Write-Info "Downloading English language model..."
    & $global:PYTHON_CMD -m spacy download en_core_web_sm

    # Verify installation
    Write-Info "Verifying spacy installation..."
    $spacyTest = & $global:PYTHON_CMD -c "import spacy; nlp = spacy.load('en_core_web_sm'); print('✅ Spacy verification successful')" 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Success "Spacy installation verified"
    } else {
        throw "Spacy verification failed: $spacyTest"
    }
}

function Get-PythonConfig {
    Write-Info "Detecting Python configuration..."

    # Try to get Python include directory
    $pythonInclude = & $global:PYTHON_CMD -c "import sysconfig; print(sysconfig.get_path('include'))" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to get Python include directory"
    }

    # Try to get Python library directory
    $pythonLibDir = & $global:PYTHON_CMD -c "import sysconfig; print(sysconfig.get_config_var('LIBDIR'))" 2>&1
    if ($LASTEXITCODE -ne 0) {
        # Fallback method
        $pythonLibDir = & $global:PYTHON_CMD -c "import sys, os; print(os.path.join(sys.prefix, 'libs'))" 2>&1
    }

    # Get Python version for library name
    $pythonVer = & $global:PYTHON_CMD -c "import sys; print(f'{sys.version_info.major}{sys.version_info.minor}')" 2>&1

    $global:PYTHON_INCLUDE = $pythonInclude.Trim()
    $global:PYTHON_LIB_DIR = $pythonLibDir.Trim()
    $global:PYTHON_LIB = "python$pythonVer"

    Write-Success "Python include: $global:PYTHON_INCLUDE"
    Write-Success "Python lib dir: $global:PYTHON_LIB_DIR"
    Write-Success "Python library: $global:PYTHON_LIB"
}

function Build-CppWrapper {
    Write-Info "Building C++ wrapper..."

    # Create directories
    New-Item -ItemType Directory -Force -Path "build" | Out-Null
    New-Item -ItemType Directory -Force -Path "lib" | Out-Null

    $sourceFile = "cpp\spacy_wrapper.cpp"
    $objectFile = "build\spacy_wrapper.obj"
    $libraryFile = "lib\libspacy_wrapper.dll"

    if (!(Test-Path $sourceFile)) {
        throw "Source file not found: $sourceFile"
    }

    # Compilation flags
    $compileArgs = @(
        "-Wall", "-Wextra", "-fPIC", "-std=c++17",
        "-Iinclude",
        "-I`"$global:PYTHON_INCLUDE`"",
        "-O3", "-DNDEBUG",
        "-c", $sourceFile,
        "-o", $objectFile
    )

    Write-Info "Compiling C++ source..."
    Write-Info "Command: $global:CXX_COMPILER $($compileArgs -join ' ')"

    $compileProcess = Start-Process -FilePath $global:CXX_COMPILER -ArgumentList $compileArgs -NoNewWindow -Wait -PassThru -RedirectStandardError "build_error.txt"

    if ($compileProcess.ExitCode -ne 0) {
        $errorOutput = Get-Content "build_error.txt" -Raw
        throw "Compilation failed:`n$errorOutput"
    }

    # Linking flags
    $linkArgs = @(
        "-shared",
        "-o", $libraryFile,
        $objectFile,
        "-L`"$global:PYTHON_LIB_DIR`"",
        "-l$global:PYTHON_LIB",
        "-Wl,--out-implib,lib\libspacy_wrapper.dll.a"
    )

    Write-Info "Linking shared library..."
    Write-Info "Command: $global:CXX_COMPILER $($linkArgs -join ' ')"

    $linkProcess = Start-Process -FilePath $global:CXX_COMPILER -ArgumentList $linkArgs -NoNewWindow -Wait -PassThru -RedirectStandardError "link_error.txt"

    if ($linkProcess.ExitCode -ne 0) {
        $errorOutput = Get-Content "link_error.txt" -Raw
        throw "Linking failed:`n$errorOutput"
    }

    # Verify build
    if (!(Test-Path $libraryFile)) {
        throw "Failed to build shared library: $libraryFile"
    }

    $libSize = (Get-Item $libraryFile).Length
    if ($libSize -lt 1000) {
        throw "Library file too small ($libSize bytes), build failed"
    }

    Write-Success "Library built successfully: $libraryFile ($([math]::Round($libSize/1024, 1)) KB)"

    # Clean up error files
    Remove-Item "build_error.txt", "link_error.txt" -Force -ErrorAction SilentlyContinue
}

function Test-Build {
    Write-Info "Testing build..."

    # Add library to PATH
    $libPath = Join-Path (Get-Location) "lib"
    $env:PATH = "$libPath;$env:PATH"

    # Test Python integration
    $testScript = @"
import spacy
try:
    nlp = spacy.load('en_core_web_sm')
    doc = nlp('Hello world')
    print('✅ Python integration test passed')
except Exception as e:
    print(f'❌ Python integration test failed: {e}')
    exit(1)
"@

    $testResult = & $global:PYTHON_CMD -c $testScript 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Success "Build test completed successfully"
    } else {
        Write-Warning "Build test failed: $testResult"
    }
}

function Main {
    Write-Output ""
    Write-ColorOutput "🚀 Go-Spacy Automatic Installation for Windows" "Blue"
    Write-Output "=============================================="
    Write-Output "Version: $VERSION"
    Write-Output "Platform: Windows $(if ([Environment]::Is64BitOperatingSystem) { "x64" } else { "x86" })"
    Write-Output ""

    try {
        # Check if we need admin privileges for some operations
        if (!(Test-IsAdmin) -and !$NoUserInstall) {
            Write-Warning "Running without administrator privileges. Some features may require manual installation."
            Write-Info "Use -NoUserInstall to suppress this warning or run as administrator for full automation."
        }

        # Install Chocolatey if we have admin rights
        if ((Test-IsAdmin) -and !(Test-Command "choco")) {
            Install-Chocolatey
        }

        Install-BuildTools
        Install-Python
        Install-PythonPackages
        Get-PythonConfig
        Build-CppWrapper
        Test-Build

        Write-Output ""
        Write-Success "🎉 Installation completed successfully!"
        Write-Output ""
        Write-Output "You can now use go-spacy in your Go projects:"
        Write-Output "  go get github.com/am-sokolov/go-spacy"
        Write-Output ""
        Write-Output "Library path for runtime:"
        $libPath = Join-Path (Get-Location) "lib"
        Write-Output "  `$env:PATH = `"$libPath;`$env:PATH`""

    } catch {
        Write-Error "Installation failed: $($_.Exception.Message)"
        Write-Output ""
        Write-Output "Troubleshooting steps:"
        Write-Output "1. Run PowerShell as Administrator"
        Write-Output "2. Enable script execution: Set-ExecutionPolicy RemoteSigned -Scope CurrentUser"
        Write-Output "3. Install dependencies manually if needed"
        Write-Output "4. Check Windows version compatibility (Windows 10+ recommended)"
        exit 1
    }
}

# Run main function
Main