package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CloudflareEmbeddingRequest represents a request to Cloudflare API
type CloudflareEmbeddingRequest struct {
	Text string `json:"text"`
}

// CloudflareEmbeddingResponse represents response from Cloudflare API
type CloudflareEmbeddingResponse struct {
	Success   bool      `json:"success"`
	Timestamp string    `json:"timestamp"`
	Model     string    `json:"model"`
	Embedding []float32 `json:"embedding"`
}

// RequestTracker tracks daily API usage
type RequestTracker struct {
	mu          sync.RWMutex
	requests    int
	lastReset   time.Time
	maxRequests int
}

// NewRequestTracker creates a new request tracker
func NewRequestTracker(maxDaily int) *RequestTracker {
	now := time.Now()
	return &RequestTracker{
		requests:    0,
		lastReset:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		maxRequests: maxDaily,
	}
}

// NewRequestTrackerWithCount creates a new request tracker with an initial count
func NewRequestTrackerWithCount(maxDaily, initialCount int) *RequestTracker {
	now := time.Now()
	return &RequestTracker{
		requests:    initialCount,
		lastReset:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		maxRequests: maxDaily,
	}
}

// SetRequests manually sets the request count (for restoring from persistent storage)
func (rt *RequestTracker) SetRequests(count int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.requests = count
}

// CanMakeRequest checks if we can make a request (under daily limit)
func (rt *RequestTracker) CanMakeRequest() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Reset if it's a new day
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if today.After(rt.lastReset) {
		rt.requests = 0
		rt.lastReset = today
	}

	return rt.requests < rt.maxRequests
}

// IncrementRequest increments the request counter
func (rt *RequestTracker) IncrementRequest() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.requests++
}

// GetStats returns current usage statistics
func (rt *RequestTracker) GetStats() (int, int, time.Time) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.requests, rt.maxRequests, rt.lastReset
}

// HybridEmbeddingProvider provides both Cloudflare and Ollama embeddings
type HybridEmbeddingProvider struct {
	CloudflareURL  string
	OllamaURL      string
	Model          string
	HTTPClient     *http.Client
	RequestTracker *RequestTracker
	UseCloudflare  bool
}

// NewHybridEmbeddingProvider creates a new hybrid embeddings provider
func NewHybridEmbeddingProvider(cloudflareURL, ollamaURL, model string, maxDailyRequests int) *HybridEmbeddingProvider {
	return &HybridEmbeddingProvider{
		CloudflareURL: cloudflareURL,
		OllamaURL:     ollamaURL,
		Model:         model,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second, // Faster timeout for Cloudflare
		},
		RequestTracker: NewRequestTracker(maxDailyRequests),
		UseCloudflare:  true,
	}
}

// GetEmbedding gets embedding using the best available method
func (p *HybridEmbeddingProvider) GetEmbedding(text string) ([]float32, error) {
	// Clean text
	cleanText := strings.TrimSpace(text)
	if len(cleanText) == 0 {
		return nil, fmt.Errorf("empty text provided")
	}

	// Try Cloudflare first if enabled and we have quota
	if p.UseCloudflare && p.RequestTracker.CanMakeRequest() {
		embedding, err := p.getCloudflareEmbedding(cleanText)
		if err == nil {
			p.RequestTracker.IncrementRequest()
			log.Printf("Used Cloudflare embedding (quota: %d/%d)",
				p.RequestTracker.requests, p.RequestTracker.maxRequests)
			return embedding, nil
		}

		log.Printf("Cloudflare embedding failed: %v, falling back to Ollama", err)
		// Don't switch to Ollama permanently on temporary failures
	} else {
		if !p.UseCloudflare {
			log.Printf("Cloudflare embeddings disabled, using Ollama")
		} else {
			log.Printf("Cloudflare quota exhausted (%d/%d), switching to Ollama for rest of day",
				p.RequestTracker.requests, p.RequestTracker.maxRequests)
			// Temporarily disable Cloudflare for the rest of the day
			p.UseCloudflare = false
			// TODO: Could add a timer to re-enable at midnight
		}
	}

	// Fall back to Ollama
	return p.getOllamaEmbedding(cleanText)
}

// getCloudflareEmbedding gets embedding from Cloudflare API
func (p *HybridEmbeddingProvider) getCloudflareEmbedding(text string) ([]float32, error) {
	req := CloudflareEmbeddingRequest{
		Text: text,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Cloudflare request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", p.CloudflareURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make Cloudflare request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Cloudflare API returned status %d: %s", resp.StatusCode, string(body))
	}

	var cloudflareResp CloudflareEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&cloudflareResp); err != nil {
		return nil, fmt.Errorf("failed to decode Cloudflare response: %w", err)
	}

	if !cloudflareResp.Success {
		return nil, fmt.Errorf("Cloudflare API returned unsuccessful response")
	}

	if len(cloudflareResp.Embedding) == 0 {
		return nil, fmt.Errorf("Cloudflare API returned empty embedding")
	}

	return cloudflareResp.Embedding, nil
}

// getOllamaEmbedding gets embedding from Ollama API
func (p *HybridEmbeddingProvider) getOllamaEmbedding(text string) ([]float32, error) {
	// Use longer timeout for Ollama (CPU processing)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req := OllamaEmbeddingRequest{
		Model:  p.Model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.OllamaURL, bytes.NewBuffer(jsonData))
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

// GetBatchEmbeddings gets embeddings for multiple texts
func (p *HybridEmbeddingProvider) GetBatchEmbeddings(texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		embedding, err := p.GetEmbedding(text)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding for text %d: %w", i, err)
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// GetProviderStats returns current provider statistics
func (p *HybridEmbeddingProvider) GetProviderStats() map[string]interface{} {
	used, max, lastReset := p.RequestTracker.GetStats()
	return map[string]interface{}{
		"cloudflare_enabled":    p.UseCloudflare,
		"cloudflare_quota_used": used,
		"cloudflare_quota_max":  max,
		"cloudflare_last_reset": lastReset.Format("2006-01-02"),
		"remaining_quota":       max - used,
		"fallback_active":       !p.UseCloudflare,
	}
}

// LoadProviderConfig loads provider configuration from environment variables
func LoadProviderConfig() (cloudflareURL, ollamaURL string, maxDaily int) {
	cloudflareURL = os.Getenv("CLOUDFLARE_EMBEDDINGS_URL")
	if cloudflareURL == "" {
		cloudflareURL = "https://embeddings.knirv.com/embeddings"
	}

	ollamaURL = os.Getenv("OLLAMA_HOST")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434/api/embeddings"
	} else {
		ollamaURL = ollamaURL + "/api/embeddings"
	}

	maxDailyStr := os.Getenv("CLOUDFLARE_DAILY_LIMIT")
	if maxDailyStr == "" {
		maxDaily = 5000 // Default 5000 requests per day
	} else {
		if parsed, err := strconv.Atoi(maxDailyStr); err == nil && parsed > 0 {
			maxDaily = parsed
		}
	}

	return cloudflareURL, ollamaURL, maxDaily
}
