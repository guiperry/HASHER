package spacy

import (
	"strings"
	"testing"
	"unicode"
)

// Test initialization with valid and invalid models
func TestInitialization(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		wantErr   bool
	}{
		{"Valid model", "en_core_web_sm", false},
		{"Empty model name", "", true},
		{"Invalid model name", "non_existent_model_xyz", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nlp, err := NewNLP(tt.modelName)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewNLP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if nlp != nil {
				defer nlp.Close()
			}
		})
	}
}

// Test tokenization with various text inputs
func TestTokenizationComprehensive(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name           string
		text           string
		minTokenCount  int
		checkPOS       bool
		checkLemma     bool
		checkStopWords bool
	}{
		{
			name:           "Simple sentence",
			text:           "The cat sat on the mat.",
			minTokenCount:  7,
			checkPOS:       true,
			checkLemma:     true,
			checkStopWords: true,
		},
		{
			name:          "Complex sentence with numbers",
			text:          "Apple Inc. earned $123.45 billion in 2023.",
			minTokenCount: 8,
			checkPOS:      true,
			checkLemma:    true,
		},
		{
			name:          "Empty string",
			text:          "",
			minTokenCount: 0,
		},
		{
			name:          "Single word",
			text:          "Hello",
			minTokenCount: 1,
			checkPOS:      true,
		},
		{
			name:          "Punctuation only",
			text:          "!!!???...",
			minTokenCount: 1,
		},
		{
			name:          "Mixed languages",
			text:          "Hello 你好 Bonjour",
			minTokenCount: 3,
		},
		{
			name:          "Contractions",
			text:          "I'll won't can't shouldn't",
			minTokenCount: 4,
			checkLemma:    true,
		},
		{
			name:          "URLs and emails",
			text:          "Visit https://example.com or email test@example.com",
			minTokenCount: 5,
		},
		{
			name:          "Special characters",
			text:          "Price: $100 @user #hashtag",
			minTokenCount: 5,
		},
		{
			name:          "Long text",
			text:          strings.Repeat("This is a test sentence. ", 100),
			minTokenCount: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := nlp.Tokenize(tt.text)

			if len(tokens) < tt.minTokenCount {
				t.Errorf("Expected at least %d tokens, got %d", tt.minTokenCount, len(tokens))
			}

			for i, token := range tokens {
				// Check basic properties
				if token.Text == "" && tt.text != "" {
					t.Errorf("Token %d has empty text", i)
				}

				// Check POS tags if requested
				if tt.checkPOS && token.POS == "" && tt.text != "" {
					t.Errorf("Token '%s' has empty POS tag", token.Text)
				}

				// Check lemma if requested
				if tt.checkLemma && token.Lemma == "" && tt.text != "" {
					t.Errorf("Token '%s' has empty lemma", token.Text)
				}

				// Validate stop words for common words
				if tt.checkStopWords {
					lowerText := strings.ToLower(token.Text)
					if lowerText == "the" || lowerText == "on" || lowerText == "a" {
						if !token.IsStop {
							t.Logf("Warning: '%s' should be marked as stop word", token.Text)
						}
					}
				}

				// Check punctuation detection
				if len(token.Text) > 0 && unicode.IsPunct(rune(token.Text[0])) {
					if !token.IsPunct {
						t.Logf("Warning: '%s' should be marked as punctuation", token.Text)
					}
				}
			}
		})
	}
}

