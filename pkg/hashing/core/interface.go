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
