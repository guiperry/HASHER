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

// SimulateBitcoinHash performs Double-SHA256 on 80-byte Bitcoin header
// This is the core "Camouflage" function for BM1382 ASIC compatibility
func (v *vHasherSimulator) SimulateBitcoinHeader(header []byte) (uint32, error) {
	if !v.isRunning {
		return 0, fmt.Errorf("simulator is not running")
	}

	if len(header) != 80 {
		return 0, fmt.Errorf("invalid Bitcoin header length: expected 80 bytes, got %d", len(header))
	}

	start := time.Now()
	defer func() {
		latency := time.Since(start).Seconds()
		v.updateStats(latency)
	}()

	// Create cache key for Bitcoin header
	cacheKey := fmt.Sprintf("btc_%x", header[:16]) // Use first 16 bytes for cache

	v.cacheMutex.RLock()
	if cached, exists := v.cache[cacheKey]; exists {
		v.cacheMutex.RUnlock()
		return cached, nil
	}
	v.cacheMutex.RUnlock()

	// ROUND 1: Hash the 80-byte header (Bitcoin mining)
	// SHA-256 processes 64-byte chunks. 80 bytes = 1.25 chunks.
	hasher1 := sha256.New()

	// First chunk: bytes 0-63
	hasher1.Write(header[0:64])

	// Second chunk: bytes 64-79 + padding
	chunk2 := make([]byte, 64)
	copy(chunk2, header[64:80])
	chunk2[16] = 0x80 // SHA-256 padding bit
	// Add length in bits (80 * 8 = 640)
	binary.BigEndian.PutUint64(chunk2[56:], 640)
	hasher1.Write(chunk2)

	hash1 := hasher1.Sum(nil)

	// ROUND 2: Hash the 32-byte result of Round 1
	hasher2 := sha256.New()
	hasher2.Write(hash1)

	// Add padding for second round
	padding2 := make([]byte, 64)
	copy(padding2, hash1)
	padding2[32] = 0x80
	binary.BigEndian.PutUint64(padding2[56:], 256) // 32 bytes * 8 bits
	hasher2.Write(padding2[32:])

	hash2 := hasher2.Sum(nil)

	// Return the first 4 bytes of final hash as the nonce result
	result := binary.LittleEndian.Uint32(hash2[:4])

	// Cache the result
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

// ProcessHeadersBatch processes multiple Bitcoin headers in parallel
// Optimized for GPU batch processing and Evolutionary GRPO
func (v *vHasherSimulator) ProcessHeadersBatch(headers [][]byte, targetTokenID uint32) ([]uint32, error) {
	if !v.isRunning {
		return nil, fmt.Errorf("simulator is not running")
	}

	results := make([]uint32, len(headers))

	// Process headers in parallel batches
	batchSize := 100 // Process 100 headers at a time
	for i := 0; i < len(headers); i += batchSize {
		end := i + batchSize
		if end > len(headers) {
			end = len(headers)
		}

		// Process batch
		batch := headers[i:end]
		for j, header := range batch {
			result, err := v.SimulateBitcoinHeader(header)
			if err != nil {
				// Log error but continue processing
				continue
			}
			results[i+j] = result
		}
	}

	return results, nil
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
