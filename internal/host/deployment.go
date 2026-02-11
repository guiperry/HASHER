// internal/host/deployment.go
// Package host provides hasher-host deployment and management functionality
package host

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"hasher/internal/analyzer"
	"hasher/internal/config"
	"hasher/internal/host/embedded"
	"hasher/pkg/hashing/methods/asic"
)

func getDeviceConfig() config.DeviceConfig {
	cfg, err := config.LoadDeviceConfig()
	if err != nil {
		return config.DeviceConfig{Username: "root"}
	}

	// Override with environment variables if set
	if envUser := os.Getenv("DEVICE_USERNAME"); envUser != "" {
		cfg.Username = envUser
	}
	if envPass := os.Getenv("DEVICE_PASSWORD"); envPass != "" {
		cfg.Password = envPass
	}

	if cfg.Username == "" {
		cfg.Username = "root"
	}
	return *cfg
}

// DeploymentConfig defines deployment configuration
type DeploymentConfig struct {
	AutoDeploy     bool          // Automatically deploy hasher-server if not found
	CleanupOnExit  bool          // Clean up deployed binaries on exit
	ConnectTimeout time.Duration // Timeout for server connection
	DeployTimeout  time.Duration // Timeout for deployment operations
	ForceRedeploy  bool          // Force redeployment even if server is detected as running
}

// DefaultDeploymentConfig returns default deployment configuration
func DefaultDeploymentConfig() *DeploymentConfig {
	return &DeploymentConfig{
		AutoDeploy:     true,
		CleanupOnExit:  true,
		ConnectTimeout: 30 * time.Second,
		DeployTimeout:  120 * time.Second,
		ForceRedeploy:  false, // Default to false
	}
}

// Deployer manages hasher-server deployment to ASIC devices
type Deployer struct {
	config         *DeploymentConfig
	analyzer       *analyzer.Deployer
	deployedDevice string // IP address of deployed device
	cleanupFunc    func() // Cleanup function to call on exit
}

// NewDeployer creates a new hasher-server deployer
func NewDeployer(config *DeploymentConfig) (*Deployer, error) {
	if config == nil {
		config = DefaultDeploymentConfig()
	}

	// Create analyzer for device operations
	analyzerConfig := analyzer.DefaultDeployerConfig()
	// If subnet flag is provided, override the empty default
	analyzerConfig.Subnet = "" // Start with empty to force flag usage or auto-detection
	analyzer, err := analyzer.NewDeployer(analyzerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create analyzer: %w", err)
	}

	return &Deployer{
		config:   config,
		analyzer: analyzer,
	}, nil
}

// rebootDevice reboots the target device
func (d *Deployer) rebootDevice(deviceIP string) error {
	log.Printf("Rebooting device %s...", deviceIP)

	cfg := getDeviceConfig()

	// Create SSH connection for reboot
	sshConfig := &ssh.ClientConfig{
		User: cfg.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
		HostKeyAlgorithms: []string{
			"ssh-rsa",
			"ssh-dss",
		},
	}

	client, err := ssh.Dial("tcp", deviceIP+":22", sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to device for reboot: %w", err)
	}
	defer client.Close()

	// Execute reboot command
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session for reboot: %w", err)
	}
	defer session.Close()

	if err := session.Run("reboot"); err != nil {
		return fmt.Errorf("failed to execute reboot command: %w", err)
	}

	log.Printf("Reboot command sent to %s", deviceIP)
	return nil
}

