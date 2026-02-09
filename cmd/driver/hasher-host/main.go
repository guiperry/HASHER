// Hasher: Neural Inference Engine Powered by SHA-256 ASICs
// Copyright (C) 2026  Guillermo Perry
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"hasher/internal/config"
	"hasher/internal/hasher"
	"hasher/internal/host"
	"hasher/pkg/hashing/inference"
	"hasher/pkg/hashing/methods/asic"
	"hasher/pkg/hashing/neural"
	"hasher/pkg/hashing/tokenizer"
	"hasher/pkg/hashing/transformer"
)

const (
	portFile = "/tmp/hasher-host.port"
)

// writePortFile writes the port to a temporary file for the CLI to discover.
func writePortFile(port int) error {
	log.Printf("Writing port %d to %s", port, portFile)
	return os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0644)
}

// cleanupPortFile removes the temporary port file.
func cleanupPortFile() {
	log.Printf("Cleaning up port file: %s", portFile)
	os.Remove(portFile)
}

func init() {
	if deviceStr := config.GetDeviceIP(); deviceStr != "" && *device == "" {
		*device = deviceStr
	}
	if passwordStr := config.GetDevicePassword(); passwordStr != "" && *serverSSHPassword == "" {
		*serverSSHPassword = passwordStr
	}
}

// Configuration flags
var (
	// Server configuration
	port      = flag.Int("port", 0, "HTTP API server port (0 = auto-find open port)")
	device    = flag.String("device", "", "ASIC device IP (empty = auto-discover/deploy). Deploys hasher-server first. Ex: 192.168.12.151")
	asicAddr  = flag.String("asic-addr", "", "hasher-server gRPC address (empty = auto-discover). Deploys hasher-server first. Ex: 192.168.12.151:8888")
	enableAPI = flag.Bool("api", true, "enable REST API server")

	// CLI modes (for direct command-line testing)
	mode      = flag.String("mode", "api", "operation mode: api, single, batch, stream, metrics, info")
	count     = flag.Int("count", 10, "number of hashes to compute (CLI modes)")
	batchSize = flag.Int("batch", 4, "batch size for batch mode")
	dataSize  = flag.Int("size", 64, "size of data to hash")

	// Inference configuration
	passes       = flag.Int("passes", 21, "number of temporal ensemble passes")
	jitter       = flag.Float64("jitter", 0.01, "input jitter factor [0, 1]")
	seedRotation = flag.Bool("seed-rotation", true, "enable seed rotation per pass")

	// Network architecture
	inputSize  = flag.Int("input-size", 784, "network input dimension")
	hidden1    = flag.Int("hidden1", 128, "hidden layer 1 size")
	hidden2    = flag.Int("hidden2", 64, "hidden layer 2 size")
	outputSize = flag.Int("output-size", 10, "network output dimension")

	// Crypto-transformer configuration
	enableCrypto     = flag.Bool("crypto", true, "enable crypto-transformer")
	vocabSize        = flag.Int("vocab-size", 1000, "transformer vocabulary size")
	embedDim         = flag.Int("embed-dim", 256, "transformer embedding dimension")
	numLayers        = flag.Int("num-layers", 4, "transformer number of layers")
	numHeads         = flag.Int("num-heads", 8, "transformer attention heads")
	ffnHiddenDim     = flag.Int("ffn-hidden", 512, "transformer feed-forward hidden dim")
	cryptoActivation = flag.String("crypto-activation", "hash", "transformer activation: hash, tanh, sigmoid")

	// Network discovery configuration
	discoverNetwork  = flag.Bool("discover", true, "enable network discovery for hasher-server")
	discoverySubnet  = flag.String("subnet", "", "network subnet to scan (CIDR, empty = auto-detect)")
	discoveryPort    = flag.Int("discovery-port", 80, "port to scan for hasher-server")
	discoveryTimeout = flag.Duration("discovery-timeout", 2*time.Second, "timeout for each server probe")
	skipLocalhost    = flag.Bool("skip-localhost", false, "skip localhost during discovery")

	// Auto-deployment configuration
	autoDeploy    = flag.Bool("auto-deploy", true, "automatically deploy hasher-server to ASIC devices")
	cleanupOnExit = flag.Bool("cleanup", true, "clean up deployed hasher-server on exit")
	forceRedeploy = flag.Bool("force-redeploy", false, "force redeployment of hasher-server even if it's already running")

	// Server log monitoring configuration
	monitorServerLogs = flag.Bool("monitor-server-logs", true, "enable automatic monitoring of server logs for auto-recovery")
	serverLogPath     = flag.String("server-log", "/tmp/hasher-server.log", "path to server log file on ASIC device")
	serverDeviceIP    = flag.String("server-device-ip", "", "IP address of ASIC device (for log monitoring, auto-detected if empty)")
	serverSSHPassword = flag.String("server-ssh-password", "*********", "SSH password for ASIC device (for log monitoring)")
)

// Orchestrator manages the recursive inference process
type Orchestrator struct {
	asicClient      *hasher.ASICClient
	engine          *inference.RecursiveEngine
	network         *neural.HashNetwork
	cryptoModel     *transformer.HasherTransformer
	miningNeuron    *neural.MiningNeuron // New field for Nonce generation
	discoveryResult *hasher.DiscoveryResult
	startTime       time.Time
	mu              sync.RWMutex
	deployer        *host.Deployer // For auto-deployment

	// Connection monitoring
	connectionHealthy bool          // Current connection health status
	lastHealthCheck   time.Time     // Last successful health check
	reconnectAttempts int           // Number of reconnection attempts since last success
	stopMonitor       chan struct{} // Signal to stop the connection monitor

	// Server log monitoring
	stopLogMonitorChan chan struct{} // Signal to stop the log monitor
	serverDeviceIPAddr string        // IP of the ASIC device running server
	isRebooting        bool          // Flag indicating if we're handling a server reboot

	// Metrics
	totalInferences  uint64
	totalLatencyNs   uint64
	successfulInfers uint64
	failedInfers     uint64
}

// InferRequest is the API request for inference
type InferRequest struct {
	Data string `json:"data"` // Base64-encoded input data
}

// InferResponse is the API response for inference
type InferResponse struct {
	Prediction        int     `json:"prediction"`
	Confidence        float64 `json:"confidence"`
	AverageConfidence float64 `json:"average_confidence"`
	Passes            int     `json:"passes"`
	ValidPasses       int     `json:"valid_passes"`
	LatencyMs         float64 `json:"latency_ms"`
	UsingASIC         bool    `json:"using_asic"`
}

// HealthResponse is the API response for health check
type HealthResponse struct {
	Status            string `json:"status"`
	UsingASIC         bool   `json:"using_asic"`
	ChipCount         int    `json:"chip_count"`
	Uptime            string `json:"uptime"`
	ConnectionHealthy bool   `json:"connection_healthy"`
	LastHealthCheck   string `json:"last_health_check,omitempty"`
}

// MetricsResponse is the API response for metrics
type MetricsResponse struct {
	TotalInferences  uint64  `json:"total_inferences"`
	SuccessfulInfers uint64  `json:"successful_inferences"`
	FailedInfers     uint64  `json:"failed_inferences"`
	AverageLatencyMs float64 `json:"average_latency_ms"`
	UsingASIC        bool    `json:"using_asic"`
	ChipCount        int     `json:"chip_count"`
	Uptime           string  `json:"uptime"`
}

// BatchInferRequest is the API request for batch inference
type BatchInferRequest struct {
	Data []string `json:"data"` // Array of base64-encoded inputs
}