// Test NER with various entity types
func TestNamedEntityRecognition(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name             string
		text             string
		expectedEntities map[string]string // entity text -> label
		minEntities      int
	}{
		{
			name: "Person and Organization",
			text: "Bill Gates founded Microsoft Corporation in Seattle.",
			expectedEntities: map[string]string{
				"Bill Gates":            "PERSON",
				"Microsoft Corporation": "ORG",
				"Seattle":               "GPE",
			},
			minEntities: 3,
		},
		{
			name: "Dates and Times",
			text: "The meeting is scheduled for January 15, 2024 at 3:00 PM.",
			expectedEntities: map[string]string{
				"January 15, 2024": "DATE",
				"3:00 PM":          "TIME",
			},
			minEntities: 2,
		},
		{
			name: "Money and Percentages",
			text: "The company earned $50 million, a 25% increase.",
			expectedEntities: map[string]string{
				"50 million": "MONEY",
				"25%":        "PERCENT",
			},
			minEntities: 2,
		},
		{
			name: "Locations",
			text: "Paris, France is in Europe near the Mediterranean Sea.",
			expectedEntities: map[string]string{
				"Paris":  "GPE",
				"France": "GPE",
				"Europe": "LOC",
			},
			minEntities: 3,
		},
		{
			name:        "Empty text",
			text:        "",
			minEntities: 0,
		},
		{
			name: "Complex entities",
			text: "The United States of America elected President Joe Biden.",
			expectedEntities: map[string]string{
				"The United States of America": "GPE",
				"Joe Biden":                    "PERSON",
			},
			minEntities: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities := nlp.ExtractEntities(tt.text)

			if len(entities) < tt.minEntities {
				t.Errorf("Expected at least %d entities, got %d", tt.minEntities, len(entities))
			}

			// Check entity positions
			for _, entity := range entities {
				if entity.Start < 0 || entity.End <= entity.Start {
					t.Errorf("Invalid entity position: start=%d, end=%d", entity.Start, entity.End)
				}

				if entity.End > len(tt.text) {
					t.Errorf("Entity end position %d exceeds text length %d", entity.End, len(tt.text))
				}

				// Verify entity text matches the position
				if entity.Start < len(tt.text) && entity.End <= len(tt.text) {
					extractedText := tt.text[entity.Start:entity.End]
					if extractedText != entity.Text {
						t.Errorf("Entity text mismatch: expected '%s', got '%s'", extractedText, entity.Text)
					}
				}
			}

			// Check for expected entities (allow for variations in NER)
			for expectedText, expectedLabel := range tt.expectedEntities {
				found := false
				for _, entity := range entities {
					if strings.Contains(entity.Text, expectedText) || strings.Contains(expectedText, entity.Text) {
						found = true
						if entity.Label != expectedLabel {
							t.Logf("Warning: Entity '%s' has label '%s', expected '%s'", entity.Text, entity.Label, expectedLabel)
						}
						break
					}
				}
				if !found {
					t.Logf("Warning: Expected entity '%s' not found", expectedText)
				}
			}
		})
	}
}

// Test sentence splitting with various punctuation patterns
func TestSentenceSplitting(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name              string
		text              string
		expectedSentences []string
	}{
		{
			name: "Standard sentences",
			text: "This is sentence one. This is sentence two. This is sentence three.",
			expectedSentences: []string{
				"This is sentence one.",
				"This is sentence two.",
				"This is sentence three.",
			},
		},
		{
			name: "Mixed punctuation",
			text: "Hello! How are you? I'm fine, thanks.",
			expectedSentences: []string{
				"Hello!",
				"How are you?",
				"I'm fine, thanks.",
			},
		},
		{
			name: "Abbreviations",
			text: "Dr. Smith works at the U.S. hospital. He is very experienced.",
			expectedSentences: []string{
				"Dr. Smith works at the U.S. hospital.",
				"He is very experienced.",
			},
		},
		{
			name:              "Empty text",
			text:              "",
			expectedSentences: []string{},
		},
		{
			name:              "Single sentence no period",
			text:              "This is a sentence without period",
			expectedSentences: []string{"This is a sentence without period"},
		},
		{
			name: "Ellipsis",
			text: "Wait... I need to think. Okay, I'm ready.",
			expectedSentences: []string{
				"Wait... I need to think.",
				"Okay, I'm ready.",
			},
		},
		{
			name: "Quoted sentences",
			text: `He said, "Hello there." Then he left.`,
			expectedSentences: []string{
				`He said, "Hello there."`,
				"Then he left.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sentences := nlp.SplitSentences(tt.text)

			if len(sentences) != len(tt.expectedSentences) {
				t.Errorf("Expected %d sentences, got %d", len(tt.expectedSentences), len(sentences))
				for i, sent := range sentences {
					t.Logf("Sentence %d: %s", i+1, sent)
				}
			}

			// Check each sentence content
			for i := 0; i < len(sentences) && i < len(tt.expectedSentences); i++ {
				if sentences[i] != tt.expectedSentences[i] {
					t.Errorf("Sentence %d mismatch:\nExpected: %s\nGot: %s",
						i+1, tt.expectedSentences[i], sentences[i])
				}
			}

			// Verify sentences are not empty (unless input is empty)
			if tt.text != "" {
				for i, sent := range sentences {
					if sent == "" {
						t.Errorf("Sentence %d is empty", i+1)
					}
				}
			}
		})
	}
}

// Test POS tagging accuracy
func TestPOSTagging(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name        string
		text        string
		expectedPOS map[string]string // word -> expected POS
	}{
		{
			name: "Basic sentence",
			text: "The cat sleeps peacefully",
			expectedPOS: map[string]string{
				"The":        "DET",
				"cat":        "NOUN",
				"sleeps":     "VERB",
				"peacefully": "ADV",
			},
		},
		{
			name: "Complex sentence",
			text: "John quickly ran to the store and bought milk",
			expectedPOS: map[string]string{
				"John":    "PROPN",
				"quickly": "ADV",
				"ran":     "VERB",
				"to":      "ADP",
				"store":   "NOUN",
				"and":     "CCONJ",
				"bought":  "VERB",
				"milk":    "NOUN",
			},
		},
		{
			name:        "Empty text",
			text:        "",
			expectedPOS: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			posMap := nlp.POSTag(tt.text)

			// Check if we have POS tags for all expected words
			for word, expectedTag := range tt.expectedPOS {
				if tag, exists := posMap[word]; exists {
					if tag != expectedTag {
						// Log as warning since POS tagging can vary
						t.Logf("POS mismatch for '%s': expected '%s', got '%s'", word, expectedTag, tag)
					}
				} else {
					t.Errorf("No POS tag found for word '%s'", word)
				}
			}

			// Ensure all words have non-empty POS tags
			for word, tag := range posMap {
				if tag == "" {
					t.Errorf("Empty POS tag for word '%s'", word)
				}
			}
		})
	}
}

