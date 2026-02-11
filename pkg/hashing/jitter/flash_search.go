package jitter

import (
	"fmt"
	"math"
	"sync"
)

// FlashSearcher provides high-speed associative memory lookup for jitter vectors
// This implements the "Flash Search" mechanism with Dimension Shift (Search 0, Retrieve 1)
type FlashSearcher struct {
	// In-memory jitter lookup table
	// Key: Slot 0 (Subject), Value: Slot 1 (Predicate/Jitter)
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

// LoadJitterTable loads a pre-built jitter table into the searcher
func (fs *FlashSearcher) LoadJitterTable(table map[uint32]uint32) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.jitterTable = make(map[uint32]JitterVector, len(table))
	for k, v := range table {
		fs.jitterTable[k] = JitterVector(v)
	}

	// Clear cache when loading new table
	fs.lruCache.Clear()
}

// Search performs a flash lookup for a jitter vector using Dimension Shift
func (fs *FlashSearcher) Search(hashKey uint32) (JitterVector, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// 1. Check the LRU cache
	if cached, found := fs.lruCache.Get(hashKey); found {
		fs.stats.CacheHits++
		return cached.(JitterVector), true
	}

	// 2. Exact match in jitter table
	jitter, exists := fs.jitterTable[hashKey]
	if exists {
		fs.stats.Hits++
		fs.lruCache.Add(hashKey, jitter)
		return jitter, true
	}

	// 3. Nearest neighbor search (Recursive Pathfinder logic)
	// Treats the hashKey as a coordinate in the Slot 0 space
	if len(fs.jitterTable) > 0 {
		var bestKey uint32
		minDiff := uint32(math.MaxUint32)
		
		for k := range fs.jitterTable {
			var diff uint32
			if hashKey > k {
				diff = hashKey - k
			} else {
				diff = k - hashKey
			}
			
			if diff < minDiff {
				minDiff = diff
				bestKey = k
			}
		}
		
		jitter = fs.jitterTable[bestKey]
		fs.stats.Hits++
		fs.lruCache.Add(hashKey, jitter)
		return jitter, true
	}

	fs.stats.Misses++
	fs.stats.CacheMisses++

	// Return default jitter
	return fs.defaultJitter, false
}

// BuildFromTrainingData constructs a jitter table using Dimension Shift (Search 0, Retrieve 1)
func (fs *FlashSearcher) BuildFromTrainingData(frames []TrainingFrame) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.jitterTable = make(map[uint32]JitterVector, len(frames))

	for _, frame := range frames {
		// Key: Slot 0 (Subject coordinate)
		// Value: Slot 1 (Jitter nudge)
		fs.jitterTable[frame.AsicSlots[0]] = JitterVector(frame.AsicSlots[1])
	}

	if fs.config.Verbose {
		fmt.Printf("[FlashSearch] Built Dimension-Shift table with %d entries\n", len(fs.jitterTable))
	}
}

// GetStats returns current search statistics
func (fs *FlashSearcher) GetStats() SearchStats {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return *fs.stats
}

func (fs *FlashSearcher) ResetStats() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.stats = &SearchStats{}
}

func (fs *FlashSearcher) Size() int {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return len(fs.jitterTable)
}

// GenerateDefaultJitter creates a deterministic default jitter
func (fs *FlashSearcher) GenerateDefaultJitter(key uint32) JitterVector {
	salt := uint32(0x9E3779B9)
	jitter := key ^ salt ^ (key >> 16)
	return JitterVector(jitter)
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

func (c *LRUCache) Add(key uint32, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item, exists := c.items[key]; exists {
		item.value = value
		c.moveToFront(item)
		return
	}
	item := &lruItem{key: key, value: value}
	c.items[key] = item
	c.addToFront(item)
	if len(c.items) > c.capacity {
		c.removeLRU()
	}
}

func (c *LRUCache) moveToFront(item *lruItem) {
	c.removeItem(item)
	c.addToFront(item)
}

func (c *LRUCache) removeItem(item *lruItem) {
	item.prev.next = item.next
	item.next.prev = item.prev
}

func (c *LRUCache) addToFront(item *lruItem) {
	item.next = c.head.next
	item.prev = c.head
	c.head.next.prev = item
	c.head.next = item
}

func (c *LRUCache) removeLRU() {
	if len(c.items) == 0 {
		return
	}
	lru := c.tail.prev
	c.removeItem(lru)
	delete(c.items, lru.key)
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[uint32]*lruItem)
	c.head.next = c.tail
	c.tail.prev = c.head
}

func (c *LRUCache) Remove(key uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item, exists := c.items[key]; exists {
		c.removeItem(item)
		delete(c.items, key)
	}
}