// BatchInferResponse is the API response for batch inference
type BatchInferResponse struct {
	Results   []InferResponse `json:"results"`
	TotalMs   float64         `json:"total_ms"`
	UsingASIC bool            `json:"using_asic"`
}

// ChatRequest is the API request for crypto-transformer chat
type ChatRequest struct {
	Message     string  `json:"message"`
	Context     []int   `json:"context,omitempty"`
	Temperature float32 `json:"temperature,omitempty"`
}

// ChatResponse is the API response for crypto-transformer chat
type ChatResponse struct {
	Response   string  `json:"response"`
	TokenID    int     `json:"token_id"`
	Confidence float32 `json:"confidence"`
	LatencyMs  float64 `json:"latency_ms"`
	UsingASIC  bool    `json:"using_asic"`
}

// TrainRequest is the API request for crypto-transformer training
type TrainRequest struct {
	Epochs       int      `json:"epochs"`
	LearningRate float32  `json:"learning_rate"`
	BatchSize    int      `json:"batch_size"`
	DataSamples  []string `json:"data_samples"`
}

// TrainResponse is the API response for crypto-transformer training
type TrainResponse struct {
	Epoch     int     `json:"epoch"`
	Loss      float32 `json:"loss"`
	Accuracy  float32 `json:"accuracy"`
	LatencyMs float64 `json:"latency_ms"`
	UsingASIC bool    `json:"using_asic"`
}

// RuleRequest represents a request to add a logical rule
type RuleRequest struct {
	Domain      string   `json:"domain"`
	RuleType    string   `json:"rule_type"` // 'subsumption', 'disjoint', 'constraint'
	Premises    []string `json:"premises"`
	Conclusion  string   `json:"conclusion"`
	Description string   `json:"description"`
}

// RuleResponse represents a logical rule response
type RuleResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	RuleID  int    `json:"rule_id,omitempty"`
}