// EnsureServerDeployed ensures hasher-server is deployed and running on target device
func (d *Deployer) EnsureServerDeployed(deviceIP string) (*asic.ASICClient, error) {
	log.Printf("Ensuring hasher-server is deployed on device: %s", deviceIP)

	var client *asic.ASICClient // Declare client once
	var err error                 // Declare err once

	// If ForceRedeploy is true, skip checking existing server and always redeploy
	if !d.config.ForceRedeploy {
		// Step 1: Check if hasher-server is already running
		client, err = d.checkExistingServer(deviceIP) // Assign to already declared client, err
		if err == nil {
			log.Printf("hasher-server already running on %s", deviceIP)
			d.deployedDevice = deviceIP
			return client, nil
		}
		log.Printf("hasher-server not available on %s, attempting deployment...", deviceIP)
	} else {
		log.Printf("Force redeployment enabled. Skipping existing server check.")
	}

	// Step 2: Deploy hasher-server if auto-deploy is enabled
	if !d.config.AutoDeploy {
		return nil, fmt.Errorf("hasher-server not found and auto-deploy is disabled")
	}

	if err = d.deployHasherServer(deviceIP); err != nil { // Use = for assignment
		return nil, fmt.Errorf("deployment failed: %w", err)
	}

	// Step 3: Wait for server to start and verify connection
	client, err = d.waitForServer(deviceIP) // Assign to already declared client, err
	if err != nil {
		// If server connection fails, try reboot as last resort
		log.Printf("Server connection failed, attempting device reboot as fallback...")
		if rebootErr := d.rebootDevice(deviceIP); rebootErr != nil {
			log.Printf("Failed to reboot device: %v", rebootErr)
			return nil, fmt.Errorf("server deployment succeeded but connection failed: %w", err)
		}

		log.Printf("Device rebooted, waiting 60 seconds for startup...")
		time.Sleep(60 * time.Second)

		// After reboot, device is wiped clean - restart entire deployment from Step 2
		log.Printf("Restarting full deployment after reboot (device wipes /tmp on shutdown)...")
		if err = d.deployHasherServer(deviceIP); err != nil {
			return nil, fmt.Errorf("post-reboot deployment failed: %w", err)
		}

		// Try server connection after fresh deployment
		log.Printf("Retrying server connection after post-reboot deployment...")
		client, err = d.waitForServer(deviceIP)
		if err != nil {
			return nil, fmt.Errorf("server deployment failed even after reboot and redeployment: %w", err)
		}
	}

	d.deployedDevice = deviceIP
	log.Printf("hasher-server successfully deployed and running on %s", deviceIP)

	// Set up cleanup if configured
	if d.config.CleanupOnExit {
		d.setupCleanup(deviceIP)
	}

	return client, nil
}

// checkExistingServer checks if hasher-server is already running on device
func (d *Deployer) checkExistingServer(deviceIP string) (*asic.ASICClient, error) {
	address := fmt.Sprintf("%s:8888", deviceIP)

	client, err := asic.NewASICClient(address)
	if err != nil {
		return nil, err
	}

	// Check if client is using software fallback - this means connection failed
	// and we should trigger deployment instead of pretending server is running
	if client.IsUsingFallback() {
		client.Close()
		return nil, fmt.Errorf("could not connect to hasher-server at %s (using fallback mode)", address)
	}

	// Simple connection test
	_, err = client.GetDeviceInfo()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("server not responding: %w", err)
	}

	return client, nil
}

// deployHasherServer deploys hasher-server to target device
func (d *Deployer) deployHasherServer(deviceIP string) error {
	log.Printf("Deploying hasher-server to %s...", deviceIP)

	// Select device in analyzer - try to find it in discovered devices first
	devices := d.analyzer.GetDevices()
	targetDevice := -1
	for i, dev := range devices {
		if dev.IPAddress == deviceIP {
			targetDevice = i
			break
		}
	}

	if targetDevice == -1 {
		// Device not found in discovery, try to select it directly by IP
		log.Printf("Device %s not found in discovered devices, attempting direct selection...", deviceIP)
		if err := d.analyzer.SelectDeviceByIP(deviceIP); err != nil {
			return fmt.Errorf("failed to select device by IP %s: %w", deviceIP, err)
		}
	} else {
		// Device found in discovery, select it by index
		if err := d.analyzer.SelectDevice(targetDevice); err != nil {
			return fmt.Errorf("failed to select device: %w", err)
		}
	}

	// Get the embedded hasher-server binary content
	binaryContent, err := embedded.GetEmbeddedServerBinary()
	if err != nil {
		return fmt.Errorf("failed to get embedded hasher-server binary: %w", err)
	}

	// Use provisioning phase to deploy and start hasher-server with the embedded binary
	log.Printf("Running provisioning phase to deploy hasher-server...")
	_, err = d.analyzer.RunProvisionWithBinary(binaryContent)
	if err != nil {
		return fmt.Errorf("provisioning failed: %w", err)
	}
	return nil
}

