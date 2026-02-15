package main

import (
	"fmt"
	"log"
	"github.com/am-sokolov/go-spacy"
)

func main() {
	nlp, err := spacy.NewNLP("en_core_web_sm")
	if err != nil {
		log.Fatal(err)
	}
	defer nlp.Close()

	tokens := nlp.Tokenize("Apple is looking at buying U.K. startup for $1 billion")
	for _, token := range tokens {
		fmt.Printf("%s\t%s\t%s\n", token.Text, token.Tag, token.Dep)
	}
}