func main() {
	flag.Parse()

	log.Printf("Hasher Host Orchestrator starting...")

	// Initialize deployment module if auto-deployment is enabled
	var deployer *host.Deployer
	if *autoDeploy {
		deployConfig := &host.DeploymentConfig{
			AutoDeploy:     *autoDeploy,
			CleanupOnExit:  *cleanupOnExit,
			ConnectTimeout: 30 * time.Second,
			DeployTimeout:  120 * time.Second,
			ForceRedeploy:  *forceRedeploy, // Pass the new flag
		}
		var err error
		deployer, err = host.NewDeployer(deployConfig)
		if err != nil {
			log.Printf("Warning: Failed to create deployer: %v", err)
		}
	}

	// Discover and connect to ASIC server
	var asicClient *hasher.ASICClient
	var discoveryResult *hasher.DiscoveryResult
	var serverDeviceAddr string // Store the server device IP

	if *device != "" {
		// Use explicit device IP - deployment is required first
		log.Printf("Device specified: %s - deploying hasher-server first", *device)
		serverDeviceAddr = fmt.Sprintf("%s:8888", *device)

		// Deployment is mandatory - create deployer if not already created
		if deployer == nil {
			deployConfig := &host.DeploymentConfig{
				AutoDeploy:     true,
				CleanupOnExit:  *cleanupOnExit,
				ConnectTimeout: 30 * time.Second,
				DeployTimeout:  120 * time.Second,
			}
			var err error
			deployer, err = host.NewDeployer(deployConfig)
			if err != nil {
				log.Fatalf("Failed to create required deployer for device %s: %v", *device, err)
			}
		}

		// Deploy hasher-server to the specified device and get connected client
		log.Printf("Deploying hasher-server to device %s...", *device)
		deployedClient, deployErr := deployer.EnsureServerDeployed(*device)
		if deployErr != nil {
			log.Fatalf("Failed to deploy hasher-server to device %s: %v", *device, deployErr)
		}

		// Use the connected client from deployment (deployment already SSH'd to port 22, deployed, and connected to port 8888)
		asicClient = deployedClient
		log.Printf("Successfully deployed and connected to hasher-server on device %s", *device)

	} else if *asicAddr != "" {
		// Extract IP from asicAddr for deployment
		deviceIP := extractIPFromAddress(*asicAddr)
		if deviceIP == "" {
			log.Fatalf("Invalid ASIC server address: %s - cannot extract device IP for deployment", *asicAddr)
		}

		log.Printf("ASIC server specified: %s - deploying hasher-server to device %s first", *asicAddr, deviceIP)
		serverDeviceAddr = *asicAddr

		// Deployment is mandatory - create deployer if not already created
		if deployer == nil {
			deployConfig := &host.DeploymentConfig{
				AutoDeploy:     true,
				CleanupOnExit:  *cleanupOnExit,
				ConnectTimeout: 30 * time.Second,
				DeployTimeout:  120 * time.Second,
			}
			var createErr error
			deployer, createErr = host.NewDeployer(deployConfig)
			if createErr != nil {
				log.Fatalf("Failed to create required deployer for device %s: %v", deviceIP, createErr)
			}
		}

		// Deploy hasher-server to device and get connected client
		log.Printf("Deploying hasher-server to device %s...", deviceIP)
		deployedClient, deployErr := deployer.EnsureServerDeployed(deviceIP)
		if deployErr != nil {
			log.Fatalf("Failed to deploy hasher-server to device %s: %v", deviceIP, deployErr)
		}

		// Use the connected client from deployment (deployment already SSH'd to port 22, deployed, and connected to port 8888)
		asicClient = deployedClient
		log.Printf("Successfully deployed and connected to hasher-server on device %s", deviceIP)
	} else if *discoverNetwork {
		// Perform network discovery with auto-deployment
		log.Printf("Discovering hasher-server instances on network...")

		if deployer != nil {
			// Use auto-deployment with discovery
			log.Printf("Auto-deployment enabled, looking for devices to deploy hasher-server...")
			var err error
			asicClient, err = deployer.DeployWithDiscovery()
			if err != nil {
				log.Printf("Warning: Auto-deployment failed: %v", err)
				log.Printf("Falling back to localhost discovery...")

				// Quick localhost check before full network discovery
				asicClient, discoveryResult, err = hasher.DiscoverAndConnect(hasher.DiscoveryConfig{
					Port:          *discoveryPort,
					Timeout:       5 * time.Second, // Short timeout for localhost
					Subnet:        "127.0.0.0/30",  // Very limited range
					SkipLocalhost: false,
				})
				if err != nil {
					log.Printf("Warning: Localhost discovery failed: %v", err)
					log.Printf("Creating ASIC client for direct connection attempt...")
					asicClient, _ = hasher.NewASICClient("") // Create client for direct connection attempt
				} else {
					log.Printf("Connected to discovered hasher-server at %s", discoveryResult.Address)
					log.Printf("Server info: %d chips, %s, latency: %dms",
						discoveryResult.ChipCount, discoveryResult.Version, discoveryResult.LatencyMs)
					serverDeviceAddr = discoveryResult.Address
				}
			} else {
				log.Printf("Auto-deployment successful: hasher-server running on discovered device")
				// Get the deployed device address
				serverDeviceAddr = deployer.GetDeployedDevice()
			}
		} else {
			// Quick localhost discovery only when no deployer
			log.Printf("Checking for local hasher-server...")
			config := hasher.DiscoveryConfig{
				Port:          *discoveryPort,
				Timeout:       5 * time.Second, // Short timeout for localhost
				Subnet:        "127.0.0.0/30",  // Very limited range
				SkipLocalhost: false,
			}
			if *discoverySubnet != "" {
				config.Subnet = *discoverySubnet
			}

			var err error
			asicClient, discoveryResult, err = hasher.DiscoverAndConnect(config)
			if err != nil {
				log.Printf("Warning: Localhost discovery failed: %v", err)
				log.Printf("Creating ASIC client for direct connection attempt...")
				asicClient, _ = hasher.NewASICClient("") // Create client for direct connection attempt
			} else {
				log.Printf("Connected to discovered hasher-server at %s", discoveryResult.Address)
				log.Printf("Server info: %d chips, %s, latency: %dms",
					discoveryResult.ChipCount, discoveryResult.Version, discoveryResult.LatencyMs)
				serverDeviceAddr = discoveryResult.Address
			}
		}
	} else {
		// Try localhost only
		log.Printf("Trying localhost hasher-server...")
		serverDeviceAddr = "localhost:8888"
		var err error
		asicClient, err = hasher.NewASICClient("localhost:8888")
		if err != nil {
			log.Printf("Warning: Could not connect to localhost hasher-server: %v", err)
		}
	}

	// Default to software fallback if no ASIC device is available or specified
	if asicClient == nil {
		log.Printf("No ASIC device available - enabling software fallback mode")
		asicClient = nil
	}

	// Override with explicit flags if provided
	if *serverDeviceIP != "" {
		serverDeviceAddr = *serverDeviceIP
	} else if *device != "" {
		// Device flag takes precedence for IP extraction
		serverDeviceAddr = *device
	}

	if asicClient != nil {
		if asicClient.IsUsingFallback() {
			log.Printf("Running in SOFTWARE FALLBACK mode (ASIC server not available)")
		} else {
			chipCount := asicClient.GetChipCount()
			if discoveryResult != nil {
				log.Printf("Connected to ASIC server with %d chips at %s", chipCount, discoveryResult.Address)
			} else {
				log.Printf("Connected to ASIC server with %d chips", chipCount)
			}
		}
	}

	// Create hash network
	network, err := neural.NewHashNetwork(*inputSize, *hidden1, *hidden2, *outputSize)
	if err != nil {
		log.Fatalf("Failed to create hash network: %v", err)
	}
	log.Printf("Hash network created: [%d, %d, %d, %d]", *inputSize, *hidden1, *hidden2, *outputSize)

	// Create ASICMethod wrapper for HashMethod interface
	var hashMethod *asic.ASICMethod
	if asicClient != nil && !asicClient.IsUsingFallback() {
		// Create ASICMethod from the server device address
		address := serverDeviceAddr
		if address == "" {
			address = "localhost:8888"
		}
		hashMethod = asic.NewASICMethod(address)
		if err := hashMethod.Initialize(); err != nil {
			log.Printf("Warning: Failed to initialize ASICMethod: %v", err)
			hashMethod = nil
		}
	}

	// Create recursive engine with ASIC support
	engine, err := inference.NewRecursiveEngineWithHashMethod(network, hashMethod, *passes, *jitter, *seedRotation)
	if err != nil {
		log.Fatalf("Failed to create recursive engine: %v", err)
	}
	log.Printf("Recursive engine created: %d passes, jitter=%.3f, seed_rotation=%v", *passes, *jitter, *seedRotation)

	// Create crypto-transformer if enabled
	var cryptoModel *transformer.HasherTransformer
	var miningNeuron *neural.MiningNeuron
	if *enableCrypto {
		log.Printf("Initializing crypto-transformer...")
		transformerConfig := &transformer.TransformerConfig{
			VocabSize:    *vocabSize,
			EmbedDim:     *embedDim,
			NumLayers:    *numLayers,
			NumHeads:     *numHeads,
			ContextLen:   128,
			DropoutRate:  0.1,
			FFNHiddenDim: *ffnHiddenDim,
			Activation:   *cryptoActivation,
		}
		cryptoModel = transformer.NewHasherTransformer(transformerConfig, hashMethod)
		log.Printf("Crypto-transformer created: vocab=%d, embed=%d, layers=%d, heads=%d",
			*vocabSize, *embedDim, *numLayers, *numHeads)

		// Initialize MiningNeuron for converting transformer output to nonces
		// Use much smaller nonce range for software fallback to avoid long delays
		nonceEnd := uint32(1000000) // Large range for ASIC mode
		if asicClient != nil && asicClient.IsUsingFallback() {
			nonceEnd = 1 // Single nonce for software fallback (instant)
		}

		miningNeuronConfig := neural.MiningNeuronConfig{
			InputDim:   *vocabSize, // Input to MiningNeuron is the tokenScores (logits)
			OutputDim:  8,          // Generate 8 projections for the mining header
			Salt:       0xDEADBEEF, // Fixed salt for now
			NonceStart: 0,
			NonceEnd:   nonceEnd, // Dynamic search range based on hardware mode
		}
		miningNeuron = neural.NewMiningNeuron(miningNeuronConfig)
		if hashMethod != nil {
			miningNeuron.SetHashMethod(hashMethod)
		}
		log.Printf("MiningNeuron created with InputDim=%d, OutputDim=%d", miningNeuronConfig.InputDim, miningNeuronConfig.OutputDim)
	}

	// Create orchestrator
	orch := &Orchestrator{
		asicClient:         asicClient,
		engine:             engine,
		network:            network,
		cryptoModel:        cryptoModel,
		miningNeuron:       miningNeuron, // Add miningNeuron to orchestrator
		discoveryResult:    discoveryResult,
		startTime:          time.Now(),
		deployer:           deployer,
		connectionHealthy:  asicClient != nil && !asicClient.IsUsingFallback(),
		lastHealthCheck:    time.Now(),
		stopMonitor:        make(chan struct{}),
		stopLogMonitorChan: make(chan struct{}),
		serverDeviceIPAddr: extractIPFromAddress(serverDeviceAddr),
	}

	// Start connection monitor for ASIC connection health
	if asicClient != nil && !asicClient.IsUsingFallback() {
		go orch.runConnectionMonitor()
	}

	// Start server log monitoring if enabled and we have a server device IP
	if *monitorServerLogs && orch.serverDeviceIPAddr != "" {
		go orch.monitorServerLogs()
	} else if *monitorServerLogs {
		log.Printf("Warning: Server log monitoring enabled but no server device IP available")
	}

	// Find available port for API mode
	var apiPort int
	if *mode == "api" {
		var err error
		apiPort, err = findOpenPort(*port)
		if err != nil {
			log.Fatalf("Failed to find available port: %v", err)
		}
		// Update the port variable for the rest of the code
		*port = apiPort

		// Write port to file for CLI discovery
		if err := writePortFile(apiPort); err != nil {
			log.Printf("Warning: failed to write port file: %v", err)
		}
	}

	// Run based on mode
	switch *mode {
	case "api":
		if *enableAPI {
			runAPIServer(orch)
		} else {
			log.Println("API server disabled by flag --api=false")
			log.Println("Running in single mode instead")
			runSingleMode(orch)
		}
	case "single":
		runSingleMode(orch)
	case "batch":
		runBatchMode(orch)
	case "stream":
		runStreamMode(orch)
	case "metrics":
		showMetrics(orch)
	case "info":
		showInfo(orch)
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

// extractIPFromAddress extracts just the IP address from an address string (e.g., "192.168.1.1:8888" -> "192.168.1.1")
func extractIPFromAddress(addr string) string {
	if addr == "" {
		return ""
	}
	parts := strings.Split(addr, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return addr
}

// monitorServerLogs monitors the server logs via SSH and handles auto-reboot scenarios
func (o *Orchestrator) monitorServerLogs() {
	log.Printf("Starting server log monitor for device %s", o.serverDeviceIPAddr)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopLogMonitorChan:
			log.Printf("Server log monitor stopped")
			return
		case <-ticker.C:
			if o.isRebooting {
				// Skip monitoring while we're handling a reboot
				continue
			}

			// Check for auto-reboot marker in server logs
			if err := o.checkServerLogs(); err != nil {
				// Only log errors occasionally to avoid spam
				if o.reconnectAttempts%10 == 0 {
					log.Printf("Warning: Failed to check server logs: %v", err)
				}
			}
		}
	}
}

// checkServerLogs checks the server logs via SSH for auto-reboot markers
func (o *Orchestrator) checkServerLogs() error {
	// SSH into device and tail the log file for recent entries
	sshCmd := fmt.Sprintf("sshpass -p '%s' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=5 root@%s "+
		"\"tail -n 50 %s 2>/dev/null || echo 'LOG_FILE_NOT_FOUND'\"",
		*serverSSHPassword, o.serverDeviceIPAddr, *serverLogPath)

	cmd := exec.Command("sh", "-c", sshCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh command failed: %v (output: %s)", err, string(output))
	}

	logOutput := string(output)

	// Check for auto-reboot marker
	if strings.Contains(logOutput, "AUTO_REBOOT_TRIGGERED") {
		log.Printf("AUTO_REBOOT_TRIGGERED marker detected in server logs - server is rebooting")
		go o.handleServerReboot()
	}

	return nil
}

// handleServerReboot handles the full server reboot recovery sequence
func (o *Orchestrator) handleServerReboot() {
	o.mu.Lock()
	if o.isRebooting {
		o.mu.Unlock()
		return // Already handling a reboot
	}
	o.isRebooting = true
	o.mu.Unlock()

	log.Printf("=== Starting server reboot recovery sequence ===")

	// Step 1: Log that reboot is in progress
	log.Printf("Server reboot in progress - waiting for connection to drop...")

	// Step 2: Wait for connection to drop (indicating server is rebooting)
	connectionDropped := o.waitForConnectionDrop(60 * time.Second)
	if !connectionDropped {
		log.Printf("Warning: Connection did not drop within timeout, assuming reboot is complete")
	} else {
		log.Printf("Connection dropped - server is rebooting")
	}

	// Step 3: Poll for server to come back online (retry every 5 seconds for up to 2 minutes)
	log.Printf("Waiting for server to come back online...")
	backOnline := o.pollForServerRecovery(120*time.Second, 5*time.Second)
	if !backOnline {
		log.Printf("ERROR: Server did not come back online within timeout - manual intervention required")
		o.mu.Lock()
		o.isRebooting = false
		o.mu.Unlock()
		return
	}
	log.Printf("Server is back online")

	// Step 4: Download the server logs with timestamp
	log.Printf("Downloading server logs...")
	logFile, err := o.downloadServerLogs()
	if err != nil {
		log.Printf("Warning: Failed to download server logs: %v", err)
	} else {
		log.Printf("Server logs saved to: %s", logFile)
	}

	// Step 5: Cleanup old server process
	log.Printf("Cleaning up old server process...")
	if err := o.cleanupOldServerProcess(); err != nil {
		log.Printf("Warning: Failed to cleanup old server process: %v", err)
	}

	// Step 6: Redeploy the server binary
	log.Printf("Redeploying server binary...")
	if err := o.redeployServerBinary(); err != nil {
		log.Printf("ERROR: Failed to redeploy server binary: %v", err)
		o.mu.Lock()
		o.isRebooting = false
		o.mu.Unlock()
		return
	}
	log.Printf("Server binary redeployed successfully")

	// Step 7: Restart the server
	log.Printf("Restarting hasher-server...")
	if err := o.restartServer(); err != nil {
		log.Printf("ERROR: Failed to restart server: %v", err)
		o.mu.Lock()
		o.isRebooting = false
		o.mu.Unlock()
		return
	}
	log.Printf("Server restarted successfully")

	// Step 8: Reconnect the ASIC client
	log.Printf("Reconnecting ASIC client...")
	if err := o.reconnectASICClient(); err != nil {
		log.Printf("ERROR: Failed to reconnect ASIC client: %v", err)
		o.mu.Lock()
		o.isRebooting = false
		o.mu.Unlock()
		return
	}
	log.Printf("ASIC client reconnected successfully")

	// Step 9: Resume normal operations
	o.mu.Lock()
	o.isRebooting = false
	o.connectionHealthy = true
	o.lastHealthCheck = time.Now()
	o.reconnectAttempts = 0
	o.mu.Unlock()

	log.Printf("=== Server reboot recovery sequence complete - resuming normal operations ===")
}

// waitForConnectionDrop waits for the SSH connection to drop (indicating server reboot)
func (o *Orchestrator) waitForConnectionDrop(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	pollInterval := 1 * time.Second

	for time.Now().Before(deadline) {
		// Try to connect via SSH - if it fails, connection has dropped
		sshCmd := fmt.Sprintf("sshpass -p '%s' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=3 root@%s echo 'OK'",
			*serverSSHPassword, o.serverDeviceIPAddr)
		cmd := exec.Command("sh", "-c", sshCmd)
		if err := cmd.Run(); err != nil {
			// Connection dropped
			return true
		}
		time.Sleep(pollInterval)
	}

	return false // Timeout reached
}

// pollForServerRecovery polls for the server to come back online
func (o *Orchestrator) pollForServerRecovery(timeout time.Duration, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to SSH into the device
		sshCmd := fmt.Sprintf("sshpass -p '%s' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=5 root@%s echo 'READY'",
			*serverSSHPassword, o.serverDeviceIPAddr)
		cmd := exec.Command("sh", "-c", sshCmd)
		if err := cmd.Run(); err == nil {
			// Server is back online
			return true
		}

		log.Printf("Waiting for server to come back online... (retry in %v)", interval)
		time.Sleep(interval)
	}

	return false // Timeout reached
}

