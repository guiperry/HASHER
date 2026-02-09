package ubpf

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/hardware"
)

// UbpfMethod implements the HashMethod interface for uBPF-based hashing
// This method supports both USB device access and CGMiner API interaction
type UbpfMethod struct {
	vm          *uBPFVM
	hwPrep      *hardware.HardwarePrep
	mutex       sync.RWMutex
	canon       *core.CanonicalSHA256
	caps        *core.Capabilities
	initialized bool
	// USB device access
	devicePath string
	// CGMiner API access
	cgminerPath string
	cgminerAPI  string
}

// NewUbpfMethod creates a new uBPF hashing method
func NewUbpfMethod(devicePath, cgminerPath string) *UbpfMethod {
	return &UbpfMethod{
		vm:          NewuBPFVM(),
		hwPrep:      hardware.NewHardwarePrep(true),
		canon:       core.NewCanonicalSHA256(),
		devicePath:  devicePath,
		cgminerPath: cgminerPath,
		cgminerAPI:  "127.0.0.1:4028", // Default CGMiner API
		initialized: false,
	}
}

// Name returns the human-readable name of the hashing method
func (m *UbpfMethod) Name() string {
	return "uBPF Simulator (USB + CGMiner)"
}

// IsAvailable returns true if this hashing method is available on the current system
func (m *UbpfMethod) IsAvailable() bool {
	// Check if uBPF VM can be loaded
	if m.vm == nil {
		return false
	}

	// Check if USB device is accessible
	if m.devicePath != "" {
		if _, err := os.Stat(m.devicePath); err != nil {
			// USB device not accessible, try CGMiner API
		} else {
			return true // USB device available
		}
	}

	// Check if CGMiner is available
	if m.cgminerPath != "" {
		if _, err := os.Stat(m.cgminerPath); err != nil {
			return false
		}
		// Try to connect to CGMiner API
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s/summary", m.cgminerAPI))
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == 200
	}

	return false // Neither USB nor CGMiner available
}

// Initialize performs any necessary setup for the hashing method
func (m *UbpfMethod) Initialize() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Try USB device access first
	if m.devicePath != "" {
		if err := m.initializeUSB(); err == nil {
			m.initialized = true
			return nil
		}
	}

	// Fallback to CGMiner API
	if m.cgminerPath != "" {
		if err := m.initializeCGMiner(); err == nil {
			m.initialized = true
			return nil
		}
	}

	return fmt.Errorf("failed to initialize uBPF method: no USB or CGMiner access")
}

// initializeUSB attempts to initialize USB device access
func (m *UbpfMethod) initializeUSB() error {
	// Check if device is accessible
	if _, err := os.Stat(m.devicePath); err != nil {
		return fmt.Errorf("USB device not accessible: %w", err)
	}

	// Try to stop CGMiner if it's running
	if m.cgminerPath != "" {
		cmd := exec.Command("pkill", "cgminer")
		cmd.Run()
		time.Sleep(1 * time.Second)
	}

	// Try to open device directly
	// TODO: Implement direct USB device access
	return fmt.Errorf("direct USB access not yet implemented")
}

// initializeCGMiner attempts to initialize CGMiner API access
func (m *UbpfMethod) initializeCGMiner() error {
	// Start CGMiner if not running
	if m.cgminerPath != "" {
		cmd := exec.Command(m.cgminerPath, "--api-allow", "W:127.0.0.1:4028")
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start CGMiner: %w", err)
		}
		time.Sleep(3 * time.Second) // Wait for CGMiner to start
	}

	// Test API connection
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/summary", m.cgminerAPI))
	if err != nil {
		return fmt.Errorf("CGMiner API not accessible: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("CGMiner API returned status: %d", resp.StatusCode)
	}

	return nil
}

// Shutdown performs cleanup and shuts down the hashing method
func (m *UbpfMethod) Shutdown() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.initialized = false

	if m.vm != nil {
		if err := m.vm.Close(); err != nil {
			return fmt.Errorf("failed to close uBPF VM: %w", err)
		}
	}

	// Stop CGMiner if we started it
	if m.cgminerPath != "" {
		cmd := exec.Command("pkill", "cgminer")
		cmd.Run()
	}

	return nil
}

