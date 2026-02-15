# API Reference

## Package Overview

Package `spacy` provides Golang bindings for the Spacy Natural Language Processing library through a high-performance C++ bridge layer. It supports all major Spacy features including tokenization, named entity recognition, part-of-speech tagging, dependency parsing, lemmatization, noun chunk extraction, word vectors, semantic similarity, and morphological analysis.

## Table of Contents

- [Types](#types)
  - [Token](#token)
  - [Entity](#entity)
  - [NounChunk](#nounchunk)
  - [VectorData](#vectordata)
  - [MorphFeature](#morphfeature)
  - [NLP](#nlp)
- [Functions](#functions)
  - [Core Functions](#core-functions)
  - [Linguistic Analysis Functions](#linguistic-analysis-functions)
  - [Advanced Analysis Functions](#advanced-analysis-functions)
- [Error Handling](#error-handling)
- [Thread Safety](#thread-safety)
- [Performance Considerations](#performance-considerations)

## Types

### Token

```go
type Token struct {
    Text    string  // Original token text
    Lemma   string  // Base form of the word
    POS     string  // Universal POS tag
    Tag     string  // Language-specific detailed POS tag
    Dep     string  // Dependency relation label
    IsStop  bool    // Whether the token is a stop word
    IsPunct bool    // Whether the token is punctuation
}
```

The `Token` struct represents a single token from text analysis with comprehensive linguistic information.

**Fields:**
- **Text**: The original text of the token as it appears in the input
- **Lemma**: The canonical/base form of the word (e.g., "running" → "run")
- **POS**: Universal part-of-speech tag (NOUN, VERB, ADJ, etc.)
- **Tag**: Detailed, language-specific POS tag (NN, VBG, JJ, etc.)
- **Dep**: Syntactic dependency relation (nsubj, dobj, root, etc.)
- **IsStop**: True if the token is a common stop word like "the", "and"
- **IsPunct**: True if the token is punctuation like ".", "!", "?"

### Entity

```go
type Entity struct {
    Text  string  // Entity text
    Label string  // Entity type label
    Start int     // Start character position in original text
    End   int     // End character position in original text
}
```

The `Entity` struct represents a named entity extracted from text.

**Fields:**
- **Text**: The actual text of the entity
- **Label**: The entity type (PERSON, ORG, GPE, DATE, MONEY, etc.)
- **Start**: Character offset where the entity begins in the original text
- **End**: Character offset where the entity ends in the original text

**Common Entity Labels:**
- PERSON: People names
- ORG: Organizations, companies, agencies
- GPE: Geopolitical entities (countries, cities, states)
- DATE: Absolute or relative dates
- TIME: Times smaller than a day
- MONEY: Monetary values
- PERCENT: Percentage values
- CARDINAL: Cardinal numbers
- ORDINAL: Ordinal numbers

### NounChunk

```go
type NounChunk struct {
    Text     string  // Full noun phrase text
    RootText string  // Root token of the chunk
    RootDep  string  // Dependency relation of root token
    Start    int     // Start character offset
    End      int     // End character offset
}
```

The `NounChunk` struct represents a base noun phrase with its syntactic analysis.

**Fields:**
- **Text**: The complete text of the noun phrase
- **RootText**: The main noun that heads the phrase
- **RootDep**: The dependency relation of the root token
- **Start**: Character position where the chunk begins
- **End**: Character position where the chunk ends

**Example:** In "the quick brown fox", the root would be "fox" with dependency "nsubj" if it's the subject.

### VectorData

```go
type VectorData struct {
    Vector    []float64  // Dense vector representation
    HasVector bool       // Whether vectors are available
}
```

The `VectorData` struct contains word or document vector representations for semantic analysis.

**Fields:**
- **Vector**: Array of float64 values representing the semantic vector
- **HasVector**: Boolean indicating if the model supports vectors for this text

**Vector Dimensions by Model:**
- Small models (sm): No vectors (HasVector = false)
- Medium models (md): 300-dimensional GloVe vectors
- Large models (lg): 300-dimensional GloVe vectors
- Transformer models (trf): 768+ dimensional contextual vectors

### MorphFeature

```go
type MorphFeature struct {
    Key   string  // Feature name
    Value string  // Feature value
}
```

The `MorphFeature` struct represents detailed morphological information about words.

**Common Features:**
- **Number**: Sing (singular), Plur (plural)
- **Gender**: Masc (masculine), Fem (feminine), Neut (neuter)
- **Case**: Nom (nominative), Acc (accusative), Gen (genitive), Dat (dative)
- **Tense**: Past, Pres (present), Fut (future)
- **Mood**: Ind (indicative), Imp (imperative), Sub (subjunctive)
- **Person**: 1, 2, 3 (first, second, third person)
- **VerbForm**: Fin (finite), Inf (infinitive), Part (participle)

### NLP

```go
type NLP struct {
    // Private fields - access through methods only
}
```

The `NLP` struct represents a Spacy natural language processing pipeline instance. This is the main interface for all NLP operations.

**Usage Pattern:**
```go
nlp, err := spacy.NewNLP("en_core_web_sm")
if err != nil {
    log.Fatal(err)
}
defer nlp.Close()

// Use nlp for processing...
```

## Functions

### Core Functions

#### NewNLP

```go
func NewNLP(modelName string) (*NLP, error)
```

Creates a new NLP instance with the specified Spacy model.

**Parameters:**
- **modelName**: The name of the Spacy model to load (e.g., "en_core_web_sm")

**Returns:**
- **\*NLP**: Pointer to the NLP instance
- **error**: Error if model loading fails

**Common Models:**
- `en_core_web_sm`: Small English model (fast, no vectors)
- `en_core_web_md`: Medium English model (balanced, with vectors)
- `en_core_web_lg`: Large English model (accurate, with vectors)
- `de_core_news_sm`: German model
- `fr_core_news_sm`: French model

**Error Conditions:**
- Model not installed
- Model file corrupted
- Python/Spacy environment issues

#### Close

```go
func (n *NLP) Close()
```

Cleans up resources associated with the NLP instance. Must be called when done processing.

**Important:** Always use `defer nlp.Close()` immediately after successful `NewNLP()` call.

#### Tokenize

```go
func (n *NLP) Tokenize(text string) []Token
```

Tokenizes input text and returns detailed linguistic information for each token.

**Parameters:**
- **text**: Input text to tokenize

**Returns:**
- **[]Token**: Slice of Token structs with linguistic analysis

**Features:**
- Handles contractions ("don't" → "do", "n't")
- Identifies sentence boundaries
- Preserves original spacing information
- Language-specific tokenization rules

#### ExtractEntities

```go
func (n *NLP) ExtractEntities(text string) []Entity
```

Extracts and classifies named entities from text using statistical models.

**Parameters:**
- **text**: Input text to analyze

**Returns:**
- **[]Entity**: Slice of identified entities with labels and positions

**Accuracy:** Depends on model size and training data. Generally 85-95% F1 score for common entity types.

#### SplitSentences

```go
func (n *NLP) SplitSentences(text string) []string
```

Intelligently segments text into individual sentences using linguistic rules and statistical models.

**Parameters:**
- **text**: Input text to segment

**Returns:**
- **[]string**: Slice of sentence strings

**Features:**
- Handles abbreviations correctly (Dr. Smith)
- Recognizes decimal numbers (3.14)
- Processes multiple punctuation marks
- Language-specific sentence boundary rules

### Linguistic Analysis Functions

#### POSTag

```go
func (n *NLP) POSTag(text string) map[string]string
```

Assigns part-of-speech tags to all tokens using Universal Dependencies tagset.

**Parameters:**
- **text**: Input text to analyze

**Returns:**
- **map[string]string**: Map of token text to POS tag

**Universal POS Tags:**
- NOUN, VERB, ADJ, ADV, PRON, DET, ADP, NUM, CONJ, PRT, PUNCT, X

**Note:** If the same token appears multiple times with different POS tags, only the last occurrence is retained in the map.

#### GetDependencies

```go
func (n *NLP) GetDependencies(text string) map[string]string
```

Analyzes grammatical dependencies between words using dependency parsing.

**Parameters:**
- **text**: Input text to parse

**Returns:**
- **map[string]string**: Map of token text to dependency relation

**Common Relations:**
- **nsubj**: Nominal subject
- **dobj**: Direct object
- **root**: Root of the sentence
- **det**: Determiner
- **amod**: Adjectival modifier
- **prep**: Preposition

#### GetLemmas

```go
func (n *NLP) GetLemmas(text string) map[string]string
```

Reduces words to their base dictionary forms using morphological analysis.

**Parameters:**
- **text**: Input text to lemmatize

**Returns:**
- **map[string]string**: Map of token text to lemma

**Examples:**
- "running", "ran", "runs" → "run"
- "better", "best" → "good"
- "mice" → "mouse"

### Advanced Analysis Functions

#### GetNounChunks

```go
func (n *NLP) GetNounChunks(text string) []NounChunk
```

Extracts base noun phrases (noun chunks) with syntactic analysis.

**Parameters:**
- **text**: Input text to analyze

**Returns:**
- **[]NounChunk**: Slice of noun chunks with root analysis

**Use Cases:**
- Topic extraction
- Keyword identification
- Concept mining
- Subject/object identification

#### GetVector

```go
func (n *NLP) GetVector(text string) VectorData
```

Returns dense numerical vector representation for semantic analysis.

**Parameters:**
- **text**: Input text to vectorize (word, phrase, or document)

**Returns:**
- **VectorData**: Struct containing vector data and availability flag

**Requirements:**
- Medium or large models for meaningful vectors
- Small models return HasVector = false

**Applications:**
- Semantic similarity
- Text clustering
- Document classification
- Word analogies

#### Similarity

```go
func (n *NLP) Similarity(text1, text2 string) float64
```

Computes semantic similarity between two texts using vector representations.

**Parameters:**
- **text1**: First text for comparison
- **text2**: Second text for comparison

**Returns:**
- **float64**: Similarity score between 0.0 (dissimilar) and 1.0 (identical)

**Requirements:**
- Model with word vectors (medium, large, or transformer models)
- Small models use limited linguistic features

**Interpretation:**
- 0.9-1.0: Nearly identical meaning
- 0.7-0.9: Very similar
- 0.5-0.7: Somewhat similar
- 0.0-0.5: Different meanings

#### GetMorphology

```go
func (n *NLP) GetMorphology(text string) []MorphFeature
```

Extracts detailed morphological features for grammatical analysis.

**Parameters:**
- **text**: Input text to analyze

**Returns:**
- **[]MorphFeature**: Slice of morphological features found

**Language Variation:**
- Rich morphological languages (German, Russian) provide more features
- English has limited morphological complexity
- Feature names follow Universal Dependencies conventions

## Error Handling

### Common Errors

1. **Model Not Found**
   ```go
   nlp, err := spacy.NewNLP("invalid_model")
   if err != nil {
       // Handle: download model or use different model
   }
   ```

2. **Memory Issues**
   - Process very large texts in chunks
   - Monitor memory usage with profiling tools

3. **CGO Compilation Errors**
   - Ensure CGO_ENABLED=1
   - Install proper C++ compiler and Python development headers

### Best Practices

- Always check errors from `NewNLP()`
- Use `defer nlp.Close()` for resource cleanup
- Handle empty results gracefully
- Validate model availability before use

## Thread Safety

The NLP instance is **thread-safe** for concurrent read operations:

```go
nlp, _ := spacy.NewNLP("en_core_web_sm")
defer nlp.Close()

var wg sync.WaitGroup
texts := []string{"Text 1", "Text 2", "Text 3"}

for _, text := range texts {
    wg.Add(1)
    go func(t string) {
        defer wg.Done()
        tokens := nlp.Tokenize(t)  // Safe for concurrent use
        // Process tokens...
    }(text)
}
wg.Wait()
```

**Thread Safety Features:**
- Mutex protection for internal state
- Safe concurrent access to all methods
- No shared mutable state between operations

## Performance Considerations

### Initialization Cost
- Model loading is expensive (100-500ms)
- Reuse NLP instances across requests
- Consider connection pooling for web applications

### Processing Performance
- Small models: ~50K tokens/second
- Medium models: ~30K tokens/second
- Large models: ~15K tokens/second
- Transformer models: ~5K tokens/second

### Memory Usage
- Small models: ~50MB RAM
- Medium models: ~200MB RAM
- Large models: ~600MB RAM
- Transformer models: ~500MB RAM

### Optimization Tips
1. Choose appropriate model size for your use case
2. Process texts in batches when possible
3. Use goroutines for parallel processing
4. Monitor memory usage with `go test -benchmem`
5. Consider caching results for repeated texts

## Version Compatibility

This package is compatible with:
- Go 1.16+
- Python 3.7+
- Spacy 3.0+
- Any Spacy model following the standard format

## Model Requirements

Different features require different model capabilities:

| Feature | Small Models | Medium/Large Models | Required |
|---------|--------------|--------------------|---------|
| Tokenization | ✓ | ✓ | Any model |
| POS Tagging | ✓ | ✓ | Any model |
| NER | ✓ | ✓ | Any model |
| Dependency Parsing | ✓ | ✓ | Any model |
| Lemmatization | ✓ | ✓ | Any model |
| Noun Chunks | ✓ | ✓ | Any model |
| Word Vectors | ✗ | ✓ | Medium/Large/Transformer |
| Similarity | Limited | ✓ | Medium/Large/Transformer |
| Morphology | ✓ | ✓ | Any model |

Choose your model based on required features and performance constraints.