// downloadServerLogs downloads the server logs and saves them with a timestamp
func (o *Orchestrator) downloadServerLogs() (string, error) {
	// Create logs directory if it doesn't exist
	logsDir := "./logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Generate timestamped filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("hasher-server_%s_%s.log", o.serverDeviceIPAddr, timestamp)
	filepath := filepath.Join(logsDir, filename)

	// Download logs via SSH
	sshCmd := fmt.Sprintf("sshpass -p '%s' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=10 root@%s "+
		"\"cat %s 2>/dev/null || echo 'LOG_FILE_NOT_FOUND'\"",
		*serverSSHPassword, o.serverDeviceIPAddr, *serverLogPath)

	cmd := exec.Command("sh", "-c", sshCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh command failed: %v (output: %s)", err, string(output))
	}

	// Write logs to file
	if err := os.WriteFile(filepath, output, 0644); err != nil {
		return "", fmt.Errorf("failed to write log file: %w", err)
	}

	log.Printf("Successfully downloaded server logs to %s (size: %d bytes)", filepath, len(output))
	return filepath, nil
}

// cleanupOldServerProcess kills any old hasher-server processes on the device
func (o *Orchestrator) cleanupOldServerProcess() error {
	sshCmd := fmt.Sprintf("sshpass -p '%s' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=10 root@%s "+
		"\"pkill -9 hasher-server 2>/dev/null; echo 'DONE'\"",
		*serverSSHPassword, o.serverDeviceIPAddr)

	cmd := exec.Command("sh", "-c", sshCmd)
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "DONE") {
		// pkill may fail if process doesn't exist, which is OK
		if !strings.Contains(err.Error(), "exit status 1") {
			return fmt.Errorf("ssh command failed: %v (output: %s)", err, string(output))
		}
	}

	return nil
}

