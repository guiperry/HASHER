package software

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"hasher/pkg/hashing/core"
)

// SoftwareMethod implements the HashMethod interface for pure software hashing
type SoftwareMethod struct {
	initialized bool
	mutex       sync.RWMutex
	canon       *core.CanonicalSHA256
	caps        *core.Capabilities
}

// NewSoftwareMethod creates a new software hashing method
func NewSoftwareMethod() *SoftwareMethod {
	return &SoftwareMethod{
		canon: core.NewCanonicalSHA256(),
	}
}

// Name returns the human-readable name of the hashing method
func (m *SoftwareMethod) Name() string {
	return "Software Fallback"
}

// IsAvailable returns true if this hashing method is available on the current system
func (m *SoftwareMethod) IsAvailable() bool {
	return true // Software method is always available
}

// Initialize performs any necessary setup for the hashing method
func (m *SoftwareMethod) Initialize() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.initialized = true
	return nil
}

// Shutdown performs cleanup and shuts down the hashing method
func (m *SoftwareMethod) Shutdown() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.initialized = false
	return nil
}

// ComputeHash computes a single SHA-256 hash
func (m *SoftwareMethod) ComputeHash(data []byte) ([32]byte, error) {
	if !m.initialized {
		return [32]byte{}, fmt.Errorf("software method not initialized")
	}

	return sha256.Sum256(data), nil
}

// ComputeBatch computes multiple SHA-256 hashes in parallel/batch
func (m *SoftwareMethod) ComputeBatch(data [][]byte) ([][32]byte, error) {
	if !m.initialized {
		return nil, fmt.Errorf("software method not initialized")
	}

	results := make([][32]byte, len(data))
	for i, d := range data {
		results[i] = sha256.Sum256(d)
	}

	return results, nil
}

// MineHeader performs Bitcoin-style mining on an 80-byte header
func (m *SoftwareMethod) MineHeader(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	if !m.initialized {
		return 0, fmt.Errorf("software method not initialized")
	}

	return m.canon.MineForNonce(header, nonceStart, nonceEnd)
}

// MineHeaderBatch performs mining on multiple headers
func (m *SoftwareMethod) MineHeaderBatch(headers [][]byte, nonceStart, nonceEnd uint32) ([]uint32, error) {
	if !m.initialized {
		return nil, fmt.Errorf("software method not initialized")
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
func (m *SoftwareMethod) GetCapabilities() *core.Capabilities {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.caps == nil {
		m.caps = &core.Capabilities{
			Name:              "Software Fallback",
			IsHardware:        false,
			HashRate:          1000000, // ~1 MH/s on typical CPU
			ProductionReady:   true,    // Software is reliable but slow
			TrainingOptimized: false,   // Not optimized for training
			MaxBatchSize:      100,     // Conservative batch size
			AvgLatencyUs:      1000,    // ~1ms latency
			HardwareInfo: &core.HardwareInfo{
				DevicePath:     "software",
				ChipCount:      0,
				Version:        "go1.22+",
				ConnectionType: "none",
				Metadata: map[string]string{
					"implementation": "crypto/sha256",
					"portable":       "true",
				},
			},
		}
	}

	return m.caps
}
