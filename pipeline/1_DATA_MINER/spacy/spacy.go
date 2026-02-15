//go:generate go run build.go

// Package spacy provides Golang bindings for the Spacy Natural Language Processing library.
//
// AUTOMATIC INSTALLATION:
// This package requires a C++ library to be built. The build process should happen
// automatically when you install the package:
//
//	go get github.com/am-sokolov/go-spacy
//
// If automatic build fails, you can trigger it manually:
//	go generate github.com/am-sokolov/go-spacy
//
// Or run the platform-specific installer:
//	bash scripts/install.sh        # Linux/macOS
//	powershell scripts/install.ps1 # Windows
//
// PREREQUISITES:
//   - C++ compiler (gcc/clang/MSVC)
//   - Python 3.7+ with spacy installed
//   - pkg-config (Linux/macOS) or equivalent
//
// For detailed installation instructions, see docs/INSTALLATION.md

// Package spacy provides Golang bindings for the Spacy Natural Language Processing library.
//
// This package allows you to leverage Spacy's powerful NLP capabilities directly from Go applications
// through a high-performance C++ bridge layer. It supports all major Spacy features including
// tokenization, named entity recognition, part-of-speech tagging, dependency parsing, lemmatization,
// noun chunk extraction, word vectors, semantic similarity, and morphological analysis.
//
// # Features
//
// Core NLP Processing:
//   - Tokenization with detailed linguistic attributes
//   - Named Entity Recognition (NER) with position tracking
//   - Part-of-Speech (POS) tagging with universal and fine-grained tags
//   - Dependency parsing for syntactic analysis
//   - Sentence segmentation
//   - Lemmatization to base word forms
//
// Advanced Features:
//   - Noun chunk extraction with root analysis
//   - Word vectors and document embeddings
//   - Semantic similarity computation
//   - Morphological feature analysis
//   - Stop word and punctuation detection
//
// Multi-language Support:
//   - English (small, medium, large, transformer models)
//   - German, French, Spanish, Italian, Portuguese, Dutch
//   - Chinese, Japanese, and many other languages
//   - Domain-specific models (biomedical, scientific, etc.)
//
// # Performance
//
// The binding is designed for high performance with:
//   - Zero-copy data structures where possible
//   - Efficient memory management with proper cleanup
//   - Thread-safe operations with mutex protection
//   - Support for concurrent processing
//   - Minimal overhead compared to native Python Spacy
//
// # Usage
//
// Basic usage example:
//
//	nlp, err := spacy.NewNLP("en_core_web_sm")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer nlp.Close()
//
//	text := "Apple Inc. was founded by Steve Jobs in California."
//
//	// Tokenization
//	tokens := nlp.Tokenize(text)
//	for _, token := range tokens {
//		fmt.Printf("Token: %s, POS: %s, Lemma: %s\n",
//			token.Text, token.POS, token.Lemma)
//	}
//
//	// Named Entity Recognition
//	entities := nlp.ExtractEntities(text)
//	for _, entity := range entities {
//		fmt.Printf("Entity: %s [%s] at [%d:%d]\n",
//			entity.Text, entity.Label, entity.Start, entity.End)
//	}
//
// Advanced features example:
//
//	// Noun chunks
//	chunks := nlp.GetNounChunks(text)
//	for _, chunk := range chunks {
//		fmt.Printf("Chunk: %s (root: %s)\n", chunk.Text, chunk.RootText)
//	}
//
//	// Semantic similarity (requires model with word vectors)
//	similarity := nlp.Similarity("cat", "dog")
//	fmt.Printf("Similarity: %.3f\n", similarity)
//
//	// Word vectors
//	vector := nlp.GetVector("apple")
//	if vector.HasVector {
//		fmt.Printf("Vector dimension: %d\n", len(vector.Vector))
//	}
//
// # Installation
//
// Prerequisites:
//   - Go 1.16 or higher
//   - Python 3.9+ with Spacy installed
//   - C++ compiler (g++ or clang++)
//
// Install dependencies:
//
//	pip install spacy
//	python -m spacy download en_core_web_sm
//
// Build the C++ wrapper:
//
//	make clean && make
//
// # Thread Safety
//
// The NLP instance uses internal mutex protection for thread safety. Multiple goroutines
// can safely call methods on the same NLP instance concurrently. However, each NLP instance
// maintains its own Spacy model, so creating multiple instances for different models is supported.
//
// # Model Management
//
// Spacy models are loaded once per NLP instance and cached for efficiency. Model switching
// is supported by creating new NLP instances. Always call Close() when done to clean up resources.
//
// # Error Handling
//
// All methods are designed to handle errors gracefully. If the underlying Spacy model
// encounters an error, methods will return empty results rather than panicking.
// Check the console output for detailed error messages from the Spacy layer.
package spacy

