package spacy

import (
	"os"
	"testing"
)

// Helper function to check if a model is installed
func isModelInstalled(modelName string) bool {
	// Try to create an NLP instance with the model
	nlp, err := NewNLP(modelName)
	if err != nil {
		return false
	}
	nlp.Close()
	return true
}

// Test different English model sizes
func TestEnglishModels(t *testing.T) {
	models := []struct {
		name        string
		modelID     string
		hasVectors  bool
		vectorSize  int
		description string
	}{
		{
			name:        "Small Model",
			modelID:     "en_core_web_sm",
			hasVectors:  false,
			vectorSize:  0,
			description: "Efficient model without word vectors",
		},
		{
			name:        "Medium Model",
			modelID:     "en_core_web_md",
			hasVectors:  true,
			vectorSize:  300,
			description: "Balanced model with word vectors",
		},
		{
			name:        "Large Model",
			modelID:     "en_core_web_lg",
			hasVectors:  true,
			vectorSize:  300,
			description: "Accurate model with word vectors",
		},
		{
			name:        "Transformer Model",
			modelID:     "en_core_web_trf",
			hasVectors:  true,
			vectorSize:  768,
			description: "Transformer-based model (RoBERTa)",
		},
	}

	testText := "Apple Inc. was founded by Steve Jobs in Cupertino, California."

	for _, model := range models {
		t.Run(model.name, func(t *testing.T) {
			if !isModelInstalled(model.modelID) {
				t.Skipf("Model %s not installed. Install with: python -m spacy download %s",
					model.modelID, model.modelID)
			}

			nlp, err := NewNLP(model.modelID)
			if err != nil {
				t.Fatalf("Failed to load model %s: %v", model.modelID, err)
			}
			defer nlp.Close()

			// Test basic functionality
			tokens := nlp.Tokenize(testText)
			if len(tokens) == 0 {
				t.Errorf("No tokens from model %s", model.modelID)
			}

			entities := nlp.ExtractEntities(testText)
			if len(entities) < 2 { // Should have at least Apple Inc. and Steve Jobs
				t.Errorf("Expected at least 2 entities from %s, got %d", model.modelID, len(entities))
			}

			// Test vectors if available
			vec := nlp.GetVector("apple")
			if model.hasVectors && vec.HasVector {
				if len(vec.Vector) < model.vectorSize {
					t.Errorf("Expected vector size >= %d for %s, got %d",
						model.vectorSize, model.modelID, len(vec.Vector))
				}
			}

			t.Logf("Model %s: %d tokens, %d entities, vector_dim=%d",
				model.modelID, len(tokens), len(entities), len(vec.Vector))
		})
	}
}

// Test multiple language models
func TestMultiLanguageSupport(t *testing.T) {
	languages := []struct {
		name      string
		modelID   string
		testText  string
		minTokens int
		entities  []string
	}{
		{
			name:      "English",
			modelID:   "en_core_web_sm",
			testText:  "London is the capital of the United Kingdom.",
			minTokens: 8,
			entities:  []string{"London", "United Kingdom"},
		},
		{
			name:      "German",
			modelID:   "de_core_news_sm",
			testText:  "Berlin ist die Hauptstadt von Deutschland.",
			minTokens: 6,
			entities:  []string{"Berlin", "Deutschland"},
		},
		{
			name:      "French",
			modelID:   "fr_core_news_sm",
			testText:  "Paris est la capitale de la France.",
			minTokens: 7,
			entities:  []string{"Paris", "France"},
		},
		{
			name:      "Spanish",
			modelID:   "es_core_news_sm",
			testText:  "Madrid es la capital de España.",
			minTokens: 6,
			entities:  []string{"Madrid", "España"},
		},
		{
			name:      "Italian",
			modelID:   "it_core_news_sm",
			testText:  "Roma è la capitale d'Italia.",
			minTokens: 6,
			entities:  []string{"Roma", "Italia"},
		},
		{
			name:      "Portuguese",
			modelID:   "pt_core_news_sm",
			testText:  "Lisboa é a capital de Portugal.",
			minTokens: 6,
			entities:  []string{"Lisboa", "Portugal"},
		},
		{
			name:      "Dutch",
			modelID:   "nl_core_news_sm",
			testText:  "Amsterdam is de hoofdstad van Nederland.",
			minTokens: 6,
			entities:  []string{"Amsterdam", "Nederland"},
		},
		{
			name:      "Chinese",
			modelID:   "zh_core_web_sm",
			testText:  "北京是中国的首都。",
			minTokens: 4,
			entities:  []string{"北京", "中国"},
		},
		{
			name:      "Japanese",
			modelID:   "ja_core_news_sm",
			testText:  "東京は日本の首都です。",
			minTokens: 5,
			entities:  []string{"東京", "日本"},
		},
	}

	for _, lang := range languages {
		t.Run(lang.name, func(t *testing.T) {
			if !isModelInstalled(lang.modelID) {
				t.Skipf("Model %s not installed. Install with: python -m spacy download %s",
					lang.modelID, lang.modelID)
			}

			nlp, err := NewNLP(lang.modelID)
			if err != nil {
				t.Fatalf("Failed to load %s model: %v", lang.name, err)
			}
			defer nlp.Close()

			// Tokenization
			tokens := nlp.Tokenize(lang.testText)
			if len(tokens) < lang.minTokens {
				t.Errorf("%s: Expected at least %d tokens, got %d",
					lang.name, lang.minTokens, len(tokens))
			}

			// Entity recognition
			entities := nlp.ExtractEntities(lang.testText)
			entityTexts := make(map[string]bool)
			for _, e := range entities {
				entityTexts[e.Text] = true
			}

			// Check expected entities
			for _, expectedEntity := range lang.entities {
				if !entityTexts[expectedEntity] {
					t.Logf("%s: Expected entity '%s' not found. Found entities: %v",
						lang.name, expectedEntity, entities)
				}
			}

			// POS tagging
			posMap := nlp.POSTag(lang.testText)
			if len(posMap) == 0 {
				t.Errorf("%s: No POS tags extracted", lang.name)
			}

			// Sentence splitting
			sentences := nlp.SplitSentences(lang.testText)
			if len(sentences) == 0 {
				t.Errorf("%s: No sentences extracted", lang.name)
			}

			t.Logf("%s model (%s): %d tokens, %d entities, %d sentences",
				lang.name, lang.modelID, len(tokens), len(entities), len(sentences))
		})
	}
}

