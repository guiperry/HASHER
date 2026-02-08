package sliding

import (
	"math"
	"testing"
)

func TestNewGenerator(t *testing.T) {
	windowSize := 128
	stride := 1

	gen := NewGenerator(windowSize, stride)

	if gen.GetWindowSize() != windowSize {
		t.Errorf("expected window size %d, got %d", windowSize, gen.GetWindowSize())
	}

	if gen.GetStride() != stride {
		t.Errorf("expected stride %d, got %d", stride, gen.GetStride())
	}
}

func TestGenerateWindows(t *testing.T) {
	gen := NewGenerator(3, 1) // Small window for testing

	t.Run("Empty tokens", func(t *testing.T) {
		windows := gen.GenerateWindows([]int{})
		if len(windows) != 0 {
			t.Errorf("expected 0 windows for empty input, got %d", len(windows))
		}
	})

	t.Run("Single token", func(t *testing.T) {
		windows := gen.GenerateWindows([]int{1})
		if len(windows) != 0 {
			t.Errorf("expected 0 windows for single token, got %d", len(windows))
		}
	})

	t.Run("Two tokens", func(t *testing.T) {
		tokens := []int{1, 2}
		windows := gen.GenerateWindows(tokens)

		if len(windows) != 1 {
			t.Errorf("expected 1 window for 2 tokens, got %d", len(windows))
		}

		window := windows[0]
		if window.StartPos != 0 {
			t.Errorf("expected start pos 0, got %d", window.StartPos)
		}
		if window.EndPos != 1 {
			t.Errorf("expected end pos 1, got %d", window.EndPos)
		}
		if len(window.ContextTokens) != 1 {
			t.Errorf("expected 1 context token, got %d", len(window.ContextTokens))
		}
		if window.ContextTokens[0] != 1 {
			t.Errorf("expected context token 1, got %d", window.ContextTokens[0])
		}
		if window.TargetToken != 2 {
			t.Errorf("expected target token 2, got %d", window.TargetToken)
		}
	})

	t.Run("Multiple tokens with small window", func(t *testing.T) {
		tokens := []int{1, 2, 3, 4, 5}
		windows := gen.GenerateWindows(tokens)

		expectedWindows := 4 // One for each possible target (positions 1-4)
		if len(windows) != expectedWindows {
			t.Errorf("expected %d windows, got %d", expectedWindows, len(windows))
		}

		// Check first window: [1] -> target 2
		if windows[0].TargetToken != 2 {
			t.Errorf("window 0: expected target 2, got %d", windows[0].TargetToken)
		}
		if len(windows[0].ContextTokens) != 1 {
			t.Errorf("window 0: expected 1 context token, got %d", len(windows[0].ContextTokens))
		}

		// Check third window: [1,2,3] -> target 4 (window size reached)
		if windows[2].StartPos != 0 {
			t.Errorf("window 2: expected start pos 0, got %d", windows[2].StartPos)
		}
		if windows[2].EndPos != 3 {
			t.Errorf("window 2: expected end pos 3, got %d", windows[2].EndPos)
		}
		if len(windows[2].ContextTokens) != 3 {
			t.Errorf("window 2: expected 3 context tokens, got %d", len(windows[2].ContextTokens))
		}
		if windows[2].TargetToken != 4 {
			t.Errorf("window 2: expected target 4, got %d", windows[2].TargetToken)
		}

		// Check last window: [2,3,4] -> target 5
		if windows[3].StartPos != 1 {
			t.Errorf("window 3: expected start pos 1, got %d", windows[3].StartPos)
		}
		if windows[3].TargetToken != 5 {
			t.Errorf("window 3: expected target 5, got %d", windows[3].TargetToken)
		}
	})
}