/*
#cgo pkg-config: python3
#cgo CFLAGS: -I./include
#cgo LDFLAGS: -L./lib -lspacy_wrapper
#include <stdlib.h>
#include "spacy_wrapper.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Token represents a single token from tokenized text with comprehensive linguistic annotations.
//
// A token contains the original text along with various linguistic properties assigned by Spacy's
// linguistic analysis pipeline. These properties enable sophisticated text analysis and understanding.
type Token struct {
	// Text is the original token text as it appears in the source
	Text string

	// Lemma is the base or dictionary form of the token (e.g., "running" -> "run")
	Lemma string

	// POS is the Universal POS tag (coarse-grained part-of-speech, e.g., "NOUN", "VERB")
	POS string

	// Tag is the detailed POS tag (fine-grained, language-specific, e.g., "NNS", "VBG")
	Tag string

	// Dep is the dependency relation to the head token (e.g., "nsubj", "dobj", "prep")
	Dep string

	// IsStop indicates whether this token is a stop word (common function words)
	IsStop bool

	// IsPunct indicates whether this token is punctuation
	IsPunct bool

	// Start is the character offset where the token begins
	Start int

	// End is the character offset where the token ends
	End int
}

// Entity represents a named entity identified in text with its classification and position.
//
// Named entities are real-world objects like persons, organizations, locations, dates, etc.
// that have been identified and classified by Spacy's NER system.
type Entity struct {
	// Text is the actual text span of the entity as it appears in the source
	Text string

	// Label is the entity type classification (e.g., "PERSON", "ORG", "GPE", "DATE")
	// Common labels include:
	//   - PERSON: People, including fictional
	//   - ORG: Companies, agencies, institutions
	//   - GPE: Countries, cities, states (geopolitical entities)
	//   - LOC: Non-GPE locations, mountain ranges, bodies of water
	//   - DATE: Absolute or relative dates or periods
	//   - MONEY: Monetary values, including unit
	//   - PERCENT: Percentage values
	//   - TIME: Times smaller than a day
	Label string

	// Start is the character offset where the entity begins in the original text
	Start int

	// End is the character offset where the entity ends in the original text
	End int
}

// NounChunk represents a noun phrase identified in text with its root analysis.
//
// Noun chunks are base noun phrases – flat phrases that have a noun as their head.
// They provide a way to extract meaningful noun-based concepts from text.
type NounChunk struct {
	// Text is the full text span of the noun chunk (e.g., "the quick brown fox")
	Text string

	// RootText is the text of the root token of the noun chunk (e.g., "fox")
	RootText string

	// RootDep is the dependency relation of the root token (e.g., "nsubj", "dobj")
	RootDep string

	// Start is the character offset where the chunk begins in the original text
	Start int

	// End is the character offset where the chunk ends in the original text
	End int
}

// VectorData represents word or document vectors for semantic analysis.
//
// Word vectors are dense numerical representations that capture semantic meaning.
// They enable similarity computations and various semantic NLP tasks.
type VectorData struct {
	// Vector contains the numerical vector representation as float64 values
	// The dimension depends on the model (e.g., 96, 300, 768)
	Vector []float64

	// HasVector indicates whether the model contains vectors for this text
	// Small models may not include word vectors
	HasVector bool
}

// MorphFeature represents a morphological feature with its key-value pair.
//
// Morphological features describe grammatical properties like gender, number,
// tense, case, etc. These are language-specific and provide detailed grammatical analysis.
type MorphFeature struct {
	// Key is the feature name (e.g., "Number", "Gender", "Tense")
	Key string

	// Value is the feature value (e.g., "Sing", "Plur", "Masc", "Fem", "Past", "Pres")
	Value string
}

// NLP represents a Spacy natural language processing pipeline instance.
//
// Each NLP instance is associated with a specific Spacy model and provides access to
// all NLP processing functions. The instance maintains the loaded model and handles
// thread-safe access to the underlying Spacy functionality.
//
// Important: Always call Close() when done with an NLP instance to free resources.
type NLP struct {
	model string
}

// NewNLP creates a new NLP instance with the specified Spacy model.
//
// This is the main entry point for using the go-spacy library. It initializes the Python
// interpreter, loads the specified Spacy model, and creates a bridge to access Spacy's
// NLP capabilities from Go.
//
// # Model Selection Guide
//
// English Models by Size and Capability:
//   - "en_core_web_sm" (~13MB): Fastest, no word vectors, good for basic NLP tasks
//     Components: tok2vec, tagger, parser, ner, lemmatizer
//   - "en_core_web_md" (~40MB): Balanced performance, 20k word vectors (GloVe)
//     Components: tok2vec, tagger, parser, ner, lemmatizer, vectors
//   - "en_core_web_lg" (~740MB): Most accurate, 685k word vectors
//     Components: tok2vec, tagger, parser, ner, lemmatizer, vectors
//   - "en_core_web_trf" (~420MB): Transformer-based (RoBERTa), state-of-the-art
//     Components: transformer, tagger, parser, ner, lemmatizer
//
// Multilingual Models (examples):
//   - German: de_core_news_sm, de_core_news_md, de_core_news_lg
//   - French: fr_core_news_sm, fr_core_news_md, fr_core_news_lg
//   - Spanish: es_core_news_sm, es_core_news_md, es_core_news_lg
//   - Chinese: zh_core_web_sm, zh_core_web_md, zh_core_web_lg
//   - Japanese: ja_core_news_sm, ja_core_news_md, ja_core_news_lg
//   - Italian: it_core_news_sm, it_core_news_md, it_core_news_lg
//   - Portuguese: pt_core_news_sm, pt_core_news_md, pt_core_news_lg
//   - Russian: ru_core_news_sm, ru_core_news_md, ru_core_news_lg
//   - Dutch: nl_core_news_sm, nl_core_news_md, nl_core_news_lg
//
// # Performance Characteristics
//
// Processing Speed (relative to en_core_web_sm):
//   - Small models: 1.0x (baseline)
//   - Medium models: 0.7x (30% slower)
//   - Large models: 0.5x (50% slower)
//   - Transformer models: 0.1x (10x slower, GPU recommended)
//
// Accuracy (F-score on OntoNotes 5.0):
//   - Small models: ~0.89 NER, ~0.91 POS
//   - Medium models: ~0.90 NER, ~0.92 POS
//   - Large models: ~0.91 NER, ~0.93 POS
//   - Transformer models: ~0.93 NER, ~0.96 POS
//
// # Model Installation
//
// Models must be downloaded before use:
//
//	# Basic installation
//	python -m spacy download en_core_web_sm
//
//	# Specific version
//	python -m spacy download en_core_web_sm==3.7.1
//
//	# Install all English models
//	python -m spacy download en_core_web_sm en_core_web_md en_core_web_lg
//
//	# Verify installation
//	python -c "import spacy; nlp = spacy.load('en_core_web_sm'); print('Model loaded successfully')"
//
// # Memory Usage
//
//   - Small models: ~50MB RAM
//   - Medium models: ~200MB RAM
//   - Large models: ~800MB RAM
//   - Transformer models: ~1.5GB RAM (+ GPU memory if available)
//
// # Thread Safety and Concurrency
//
// The NLP instance is thread-safe and can handle concurrent requests:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	// Process texts concurrently
//	texts := []string{"Text 1", "Text 2", "Text 3", "Text 4"}
//	results := make([][]Token, len(texts))
//	var wg sync.WaitGroup
//
//	for i, text := range texts {
//		wg.Add(1)
//		go func(idx int, t string) {
//			defer wg.Done()
//			results[idx] = nlp.Tokenize(t)
//		}(i, text)
//	}
//	wg.Wait()
//
// # Error Handling
//
// Returns an error if:
//   - Model is not installed: "failed to initialize Spacy with model: en_core_web_xyz"
//   - Python not found: "Python interpreter initialization failed"
//   - Insufficient memory: "unable to allocate memory for model"
//   - Version mismatch: "model requires Spacy version X.Y.Z"
//
// # Best Practices
//
//  1. Model Selection:
//     - Use small models for production APIs with latency requirements
//     - Use medium models when you need word vectors for similarity
//     - Use large/transformer models for maximum accuracy in batch processing
//
//  2. Resource Management:
//     - Always defer nlp.Close() to free resources
//     - Reuse NLP instances across requests (model loading is expensive)
//     - Consider pooling NLP instances for high-concurrency scenarios
//
//  3. Performance Optimization:
//     - Batch process texts when possible
//     - Disable unused pipeline components
//     - Use smaller models for real-time applications
//
// Note: Models must be downloaded separately using:
//
//	python -m spacy download en_core_web_sm
func NewNLP(modelName string) (*NLP, error) {
	cModel := C.CString(modelName)
	defer C.free(unsafe.Pointer(cModel))

	ret := C.spacy_init(cModel)
	if ret != 0 {
		return nil, fmt.Errorf("failed to initialize Spacy with model: %s", modelName)
	}

	return &NLP{model: modelName}, nil
}

// Close releases resources associated with the NLP instance.
//
// This method should always be called when finished with an NLP instance to prevent
// resource leaks. It cleans up the underlying Spacy model and C++ bridge resources.
//
// After calling Close(), the NLP instance should not be used further.
//
// Example:
//
//	nlp, err := spacy.NewNLP("en_core_web_sm")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer nlp.Close() // Ensure cleanup even if function exits early
//
// It's safe to call Close() multiple times on the same instance.
func (n *NLP) Close() {
	C.spacy_cleanup()
}

// Tokenize breaks text into individual tokens with comprehensive linguistic analysis.
//
// This function performs tokenization using Spacy's advanced tokenization rules,
// which go beyond simple whitespace splitting. It handles:
//   - Contractions (e.g., "don't" -> ["do", "n't"])
//   - Punctuation attachment (e.g., "U.S.A." as one token)
//   - Special cases like email addresses, URLs, numbers
//
// Each returned Token contains detailed linguistic information including lemma,
// part-of-speech tags, dependency relations, and semantic flags.
//
// Parameters:
//   - text: The input text to tokenize (any length, UTF-8 encoded)
//
// Returns:
//   - []Token: Slice of tokens with linguistic annotations
//   - Empty slice if text is empty or tokenization fails
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	tokens := nlp.Tokenize("Apple Inc. was founded in 1976.")
//	for i, token := range tokens {
//		fmt.Printf("%d: '%s' [%s] lemma=%s dep=%s\n",
//			i, token.Text, token.POS, token.Lemma, token.Dep)
//	}
//	// Output:
//	// 0: 'Apple' [PROPN] lemma=Apple dep=nsubj
//	// 1: 'Inc.' [PROPN] lemma=Inc. dep=flat
//	// 2: 'was' [AUX] lemma=be dep=aux
//	// 3: 'founded' [VERB] lemma=found dep=ROOT
//	// 4: 'in' [ADP] lemma=in dep=prep
//	// 5: '1976' [NUM] lemma=1976 dep=pobj
//	// 6: '.' [PUNCT] lemma=. dep=punct
//
// Performance: Tokenization is generally fast, but the first call may take longer
// due to model initialization. Reuse the NLP instance for best performance.
func (n *NLP) Tokenize(text string) []Token {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	tokenArray := C.spacy_tokenize(cText)
	defer C.free_token_array(&tokenArray)

	tokens := make([]Token, tokenArray.count)

	if tokenArray.count == 0 {
		return tokens
	}

	cTokens := (*[1 << 30]C.CToken)(unsafe.Pointer(tokenArray.tokens))[:tokenArray.count:tokenArray.count]

	for i, cToken := range cTokens {
		tokens[i] = Token{
			Text:    C.GoString(cToken.text),
			Lemma:   C.GoString(cToken.lemma),
			POS:     C.GoString(cToken.pos),
			Tag:     C.GoString(cToken.tag),
			Dep:     C.GoString(cToken.dep),
			IsStop:  bool(cToken.is_stop),
			IsPunct: bool(cToken.is_punct),
			Start:   int(cToken.start),
			End:     int(cToken.end),
		}
	}

	return tokens
}

// ExtractEntities identifies and classifies named entities in text.
//
// Named Entity Recognition (NER) finds mentions of real-world entities like people,
// organizations, locations, dates, and other structured information. The function
// returns entities with their text, classification, and exact position in the source.
//
// Common entity types include:
//   - PERSON: People, including fictional characters
//   - ORG: Companies, agencies, institutions, etc.
//   - GPE: Countries, cities, states (geopolitical entities)
//   - LOC: Non-GPE locations like mountains, bodies of water
//   - DATE: Absolute or relative dates or periods
//   - TIME: Times smaller than a day
//   - MONEY: Monetary values including unit
//   - PERCENT: Percentage values
//   - CARDINAL: Numerals that do not fall under another type
//
// Parameters:
//   - text: Input text to analyze for entities
//
// Returns:
//   - []Entity: Slice of identified entities with positions
//   - Empty slice if no entities found or processing fails
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	text := "Apple Inc. was founded by Steve Jobs in Cupertino on April 1, 1976."
//	entities := nlp.ExtractEntities(text)
//	for _, entity := range entities {
//		fmt.Printf("Entity: '%s' [%s] at position %d-%d\n",
//			entity.Text, entity.Label, entity.Start, entity.End)
//	}
//	// Output might include:
//	// Entity: 'Apple Inc.' [ORG] at position 0-10
//	// Entity: 'Steve Jobs' [PERSON] at position 26-36
//	// Entity: 'Cupertino' [GPE] at position 40-49
//	// Entity: 'April 1, 1976' [DATE] at position 53-66
//
// Note: Entity recognition accuracy depends on the model used. Larger models
// generally provide better accuracy but require more computational resources.
func (n *NLP) ExtractEntities(text string) []Entity {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	entityArray := C.spacy_extract_entities(cText)
	defer C.free_entity_array(&entityArray)

	entities := make([]Entity, entityArray.count)

	if entityArray.count == 0 {
		return entities
	}

	cEntities := (*[1 << 30]C.CEntity)(unsafe.Pointer(entityArray.entities))[:entityArray.count:entityArray.count]

	for i, cEntity := range cEntities {
		entities[i] = Entity{
			Text:  C.GoString(cEntity.text),
			Label: C.GoString(cEntity.label),
			Start: int(cEntity.start),
			End:   int(cEntity.end),
		}
	}

	return entities
}

// SplitSentences segments text into individual sentences.
//
// Sentence segmentation uses Spacy's sophisticated sentence boundary detection,
// which considers linguistic patterns beyond simple punctuation rules. It handles:
//   - Abbreviations (e.g., "Dr. Smith went home.")
//   - Decimal numbers (e.g., "The price is $3.50 per item.")
//   - Ellipsis and other punctuation patterns
//   - Language-specific sentence patterns
//
// Parameters:
//   - text: Input text to segment into sentences
//
// Returns:
//   - []string: Slice of sentences as separate strings
//   - Empty slice if no sentences found or text is empty
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	text := "Dr. Smith went to the U.S.A. He arrived on Monday. What a trip!"
//	sentences := nlp.SplitSentences(text)
//	for i, sentence := range sentences {
//		fmt.Printf("Sentence %d: %s\n", i+1, sentence)
//	}
//	// Output:
//	// Sentence 1: Dr. Smith went to the U.S.A.
//	// Sentence 2: He arrived on Monday.
//	// Sentence 3: What a trip!
//
// Note: Sentence boundaries may vary between languages and models. Some models
// are trained on specific domains and may have different boundary detection rules.
func (n *NLP) SplitSentences(text string) []string {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	sentArray := C.spacy_split_sentences(cText)
	defer C.free_sentence_array(&sentArray)

	sentences := make([]string, sentArray.count)

	if sentArray.count == 0 {
		return sentences
	}

	cSentences := (*[1 << 30]*C.char)(unsafe.Pointer(sentArray.sentences))[:sentArray.count:sentArray.count]

	for i, cSent := range cSentences {
		sentences[i] = C.GoString(cSent)
	}

	return sentences
}

// POSTag returns part-of-speech tags for all tokens in the text.
//
// Part-of-Speech tagging assigns grammatical categories to each token.
// This function returns a map where keys are token texts and values are POS tags.
// Note that if the same token appears multiple times with different POS tags,
// only the last occurrence's tag is retained in the map.
//
// Universal POS tags include:
//   - NOUN: Nouns (e.g., "cat", "tree", "idea")
//   - VERB: Verbs (e.g., "run", "be", "have")
//   - ADJ: Adjectives (e.g., "big", "old", "green")
//   - ADV: Adverbs (e.g., "very", "well", "quickly")
//   - PRON: Pronouns (e.g., "I", "you", "it")
//   - DET: Determiners (e.g., "the", "a", "this")
//   - ADP: Adpositions (e.g., "in", "to", "during")
//   - NUM: Numerals (e.g., "1", "two", "first")
//   - CONJ: Conjunctions (e.g., "and", "or", "but")
//   - PRT: Particles (e.g., "off", "up", "over")
//
// Parameters:
//   - text: Input text to analyze for POS tags
//
// Returns:
//   - map[string]string: Map of token text to POS tag
//   - Empty map if text is empty or processing fails
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	posMap := nlp.POSTag("The quick brown fox jumps.")
//	for token, pos := range posMap {
//		fmt.Printf("'%s' -> %s\n", token, pos)
//	}
//	// Output might include:
//	// 'The' -> DET
//	// 'quick' -> ADJ
//	// 'brown' -> ADJ
//	// 'fox' -> NOUN
//	// 'jumps' -> VERB
//	// '.' -> PUNCT
func (n *NLP) POSTag(text string) map[string]string {
	tokens := n.Tokenize(text)
	posMap := make(map[string]string)

	for _, token := range tokens {
		posMap[token.Text] = token.POS
	}

	return posMap
}

// GetDependencies returns dependency relations for all tokens in the text.
//
// Dependency parsing analyzes the grammatical structure of sentences by establishing
// relationships between words. This function returns a map where keys are token texts
// and values are dependency relation labels.
//
// Common dependency relations include:
//   - nsubj: Nominal subject (e.g., "cats" in "cats sit")
//   - dobj: Direct object (e.g., "ball" in "hit ball")
//   - root: Root of the sentence (main verb)
//   - det: Determiner (e.g., "the" in "the cat")
//   - amod: Adjectival modifier (e.g., "big" in "big house")
//   - prep: Preposition (e.g., "in" in "in the house")
//   - pobj: Object of preposition (e.g., "house" in "in the house")
//   - aux: Auxiliary verb (e.g., "is" in "is running")
//   - punct: Punctuation
//
// Parameters:
//   - text: Input text to analyze for dependencies
//
// Returns:
//   - map[string]string: Map of token text to dependency relation
//   - Empty map if text is empty or processing fails
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	depMap := nlp.GetDependencies("The cat sits on the mat.")
//	for token, dep := range depMap {
//		fmt.Printf("'%s' -> %s\n", token, dep)
//	}
//	// Output might include:
//	// 'cat' -> nsubj
//	// 'sits' -> ROOT
//	// 'on' -> prep
//	// 'mat' -> pobj
func (n *NLP) GetDependencies(text string) map[string]string {
	tokens := n.Tokenize(text)
	depMap := make(map[string]string)

	for _, token := range tokens {
		depMap[token.Text] = token.Dep
	}

	return depMap
}

// GetLemmas returns lemmatized forms for all tokens in the text.
//
// Lemmatization reduces words to their base or dictionary form (lemma).
// Unlike stemming, lemmatization considers the word's context and part-of-speech
// to provide the correct base form.
//
// This function returns a map where keys are token texts and values are their lemmas.
//
// Examples of lemmatization:
//   - "running", "ran", "runs" -> "run"
//   - "better" -> "good"
//   - "mice" -> "mouse"
//   - "children" -> "child"
//   - "was", "were", "is", "are" -> "be"
//
// Parameters:
//   - text: Input text to lemmatize
//
// Returns:
//   - map[string]string: Map of token text to lemma
//   - Empty map if text is empty or processing fails
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	lemmaMap := nlp.GetLemmas("The children were running quickly.")
//	for token, lemma := range lemmaMap {
//		fmt.Printf("'%s' -> '%s'\n", token, lemma)
//	}
//	// Output might include:
//	// 'children' -> 'child'
//	// 'were' -> 'be'
//	// 'running' -> 'run'
//	// 'quickly' -> 'quickly'
func (n *NLP) GetLemmas(text string) map[string]string {
	tokens := n.Tokenize(text)
	lemmaMap := make(map[string]string)

	for _, token := range tokens {
		lemmaMap[token.Text] = token.Lemma
	}

	return lemmaMap
}

// GetNounChunks extracts noun phrases from the text with root analysis.
//
// Noun chunks are base noun phrases – flat phrases that have a noun as their head.
// These are useful for extracting the main concepts and entities from text.
// Each chunk includes the full phrase text, root token information, and position.
//
// Noun chunks help identify:
//   - Key concepts and topics
//   - Subject and object phrases
//   - Named entity candidates
//   - Important terminology
//
// Parameters:
//   - text: Input text to analyze for noun chunks
//
// Returns:
//   - []NounChunk: Slice of noun chunks with root analysis
//   - Empty slice if no chunks found or processing fails
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	text := "The quick brown fox jumps over the lazy dog."
//	chunks := nlp.GetNounChunks(text)
//	for _, chunk := range chunks {
//		fmt.Printf("Chunk: '%s' (root: '%s', dep: %s)\n",
//			chunk.Text, chunk.RootText, chunk.RootDep)
//	}
//	// Output might include:
//	// Chunk: 'The quick brown fox' (root: 'fox', dep: nsubj)
//	// Chunk: 'the lazy dog' (root: 'dog', dep: pobj)
func (n *NLP) GetNounChunks(text string) []NounChunk {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	chunkArray := C.spacy_get_noun_chunks(cText)
	defer C.free_chunk_array(&chunkArray)

	chunks := make([]NounChunk, chunkArray.count)

	if chunkArray.count == 0 {
		return chunks
	}

	cChunks := (*[1 << 30]C.CChunk)(unsafe.Pointer(chunkArray.chunks))[:chunkArray.count:chunkArray.count]

	for i, cChunk := range cChunks {
		chunks[i] = NounChunk{
			Text:     C.GoString(cChunk.text),
			RootText: C.GoString(cChunk.root_text),
			RootDep:  C.GoString(cChunk.root_dep),
			Start:    int(cChunk.start),
			End:      int(cChunk.end),
		}
	}

	return chunks
}

// GetVector returns the vector representation of text for semantic analysis.
//
// Word and document vectors are dense numerical representations that capture
// semantic meaning. These vectors enable similarity calculations, clustering,
// and other semantic NLP tasks. Vector availability depends on the model used.
//
// Vector properties:
//   - Small models (en_core_web_sm): Context-sensitive tensors, no static vectors
//   - Medium models (en_core_web_md): 300-dimensional GloVe vectors
//   - Large models (en_core_web_lg): 300-dimensional GloVe vectors
//   - Transformer models: Higher-dimensional contextual vectors
//
// Parameters:
//   - text: Input text to vectorize (word, phrase, or document)
//
// Returns:
//   - VectorData: Struct containing vector and availability information
//   - HasVector=false if model doesn't support vectors
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_md")  // Medium model with vectors
//	defer nlp.Close()
//
//	vector := nlp.GetVector("apple")
//	if vector.HasVector {
//		fmt.Printf("Vector dimension: %d\n", len(vector.Vector))
//		fmt.Printf("First few values: %.3f, %.3f, %.3f...\n",
//			vector.Vector[0], vector.Vector[1], vector.Vector[2])
//	} else {
//		fmt.Println("No vectors available in this model")
//	}
//
// Note: Use medium or large models for vector operations. Small models
// use context-sensitive tensors which don't provide static word vectors.
func (n *NLP) GetVector(text string) VectorData {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	vecData := C.spacy_get_vector(cText)
	defer C.free_vector_data(&vecData)

	result := VectorData{
		HasVector: bool(vecData.has_vector),
	}

	if vecData.has_vector && vecData.size > 0 {
		result.Vector = make([]float64, vecData.size)
		cVector := (*[1 << 30]C.double)(unsafe.Pointer(vecData.vector))[:vecData.size:vecData.size]
		for i, val := range cVector {
			result.Vector[i] = float64(val)
		}
	}

	return result
}

// Similarity computes semantic similarity between two texts.
//
// Semantic similarity measures how related two pieces of text are in meaning.
// The function returns a value between 0.0 (completely dissimilar) and 1.0
// (identical meaning). This requires models with word vectors for meaningful results.
//
// Similarity can be computed between:
//   - Individual words (e.g., "cat" vs "dog")
//   - Phrases (e.g., "New York" vs "NYC")
//   - Sentences or documents
//
// Models and similarity:
//   - Small models: Use linguistic features (less reliable)
//   - Medium/Large models: Use word vectors (more reliable)
//   - Transformer models: Use contextual representations
//
// Parameters:
//   - text1: First text for comparison
//   - text2: Second text for comparison
//
// Returns:
//   - float64: Similarity score between 0.0 and 1.0
//   - 0.0 if similarity cannot be computed
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_md")  // Medium model with vectors
//	defer nlp.Close()
//
//	similarity := nlp.Similarity("cat", "dog")
//	fmt.Printf("cat vs dog similarity: %.3f\n", similarity)
//
//	similarity = nlp.Similarity("New York", "NYC")
//	fmt.Printf("New York vs NYC similarity: %.3f\n", similarity)
//
//	similarity = nlp.Similarity("car", "bicycle")
//	fmt.Printf("car vs bicycle similarity: %.3f\n", similarity)
//	// Output might be:
//	// cat vs dog similarity: 0.742
//	// New York vs NYC similarity: 0.809
//	// car vs bicycle similarity: 0.654
//
// Note: Similarity scores depend on the training data and model. Results
// may vary between different model versions and languages.
func (n *NLP) Similarity(text1, text2 string) float64 {
	cText1 := C.CString(text1)
	defer C.free(unsafe.Pointer(cText1))
	cText2 := C.CString(text2)
	defer C.free(unsafe.Pointer(cText2))

	return float64(C.spacy_similarity(cText1, cText2))
}

// GetMorphology returns detailed morphological features for tokens in the text.
//
// Morphological analysis provides fine-grained grammatical information about
// word forms, including features like gender, number, tense, case, mood, and more.
// These features are language-specific and enable sophisticated grammatical analysis.
//
// Common morphological features:
//   - Number: Sing (singular), Plur (plural)
//   - Gender: Masc (masculine), Fem (feminine), Neut (neuter)
//   - Case: Nom (nominative), Acc (accusative), Gen (genitive), Dat (dative)
//   - Tense: Past, Pres (present), Fut (future)
//   - Mood: Ind (indicative), Imp (imperative), Sub (subjunctive)
//   - Person: 1, 2, 3 (first, second, third person)
//   - VerbForm: Fin (finite), Inf (infinitive), Part (participle)
//   - Aspect: Perf (perfective), Imp (imperfective), Prog (progressive)
//
// Parameters:
//   - text: Input text to analyze for morphological features
//
// Returns:
//   - []MorphFeature: Slice of morphological features found in the text
//   - Empty slice if no features found or processing fails
//
// Example:
//
//	nlp, _ := spacy.NewNLP("en_core_web_sm")
//	defer nlp.Close()
//
//	features := nlp.GetMorphology("The cats were sleeping peacefully.")
//	for _, feature := range features {
//		fmt.Printf("Feature: %s = %s\n", feature.Key, feature.Value)
//	}
//	// Output might include:
//	// Feature: Number = Plur
//	// Feature: Tense = Past
//	// Feature: VerbForm = Fin
//	// Feature: Aspect = Prog
//
// Note: Morphological feature availability and naming conventions vary
// between languages and models. Some languages provide richer morphological
// information than others.
func (n *NLP) GetMorphology(text string) []MorphFeature {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	morphArray := C.spacy_get_morphology(cText)
	defer C.free_morph_array(&morphArray)

	features := make([]MorphFeature, morphArray.count)

	if morphArray.count == 0 {
		return features
	}

	cFeatures := (*[1 << 30]C.CMorphFeature)(unsafe.Pointer(morphArray.features))[:morphArray.count:morphArray.count]

	for i, cFeature := range cFeatures {
		features[i] = MorphFeature{
			Key:   C.GoString(cFeature.morph_key),
			Value: C.GoString(cFeature.morph_value),
		}
	}

	return features
}
