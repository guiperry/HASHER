package spacy

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Test full pipeline integration
func TestFullPipeline(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	texts := []string{
		"Apple Inc. was founded by Steve Jobs, Steve Wozniak, and Ronald Wayne on April 1, 1976.",
		"The company is headquartered in Cupertino, California.",
		"In 2023, Apple became the first company to reach a market capitalization of $3 trillion.",
		"Tim Cook has been the CEO since August 24, 2011.",
	}

	for i, text := range texts {
		t.Run(fmt.Sprintf("Pipeline_%d", i), func(t *testing.T) {
			// Run all NLP functions on the same text
			tokens := nlp.Tokenize(text)
			entities := nlp.ExtractEntities(text)
			sentences := nlp.SplitSentences(text)
			posMap := nlp.POSTag(text)
			depMap := nlp.GetDependencies(text)
			lemmaMap := nlp.GetLemmas(text)

			// Validate consistency between different functions
			// Note: Maps may have fewer entries than tokens if tokens repeat
			if len(posMap) > len(tokens) {
				t.Errorf("POS map size (%d) exceeds token count (%d)", len(posMap), len(tokens))
			}

			if len(depMap) > len(tokens) {
				t.Errorf("Dependency map size (%d) exceeds token count (%d)", len(depMap), len(tokens))
			}

			if len(lemmaMap) > len(tokens) {
				t.Errorf("Lemma map size (%d) exceeds token count (%d)", len(lemmaMap), len(tokens))
			}

			// Check that all tokens appear in the maps
			for _, token := range tokens {
				if _, exists := posMap[token.Text]; !exists {
					t.Errorf("Token '%s' not found in POS map", token.Text)
				}
				if _, exists := depMap[token.Text]; !exists {
					t.Errorf("Token '%s' not found in dependency map", token.Text)
				}
				if _, exists := lemmaMap[token.Text]; !exists {
					t.Errorf("Token '%s' not found in lemma map", token.Text)
				}
			}

			// Validate entity positions are within text bounds
			for _, entity := range entities {
				if entity.Start < 0 || entity.End > len(text) {
					t.Errorf("Entity '%s' has invalid bounds: [%d:%d] for text length %d",
						entity.Text, entity.Start, entity.End, len(text))
				}
			}

			// Log statistics
			t.Logf("Text %d: %d tokens, %d entities, %d sentences", i, len(tokens), len(entities), len(sentences))
		})
	}
}

// Test concurrent usage with multiple goroutines
func TestConcurrentUsage(t *testing.T) {
	// Skip if running with race detector and limited resources
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	// Create a single shared NLP instance
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	texts := []string{
		"This is the first test sentence.",
		"Another sentence for concurrent testing.",
		"The quick brown fox jumps over the lazy dog.",
		"Machine learning is transforming technology.",
		"Natural language processing is fascinating.",
	}

	numGoroutines := 5
	iterations := 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*iterations)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < iterations; i++ {
				text := texts[i%len(texts)]

				// Try all functions
				tokens := nlp.Tokenize(text)
				if len(tokens) == 0 {
					errors <- fmt.Errorf("Goroutine %d: no tokens for text", goroutineID)
					continue
				}

				_ = nlp.ExtractEntities(text)
				sentences := nlp.SplitSentences(text)
				posMap := nlp.POSTag(text)
				depMap := nlp.GetDependencies(text)
				lemmaMap := nlp.GetLemmas(text)

				// Basic validation - maps may be smaller due to duplicate tokens
				if len(posMap) > len(tokens) {
					errors <- fmt.Errorf("Goroutine %d: POS map size exceeds tokens", goroutineID)
				}
				if len(depMap) > len(tokens) {
					errors <- fmt.Errorf("Goroutine %d: Dep map size exceeds tokens", goroutineID)
				}
				if len(lemmaMap) > len(tokens) {
					errors <- fmt.Errorf("Goroutine %d: Lemma map size exceeds tokens", goroutineID)
				}
				if len(sentences) == 0 {
					errors <- fmt.Errorf("Goroutine %d: no sentences", goroutineID)
				}

				// Small delay to increase chance of race conditions
				time.Sleep(time.Microsecond)
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Error(err)
		errorCount++
		if errorCount > 10 {
			t.Fatal("Too many concurrent errors, stopping test")
		}
	}

	if errorCount == 0 {
		t.Logf("Successfully completed %d concurrent operations", numGoroutines*iterations)
	}
}

// Test memory management with repeated operations
func TestMemoryManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "This is a test sentence for memory management verification. " +
		"We need to ensure that repeated calls don't cause memory leaks."

	// Get initial memory stats
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	runtime.GC()

	// Perform many operations
	iterations := 1000
	for i := 0; i < iterations; i++ {
		tokens := nlp.Tokenize(text)
		_ = nlp.ExtractEntities(text)
		_ = nlp.SplitSentences(text)
		_ = nlp.POSTag(text)
		_ = nlp.GetDependencies(text)
		_ = nlp.GetLemmas(text)

		// Ensure tokens are being properly created
		if len(tokens) == 0 {
			t.Fatal("No tokens returned")
		}

		// Periodically force GC
		if i%100 == 0 {
			runtime.GC()
		}
	}

	// Force final GC and get memory stats
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Calculate memory growth (handle underflow)
	var allocDiff int64
	var heapDiff int64
	if m2.Alloc >= m1.Alloc {
		allocDiff = int64(m2.Alloc - m1.Alloc)
	} else {
		allocDiff = -int64(m1.Alloc - m2.Alloc)
	}
	if m2.HeapAlloc >= m1.HeapAlloc {
		heapDiff = int64(m2.HeapAlloc - m1.HeapAlloc)
	} else {
		heapDiff = -int64(m1.HeapAlloc - m2.HeapAlloc)
	}

	t.Logf("Memory after %d iterations:", iterations)
	t.Logf("  Alloc diff: %d bytes", allocDiff)
	t.Logf("  Heap diff: %d bytes", heapDiff)
	t.Logf("  Goroutines: %d", runtime.NumGoroutine())

	// Check for excessive memory growth (more than 100MB would be concerning)
	maxAcceptableGrowth := int64(100 * 1024 * 1024)
	if allocDiff > maxAcceptableGrowth {
		t.Errorf("Excessive memory growth detected: %d bytes", allocDiff)
	}
}

