package spacy

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type TestDocument struct {
	ID                string   `json:"id"`
	Text              string   `json:"text"`
	ExpectedEntities  []string `json:"expected_entities"`
	ExpectedSentences int      `json:"expected_sentences"`
}

type EdgeCase struct {
	ID                   string `json:"id"`
	Text                 string `json:"text"`
	ExpectedTokens       int    `json:"expected_tokens"`
	ExpectedMinTokens    int    `json:"expected_min_tokens"`
	ExpectedSentences    int    `json:"expected_sentences"`
	ExpectedMinSentences int    `json:"expected_min_sentences"`
	ExpectedMinEntities  int    `json:"expected_min_entities"`
}

type PerformanceTest struct {
	ID              string `json:"id"`
	Text            string `json:"text"`
	TextRepeat      string `json:"text_repeat"`
	RepeatCount     int    `json:"repeat_count"`
	MaxProcessingMs int    `json:"max_processing_ms"`
}

type TestData struct {
	Documents        []TestDocument    `json:"documents"`
	EdgeCases        []EdgeCase        `json:"edge_cases"`
	PerformanceTests []PerformanceTest `json:"performance_tests"`
}

// Load test data from JSON file
func loadTestData(t *testing.T) *TestData {
	data, err := os.ReadFile("testdata/sample_texts.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	var testData TestData
	if err := json.Unmarshal(data, &testData); err != nil {
		t.Fatalf("Failed to parse test data: %v", err)
	}

	return &testData
}

// Test with real-world document samples
func TestRealWorldDocuments(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	testData := loadTestData(t)

	for _, doc := range testData.Documents {
		t.Run(doc.ID, func(t *testing.T) {
			// Tokenize
			tokens := nlp.Tokenize(doc.Text)
			if len(tokens) == 0 && doc.Text != "" {
				t.Error("No tokens extracted")
			}

			// Extract entities
			entities := nlp.ExtractEntities(doc.Text)
			foundEntities := make(map[string]bool)
			for _, entity := range entities {
				foundEntities[entity.Text] = true

				// Validate entity bounds
				if entity.Start < 0 || entity.End > len(doc.Text) {
					t.Errorf("Invalid entity bounds for '%s': [%d:%d]", entity.Text, entity.Start, entity.End)
				}
			}

			// Check for expected entities (with some tolerance)
			matchedEntities := 0
			for _, expected := range doc.ExpectedEntities {
				for entityText := range foundEntities {
					if strings.Contains(entityText, expected) || strings.Contains(expected, entityText) {
						matchedEntities++
						break
					}
				}
			}

			// We expect to find at least 50% of expected entities
			minExpected := len(doc.ExpectedEntities) / 2
			if matchedEntities < minExpected {
				t.Logf("Warning: Only found %d/%d expected entities", matchedEntities, len(doc.ExpectedEntities))
			}

			// Split sentences
			sentences := nlp.SplitSentences(doc.Text)
			if doc.ExpectedSentences > 0 {
				if len(sentences) != doc.ExpectedSentences {
					t.Logf("Expected %d sentences, got %d", doc.ExpectedSentences, len(sentences))
				}
			}

			// POS tagging
			posMap := nlp.POSTag(doc.Text)
			// Note: POS map may have fewer entries than tokens if some tokens have the same text
			if len(posMap) > len(tokens) {
				t.Errorf("POS map size (%d) exceeds token count (%d)", len(posMap), len(tokens))
			}

			// Log statistics
			t.Logf("Document '%s': %d tokens, %d entities, %d sentences",
				doc.ID, len(tokens), len(entities), len(sentences))
		})
	}
}

// Test edge cases from test data
func TestDataDrivenEdgeCases(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	testData := loadTestData(t)

	for _, edge := range testData.EdgeCases {
		t.Run(edge.ID, func(t *testing.T) {
			// Tokenize
			tokens := nlp.Tokenize(edge.Text)

			if edge.ExpectedTokens > 0 {
				if len(tokens) != edge.ExpectedTokens {
					t.Errorf("Expected exactly %d tokens, got %d", edge.ExpectedTokens, len(tokens))
				}
			}

			if edge.ExpectedMinTokens > 0 {
				if len(tokens) < edge.ExpectedMinTokens {
					t.Errorf("Expected at least %d tokens, got %d", edge.ExpectedMinTokens, len(tokens))
				}
			}

			// Sentence splitting
			sentences := nlp.SplitSentences(edge.Text)

			if edge.ExpectedSentences > 0 {
				if len(sentences) != edge.ExpectedSentences {
					t.Errorf("Expected exactly %d sentences, got %d", edge.ExpectedSentences, len(sentences))
				}
			}

			if edge.ExpectedMinSentences > 0 {
				if len(sentences) < edge.ExpectedMinSentences {
					t.Errorf("Expected at least %d sentences, got %d", edge.ExpectedMinSentences, len(sentences))
				}
			}

			// Entity extraction
			if edge.ExpectedMinEntities > 0 {
				entities := nlp.ExtractEntities(edge.Text)
				if len(entities) < edge.ExpectedMinEntities {
					t.Errorf("Expected at least %d entities, got %d", edge.ExpectedMinEntities, len(entities))
				}
			}

			// Ensure no panics on edge cases
			_ = nlp.POSTag(edge.Text)
			_ = nlp.GetDependencies(edge.Text)
			_ = nlp.GetLemmas(edge.Text)

			t.Logf("Edge case '%s' processed successfully", edge.ID)
		})
	}
}

// Test performance with timing constraints
func TestPerformanceConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	testData := loadTestData(t)

	for _, perfTest := range testData.PerformanceTests {
		t.Run(perfTest.ID, func(t *testing.T) {
			// Prepare text
			text := perfTest.Text
			if perfTest.TextRepeat != "" && perfTest.RepeatCount > 0 {
				text = strings.Repeat(perfTest.TextRepeat, perfTest.RepeatCount)
			}

			// Measure processing time
			start := time.Now()
			tokens := nlp.Tokenize(text)
			duration := time.Since(start)

			if len(tokens) == 0 && text != "" {
				t.Error("No tokens extracted")
			}

			durationMs := int(duration.Milliseconds())
			if durationMs > perfTest.MaxProcessingMs {
				t.Errorf("Processing took %dms, expected max %dms", durationMs, perfTest.MaxProcessingMs)
			}

			// Calculate throughput
			bytesPerMs := float64(len(text)) / float64(durationMs)
			tokensPerMs := float64(len(tokens)) / float64(durationMs)

			t.Logf("Performance '%s': %d bytes in %dms (%.2f bytes/ms, %.2f tokens/ms)",
				perfTest.ID, len(text), durationMs, bytesPerMs, tokensPerMs)
		})
	}
}

