package sliding

import (
	"math"
)

// SlidingWindow represents a single sliding window context
type SlidingWindow struct {
	StartPos      int   // Start token position
	EndPos        int   // End token position (exclusive)
	ContextTokens []int // Tokens for context embedding
	TargetToken   int   // Token to predict
}

// Generator handles sliding window generation
type Generator struct {
	windowSize int
	stride     int
}

// NewGenerator creates a new sliding window generator
func NewGenerator(windowSize, stride int) *Generator {
	return &Generator{
		windowSize: windowSize,
		stride:     stride,
	}
}

// GenerateWindows creates sliding windows from token sequence
func (g *Generator) GenerateWindows(tokens []int) []SlidingWindow {
	var windows []SlidingWindow

	if len(tokens) < 2 {
		return windows // Need at least context + target
	}

	// Generate windows starting from position 1 (first possible target)
	for i := 1; i < len(tokens); i += g.stride {
		// Calculate context window boundaries
		// Use max(0, i-windowSize) to accumulate context up to windowSize before sliding
		start := max(0, i-g.windowSize)
		end := i // Target token at position i

		// Extract context tokens (everything before target)
		contextTokens := tokens[start:end]

		if len(contextTokens) == 0 {
			continue
		}

		window := SlidingWindow{
			StartPos:      start,
			EndPos:        end,
			ContextTokens: contextTokens,
			TargetToken:   tokens[i],
		}

		windows = append(windows, window)
	}

	return windows
}

// EstimateWindowCount estimates the number of windows for a given token count
func (g *Generator) EstimateWindowCount(tokenCount int) int {
	if tokenCount < 2 {
		return 0
	}

	// Calculate approximate number of windows
	availableTargets := tokenCount - 1
	if g.stride == 1 {
		return availableTargets
	}

	return int(math.Ceil(float64(availableTargets) / float64(g.stride)))
}

// GetWindowSize returns the configured window size
func (g *Generator) GetWindowSize() int {
	return g.windowSize
}

// GetStride returns the configured stride
func (g *Generator) GetStride() int {
	return g.stride
}

// Helper functions
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