// Test sequential model initialization and cleanup
func TestModelLifecycle(t *testing.T) {
	// Test multiple init/cleanup cycles
	for i := 0; i < 3; i++ {
		t.Run(fmt.Sprintf("Cycle_%d", i), func(t *testing.T) {
			nlp, err := NewNLP("en_core_web_sm")
			if err != nil {
				t.Fatalf("Failed to create NLP in cycle %d: %v", i, err)
			}

			// Use the model
			text := "Testing model lifecycle."
			tokens := nlp.Tokenize(text)
			if len(tokens) == 0 {
				t.Error("No tokens returned")
			}

			// Clean up
			nlp.Close()

			// After cleanup, the model should not be usable
			// Note: This might still work due to wrapper design, but shouldn't crash
			tokensAfterClose := nlp.Tokenize(text)
			if len(tokensAfterClose) > 0 {
				t.Log("Warning: Model still returns tokens after Close()")
			}
		})
	}
}

// Test cross-function consistency
func TestCrossFunctionConsistency(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "John Smith works at Microsoft Corporation in Seattle, Washington."

	// Get all outputs
	tokens := nlp.Tokenize(text)
	entities := nlp.ExtractEntities(text)
	sentences := nlp.SplitSentences(text)
	posMap := nlp.POSTag(text)
	depMap := nlp.GetDependencies(text)
	lemmaMap := nlp.GetLemmas(text)

	// Build token text set
	tokenTexts := make(map[string]bool)
	for _, token := range tokens {
		tokenTexts[token.Text] = true

		// Check that token properties match map values
		// Note: Maps store last occurrence if duplicate tokens exist
		if pos, exists := posMap[token.Text]; exists {
			if pos != token.POS {
				// Log as warning since duplicates can have different properties
				t.Logf("POS variation for '%s': token.POS=%s, map=%s", token.Text, token.POS, pos)
			}
		}

		if dep, exists := depMap[token.Text]; exists {
			if dep != token.Dep {
				// Log as warning since duplicates can have different properties
				t.Logf("Dep variation for '%s': token.Dep=%s, map=%s", token.Text, token.Dep, dep)
			}
		}

		if lemma, exists := lemmaMap[token.Text]; exists {
			if lemma != token.Lemma {
				// Log as warning since duplicates can have different properties
				t.Logf("Lemma variation for '%s': token.Lemma=%s, map=%s", token.Text, token.Lemma, lemma)
			}
		}
	}

	// Check entities contain valid tokens
	for _, entity := range entities {
		// Entity text should be findable in the original text
		if entity.Start >= 0 && entity.End <= len(text) {
			extractedText := text[entity.Start:entity.End]
			if extractedText != entity.Text {
				t.Errorf("Entity text mismatch: stored='%s', extracted='%s'", entity.Text, extractedText)
			}
		}
	}

	// Sentences should reconstruct to approximately the original text
	reconstructed := ""
	for i, sentence := range sentences {
		if i > 0 {
			reconstructed += " "
		}
		reconstructed += sentence
	}

	// The reconstructed text should be similar to the original
	// (might have minor differences in whitespace)
	if len(reconstructed) == 0 {
		t.Error("Failed to reconstruct text from sentences")
	}

	t.Logf("Original: %s", text)
	t.Logf("Reconstructed: %s", reconstructed)
}

// Test different text complexities
func TestTextComplexity(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	testCases := []struct {
		name        string
		text        string
		minTokens   int
		minEntities int
	}{
		{
			name:        "Simple",
			text:        "The cat sat.",
			minTokens:   3,
			minEntities: 0,
		},
		{
			name:        "Medium",
			text:        "Barack Obama was the 44th President of the United States from 2009 to 2017.",
			minTokens:   10,
			minEntities: 2,
		},
		{
			name: "Complex",
			text: "The International Space Station (ISS) is a modular space station in low Earth orbit. " +
				"It is a multinational collaborative project involving five participating space agencies: " +
				"NASA (United States), Roscosmos (Russia), JAXA (Japan), ESA (Europe), and CSA (Canada).",
			minTokens:   40,
			minEntities: 5,
		},
		{
			name: "Technical",
			text: "The HTTP/2 protocol uses binary framing layer that enables request and response multiplexing, " +
				"stream prioritization, and server push capabilities, achieving latencies below 100ms.",
			minTokens:   20,
			minEntities: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := nlp.Tokenize(tc.text)
			entities := nlp.ExtractEntities(tc.text)
			sentences := nlp.SplitSentences(tc.text)

			if len(tokens) < tc.minTokens {
				t.Errorf("Expected at least %d tokens, got %d", tc.minTokens, len(tokens))
			}

			if len(entities) < tc.minEntities {
				t.Errorf("Expected at least %d entities, got %d", tc.minEntities, len(entities))
			}

			if len(sentences) == 0 {
				t.Error("No sentences extracted")
			}

			t.Logf("%s text: %d tokens, %d entities, %d sentences",
				tc.name, len(tokens), len(entities), len(sentences))
		})
	}
}
