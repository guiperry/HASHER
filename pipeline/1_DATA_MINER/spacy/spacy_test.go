package spacy

import (
	"testing"
)

func TestNewNLP(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	if nlp.model != "en_core_web_sm" {
		t.Errorf("Expected model name 'en_core_web_sm', got '%s'", nlp.model)
	}
}

func TestTokenize(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "Apple is looking at buying U.K. startup for $1 billion"
	tokens := nlp.Tokenize(text)

	if len(tokens) == 0 {
		t.Fatal("Expected tokens, got none")
	}

	expectedTokens := []string{"Apple", "is", "looking", "at", "buying", "U.K.", "startup", "for", "$", "1", "billion"}

	if len(tokens) != len(expectedTokens) {
		t.Errorf("Expected %d tokens, got %d", len(expectedTokens), len(tokens))
	}

	for i, token := range tokens {
		if i < len(expectedTokens) && token.Text != expectedTokens[i] {
			t.Errorf("Token %d: expected '%s', got '%s'", i, expectedTokens[i], token.Text)
		}

		if token.POS == "" {
			t.Errorf("Token '%s' has empty POS tag", token.Text)
		}

		if token.Lemma == "" {
			t.Errorf("Token '%s' has empty lemma", token.Text)
		}
	}
}

func TestPOSTag(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "The quick brown fox jumps over the lazy dog"
	posMap := nlp.POSTag(text)

	if len(posMap) == 0 {
		t.Fatal("Expected POS tags, got none")
	}

	expectedPOS := map[string]string{
		"The":   "DET",
		"quick": "ADJ",
		"brown": "ADJ",
		"fox":   "NOUN",
		"jumps": "VERB",
		"over":  "ADP",
		"lazy":  "ADJ",
		"dog":   "NOUN",
	}

	for word, expectedTag := range expectedPOS {
		if tag, exists := posMap[word]; exists {
			if tag != expectedTag {
				t.Logf("Word '%s': expected POS '%s', got '%s' (warning only)", word, expectedTag, tag)
			}
		} else {
			t.Errorf("Word '%s' not found in POS map", word)
		}
	}
}

func TestExtractEntities(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "Apple Inc. was founded by Steve Jobs in Cupertino, California on April 1, 1976."
	entities := nlp.ExtractEntities(text)

	if len(entities) == 0 {
		t.Fatal("Expected entities, got none")
	}

	foundApple := false
	foundSteve := false
	foundLocation := false
	foundDate := false

	for _, entity := range entities {
		t.Logf("Entity: %s [%s] (%d-%d)", entity.Text, entity.Label, entity.Start, entity.End)

		if entity.Text == "Apple Inc." {
			foundApple = true
			if entity.Label != "ORG" {
				t.Errorf("Expected 'Apple Inc.' to be labeled as ORG, got %s", entity.Label)
			}
		}

		if entity.Text == "Steve Jobs" {
			foundSteve = true
			if entity.Label != "PERSON" {
				t.Errorf("Expected 'Steve Jobs' to be labeled as PERSON, got %s", entity.Label)
			}
		}

		if entity.Label == "GPE" || entity.Label == "LOC" {
			foundLocation = true
		}

		if entity.Label == "DATE" {
			foundDate = true
		}
	}

	if !foundApple {
		t.Error("Failed to find 'Apple Inc.' entity")
	}
	if !foundSteve {
		t.Error("Failed to find 'Steve Jobs' entity")
	}
	if !foundLocation {
		t.Error("Failed to find location entity")
	}
	if !foundDate {
		t.Error("Failed to find date entity")
	}
}

func TestSplitSentences(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "This is the first sentence. This is the second sentence! Is this the third sentence? Yes, and this is the fourth."
	sentences := nlp.SplitSentences(text)

	if len(sentences) != 4 {
		t.Errorf("Expected 4 sentences, got %d", len(sentences))
		for i, sent := range sentences {
			t.Logf("Sentence %d: %s", i+1, sent)
		}
	}

	expectedSentences := []string{
		"This is the first sentence.",
		"This is the second sentence!",
		"Is this the third sentence?",
		"Yes, and this is the fourth.",
	}

	for i, sent := range sentences {
		if i < len(expectedSentences) {
			if sent != expectedSentences[i] {
				t.Errorf("Sentence %d: expected '%s', got '%s'", i+1, expectedSentences[i], sent)
			}
		}
	}
}

func TestGetDependencies(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "The cat sat on the mat"
	depMap := nlp.GetDependencies(text)

	if len(depMap) == 0 {
		t.Fatal("Expected dependencies, got none")
	}

	for word, dep := range depMap {
		t.Logf("Word '%s' has dependency: %s", word, dep)
		if dep == "" {
			t.Errorf("Word '%s' has empty dependency", word)
		}
	}

	if _, exists := depMap["cat"]; !exists {
		t.Error("Expected 'cat' in dependency map")
	}
	if _, exists := depMap["sat"]; !exists {
		t.Error("Expected 'sat' in dependency map")
	}
}

func TestGetLemmas(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "The cats are running quickly"
	lemmaMap := nlp.GetLemmas(text)

	if len(lemmaMap) == 0 {
		t.Fatal("Expected lemmas, got none")
	}

	expectedLemmas := map[string]string{
		"cats":    "cat",
		"are":     "be",
		"running": "run",
		"quickly": "quickly",
	}

	for word, expectedLemma := range expectedLemmas {
		if lemma, exists := lemmaMap[word]; exists {
			if lemma != expectedLemma {
				t.Logf("Word '%s': expected lemma '%s', got '%s' (warning only)", word, expectedLemma, lemma)
			}
		} else {
			t.Errorf("Word '%s' not found in lemma map", word)
		}
	}
}

func TestTokenProperties(t *testing.T) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		t.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "The, and, or are stop words. Hello world!"
	tokens := nlp.Tokenize(text)

	foundStop := false
	foundPunct := false

	for _, token := range tokens {
		if token.IsStop {
			foundStop = true
			t.Logf("Stop word found: %s", token.Text)
		}
		if token.IsPunct {
			foundPunct = true
			t.Logf("Punctuation found: %s", token.Text)
		}
	}

	if !foundStop {
		t.Error("Expected to find stop words")
	}
	if !foundPunct {
		t.Error("Expected to find punctuation")
	}
}

func BenchmarkTokenize(b *testing.B) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		b.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "Apple is looking at buying U.K. startup for $1 billion. This is a test sentence for benchmarking."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nlp.Tokenize(text)
	}
}

func BenchmarkExtractEntities(b *testing.B) {
	nlp, err := NewNLP("en_core_web_sm")
	if err != nil {
		b.Fatalf("Failed to create NLP: %v", err)
	}
	defer nlp.Close()

	text := "Apple Inc. was founded by Steve Jobs in Cupertino, California on April 1, 1976."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nlp.ExtractEntities(text)
	}
}
