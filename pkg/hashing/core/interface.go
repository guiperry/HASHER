package core

// HashMethod defines the interface that all hashing implementations must follow
type HashMethod interface {
	// Name returns the human-readable name of the hashing method
	Name() string

	// IsAvailable returns true if this hashing method is available on the current system
	IsAvailable() bool

	// Initialize performs any necessary setup for the hashing method
	Initialize() error

	// Shutdown performs cleanup and shuts down the hashing method
	Shutdown() error

	// ComputeHash computes a single SHA-256 hash
	ComputeHash(data []byte) ([32]byte, error)

	// ComputeBatch computes multiple SHA-256 hashes in parallel/batch
	ComputeBatch(data [][]byte) ([][32]byte, error)

	// MineHeader performs Bitcoin-style mining on an 80-byte header
	MineHeader(header []byte, nonceStart, nonceEnd uint32) (uint32, error)

	// MineHeaderBatch performs mining on multiple headers
	MineHeaderBatch(headers [][]byte, nonceStart, nonceEnd uint32) ([]uint32, error)

	// GetCapabilities returns the capabilities and performance characteristics
	GetCapabilities() *Capabilities

	// Execute21PassLoop runs the 21-pass temporal loop with flash search jitter
	// This is the core mechanism for dynamic associative hashing
	Execute21PassLoop(header []byte, targetTokenID uint32) (*JitterResult, error)

	// Execute21PassLoopBatch runs the temporal loop for multiple headers in batch
	Execute21PassLoopBatch(headers [][]byte, targetTokenID uint32) ([]*JitterResult, error)

	// ExecuteRecursiveMine runs the complete 21-pass temporal loop and returns the full 32-byte hash
	// This represents the final 'Golden Seed' discovered by the pathfinder
	ExecuteRecursiveMine(header []byte, passes int) ([]byte, error)

	// LoadJitterTable loads associative memory for flash search jitter lookup
	LoadJitterTable(table map[uint32]uint32) error

	// GetJitterStats returns jitter-specific statistics
	GetJitterStats() map[string]interface{}
}

// Capabilities describes the capabilities of a hashing method
type Capabilities struct {
	// Name of the hashing method
	Name string `json:"name"`

	// Whether this method uses actual ASIC hardware
	IsHardware bool `json:"is_hardware"`

	// Expected hash rate (hashes per second)
	HashRate uint64 `json:"hash_rate"`

	// Whether this method is recommended for production use
	ProductionReady bool `json:"production_ready"`

	// Whether this method is optimized for training
	TrainingOptimized bool `json:"training_optimized"`

	// Whether this method supports 21-pass temporal jitter
	JitterSupported bool `json:"jitter_supported"`

	// Whether this method supports advanced recursive pathfinding (21-pass loop)
	RecursiveJitterSupported bool `json:"recursive_jitter_supported"`

	// Maximum batch size for batch operations
	MaxBatchSize int `json:"max_batch_size"`

	// Latency characteristics
	AvgLatencyUs uint64 `json:"avg_latency_us"`

	// Hardware-specific details
	HardwareInfo *HardwareInfo `json:"hardware_info,omitempty"`

	// Reason for unavailability (if applicable)
	Reason string `json:"reason,omitempty"`
}

// HardwareInfo contains hardware-specific information
type HardwareInfo struct {
	// Device path (e.g., "/dev/bitmain-asic")
	DevicePath string `json:"device_path"`

	// Number of chips/cores
	ChipCount int `json:"chip_count"`

	// Firmware/hardware version
	Version string `json:"version"`

	// Connection type (USB, SPI, etc.)
	ConnectionType string `json:"connection_type"`

	// Additional hardware metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// HashResult represents the result of a hash operation with metadata
type HashResult struct {
	// The computed hash
	Hash [32]byte `json:"hash"`

	// Time taken to compute the hash (microseconds)
	LatencyUs uint64 `json:"latency_us"`

	// Which method was used
	Method string `json:"method"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// MiningResult represents the result of a mining operation
type MiningResult struct {
	// The discovered nonce
	Nonce uint32 `json:"nonce"`

	// Whether a valid nonce was found
	Found bool `json:"found"`

	// Time taken to find the nonce (microseconds)
	LatencyUs uint64 `json:"latency_us"`

	// Number of hashes attempted
	HashesAttempted uint64 `json:"hashes_attempted"`

	// Which method was used
	Method string `json:"method"`
}

// JitterResult represents the result of a 21-pass temporal loop operation
type JitterResult struct {
	// The discovered golden nonce
	Nonce uint32 `json:"nonce"`

	// Whether a valid golden nonce was found
	Found bool `json:"found"`

	// Final hash after 21 passes
	FinalHash [32]byte `json:"final_hash"`

	// Number of passes completed
	PassesCompleted int `json:"passes_completed"`

	// Stability score (consistency across passes)
	Stability float64 `json:"stability"`

	// Alignment score (how well final hash matches target)
	Alignment float64 `json:"alignment"`

	// All jitter vectors applied during the search
	JitterVectors []uint32 `json:"jitter_vectors"`

	// Time taken to complete the temporal loop (microseconds)
	LatencyUs uint64 `json:"latency_us"`

	// Which method was used
	Method string `json:"method"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
