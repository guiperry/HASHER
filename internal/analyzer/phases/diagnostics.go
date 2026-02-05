// Package phases contains phase-specific programs that are compiled for MIPS
// and deployed to ASIC devices for diagnostics.
//
// These programs are designed to be:
// - Small (under 1MB when compiled for MIPS)
// - Self-contained (minimal dependencies)
// - Output JSON or structured text for easy parsing
//
// Build for MIPS:
//
//	GOOS=linux GOARCH=mips GOMIPS=softfloat go build -o diagnostics-mips ./internal/analyzer/phases/diagnostics.go
package phases

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DiagnosticResult represents the output of a diagnostic phase
type DiagnosticResult struct {
	Phase     string                 `json:"phase"`
	Timestamp string                 `json:"timestamp"`
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data"`
	Errors    []string               `json:"errors,omitempty"`
}

// SystemInfo gathers system information
func SystemInfo() DiagnosticResult {
	result := DiagnosticResult{
		Phase:     "system_info",
		Timestamp: time.Now().Format(time.RFC3339),
		Success:   true,
		Data:      make(map[string]interface{}),
	}

	// CPU Info
	if output, err := runCmd("cat", "/proc/cpuinfo"); err == nil {
		cpuInfo := make(map[string]string)
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, ":") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if key == "system type" || key == "cpu model" || key == "BogoMIPS" {
						cpuInfo[key] = value
					}
				}
			}
		}
		result.Data["cpu"] = cpuInfo
	} else {
		result.Errors = append(result.Errors, fmt.Sprintf("cpu info: %v", err))
	}

	// Memory Info
	if output, err := runCmd("cat", "/proc/meminfo"); err == nil {
		memInfo := make(map[string]string)
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "MemTotal") ||
				strings.HasPrefix(line, "MemFree") ||
				strings.HasPrefix(line, "MemAvailable") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					memInfo[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
		}
		result.Data["memory"] = memInfo
	}

	// Kernel
	if output, err := runCmd("uname", "-a"); err == nil {
		result.Data["kernel"] = strings.TrimSpace(output)
	}

	// Architecture
	if output, err := runCmd("uname", "-m"); err == nil {
		result.Data["arch"] = strings.TrimSpace(output)
	}

	// Uptime
	if output, err := runCmd("cat", "/proc/uptime"); err == nil {
		result.Data["uptime"] = strings.TrimSpace(output)
	}

	return result
}

// DeviceInfo gathers ASIC device information
func DeviceInfo() DiagnosticResult {
	result := DiagnosticResult{
		Phase:     "device_info",
		Timestamp: time.Now().Format(time.RFC3339),
		Success:   true,
		Data:      make(map[string]interface{}),
	}

	devicePath := "/dev/bitmain-asic"

	// Check device file
	info, err := os.Stat(devicePath)
	if err != nil {
		result.Data["device_exists"] = false
		result.Errors = append(result.Errors, fmt.Sprintf("device stat: %v", err))
	} else {
		result.Data["device_exists"] = true
		result.Data["device_mode"] = info.Mode().String()
		result.Data["device_size"] = info.Size()
	}

	// USB devices
	if output, err := runCmd("lsusb"); err == nil {
		result.Data["usb_devices"] = strings.Split(strings.TrimSpace(output), "\n")
	}

	// Check for Bitmain USB device
	if output, err := runCmd("lsusb"); err == nil {
		result.Data["bitmain_usb_found"] = strings.Contains(output, "4254:4153")
	}

	// Kernel modules
	if output, err := runCmd("lsmod"); err == nil {
		var bitmainModules []string
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(strings.ToLower(line), "bitmain") {
				bitmainModules = append(bitmainModules, strings.TrimSpace(line))
			}
		}
		result.Data["bitmain_modules"] = bitmainModules
	}

	// Sysfs interface
	sysfsPath := "/sys/class/misc/bitmain-asic"
	if _, err := os.Stat(sysfsPath); err == nil {
		result.Data["sysfs_exists"] = true
		if output, err := runCmd("ls", "-la", sysfsPath); err == nil {
			result.Data["sysfs_contents"] = strings.Split(strings.TrimSpace(output), "\n")
		}
	} else {
		result.Data["sysfs_exists"] = false
	}

	return result
}

// ProcessInfo gathers mining process information
func ProcessInfo() DiagnosticResult {
	result := DiagnosticResult{
		Phase:     "process_info",
		Timestamp: time.Now().Format(time.RFC3339),
		Success:   true,
		Data:      make(map[string]interface{}),
	}

	// Check CGMiner status
	if output, err := runCmd("pgrep", "cgminer"); err == nil {
		result.Data["cgminer_running"] = true
		result.Data["cgminer_pid"] = strings.TrimSpace(output)
	} else {
		result.Data["cgminer_running"] = false
	}

	// Check BMMiner status
	if output, err := runCmd("pgrep", "bmminer"); err == nil {
		result.Data["bmminer_running"] = true
		result.Data["bmminer_pid"] = strings.TrimSpace(output)
	} else {
		result.Data["bmminer_running"] = false
	}

	// CGMiner binary info
	cgminerPath := "/usr/bin/cgminer"
	if _, err := os.Stat(cgminerPath); err == nil {
		result.Data["cgminer_exists"] = true
		if output, err := runCmd("ls", "-lh", cgminerPath); err == nil {
			result.Data["cgminer_info"] = strings.TrimSpace(output)
		}
	} else {
		result.Data["cgminer_exists"] = false
	}

	// Running processes
	if output, err := runCmd("ps", "w"); err == nil {
		var miningProcs []string
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(strings.ToLower(line), "miner") ||
				strings.Contains(strings.ToLower(line), "cgminer") ||
				strings.Contains(strings.ToLower(line), "bmminer") {
				miningProcs = append(miningProcs, strings.TrimSpace(line))
			}
		}
		result.Data["mining_processes"] = miningProcs
	}

	return result
}

