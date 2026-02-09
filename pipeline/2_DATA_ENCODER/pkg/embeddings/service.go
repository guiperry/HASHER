package embeddings

import (
	"bytes"
	"data-encoder/pkg/config"
	"data-encoder/pkg/ollama"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	svc := &Service{
		baseURL:     endpoint,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		batchSize:   DefaultBatchSize,
		model:       CloudflareWorkersBGE,
		ollamaHost:  "http://localhost:11434/api/embeddings",
		ollamaModel: "nomic-embed-text",
	}

	// Validate endpoint at initialization
	if endpoint != "" {
		if err := svc.ValidateEndpoint(); err != nil {
			log.Fatalf("❌ Embeddings endpoint validation failed: %v", err)
		}
		log.Printf("✅ Embeddings endpoint validated: %s", endpoint)
	}

	return svc
}

// NewWithBatchSize creates a new embeddings service with custom batch size
func NewWithBatchSize(batchSize int) *Service {
	config.LoadEnv()
	endpoint := config.GetCloudflareEndpoint()
	svc := &Service{
		baseURL:     endpoint,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		batchSize:   batchSize,
		model:       CloudflareWorkersBGE,
		ollamaHost:  "http://localhost:11434/api/embeddings",
		ollamaModel: "nomic-embed-text",
	}

	// Validate endpoint at initialization
	if endpoint != "" {
		if err := svc.ValidateEndpoint(); err != nil {
			log.Fatalf("❌ Embeddings endpoint validation failed: %v", err)
		}
		log.Printf("✅ Embeddings endpoint validated: %s", endpoint)
	}

	return svc
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

	// Use baseURL directly - model is specified in request body
	httpReq, err := http.NewRequest("POST", s.baseURL, bytes.NewBuffer(jsonBody))
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

// getBatchChunkWithRetry processes a batch without retries - fails immediately on error
func (s *Service) getBatchChunkWithRetry(texts []string, maxRetries int) ([][]float32, error) {
	embeddings, err := s.getBatchChunk(texts)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	return embeddings, nil
}

// GetBatchSize returns the configured batch size
func (s *Service) GetBatchSize() int {
	return s.batchSize
}

// ValidateEndpoint checks if the embeddings endpoint is accessible
func (s *Service) ValidateEndpoint() error {
	testReq := CloudflareWorkersRequest{
		Texts: []string{"test"},
	}

	jsonBody, err := json.Marshal(testReq)
	if err != nil {
		return fmt.Errorf("failed to marshal test request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", s.baseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Use a shorter timeout for validation
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SetTimeout sets the HTTP client timeout
func (s *Service) SetTimeout(timeout time.Duration) {
	s.httpClient.Timeout = timeout
}
