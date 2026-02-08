package tokenizer

import (
	"testing"
)

func TestNew(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	if svc == nil {
		t.Fatal("expected service to not be nil")
	}

	if svc.model == nil {
		t.Fatal("expected model to be initialized")
	}
}

func TestEncode(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	tests := []struct {
		name   string
		text   string
		minLen int
		maxLen int
	}{
		{
			name:   "simple text",
			text:   "Hello World",
			minLen: 1,
			maxLen: 10,
		},
		{
			name:   "empty string",
			text:   "",
			minLen: 0,
			maxLen: 0,
		},
		{
			name:   "long text",
			text:   "This is a longer piece of text that should be tokenized into multiple tokens for testing purposes",
			minLen: 10,
			maxLen: 50,
		},
		{
			name:   "special characters",
			text:   "Hello! @#$% World...",
			minLen: 1,
			maxLen: 20,
		},
		{
			name:   "unicode text",
			text:   "Hello 世界 🌍",
			minLen: 3,
			maxLen: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := svc.Encode(tt.text)

			if len(tokens) < tt.minLen {
				t.Errorf("expected at least %d tokens, got %d", tt.minLen, len(tokens))
			}
			if len(tokens) > tt.maxLen {
				t.Errorf("expected at most %d tokens, got %d", tt.maxLen, len(tokens))
			}

			// Verify all tokens are positive
			for i, token := range tokens {
				if token < 0 {
					t.Errorf("token %d is negative: %d", i, token)
				}
			}
		})
	}
}

func TestDecode(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	tests := []struct {
		name string
		text string
	}{
		{"simple", "Hello World"},
		{"punctuation", "Hello, World!"},
		{"numbers", "The year is 2024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := svc.Encode(tt.text)
			decoded := svc.Decode(tokens)

			// Note: Due to tokenization, the decoded text might not be identical
			// but should be semantically similar. For now, just verify it doesn't panic.
			if decoded == "" && len(tokens) > 0 {
				t.Error("expected non-empty decoded text")
			}
		})
	}
}

func TestCount(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"Hello", 1},
		{"Hello World", 2},
		{"This is a test", 4},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			count := svc.Count(tt.text)
			if count != tt.expected {
				t.Errorf("Count(%q) = %d, want %d", tt.text, count, tt.expected)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	text := "The quick brown fox jumps over the lazy dog"
	tokens := svc.Encode(text)
	decoded := svc.Decode(tokens)

	// Re-encode the decoded text to verify consistency
	tokens2 := svc.Encode(decoded)

	if len(tokens) != len(tokens2) {
		t.Errorf("token count mismatch: original=%d, decoded=%d", len(tokens), len(tokens2))
	}

	for i := range tokens {
		if i < len(tokens2) && tokens[i] != tokens2[i] {
			t.Errorf("token %d mismatch: original=%d, decoded=%d", i, tokens[i], tokens2[i])
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	svc, _ := New()
	text := "This is a benchmark test for tokenization performance"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Encode(text)
	}
}

func BenchmarkEncodeLong(b *testing.B) {
	svc, _ := New()
	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Encode(text)
	}
}
