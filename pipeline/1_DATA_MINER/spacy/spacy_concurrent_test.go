package spacy

import (
	"sync"
	"testing"
)

// TestSimpleConcurrent tests basic concurrent operation
func TestSimpleConcurrent(t *testing.T) {
	// Create a single shared NLP instance with mutex protection
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	texts := []string{
		"First sentence for testing.",
		"Second sentence with more words.",
		"Third sentence is here.",
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0

	// Run 5 goroutines, each processing texts sequentially
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			for j := 0; j < 3; j++ {
				text := texts[j%len(texts)]

				// Serialize access to NLP
				mu.Lock()
				tokens := nlp.Tokenize(text)
				if len(tokens) > 0 {
					successCount++
				}
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	expectedOps := 15 // 5 goroutines * 3 iterations
	if successCount != expectedOps {
		t.Errorf("Expected %d successful operations, got %d", expectedOps, successCount)
	} else {
		t.Logf("Successfully completed %d operations", successCount)
	}
}

// TestSequentialMultiInstance tests multiple NLP instances used sequentially
func TestSequentialMultiInstance(t *testing.T) {
	// Test that we can create multiple instances and use them sequentially
	for i := 0; i < 3; i++ {
		nlp, err := NewNLP("en_core_web_sm")
		if err != nil {
			t.Fatalf("Failed to create NLP instance %d: %v", i, err)
		}

		text := "Testing multiple instances."
		tokens := nlp.Tokenize(text)
		if len(tokens) == 0 {
			t.Errorf("Instance %d: no tokens returned", i)
		}

		nlp.Close()
	}

	t.Log("Multiple sequential instances work correctly")
}
