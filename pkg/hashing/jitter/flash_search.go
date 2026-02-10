package jitter

import (
	"encoding/binary"
	"fmt"
	"sync"
)

// FlashSearcher provides high-speed associative memory lookup for jitter vectors
// This implements the "Flash Search" mechanism described in JITTER_IMPLEMENTATION.md
type FlashSearcher struct {
	// In-memory jitter lookup table (simulates SRAM-like access)
	jitterTable map[uint32]JitterVector

	// Cache for recently accessed jitters
	lruCache *LRUCache

	// Statistics for monitoring
	stats *SearchStats

	// Configuration
	config *JitterConfig

	// Thread-safe access
	mu sync.RWMutex

	// Default jitter when lookup fails
	defaultJitter JitterVector
}

// SearchStats tracks flash search performance metrics
type SearchStats struct {
	Hits         uint64
	Misses       uint64
	CacheHits    uint64
	CacheMisses  uint64
	TotalLatency uint64 // microseconds
}

// NewFlashSearcher creates a new flash searcher with the given configuration
func NewFlashSearcher(config *JitterConfig) *FlashSearcher {
	if config == nil {
		config = DefaultJitterConfig()
	}

	return &FlashSearcher{
		jitterTable:   make(map[uint32]JitterVector),
		lruCache:      NewLRUCache(config.JitterCacheSize),
		stats:         &SearchStats{},
		config:        config,
		defaultJitter: config.DefaultJitter,
	}
}

// LoadJitterFromParquet loads jitter vectors from a parquet file
// This is a high-level wrapper that can be extended to support actual parquet parsing
func LoadJitterFromParquet(filename string) map[uint32]JitterVector {
	// For now, return an empty map
	// In production, this would parse the parquet file and extract jitter vectors
	fmt.Printf("[FlashSearch] Loading jitter table from: %s\n", filename)

	// Placeholder: Return empty table
	// Real implementation would:
	// 1. Open parquet file
	// 2. Read weight records
	// 3. Map hash prefixes to jitter vectors
	return make(map[uint32]JitterVector)
}

// LoadJitterTable loads a pre-built jitter table into the searcher
func (fs *FlashSearcher) LoadJitterTable(table map[uint32]JitterVector) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.jitterTable = make(map[uint32]JitterVector, len(table))
	for k, v := range table {
		fs.jitterTable[k] = v
	}

	// Clear cache when loading new table
	fs.lruCache.Clear()

	if fs.config.Verbose {
		fmt.Printf("[FlashSearch] Loaded %d jitter vectors\n", len(table))
	}
}

// Search performs a flash lookup for a jitter vector
// This is the core "Flash Search" operation that simulates high-speed SRAM lookup
func (fs *FlashSearcher) Search(hashKey uint32) (JitterVector, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// First, check the LRU cache for ultra-fast access
	if cached, found := fs.lruCache.Get(hashKey); found {
		fs.stats.CacheHits++
		return cached.(JitterVector), true
	}

	// Check the main jitter table
	jitter, exists := fs.jitterTable[hashKey]

	if exists {
		fs.stats.Hits++
		// Add to cache for future fast access
		fs.lruCache.Add(hashKey, jitter)
		return jitter, true
	}

	fs.stats.Misses++
	fs.stats.CacheMisses++

	// Return default jitter with false to indicate not found
	return fs.defaultJitter, false
}

// SearchFromHash extracts the lookup key from a hash and performs the search
func (fs *FlashSearcher) SearchFromHash(hash [32]byte) (JitterVector, bool) {
	key := ExtractLookupKey(hash)
	return fs.Search(key)
}

// GenerateDefaultJitter creates a deterministic default jitter for a given key
// Uses a simple hash-based fallback when no database entry exists
func (fs *FlashSearcher) GenerateDefaultJitter(key uint32) JitterVector {
	// XOR with a salt to create pseudo-random but deterministic jitter
	salt := uint32(0x9E3779B9) // Golden ratio approximation
	jitter := key ^ salt ^ (key >> 16)
	return JitterVector(jitter)
}

// GetOrGenerate returns a jitter vector, generating one if not found
func (fs *FlashSearcher) GetOrGenerate(hashKey uint32) JitterVector {
	jitter, found := fs.Search(hashKey)
	if found {
		return jitter
	}

	// Generate deterministic jitter
	return fs.GenerateDefaultJitter(hashKey)
}

