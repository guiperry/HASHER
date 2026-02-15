package spacy

import (
	"math"
	"testing"
)

// Test noun chunks extraction
func TestNounChunks(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name      string
		text      string
		minChunks int
	}{
		{
			name:      "Simple sentence",
			text:      "The quick brown fox jumps over the lazy dog.",
			minChunks: 2, // At least "the quick brown fox" and "the lazy dog"
		},
		{
			name:      "Complex sentence",
			text:      "Apple Inc., a multinational technology company, develops consumer electronics.",
			minChunks: 3,
		},
		{
			name:      "Technical text",
			text:      "Machine learning algorithms process large datasets to identify patterns.",
			minChunks: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := nlp.GetNounChunks(tt.text)

			if len(chunks) < tt.minChunks {
				t.Errorf("Expected at least %d chunks, got %d", tt.minChunks, len(chunks))
			}

			// Validate chunk properties
			for _, chunk := range chunks {
				if chunk.Text == "" {
					t.Error("Empty chunk text")
				}
				if chunk.RootText == "" {
					t.Error("Empty chunk root text")
				}
				if chunk.Start < 0 || chunk.End > len(tt.text) {
					t.Errorf("Invalid chunk bounds: [%d:%d] for text length %d",
						chunk.Start, chunk.End, len(tt.text))
				}
			}

			t.Logf("Found %d noun chunks: %v", len(chunks), chunks)
		})
	}
}

// Test word vectors and similarity
func TestVectorsAndSimilarity(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	// Test vector extraction
	texts := []string{
		"king",
		"queen",
		"cat",
		"dog",
	}

	vectors := make([]VectorData, len(texts))
	for i, text := range texts {
		vectors[i] = nlp.GetVector(text)
		if !vectors[i].HasVector {
			t.Logf("Warning: No vector for '%s' (model may not have word vectors)", text)
			continue
		}
		if len(vectors[i].Vector) == 0 {
			t.Errorf("Empty vector for '%s'", text)
		}
		t.Logf("Vector for '%s': dimension=%d", text, len(vectors[i].Vector))
	}

	// Test similarity
	similarityTests := []struct {
		text1  string
		text2  string
		minSim float64
	}{
		{"cat", "dog", 0.5},        // Animals should be somewhat similar
		{"king", "queen", 0.5},     // Royal terms should be similar
		{"cat", "automobile", 0.2}, // Should have lower similarity
	}

	for _, tt := range similarityTests {
		sim := nlp.Similarity(tt.text1, tt.text2)
		// Note: similarity might be 0 if model doesn't have vectors
		if sim > 0 {
			t.Logf("Similarity between '%s' and '%s': %.3f", tt.text1, tt.text2, sim)
			if sim < 0 || sim > 1 {
				t.Errorf("Similarity out of bounds [0,1]: %.3f", sim)
			}
		} else {
			t.Logf("No similarity computed (model may lack vectors)")
		}
	}
}

// Test morphological analysis
func TestMorphology(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	texts := []string{
		"The cats are sleeping.",
		"She walked quickly to the store.",
		"They have been working all day.",
	}

	for _, text := range texts {
		features := nlp.GetMorphology(text)

		if len(features) == 0 {
			t.Logf("No morphological features for: %s", text)
			continue
		}

		t.Logf("Morphological features for '%s':", text)
		for _, feat := range features {
			if feat.Key == "" && feat.Value == "" {
				continue // Skip empty features
			}
			t.Logf("  %s: %s", feat.Key, feat.Value)
		}
	}
}

// Test advanced document processing
func TestAdvancedDocumentProcessing(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "Apple Inc. was founded by Steve Jobs in 1976. The company creates innovative products."

	// Get all analyses
	tokens := nlp.Tokenize(text)
	entities := nlp.ExtractEntities(text)
	chunks := nlp.GetNounChunks(text)
	sentences := nlp.SplitSentences(text)

	// Verify we got comprehensive analysis
	if len(tokens) == 0 {
		t.Error("No tokens extracted")
	}
	if len(entities) < 2 { // Should have at least Apple Inc. and Steve Jobs
		t.Errorf("Expected at least 2 entities, got %d", len(entities))
	}
	if len(chunks) < 3 { // Should have multiple noun phrases
		t.Errorf("Expected at least 3 noun chunks, got %d", len(chunks))
	}
	if len(sentences) != 2 {
		t.Errorf("Expected 2 sentences, got %d", len(sentences))
	}

	// Check vector operations
	vec := nlp.GetVector(text)
	if vec.HasVector {
		if len(vec.Vector) == 0 {
			t.Error("Has vector flag is true but vector is empty")
		}

		// Check vector has reasonable values
		var sum float64
		for _, v := range vec.Vector {
			sum += v * v
		}
		norm := math.Sqrt(sum)
		t.Logf("Document vector: dimension=%d, L2 norm=%.3f", len(vec.Vector), norm)
	}

	t.Logf("Comprehensive analysis complete: %d tokens, %d entities, %d chunks, %d sentences",
		len(tokens), len(entities), len(chunks), len(sentences))
}

// Benchmark new functions
func BenchmarkNounChunks(b *testing.B) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		b.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "The quick brown fox jumps over the lazy dog near the old oak tree."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunks := nlp.GetNounChunks(text)
		if len(chunks) == 0 {
			b.Fatal("No chunks returned")
		}
	}
}

func BenchmarkSimilarity(b *testing.B) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		b.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text1 := "cat"
	text2 := "dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nlp.Similarity(text1, text2)
	}
}
