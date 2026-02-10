package asic

import (
	"fmt"
	"sync"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/jitter"
)

// ASICMethod implements the HashMethod interface for direct ASIC hardware hashing
type ASICMethod struct {
	client *ASICClient
	mutex  sync.RWMutex
	caps   *core.Capabilities
}

// NewASICMethod creates a new ASIC hashing method
func NewASICMethod(address string) *ASICMethod {
	client, err := NewASICClient(address)
	if err != nil {
		// Log error but still create method - client will use fallback
		fmt.Printf("Warning: Failed to connect to ASIC, using fallback: %v\n", err)
	}

	method := &ASICMethod{
		client: client,
	}

	// Initialize capabilities
	method.initializeCapabilities()

	return method
}

// Name returns the human-readable name of the hashing method
func (m *ASICMethod) Name() string {
	return "ASIC Hardware"
}

// IsAvailable returns true if this hashing method is available on the current system
func (m *ASICMethod) IsAvailable() bool {
	if m.client == nil {
		return false
	}

	// Check if we have a real ASIC connection (not fallback)
	return m.client.IsConnected()
}

// Initialize performs any necessary setup for the hashing method
func (m *ASICMethod) Initialize() error {
	if m.client == nil {
		return fmt.Errorf("ASIC client not initialized")
	}

	// Try to reconnect if not connected
	if !m.client.IsConnected() {
		if err := m.client.Reconnect(); err != nil {
			return fmt.Errorf("failed to initialize ASIC: %w", err)
		}
	}

	return nil
}

// Shutdown performs cleanup and shuts down the hashing method
func (m *ASICMethod) Shutdown() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// ComputeHash computes a single SHA-256 hash
func (m *ASICMethod) ComputeHash(data []byte) ([32]byte, error) {
	if m.client == nil {
		return [32]byte{}, fmt.Errorf("ASIC client not initialized")
	}

	return m.client.ComputeHash(data)
}

// ComputeBatch computes multiple SHA-256 hashes in parallel/batch
func (m *ASICMethod) ComputeBatch(data [][]byte) ([][32]byte, error) {
	if m.client == nil {
		return nil, fmt.Errorf("ASIC client not initialized")
	}

	return m.client.ComputeBatch(data)
}

// MineHeader performs Bitcoin-style mining on an 80-byte header
func (m *ASICMethod) MineHeader(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	if m.client == nil {
		return 0, fmt.Errorf("ASIC client not initialized")
	}

	return m.client.MineHeader(header, nonceStart, nonceEnd)
}

// MineHeaderBatch performs mining on multiple headers
func (m *ASICMethod) MineHeaderBatch(headers [][]byte, nonceStart, nonceEnd uint32) ([]uint32, error) {
	if m.client == nil {
		return nil, fmt.Errorf("ASIC client not initialized")
	}

	return m.client.MineHeaderBatch(headers, nonceStart, nonceEnd)
}

// GetCapabilities returns the capabilities and performance characteristics
func (m *ASICMethod) GetCapabilities() *core.Capabilities {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.caps == nil {
		m.initializeCapabilities()
	}

	return m.caps
}

// initializeCapabilities sets up the capabilities based on current client state
func (m *ASICMethod) initializeCapabilities() {
	if m.client == nil {
		m.caps = &core.Capabilities{
			Name:            "ASIC Hardware (Uninitialized)",
			IsHardware:      false,
			HashRate:        0,
			ProductionReady: false,
			MaxBatchSize:    0,
		}
		return
	}

	// Get device info
	deviceInfo, err := m.client.GetDeviceInfo()
	if err != nil {
		// Client exists but can't get device info
		m.caps = &core.Capabilities{
			Name:            "ASIC Hardware (Error)",
			IsHardware:      false,
			HashRate:        0,
			ProductionReady: false,
			MaxBatchSize:    0,
		}
		return
	}

	// Determine if using real ASIC or fallback
	isHardware := m.client.IsConnected()
	hashRate := uint64(500000000000) // 500 GH/s for real ASIC
	productionReady := isHardware && deviceInfo.IsOperational
	maxBatchSize := 256
	avgLatencyUs := uint64(100) // 100 microseconds typical latency

	if !isHardware {
		// Software fallback characteristics
		hashRate = 1000000 // 1 MH/s for software
		productionReady = false
		maxBatchSize = 100
		avgLatencyUs = 1000 // 1 millisecond for software
	}

	m.caps = &core.Capabilities{
		Name:              "ASIC Hardware",
		IsHardware:        isHardware,
		HashRate:          hashRate,
		ProductionReady:   productionReady,
		TrainingOptimized: false, // ASIC is for production, not training
		JitterSupported:   false, // ASIC does not support jitter
		MaxBatchSize:      maxBatchSize,
		AvgLatencyUs:      avgLatencyUs,
		HardwareInfo: &core.HardwareInfo{
			DevicePath:     deviceInfo.DevicePath,
			ChipCount:      int(deviceInfo.ChipCount),
			Version:        deviceInfo.FirmwareVersion,
			ConnectionType: "USB", // Current Antminer S3 uses USB
			Metadata: map[string]string{
				"fallback_mode":  fmt.Sprintf("%t", !isHardware),
				"operational":    fmt.Sprintf("%t", deviceInfo.IsOperational),
				"uptime_seconds": fmt.Sprintf("%d", deviceInfo.UptimeSeconds),
			},
		},
	}
}