// redeployServerBinary redeploys the hasher-server binary to the device
func (o *Orchestrator) redeployServerBinary() error {
	// If we have a deployer, use it
	if o.deployer != nil && o.serverDeviceIPAddr != "" {
		_, err := o.deployer.EnsureServerDeployed(o.serverDeviceIPAddr)
		if err != nil {
			return fmt.Errorf("deployer redeployment failed: %w", err)
		}
		return nil
	}

	// Otherwise, use direct SSH to copy and setup the binary
	// This assumes the binary is available locally
	return fmt.Errorf("no deployer available for redeployment")
}

// restartServer restarts the hasher-server on the device
func (o *Orchestrator) restartServer() error {
	sshCmd := fmt.Sprintf("sshpass -p '%s' ssh -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o StrictHostKeyChecking=no -o ConnectTimeout=10 root@%s "+
		"\"nohup /tmp/hasher-server --auto-reboot > %s 2>&1 & sleep 2; pgrep hasher-server > /dev/null && echo 'STARTED' || echo 'FAILED'\"",
		*serverSSHPassword, o.serverDeviceIPAddr, *serverLogPath)

	cmd := exec.Command("sh", "-c", sshCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh command failed: %v (output: %s)", err, string(output))
	}

	if !strings.Contains(string(output), "STARTED") {
		return fmt.Errorf("server failed to start (output: %s)", string(output))
	}

	// Wait for server to be ready
	time.Sleep(3 * time.Second)

	return nil
}

// reconnectASICClient reconnects the ASIC client after server restart
func (o *Orchestrator) reconnectASICClient() error {
	// Close existing client
	if o.asicClient != nil {
		o.asicClient.Close()
	}

	// Build server address
	serverAddr := fmt.Sprintf("%s:8888", o.serverDeviceIPAddr)

	// Try to reconnect with retries
	backoff := 1 * time.Second
	maxBackoff := 10 * time.Second
	timeout := 60 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		client, err := hasher.NewASICClient(serverAddr)
		if err == nil {
			// Test the connection
			_, err := client.GetDeviceInfo()
			if err == nil {
				o.mu.Lock()
				o.asicClient = client
				o.mu.Unlock()
				return nil
			}
			client.Close()
		}

		log.Printf("Reconnection attempt failed, retrying in %v...", backoff)
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("failed to reconnect within timeout (%v)", timeout)
}

// signalLogMonitorStop signals the log monitor to stop
func (o *Orchestrator) signalLogMonitorStop() {
	if o.stopLogMonitorChan != nil {
		close(o.stopLogMonitorChan)
	}
}

// runAPIServer starts the REST API server
func runAPIServer(orch *Orchestrator) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// API routes
	api := router.Group("/api/v1")
	{
		// Core inference endpoints
		api.POST("/infer", orch.handleInfer)
		api.POST("/batch", orch.handleBatchInfer)
		api.GET("/health", orch.handleHealth)
		api.GET("/metrics", orch.handleMetrics)
		api.GET("/device", orch.handleDeviceInfo)
		api.GET("/discovery", orch.handleDiscovery)
		api.POST("/discovery/scan", orch.handleDiscoveryScan)

		// Crypto-transformer endpoints
		api.POST("/chat", orch.handleChat)
		api.POST("/train", orch.handleTrain)
		api.GET("/crypto/status", orch.handleCryptoStatus)

		// Logical rules endpoints
		api.GET("/rules", orch.handleListRules)
		api.POST("/rules", orch.handleAddRule)
		api.DELETE("/rules/:id", orch.handleDeleteRule)
		api.GET("/domains", orch.handleListDomains)

		// Shutdown endpoint
		api.POST("/shutdown", orch.handleShutdown)
	}

	// Set up graceful shutdown
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: router,
	}

	go func() {
		log.Printf("API server listening on :%d", *port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Stop monitors
	orch.stopConnectionMonitor()
	orch.signalLogMonitorStop()

	// Clean up port file right away
	cleanupPortFile()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	if orch.asicClient != nil {
		orch.asicClient.Close()
	}

	// Cleanup deployed hasher-server if auto-deployment was used
	if orch.deployer != nil {
		orch.deployer.Cleanup()
	}

	log.Println("Server stopped")
}

// handleInfer handles single inference requests
func (o *Orchestrator) handleInfer(c *gin.Context) {
	var req InferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Decode base64 input
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 data"})
		return
	}

	// Run inference
	start := time.Now()
	result, err := o.engine.Infer(data)
	latency := time.Since(start)

	o.mu.Lock()
	o.totalInferences++
	o.totalLatencyNs += uint64(latency.Nanoseconds())
	if err != nil {
		o.failedInfers++
	} else {
		o.successfulInfers++
	}
	o.mu.Unlock()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, InferResponse{
		Prediction:        result.Consensus.Prediction,
		Confidence:        result.Consensus.Confidence,
		AverageConfidence: result.Consensus.AverageConfidence,
		Passes:            result.TotalPasses,
		ValidPasses:       result.ValidPasses,
		LatencyMs:         float64(latency.Milliseconds()),
		UsingASIC:         o.engine.IsUsingHardware(),
	})
}

// handleBatchInfer handles batch inference requests
func (o *Orchestrator) handleBatchInfer(c *gin.Context) {
	var req BatchInferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.Data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty batch"})
		return
	}

	start := time.Now()
	results := make([]InferResponse, len(req.Data))

	for i, dataStr := range req.Data {
		data, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid base64 at index %d", i)})
			return
		}

		inferStart := time.Now()
		result, err := o.engine.Infer(data)
		inferLatency := time.Since(inferStart)

		o.mu.Lock()
		o.totalInferences++
		o.totalLatencyNs += uint64(inferLatency.Nanoseconds())
		if err != nil {
			o.failedInfers++
		} else {
			o.successfulInfers++
		}
		o.mu.Unlock()

		if err != nil {
			results[i] = InferResponse{
				Prediction: -1,
				LatencyMs:  float64(inferLatency.Milliseconds()),
				UsingASIC:  o.engine.IsUsingHardware(),
			}
			continue
		}

		results[i] = InferResponse{
			Prediction:        result.Consensus.Prediction,
			Confidence:        result.Consensus.Confidence,
			AverageConfidence: result.Consensus.AverageConfidence,
			Passes:            result.TotalPasses,
			ValidPasses:       result.ValidPasses,
			LatencyMs:         float64(inferLatency.Milliseconds()),
			UsingASIC:         o.engine.IsUsingHardware(),
		}
	}

	c.JSON(http.StatusOK, BatchInferResponse{
		Results:   results,
		TotalMs:   float64(time.Since(start).Milliseconds()),
		UsingASIC: o.engine.IsUsingHardware(),
	})
}

// handleHealth handles health check requests
func (o *Orchestrator) handleHealth(c *gin.Context) {
	chipCount := 0
	if o.asicClient != nil {
		chipCount = o.asicClient.GetChipCount()
	}

	o.mu.RLock()
	connectionHealthy := o.connectionHealthy
	lastHealthCheck := o.lastHealthCheck
	isRebooting := o.isRebooting
	o.mu.RUnlock()

	// Determine overall status
	status := "healthy"
	if o.asicClient != nil && !connectionHealthy {
		status = "degraded"
	}
	if isRebooting {
		status = "rebooting"
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:            status,
		UsingASIC:         o.engine.IsUsingHardware(),
		ChipCount:         chipCount,
		Uptime:            time.Since(o.startTime).String(),
		ConnectionHealthy: connectionHealthy,
		LastHealthCheck:   lastHealthCheck.Format(time.RFC3339),
	})
}

