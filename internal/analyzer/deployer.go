package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"hasher/internal/config"
)

// Phase represents a diagnostic phase
type Phase int

const (
	PhaseDiscovery Phase = iota
	PhaseProbe
	PhaseProtocol
	PhaseProvision
	PhaseTroubleshoot
	PhaseTest
)

func (p Phase) String() string {
	switch p {
	case PhaseDiscovery:
		return "Discovery"
	case PhaseProbe:
		return "Probe"
	case PhaseProtocol:
		return "Protocol"
	case PhaseProvision:
		return "Provision"
	case PhaseTroubleshoot:
		return "Troubleshoot"
	case PhaseTest:
		return "Test"
	default:
		return "Unknown"
	}
}

// DeployerConfig holds configuration for the deployer
type DeployerConfig struct {
	Subnet          string        // CIDR notation for network scan
	Username        string        // SSH username (default: root)
	Password        string        // SSH password
	Timeout         time.Duration // Connection timeout
	WorkDir         string        // Local working directory for binaries
	RemoteDir       string        // Remote directory for uploaded binaries
	ConcurrentScans int           // Number of concurrent network scans
}

// DefaultDeployerConfig returns a default configuration
func DefaultDeployerConfig() DeployerConfig {
	cfg, err := config.LoadDeviceConfig()
	homeDir, _ := os.UserHomeDir()
	username := "root"
	password := ""

	if err == nil {
		username = cfg.Username
		if username == "" {
			username = "root"
		}
		password = cfg.Password
	}

	// Override with environment variables if set
	if envUser := os.Getenv("DEVICE_USERNAME"); envUser != "" {
		username = envUser
	}
	if envPass := os.Getenv("DEVICE_PASSWORD"); envPass != "" {
		password = envPass
	}

	return DeployerConfig{
		Subnet:          "",
		Username:        username,
		Password:        password,
		Timeout:         10 * time.Second,
		WorkDir:         filepath.Join(homeDir, ".hasher", "bin"),
		RemoteDir:       "/tmp",
		ConcurrentScans: 20,
	}
}

// PhaseResult contains the result of a phase execution
type PhaseResult struct {
	Phase     Phase     `json:"phase"`
	Success   bool      `json:"success"`
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	Duration  float64   `json:"duration_seconds"`
	Timestamp time.Time `json:"timestamp"`
}

// Deployer manages ASIC device diagnostic phases
type Deployer struct {
	config       DeployerConfig
	devices      []DeviceInfo
	activeDevice *DeviceInfo
	sshClient    *ssh.Client
	mu           sync.Mutex
	results      []PhaseResult
	logWriter    io.Writer
}

// NewDeployer creates a new deployer instance
func NewDeployer(config DeployerConfig) (*Deployer, error) {
	// Ensure work directory exists
	if err := os.MkdirAll(config.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	return &Deployer{
		config:    config,
		devices:   []DeviceInfo{},
		results:   []PhaseResult{},
		logWriter: os.Stdout,
	}, nil
}

// SetLogWriter sets the writer for log output
func (d *Deployer) SetLogWriter(w io.Writer) {
	d.logWriter = w
}

// log writes a message to the log writer
func (d *Deployer) log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if d.logWriter != nil {
		fmt.Fprintln(d.logWriter, msg)
	}
}

// GetDevices returns discovered devices
func (d *Deployer) GetDevices() []DeviceInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.devices
}

// GetActiveDevice returns the currently active device
func (d *Deployer) GetActiveDevice() *DeviceInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.activeDevice
}

// GetResults returns all phase results
func (d *Deployer) GetResults() []PhaseResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.results
}

// ============================================================================
// Phase 1: Discovery - Scan network for ASIC devices
// ============================================================================

// RunDiscovery scans the network for ASIC devices
func (d *Deployer) RunDiscovery() (*PhaseResult, error) {
	start := time.Now()
	result := &PhaseResult{
		Phase:     PhaseDiscovery,
		Timestamp: start,
	}

	d.log("Starting network discovery on %s...", d.config.Subnet)

	// Add global timeout for discovery (30 seconds max)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse CIDR
	ip, ipnet, err := net.ParseCIDR(d.config.Subnet)
	if err != nil {
		result.Error = fmt.Sprintf("invalid subnet: %v", err)
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, err
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, d.config.ConcurrentScans)
	devicesChan := make(chan DeviceInfo, 256)
	var foundDevices []DeviceInfo
	var mu sync.Mutex

	// Generate IP list
	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
	}

	d.log("Scanning %d IP addresses...", len(ips))

	// Scan each IP
	for _, ipStr := range ips {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			d.log("Discovery timeout reached, stopping scan")
			goto collectResults
		default:
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(targetIP string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Check context before probing
			select {
			case <-ctx.Done():
				return
			default:
			}

			if device := d.probeHost(targetIP); device != nil {
				devicesChan <- *device
			}
		}(ipStr)
	}

collectResults:

	// Collector goroutine
	go func() {
		wg.Wait()
		close(devicesChan)
	}()

	// Collect results
	for device := range devicesChan {
		mu.Lock()
		foundDevices = append(foundDevices, device)
		d.log("Found device: %s (%s) - %s", device.IPAddress, device.Hostname, device.DeviceType)
		mu.Unlock()
	}

	d.mu.Lock()
	d.devices = foundDevices
	d.mu.Unlock()

	// Build output
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Network Scan Results (%s)\n", d.config.Subnet))
	output.WriteString(strings.Repeat("=", 50) + "\n")
	output.WriteString(fmt.Sprintf("Found %d ASIC device(s)\n\n", len(foundDevices)))

	for i, dev := range foundDevices {
		output.WriteString(fmt.Sprintf("[%d] %s\n", i+1, dev.IPAddress))
		output.WriteString(fmt.Sprintf("    Hostname: %s\n", dev.Hostname))
		output.WriteString(fmt.Sprintf("    Type: %s\n", dev.DeviceType))
		output.WriteString(fmt.Sprintf("    Protocol: %s\n", dev.Protocol.String()))
		output.WriteString(fmt.Sprintf("    Ports: %v\n", dev.OpenPorts))
		output.WriteString(fmt.Sprintf("    Accessible: %v\n", dev.Accessible))
		output.WriteString("\n")
	}

	result.Success = true
	result.Output = output.String()
	result.Duration = time.Since(start).Seconds()
	d.addResult(*result)

	return result, nil
}

