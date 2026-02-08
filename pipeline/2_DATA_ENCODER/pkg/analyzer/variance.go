package analyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// VarianceOutputFile is where we save the high-variance indices
	VarianceOutputFile = "bge_signal_indices.json"
	// DefaultVarianceIndices are used if analysis hasn't been run yet
	DefaultVarianceIndices = "0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23"
)

// VarianceAnalyzer processes embeddings to identify high-variance dimensions
type VarianceAnalyzer struct {
	signalIndices  []int
	dimensionSum   []float64
	dimensionSumSq []float64
	sampleCount    int
	isAnalyzed     bool
}

// VarianceResult holds the output of variance analysis
type VarianceResult struct {
	SignalIndices []int     `json:"signal_indices"`
	Variances     []float32 `json:"variances"`
	SampleCount   int       `json:"sample_count"`
	ModelName     string    `json:"model_name"`
}

// NewVarianceAnalyzer creates a new analyzer
func NewVarianceAnalyzer() *VarianceAnalyzer {
	return &VarianceAnalyzer{
		signalIndices:  make([]int, 24),
		dimensionSum:   make([]float64, 768),
		dimensionSumSq: make([]float64, 768),
		sampleCount:    0,
		isAnalyzed:     false,
	}
}

// Sample adds an embedding to the analysis sample set
func (v *VarianceAnalyzer) Sample(embedding []float32) error {
	if len(embedding) != 768 {
		return fmt.Errorf("embedding must be 768 dimensions, got %d", len(embedding))
	}

	for i, val := range embedding {
		fv := float64(val)
		v.dimensionSum[i] += fv
		v.dimensionSumSq[i] += fv * fv
	}
	v.sampleCount++

	return nil
}

// SampleFromRecord processes a DocumentRecord or MinedRecord embedding
func (v *VarianceAnalyzer) SampleFromRecord(embedding interface{}) error {
	// Handle both []float32 and interface{} from JSON
	switch e := embedding.(type) {
	case []float32:
		return v.Sample(e)
	case []interface{}:
		floatEmbedding := make([]float32, len(e))
		for i, val := range e {
			floatEmbedding[i] = float32(val.(float64))
		}
		return v.Sample(floatEmbedding)
	default:
		return fmt.Errorf("unsupported embedding type: %T", embedding)
	}
}

// Calculate performs variance analysis on the collected samples
func (v *VarianceAnalyzer) Calculate() error {
	if v.sampleCount == 0 {
		return fmt.Errorf("no samples collected for variance analysis")
	}

	// Calculate variance for each dimension
	variances := make([]float32, 768)
	for i := 0; i < 768; i++ {
		mean := v.dimensionSum[i] / float64(v.sampleCount)
		// Variance = E[X^2] - (E[X])^2
		variance := (v.dimensionSumSq[i] / float64(v.sampleCount)) - (mean * mean)
		variances[i] = float32(variance)
	}

	// Find top 24 indices with highest variance
	v.signalIndices = FindTopVarianceIndices(variances, 24)
	v.isAnalyzed = true

	return nil
}

// GetSignalIndices returns the high-variance indices
func (v *VarianceAnalyzer) GetSignalIndices() []int {
	if !v.isAnalyzed {
		return ParseIndices(DefaultVarianceIndices)
	}
	return v.signalIndices
}

// SaveToFile saves the analysis results to a JSON file
func (v *VarianceAnalyzer) SaveToFile(outputPath string, modelName string) error {
	if !v.isAnalyzed {
		if err := v.Calculate(); err != nil {
			return err
		}
	}

	variances := make([]float32, 768)
	for i := 0; i < 768; i++ {
		mean := v.dimensionSum[i] / float64(v.sampleCount)
		variance := (v.dimensionSumSq[i] / float64(v.sampleCount)) - (mean * mean)
		variances[i] = float32(variance)
	}

	result := VarianceResult{
		SignalIndices: v.signalIndices,
		Variances:     variances,
		SampleCount:   v.sampleCount,
		ModelName:     modelName,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal variance result: %w", err)
	}

	return os.WriteFile(outputPath, data, 0644)
}

// LoadFromFile loads previously calculated variance indices
func (v *VarianceAnalyzer) LoadFromFile(inputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read variance file: %w", err)
	}

	var result VarianceResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to unmarshal variance result: %w", err)
	}

	v.signalIndices = result.SignalIndices
	v.isAnalyzed = true

	return nil
}

// GetOrCreateSignalIndices loads existing indices or returns defaults
func GetOrCreateSignalIndices(configDir string) ([]int, error) {
	varianceFile := filepath.Join(configDir, VarianceOutputFile)

	// Try to load existing analysis
	var analyzer VarianceAnalyzer
	if err := analyzer.LoadFromFile(varianceFile); err == nil {
		return analyzer.GetSignalIndices(), nil
	}

	// Return defaults if no analysis exists
	return ParseIndices(DefaultVarianceIndices), nil
}

// FindTopVarianceIndices finds the N indices with highest variance
func FindTopVarianceIndices(variances []float32, n int) []int {
	indices := make([]int, n)
	used := make(map[int]bool)

	for i := 0; i < n; i++ {
		maxVar := float32(-1.0)
		bestIdx := -1

		for j, v := range variances {
			if v > maxVar && !used[j] {
				maxVar = v
				bestIdx = j
			}
		}

		if bestIdx == -1 {
			break
		}

		indices[i] = bestIdx
		used[bestIdx] = true
	}

	return indices
}

// ParseIndices parses a comma-separated string of indices
func ParseIndices(s string) []int {
	indices := make([]int, 24)
	// Default case - just use sequential indices
	for i := 0; i < 24; i++ {
		indices[i] = i
	}
	return indices
}

// VarianceStats provides statistics about the variance analysis
type VarianceStats struct {
	SampleCount    int     `json:"sample_count"`
	TopVariance    float32 `json:"top_variance"`
	BottomVariance float32 `json:"bottom_variance"`
	MeanVariance   float32 `json:"mean_variance"`
	SignalIndices  []int   `json:"signal_indices"`
}

// GetStats returns statistics about the variance analysis
func (v *VarianceAnalyzer) GetStats() (*VarianceStats, error) {
	if !v.isAnalyzed {
		return nil, fmt.Errorf("variance analysis not yet performed")
	}

	variances := make([]float32, 768)
	var sum, top, bottom float32
	for i := 0; i < 768; i++ {
		mean := v.dimensionSum[i] / float64(v.sampleCount)
		variance := (v.dimensionSumSq[i] / float64(v.sampleCount)) - (mean * mean)
		variances[i] = float32(variance)
		sum += variances[i]
		if variances[i] > top || top == 0 {
			top = variances[i]
		}
		if variances[i] < bottom || bottom == 0 {
			bottom = variances[i]
		}
	}

	return &VarianceStats{
		SampleCount:    v.sampleCount,
		TopVariance:    top,
		BottomVariance: bottom,
		MeanVariance:   sum / 768,
		SignalIndices:  v.signalIndices,
	}, nil
}
