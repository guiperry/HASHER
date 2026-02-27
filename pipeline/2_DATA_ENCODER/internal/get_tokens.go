package internal

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
)

func GetTokens() error {
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return err
	}

	texts := []string{
		"Hello", " world", "!",
		"What is", " your", " name", "?",
		"My name", " is", " Hasher", ".",
		"How are", " you", " today", "?",
		"I am", " doing", " well", ".",
	}

	for _, t := range texts {
		tokens := tkm.Encode(t, nil, nil)
		fmt.Printf("TEXT: %s TOKENS: %v\n", t, tokens)
	}

	return nil
}
