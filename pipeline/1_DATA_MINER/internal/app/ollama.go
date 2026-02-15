package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"data-miner/internal/embedder"
)

// CheckOrStartOllama checks if Ollama is running and starts it if necessary
func CheckOrStartOllama(host, model string) error {
	// First check if Ollama is running
	if isOllamaRunning(host) {
		return nil
	}

	// If we get here, Ollama is not running, try to start it
	fmt.Printf("🚀 Ollama not running at %s, attempting to start...\n", host)

	// Try to start Ollama using direct command
	cmd := exec.Command("ollama", "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start Ollama: %w", err)
	}

	// Give Ollama a moment to start and be ready
	return WaitForOllama(host, 30*time.Second)
}

// ConfigureOllamaEnvironment sets up optimal environment variables for Ollama
func ConfigureOllamaEnvironment(gpuOptimizations bool, gpuOverride bool, numWorkers int) {
	// Set optimal environment variables for Ollama (even on CPU)
	os.Setenv("OLLAMA_NUM_PARALLEL", "8")
	os.Setenv("OLLAMA_MAX_LOADED_MODELS", "1")
	os.Setenv("OLLAMA_MAX_QUEUE", "512")
	os.Setenv("OLLAMA_FLASH_ATTENTION", "1")
	os.Setenv("OLLAMA_KV_CACHE_TYPE", "f16")
	os.Setenv("OLLAMA_LOAD_TIMEOUT", "10m")

	// Check if GPU is available and set GPU-specific vars
	gpuAvailable := gpuOverride || isGPUAvailable()
	if gpuOptimizations && gpuAvailable {
		log.Println("🚀 GPU optimizations enabled")
		os.Setenv("CUDA_VISIBLE_DEVICES", "0")
		os.Setenv("OLLAMA_GPU_OVERHEAD", "1073741824")
		os.Setenv("OLLAMA_SCHED_SPREAD", "false")
	} else {
		log.Println("💻 Optimizing for CPU performance")
		os.Setenv("OLLAMA_GPU_OVERHEAD", "0")
		os.Setenv("OLLAMA_SCHED_SPREAD", "false")
	}

	log.Println("Environment variables configured:")
	log.Printf("  OLLAMA_NUM_PARALLEL: %s", os.Getenv("OLLAMA_NUM_PARALLEL"))
	log.Printf("  OLLAMA_MAX_LOADED_MODELS: %s", os.Getenv("OLLAMA_MAX_LOADED_MODELS"))
	log.Printf("  OLLAMA_FLASH_ATTENTION: %s", os.Getenv("OLLAMA_FLASH_ATTENTION"))
	log.Printf("  OLLAMA_KV_CACHE_TYPE: %s", os.Getenv("OLLAMA_KV_CACHE_TYPE"))
	log.Printf("  GPU_AVAILABLE: %t", gpuAvailable)
}

// WaitForOllama waits for Ollama to be ready with timeout
func WaitForOllama(host string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		if isOllamaRunning(host) {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("Ollama did not become ready within %v", timeout)
}

// StartOllamaWithOptimizations starts Ollama with environment optimizations
func StartOllamaWithOptimizations(host string, gpuOptimizations bool, gpuOverride bool, numWorkers int) error {
	// Configure environment variables
	ConfigureOllamaEnvironment(gpuOptimizations, gpuOverride, numWorkers)

	return CheckOrStartOllama(host, "")
}

// isOllamaRunning checks if Ollama is currently running
func isOllamaRunning(host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/api/tags", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetOllamaModels returns a list of all models available in Ollama
func GetOllamaModels(host string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/api/tags", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
	}

	// The API can return either {"models": [...]} or just [...]
	var body interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	var modelNames []string
	
	// Handle map format {"models": [...]}
	if m, ok := body.(map[string]interface{}); ok {
		if models, ok := m["models"].([]interface{}); ok {
			for _, model := range models {
				if modelMap, ok := model.(map[string]interface{}); ok {
					if name, ok := modelMap["name"].(string); ok {
						modelNames = append(modelNames, name)
					}
				}
			}
		}
	} else if models, ok := body.([]interface{}); ok {
		// Handle array format [...]
		for _, model := range models {
			if modelMap, ok := model.(map[string]interface{}); ok {
				if name, ok := modelMap["name"].(string); ok {
					modelNames = append(modelNames, name)
				}
			}
		}
	}

	return modelNames, nil
}

// GetOllamaModel checks if a specific model is available in Ollama
func GetOllamaModel(host, model string) (bool, error) {
	models, err := GetOllamaModels(host)
	if err != nil {
		return false, err
	}

	for _, m := range models {
		// Check for exact match or prefix (e.g. "llama3" matches "llama3:latest")
		if m == model || strings.HasPrefix(m, model+":") || (model == "llama3" && strings.HasPrefix(m, "llama3")) {
			return true, nil
		}
	}

	return false, nil
}

// PullOllamaModel pulls a model if it's not available
func PullOllamaModel(host, model string) error {
	available, err := GetOllamaModel(host, model)
	if err != nil {
		return fmt.Errorf("failed to check model availability: %w", err)
	}

	if available {
		log.Printf("✅ Model %s is already available", model)
		return nil
	}

	log.Printf("📥 Pulling model %s...", model)
	cmd := exec.Command("ollama", "pull", model)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull model %s: %w", model, err)
	}

	log.Printf("✅ Successfully pulled model %s", model)
	return nil
}