// Test domain-specific models
func TestDomainSpecificModels(t *testing.T) {
	domainModels := []struct {
		name        string
		modelID     string
		domain      string
		testText    string
		minEntities int
	}{
		{
			name:        "Biomedical NER",
			modelID:     "en_ner_bc5cdr_md",
			domain:      "Biomedical",
			testText:    "The patient was treated with aspirin for acute myocardial infarction.",
			minEntities: 2, // aspirin (CHEMICAL), myocardial infarction (DISEASE)
		},
		{
			name:        "Craft Corpus",
			modelID:     "en_ner_craft_md",
			domain:      "Biomedical Literature",
			testText:    "The BRCA1 gene is associated with breast cancer susceptibility.",
			minEntities: 1,
		},
		{
			name:        "OntoNotes",
			modelID:     "xx_ent_wiki_sm",
			domain:      "Multilingual NER",
			testText:    "Microsoft Corporation is located in Seattle, Washington.",
			minEntities: 2,
		},
	}

	for _, model := range domainModels {
		t.Run(model.name, func(t *testing.T) {
			if !isModelInstalled(model.modelID) {
				t.Skipf("Domain model %s not installed. Install with: pip install %s",
					model.modelID, model.modelID)
			}

			nlp, err := NewNLP(model.modelID)
			if err != nil {
				t.Fatalf("Failed to load domain model %s: %v", model.modelID, err)
			}
			defer nlp.Close()

			entities := nlp.ExtractEntities(model.testText)
			if len(entities) < model.minEntities {
				t.Errorf("%s: Expected at least %d entities, got %d",
					model.name, model.minEntities, len(entities))
			}

			t.Logf("%s model (%s): Found %d entities in %s domain",
				model.name, model.modelID, len(entities), model.domain)
			for _, e := range entities {
				t.Logf("  - %s (%s)", e.Text, e.Label)
			}
		})
	}
}

// Test model switching and multiple models in same session
func TestModelSwitching(t *testing.T) {
	models := []string{"en_core_web_sm"}

	// Add other models if available
	if isModelInstalled("en_core_web_md") {
		models = append(models, "en_core_web_md")
	}
	if isModelInstalled("de_core_news_sm") {
		models = append(models, "de_core_news_sm")
	}

	if len(models) < 2 {
		t.Skip("Need at least 2 models installed for switching test")
	}

	testText := "This is a test sentence."

	// Test sequential model loading
	for i, modelName := range models {
		nlp, err := NewNLP(modelName)
		if err != nil {
			t.Fatalf("Failed to load model %d (%s): %v", i, modelName, err)
		}

		tokens := nlp.Tokenize(testText)
		if len(tokens) == 0 {
			t.Errorf("No tokens from model %s", modelName)
		}

		nlp.Close()
		t.Logf("Successfully loaded and used model %s", modelName)
	}

	// Test rapid switching
	for i := 0; i < 3; i++ {
		for _, modelName := range models {
			nlp, err := NewNLP(modelName)
			if err != nil {
				t.Fatalf("Rapid switch failed for %s: %v", modelName, err)
			}
			_ = nlp.Tokenize(testText)
			nlp.Close()
		}
	}
	t.Log("Rapid model switching completed successfully")
}