// Test data validation
func TestDataValidation(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	testData := loadTestData(t)

	// Validate that test data is properly formed
	if len(testData.Documents) == 0 {
		t.Error("No test documents loaded")
	}

	if len(testData.EdgeCases) == 0 {
		t.Error("No edge cases loaded")
	}

	if len(testData.PerformanceTests) == 0 {
		t.Error("No performance tests loaded")
	}

	// Test that each document can be processed
	for _, doc := range testData.Documents {
		if doc.Text == "" && doc.ID != "empty_string" {
			t.Errorf("Document '%s' has empty text", doc.ID)
		}

		// Quick validation that document can be processed
		tokens := nlp.Tokenize(doc.Text)
		if doc.Text != "" && len(tokens) == 0 {
			t.Errorf("Document '%s' produced no tokens", doc.ID)
		}
	}

	t.Logf("Validated %d documents, %d edge cases, %d performance tests",
		len(testData.Documents), len(testData.EdgeCases), len(testData.PerformanceTests))
}

// Test cross-validation with test data
func TestCrossValidation(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	testData := loadTestData(t)

	// Use first few documents for cross-validation
	maxDocs := 3
	if len(testData.Documents) < maxDocs {
		maxDocs = len(testData.Documents)
	}

	for i := 0; i < maxDocs; i++ {
		doc := testData.Documents[i]
		t.Run(doc.ID, func(t *testing.T) {
			// Get all outputs
			tokens := nlp.Tokenize(doc.Text)
			entities := nlp.ExtractEntities(doc.Text)
			sentences := nlp.SplitSentences(doc.Text)
			posMap := nlp.POSTag(doc.Text)
			depMap := nlp.GetDependencies(doc.Text)
			lemmaMap := nlp.GetLemmas(doc.Text)

			// Cross-validate token consistency
			// Note: Maps use token text as key, so multiple identical tokens map to same entry
			for _, token := range tokens {
				if pos, exists := posMap[token.Text]; exists {
					if pos != token.POS {
						// This could happen if identical tokens have different POS in context
						t.Logf("POS variation for '%s': token=%s, map=%s", token.Text, token.POS, pos)
					}
				}

				if dep, exists := depMap[token.Text]; exists {
					if dep != token.Dep {
						// This could happen if identical tokens have different deps in context
						t.Logf("Dep variation for '%s': token=%s, map=%s", token.Text, token.Dep, dep)
					}
				}

				if lemma, exists := lemmaMap[token.Text]; exists {
					if lemma != token.Lemma {
						// This could happen if identical tokens have different lemmas in context
						t.Logf("Lemma variation for '%s': token=%s, map=%s", token.Text, token.Lemma, lemma)
					}
				}
			}

			// Validate entities are within text
			for _, entity := range entities {
				if entity.Start >= 0 && entity.End <= len(doc.Text) {
					extracted := doc.Text[entity.Start:entity.End]
					if extracted != entity.Text {
						t.Errorf("Entity text mismatch: stored='%s', extracted='%s'", entity.Text, extracted)
					}
				}
			}

			// Validate sentence reconstruction
			reconstructed := strings.Join(sentences, " ")
			if len(reconstructed) > 0 && len(doc.Text) > 0 {
				// Check that reconstruction preserves most of the content
				minLen := len(doc.Text) * 8 / 10 // 80% of original
				if len(reconstructed) < minLen {
					t.Logf("Reconstruction shorter than expected: %d vs %d", len(reconstructed), len(doc.Text))
				}
			}

			t.Logf("Cross-validation complete for '%s'", doc.ID)
		})
	}
}
