package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// OllamaEmbeddingRequest represents a request to the Ollama API
type OllamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// OllamaEmbeddingResponse represents a response from the Ollama API
type OllamaEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// GetOllamaEmbedding gets an embedding from a local Ollama instance
func GetOllamaEmbedding(text string, model string, host string) ([]float32, error) {
	// Use longer timeout for Ollama (CPU processing)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req := OllamaEmbeddingRequest{
		Model:  model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", host, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Use longer timeout for Ollama
	ollamaClient := &http.Client{
		Timeout: 90 * time.Second,
	}

	var ollamaResp OllamaEmbeddingResponse
	var lastErr error

	// Retry logic for Ollama
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			backoffDuration := time.Duration(retry) * 2 * time.Second
			log.Printf("Ollama request retry %d, waiting %v...", retry+1, backoffDuration)
			time.Sleep(backoffDuration)
		}

		resp, err := ollamaClient.Do(httpReq)
		if err != nil {
			lastErr = err
			log.Printf("Ollama request failed (attempt %d): %v", retry+1, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("Ollama returned status %d", resp.StatusCode)
			log.Printf("Ollama request failed (attempt %d): HTTP %d", retry+1, resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			log.Printf("Failed to read Ollama response (attempt %d): %v", retry+1, err)
			continue
		}

		if err := json.Unmarshal(body, &ollamaResp); err != nil {
			lastErr = err
			log.Printf("Failed to decode Ollama response (attempt %d): %v", retry+1, err)
			continue
		}

		if len(ollamaResp.Embedding) == 0 {
			lastErr = fmt.Errorf("received empty embedding")
			log.Printf("Empty Ollama embedding (attempt %d)", retry+1)
			continue
		}

		// Success
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to get Ollama embedding after 3 attempts: %w", lastErr)
	}

	log.Printf("Used Ollama embedding")
	return ollamaResp.Embedding, nil
}
