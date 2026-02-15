package spacy

import (
	"testing"
)

// Test small vs medium model differences
func TestSmallVsMediumModels(t *testing.T) {
	tests := []struct {
		modelName string
		modelID   string
	}{
		{"Small Model", "en_core_web_sm"},
		{"Medium Model", "en_core_web_md"},
	}

	text := "Apple and Microsoft are competing technology companies."

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			if !isModelInstalled(tt.modelID) {
				t.Skipf("Model %s not installed", tt.modelID)
			}

			nlp, err := NewNLP(tt.modelID)
			if err != nil {
				t.Fatalf("Failed to load %s: %v", tt.modelID, err)
			}
			defer nlp.Close()

			// Test basic tokenization
			tokens := nlp.Tokenize(text)
			t.Logf("%s - Tokens: %d", tt.modelName, len(tokens))

			// Test entities
			entities := nlp.ExtractEntities(text)
			t.Logf("%s - Entities found:", tt.modelName)
			for _, e := range entities {
				t.Logf("  - %s (%s)", e.Text, e.Label)
			}

			// Test vectors
			vec := nlp.GetVector(text)
			if vec.HasVector {
				t.Logf("%s - Vector dimension: %d", tt.modelName, len(vec.Vector))
			} else {
				t.Logf("%s - No word vectors", tt.modelName)
			}

			// Test similarity (only meaningful with vectors)
			sim := nlp.Similarity("cat", "dog")
			t.Logf("%s - Similarity(cat, dog): %.3f", tt.modelName, sim)
		})
	}
}

// Test language-specific features
func TestLanguageSpecificFeatures(t *testing.T) {
	languages := []struct {
		name    string
		model   string
		text    string
		feature string
	}{
		{
			name:    "German Compound Words",
			model:   "de_core_news_sm",
			text:    "Die Donaudampfschifffahrtsgesellschaft war eine österreichische Firma.",
			feature: "compound noun handling",
		},
		{
			name:    "French Gender Agreement",
			model:   "fr_core_news_sm",
			text:    "La belle maison est grande et moderne.",
			feature: "gender agreement",
		},
		{
			name:    "Spanish Verb Conjugation",
			model:   "es_core_news_sm",
			text:    "Yo hablo, tú hablas, él habla español.",
			feature: "verb conjugation",
		},
	}

	for _, lang := range languages {
		t.Run(lang.name, func(t *testing.T) {
			if !isModelInstalled(lang.model) {
				t.Skipf("Model %s not installed", lang.model)
			}

			nlp, err := NewNLP(lang.model)
			if err != nil {
				t.Fatalf("Failed to load %s: %v", lang.model, err)
			}
			defer nlp.Close()

			// Tokenize and get morphological features
			tokens := nlp.Tokenize(lang.text)
			morph := nlp.GetMorphology(lang.text)

			t.Logf("%s - Testing %s", lang.name, lang.feature)
			t.Logf("  Tokens: %d", len(tokens))
			t.Logf("  Sample tokens:")
			for i, token := range tokens {
				if i < 5 { // Show first 5 tokens
					t.Logf("    - '%s' (POS: %s, Lemma: %s)", token.Text, token.POS, token.Lemma)
				}
			}

			if len(morph) > 0 {
				t.Logf("  Morphological features found: %d", len(morph))
			}
		})
	}
}

// Test cross-lingual similarity (if applicable)
func TestCrossLingualProcessing(t *testing.T) {
	// Test processing the same concept in different languages
	concepts := map[string]string{
		"en_core_web_sm":  "The capital of Germany is Berlin.",
		"de_core_news_sm": "Die Hauptstadt von Deutschland ist Berlin.",
		"fr_core_news_sm": "La capitale de l'Allemagne est Berlin.",
		"es_core_news_sm": "La capital de Alemania es Berlín.",
	}

	results := make(map[string][]Entity)

	for model, text := range concepts {
		if !isModelInstalled(model) {
			continue
		}

		nlp, err := NewNLP(model)
		if err != nil {
			t.Errorf("Failed to load %s: %v", model, err)
			continue
		}

		entities := nlp.ExtractEntities(text)
		results[model] = entities

		nlp.Close()
	}

	// Compare entity extraction across languages
	t.Log("Cross-lingual entity extraction comparison:")
	for model, entities := range results {
		t.Logf("  %s: Found %d entities", model, len(entities))
		for _, e := range entities {
			t.Logf("    - %s (%s)", e.Text, e.Label)
		}
	}
}

// Performance comparison across models
func TestModelPerformanceComparison(t *testing.T) {
	models := []string{
		"en_core_web_sm",
		"en_core_web_md",
		"de_core_news_sm",
		"fr_core_news_sm",
		"es_core_news_sm",
	}

	text := "This is a test sentence for performance comparison."

	t.Log("Model Performance Comparison:")
	t.Log("Model | Tokens | Entities | POS Tags | Chunks | Features")
	t.Log("------|--------|----------|----------|--------|----------")

	for _, model := range models {
		if !isModelInstalled(model) {
			continue
		}

		nlp, err := NewNLP(model)
		if err != nil {
			continue
		}

		tokens := nlp.Tokenize(text)
		entities := nlp.ExtractEntities(text)
		pos := nlp.POSTag(text)
		chunks := nlp.GetNounChunks(text)
		morph := nlp.GetMorphology(text)

		t.Logf("%-16s | %6d | %8d | %8d | %6d | %8d",
			model, len(tokens), len(entities), len(pos), len(chunks), len(morph))

		nlp.Close()
	}
}

// Test advanced features with different models
func TestAdvancedFeaturesAcrossModels(t *testing.T) {
	text := "The United Nations headquarters is located in New York City."

	models := []string{"en_core_web_sm", "en_core_web_md"}

	for _, model := range models {
		if !isModelInstalled(model) {
			continue
		}

		t.Run(model, func(t *testing.T) {
			nlp, err := NewNLP(model)
			if err != nil {
				t.Fatal(err)
			}
			defer nlp.Close()

			// Get all features
			tokens := nlp.Tokenize(text)
			entities := nlp.ExtractEntities(text)
			chunks := nlp.GetNounChunks(text)
			vec := nlp.GetVector(text)

			t.Logf("Model: %s", model)
			t.Logf("  Tokens: %d", len(tokens))
			t.Logf("  Entities: %d", len(entities))
			for _, e := range entities {
				t.Logf("    - %s: %s", e.Label, e.Text)
			}
			t.Logf("  Noun chunks: %d", len(chunks))
			for _, c := range chunks {
				t.Logf("    - %s (root: %s)", c.Text, c.RootText)
			}

			if vec.HasVector {
				t.Logf("  Vector size: %d", len(vec.Vector))

				// Test similarity with medium model
				if model == "en_core_web_md" {
					sim1 := nlp.Similarity("United Nations", "UN")
					sim2 := nlp.Similarity("New York", "NYC")
					t.Logf("  Similarity tests:")
					t.Logf("    'United Nations' vs 'UN': %.3f", sim1)
					t.Logf("    'New York' vs 'NYC': %.3f", sim2)
				}
			} else {
				t.Log("  No word vectors available")
			}
		})
	}
}
