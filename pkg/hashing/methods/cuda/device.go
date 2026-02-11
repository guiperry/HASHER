package cuda

import (
	"fmt"
	"sync"

	"hasher/pkg/hashing/core"
	"hasher/pkg/hashing/jitter"
)

// CudaMethod implements the HashMethod interface for CUDA-accelerated hashing
// This method is optimized for training pipeline only
type CudaMethod struct {
	bridge      *CudaBridge
	mutex       sync.RWMutex
	canon       *core.CanonicalSHA256
	caps        *core.Capabilities
	jitterTable map[uint32]uint32
}

// NewCudaMethod creates a new CUDA hashing method
func NewCudaMethod() *CudaMethod {
	bridge := NewCudaBridge()

	return &CudaMethod{
		bridge:      bridge,
		canon:       core.NewCanonicalSHA256(),
		jitterTable: make(map[uint32]uint32),
	}
}

// Name returns the human-readable name of the hashing method
func (m *CudaMethod) Name() string {
	return "CUDA Simulator (Training Only)"
}

// IsAvailable returns true if this hashing method is available on the current system
func (m *CudaMethod) IsAvailable() bool {
	if m.bridge == nil {
		return false
	}

	// Check if CUDA is properly initialized
	deviceCount := m.bridge.GetDeviceCount()
	return deviceCount > 0
}

// Initialize performs any necessary setup for the hashing method
func (m *CudaMethod) Initialize() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.bridge == nil {
		return fmt.Errorf("CUDA bridge not initialized")
	}

	// CUDA bridge should already be initialized in NewCudaBridge
	return nil
}

// Shutdown performs cleanup and shuts down the hashing method
func (m *CudaMethod) Shutdown() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.bridge != nil {
		return m.bridge.Close()
	}
	return nil
}

// ComputeHash computes a single SHA-256 hash
func (m *CudaMethod) ComputeHash(data []byte) ([32]byte, error) {
	if !m.IsAvailable() {
		return [32]byte{}, fmt.Errorf("CUDA not available")
	}

	// Delegate to canonical implementation for single hash
	return m.canon.ComputeSHA256(data), nil
}

// ComputeBatch computes multiple SHA-256 hashes in parallel/batch
func (m *CudaMethod) ComputeBatch(data [][]byte) ([][32]byte, error) {
	if !m.IsAvailable() {
		return nil, fmt.Errorf("CUDA not available")
	}

	// For now, use canonical implementation
	// TODO: Integrate with CUDA bridge for batch processing
	results := make([][32]byte, len(data))
	for i, d := range data {
		results[i] = m.canon.ComputeSHA256(d)
	}

	return results, nil
}

// MineHeader performs Bitcoin-style mining on an 80-byte header
func (m *CudaMethod) MineHeader(header []byte, nonceStart, nonceEnd uint32) (uint32, error) {
	if !m.IsAvailable() {
		return 0, fmt.Errorf("CUDA not available")
	}

	if len(header) != 80 {
		return 0, fmt.Errorf("header must be exactly 80 bytes")
	}

	// Use CUDA bridge for mining if available
	if m.bridge != nil {
		result, err := m.bridge.ProcessSingleHeader(header, uint32(nonceStart))
		if err == nil {
			return result, nil
		}
	}

	// Fallback to canonical implementation
	return m.canon.MineForNonce(header, nonceStart, nonceEnd)
}

