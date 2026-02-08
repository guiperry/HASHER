package simulator

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

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

type vHasherSimulator struct {
	config     *SimulatorConfig
	stats      *DeviceStats
	mutex      sync.RWMutex
	cache      map[string]uint32
	cacheMutex sync.RWMutex
	isRunning  bool
}

func NewvHasherSimulator(config *SimulatorConfig) *vHasherSimulator {
	if config == nil {
		config = &SimulatorConfig{
			DeviceType:     "vhasher",
			MaxConcurrency: 100,
			TargetHashRate: 500000000,
			CacheSize:      10000,
			GPUDevice:      0,
			Timeout:        30,
		}
	}

	return &vHasherSimulator{
		config: config,
		stats: &DeviceStats{
			TotalHashes:    0,
			HashRate:       0,
			DeviceTemp:     45.0,
			MemoryUsage:    0,
			ActiveSeeds:    0,
			LastUpdateTime: time.Now().Unix(),
		},
		cache:     make(map[string]uint32),
		isRunning: false,
	}
}

func (v *vHasherSimulator) Initialize(config *SimulatorConfig) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if config != nil {
		v.config = config
	}

	v.isRunning = true
	v.stats.LastUpdateTime = time.Now().Unix()

	return nil
}

func (v *vHasherSimulator) Shutdown() error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.isRunning = false
	v.cache = make(map[string]uint32)

	return nil
}

func (v *vHasherSimulator) SimulateHash(seed []byte, pass int) (uint32, error) {
	if !v.isRunning {
		return 0, fmt.Errorf("simulator is not running")
	}

	start := time.Now()
	defer func() {
		latency := time.Since(start).Seconds()
		v.updateStats(latency)
	}()

	cacheKey := fmt.Sprintf("%x_%d", seed, pass)

	v.cacheMutex.RLock()
	if cached, exists := v.cache[cacheKey]; exists {
		v.cacheMutex.RUnlock()
		return cached, nil
	}
	v.cacheMutex.RUnlock()

	// HASHER deterministic hashing: SHA256(seed) with controlled iteration
	hasher := sha256.New()
	hasher.Write(seed)

	// Mining-like behavior: deterministic based on seed
	hash := hasher.Sum(nil)

	// Perform multiple passes to simulate mining recursion
	for i := 0; i < pass; i++ {
		// Each pass is deterministic based on the previous hash
		hasher.Reset()
		hasher.Write(hash)

		// Add pass counter to create deterministic evolution
		passBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(passBytes, uint32(i+1))
		hasher.Write(passBytes)

		hash = hasher.Sum(nil)
	}

	result := binary.LittleEndian.Uint32(hash[:4])

	if len(v.cache) < v.config.CacheSize {
		v.cacheMutex.Lock()
		v.cache[cacheKey] = result
		v.cacheMutex.Unlock()
	}

	return result, nil
}

func (v *vHasherSimulator) ValidateSeed(seed []byte, targetToken int32) (bool, error) {
	if !v.isRunning {
		return false, fmt.Errorf("simulator is not running")
	}

	// Check if the seed produces the target in any pass
	for pass := 0; pass < 21; pass++ {
		nonce, err := v.SimulateHash(seed, pass)
		if err != nil {
			return false, err
		}

		// Found target in any pass = success for mining
		if nonce == uint32(targetToken) {
			return true, nil
		}
	}

	// Check final pass (pass 20) specifically - this is the "golden nonce"
	finalNonce, err := v.SimulateHash(seed, 20)
	if err != nil {
		return false, err
	}

	// Mining success if final nonce equals target
	isValid := finalNonce == uint32(targetToken)

	return isValid, nil
}

func (v *vHasherSimulator) GetDeviceStats() (*DeviceStats, error) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	return &DeviceStats{
		TotalHashes:    v.stats.TotalHashes,
		HashRate:       v.stats.HashRate,
		DeviceTemp:     v.stats.DeviceTemp,
		MemoryUsage:    uint64(len(v.cache) * 36),
		ActiveSeeds:    v.stats.ActiveSeeds,
		LastUpdateTime: v.stats.LastUpdateTime,
	}, nil
}

func (v *vHasherSimulator) updateStats(latency float64) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.stats.TotalHashes++

	if latency > 0 {
		currentRate := 1.0 / latency
		v.stats.HashRate = v.stats.HashRate*0.9 + currentRate*0.1
	}

	v.stats.DeviceTemp = 45.0 + (v.stats.HashRate/1000000.0)*5.0
	if v.stats.DeviceTemp > 85.0 {
		v.stats.DeviceTemp = 85.0
	}

	v.stats.LastUpdateTime = time.Now().Unix()
}

func (v *vHasherSimulator) ClearCache() {
	v.cacheMutex.Lock()
	defer v.cacheMutex.Unlock()
	v.cache = make(map[string]uint32)
}

func (v *vHasherSimulator) GetCacheSize() int {
	v.cacheMutex.RLock()
	defer v.cacheMutex.RUnlock()
	return len(v.cache)
}
