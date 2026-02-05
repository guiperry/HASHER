// internal/client/api.go
// Package client provides API client functionality for hasher-host
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIClient represents a client for hasher-host API
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewAPIClient creates a new API client
func NewAPIClient(port int) *APIClient {
	return &APIClient{
		BaseURL: fmt.Sprintf("http://localhost:%d", port),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CallInference calls the inference endpoint
func (c *APIClient) CallInference(input string) (*InferenceResponse, error) {
	req := map[string]interface{}{
		"data": input, // Will be base64 encoded by server
	}

	resp, err := c.post("/api/v1/infer", req)
	if err != nil {
		return nil, err
	}

	var result InferenceResponse
	if err := json.Unmarshal(*resp, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// CallTraining calls the training endpoint
func (c *APIClient) CallTraining(epochs int, learningRate float32, batchSize int, dataSamples []string) (*TrainingResponse, error) {
	req := map[string]interface{}{
		"epochs":        epochs,
		"learning_rate": learningRate,
		"batch_size":    batchSize,
		"data_samples":  dataSamples,
	}

	resp, err := c.post("/api/v1/train", req)
	if err != nil {
		return nil, err
	}

	var result TrainingResponse
	if err := json.Unmarshal(*resp, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// CallCryptoTransformer calls the crypto transformer chat endpoint
func (c *APIClient) CallCryptoTransformer(input string, context []int) (*ChatResponse, error) {
	req := map[string]interface{}{
		"message":     input,
		"temperature": 0.8,
		"context":     context,
	}

	resp, err := c.post("/api/v1/chat", req)
	if err != nil {
		return nil, err
	}

	var result ChatResponse
	if err := json.Unmarshal(*resp, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// GetHealth calls the health endpoint
func (c *APIClient) GetHealth() (*HealthResponse, error) {
	resp, err := c.get("/api/v1/health")
	if err != nil {
		return nil, err
	}

	var result HealthResponse
	if err := json.Unmarshal(*resp, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// post makes a POST request to the API
func (c *APIClient) post(endpoint string, data interface{}) (*json.RawMessage, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.HTTPClient.Post(
		c.BaseURL+endpoint,
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body first to provide better error messages
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to extract error message from response
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && (errResp.Error != "" || errResp.Message != "") {
			errMsg := errResp.Error
			if errMsg == "" {
				errMsg = errResp.Message
			}
			return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, errMsg)
		}
		// Truncate response for error message (avoid huge HTML dumps)
		preview := string(respBody)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, preview)
	}

	// Check content type to ensure we're getting JSON
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !bytes.Contains([]byte(contentType), []byte("json")) {
		preview := string(respBody)
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		return nil, fmt.Errorf("unexpected content type %q (expected JSON): %s", contentType, preview)
	}

	var result json.RawMessage
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Provide helpful context for decode errors
		preview := string(respBody)
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		return nil, fmt.Errorf("failed to decode JSON response: %w (response: %s)", err, preview)
	}

	return &result, nil
}

// get makes a GET request to the API
func (c *APIClient) get(endpoint string) (*json.RawMessage, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body first to provide better error messages
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to extract error message from response
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && (errResp.Error != "" || errResp.Message != "") {
			errMsg := errResp.Error
			if errMsg == "" {
				errMsg = errResp.Message
			}
			return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, errMsg)
		}
		// Truncate response for error message
		preview := string(respBody)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, preview)
	}

	var result json.RawMessage
	if err := json.Unmarshal(respBody, &result); err != nil {
		preview := string(respBody)
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		return nil, fmt.Errorf("failed to decode JSON response: %w (response: %s)", err, preview)
	}

	return &result, nil
}

// Response types
type InferenceResponse struct {
	Prediction        int     `json:"prediction"`
	Confidence        float64 `json:"confidence"`
	AverageConfidence float64 `json:"average_confidence"`
	Passes            int     `json:"passes"`
	ValidPasses       int     `json:"valid_passes"`
	LatencyMs         float64 `json:"latency_ms"`
	UsingASIC         bool    `json:"using_asic"`
}

type TrainingResponse struct {
	Epoch     int     `json:"epoch"`
	Loss      float32 `json:"loss"`
	Accuracy  float32 `json:"accuracy"`
	LatencyMs float64 `json:"latency_ms"`
	UsingASIC bool    `json:"using_asic"`
}

type ChatResponse struct {
	Response   string  `json:"response"`
	TokenID    int     `json:"token_id"`
	Confidence float32 `json:"confidence"`
	LatencyMs  float64 `json:"latency_ms"`
	UsingASIC  bool    `json:"using_asic"`
}

type HealthResponse struct {
	Status            string `json:"status"`
	UsingASIC         bool   `json:"using_asic"`
	ChipCount         int    `json:"chip_count"`
	Uptime            string `json:"uptime"`
	ConnectionHealthy bool   `json:"connection_healthy"`
	LastHealthCheck   string `json:"last_health_check,omitempty"`
}