func TestGenerateWindowsWithStride(t *testing.T) {
	gen := NewGenerator(3, 2) // Window size 3, stride 2
	tokens := []int{1, 2, 3, 4, 5, 6, 7}
	windows := gen.GenerateWindows(tokens)

	// Should generate windows at positions 1, 3, 5 (stride 2)
	expectedWindows := 3
	if len(windows) != expectedWindows {
		t.Errorf("expected %d windows with stride 2, got %d", expectedWindows, len(windows))
	}

	// Check window positions
	// With sliding windows, context accumulates up to windowSize before sliding
	// Window size 3 means max 3 context tokens
	expectedPositions := []struct{ start, end, target int }{
		{0, 1, 2}, // Position 1: context [1], target 2
		{0, 3, 4}, // Position 3: context [1,2,3] (3 tokens = window size), target 4
		{2, 5, 6}, // Position 5: context [3,4,5], target 6
	}

	for i, expected := range expectedPositions {
		if windows[i].StartPos != expected.start {
			t.Errorf("window %d: expected start %d, got %d", i, expected.start, windows[i].StartPos)
		}
		if windows[i].EndPos != expected.end {
			t.Errorf("window %d: expected end %d, got %d", i, expected.end, windows[i].EndPos)
		}
		if windows[i].TargetToken != expected.target {
			t.Errorf("window %d: expected target %d, got %d", i, expected.target, windows[i].TargetToken)
		}
	}
}

func TestGenerateWindowsLargeInput(t *testing.T) {
	gen := NewGenerator(128, 1) // Typical configuration

	// Generate 1000 tokens
	tokens := make([]int, 1000)
	for i := range tokens {
		tokens[i] = i + 1
	}

	windows := gen.GenerateWindows(tokens)
	expectedWindows := 999 // 1000 - 1 (can't predict from position 0)

	if len(windows) != expectedWindows {
		t.Errorf("expected %d windows for 1000 tokens, got %d", expectedWindows, len(windows))
	}

	// Check first window
	if windows[0].StartPos != 0 {
		t.Errorf("first window: expected start 0, got %d", windows[0].StartPos)
	}
	if windows[0].EndPos != 1 {
		t.Errorf("first window: expected end 1, got %d", windows[0].EndPos)
	}
	if len(windows[0].ContextTokens) != 1 {
		t.Errorf("first window: expected 1 context token, got %d", len(windows[0].ContextTokens))
	}

	// Check window after full context is established (position 128)
	if len(windows) > 128 {
		window128 := windows[128]
		if window128.StartPos != 1 {
			t.Errorf("window 128: expected start 1, got %d", window128.StartPos)
		}
		if len(window128.ContextTokens) != 128 {
			t.Errorf("window 128: expected 128 context tokens, got %d", len(window128.ContextTokens))
		}
	}
}

func TestEstimateWindowCount(t *testing.T) {
	gen := NewGenerator(128, 1)

	tests := []struct {
		tokenCount      int
		expectedWindows int
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{10, 9},
		{100, 99},
		{1000, 999},
	}

	for _, tt := range tests {
		count := gen.EstimateWindowCount(tt.tokenCount)
		if count != tt.expectedWindows {
			t.Errorf("EstimateWindowCount(%d) = %d, want %d", tt.tokenCount, count, tt.expectedWindows)
		}
	}
}

func TestEstimateWindowCountWithStride(t *testing.T) {
	gen := NewGenerator(128, 2) // Stride 2

	// For stride 2, we should get approximately half the windows
	tokenCount := 100
	expectedApprox := int(math.Ceil(float64(tokenCount-1) / 2.0))

	count := gen.EstimateWindowCount(tokenCount)
	if count != expectedApprox {
		t.Errorf("EstimateWindowCount(%d) with stride 2 = %d, want %d", tokenCount, count, expectedApprox)
	}
}

func BenchmarkGenerateWindowsSmall(b *testing.B) {
	gen := NewGenerator(128, 1)
	tokens := make([]int, 100)
	for i := range tokens {
		tokens[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.GenerateWindows(tokens)
	}
}

func BenchmarkGenerateWindowsLarge(b *testing.B) {
	gen := NewGenerator(128, 1)
	tokens := make([]int, 1000)
	for i := range tokens {
		tokens[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.GenerateWindows(tokens)
	}
}
