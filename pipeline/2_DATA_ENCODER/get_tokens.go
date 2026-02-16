package main

import (
	"fmt"
	"github.com/pkoukk/tiktoken-go"
)

func main() {
	tkm, _ := tiktoken.GetEncoding("cl100k_base")
	
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
}