// GetStats returns current search statistics
func (fs *FlashSearcher) GetStats() SearchStats {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	return SearchStats{
		Hits:         fs.stats.Hits,
		Misses:       fs.stats.Misses,
		CacheHits:    fs.stats.CacheHits,
		CacheMisses:  fs.stats.CacheMisses,
		TotalLatency: fs.stats.TotalLatency,
	}
}

// ResetStats clears all statistics
func (fs *FlashSearcher) ResetStats() {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.stats = &SearchStats{}
}

// Size returns the number of entries in the jitter table
func (fs *FlashSearcher) Size() int {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	return len(fs.jitterTable)
}

// AddJitter adds a new jitter vector to the table
func (fs *FlashSearcher) AddJitter(hashKey uint32, jitter JitterVector) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.jitterTable[hashKey] = jitter
}

// RemoveJitter removes a jitter vector from the table
func (fs *FlashSearcher) RemoveJitter(hashKey uint32) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	delete(fs.jitterTable, hashKey)
	fs.lruCache.Remove(hashKey)
}

// Clear removes all entries from the jitter table
func (fs *FlashSearcher) Clear() {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.jitterTable = make(map[uint32]JitterVector)
	fs.lruCache.Clear()
}

// BuildFromTrainingData constructs a jitter table from training results
// This creates the associative memory mapping between hash prefixes and optimal jitters
func (fs *FlashSearcher) BuildFromTrainingData(records []WeightRecord) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.jitterTable = make(map[uint32]JitterVector, len(records))

	for _, record := range records {
		// Use context key as the lookup key
		// The jitter is derived from the best seed
		if record.BestSeed != nil && len(record.BestSeed) >= 4 {
			jitterValue := binary.LittleEndian.Uint32(record.BestSeed[:4])
			fs.jitterTable[uint32(record.ContextKey)] = JitterVector(jitterValue)
		}
	}

	if fs.config.Verbose {
		fmt.Printf("[FlashSearch] Built jitter table with %d entries from training data\n", len(fs.jitterTable))
	}
}

// WeightRecord represents a training result for building the jitter table
type WeightRecord struct {
	TokenID      int32
	BestSeed     []byte
	FitnessScore float64
	Generation   int32
	ContextKey   uint32
}

// LRUCache implements a simple LRU cache for jitter vectors
type LRUCache struct {
	capacity int
	items    map[uint32]*lruItem
	head     *lruItem
	tail     *lruItem
	mu       sync.RWMutex
}

type lruItem struct {
	key   uint32
	value interface{}
	prev  *lruItem
	next  *lruItem
}

// NewLRUCache creates a new LRU cache with the given capacity
func NewLRUCache(capacity int) *LRUCache {
	cache := &LRUCache{
		capacity: capacity,
		items:    make(map[uint32]*lruItem),
		head:     &lruItem{},
		tail:     &lruItem{},
	}
	cache.head.next = cache.tail
	cache.tail.prev = cache.head
	return cache
}

// Get retrieves an item from the cache
func (c *LRUCache) Get(key uint32) (interface{}, bool) {
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	c.mu.Lock()
	c.moveToFront(item)
	c.mu.Unlock()

	return item.value, true
}

// Add inserts or updates an item in the cache
func (c *LRUCache) Add(key uint32, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		item.value = value
		c.moveToFront(item)
		return
	}

	item := &lruItem{
		key:   key,
		value: value,
	}
	c.items[key] = item
	c.addToFront(item)

	if len(c.items) > c.capacity {
		c.removeLRU()
	}
}

// Remove deletes an item from the cache
func (c *LRUCache) Remove(key uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		c.removeItem(item)
		delete(c.items, key)
	}
}

// Clear removes all items from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[uint32]*lruItem)
	c.head.next = c.tail
	c.tail.prev = c.head
}

// Len returns the number of items in the cache
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *LRUCache) addToFront(item *lruItem) {
	item.next = c.head.next
	item.prev = c.head
	c.head.next.prev = item
	c.head.next = item
}

func (c *LRUCache) moveToFront(item *lruItem) {
	c.removeItem(item)
	c.addToFront(item)
}

func (c *LRUCache) removeItem(item *lruItem) {
	item.prev.next = item.next
	item.next.prev = item.prev
}

func (c *LRUCache) removeLRU() {
	if len(c.items) == 0 {
		return
	}
	lru := c.tail.prev
	c.removeItem(lru)
	delete(c.items, lru.key)
}