// probeHost checks if a host is an ASIC device
func (d *Deployer) probeHost(ip string) *DeviceInfo {
	device := &DeviceInfo{
		IPAddress:   ip,
		Services:    make(map[string]string),
		AuthMethods: []string{},
	}

	// Scan common ASIC ports
	ports := []int{22, 80, 4028, 4029}
	openPorts := d.scanPorts(ip, ports)
	if len(openPorts) == 0 {
		return nil
	}
	device.OpenPorts = openPorts

	// Try SSH connection
	for _, port := range openPorts {
		if port == 22 {
			if d.checkSSHAccess(ip, device) {
				device.Services["ssh"] = "OpenSSH"
			}
		}
	}

	// Determine if this is a miner
	hasMiningPort := false
	for _, port := range openPorts {
		if port == 4028 || port == 4029 {
			hasMiningPort = true
			break
		}
	}

	if hasMiningPort || device.Accessible {
		device.Protocol = detectProtocol(device)
		return device
	}

	return nil
}

// scanPorts scans a list of ports on a host
func (d *Deployer) scanPorts(ip string, ports []int) []int {
	var openPorts []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			address := net.JoinHostPort(ip, fmt.Sprintf("%d", p))
			conn, err := net.DialTimeout("tcp", address, d.config.Timeout/2)
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			openPorts = append(openPorts, p)
			mu.Unlock()
		}(port)
	}
	wg.Wait()
	return openPorts
}

// checkSSHAccess tests SSH access to a device
func (d *Deployer) checkSSHAccess(ip string, device *DeviceInfo) bool {
	sshConfig := &ssh.ClientConfig{
		User: d.config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(d.config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         d.config.Timeout,
		HostKeyAlgorithms: []string{
			"ssh-rsa",
			"ssh-dss",
		},
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(ip, "22"), sshConfig)
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			device.AuthMethods = append(device.AuthMethods, "ssh-password")
			return true
		}
		return false
	}
	defer client.Close()

	device.Accessible = true
	device.AuthMethods = append(device.AuthMethods, "ssh-success")

	// Get hostname
	session, err := client.NewSession()
	if err == nil {
		output, _ := session.Output("hostname")
		if len(output) > 0 {
			device.Hostname = strings.TrimSpace(string(output))
		}
		session.Close()
	}

	// Get device type
	session, err = client.NewSession()
	if err == nil {
		output, _ := session.Output("cat /usr/bin/compile_time 2>/dev/null || echo 'unknown'")
		if len(output) > 0 {
			device.DeviceType = strings.TrimSpace(string(output))
		}
		session.Close()
	}

	return true
}

// ============================================================================
// Phase 2: Probe - Probe connected ASIC device
// ============================================================================

// SelectDevice selects a device for subsequent operations
func (d *Deployer) SelectDevice(index int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if index < 0 || index >= len(d.devices) {
		return fmt.Errorf("invalid device index: %d", index)
	}

	d.activeDevice = &d.devices[index]
	return nil
}

// SelectDeviceByIP selects a device by IP address
func (d *Deployer) SelectDeviceByIP(ip string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.devices {
		if d.devices[i].IPAddress == ip {
			d.activeDevice = &d.devices[i]
			return nil
		}
	}

	// If not found in discovered devices, create a new one
	d.activeDevice = &DeviceInfo{
		IPAddress: ip,
		Services:  make(map[string]string),
	}
	return nil
}