// ComputeHash computes a single SHA-256 hash
func (m *UbpfMethod) ComputeHash(data []byte) ([32]byte, error) {
	if !m.initialized {
		return [32]byte{}, fmt.Errorf("uBPF method not initialized")
	}

	// Use canonical implementation for single hash
	return m.canon.ComputeSHA256(data), nil
}

// ComputeBatch computes multiple SHA-256 hashes in parallel/batch
func (m *UbpfMethod) ComputeBatch(data [][]byte) ([][32]byte, error) {
	if !m.initialized {
		return nil, fmt.Errorf("uBPF method not initialized")
	}

	// Use canonical implementation for batch
	results := make([][32]byte, len(data))
	for i, d := range data {
		results[i] = m.canon.ComputeSHA256(d)
	}

	return results, nil
}

// MineHeader performs Bitcoin-style mining on an 80-byte header
func (m *UbpfMethod) MineHeader(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	if !m.initialized {
		return 0, fmt.Errorf("uBPF method not initialized")
	}

	if len(header) != 80 {
		return 0, fmt.Errorf("header must be exactly 80 bytes")
	}

	// Try CGMiner API first
	if m.cgminerPath != "" {
		if nonce, err := m.mineWithCGMiner(header, nonceStart, nonceEnd); err == nil {
			return nonce, nil
		}
	}

	// Fallback to canonical implementation
	return m.canon.MineForNonce(header, nonceStart, nonceEnd)
}

// mineWithCGMiner attempts mining using CGMiner API
func (m *UbpfMethod) mineWithCGMiner(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	// Convert header to CGMiner format
	// TODO: Implement proper CGMiner API call
	_ = nonceStart
	_ = nonceEnd

	return 0, fmt.Errorf("CGMiner API mining not yet implemented")
}

// MineHeaderBatch performs mining on multiple headers
func (m *UbpfMethod) MineHeaderBatch(headers [][]byte, nonceStart, nonceEnd uint32) ([]uint32, error) {
	if !m.initialized {
		return nil, fmt.Errorf("uBPF method not initialized")
	}

	results := make([]uint32, len(headers))
	for i, header := range headers {
		nonce, err := m.MineHeader(header, nonceStart, nonceEnd)
		if err != nil {
			return nil, fmt.Errorf("mining failed for header %d: %w", i, err)
		}
		results[i] = nonce
	}

	return results, nil
}

// GetCapabilities returns the capabilities and performance characteristics
func (m *UbpfMethod) GetCapabilities() *core.Capabilities {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.caps == nil {
		m.initializeCapabilities()
	}

	return m.caps
}

// initializeCapabilities sets up the capabilities based on availability
func (m *UbpfMethod) initializeCapabilities() {
	isAvailable := m.IsAvailable()
	usbAvailable := false
	cgminerAvailable := false

	// Check USB availability
	if m.devicePath != "" {
		if _, err := os.Stat(m.devicePath); err == nil {
			usbAvailable = true
		}
	}

	// Check CGMiner availability
	if m.cgminerPath != "" {
		if _, err := os.Stat(m.cgminerPath); err == nil {
			cgminerAvailable = true
		}
	}

	var connectionType string
	var metadata map[string]string

	if usbAvailable {
		connectionType = "USB"
		metadata = map[string]string{
			"usb_device":       m.devicePath,
			"cgminer_fallback": fmt.Sprintf("%t", cgminerAvailable),
		}
	} else if cgminerAvailable {
		connectionType = "CGMiner API"
		metadata = map[string]string{
			"cgminer_api":  m.cgminerAPI,
			"cgminer_path": m.cgminerPath,
		}
	} else {
		connectionType = "none"
		metadata = map[string]string{
			"status": "no_access",
		}
	}

	m.caps = &core.Capabilities{
		Name:              "uBPF Simulator (USB + CGMiner)",
		IsHardware:        isAvailable,
		HashRate:          100000000, // 100 MH/s simulated
		ProductionReady:   false,     // Simulation only
		TrainingOptimized: false,
		MaxBatchSize:      50,
		AvgLatencyUs:      500, // 500 microseconds
		HardwareInfo: &core.HardwareInfo{
			DevicePath:     m.devicePath,
			ChipCount:      32, // Simulated BM1382 chips
			Version:        "ubpf-sim",
			ConnectionType: connectionType,
			Metadata:       metadata,
		},
	}
}