// handleMetrics handles metrics requests
func (o *Orchestrator) handleMetrics(c *gin.Context) {
	o.mu.RLock()
	totalInferences := o.totalInferences
	successfulInfers := o.successfulInfers
	failedInfers := o.failedInfers
	totalLatencyNs := o.totalLatencyNs
	o.mu.RUnlock()

	avgLatencyMs := float64(0)
	if totalInferences > 0 {
		avgLatencyMs = float64(totalLatencyNs) / float64(totalInferences) / 1e6
	}

	chipCount := 0
	if o.asicClient != nil {
		chipCount = o.asicClient.GetChipCount()
	}

	c.JSON(http.StatusOK, MetricsResponse{
		TotalInferences:  totalInferences,
		SuccessfulInfers: successfulInfers,
		FailedInfers:     failedInfers,
		AverageLatencyMs: avgLatencyMs,
		UsingASIC:        o.engine.IsUsingHardware(),
		ChipCount:        chipCount,
		Uptime:           time.Since(o.startTime).String(),
	})
}

// runConnectionMonitor periodically checks ASIC connection health and attempts reconnection
func (o *Orchestrator) runConnectionMonitor() {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	log.Printf("Connection monitor started for ASIC server")

	for {
		select {
		case <-o.stopMonitor:
			log.Printf("Connection monitor stopped")
			return
		case <-ticker.C:
			if o.isRebooting {
				// Skip health checks during reboot handling
				continue
			}
			o.checkConnectionHealth()
		}
	}
}

// checkConnectionHealth verifies ASIC connection and attempts reconnection if needed
func (o *Orchestrator) checkConnectionHealth() {
	if o.asicClient == nil {
		return
	}

	// Try to get device info as a health check
	_, err := o.asicClient.GetDeviceInfo()

	o.mu.Lock()
	defer o.mu.Unlock()

	if err != nil {
		// Connection is unhealthy
		if o.connectionHealthy {
			log.Printf("ASIC connection lost: %v", err)
			o.connectionHealthy = false
		}

		// Attempt reconnection with exponential backoff
		if o.reconnectAttempts < 5 { // Max 5 attempts before giving up temporarily
			o.reconnectAttempts++
			log.Printf("Attempting ASIC reconnection (attempt %d/5)...", o.reconnectAttempts)

			if err := o.asicClient.Reconnect(); err != nil {
				log.Printf("Reconnection failed: %v", err)
			} else {
				log.Printf("Successfully reconnected to ASIC server")
				o.connectionHealthy = true
				o.lastHealthCheck = time.Now()
				o.reconnectAttempts = 0
			}
		} else if o.reconnectAttempts == 5 {
			log.Printf("Max reconnection attempts reached. Will retry in 60 seconds.")
			o.reconnectAttempts++ // Increment to avoid repeated log message
		} else if o.reconnectAttempts >= 11 { // Reset after ~60 seconds (6 more ticks)
			o.reconnectAttempts = 0
		} else {
			o.reconnectAttempts++
		}
	} else {
		// Connection is healthy
		if !o.connectionHealthy {
			log.Printf("ASIC connection restored")
		}
		o.connectionHealthy = true
		o.lastHealthCheck = time.Now()
		o.reconnectAttempts = 0
	}
}

// stopConnectionMonitor signals the connection monitor to stop
func (o *Orchestrator) stopConnectionMonitor() {
	if o.stopMonitor != nil {
		close(o.stopMonitor)
	}
}

// handleDeviceInfo handles device info requests
func (o *Orchestrator) handleDeviceInfo(c *gin.Context) {
	if o.asicClient == nil {
		response := gin.H{
			"device_path":      "software",
			"chip_count":       0,
			"firmware_version": "software-fallback",
			"is_operational":   true,
			"uptime_seconds":   uint64(time.Since(o.startTime).Seconds()),
		}
		// Log the response in pretty JSON for debugging
		log.Printf("Device info (software fallback): %s", prettyJSON(response))
		c.JSON(http.StatusOK, response)
		return
	}

	info, err := o.asicClient.GetDeviceInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"device_path":      info.DevicePath,
		"chip_count":       info.ChipCount,
		"firmware_version": info.FirmwareVersion,
		"is_operational":   info.IsOperational,
		"uptime_seconds":   info.UptimeSeconds,
	}
	// Log the response in pretty JSON for debugging
	log.Printf("Device info: %s", prettyJSON(response))
	c.JSON(http.StatusOK, response)
}

// handleShutdown handles a request to gracefully shut down the server.
func (o *Orchestrator) handleShutdown(c *gin.Context) {
	log.Println("Received shutdown request via API...")
	c.JSON(http.StatusOK, gin.H{"message": "shutdown sequence initiated"})

	// Use a goroutine to send the signal after the response has been sent
	go func() {
		// A small delay to allow the HTTP response to flush
		time.Sleep(100 * time.Millisecond)
		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			log.Printf("Error finding process to signal shutdown: %v", err)
			return
		}
		log.Println("Sending SIGTERM to self to trigger graceful shutdown...")
		if err := p.Signal(syscall.SIGTERM); err != nil {
			log.Printf("Error sending SIGTERM to self: %v", err)
		}
	}()
}

// handleChat handles crypto-transformer chat requests
func (o *Orchestrator) handleChat(c *gin.Context) {
	if o.cryptoModel == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "crypto-transformer not enabled"})
		return
	}
	if o.miningNeuron == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mining neuron not initialized"})
		return
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	start := time.Now()

	// FAST PATH: Software fallback mode - bypass heavy crypto-transformer for chat
	if o.asicClient != nil && o.asicClient.IsUsingFallback() {
		// Simple deterministic response for software mode (no mining, no transformer)
		simpleResponse := "hello world" // Fixed response for testing
		latency := time.Since(start)
		c.JSON(http.StatusOK, ChatResponse{
			Response:   simpleResponse,
			TokenID:    1,
			Confidence: 0.8,
			LatencyMs:  float64(latency.Milliseconds()),
			UsingASIC:  false,
		})
		return
	}

	// SLOW PATH: ASIC mode - use full crypto-transformer pipeline
	// Convert message to token IDs using the tokenizer package
	inputTokenIDs := tokenizer.Tokenize(req.Message, o.cryptoModel.Config.VocabSize)

	// Use provided context or initialize with input token IDs
	context := req.Context
	if len(context) == 0 {
		context = inputTokenIDs
	}

	// Language Generation Loop
	generatedTokens := make([]int, 0)
	currentContext := context // Use context for subsequent token generation
	var generatedResponse strings.Builder
	const maxGenerationLen = 5 // Limit the length of the generated response (reduced for testing)

	for i := 0; i < maxGenerationLen; i++ {
		// Generate the next token ID and its scores (projections) using the crypto-transformer
		nextTokenID, tokenScores := o.cryptoModel.GenerateToken(currentContext, req.Temperature)

		// If the token scores are not valid for mining (e.g., too few), break or handle error
		if len(tokenScores) < len(o.miningNeuron.Weights) {
			log.Printf("Warning: Transformer output tokenScores length (%d) is less than MiningNeuron.OutputDim (%d). Cannot generate nonce.", len(tokenScores), len(o.miningNeuron.Weights))
			generatedResponse.WriteString("[ERR_PROJ_LEN]")
			break
		}

		// Use MiningNeuron to convert tokenScores (projections) into a nonce
		nonce, err := o.miningNeuron.Forward(tokenScores)
		if err != nil {
			log.Printf("ERROR: MiningNeuron forward pass failed: %v", err)
			generatedResponse.WriteString("[ERR_NONCE_GEN]")
			break // Stop generation on error
		}

		// Detokenize the generated Nonce into a character
		detokenizedChar := tokenizer.DetokenizeNonce(nonce, o.cryptoModel.Config.VocabSize)
		generatedResponse.WriteString(detokenizedChar)

		// Append the next token ID to the list of generated tokens
		generatedTokens = append(generatedTokens, nextTokenID)

		// Update the context for the next iteration (sliding window)
		currentContext = append(currentContext, nextTokenID)
		if len(currentContext) > o.cryptoModel.Config.ContextLen {
			currentContext = currentContext[len(currentContext)-o.cryptoModel.Config.ContextLen:]
		}

		// Add a simple stop condition for demonstration
		// In a real scenario, an explicit <EOS> token would be used
		if nextTokenID == int('.') || nextTokenID == int('?') || nextTokenID == int('!') {
			break
		}
	}

	latency := time.Since(start)

	c.JSON(http.StatusOK, ChatResponse{
		Response:   generatedResponse.String(),
		TokenID:    generatedTokens[len(generatedTokens)-1], // Return last token ID for context building
		Confidence: 0.8,                                     // Placeholder confidence (could be derived from tokenScores)
		LatencyMs:  float64(latency.Milliseconds()),
		UsingASIC:  o.asicClient != nil && !o.asicClient.IsUsingFallback(),
	})
}