// IsUsingFallback returns true if the ASIC method is currently using software fallback
func (m *ASICMethod) IsUsingFallback() bool {
	if m.client == nil {
		return true
	}
	return m.client.IsUsingFallback()
}

// Reconnect attempts to reconnect to the ASIC server
func (m *ASICMethod) Reconnect() error {
	if m.client == nil {
		return fmt.Errorf("ASIC client not initialized")
	}

	err := m.client.Reconnect()
	if err == nil {
		// Update capabilities after successful reconnection
		m.initializeCapabilities()
	}

	return err
}

// GetClient returns the underlying ASIC client for advanced operations
func (m *ASICMethod) GetClient() *ASICClient {
	return m.client
}

// Execute21PassLoop runs the 21-pass temporal loop with flash search jitter
func (m *ASICMethod) Execute21PassLoop(header []byte, targetTokenID uint32) (*core.JitterResult, error) {
	if m.client == nil {
		return nil, fmt.Errorf("ASIC client not initialized")
	}

	if len(header) != 80 {
		return nil, fmt.Errorf("header must be exactly 80 bytes")
	}

	// Create jitter engine for this method
	jitterConfig := jitter.DefaultJitterConfig()
	jitterEngine := jitter.NewJitterEngine(jitterConfig)

	// Set the hash method to use our ASIC implementation
	jitterEngine.SetHashMethod(&ASICHASHMethod{client: m.client})

	// Execute the 21-pass loop
	result, err := jitterEngine.Execute21PassLoop(header, targetTokenID)
	if err != nil {
		return nil, fmt.Errorf("21-pass loop failed: %w", err)
	}

	// Convert jitter result to core result
	jitterVectors := make([]uint32, len(result.JitterVectors))
	for i, jv := range result.JitterVectors {
		jitterVectors[i] = uint32(jv)
	}

	return &core.JitterResult{
		Nonce:           result.Nonce,
		Found:           result.Found,
		FinalHash:       result.FinalHash,
		PassesCompleted: result.PassesCompleted,
		Stability:       result.Stability,
		Alignment:       result.Alignment,
		JitterVectors:   jitterVectors,
		LatencyUs:       0, // TODO: Add timing
		Method:          m.Name(),
		Metadata:        result.Metadata,
	}, nil
}

// LoadJitterTable loads associative memory for flash search jitter lookup
func (m *ASICMethod) LoadJitterTable(table map[uint32]uint32) error {
	// Convert uint32 to JitterVector
	jitterTable := make(map[uint32]jitter.JitterVector, len(table))
	for k, v := range table {
		jitterTable[k] = jitter.JitterVector(v)
	}

	// Store for use in Execute21PassLoop
	// TODO: Store this in the method instance
	return nil
}

// GetJitterStats returns jitter-specific statistics
func (m *ASICMethod) GetJitterStats() map[string]interface{} {
	stats := map[string]interface{}{
		"method":            m.Name(),
		"jitter_enabled":    true,
		"jitter_table_size": 0, // TODO: Track actual table size
	}

	// Add ASIC-specific stats
	if m.client != nil {
		stats["asic_connected"] = m.client.IsConnected()
		stats["using_fallback"] = m.client.useFallback
	}

	return stats
}

// ASICHASHMethod implements the jitter.HashMethod interface for ASIC
type ASICHASHMethod struct {
	client *ASICClient
}

// ComputeHash computes SHA-256 using ASIC hardware
func (a *ASICHASHMethod) ComputeHash(data []byte) ([32]byte, error) {
	if a.client == nil {
		return [32]byte{}, fmt.Errorf("ASIC client not available")
	}
	return a.client.ComputeHash(data)
}

// ComputeDoubleHash computes double SHA-256 using ASIC hardware
func (a *ASICHASHMethod) ComputeDoubleHash(data []byte) ([32]byte, error) {
	if a.client == nil {
		return [32]byte{}, fmt.Errorf("ASIC client not available")
	}
	return a.client.ComputeDoubleHash(data)
}
