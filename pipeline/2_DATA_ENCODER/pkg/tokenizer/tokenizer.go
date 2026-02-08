package tokenizer

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
)

// Service wraps tiktoken for tokenization
type Service struct {
	model *tiktoken.Tiktoken
}

// New creates a new tokenizer service using cl100k_base encoding
func New() (*Service, error) {
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tokenizer: %w", err)
	}
	return &Service{model: tkm}, nil
}

// Encode converts string to token IDs
func (s *Service) Encode(text string) []int {
	if s.model == nil {
		return nil
	}
	return s.model.Encode(text, nil, nil)
}

// Decode converts token IDs back to string
func (s *Service) Decode(tokens []int) string {
	if s.model == nil {
		return ""
	}
	return s.model.Decode(tokens)
}

// Count returns the number of tokens in the text
func (s *Service) Count(text string) int {
	tokens := s.Encode(text)
	return len(tokens)
}