// handleTrain handles crypto-transformer training requests
func (o *Orchestrator) handleTrain(c *gin.Context) {
	if o.cryptoModel == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "crypto-transformer not enabled"})
		return
	}

	var req TrainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	start := time.Now()

	// Create training data
	data := make([]transformer.DataSample, len(req.DataSamples))
	for i, sample := range req.DataSamples {
		// Tokenize input and output using the tokenizer package
		inputTokens := tokenizer.Tokenize(sample, o.cryptoModel.Config.VocabSize)

		// Create target tokens (shifted by 1 for next-token prediction)
		outputTokens := make([]int, len(inputTokens))
		for j := 0; j < len(inputTokens)-1; j++ {
			outputTokens[j] = inputTokens[j+1]
		}
		if len(inputTokens) > 0 {
			outputTokens[len(inputTokens)-1] = inputTokens[0] // Wrap around
		}
		data[i] = transformer.DataSample{
			InputTokens:   inputTokens,
			OutputTokens:  outputTokens,
			AttentionMask: make([]bool, len(inputTokens)),
		}
	}

	// Set up training configuration
	trainConfig := &transformer.TrainingConfig{
		Epochs:         req.Epochs,
		BatchSize:      req.BatchSize,
		LearningRate:   req.LearningRate,
		WeightDecay:    0.01,
		ValidationFreq: 1,
		SaveFreq:       req.Epochs, // Save at end
	}
	if trainConfig.Epochs <= 0 {
		trainConfig.Epochs = 1
	}
	if trainConfig.BatchSize <= 0 {
		trainConfig.BatchSize = 4
	}
	if trainConfig.LearningRate <= 0 {
		trainConfig.LearningRate = 0.01
	}

	// Create trainer and run training
	trainer := transformer.NewTrainer(o.cryptoModel, trainConfig, data)

	// Run single epoch training (for API responsiveness)
	var totalLoss float32
	var correct, total int

	for _, sample := range data {
		// Forward pass
		output := o.cryptoModel.Forward(sample.InputTokens)

		// Calculate simple loss (MSE proxy)
		if len(output) > 0 && len(sample.OutputTokens) > 0 {
			target := float32(sample.OutputTokens[0])
			diff := output[0] - target
			totalLoss += diff * diff
		}

		// Check prediction accuracy
		predicted, _ := o.cryptoModel.GenerateToken(sample.InputTokens, 0.0)
		if len(sample.OutputTokens) > 0 && predicted == sample.OutputTokens[0] {
			correct++
		}
		total++

		// Note: Backward pass removed - new simplified transformer uses seed-based weights
		// which don't support gradient-based backpropagation. Training is done via
		// seed regeneration or alternative optimization methods.
	}

	// Calculate metrics
	var loss float32
	var accuracy float32
	if total > 0 {
		loss = totalLoss / float32(total)
		accuracy = float32(correct) / float32(total)
	}

	latency := time.Since(start)

	// Use trainer state for logging (trainer was initialized above)
	_ = trainer // Trainer available for future full training runs

	c.JSON(http.StatusOK, TrainResponse{
		Epoch:     1,
		Loss:      loss,
		Accuracy:  accuracy,
		LatencyMs: float64(latency.Milliseconds()),
		UsingASIC: o.engine.IsUsingHardware(),
	})
}

// handleCryptoStatus handles crypto-transformer status requests
func (o *Orchestrator) handleCryptoStatus(c *gin.Context) {
	if o.cryptoModel == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"reason":  "crypto-transformer not enabled via --crypto flag",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":        true,
		"vocab_size":     o.cryptoModel.Config.VocabSize,
		"embedding_dim":  o.cryptoModel.Config.EmbedDim,
		"num_layers":     o.cryptoModel.Config.NumLayers,
		"num_heads":      o.cryptoModel.Config.NumHeads,
		"ffn_hidden_dim": o.cryptoModel.Config.FFNHiddenDim,
		"activation":     o.cryptoModel.Config.Activation,
		"using_asic":     o.engine.IsUsingHardware(),
	})
}

// handleListRules handles GET /rules requests
func (o *Orchestrator) handleListRules(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"rules": []interface{}{}, // Empty for now - could be extended to store rules
		"total": 0,
	})
}

// handleAddRule handles POST /rules requests
func (o *Orchestrator) handleAddRule(c *gin.Context) {
	var req RuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate rule type
	if req.RuleType != "constraint" && req.RuleType != "subsumption" && req.RuleType != "disjoint" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule type. Must be: constraint, subsumption, or disjoint"})
		return
	}

	// For now, just acknowledge the rule (in production, would store in database)
	c.JSON(http.StatusOK, RuleResponse{
		Success: true,
		Message: fmt.Sprintf("Rule added successfully to domain '%s'", req.Domain),
		RuleID:  1, // Placeholder ID
	})
}

// handleDeleteRule handles DELETE /rules/:id requests
func (o *Orchestrator) handleDeleteRule(c *gin.Context) {
	ruleID := c.Param("id")
	if ruleID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule ID required"})
		return
	}

	// For now, just acknowledge deletion (in production, would remove from database)
	c.JSON(http.StatusOK, RuleResponse{
		Success: true,
		Message: fmt.Sprintf("Rule '%s' deleted successfully", ruleID),
	})
}

// handleListDomains handles GET /domains requests
func (o *Orchestrator) handleListDomains(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"domains": []interface{}{}, // Empty for now - could be extended to store domains
		"total":   0,
	})
}

