package software

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/jitter"
)

// SoftwareMethod implements the HashMethod interface for pure software hashing
type SoftwareMethod struct {
	initialized bool
	mutex       sync.RWMutex
	canon       *core.CanonicalSHA256
	caps        *core.Capabilities
	jitterTable map[uint32]uint32
}

// NewSoftwareMethod creates a new software hashing method
func NewSoftwareMethod() *SoftwareMethod {
	return &SoftwareMethod{
		canon:       core.NewCanonicalSHA256(),
		jitterTable: make(map[uint32]uint32),
	}
}

// SoftwareHashMethod implements the jitter.HashMethod interface
type SoftwareHashMethod struct{}

// ComputeHash computes SHA-256 using crypto/sha256
func (s *SoftwareHashMethod) ComputeHash(data []byte) ([32]byte, error) {
	return sha256.Sum256(data), nil
}

// ComputeDoubleHash computes double SHA-256
func (s *SoftwareHashMethod) ComputeDoubleHash(data []byte) ([32]byte, error) {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second, nil
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
			HashRate:          1000000, // 1 MH/s
			ProductionReady:   true,
			TrainingOptimized: false,
			JitterSupported:   false, // No jitter support in software method
			MaxBatchSize:      100,
			AvgLatencyUs:      1000, // 1 millisecond
		}
	}

	return m.caps
}

// Execute21PassLoop runs the 21-pass temporal loop with flash search jitter
func (m *SoftwareMethod) Execute21PassLoop(header []byte, targetTokenID uint32) (*core.JitterResult, error) {
	if !m.initialized {
		return nil, fmt.Errorf("software method not initialized")
	}

	if len(header) != 80 {
		return nil, fmt.Errorf("header must be exactly 80 bytes")
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Create jitter engine for this method
	jitterConfig := jitter.DefaultJitterConfig()
	jitterEngine := jitter.NewJitterEngine(jitterConfig)
	
	// Load associative memory
	jitterEngine.GetSearcher().LoadJitterTable(m.jitterTable)

	// Set the hash method to use our software implementation
	jitterEngine.SetHashMethod(&SoftwareHashMethod{})

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

// ExecuteRecursiveMine runs the complete 21-pass temporal loop and returns the full 32-byte hash
func (m *SoftwareMethod) ExecuteRecursiveMine(header []byte, passes int) ([]byte, error) {
	if !m.initialized {
		return nil, fmt.Errorf("software method not initialized")
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Create jitter engine
	jitterConfig := jitter.DefaultJitterConfig()
	jitterConfig.PassCount = passes
	jitterEngine := jitter.NewJitterEngine(jitterConfig)
	
	// Load associative memory
	jitterEngine.GetSearcher().LoadJitterTable(m.jitterTable)
	
	jitterEngine.SetHashMethod(&SoftwareHashMethod{})

	// Target doesn't matter for raw mining result, but we need one for the loop
	result, err := jitterEngine.Execute21PassLoop(header, 0)
	if err != nil {
		return nil, err
	}

	return result.FullSeed, nil
}

// LoadJitterTable loads associative memory for flash search jitter lookup
func (m *SoftwareMethod) LoadJitterTable(table map[uint32]uint32) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.jitterTable = make(map[uint32]uint32, len(table))
	for k, v := range table {
		m.jitterTable[k] = v
	}

	return nil
}

// GetJitterStats returns jitter-specific statistics
func (m *SoftwareMethod) GetJitterStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"method":            m.Name(),
		"jitter_enabled":    true,
		"jitter_table_size": len(m.jitterTable),
	}
}