// MineHeaderBatch performs mining on multiple headers
func (m *CudaMethod) MineHeaderBatch(headers [][]byte, nonceStart, nonceEnd uint32) ([]uint32, error) {
	if !m.IsAvailable() {
		return nil, fmt.Errorf("CUDA not available")
	}

	// Use CUDA bridge for batch mining if available
	if m.bridge != nil {
		results, err := m.bridge.ProcessHeadersBatch(headers, uint32(nonceStart))
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}

	// Fallback to canonical implementation
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
func (m *CudaMethod) GetCapabilities() *core.Capabilities {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.caps == nil {
		m.initializeCapabilities()
	}

	return m.caps
}

// initializeCapabilities sets up the capabilities based on CUDA availability
func (m *CudaMethod) initializeCapabilities() {
	isAvailable := m.IsAvailable()
	hashRate := uint64(0)
	avgLatencyUs := uint64(100)
	deviceCount := 0

	if isAvailable && m.bridge != nil {
		// CUDA performance characteristics
		hashRate = 50000000000 // 50 GH/s for CUDA
		avgLatencyUs = 50      // 50 microseconds
		deviceCount = m.bridge.GetDeviceCount()
	}

	m.caps = &core.Capabilities{
		Name:              "CUDA Simulator (Training Only)",
		IsHardware:        isAvailable,
		HashRate:          hashRate,
		ProductionReady:   false, // CUDA is for training only
		TrainingOptimized: true,  // Optimized for training pipeline
		MaxBatchSize:      1000,  // Large batch size for GPU
		AvgLatencyUs:      avgLatencyUs,
		HardwareInfo: &core.HardwareInfo{
			DevicePath:     "cuda",
			ChipCount:      deviceCount,
			Version:        "cuda-runtime",
			ConnectionType: "PCIe",
			Metadata: map[string]string{
				"purpose":      "training_only",
				"available":    fmt.Sprintf("%t", isAvailable),
				"device_count": fmt.Sprintf("%d", deviceCount),
			},
		},
	}
}

// GetBridge returns the underlying CUDA bridge for advanced operations
func (m *CudaMethod) GetBridge() *CudaBridge {
	return m.bridge
}

// CudaHashMethod implements jitter.HashMethod for CUDA
type CudaHashMethod struct {
	bridge *CudaBridge
}

func (c *CudaHashMethod) ComputeHash(data []byte) ([32]byte, error) {
	return core.NewCanonicalSHA256().ComputeSHA256(data), nil
}

func (c *CudaHashMethod) ComputeDoubleHash(data []byte) ([32]byte, error) {
	if len(data) == 80 && c.bridge != nil {
		results, err := c.bridge.ComputeDoubleHashFull([][]byte{data})
		if err == nil && len(results) > 0 {
			return results[0], nil
		}
	}

	h1 := core.NewCanonicalSHA256().ComputeSHA256(data)
	return core.NewCanonicalSHA256().ComputeSHA256(h1[:]), nil
}

// ExecuteRecursiveMine runs the complete 21-pass temporal loop and returns the full 32-byte hash
func (m *CudaMethod) ExecuteRecursiveMine(header []byte, passes int) ([]byte, error) {
	if !m.IsAvailable() {
		return nil, fmt.Errorf("CUDA not available")
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Create jitter engine
	jitterConfig := jitter.DefaultJitterConfig()
	jitterConfig.PassCount = passes
	jitterEngine := jitter.NewJitterEngine(jitterConfig)
	
	// Load associative memory
	jitterEngine.GetSearcher().LoadJitterTable(m.jitterTable)
	
	// Set the hash method to use CUDA bridge
	jitterEngine.SetHashMethod(&CudaHashMethod{bridge: m.bridge})

	// Target doesn't matter for raw mining result
	result, err := jitterEngine.Execute21PassLoop(header, 0)
	if err != nil {
		return nil, err
	}

	return result.FullSeed, nil
}

// Execute21PassLoop runs the 21-pass temporal loop with flash search jitter
func (m *CudaMethod) Execute21PassLoop(header []byte, targetTokenID uint32) (*core.JitterResult, error) {
	if !m.IsAvailable() {
		return nil, fmt.Errorf("CUDA method not available")
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if len(header) != 80 {
		return nil, fmt.Errorf("header must be exactly 80 bytes")
	}

	// Create jitter engine
	jitterConfig := jitter.DefaultJitterConfig()
	jitterEngine := jitter.NewJitterEngine(jitterConfig)
	
	// Load associative memory
	jitterEngine.GetSearcher().LoadJitterTable(m.jitterTable)
	
	jitterEngine.SetHashMethod(&CudaHashMethod{bridge: m.bridge})

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
		LatencyUs:       0,
		Method:          m.Name(),
		Metadata:        result.Metadata,
	}, nil
}

// LoadJitterTable loads associative memory for flash search jitter lookup
func (m *CudaMethod) LoadJitterTable(table map[uint32]uint32) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.jitterTable = make(map[uint32]uint32, len(table))
	for k, v := range table {
		m.jitterTable[k] = v
	}

	return nil
}

// GetJitterStats returns jitter-specific statistics
func (m *CudaMethod) GetJitterStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"method":            m.Name(),
		"jitter_enabled":    true,
		"jitter_table_size": len(m.jitterTable),
		"cuda_available":    m.bridge.GetDeviceCount() > 0,
	}
}
