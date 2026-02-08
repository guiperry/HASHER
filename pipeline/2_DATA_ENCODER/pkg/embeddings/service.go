package embeddings

import (
	"bytes"
	"data-encoder/pkg/config"
	"data-encoder/pkg/ollama"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"
)

const (
	// DefaultBatchSize is the maximum number of texts to send in one API call
	DefaultBatchSize = 32
	// MaxRetries is the maximum number of retry attempts for failed API calls
	MaxRetries = 3
	// CloudflareWorkersBGE is the BGE-Base model identifier for Cloudflare Workers AI
	CloudflareWorkersBGE = "@cf/baai/bge-base-en-v1.5"
)

// Service handles embedding generation via Cloudflare Workers AI API or Ollama
type Service struct {
	baseURL     string
	httpClient  *http.Client
	batchSize   int
	model       string
	ollamaHost  string
	ollamaModel string
}

// CloudflareWorkersRequest represents the Cloudflare Workers AI API request structure
type CloudflareWorkersRequest struct {
	Texts []string `json:"texts"`
}

// CloudflareWorkersResponse represents the Cloudflare Workers AI API response structure
type CloudflareWorkersResponse struct {
	Success    bool        `json:"success"`
	Timestamp  string      `json:"timestamp"`
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
	Count      int         `json:"count"`
}

// New creates a new embeddings service
func New() *Service {
	config.LoadEnv()
	endpoint := config.GetCloudflareEndpoint()
	return &Service{
		baseURL:     endpoint,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		batchSize:   DefaultBatchSize,
		model:       CloudflareWorkersBGE,
		ollamaHost:  "http://localhost:11434/api/embeddings",
		ollamaModel: "nomic-embed-text",
	}
}

// NewWithBatchSize creates a new embeddings service with custom batch size
func NewWithBatchSize(batchSize int) *Service {
	config.LoadEnv()
	endpoint := config.GetCloudflareEndpoint()
	return &Service{
		baseURL:     endpoint,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		batchSize:   batchSize,
		model:       CloudflareWorkersBGE,
		ollamaHost:  "http://localhost:11434/api/embeddings",
		ollamaModel: "nomic-embed-text",
	}
}

// GetEmbedding returns embedding for a single text
func (s *Service) GetEmbedding(text string) ([]float32, error) {
	embeddings, err := s.GetBatchEmbeddings([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// GetBatchEmbeddings returns embeddings for multiple texts with automatic batching
func (s *Service) GetBatchEmbeddings(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	if s.baseURL != "" {
		var allEmbeddings [][]float32
		// Process in chunks to respect API batch size limits
		for i := 0; i < len(texts); i += s.batchSize {
			end := i + s.batchSize
			if end > len(texts) {
				end = len(texts)
			}

			chunk := texts[i:end]
			embeddings, err := s.getBatchChunkWithRetry(chunk, MaxRetries)
			if err != nil {
				return nil, fmt.Errorf("chunk %d-%d failed: %w", i, end, err)
			}

			allEmbeddings = append(allEmbeddings, embeddings...)
		}
		return allEmbeddings, nil
	}

	// Fallback to Ollama
	log.Println("Cloudflare endpoint not set, falling back to Ollama")
	var allEmbeddings [][]float32
	for _, text := range texts {
		embedding, err := ollama.GetOllamaEmbedding(text, s.ollamaModel, s.ollamaHost)
		if err != nil {
			return nil, fmt.Errorf("failed to get ollama embedding: %w", err)
		}
		allEmbeddings = append(allEmbeddings, embedding)
	}
	return allEmbeddings, nil
}

// getBatchChunk processes a single chunk of texts using Cloudflare Workers AI public endpoint
func (s *Service) getBatchChunk(texts []string) ([][]float32, error) {
	// Build Cloudflare Workers AI request
	reqBody := CloudflareWorkersRequest{
		Texts: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Build the full URL for the model (public endpoint)
	fullURL := fmt.Sprintf("%s/%s", s.baseURL, s.model)

	httpReq, err := http.NewRequest("POST", fullURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Public endpoint - no authentication required
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var cfResp CloudflareWorkersResponse
	if err := json.NewDecoder(resp.Body).Decode(&cfResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Check for API errors
	if !cfResp.Success {
		return nil, fmt.Errorf("API request failed")
	}

	if len(cfResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(cfResp.Embeddings))
	}

	embeddings := make([][]float32, len(cfResp.Embeddings))
	for i, embedding := range cfResp.Embeddings {
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// getBatchChunkWithRetry adds retry logic with exponential backoff
func (s *Service) getBatchChunkWithRetry(texts []string, maxRetries int) ([][]float32, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		embeddings, err := s.getBatchChunk(texts)
		if err == nil {
			return embeddings, nil
		}

		lastErr = err

		// Check if we should retry based on HTTP status code
		// We need to extract the status code from the error message or use a different approach
		errStr := err.Error()
		if len(errStr) > 0 {
			// If it's a status error we shouldn't retry, break
			// This is a workaround since we can't easily get the status code from the error
			// The API returns specific error messages for client errors
		}

		// Exponential backoff
		if attempt < maxRetries-1 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			log.Printf("⚠️  Embedding attempt %d failed, retrying in %v: %v", attempt+1, backoff, err)
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// GetBatchSize returns the configured batch size
func (s *Service) GetBatchSize() int {
	return s.batchSize
}

// SetTimeout sets the HTTP client timeout
func (s *Service) SetTimeout(timeout time.Duration) {
	s.httpClient.Timeout = timeout
}