// waitForServer waits for hasher-server to start and accepts connections
func (d *Deployer) waitForServer(deviceIP string) (*asic.ASICClient, error) {
	address := fmt.Sprintf("%s:8888", deviceIP)

	// Try connecting with exponential backoff
	backoff := 1 * time.Second
	maxBackoff := 10 * time.Second
	timeout := time.After(d.config.ConnectTimeout)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for hasher-server to start")
		default:
			client, err := asic.NewASICClient(address)
			if err == nil {
				// Test the connection
				if _, err := client.GetDeviceInfo(); err == nil {
					// CRITICAL: Verify we're NOT in fallback mode
					if !client.IsUsingFallback() {
						return client, nil
					}
					log.Printf("Server responded but client is in fallback mode - server not ready yet")
				}
				client.Close()
			}

			// Wait and retry
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// setupCleanup sets up cleanup function for deployed hasher-server
func (d *Deployer) setupCleanup(deviceIP string) {
	d.cleanupFunc = func() {
		log.Printf("Cleaning up hasher-server from device %s...", deviceIP)

		// Step 1: Download server logs before cleanup
		log.Printf("Downloading server logs before cleanup...")
		if logFile, err := d.DownloadServerLogs(deviceIP); err != nil {
			log.Printf("Warning: Failed to download server logs: %v", err)
		} else if logFile != "" {
			log.Printf("Server logs saved to: %s", logFile)
		}

		// Connect and cleanup using analyzer's cleanup
		if err := d.analyzer.Connect(); err != nil {
			log.Printf("Failed to connect for cleanup: %v", err)
			return
		}
		defer d.analyzer.Disconnect()

		// Use analyzer's cleanup which should handle hasher-server
		if err := d.analyzer.Cleanup(); err != nil {
			log.Printf("Warning: Cleanup failed: %v", err)
		}

		log.Printf("Cleanup complete for device %s", deviceIP)
	}
}

// DownloadServerLogs downloads the server logs from the device and saves them locally
func (d *Deployer) DownloadServerLogs(deviceIP string) (string, error) {
	return d.DownloadServerLogsWithPath(deviceIP, "/tmp/hasher-server.log")
}

// DownloadServerLogsWithPath downloads the server logs from a specific path on the device
func (d *Deployer) DownloadServerLogsWithPath(deviceIP, remoteLogPath string) (string, error) {
	cfg := getDeviceConfig()

	// Create logs directory if it doesn't exist
	logsDir := "./logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Generate timestamped filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("hasher-server_%s_%s.log", deviceIP, timestamp)
	localPath := filepath.Join(logsDir, filename)

	// Download logs via SSH using sshpass
	sshPassword := cfg.Password
	sshCmd := fmt.Sprintf("sshpass -p '%s' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=10 %s@%s "+
		"\"cat %s 2>/dev/null || echo 'LOG_FILE_NOT_FOUND'\"",
		sshPassword, cfg.Username, deviceIP, remoteLogPath)

	cmd := exec.Command("sh", "-c", sshCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's just that the log file doesn't exist
		if strings.Contains(string(output), "LOG_FILE_NOT_FOUND") {
			log.Printf("Server log file not found on device %s (this is normal for new deployments)", deviceIP)
			return "", nil
		}
		return "", fmt.Errorf("ssh command failed: %v (output: %s)", err, string(output))
	}

	// Check if log file was not found
	if strings.Contains(string(output), "LOG_FILE_NOT_FOUND") {
		log.Printf("Server log file not found on device %s (this is normal for new deployments)", deviceIP)
		return "", nil
	}

	// Write logs to file
	if err := os.WriteFile(localPath, output, 0644); err != nil {
		return "", fmt.Errorf("failed to write log file: %w", err)
	}

	log.Printf("Successfully downloaded server logs from %s to %s (size: %d bytes)",
		deviceIP, localPath, len(output))
	return localPath, nil
}

// Cleanup performs cleanup of deployed hasher-server
func (d *Deployer) Cleanup() {
	if d.cleanupFunc != nil {
		d.cleanupFunc()
	}
}

// GetDeployedDevice returns the IP address of the device where server was deployed
func (d *Deployer) GetDeployedDevice() string {
	return d.deployedDevice
}

// DeployWithDiscovery performs device discovery and deployment in one step
func (d *Deployer) DeployWithDiscovery() (*asic.ASICClient, error) {
	log.Printf("Performing device discovery and deployment...")

	// Run discovery
	_, err := d.analyzer.RunDiscovery()
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	devices := d.analyzer.GetDevices()
	if len(devices) == 0 {
		return nil, fmt.Errorf("no devices found")
	}

	// Use first accessible device
	for _, dev := range devices {
		if dev.Accessible {
			log.Printf("Found accessible device: %s (%s)", dev.IPAddress, dev.DeviceType)
			return d.EnsureServerDeployed(dev.IPAddress)
		}
	}

	return nil, fmt.Errorf("no accessible devices found")
}

// SetLogWriter sets the log writer for the analyzer
func (d *Deployer) SetLogWriter(w interface{}) {
	// This would need proper type matching with analyzer
	// For now, skip this implementation
}
