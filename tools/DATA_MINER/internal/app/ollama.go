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

	"dataminer/internal/embedder"
)

// CheckOrStartOllama checks if Ollama is running and starts it if necessary
func CheckOrStartOllama(host, model string) error {
	// First check if Ollama is running
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/api/tags", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create Ollama check request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		// Ollama is running, test embeddings endpoint
		resp.Body.Close()

		// Test embeddings endpoint with more patient timeout
		embedCtx, embedCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer embedCancel()

		embedURL := fmt.Sprintf("%s/api/embeddings", host)
		embedReq := embedder.OllamaEmbeddingRequest{
			Model:  model,
			Prompt: "test embedding generation check",
		}

		jsonData, _ := json.Marshal(embedReq)
		embedHttpReq, err := http.NewRequestWithContext(embedCtx, "POST", embedURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to test embeddings endpoint: %w", err)
		}
		embedHttpReq.Header.Set("Content-Type", "application/json")

		embedClient := &http.Client{Timeout: 25 * time.Second}
		resp2, err := embedClient.Do(embedHttpReq)
		if err != nil {
			return fmt.Errorf("failed to test embeddings endpoint: %w", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			return fmt.Errorf("embeddings endpoint returned status %d", resp2.StatusCode)
		}

		return nil
	}

	// If we get here, Ollama is not running, try to start it
	fmt.Printf("🚀 Ollama not running at %s, attempting to start...\n", host)

	// Try to start Ollama using go-ollama equivalent (direct command)
	cmd := exec.Command("ollama", "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start Ollama: %w", err)
	}

	// Give Ollama a moment to start
	time.Sleep(3 * time.Second)

	// Check again if it's running now
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	req2, err := http.NewRequestWithContext(ctx2, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create second Ollama check request: %w", err)
	}

	resp2, err := client.Do(req2)
	if err != nil {
		return fmt.Errorf("failed to check Ollama after starting: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama failed to start properly")
	}

	fmt.Printf("✅ Ollama started successfully at %s\n", host)
	return nil
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, "GET", host+"/api/tags", nil)
		cancel()

		if err == nil {
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("Ollama did not become ready within %v", timeout)
}

// StartOllamaWithOptimizations starts Ollama with environment optimizations
func StartOllamaWithOptimizations(host string, gpuOptimizations bool, gpuOverride bool, numWorkers int) error {
	// Configure environment variables
	ConfigureOllamaEnvironment(gpuOptimizations, gpuOverride, numWorkers)

	// Check if Ollama is already running
	if isOllamaRunning(host) {
		log.Println("✅ Ollama is already running")
		return nil
	}

	// Start Ollama if not running
	log.Println("🚀 Starting Ollama with optimizations...")
	cmd := exec.Command("ollama", "serve")

	// Create log file for Ollama output
	logFile, err := os.Create("/tmp/ollama_start.log")
	if err != nil {
		log.Printf("Warning: Could not create log file: %v", err)
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		defer logFile.Close()
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start Ollama: %w", err)
	}

	// Wait for Ollama to be ready
	log.Println("⏳ Waiting for Ollama to be ready...")
	err = WaitForOllama(host, 30*time.Second)
	if err != nil {
		return fmt.Errorf("Ollama failed to start: %w (check /tmp/ollama_start.log for details)", err)
	}

	log.Printf("✅ Ollama started successfully at %s", host)
	return nil
}

// isOllamaRunning checks if Ollama is currently running
func isOllamaRunning(host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", host+"/api/tags", nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetOllamaModel checks if a specific model is available in Ollama
func GetOllamaModel(host, model string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", host+"/api/tags", nil)
	if err != nil {
		return false, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
	}

	var tagsResponse struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tagsResponse); err != nil {
		return false, err
	}

	for _, m := range tagsResponse.Models {
		if strings.HasPrefix(m.Name, model) {
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
