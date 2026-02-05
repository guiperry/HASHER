package tokenizer

import (
	"fmt"
	"strings"
)

// Tokenize converts a string into a slice of integer token IDs.
// It uses a simple character-level mapping, where each character's ASCII value
// is mapped to a token ID modulo vocabSize.
func Tokenize(input string, vocabSize int) []int {
	tokenIDs := make([]int, len(input))
	for i, char := range input {
		tokenIDs[i] = int(char) % vocabSize
	}
	return tokenIDs
}

// Detokenize converts a slice of integer token IDs back into a string.
// It assumes token IDs are ASCII values. Non-printable ASCII characters
// or out-of-range IDs are represented by a placeholder.
func Detokenize(tokenIDs []int) string {
	var result strings.Builder
	for _, id := range tokenIDs {
		if id >= 32 && id <= 126 { // Printable ASCII range
			result.WriteRune(rune(id))
		} else {
			// For non-printable or special tokens, print a placeholder
			result.WriteString(fmt.Sprintf("[%d]", id))
		}
	}
	return result.String()
}

// DetokenizeChar converts a single integer token ID into a string character.
// It assumes the token ID is an ASCII value. Non-printable ASCII characters
// or out-of-range IDs are represented by a placeholder "[?]"
func DetokenizeChar(id int) string {
	if id >= 32 && id <= 126 { // Printable ASCII range
		return fmt.Sprintf("%c", rune(id))
	}
	return "[?]"
}

// DetokenizeNonce converts a 32-bit Nonce into a character.
// It treats the Nonce as a raw token ID, optionally taking the modulo of the vocabSize,
// and then uses the existing character-level detokenization.
func DetokenizeNonce(nonce uint32, vocabSize int) string {
	// Use the nonce as a token ID, modulo vocabSize
	tokenID := int(nonce) % vocabSize
	return DetokenizeChar(tokenID)
}