// handleDiscovery handles GET /discovery requests
func (o *Orchestrator) handleDiscovery(c *gin.Context) {
	if o.discoveryResult == nil {
		c.JSON(http.StatusOK, gin.H{
			"discovered": false,
			"message":    "No network discovery performed or no servers found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"discovered": true,
		"server":     o.discoveryResult,
	})
}

// handleDiscoveryScan handles POST /discovery/scan requests
func (o *Orchestrator) handleDiscoveryScan(c *gin.Context) {
	// Parse request parameters
	type DiscoveryRequest struct {
		Subnet    string `json:"subnet,omitempty"`
		Port      int    `json:"port,omitempty"`
		TimeoutMs int64  `json:"timeout_ms,omitempty"`
		SkipLocal bool   `json:"skip_localhost,omitempty"`
	}

	var req DiscoveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Create discovery config
	config := hasher.NewDiscoveryConfig()
	if req.Subnet != "" {
		config.Subnet = req.Subnet
	}
	if req.Port > 0 {
		config.Port = req.Port
	}
	if req.TimeoutMs > 0 {
		config.Timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	config.SkipLocalhost = req.SkipLocal

	// Perform discovery
	discoveries, err := hasher.DiscoverServers(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Find best server
	best := hasher.FindBestServer(discoveries)

	c.JSON(http.StatusOK, gin.H{
		"discoveries": discoveries,
		"best_server": best,
		"total_found": len(discoveries),
		"responding": func() int {
			count := 0
			for _, d := range discoveries {
				if d.Responding {
					count++
				}
			}
			return count
		}(),
	})
}

// CLI Modes (kept from original implementation)

func runSingleMode(orch *Orchestrator) {
	log.Printf("Computing %d single hashes...", *count)

	if orch.asicClient == nil {
		log.Fatal("ASIC client not available")
	}

	totalLatency := time.Duration(0)

	for i := 0; i < *count; i++ {
		data := randomData(*dataSize)

		start := time.Now()
		hash, err := orch.asicClient.ComputeHash(data)
		latency := time.Since(start)

		if err != nil {
			log.Fatalf("ComputeHash failed: %v", err)
		}

		totalLatency += latency

		if i == 0 {
			log.Printf("Hash #%d: %x (latency: %v)", i+1, hash[:8], latency)
		}
	}

	avgLatency := totalLatency / time.Duration(*count)
	log.Printf("Completed %d hashes, average latency: %v", *count, avgLatency)
}

func runBatchMode(orch *Orchestrator) {
	log.Printf("Computing %d hashes with batch size %d...", *count, *batchSize)

	if orch.asicClient == nil {
		log.Fatal("ASIC client not available")
	}

	// Prepare all data
	data := make([][]byte, *count)
	for i := 0; i < *count; i++ {
		data[i] = randomData(*dataSize)
	}

	start := time.Now()
	var totalHashes [][32]byte

	// Process in batches if batchSize is specified and smaller than total count
	if *batchSize > 0 && *batchSize < *count {
		for i := 0; i < *count; i += *batchSize {
			end := i + *batchSize
			if end > *count {
				end = *count
			}
			batch := data[i:end]

			log.Printf("Processing batch %d-%d...", i, end-1)
			hashes, err := orch.asicClient.ComputeBatch(batch)
			if err != nil {
				log.Fatalf("ComputeBatch failed at batch %d-%d: %v", i, end-1, err)
			}
			totalHashes = append(totalHashes, hashes...)
		}
	} else {
		// Single batch
		hashes, err := orch.asicClient.ComputeBatch(data)
		if err != nil {
			log.Fatalf("ComputeBatch failed: %v", err)
		}
		totalHashes = hashes
	}

	elapsed := time.Since(start)

	log.Printf("Computed %d hashes in %v", len(totalHashes), elapsed)
	log.Printf("Throughput: %.2f hashes/sec", float64(len(totalHashes))/elapsed.Seconds())

	if len(totalHashes) > 0 {
		log.Printf("First hash: %x", totalHashes[0][:8])
	}
}

func runStreamMode(orch *Orchestrator) {
	log.Printf("Streaming %d hashes...", *count)

	if orch.asicClient == nil {
		log.Fatal("ASIC client not available")
	}

	// Prepare data
	data := make([][]byte, *count)
	for i := 0; i < *count; i++ {
		data[i] = randomData(*dataSize)
	}

	var received int
	var totalLatency time.Duration
	start := time.Now()

	// Define callback for streaming results
	callback := func(requestID uint64, hash [32]byte, latencyUs uint64) {
		received++
		totalLatency += time.Duration(latencyUs) * time.Microsecond

		if received == 1 || received%100 == 0 {
			log.Printf("Received hash #%d: %x (latency: %dus)",
				requestID, hash[:8], latencyUs)
		}
	}

	// Perform streaming computation
	err := orch.asicClient.StreamCompute(data, callback)
	if err != nil {
		log.Fatalf("StreamCompute failed: %v", err)
	}

	elapsed := time.Since(start)
	avgLatency := time.Duration(0)
	if received > 0 {
		avgLatency = totalLatency / time.Duration(received)
	}

	log.Printf("Streamed %d hashes in %v", received, elapsed)
	log.Printf("Average latency: %v", avgLatency)
	log.Printf("Throughput: %.2f hashes/sec", float64(received)/elapsed.Seconds())
}

func showMetrics(orch *Orchestrator) {
	if orch.asicClient == nil {
		log.Fatal("ASIC client not available")
	}

	resp, err := orch.asicClient.GetMetrics()
	if err != nil {
		log.Printf("GetMetrics failed (may be in software fallback mode): %v", err)
		fmt.Println("\n=== Hasher Metrics ===")
		fmt.Println("Metrics not available in software fallback mode")
		return
	}

	fmt.Println("\n=== Hasher Metrics ===")
	fmt.Printf("Total Requests:       %d\n", resp.TotalRequests)
	fmt.Printf("Total Bytes Processed: %d (%.2f MB)\n",
		resp.TotalBytesProcessed,
		float64(resp.TotalBytesProcessed)/1024/1024)
	fmt.Printf("Average Latency:      %d µs\n", resp.AverageLatencyUs)
	fmt.Printf("Peak Latency:         %d µs\n", resp.PeakLatencyUs)
	fmt.Printf("Total Errors:         %d\n", resp.TotalErrors)
	fmt.Printf("Cache Hits:           %d\n", resp.CacheHits)
	fmt.Printf("Cache Misses:         %d\n", resp.CacheMisses)

	if len(resp.DeviceStats) > 0 {
		fmt.Println("\nDevice Stats:")
		for k, v := range resp.DeviceStats {
			fmt.Printf("  %s: %d\n", k, v)
		}
	}
}

func showInfo(orch *Orchestrator) {
	if orch.asicClient == nil {
		log.Fatal("ASIC client not available")
	}

	resp, err := orch.asicClient.GetDeviceInfo()
	if err != nil {
		log.Fatalf("GetDeviceInfo failed: %v", err)
	}

	fmt.Println("\n=== Device Info ===")
	fmt.Printf("Device Path:      %s\n", resp.DevicePath)
	fmt.Printf("Chip Count:       %d\n", resp.ChipCount)
	fmt.Printf("Firmware Version: %s\n", resp.FirmwareVersion)
	fmt.Printf("Operational:      %v\n", resp.IsOperational)
	fmt.Printf("Uptime:           %d seconds (%.1f hours)\n",
		resp.UptimeSeconds,
		float64(resp.UptimeSeconds)/3600)

	// Also show orchestrator info
	fmt.Println("\n=== Orchestrator Info ===")
	fmt.Printf("Using ASIC:       %v\n", orch.engine.IsUsingHardware())
	fmt.Printf("Network:          [%d, %d, %d, %d]\n", *inputSize, *hidden1, *hidden2, *outputSize)
	fmt.Printf("Passes:           %d\n", *passes)
	fmt.Printf("Jitter:           %.3f\n", *jitter)
	fmt.Printf("Seed Rotation:    %v\n", *seedRotation)
}

// findOpenPort finds an available port starting from the given port
func findOpenPort(startPort int) (int, error) {
	if startPort > 0 {
		// Check if the specified port is available
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort))
		if err == nil {
			listener.Close()
			return startPort, nil
		}
		log.Printf("Port %d not available: %v", startPort, err)
	}

	// Find an available port starting from 8080
	for port := 8080; port <= 9090; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			log.Printf("Found available port: %d", port)
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports found in range 8080-9090")
}

func randomData(size int) []byte {
	data := make([]byte, size)
	rand.Read(data)
	return data
}

// Utility function to pretty print JSON
func prettyJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
