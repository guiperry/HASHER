package embedder

// OllamaEmbeddingRequest represents a request to Ollama API
type OllamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// OllamaEmbeddingResponse represents response from Ollama API
type OllamaEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}
