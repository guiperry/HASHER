package app

import (
	"github.com/am-sokolov/go-spacy"
	"hash/fnv"
	"strings"
)

// NLPBridge handles linguistic metadata extraction
type NLPBridge struct {
	nlp *spacy.NLP
}

// NewNLPBridge initializes the NLP Bridge with a SpaCy model
func NewNLPBridge() (bridge *NLPBridge, err error) {
	// Using the small model as it's fastest and sufficient for POS/Dep
	nlp, nlpErr := spacy.NewNLP("en_core_web_sm")
	if nlpErr != nil {
		return nil, nlpErr
	}

	// Test the NLP bridge with a simple text to ensure it works
	func() {
		defer func() {
			if r := recover(); r != nil {
				// If the test crashes, close the nlp and set error
				nlp.Close()
				err = nlpErr
			}
		}()
		// Quick health check
		_ = nlp.Tokenize("test")
	}()

	if err != nil {
		return nil, err
	}

	return &NLPBridge{nlp: nlp}, nil
}

// Close releases SpaCy resources
func (nb *NLPBridge) Close() {
	if nb.nlp != nil {
		nb.nlp.Close()
	}
}

// ProcessText extracts linguistic features from text
func (nb *NLPBridge) ProcessText(text string) (words []string, offsets []int32, posTags []uint8, tenses []uint8, depHashes []uint32) {
	// Limit text size to prevent memory issues with spaCy
	const maxTextLength = 50000
	if len(text) > maxTextLength {
		text = text[:maxTextLength]
	}

	// Recover from any panics in the CGO/spaCy code
	defer func() {
		if r := recover(); r != nil {
			// Return empty values on panic but don't crash
			words = nil
			offsets = nil
			posTags = nil
			tenses = nil
			depHashes = nil
		}
	}()

	if nb == nil || nb.nlp == nil {
		return nil, nil, nil, nil, nil
	}

	tokens := nb.nlp.Tokenize(text)

	words = make([]string, len(tokens))
	offsets = make([]int32, len(tokens))
	posTags = make([]uint8, len(tokens))
	tenses = make([]uint8, len(tokens))
	depHashes = make([]uint32, len(tokens))

	for i, t := range tokens {
		words[i] = t.Text
		offsets[i] = int32(t.Start)
		posTags[i] = MapPOSTag(t.POS)
		depHashes[i] = HashDependency(t.Dep)
		tenses[i] = 0
	}

	return
}

// MapPOSTag converts Spacy POS strings to uint8 IDs (from DATA-MAPPER.md spec)
func MapPOSTag(pos string) uint8 {
	switch strings.ToUpper(pos) {
	case "NOUN":
		return 0x01
	case "VERB":
		return 0x02
	case "ADJ":
		return 0x03
	case "ADV":
		return 0x04
	case "PRON":
		return 0x05
	case "PROPN":
		return 0x06
	case "DET":
		return 0x07
	case "ADP":
		return 0x08
	case "NUM":
		return 0x09
	case "CONJ", "CCONJ":
		return 0x0A
	case "SCONJ":
		return 0x0B
	case "AUX":
		return 0x0C
	case "PART":
		return 0x0D
	case "INTJ":
		return 0x0E
	case "PUNCT":
		return 0x0F
	case "SYM":
		return 0x10
	case "X":
		return 0x11
	case "SPACE":
		return 0x12
	default:
		return 0x00
	}
}

// HashDependency creates a 32-bit hash for the dependency link
func HashDependency(dep string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(dep))
	return h.Sum32()
}
