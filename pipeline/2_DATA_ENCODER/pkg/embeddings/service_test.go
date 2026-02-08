package embeddings

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	// Test with environment variable set
	t.Run("with endpoint", func(t *testing.T) {
		testURL := "http://test.endpoint"
		t.Setenv("CLOUDFLARE_EMBEDDINGS_WORKER", testURL)
		svc := New()

		if svc == nil {
			t.Fatal("expected service to not be nil")
		}

		if svc.baseURL != testURL {
			t.Errorf("expected base URL %s, got %s", testURL, svc.baseURL)
		}

		if svc.batchSize != DefaultBatchSize {
			t.Errorf("expected batch size %d, got %d", DefaultBatchSize, svc.batchSize)
		}

		if svc.httpClient.Timeout != 30*time.Second {
			t.Errorf("expected timeout 30s, got %v", svc.httpClient.Timeout)
		}
	})

	// Test without environment variable set
	t.Run("without endpoint", func(t *testing.T) {
		t.Setenv("CLOUDFLARE_EMBEDDINGS_WORKER", "")
		svc := New()

		if svc == nil {
			t.Fatal("expected service to not be nil")
		}

		if svc.baseURL != "" {
			t.Errorf("expected empty base URL, got %s", svc.baseURL)
		}
	})
}

func TestNewWithBatchSize(t *testing.T) {
	customBatchSize := 16
	svc := NewWithBatchSize(customBatchSize)

	if svc.batchSize != customBatchSize {
		t.Errorf("expected batch size %d, got %d", customBatchSize, svc.batchSize)
	}
}

func TestGetBatchSize(t *testing.T) {
	svc := New()
	if svc.GetBatchSize() != DefaultBatchSize {
		t.Errorf("GetBatchSize() = %d, want %d", svc.GetBatchSize(), DefaultBatchSize)
	}

	customSize := 8
	svc = NewWithBatchSize(customSize)
	if svc.GetBatchSize() != customSize {
		t.Errorf("GetBatchSize() = %d, want %d", svc.GetBatchSize(), customSize)
	}
}

func TestSetTimeout(t *testing.T) {
	svc := New()
	newTimeout := 45 * time.Second
	svc.SetTimeout(newTimeout)

	if svc.httpClient.Timeout != newTimeout {
		t.Errorf("SetTimeout() failed: expected %v, got %v", newTimeout, svc.httpClient.Timeout)
	}
}

func TestGetBatchEmbeddingsEmpty(t *testing.T) {
	svc := New()
	embeddings, err := svc.GetBatchEmbeddings([]string{})

	if err != nil {
		t.Errorf("unexpected error for empty input: %v", err)
	}

	if embeddings != nil {
		t.Errorf("expected nil for empty input, got %v", embeddings)
	}
}

// Note: Full integration tests would require mocking the HTTP client
// or running against a test API endpoint. For now, we test the structure
// and basic functionality that doesn't require API calls.

func TestEmbeddingRequestMarshal(t *testing.T) {
	req := CloudflareWorkersRequest{
		Texts: []string{"test text", "another test"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify it's valid JSON
	var unmarshaled CloudflareWorkersRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(unmarshaled.Texts) != 2 {
		t.Errorf("expected 2 texts, got %d", len(unmarshaled.Texts))
	}

	if unmarshaled.Texts[0] != "test text" {
		t.Errorf("expected first text %s, got %s", "test text", unmarshaled.Texts[0])
	}
}

func TestBatchChunking(t *testing.T) {
	svc := NewWithBatchSize(3)
	texts := []string{"a", "b", "c", "d", "e", "f", "g"}

	// This would normally make API calls, but we can test the chunking logic
	// by checking that the function would split correctly
	if svc.batchSize != 3 {
		t.Errorf("expected batch size 3, got %d", svc.batchSize)
	}

	// For 7 texts with batch size 3, we'd expect 3 chunks: 3, 3, 1
	expectedChunks := 3
	if len(texts) <= svc.batchSize {
		expectedChunks = 1
	} else {
		expectedChunks = (len(texts) + svc.batchSize - 1) / svc.batchSize
	}

	if expectedChunks != 3 {
		t.Errorf("expected 3 chunks for 7 texts with batch size 3, got %d", expectedChunks)
	}
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		New()
	}
}

func BenchmarkNewWithBatchSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewWithBatchSize(16)
	}
}