// Connect establishes SSH connection to the active device
func (d *Deployer) Connect() error {
	if d.activeDevice == nil {
		return fmt.Errorf("no device selected")
	}

	sshConfig := &ssh.ClientConfig{
		User: d.config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(d.config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         d.config.Timeout,
		HostKeyAlgorithms: []string{
			"ssh-rsa",
			"ssh-dss",
		},
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(d.activeDevice.IPAddress, "22"), sshConfig)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	d.sshClient = client
	d.log("Connected to %s", d.activeDevice.IPAddress)
	return nil
}

// Disconnect closes the SSH connection
func (d *Deployer) Disconnect() {
	if d.sshClient != nil {
		d.sshClient.Close()
		d.sshClient = nil
	}
}

// RunProbe runs device probing phase
func (d *Deployer) RunProbe() (*PhaseResult, error) {
	start := time.Now()
	result := &PhaseResult{
		Phase:     PhaseProbe,
		Timestamp: start,
	}

	if d.activeDevice == nil {
		result.Error = "no device selected"
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	d.log("Probing device %s...", d.activeDevice.IPAddress)

	// Connect if not connected
	if d.sshClient == nil {
		if err := d.Connect(); err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Seconds()
			d.addResult(*result)
			return result, err
		}
		defer d.Disconnect()
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Device Probe Results (%s)\n", d.activeDevice.IPAddress))
	output.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Gather system information
	d.log("Gathering system information...")
	output.WriteString("System Information:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	// CPU info
	if cpuInfo, err := d.runRemoteCommand("cat /proc/cpuinfo | grep -E 'system type|cpu model|BogoMIPS'"); err == nil {
		output.WriteString("CPU:\n" + cpuInfo + "\n")
	}

	// Memory info
	if memInfo, err := d.runRemoteCommand("cat /proc/meminfo | grep -E 'MemTotal|MemFree|MemAvailable'"); err == nil {
		output.WriteString("Memory:\n" + memInfo + "\n")
	}

	// Kernel version
	if kernel, err := d.runRemoteCommand("uname -a"); err == nil {
		output.WriteString("Kernel: " + strings.TrimSpace(kernel) + "\n\n")
	}

	// Device information
	output.WriteString("ASIC Device:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	// Check device file
	if devInfo, err := d.runRemoteCommand("ls -la /dev/bitmain-asic 2>/dev/null || echo 'Device not found'"); err == nil {
		output.WriteString("Device file:\n" + devInfo + "\n")
	}

	// USB devices
	if usbInfo, err := d.runRemoteCommand("lsusb 2>/dev/null || echo 'lsusb not available'"); err == nil {
		output.WriteString("USB devices:\n" + usbInfo + "\n")
	}

	// Kernel modules
	if modInfo, err := d.runRemoteCommand("lsmod | grep -i bitmain 2>/dev/null || echo 'No bitmain modules'"); err == nil {
		output.WriteString("Kernel modules:\n" + modInfo + "\n")
	}

	// CGMiner status
	if psInfo, err := d.runRemoteCommand("ps w | grep -i cgminer | grep -v grep || echo 'CGMiner not running'"); err == nil {
		output.WriteString("CGMiner status:\n" + psInfo + "\n")
	}

	result.Success = true
	result.Output = output.String()
	result.Duration = time.Since(start).Seconds()
	d.addResult(*result)

	return result, nil
}

// ============================================================================
// Phase 3: Protocol - Detect ASIC device protocol
// ============================================================================

// RunProtocol runs protocol detection phase
func (d *Deployer) RunProtocol() (*PhaseResult, error) {
	start := time.Now()
	result := &PhaseResult{
		Phase:     PhaseProtocol,
		Timestamp: start,
	}

	if d.activeDevice == nil {
		result.Error = "no device selected"
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	d.log("Detecting protocol for %s...", d.activeDevice.IPAddress)

	// Connect if not connected
	if d.sshClient == nil {
		if err := d.Connect(); err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Seconds()
			d.addResult(*result)
			return result, err
		}
		defer d.Disconnect()
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Protocol Detection Results (%s)\n", d.activeDevice.IPAddress))
	output.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Build and deploy protocol detection script
	script := d.generateProtocolScript()
	remotePath := filepath.Join(d.config.RemoteDir, "protocol_detect.sh")

	d.log("Uploading protocol detection script...")
	if err := d.uploadFile(remotePath, []byte(script)); err != nil {
		result.Error = fmt.Sprintf("failed to upload script: %v", err)
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	// Execute script
	d.log("Executing protocol detection...")
	scriptOutput, err := d.runRemoteCommand(fmt.Sprintf("chmod +x %s && %s 2>&1", remotePath, remotePath))
	if err != nil {
		result.Error = fmt.Sprintf("script execution failed: %v", err)
		d.log("Warning: %s", result.Error)
	}
	output.WriteString(scriptOutput + "\n")

	// Cleanup
	d.runRemoteCommand(fmt.Sprintf("rm -f %s", remotePath))

	// Extract protocol information from CGMiner config
	d.log("Analyzing CGMiner configuration...")
	output.WriteString("\nCGMiner Analysis:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	if cgConfig, err := d.runRemoteCommand("cat /config/cgminer.conf 2>/dev/null || echo 'Config not found'"); err == nil {
		output.WriteString(cgConfig + "\n")
	}

	// Check dmesg for protocol hints
	if dmesg, err := d.runRemoteCommand("dmesg | grep -i 'bitmain\\|asic' | tail -20 2>/dev/null || echo 'No dmesg entries'"); err == nil {
		output.WriteString("\nKernel Messages:\n")
		output.WriteString(strings.Repeat("-", 30) + "\n")
		output.WriteString(dmesg + "\n")
	}

	result.Success = true
	result.Output = output.String()
	result.Duration = time.Since(start).Seconds()
	d.addResult(*result)

	return result, nil
}

// generateProtocolScript generates a shell script for protocol detection
func (d *Deployer) generateProtocolScript() string {
	return `#!/bin/sh
echo "Protocol Detection Script"
echo "========================="
echo ""

echo "1. Device Information:"
echo "---------------------"
if [ -c /dev/bitmain-asic ]; then
    ls -la /dev/bitmain-asic
    echo "Device exists: YES"
    MAJOR=$(ls -la /dev/bitmain-asic | awk '{print $5}' | tr -d ',')
    MINOR=$(ls -la /dev/bitmain-asic | awk '{print $6}')
    echo "Major: $MAJOR, Minor: $MINOR"
else
    echo "Device exists: NO"
fi
echo ""

echo "2. USB Device Information:"
echo "-------------------------"
lsusb 2>/dev/null || echo "lsusb not available"
echo ""

echo "3. Protocol Signatures:"
echo "----------------------"
if [ -f /usr/bin/cgminer ]; then
    strings /usr/bin/cgminer 2>/dev/null | grep -i "bitmain" | head -5
    strings /usr/bin/cgminer 2>/dev/null | grep -i "ioctl" | head -5
fi
echo ""

echo "4. Device Registers:"
echo "-------------------"
if [ -d /sys/class/misc/bitmain-asic ]; then
    ls -la /sys/class/misc/bitmain-asic/
fi
echo ""

echo "5. Firmware Version:"
echo "-------------------"
cat /usr/bin/compile_time 2>/dev/null || echo "Not available"
echo ""

echo "Protocol detection complete."
`
}

// ============================================================================
// Phase 4: Provision - Deploy hasher-server to ASIC device
// ============================================================================

// RunProvision runs the provisioning phase
func (d *Deployer) RunProvision() (*PhaseResult, error) {
	start := time.Now()
	result := &PhaseResult{
		Phase:     PhaseProvision,
		Timestamp: start,
	}

	if d.activeDevice == nil {
		result.Error = "no device selected"
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	d.log("Provisioning device %s...", d.activeDevice.IPAddress)

	// Connect if not connected
	if d.sshClient == nil {
		if err := d.Connect(); err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Seconds()
			d.addResult(*result)
			return result, err
		}
		defer d.Disconnect()
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Provisioning Results (%s)\n", d.activeDevice.IPAddress))
	output.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Step 1: Check available disk space
	d.log("Checking available disk space...")
	if df, err := d.runRemoteCommand("df -h /tmp"); err == nil {
		output.WriteString("Disk Space:\n" + df + "\n\n")
	}

	// Step 2: Stop CGMiner and release device, stop existing hasher-server, and aggressively manage kernel module
	d.log("Aggressively stopping competing processes and managing kernel module for clean USB access...")
	output.WriteString("Aggressive Device Cleanup and Kernel Module Management:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	// Upload and run the force reset script first
	d.log("Uploading force reset script...")
	forceResetScript := `#!/bin/bash
# Force reset ASIC device when stuck in usbfs claim
echo "=== FORCE RESET OF ASIC DEVICE ==="

# Kill any remaining hasher-server processes
echo "Killing hasher-server processes..."
killall -9 hasher-server 2>/dev/null || true
sleep 3

# Kill CGMiner processes
echo "Stopping CGMiner processes..."
if pgrep cgminer > /dev/null 2>&1; then
    /etc/init.d/cgminer stop 2>/dev/null || true
    sleep 3
    killall -9 cgminer bmminer 2>/dev/null || true
    sleep 1
    if pgrep cgminer > /dev/null 2>&1; then
        echo "WARNING: CGMiner still running"
    else
        echo "SUCCESS: CGMiner stopped"
    fi
else
    echo "CGMiner was not running"
fi

# Try to unload modules
echo "Attempting to unload modules..."
rmmod bitmain_asic 2>/dev/null || echo "Module busy (expected)"
rmmod usb_bitmain 2>/dev/null || echo "USB module busy (expected)"

# Force USB reset by writing to sysfs
echo "Forcing USB reset via sysfs..."
if [ -d /sys/bus/usb/devices/1-1.1 ]; then
    echo '0' > /sys/bus/usb/devices/1-1.1/bConfiguration 2>/dev/null || true
    sleep 1
fi

# Now try to reload module
echo "Reloading kernel module..."
sleep 2
modprobe -r bitmain_asic 2>/dev/null || true
sleep 2
modprobe bitmain_asic 2>/dev/null || echo "Module reload failed"
sleep 3

# Create device nodes with correct major/minor
echo "Creating device nodes..."
rm -f /dev/bitmain-asic /dev/bitmain0 2>/dev/null || true
mknod /dev/bitmain-asic c 10 60 2>/dev/null || echo "Failed to create /dev/bitmain-asic"
mknod /dev/bitmain0 c 180 0 2>/dev/null || echo "Failed to create /dev/bitmain0"
chmod 666 /dev/bitmain-asic /dev/bitmain0 2>/dev/null || true

echo "=== FORCE RESET COMPLETE ==="
echo "Final device status:"
ls -la /dev/bitmain* 2>/dev/null || echo "No devices found"
echo "Kernel module status:"
lsmod | grep bitmain || echo "No bitmain modules"
`

	// Upload script to device
	if err := d.uploadFile("/tmp/force-reset-device.sh", []byte(forceResetScript)); err != nil {
		output.WriteString(fmt.Sprintf("Failed to upload force reset script: %v\n", err))
	} else {
		// Make script executable and run it
		d.runRemoteCommand("chmod +x /tmp/force-reset-device.sh")
		stopOutput, _ := d.runRemoteCommand("/tmp/force-reset-device.sh")
		output.WriteString(stopOutput + "\n\n")
	}

	// Step 3: Deploy hasher-server
	d.log("Checking for hasher-server binary...")
	output.WriteString("Deploying hasher-server:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	// hasher-server is no longer embedded in CLI - look in work directory
	serverBinaryPath := filepath.Join(d.config.WorkDir, "hasher-server-mips")

	if _, err := os.Stat(serverBinaryPath); err == nil {
		d.log("Using local hasher-server binary at %s", serverBinaryPath)

		// Read binary
		binaryContent, err := os.ReadFile(serverBinaryPath) // Changed variable name from 'binary' to 'binaryContent'
		if err != nil {
			output.WriteString(fmt.Sprintf("Failed to read local binary: %v\n", err))
		} else {
			remoteBinaryName := "hasher-server"
			remoteBinaryPath := filepath.Join(d.config.RemoteDir, remoteBinaryName)

			d.log("Uploading %s to %s...", remoteBinaryName, remoteBinaryPath)
			if err := d.uploadFile(remoteBinaryPath, binaryContent); err != nil { // Use binaryContent
				output.WriteString(fmt.Sprintf("Failed to upload binary: %v\n", err))
			} else {
				d.log("Binary uploaded successfully.")
				// Store the remote binary path for execution
				remoteServerPath := remoteBinaryPath
				d.log("Setting executable permissions on %s...", remoteBinaryPath)
				d.runRemoteCommand(fmt.Sprintf("chmod +x %s", remoteBinaryPath))
				output.WriteString(fmt.Sprintf("Deployed to %s\n", remoteBinaryPath))
				d.log("✅ hasher-server deployed successfully at %s", remoteBinaryPath)

				// Get MD5 sum of deployed binary for verification
				d.log("Getting MD5 sum of deployed binary...")
				md5sumOutput, _ := d.runRemoteCommand(fmt.Sprintf("md5sum %s", remoteBinaryPath))
				d.log("Deployed hasher-server MD5 sum on ASIC: %s", md5sumOutput)
				output.WriteString(fmt.Sprintf("Deployed hasher-server MD5 sum: %s\n", md5sumOutput))

				// Clear old log file before starting new server
				d.log("Clearing old hasher-server log file...")
				d.runRemoteCommand(fmt.Sprintf("rm -f %s/hasher-server.log", d.config.RemoteDir))
				output.WriteString("Old hasher-server.log cleared.\n")

				// Start hasher-server with proper initialization using a subshell for BusyBox compatibility
				d.log("Starting hasher-server using sh -c '... & '...")
				startCmd := fmt.Sprintf("sh -c '%s --port=8888 --trace=true > %s/hasher-server.log 2>&1 &' && echo 'SERVER_START_PID:$!' && sleep 1", remoteServerPath, d.config.RemoteDir)
				startOutput, _ := d.runRemoteCommand(startCmd)
				output.WriteString("Started hasher-server: " + startOutput + "\n")
				d.log("sh -c '... & ' command executed to start hasher-server.") // Give hasher-server more time to initialize and start listening
				d.log("Waiting for hasher-server to initialize...")
				d.runRemoteCommand("sleep 3") // Initial wait

				// Verify the process is actually running and get its PID
				d.log("Verifying hasher-server process...")
				psOutput, _ := d.runRemoteCommand("ps w | grep hasher-server | grep -v grep || echo 'PROCESS_NOT_FOUND'")
				if strings.Contains(psOutput, "PROCESS_NOT_FOUND") {
					output.WriteString("WARNING: hasher-server process not found after startup\n")
					d.log("WARNING: hasher-server process not found after startup.")
				} else {
					output.WriteString("hasher-server process confirmed running:\n" + psOutput + "\n")
					d.log("hasher-server process confirmed running.")

					// Additional wait to ensure server is fully initialized
					d.log("Giving hasher-server additional time to bind to port...")
					d.runRemoteCommand("sleep 2")

					// Check if port 8888 is listening
					portOutput, _ := d.runRemoteCommand("netstat -ln | grep :8888 || echo 'PORT_NOT_BOUND'")
					if strings.Contains(portOutput, "PORT_NOT_BOUND") {
						output.WriteString("WARNING: hasher-server not listening on port 8888 yet\n")
						d.log("WARNING: hasher-server not listening on port 8888 yet.")
					} else {
						output.WriteString("hasher-server confirmed listening on port 8888\n" + portOutput + "\n")
						d.log("hasher-server confirmed listening on port 8888.")
					}
				}
			}
		}
	} else {
		output.WriteString("hasher-server binary not found locally.\n")
		output.WriteString("Build it first with:\n")
		output.WriteString("  make build-server-mips\n")
		output.WriteString("Or place it in: " + serverBinaryPath + "\n\n")
		d.log("ERROR: hasher-server binary not found locally at %s", serverBinaryPath)
		result.Error = fmt.Sprintf("hasher-server binary not found locally: %s", serverBinaryPath)
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	// Step 4: Verify provisioning
	d.log("Verifying provisioning...")
	output.WriteString("\nProvisioning Status:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	verifyOutput, _ := d.runRemoteCommand(`
		echo "CGMiner: $(pgrep cgminer > /dev/null 2>&1 && echo 'RUNNING' || echo 'STOPPED')"
		echo "Device: $(test -c /dev/bitmain-asic && echo 'AVAILABLE' || echo 'NOT FOUND')"
		echo "Hasher: $(test -x /tmp/hasher-server && echo 'DEPLOYED' || echo 'NOT DEPLOYED')"
	`)
	output.WriteString(verifyOutput + "\n")

	result.Success = true
	result.Output = output.String()
	result.Duration = time.Since(start).Seconds()
	d.addResult(*result)

	return result, nil
}

// RunProvisionWithBinary runs the provisioning phase using the provided binary content.
func (d *Deployer) RunProvisionWithBinary(binaryContent []byte) (*PhaseResult, error) {
	start := time.Now()
	result := &PhaseResult{
		Phase:     PhaseProvision,
		Timestamp: start,
	}

	if d.activeDevice == nil {
		result.Error = "no device selected"
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	d.log("Provisioning device %s...", d.activeDevice.IPAddress)

	// Connect if not connected
	if d.sshClient == nil {
		if err := d.Connect(); err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Seconds()
			d.addResult(*result)
			return result, err
		}
		defer d.Disconnect()
	}

	// Check if CGMiner is already running and responding to API
	d.log("Checking CGMiner status...")
	cgminerRunning := false
	if cgStatus, err := d.runRemoteCommand("pgrep cgminer > /dev/null 2>&1 && echo 'RUNNING' || echo 'NOT_RUNNING'"); err == nil && strings.Contains(cgStatus, "RUNNING") {
		// CGMiner is running, check if API is responding
		d.log("CGMiner process found, checking API...")
		if apiTest, err := d.runRemoteCommand("echo '{\"command\":\"summary\"}' | nc 127.0.0.1 4028 2>/dev/null | head -c 50 || echo 'API_NOT_RESPONDING'"); err == nil && !strings.Contains(apiTest, "API_NOT_RESPONDING") && len(apiTest) > 10 {
			d.log("✓ CGMiner is running and API is responding - will use CGMiner mode")
			cgminerRunning = true
		} else {
			d.log("CGMiner running but API not responding, will restart...")
		}
	}

	// If CGMiner is not running or not responding, start it
	if !cgminerRunning {
		d.log("Starting CGMiner...")
		startScript := `#!/bin/ash
# Start CGMiner for hasher-server integration
echo "=== STARTING CGMINER ==="

# Kill any existing CGMiner first to ensure clean start
killall -9 cgminer bmminer 2>/dev/null || true
sleep 2

# Start CGMiner in benchmark mode with API enabled
cgminer --benchmark --bitmain-options 115200:32:8:16:250:0982 --api-listen --api-allow W:0/0 --api-port 4028 --quiet &
sleep 5

# Verify it's running
if pgrep cgminer > /dev/null 2>&1; then
    echo "SUCCESS: CGMiner started"
    # Test API
    echo "Testing API..."
    echo '{"command":"summary"}' | nc 127.0.0.1 4028 2>/dev/null | head -c 20 || echo "API_TEST_FAILED"
else
    echo "ERROR: CGMiner failed to start"
fi
echo "=== CGMINER START COMPLETE ==="
`
		if err := d.uploadFile("/tmp/start-cgminer.sh", []byte(startScript)); err == nil {
			d.runRemoteCommand("chmod +x /tmp/start-cgminer.sh && ash /tmp/start-cgminer.sh")
		}

		// Wait a bit for CGMiner to initialize
		time.Sleep(3 * time.Second)
	}

	// Check for zombie processes that might cause issues
	d.log("Checking for zombie processes...")
	zombieCheck, _ := d.runRemoteCommand("ps w | grep -E 'hasher-server|cgminer' | grep -i zombie || echo 'NO_ZOMBIES'")
	if strings.Contains(zombieCheck, "zombie") || strings.Contains(zombieCheck, "Z") {
		d.log("WARNING: Zombie processes detected, will clean up...")
		// Only do aggressive cleanup if zombies are present
		d.runRemoteCommand("killall -9 hasher-server cgminer bmminer 2>/dev/null || true")
		time.Sleep(2 * time.Second)
	}

	// Minimal cleanup: just kill old hasher-server processes
	d.log("Cleaning up old hasher-server processes...")
	d.runRemoteCommand("killall -9 hasher-server 2>/dev/null || true")
	time.Sleep(1 * time.Second)

	d.log("CGMiner-first deployment ready")

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Provisioning Results (%s)\n", d.activeDevice.IPAddress))
	output.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Step 1: Check available disk space
	d.log("Checking available disk space...")
	if df, err := d.runRemoteCommand("df -h /tmp"); err == nil {
		output.WriteString("Disk Space:\n" + df + "\n\n")
	}

	// Step 2: Verify device is ready after pre-cleanup
	d.log("Verifying device status after pre-cleanup...")
	output.WriteString("Device Status Verification:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	var verifyOutput string
	var err error
	verifyOutput, err = d.runRemoteCommand(`
		echo "Checking device accessibility..."
		if [ -e /dev/bitmain-asic ]; then
			echo "✅ /dev/bitmain-asic exists"
			ls -la /dev/bitmain-asic
		else
			echo "❌ /dev/bitmain-asic not found"
		fi
		
		echo "Checking kernel module status..."
		if lsmod | grep -q bitmain_asic; then
			echo "✅ bitmain_asic module loaded"
		else
			echo "❌ bitmain_asic module not loaded"
		fi
		
		echo "Checking for competing processes..."
		if pgrep -f "cgminer\|bmminer" > /dev/null; then
			echo "⚠️  Mining processes still running:"
			ps w | grep -E "cgminer|bmminer" | grep -v grep
		else
			echo "✅ No competing mining processes found"
		fi
	`)
	if err == nil {
		output.WriteString(verifyOutput + "\n\n")
	} else {
		output.WriteString(fmt.Sprintf("Failed to verify device status: %v\n\n", err))
	}

	// Step 3: Deploy hasher-server
	d.log("Checking for hasher-server binary...")
	output.WriteString("Deploying hasher-server:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	if len(binaryContent) == 0 {
		output.WriteString("ERROR: binary content is empty\n")
		result.Error = "binary content is empty"
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	remoteBinaryName := "hasher-server"
	remoteBinaryPath := filepath.Join(d.config.RemoteDir, remoteBinaryName)

	d.log("Uploading %s to %s...", remoteBinaryName, remoteBinaryPath)
	if err := d.uploadFile(remoteBinaryPath, binaryContent); err != nil {
		output.WriteString(fmt.Sprintf("Failed to upload binary: %v\n", err))
		result.Error = err.Error()
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, err
	}
	d.log("Binary uploaded successfully.")
	// Store the remote binary path for execution
	remoteServerPath := remoteBinaryPath
	d.log("Setting executable permissions on %s...", remoteBinaryPath)
	d.runRemoteCommand(fmt.Sprintf("chmod +x %s", remoteBinaryPath))
	output.WriteString(fmt.Sprintf("Deployed to %s\n", remoteBinaryPath))
	d.log("✅ hasher-server deployed successfully at %s", remoteBinaryPath)

	// Get MD5 sum of deployed binary for verification
	d.log("Getting MD5 sum of deployed binary...")
	md5sumOutput, _ := d.runRemoteCommand(fmt.Sprintf("md5sum %s", remoteBinaryPath))
	d.log("Deployed hasher-server MD5 sum on ASIC: %s", md5sumOutput)
	output.WriteString(fmt.Sprintf("Deployed hasher-server MD5 sum: %s\n", md5sumOutput))

	// Clear old log file before starting new server
	d.log("Clearing old hasher-server log file...")
	d.runRemoteCommand(fmt.Sprintf("rm -f %s/hasher-server.log", d.config.RemoteDir))
	output.WriteString("Old hasher-server.log cleared.\n")

	// Start hasher-server with proper initialization using a subshell for BusyBox compatibility
	d.log("Starting hasher-server using sh -c '... & '...")
	startCmd := fmt.Sprintf("sh -c '%s --port=8888 --trace=true > %s/hasher-server.log 2>&1 &' && echo 'SERVER_START_PID:$!' && sleep 1", remoteServerPath, d.config.RemoteDir)
	startOutput, _ := d.runRemoteCommand(startCmd)
	output.WriteString("Started hasher-server: " + startOutput + "\n")
	d.log("sh -c '... & ' command executed to start hasher-server.")
	// Give hasher-server more time to initialize and start listening
	d.log("Waiting for hasher-server to initialize...")
	d.runRemoteCommand("sleep 3") // Initial wait

	// Verify the process is actually running and get its PID
	d.log("Verifying hasher-server process...")
	psOutput, _ := d.runRemoteCommand("ps w | grep hasher-server | grep -v grep || echo 'PROCESS_NOT_FOUND'")
	if strings.Contains(psOutput, "PROCESS_NOT_FOUND") {
		output.WriteString("WARNING: hasher-server process not found after startup\n")
		d.log("WARNING: hasher-server process not found after startup.")
	} else {
		output.WriteString("hasher-server process confirmed running:\n" + psOutput + "\n")
		d.log("hasher-server process confirmed running.")

		// Additional wait to ensure server is fully initialized
		d.log("Giving hasher-server additional time to bind to port...")
		d.runRemoteCommand("sleep 2")

		// Check if port 8888 is listening
		portOutput, _ := d.runRemoteCommand("netstat -ln | grep :8888 || echo 'PORT_NOT_BOUND'")
		if strings.Contains(portOutput, "PORT_NOT_BOUND") {
			output.WriteString("WARNING: hasher-server not listening on port 8888 yet\n")
			d.log("WARNING: hasher-server not listening on port 8888 yet.")
		} else {
			output.WriteString("hasher-server confirmed listening on port 8888\n" + portOutput + "\n")
			d.log("hasher-server confirmed listening on port 8888.")
		}
	}

	// Step 4: Verify provisioning
	d.log("Verifying provisioning...")
	output.WriteString("\nProvisioning Status:\n")
	output.WriteString(strings.Repeat("-", 30) + "\n")

	verifyOutput, _ = d.runRemoteCommand(`
		echo "CGMiner: $(pgrep cgminer > /dev/null 2>&1 && echo 'RUNNING' || echo 'STOPPED')"
		echo "Device: $(test -c /dev/bitmain-asic && echo 'AVAILABLE' || echo 'NOT FOUND')"
		echo "Hasher: $(test -x /tmp/hasher-server && echo 'DEPLOYED' || echo 'NOT DEPLOYED')"
	`)
	output.WriteString(verifyOutput + "\n")

	result.Success = true
	result.Output = output.String()
	result.Duration = time.Since(start).Seconds()
	d.addResult(*result)

	return result, nil
}

// ============================================================================
// Phase 5: Troubleshoot - Run troubleshooting diagnostics
// ============================================================================

// RunTroubleshoot runs the troubleshooting phase
func (d *Deployer) RunTroubleshoot() (*PhaseResult, error) {
	start := time.Now()
	result := &PhaseResult{
		Phase:     PhaseTroubleshoot,
		Timestamp: start,
	}

	if d.activeDevice == nil {
		result.Error = "no device selected"
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	d.log("Running troubleshooting on %s...", d.activeDevice.IPAddress)

	// Connect if not connected
	if d.sshClient == nil {
		if err := d.Connect(); err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Seconds()
			d.addResult(*result)
			return result, err
		}
		defer d.Disconnect()
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Troubleshooting Report (%s)\n", d.activeDevice.IPAddress))
	output.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Compile and deploy diagnostics script
	script := d.generateTroubleshootScript()
	remotePath := filepath.Join(d.config.RemoteDir, "troubleshoot.sh")

	d.log("Uploading troubleshooting script...")
	if err := d.uploadFile(remotePath, []byte(script)); err != nil {
		result.Error = fmt.Sprintf("failed to upload script: %v", err)
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	// Execute script
	d.log("Executing troubleshooting diagnostics...")
	scriptOutput, err := d.runRemoteCommand(fmt.Sprintf("chmod +x %s && %s 2>&1", remotePath, remotePath))
	if err != nil {
		d.log("Warning: script execution had errors: %v", err)
	}
	output.WriteString(scriptOutput + "\n")

	// Cleanup
	d.runRemoteCommand(fmt.Sprintf("rm -f %s", remotePath))

	result.Success = true
	result.Output = output.String()
	result.Duration = time.Since(start).Seconds()
	d.addResult(*result)

	return result, nil
}

// generateTroubleshootScript generates comprehensive troubleshooting script
func (d *Deployer) generateTroubleshootScript() string {
	return `#!/bin/sh
echo "Troubleshooting Diagnostics"
echo "==========================="
echo ""

echo "1. System Health:"
echo "----------------"
echo "Uptime: $(uptime)"
echo "Load: $(cat /proc/loadavg)"
echo "Memory: $(free -m 2>/dev/null || cat /proc/meminfo | grep -E 'MemTotal|MemFree' | head -2)"
echo ""

echo "2. Process Status:"
echo "-----------------"
ps w | head -20
echo ""

echo "3. Network Status:"
echo "-----------------"
ifconfig | head -20
echo ""

echo "4. Device Lock Investigation:"
echo "----------------------------"
echo "Processes with open files in /dev:"
lsof /dev/bitmain-asic 2>/dev/null || echo "lsof not available or no locks"
echo ""

echo "5. Kernel Module Status:"
echo "-----------------------"
lsmod | grep -i bitmain
cat /proc/devices | grep -i bitmain
echo ""

echo "6. USB Debug:"
echo "------------"
lsusb
cat /sys/kernel/debug/usb/devices 2>/dev/null | head -50 || echo "USB debug not available"
echo ""

echo "7. Recent Kernel Messages:"
echo "-------------------------"
dmesg | tail -30
echo ""

echo "8. Configuration Files:"
echo "----------------------"
echo "CGMiner config:"
cat /config/cgminer.conf 2>/dev/null | head -20 || echo "Not found"
echo ""

echo "9. File System Status:"
echo "---------------------"
df -h
echo ""
mount
echo ""

echo "Troubleshooting complete."
`
}

// ============================================================================
// Phase 6: Test - Test ASIC communication pattern
// ============================================================================

// RunTest runs the communication test phase
func (d *Deployer) RunTest() (*PhaseResult, error) {
	start := time.Now()
	result := &PhaseResult{
		Phase:     PhaseTest,
		Timestamp: start,
	}

	if d.activeDevice == nil {
		result.Error = "no device selected"
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	d.log("Testing communication with %s...", d.activeDevice.IPAddress)

	// Connect if not connected
	if d.sshClient == nil {
		if err := d.Connect(); err != nil {
			result.Error = err.Error()
			result.Duration = time.Since(start).Seconds()
			d.addResult(*result)
			return result, err
		}
		defer d.Disconnect()
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Communication Test Results (%s)\n", d.activeDevice.IPAddress))
	output.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Generate test script
	script := d.generateTestScript()
	remotePath := filepath.Join(d.config.RemoteDir, "comm_test.sh")

	d.log("Uploading communication test script...")
	if err := d.uploadFile(remotePath, []byte(script)); err != nil {
		result.Error = fmt.Sprintf("failed to upload script: %v", err)
		result.Duration = time.Since(start).Seconds()
		d.addResult(*result)
		return result, fmt.Errorf("%s", result.Error)
	}

	// Stop CGMiner first
	d.log("Stopping CGMiner for exclusive device access...")
	d.runRemoteCommand("killall -9 cgminer bmminer 2>/dev/null || true")
	time.Sleep(2 * time.Second)

	// Execute script
	d.log("Executing communication test...")
	scriptOutput, err := d.runRemoteCommand(fmt.Sprintf("chmod +x %s && %s 2>&1", remotePath, remotePath))
	if err != nil {
		d.log("Warning: script execution had errors: %v", err)
	}
	output.WriteString(scriptOutput + "\n")

	// Cleanup
	d.runRemoteCommand(fmt.Sprintf("rm -f %s", remotePath))

	result.Success = true
	result.Output = output.String()
	result.Duration = time.Since(start).Seconds()
	d.addResult(*result)

	return result, nil
}

// generateTestScript generates a communication test script
func (d *Deployer) generateTestScript() string {
	return `#!/bin/sh
echo "ASIC Communication Test"
echo "======================"
echo ""

DEVICE="/dev/bitmain-asic"

echo "1. Device Access Test:"
echo "---------------------"
if [ -c "$DEVICE" ]; then
    echo "Device exists: YES"

    # Check if device is busy
    if dd if=$DEVICE bs=1 count=0 2>/dev/null; then
        echo "Device accessible: YES"
    else
        echo "Device accessible: NO (may be locked)"
    fi
else
    echo "Device exists: NO"
fi
echo ""

echo "2. USB Device Test:"
echo "------------------"
# Check USB device presence
if lsusb | grep -q "4254:4153"; then
    echo "Bitmain USB device: FOUND"
else
    echo "Bitmain USB device: NOT FOUND"
fi
echo ""

echo "3. Kernel Module Test:"
echo "---------------------"
if lsmod | grep -q bitmain; then
    echo "Bitmain module: LOADED"
else
    echo "Bitmain module: NOT LOADED"
fi
echo ""

echo "4. CGMiner API Test:"
echo "-------------------"
if nc -z 127.0.0.1 4028 2>/dev/null; then
    echo "CGMiner API (4028): LISTENING"
    echo '{"command":"summary"}' | nc 127.0.0.1 4028 | head -c 500
else
    echo "CGMiner API (4028): NOT LISTENING"
fi
echo ""

if nc -z 127.0.0.1 4029 2>/dev/null; then
    echo "BMMiner API (4029): LISTENING"
else
    echo "BMMiner API (4029): NOT LISTENING"
fi
echo ""

echo "5. Device Read Test:"
echo "-------------------"
if [ -c "$DEVICE" ]; then
    # Try to read a few bytes with timeout
    timeout 2 dd if=$DEVICE bs=64 count=1 2>&1 | xxd | head -5 || echo "Read timed out or failed"
else
    echo "Skipped (device not available)"
fi
echo ""

echo "Communication test complete."
`
}

// ============================================================================
// Helper methods
// ============================================================================

// runRemoteCommand executes a command on the remote device
func (d *Deployer) runRemoteCommand(cmd string) (string, error) {
	if d.sshClient == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := d.sshClient.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
}

// uploadFile uploads content to a remote file
func (d *Deployer) uploadFile(remotePath string, content []byte) error {
	if d.sshClient == nil {
		return fmt.Errorf("not connected")
	}

	session, err := d.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	cmd := fmt.Sprintf("cat > %s", remotePath)
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	if _, err := stdin.Write(content); err != nil {
		return fmt.Errorf("failed to write content: %w", err)
	}
	stdin.Close()

	return session.Wait()
}

// addResult adds a phase result to the results list
func (d *Deployer) addResult(result PhaseResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.results = append(d.results, result)
}

// SaveResults saves all results to a JSON file
func (d *Deployer) SaveResults(filename string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := json.MarshalIndent(d.results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Cleanup removes temporary files and stops hasher-server from the device
func (d *Deployer) Cleanup() error {
	if d.sshClient == nil {
		return nil
	}

	d.log("Stopping hasher-server on device...")
	// Stop hasher-server process
	d.runRemoteCommand("killall hasher-server 2>/dev/null || true")

	d.log("Removing deployed binaries...")
	// Remove hasher-server and temporary scripts
	d.runRemoteCommand(fmt.Sprintf("rm -f %s/hasher-server %s/*.sh 2>/dev/null || true", d.config.RemoteDir, d.config.RemoteDir))

	// Also clean up the log file
	d.runRemoteCommand(fmt.Sprintf("rm -f %s/hasher-server.log 2>/dev/null || true", d.config.RemoteDir))

	d.log("Cleanup complete")
	return nil
}

// CompileMIPSBinary compiles a Go source file for MIPS architecture
func (d *Deployer) CompileMIPSBinary(sourcePath, outputName string) (string, error) {
	outputPath := filepath.Join(d.config.WorkDir, outputName)

	d.log("Compiling %s for MIPS...", sourcePath)

	cmd := exec.Command("go", "build", "-o", outputPath, sourcePath)
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=mips",
		"GOMIPS=softfloat",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("compilation failed: %w: %s", err, stderr.String())
	}

	d.log("Compiled successfully: %s", outputPath)
	return outputPath, nil
}

// DeployBinary deploys a compiled binary to the device
func (d *Deployer) DeployBinary(localPath, remoteName string) error {
	if d.sshClient == nil {
		return fmt.Errorf("not connected")
	}

	binary, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read binary: %w", err)
	}

	remotePath := filepath.Join(d.config.RemoteDir, remoteName)
	d.log("Deploying %s to %s...", remoteName, remotePath)

	if err := d.uploadFile(remotePath, binary); err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	// Make executable
	if _, err := d.runRemoteCommand(fmt.Sprintf("chmod +x %s", remotePath)); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	d.log("Deployed successfully")
	return nil
}

// RunRemoteBinary runs a deployed binary and returns its output
func (d *Deployer) RunRemoteBinary(remoteName string, timeout time.Duration) (string, error) {
	if d.sshClient == nil {
		return "", fmt.Errorf("not connected")
	}

	remotePath := filepath.Join(d.config.RemoteDir, remoteName)
	cmd := fmt.Sprintf("timeout %d %s 2>&1", int(timeout.Seconds()), remotePath)

	d.log("Running %s...", remoteName)
	output, err := d.runRemoteCommand(cmd)
	if err != nil {
		return output, fmt.Errorf("execution failed: %w", err)
	}

	return output, nil
}

// RunAllPhases runs all diagnostic phases in sequence
func (d *Deployer) RunAllPhases() ([]PhaseResult, error) {
	var results []PhaseResult

	// Phase 1: Discovery
	if result, err := d.RunDiscovery(); err != nil {
		d.log("Discovery failed: %v", err)
	} else {
		results = append(results, *result)
	}

	// Select first device if found
	if len(d.devices) > 0 {
		d.SelectDevice(0)
	} else {
		return results, fmt.Errorf("no devices found")
	}

	// Phase 2: Probe
	if result, err := d.RunProbe(); err != nil {
		d.log("Probe failed: %v", err)
	} else {
		results = append(results, *result)
	}

	// Phase 3: Protocol
	if result, err := d.RunProtocol(); err != nil {
		d.log("Protocol detection failed: %v", err)
	} else {
		results = append(results, *result)
	}

	// Phase 4: Provision
	if result, err := d.RunProvision(); err != nil {
		d.log("Provisioning failed: %v", err)
	} else {
		results = append(results, *result)
	}

	// Phase 5: Troubleshoot
	if result, err := d.RunTroubleshoot(); err != nil {
		d.log("Troubleshooting failed: %v", err)
	} else {
		results = append(results, *result)
	}

	// Phase 6: Test
	if result, err := d.RunTest(); err != nil {
		d.log("Test failed: %v", err)
	} else {
		results = append(results, *result)
	}

	// Cleanup
	d.Cleanup()

	return results, nil
}