// ProtocolInfo gathers protocol-related information
func ProtocolInfo() DiagnosticResult {
	result := DiagnosticResult{
		Phase:     "protocol_info",
		Timestamp: time.Now().Format(time.RFC3339),
		Success:   true,
		Data:      make(map[string]interface{}),
	}

	// Firmware version
	if output, err := runCmd("cat", "/usr/bin/compile_time"); err == nil {
		result.Data["firmware_version"] = strings.TrimSpace(output)
	}

	// CGMiner config
	configPath := "/config/cgminer.conf"
	if content, err := os.ReadFile(configPath); err == nil {
		result.Data["cgminer_config_exists"] = true
		// Try to parse as JSON
		var config map[string]interface{}
		if json.Unmarshal(content, &config) == nil {
			result.Data["cgminer_config"] = config
		} else {
			result.Data["cgminer_config_raw"] = string(content)
		}
	} else {
		result.Data["cgminer_config_exists"] = false
	}

	// Recent kernel messages related to ASIC
	if output, err := runCmd("dmesg"); err == nil {
		var asicMessages []string
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), "bitmain") ||
				strings.Contains(strings.ToLower(line), "asic") {
				asicMessages = append(asicMessages, strings.TrimSpace(line))
			}
		}
		// Keep last 20 messages
		if len(asicMessages) > 20 {
			asicMessages = asicMessages[len(asicMessages)-20:]
		}
		result.Data["asic_kernel_messages"] = asicMessages
	}

	return result
}

// DeviceAccessTest tests device access
func DeviceAccessTest() DiagnosticResult {
	result := DiagnosticResult{
		Phase:     "device_access_test",
		Timestamp: time.Now().Format(time.RFC3339),
		Success:   true,
		Data:      make(map[string]interface{}),
	}

	devicePath := "/dev/bitmain-asic"

	// Try to open device
	file, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		result.Data["open_rdwr"] = false
		result.Data["open_rdwr_error"] = err.Error()

		// Try read-only
		file, err = os.OpenFile(devicePath, os.O_RDONLY, 0)
		if err != nil {
			result.Data["open_rdonly"] = false
			result.Data["open_rdonly_error"] = err.Error()
			result.Success = false
			return result
		}
		result.Data["open_rdonly"] = true
	} else {
		result.Data["open_rdwr"] = true
	}
	defer file.Close()

	result.Data["fd"] = file.Fd()

	// Try to read with timeout
	readDone := make(chan []byte, 1)
	readErr := make(chan error, 1)

	go func() {
		buf := make([]byte, 64)
		n, err := file.Read(buf)
		if err != nil {
			readErr <- err
			return
		}
		readDone <- buf[:n]
	}()

	select {
	case data := <-readDone:
		result.Data["read_success"] = true
		result.Data["read_bytes"] = len(data)
		result.Data["read_hex"] = hex.EncodeToString(data)
	case err := <-readErr:
		result.Data["read_success"] = false
		result.Data["read_error"] = err.Error()
	case <-time.After(3 * time.Second):
		result.Data["read_success"] = false
		result.Data["read_error"] = "timeout"
	}

	return result
}

// RunAllDiagnostics runs all diagnostic phases
func RunAllDiagnostics() []DiagnosticResult {
	return []DiagnosticResult{
		SystemInfo(),
		DeviceInfo(),
		ProcessInfo(),
		ProtocolInfo(),
		DeviceAccessTest(),
	}
}

// Helper function to run a command
func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// PrintJSON prints the results as JSON
func PrintJSON(results []DiagnosticResult) {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling results: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

// PrintText prints the results as human-readable text
func PrintText(results []DiagnosticResult) {
	for _, result := range results {
		fmt.Printf("\n%s\n", strings.Repeat("=", 50))
		fmt.Printf("Phase: %s\n", result.Phase)
		fmt.Printf("Timestamp: %s\n", result.Timestamp)
		fmt.Printf("Success: %v\n", result.Success)
		fmt.Println(strings.Repeat("-", 50))

		for key, value := range result.Data {
			switch v := value.(type) {
			case []string:
				fmt.Printf("%s:\n", key)
				for _, item := range v {
					fmt.Printf("  - %s\n", item)
				}
			case map[string]string:
				fmt.Printf("%s:\n", key)
				for k, val := range v {
					fmt.Printf("  %s: %s\n", k, val)
				}
			default:
				fmt.Printf("%s: %v\n", key, value)
			}
		}

		if len(result.Errors) > 0 {
			fmt.Println("Errors:")
			for _, err := range result.Errors {
				fmt.Printf("  - %s\n", err)
			}
		}
	}
}
