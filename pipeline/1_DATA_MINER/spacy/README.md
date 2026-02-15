# Go-Spacy: Golang Bindings for Spacy NLP

[![Go Reference](https://pkg.go.dev/badge/github.com/am-sokolov/go-spacy.svg)](https://pkg.go.dev/github.com/am-sokolov/go-spacy)
[![Go Report Card](https://goreportcard.com/badge/github.com/am-sokolov/go-spacy)](https://goreportcard.com/report/github.com/am-sokolov/go-spacy)
[![CI](https://github.com/am-sokolov/go-spacy/actions/workflows/ci.yml/badge.svg)](https://github.com/am-sokolov/go-spacy/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/am-sokolov/go-spacy/branch/main/graph/badge.svg)](https://codecov.io/gh/am-sokolov/go-spacy)
[![Release](https://img.shields.io/github/v/release/am-sokolov/go-spacy)](https://github.com/am-sokolov/go-spacy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Go-Spacy provides high-performance Golang bindings for the Spacy Natural Language Processing library through an optimized C++ bridge layer. This enables you to leverage Spacy's powerful NLP capabilities directly from Go applications with minimal overhead.

## Features

### Core NLP Features
- **Tokenization**: Break text into individual tokens with linguistic attributes
- **Part-of-Speech (POS) Tagging**: Identify grammatical parts of speech with Universal POS tags
- **Named Entity Recognition (NER)**: Extract and classify named entities (PERSON, ORG, GPE, etc.)
- **Sentence Splitting**: Segment text into sentences intelligently
- **Dependency Parsing**: Analyze grammatical structure and relationships between words
- **Lemmatization**: Get base forms of words considering context and part-of-speech
- **Stop Words & Punctuation Detection**: Identify common words and punctuation marks

### Advanced NLP Features
- **Noun Chunk Extraction**: Extract base noun phrases with root analysis
- **Word Vectors**: Access dense vector representations for semantic analysis
- **Semantic Similarity**: Compute similarity between texts using word vectors
- **Morphological Analysis**: Get detailed grammatical features (gender, number, tense, etc.)
- **Multi-language Support**: Support for 12+ languages with language-specific models
- **Thread-Safe Operations**: Concurrent processing with mutex protection

## Prerequisites

- Go 1.16 or higher
- Python 3.9+ with Spacy installed
- C++ compiler (g++ or clang++)
- Make
- pkg-config (for Python detection)

## Installation

### 1. Install Python Dependencies

```bash
pip install spacy
python -m spacy download en_core_web_sm
```

### 2. Build the C++ Wrapper

```bash
make clean
make
```

### 3. Install the Go Package

```bash
go get github.com/am-sokolov/go-spacy
```

**🚀 Automatic Build**: The package includes an automatic build system that compiles the required C++ library during installation. If the automatic build fails, you can trigger it manually:

```bash
# Method 1: Use go generate
go generate github.com/am-sokolov/go-spacy

# Method 2: Run platform-specific installer
bash scripts/install.sh        # Linux/macOS
powershell scripts/install.ps1 # Windows

# Method 3: Use traditional make
make lib
```

For detailed build instructions, see [Automatic Build Guide](docs/AUTOMATIC_BUILD.md).

## Usage

### Basic Usage

```go
package main

import (
    "fmt"
    "log"
    "github.com/am-sokolov/go-spacy"
)

func main() {
    // Initialize NLP with a Spacy model
    nlp, err := spacy.NewNLP("en_core_web_sm")
    if err != nil {
        log.Fatal(err)
    }
    defer nlp.Close()

    text := "Apple Inc. was founded by Steve Jobs in California."

    // Tokenization
    tokens := nlp.Tokenize(text)
    for _, token := range tokens {
        fmt.Printf("Token: %s, POS: %s, Lemma: %s\n",
            token.Text, token.POS, token.Lemma)
    }

    // Named Entity Recognition
    entities := nlp.ExtractEntities(text)
    for _, entity := range entities {
        fmt.Printf("Entity: %s [%s]\n", entity.Text, entity.Label)
    }

    // Sentence Splitting
    sentences := nlp.SplitSentences(text)
    for _, sentence := range sentences {
        fmt.Println("Sentence:", sentence)
    }
}
```

### Advanced Usage

```go
package main

import (
    "fmt"
    "log"
    "github.com/am-sokolov/go-spacy"
)

func main() {
    // Use medium model for vector operations
    nlp, err := spacy.NewNLP("en_core_web_md")
    if err != nil {
        log.Fatal(err)
    }
    defer nlp.Close()

    text := "The quick brown fox jumps over the lazy dog."

    // Noun Chunk Extraction
    chunks := nlp.GetNounChunks(text)
    fmt.Println("Noun Chunks:")
    for _, chunk := range chunks {
        fmt.Printf("  '%s' (root: '%s', dep: %s)\n",
            chunk.Text, chunk.RootText, chunk.RootDep)
    }

    // Word Vectors
    vector := nlp.GetVector("fox")
    if vector.HasVector {
        fmt.Printf("Vector for 'fox': %d dimensions\n", len(vector.Vector))
    }

    // Semantic Similarity
    similarity := nlp.Similarity("dog", "puppy")
    fmt.Printf("Similarity between 'dog' and 'puppy': %.3f\n", similarity)

    // Morphological Analysis
    features := nlp.GetMorphology("The cats were running quickly.")
    fmt.Println("Morphological Features:")
    for _, feature := range features {
        fmt.Printf("  %s = %s\n", feature.Key, feature.Value)
    }

    // Dependency Parsing
    deps := nlp.GetDependencies(text)
    fmt.Println("Dependencies:")
    for token, dep := range deps {
        fmt.Printf("  '%s' -> %s\n", token, dep)
    }
}
```

## API Reference

### Types

#### Token
```go
type Token struct {
    Text    string  // Original token text
    Lemma   string  // Base form of the word
    POS     string  // Universal POS tag
    Tag     string  // Detailed POS tag
    Dep     string  // Dependency relation
    IsStop  bool    // Is it a stop word?
    IsPunct bool    // Is it punctuation?
}
```

#### Entity
```go
type Entity struct {
    Text  string  // Entity text
    Label string  // Entity type (PERSON, ORG, LOC, etc.)
    Start int     // Start position in text
    End   int     // End position in text
}
```

#### NounChunk
```go
type NounChunk struct {
    Text     string  // Full noun phrase text
    RootText string  // Root token of the chunk
    RootDep  string  // Dependency relation of root token
    Start    int     // Start character offset
    End      int     // End character offset
}
```

#### VectorData
```go
type VectorData struct {
    Vector    []float64  // Vector values (300-dimensional for medium/large models)
    HasVector bool       // Whether vectors are available for this model
}
```

#### MorphFeature
```go
type MorphFeature struct {
    Key   string  // Feature name (e.g., "Number", "Tense")
    Value string  // Feature value (e.g., "Sing", "Past")
}
```

### Core Functions

#### NewNLP
```go
func NewNLP(modelName string) (*NLP, error)
```
Creates a new NLP instance with the specified Spacy model.

#### Close
```go
func (n *NLP) Close()
```
Cleans up resources. Should be called when done using the NLP instance.

#### Tokenize
```go
func (n *NLP) Tokenize(text string) []Token
```
Tokenizes the input text and returns detailed token information.

#### ExtractEntities
```go
func (n *NLP) ExtractEntities(text string) []Entity
```
Extracts named entities from the text.

#### SplitSentences
```go
func (n *NLP) SplitSentences(text string) []string
```
Splits text into individual sentences.

### Linguistic Analysis Functions

#### POSTag
```go
func (n *NLP) POSTag(text string) map[string]string
```
Returns a map of tokens to their part-of-speech tags.

#### GetDependencies
```go
func (n *NLP) GetDependencies(text string) map[string]string
```
Returns a map of tokens to their dependency relations.

#### GetLemmas
```go
func (n *NLP) GetLemmas(text string) map[string]string
```
Returns a map of tokens to their lemmatized forms.

### Advanced Analysis Functions

#### GetNounChunks
```go
func (n *NLP) GetNounChunks(text string) []NounChunk
```
Extracts noun phrases with root analysis and position information.

#### GetVector
```go
func (n *NLP) GetVector(text string) VectorData
```
Returns the vector representation of text for semantic analysis.

#### Similarity
```go
func (n *NLP) Similarity(text1, text2 string) float64
```
Computes semantic similarity between two texts (requires models with vectors).

#### GetMorphology
```go
func (n *NLP) GetMorphology(text string) []MorphFeature
```
Returns detailed morphological features for tokens in the text.

## Running Tests

```bash
make test
```

## Running Example

```bash
make run-example
```

## Benchmarks

Run benchmarks to test performance:

```bash
go test -bench=. -benchmem
```

## Project Structure

```
.
├── cpp/
│   └── spacy_wrapper.cpp    # C++ wrapper for Spacy
├── include/
│   └── spacy_wrapper.h      # C interface header
├── lib/
│   └── libspacy_wrapper.so  # Compiled shared library
├── example/
│   └── main.go              # Example usage
├── spacy.go                 # Go bindings
├── spacy_test.go           # Test suite
├── Makefile                # Build configuration
└── README.md               # This file
```

## Supported Spacy Models

This binding works with any Spacy model. Models are categorized by size and language:

### English Models
- `en_core_web_sm` - Small model (13MB, no word vectors)
- `en_core_web_md` - Medium model (40MB, 300-dim word vectors)
- `en_core_web_lg` - Large model (560MB, 300-dim word vectors)
- `en_core_web_trf` - Transformer model (440MB, BERT-based)

### Multi-Language Models
- `de_core_news_sm` - German
- `fr_core_news_sm` - French
- `es_core_news_sm` - Spanish
- `it_core_news_sm` - Italian
- `pt_core_news_sm` - Portuguese
- `nl_core_news_sm` - Dutch
- `zh_core_web_sm` - Chinese
- `ja_core_news_sm` - Japanese

### Model Capabilities Comparison
| Feature | Small Models | Medium/Large Models | Transformer Models |
|---------|--------------|--------------------|--------------------|
| Tokenization | ✓ | ✓ | ✓ |
| POS Tagging | ✓ | ✓ | ✓ |
| NER | ✓ | ✓ | ✓ |
| Dependency Parsing | ✓ | ✓ | ✓ |
| Word Vectors | ✗ | ✓ (GloVe) | ✓ (Contextual) |
| Similarity | Limited | ✓ | ✓ |

Download models with:
```bash
python -m spacy download [model_name]

# Example
python -m spacy download en_core_web_md
python -m spacy download de_core_news_sm
```

## Performance

This package includes comprehensive benchmarks to help you understand performance characteristics:

### Running Benchmarks
```bash
# Run all benchmarks
go test -bench=. -benchmem

# Run specific benchmark categories
go test -bench=BenchmarkTokenize -benchmem
go test -bench=BenchmarkEntity -benchmem
go test -bench=BenchmarkAdvanced -benchmem

# Multi-language benchmarks (requires models)
go test -bench=BenchmarkMultiLanguage -benchmem
```

### Performance Tips
1. **Model Selection**: Small models are fastest but lack word vectors
2. **Reuse NLP Instances**: Model initialization is expensive (~100-500ms)
3. **Concurrent Processing**: The package is thread-safe for parallel processing
4. **Text Size**: Performance scales roughly linearly with text length
5. **Memory Usage**: Use the `-benchmem` flag to monitor memory allocation

### Typical Performance (en_core_web_sm)
- **Tokenization**: ~50,000 tokens/second
- **Entity Recognition**: ~30,000 tokens/second
- **Full Pipeline**: ~15,000 tokens/second

## Troubleshooting

### Python/Spacy Not Found

Ensure Python and Spacy are properly installed:
```bash
python --version
python -c "import spacy; print(spacy.__version__)"
```

### Build Errors

Check that Python development headers and pkg-config are installed:
```bash
# Ubuntu/Debian
sudo apt-get install python3-dev pkg-config

# macOS
brew install python3 pkg-config

# Check pkg-config detection
pkg-config --cflags --libs python3

# Fallback check
python3-config --cflags --ldflags
```

### Model Not Found

Download the required Spacy model:
```bash
python -m spacy download en_core_web_sm
```

### CGO Compilation Issues

If you encounter CGO-related compilation errors:
```bash
# Ensure CGO is enabled
export CGO_ENABLED=1

# Check your C++ compiler
g++ --version
clang++ --version

# Install build dependencies (Ubuntu/Debian)
sudo apt-get install build-essential python3-dev

# Install build dependencies (macOS)
xcode-select --install
```

### Runtime Errors

#### "shared library not found"
Make sure the shared library is built and in the correct location:
```bash
make clean && make
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:./lib
```

#### "Python module not found"
Verify your Python environment has Spacy installed:
```bash
python -c "import spacy; print('Spacy version:', spacy.__version__)"
```

#### Memory Issues with Large Texts
For processing very large texts (>10MB):
```go
// Process in chunks for large documents
func processLargeText(nlp *spacy.NLP, text string) {
    const chunkSize = 100000 // 100KB chunks
    for i := 0; i < len(text); i += chunkSize {
        end := i + chunkSize
        if end > len(text) {
            end = len(text)
        }
        chunk := text[i:end]
        // Process chunk...
    }
}
```

### Multi-Language Setup

When working with multiple language models:
```bash
# Install language models
python -m spacy download de_core_news_sm
python -m spacy download fr_core_news_sm
python -m spacy download es_core_news_sm

# Test model availability
go test -v -run TestModelAvailability
```

## Multi-Language Support

This package supports 12+ languages through their respective Spacy models:

### Language Support Matrix
| Language | Model | Features | Vectors | Status |
|----------|--------|----------|---------|---------|
| English | `en_core_web_*` | Full | ✓ (md/lg) | ✅ Fully Tested |
| German | `de_core_news_*` | Full | ✓ | ✅ Tested |
| French | `fr_core_news_*` | Full | ✓ | ✅ Tested |
| Spanish | `es_core_news_*` | Full | ✓ | ✅ Tested |
| Italian | `it_core_news_*` | Full | ✓ | ✅ Tested |
| Portuguese | `pt_core_news_*` | Full | ✓ | ✅ Tested |
| Dutch | `nl_core_news_*` | Full | ✓ | ✅ Tested |
| Chinese | `zh_core_web_*` | Full | ✓ | ✅ Tested |
| Japanese | `ja_core_news_*` | Full | ✓ | ✅ Tested |

### Language-Specific Examples
```go
// German processing
nlp_de, _ := spacy.NewNLP("de_core_news_sm")
defer nlp_de.Close()
chunks := nlp_de.GetNounChunks("Die Donaudampfschifffahrtsgesellschaft war sehr groß.")

// Chinese processing
nlp_zh, _ := spacy.NewNLP("zh_core_web_sm")
defer nlp_zh.Close()
entities := nlp_zh.ExtractEntities("北京是中国的首都。")

// Multi-language entity comparison
// See spacy_multilang_test.go for comprehensive examples
```

## License

This project is licensed under the MIT License.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Author

**Alexey Sokolov** - *Creator and Maintainer*

## Acknowledgments

- [Spacy](https://spacy.io/) - Industrial-strength Natural Language Processing
- Built with love for the Go community