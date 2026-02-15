package simulator

type SimulatorConfig struct {
	DeviceType     string  `json:"device_type"`
	MaxConcurrency int     `json:"max_concurrency"`
	TargetHashRate float64 `json:"target_hash_rate"`
	CacheSize      int     `json:"cache_size"`
	GPUDevice      int     `json:"gpu_device"`
	Timeout        int     `json:"timeout"`
}

type DeviceStats struct {
	TotalHashes    uint64  `json:"total_hashes"`
	HashRate       float64 `json:"hash_rate"`
	DeviceTemp     float64 `json:"device_temp"`
	MemoryUsage    uint64  `json:"memory_usage"`
	ActiveSeeds    int     `json:"active_seeds"`
	LastUpdateTime int64   `json:"last_update_time"`
}

type HashSimulator interface {
	SimulateHash(seed []byte, pass int) (uint32, error)
	SimulateBitcoinHeader(header []byte) (uint32, error) // BM1382 "Camouflage" support
	RecursiveMine(header []byte, passes int) ([]byte, error) // 21-pass loop with jitter, returns full hash
	ValidateSeed(seed []byte, targetToken int32) (bool, error)
	Shutdown() error
}