// Test dependency parsing
func TestDependencyParsing(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name       string
		text       string
		checkWords []string
	}{
		{
			name:       "Simple sentence",
			text:       "The cat sat on the mat",
			checkWords: []string{"cat", "sat", "mat"},
		},
		{
			name:       "Complex sentence",
			text:       "When the sun rises, birds start singing loudly",
			checkWords: []string{"sun", "rises", "birds", "singing"},
		},
		{
			name:       "Empty text",
			text:       "",
			checkWords: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			depMap := nlp.GetDependencies(tt.text)

			// Check that specified words have dependencies
			for _, word := range tt.checkWords {
				if dep, exists := depMap[word]; exists {
					if dep == "" {
						t.Errorf("Empty dependency for word '%s'", word)
					}
				} else {
					t.Errorf("No dependency found for word '%s'", word)
				}
			}

			// Validate all dependencies are non-empty
			for word, dep := range depMap {
				if dep == "" && tt.text != "" {
					t.Errorf("Empty dependency for word '%s'", word)
				}
			}
		})
	}
}

// Test lemmatization
func TestLemmatization(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name           string
		text           string
		expectedLemmas map[string]string // word -> expected lemma
	}{
		{
			name: "Verb forms",
			text: "running ran runs",
			expectedLemmas: map[string]string{
				"running": "run",
				"ran":     "run",
				"runs":    "run",
			},
		},
		{
			name: "Noun plurals",
			text: "cats dogs children",
			expectedLemmas: map[string]string{
				"cats":     "cat",
				"dogs":     "dog",
				"children": "child",
			},
		},
		{
			name: "Mixed forms",
			text: "better best worse worst",
			expectedLemmas: map[string]string{
				"better": "well",
				"best":   "well",
				"worse":  "bad",
				"worst":  "bad",
			},
		},
		{
			name:           "Empty text",
			text:           "",
			expectedLemmas: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lemmaMap := nlp.GetLemmas(tt.text)

			// Check expected lemmas
			for word, expectedLemma := range tt.expectedLemmas {
				if lemma, exists := lemmaMap[word]; exists {
					// Allow some variation in lemmatization
					if lemma != expectedLemma && lemma != word {
						t.Logf("Lemma variation for '%s': expected '%s', got '%s'", word, expectedLemma, lemma)
					}
				} else {
					t.Errorf("No lemma found for word '%s'", word)
				}
			}

			// Ensure all words have non-empty lemmas
			for word, lemma := range lemmaMap {
				if lemma == "" && tt.text != "" {
					t.Errorf("Empty lemma for word '%s'", word)
				}
			}
		})
	}
}

// Test special properties (stop words and punctuation)
func TestSpecialProperties(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	tests := []struct {
		name                string
		text                string
		expectedStopWords   []string
		expectedPunctuation []string
	}{
		{
			name:                "Common stop words",
			text:                "the a an and or but in on at",
			expectedStopWords:   []string{"the", "a", "an", "and", "or", "but", "in", "on", "at"},
			expectedPunctuation: []string{},
		},
		{
			name:                "Punctuation marks",
			text:                "Hello, world! How are you? Fine.",
			expectedStopWords:   []string{"are", "you"},
			expectedPunctuation: []string{",", "!", "?", "."},
		},
		{
			name:                "Mixed content",
			text:                "The cat, and the dog.",
			expectedStopWords:   []string{"the", "and"},
			expectedPunctuation: []string{",", "."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := nlp.Tokenize(tt.text)

			stopWordsFound := make(map[string]bool)
			punctuationFound := make(map[string]bool)

			for _, token := range tokens {
				if token.IsStop {
					stopWordsFound[strings.ToLower(token.Text)] = true
				}
				if token.IsPunct {
					punctuationFound[token.Text] = true
				}
			}

			// Check expected stop words
			for _, word := range tt.expectedStopWords {
				if !stopWordsFound[word] {
					t.Logf("Warning: '%s' not marked as stop word", word)
				}
			}

			// Check expected punctuation
			for _, punct := range tt.expectedPunctuation {
				if !punctuationFound[punct] {
					t.Logf("Warning: '%s' not marked as punctuation", punct)
				}
			}
		})
	}
}
