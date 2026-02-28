package main

import (
	"fmt"
	"hasher/pkg/hashing/tokenizer"
)

func main() {
	phrases := []string{
		"Hello HAsher",
		"Hello?",
		"Hello",
		"Hello ",
		"Hasher",
		"hasher",
	}

	for _, p := range phrases {
		tokens := tokenizer.Tokenize(p, 0)
		fmt.Printf("Phrase: %q\n", p)
		fmt.Printf("Tokens: %v\n", tokens)
		for _, t := range tokens {
			fmt.Printf("  %d -> %q\n", t, tokenizer.Detokenize([]int{t}))
		}
		fmt.Println("---")
	}
}