// GenerateAlpacaRecord transforms a text chunk into an Alpaca-styled record using prioritized providers:
// 1. OpenCode (via opencode run)
// 2. Ollama (if running)
// 3. SpaCy (deterministic extraction)
func GenerateAlpacaRecord(ctx context.Context, host, model, text string, nlpBridge *NLPBridge) (*AlpacaRecord, error) {
	// Try OpenCode first as it's the prioritized primary provider
	record, err := GenerateAlpacaRecordOpenCode(ctx, text)
	if err == nil {
		return record, nil
	}
	log.Printf("⚠️  OpenCode Alpaca generation failed: %v. Trying Ollama...", err)

	// Try Ollama second
	record, err = GenerateAlpacaRecordOllama(ctx, host, model, text)
	if err == nil {
		return record, nil
	}
	log.Printf("⚠️  Ollama Alpaca generation failed: %v. Falling back to deterministic SpaCy...", err)

	// Fallback to deterministic SpaCy
	return GenerateAlpacaRecordSpaCy(text, nlpBridge)
}

// GenerateAlpacaRecordOllama transforms a text chunk using Ollama
func GenerateAlpacaRecordOllama(ctx context.Context, host, model, text string) (*AlpacaRecord, error) {
	prompt := fmt.Sprintf(`Transform the following text into a single Alpaca-styled instruction record. 
The record must have "instruction", "input", and "output" fields.
- "instruction": A task or question derived from the text.
- "input": The relevant context or supporting information from the text.
- "output": The answer or completion based on the text.

Respond ONLY with a valid JSON object.

Text:
%s`, text)

	request := struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Format string `json:"format"`
		Stream bool   `json:"stream"`
	}{
		Model:  model,
		Prompt: prompt,
		Format: "json",
		Stream: false,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/generate", host)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	var response struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var record AlpacaRecord
	if err := json.Unmarshal([]byte(response.Response), &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal alpaca record: %w. Response: %s", err, response.Response)
	}

	return &record, nil
}

// GenerateAlpacaRecordOpenCode transforms a text chunk using the opencode run command
func GenerateAlpacaRecordOpenCode(ctx context.Context, text string) (*AlpacaRecord, error) {
	prompt := fmt.Sprintf(`Transform the following text into a single Alpaca-styled instruction record. 
The record must have "instruction", "input", and "output" fields.
- "instruction": A task or question derived from the text.
- "input": The relevant context or supporting information from the text.
- "output": The answer or completion based on the text.

Respond ONLY with a valid JSON object.

Text:
%s`, text)

	// Use opencode run command
	cmd := exec.CommandContext(ctx, "opencode", "run", prompt)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("opencode run failed: %w (output: %s)", err, string(output))
	}

	// Filter out status lines like "> build · big-pickle"
	lines := strings.Split(string(output), "\n")
	var jsonStr string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || jsonStr != "" {
			jsonStr += line
		}
	}

	// Try to extract JSON if there's other text
	start := strings.Index(jsonStr, "{")
	end := strings.LastIndex(jsonStr, "}")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("failed to find JSON in opencode output: %s", jsonStr)
	}
	jsonStr = jsonStr[start : end+1]

	var record AlpacaRecord
	if err := json.Unmarshal([]byte(jsonStr), &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal opencode alpaca record: %w. Output: %s", err, jsonStr)
	}

	return &record, nil
}

// GenerateAlpacaRecordSpaCy creates a deterministic Alpaca record using NLP features
func GenerateAlpacaRecordSpaCy(text string, nlpBridge *NLPBridge) (*AlpacaRecord, error) {
	if nlpBridge == nil {
		return &AlpacaRecord{
			Instruction: "Analyze the provided text fragment.",
			Input:       text,
			Output:      "The text contains information about linguistic patterns.",
		}, nil
	}

	// Deterministic extraction
	tokens, _, posTags, _, _ := nlpBridge.ProcessText(text)
	
	// Count nouns and verbs
	var nouns, verbs []string
	for i, token := range tokens {
		if posTags[i] == 0x01 { // NOUN
			nouns = append(nouns, token)
		} else if posTags[i] == 0x02 { // VERB
			verbs = append(verbs, token)
		}
	}

	instruction := "Analyze this text fragment and identify key concepts."
	if len(verbs) > 0 {
		instruction = fmt.Sprintf("Analyze the action described by: %s", strings.Join(verbs[:min(3, len(verbs))], ", "))
	}

	output := "The text fragment discusses several concepts."
	if len(nouns) > 0 {
		output = fmt.Sprintf("Key concepts identified include: %s.", strings.Join(nouns[:min(5, len(nouns))], ", "))
	}

	return &AlpacaRecord{
		Instruction: instruction,
		Input:       text,
		Output:      output,
	}, nil
}

// TestEmbeddingsEndpoint tests the embeddings endpoint with a sample request
func TestEmbeddingsEndpoint(host, model string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/api/embeddings", host)
	request := embedder.OllamaEmbeddingRequest{
		Model:  model,
		Prompt: "test embedding generation check",
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal test request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to test embeddings endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embeddings endpoint returned status %d", resp.StatusCode)
	}

	return nil
}

// IsOpenCodeRunning checks if the OpenCode server is responsive on port 5500
func IsOpenCodeRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:5500")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
