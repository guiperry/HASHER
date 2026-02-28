package jitter

import (
	"fmt"
	"math"
	"sync"
)

// FlashSearcher provides high-speed associative memory lookup for jitter vectors
// This implements the "Optiplex Logic" described in COHERENCE.md
type FlashSearcher struct {
	// In-memory Knowledge Base
	knowledgeBase []TrainingFrame

	// Indices for fast lookup
	slot0Index     map[uint32][]int // For Zone 1 (Topic)
	domainPosIndex map[uint32][]int // For Zone 2 (Grammar): Key = (Domain&0xF000) | (POS&0xFF)
	slot3Index     map[uint32]int   // For Zone 3 (Identity): Key = Slot 3

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
		knowledgeBase:  make([]TrainingFrame, 0),
		slot0Index:     make(map[uint32][]int),
		domainPosIndex: make(map[uint32][]int),
		slot3Index:     make(map[uint32]int),
		lruCache:       NewLRUCache(config.JitterCacheSize),
		stats:          &SearchStats{},
		config:         config,
		defaultJitter:  config.DefaultJitter,
	}
}

// LoadDomainTables is deprecated/removed in favor of BuildFromTrainingData
func (fs *FlashSearcher) LoadDomainTables(tables map[uint32]map[uint32]uint32) {
	// No-op or throw error - this structure no longer fits the 3-Zone logic
}

// LoadJitterTable loads a jitter table into the searcher
func (fs *FlashSearcher) LoadJitterTable(table map[uint32]uint32) {
	frames := make([]TrainingFrame, 0, len(table))
	for k, v := range table {
		var slots [12]uint32
		slots[0] = k // Slot 0: Anchor
		slots[1] = v // Slot 1: Jitter
		// Other slots default to 0
		
		frames = append(frames, TrainingFrame{
			AsicSlots: slots,
		})
	}
	fs.BuildFromTrainingData(frames)
}

// Search performs a flash lookup for a jitter vector using the 3-Zone Logic
func (fs *FlashSearcher) Search(slots [12]uint32, currentHash uint32, pass int) (JitterVector, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Zone Logic based on Pass Number
	if pass <= 7 {
		// Zone 1: The Topic Filter
		// Focus: Slot 0 (The Anchor)
		// Logic: Find neighbor with closest Slot 0
		return fs.searchZone1(slots[0], currentHash)
	} else if pass <= 14 {
		// Zone 2: The Grammatical Filter
		// Focus: Slot 4 (POS) & Slot 10 (Domain) + Slot 1/2
		// Logic: Filter by Domain/POS, then find nearest neighbor on Slot 1/2
		return fs.searchZone2(slots, currentHash)
	} else {
		// Zone 3: The Specificity Filter
		// Focus: Slot 3 (Entropy)
		// Logic: Check for stabilization lock on Slot 3
		return fs.searchZone3(slots[3], currentHash)
	}
}

// searchZone1 implements Topic Filter (Slot 0)
func (fs *FlashSearcher) searchZone1(targetSlot0 uint32, currentHash uint32) (JitterVector, bool) {
	// Simplified: Check if currentHash is "close" to targetSlot0?
	// COHERENCE.md: "Host checks if ASIC's current hash is 'Semantically Neighborly' to the Global Anchor."
	// "If the hash drifts... return High-Entropy Jitter."
	// "It looks for a neighbor in the Arrow DB based on Slot 0."
	
	// We search our KB for records with this Slot 0 (or close to it)
	// For exact match on Slot 0 (Variance Anchor):
	indices, ok := fs.slot0Index[targetSlot0]
	if !ok || len(indices) == 0 {
		// No records with this anchor. Return default (high entropy).
		return fs.defaultJitter, false 
	}
	
	// Just pick the first valid neighbor's jitter (Slot 1)
	// In a real implementation, we might do nearest neighbor on Hash vs Entry.
	idx := indices[0]
	jitter := JitterVector(fs.knowledgeBase[idx].AsicSlots[1])
	return jitter, true
}

// searchZone2 implements Grammatical Filter (Slot 4 + 10)
func (fs *FlashSearcher) searchZone2(slots [12]uint32, currentHash uint32) (JitterVector, bool) {
	domain := slots[10] & 0xF000
	pos := slots[4] & 0xFF
	key := domain | pos
	
	indices, ok := fs.domainPosIndex[key]
	if !ok || len(indices) == 0 {
		return fs.defaultJitter, false
	}
	
	// "Retrieves the Slot 1 or 2 jitter from a neighbor that is a Verb"
	// We find the nearest neighbor in terms of Slot 1 (Subject) distance to currentHash?
	// COHERENCE.md says: "Nudges nonces to satisfy POS/Tense requirements."
	
	// Let's find the record with the closest Slot 1 to our currentHash
	bestDist := uint32(math.MaxUint32)
	bestIdx := -1
	
	for _, idx := range indices {
		recSlot1 := fs.knowledgeBase[idx].AsicSlots[1]
		var dist uint32
		if currentHash > recSlot1 {
			dist = currentHash - recSlot1
		} else {
			dist = recSlot1 - currentHash
		}
		
		if dist < bestDist {
			bestDist = dist
			bestIdx = idx
		}
	}
	
	if bestIdx != -1 {
		// Return Slot 2 as the "Action" jitter
		return JitterVector(fs.knowledgeBase[bestIdx].AsicSlots[2]), true
	}
	
	return fs.defaultJitter, false
}

