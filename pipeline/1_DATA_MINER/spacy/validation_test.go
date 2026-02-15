package spacy

import (
	"testing"
	"time"
)

// TestValidationSuite runs a comprehensive validation of all core functions
func TestValidationSuite(t *testing.T) {
	// Initialize once
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}
	defer nlp.Close()

	// Test texts
	tests := []struct {
		name string
		text string
		test func(t *testing.T)
	}{
		{
			name: "Simple tokenization",
			text: "The quick brown fox jumps over the lazy dog.",
			test: func(t *testing.T) {
				tokens := nlp.Tokenize("The quick brown fox jumps over the lazy dog.")
				if len(tokens) < 9 {
					t.Errorf("Expected at least 9 tokens, got %d", len(tokens))
				}
				for _, token := range tokens {
					if token.Text == "" {
						t.Error("Empty token text")
					}
					if token.POS == "" {
						t.Error("Empty POS tag")
					}
				}
			},
		},
		{
			name: "Named Entity Recognition",
			text: "Apple Inc. was founded by Steve Jobs in California.",
			test: func(t *testing.T) {
				entities := nlp.ExtractEntities("Apple Inc. was founded by Steve Jobs in California.")
				if len(entities) < 2 {
					t.Errorf("Expected at least 2 entities, got %d", len(entities))
				}
				foundCompany := false
				foundPerson := false
				for _, entity := range entities {
					if entity.Label == "ORG" {
						foundCompany = true
					}
					if entity.Label == "PERSON" {
						foundPerson = true
					}
				}
				if !foundCompany {
					t.Error("Failed to find company entity")
				}
				if !foundPerson {
					t.Error("Failed to find person entity")
				}
			},
		},
		{
			name: "Sentence Splitting",
			text: "First sentence. Second sentence! Third sentence?",
			test: func(t *testing.T) {
				sentences := nlp.SplitSentences("First sentence. Second sentence! Third sentence?")
				if len(sentences) != 3 {
					t.Errorf("Expected 3 sentences, got %d", len(sentences))
				}
			},
		},
		{
			name: "POS Tagging",
			text: "The cat sleeps peacefully",
			test: func(t *testing.T) {
				posMap := nlp.POSTag("The cat sleeps peacefully")
				if len(posMap) == 0 {
					t.Error("No POS tags returned")
				}
				if pos, exists := posMap["cat"]; exists {
					if pos != "NOUN" {
						t.Logf("Warning: 'cat' tagged as %s instead of NOUN", pos)
					}
				}
			},
		},
		{
			name: "Dependency Parsing",
			text: "The dog chased the cat",
			test: func(t *testing.T) {
				depMap := nlp.GetDependencies("The dog chased the cat")
				if len(depMap) == 0 {
					t.Error("No dependencies returned")
				}
				for word, dep := range depMap {
					if dep == "" {
						t.Errorf("Empty dependency for word '%s'", word)
					}
				}
			},
		},
		{
			name: "Lemmatization",
			text: "running runs ran",
			test: func(t *testing.T) {
				lemmaMap := nlp.GetLemmas("running runs ran")
				if len(lemmaMap) == 0 {
					t.Error("No lemmas returned")
				}
				// Check if lemmatization works
				if lemma, exists := lemmaMap["running"]; exists {
					if lemma != "run" && lemma != "running" {
						t.Logf("Unexpected lemma for 'running': %s", lemma)
					}
				}
			},
		},
	}

	// Run all tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			tt.test(t)
			duration := time.Since(start)
			if duration > 5*time.Second {
				t.Errorf("Test took too long: %v", duration)
			}
			t.Logf("Test completed in %v", duration)
		})
	}
}

// TestPerformanceBasic tests basic performance requirements
func TestPerformanceBasic(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}
	defer nlp.Close()

	// Test that operations complete within reasonable time
	text := "This is a simple test sentence for performance validation."

	// Tokenization should be fast
	start := time.Now()
	tokens := nlp.Tokenize(text)
	duration := time.Since(start)
	if duration > 100*time.Millisecond {
		t.Errorf("Tokenization took too long: %v", duration)
	}
	if len(tokens) == 0 {
		t.Error("No tokens returned")
	}
	t.Logf("Tokenization: %d tokens in %v", len(tokens), duration)

	// NER should complete quickly
	start = time.Now()
	entities := nlp.ExtractEntities(text)
	duration = time.Since(start)
	if duration > 100*time.Millisecond {
		t.Errorf("NER took too long: %v", duration)
	}
	t.Logf("NER: %d entities in %v", len(entities), duration)

	// Sentence splitting should be fast
	start = time.Now()
	sentences := nlp.SplitSentences(text)
	duration = time.Since(start)
	if duration > 100*time.Millisecond {
		t.Errorf("Sentence splitting took too long: %v", duration)
	}
	t.Logf("Sentences: %d sentences in %v", len(sentences), duration)
}

// TestStressSequential tests sequential processing without concurrency
func TestStressSequential(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}
	defer nlp.Close()

	texts := []string{
		"First test sentence.",
		"Second test with more words.",
		"Apple Inc. is a technology company.",
		"The United Nations is in New York.",
		"Machine learning transforms technology.",
	}

	// Process many times sequentially with smaller iteration count
	iterations := 20
	start := time.Now()

	for i := 0; i < iterations; i++ {
		text := texts[i%len(texts)]

		tokens := nlp.Tokenize(text)
		if len(tokens) == 0 {
			t.Errorf("Iteration %d: No tokens", i)
		}

		_ = nlp.ExtractEntities(text)
		_ = nlp.SplitSentences(text)
		_ = nlp.POSTag(text)

		// Small delay to avoid overwhelming Python
		time.Sleep(10 * time.Millisecond)

		// Check for timeout
		if time.Since(start) > 30*time.Second {
			t.Fatal("Stress test timeout")
		}
	}

	duration := time.Since(start)
	t.Logf("Processed %d iterations in %v", iterations, duration)

	avgTime := duration / time.Duration(iterations)
	if avgTime > 500*time.Millisecond {
		t.Errorf("Average processing time too high: %v", avgTime)
	}
}
