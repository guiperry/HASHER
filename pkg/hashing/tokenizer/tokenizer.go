package tokenizer

import (
	"fmt"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// Tokenize converts a string into a slice of integer token IDs using Tiktoken (cl100k_base).
// This matches the tokenization used in the data pipeline.
func Tokenize(input string, vocabSize int) []int {
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Fallback to character-level if tiktoken fails
		tokenIDs := make([]int, len(input))
		for i, char := range input {
			tokenIDs[i] = int(char)
		}
		return tokenIDs
	}

	tokens := tkm.Encode(input, nil, nil)
	return tokens
}

// Detokenize converts a slice of integer token IDs back into a string.
func Detokenize(tokenIDs []int) string {
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Fallback to ASCII
		var result strings.Builder
		for _, id := range tokenIDs {
			if id >= 32 && id <= 126 {
				result.WriteRune(rune(id))
			} else {
				result.WriteString(fmt.Sprintf("[%d]", id))
			}
		}
		return result.String()
	}

	return tkm.Decode(tokenIDs)
}

// DetokenizeChar converts a single integer token ID into a string character.
func DetokenizeChar(id int) string {
	return Detokenize([]int{id})
}

// DetokenizeNonce converts a 32-bit Nonce into a character/word.
func DetokenizeNonce(nonce uint32, vocabSize int) string {
	tokenID := int(nonce)
	if vocabSize > 0 {
		tokenID = tokenID % vocabSize
	}
	return DetokenizeChar(tokenID)
}