// searchZone3 implements Specificity Filter (Slot 3)
func (fs *FlashSearcher) searchZone3(targetSlot3 uint32, currentHash uint32) (JitterVector, bool) {
	// "Stabilizer: If current hash is within Hamming Distance of a high-prob Token ID... return Targeted XOR"
	// "Targeted XOR = diff ^ 0xFEEDFACE"
	// We need the Target Token's fingerprint (Slot 3).
	
	// Check if currentHash is close to the targetSlot3 (Entropy Fingerprint)
	diff := currentHash ^ targetSlot3
	
	// Threshold check (leading zeros)
	// COHERENCE.md example: 12 bits
	zeros := 0
	for i := 31; i >= 0; i-- {
		if (diff>>i)&1 == 0 {
			zeros++
		} else {
			break
		}
	}
	
	if zeros < 12 {
		// Not close enough, keep searching (high entropy)
		return fs.defaultJitter, false
	}
	
	// Stabilizer: deterministic slide
	return JitterVector(diff ^ 0xFEEDFACE), true
}

// BuildFromTrainingData constructs the indices from training frames
func (fs *FlashSearcher) BuildFromTrainingData(frames []TrainingFrame) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.knowledgeBase = make([]TrainingFrame, len(frames))
	copy(fs.knowledgeBase, frames)
	
	fs.slot0Index = make(map[uint32][]int)
	fs.domainPosIndex = make(map[uint32][]int)
	fs.slot3Index = make(map[uint32]int)

	for i, frame := range frames {
		// Index Slot 0
		s0 := frame.AsicSlots[0]
		fs.slot0Index[s0] = append(fs.slot0Index[s0], i)
		
		// Index Domain+POS
		domain := frame.AsicSlots[10] & 0xF000
		pos := frame.AsicSlots[4] & 0xFF
		key := domain | pos
		fs.domainPosIndex[key] = append(fs.domainPosIndex[key], i)
		
		// Index Slot 3 (Assume unique for simplicity, or overwrite)
		fs.slot3Index[frame.AsicSlots[3]] = i
	}

	if fs.config.Verbose {
		fmt.Printf("[FlashSearch] Built 3-Zone Indices from %d frames\n", len(frames))
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
	return len(fs.knowledgeBase)
}

// LookupByNonce performs the Reverse Lookup step of the HASHER inference pipeline.
// Given the golden nonce (first 4 bytes of the JitterEngine's final hash), it finds the
// training frame whose TargetTokenID matches the projected nonce and returns that token ID.
// This implements the "hash → word" lookup described in the architecture specification:
// the Result_Hash IS the address of the next token in the Arrow Knowledge Base.
// Returns 0, false when the knowledge base is empty or contains no matching frame.
func (fs *FlashSearcher) LookupByNonce(nonce uint32, vocabSize int) (int, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if len(fs.knowledgeBase) == 0 {
		return 0, false
	}

	// Project the nonce into the vocabulary range for comparison
	targetID := int32(nonce % uint32(vocabSize))

	for _, frame := range fs.knowledgeBase {
		if frame.TargetTokenID == targetID {
			return int(frame.TargetTokenID), true
		}
	}

	return 0, false
}

// LookupByContext performs a semantic lookup based on the current context tokens.
// This provides a deterministic "Exact Match" path for known training patterns,
// ensuring the system can accurately reproduce sequences it has been explicitly trained on.
func (fs *FlashSearcher) LookupByContext(context []int) (int, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if len(fs.knowledgeBase) == 0 || len(context) == 0 {
		return 0, false
	}

	bestMatchID := -1
	maxMatchLen := 0
	maxStartPos := -1

	for _, frame := range fs.knowledgeBase {
		if len(frame.TokenSequence) == 0 {
			continue
		}

		// Try to find the frame's token sequence as a sub-sequence within the context
		for start := 0; start <= len(context)-len(frame.TokenSequence); start++ {
			match := true
			for i, tok := range frame.TokenSequence {
				if context[start+i] != tok {
					match = false
					break
				}
			}

			if match {
				// We prioritize:
				// 1. Longer matches (more specific)
				// 2. Later start positions (more recent context)
				if len(frame.TokenSequence) > maxMatchLen || (len(frame.TokenSequence) == maxMatchLen && start > maxStartPos) {
					maxMatchLen = len(frame.TokenSequence)
					maxStartPos = start
					bestMatchID = int(frame.TargetTokenID)
				}
			}
		}
	}

	if bestMatchID != -1 {
		return bestMatchID, true
	}

	return 0, false
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