// Benchmark different model sizes
func BenchmarkModelComparison(b *testing.B) {
	models := []struct {
		name    string
		modelID string
	}{
		{"Small", "en_core_web_sm"},
		{"Medium", "en_core_web_md"},
		{"Large", "en_core_web_lg"},
	}

	testText := "Apple Inc. is an American multinational technology company headquartered in Cupertino, California."

	for _, model := range models {
		if !isModelInstalled(model.modelID) {
			continue
		}

		b.Run(model.name+"_Tokenization", func(b *testing.B) {
			nlp, err := NewNLP(model.modelID)
			if err != nil {
				b.Fatal(err)
			}
			defer nlp.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = nlp.Tokenize(testText)
			}
		})

		b.Run(model.name+"_NER", func(b *testing.B) {
			nlp, err := NewNLP(model.modelID)
			if err != nil {
				b.Fatal(err)
			}
			defer nlp.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = nlp.ExtractEntities(testText)
			}
		})

		b.Run(model.name+"_FullPipeline", func(b *testing.B) {
			nlp, err := NewNLP(model.modelID)
			if err != nil {
				b.Fatal(err)
			}
			defer nlp.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = nlp.Tokenize(testText)
				_ = nlp.ExtractEntities(testText)
				_ = nlp.POSTag(testText)
				_ = nlp.GetNounChunks(testText)
			}
		})
	}
}

// Test error handling with invalid models
func TestInvalidModelHandling(t *testing.T) {
	invalidModels := []string{
		"non_existent_model",
		"invalid_model_name",
		"",
	}

	for _, modelName := range invalidModels {
		nlp, err := NewNLP(modelName)
		if err == nil {
			nlp.Close()
			t.Errorf("Expected error for invalid model '%s', but got none", modelName)
		}
	}
}

// Test concurrent access with different models
func TestConcurrentMultiModel(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent multi-model test in short mode")
	}

	// For this test, we'll use the same model but pretend they're different
	// In production, you could use different language models
	modelName := "en_core_web_sm"

	if !isModelInstalled(modelName) {
		t.Skip("Model not installed")
	}

	// Create a single shared instance
	nlp, err := NewNLP(modelName)
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	texts := []string{
		"The quick brown fox jumps over the lazy dog.",
		"Apple Inc. was founded by Steve Jobs.",
		"Machine learning is a subset of artificial intelligence.",
	}

	done := make(chan bool, len(texts))

	for _, text := range texts {
		go func(t string) {
			defer func() { done <- true }()

			// Perform various operations
			tokens := nlp.Tokenize(t)
			_ = nlp.ExtractEntities(t)
			_ = nlp.GetNounChunks(t)
			_ = nlp.POSTag(t)

			if len(tokens) == 0 {
				panic("No tokens returned")
			}
		}(text)
	}

	// Wait for all goroutines
	for i := 0; i < len(texts); i++ {
		<-done
	}

	t.Log("Concurrent multi-model access completed successfully")
}

// Helper function to download missing models
func TestModelAvailability(t *testing.T) {
	// Check which models are available
	commonModels := []string{
		"en_core_web_sm",
		"en_core_web_md",
		"en_core_web_lg",
		"de_core_news_sm",
		"fr_core_news_sm",
		"es_core_news_sm",
	}

	available := []string{}
	missing := []string{}

	for _, model := range commonModels {
		if isModelInstalled(model) {
			available = append(available, model)
		} else {
			missing = append(missing, model)
		}
	}

	t.Logf("Available models: %v", available)
	if len(missing) > 0 {
		t.Logf("Missing models (install with 'python -m spacy download MODEL_NAME'):")
		for _, m := range missing {
			t.Logf("  - %s", m)
		}
	}

	// Ensure at least the basic model is available
	if !isModelInstalled("en_core_web_sm") {
		t.Error("Basic model en_core_web_sm is not installed. Please install it with: python -m spacy download en_core_web_sm")
	}
}

// Test with environment variable for model selection
func TestModelFromEnvironment(t *testing.T) {
	// Check if SPACY_MODEL env var is set
	modelName := os.Getenv("SPACY_MODEL")
	if modelName == "" {
		modelName = "en_core_web_sm" // default
		t.Logf("SPACY_MODEL not set, using default: %s", modelName)
	} else {
		t.Logf("Using model from SPACY_MODEL env: %s", modelName)
	}

	if !isModelInstalled(modelName) {
		t.Skipf("Model %s from environment not installed", modelName)
	}

	nlp, err := NewNLP(modelName)
	if err != nil {
		t.Fatalf("Failed to load model from env: %v", err)
	}
	defer nlp.Close()

	text := "This is a test sentence for the model specified in environment."
	tokens := nlp.Tokenize(text)

	if len(tokens) == 0 {
		t.Error("No tokens returned from environment-specified model")
	}

	t.Logf("Environment model %s: %d tokens", modelName, len(tokens))
}
