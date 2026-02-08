package validator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type ValidatorConfig struct {
	Timeout            time.Duration `json:"timeout"`
	MaxConcurrency     int           `json:"max_concurrency"`
	RetryAttempts      int           `json:"retry_attempts"`
	ToleranceThreshold float64       `json:"tolerance_threshold"`
	EnableASIC         bool          `json:"enable_asic"`
}

type SeedValidationRequest struct {
	Seed        []byte `json:"seed"`
	TargetToken int32  `json:"target_token"`
	SeedID      uint32 `json:"seed_id"`
}

type ValidationResult struct {
	SeedID       uint32  `json:"seed_id"`
	GPUOutput    uint32  `json:"gpu_output"`
	ASICOutput   uint32  `json:"asic_output"`
	IsConsistent bool    `json:"is_consistent"`
	ErrorMargin  float64 `json:"error_margin"`
	IsValid      bool    `json:"is_valid"`
	GPULatency   float64 `json:"gpu_latency"`
	ASILatency   float64 `json:"asic_latency"`
	ErrorMessage string  `json:"error_message,omitempty"`
}

type ValidatorStats struct {
	TotalValidations   int64     `json:"total_validations"`
	SuccessfulMatches  int64     `json:"successful_matches"`
	FailedMatches      int64     `json:"failed_matches"`
	AverageGPULatency  float64   `json:"average_gpu_latency"`
	AverageASILatency  float64   `json:"average_asic_latency"`
	ConsistencyRate    float64   `json:"consistency_rate"`
	LastValidationTime time.Time `json:"last_validation_time"`
}

type Validator interface {
	ValidateSeed(ctx context.Context, seed []byte, targetToken int32) (*ValidationResult, error)
	BatchValidate(ctx context.Context, seeds []SeedValidationRequest) ([]ValidationResult, error)
	GetValidatorStats() (*ValidatorStats, error)
	Close() error
}

type CrossHardwareValidator struct {
	simulator HashSimulator
	config    *ValidatorConfig
	mutex     sync.RWMutex
	stats     *ValidatorStats
	isRunning bool
}

type HashSimulator interface {
	SimulateHash(seed []byte, pass int) (uint32, error)
	ValidateSeed(seed []byte, targetToken int32) (bool, error)
}

func NewCrossHardwareValidator(simulator HashSimulator, config *ValidatorConfig) *CrossHardwareValidator {
	if config == nil {
		config = &ValidatorConfig{
			Timeout:            30 * time.Second,
			MaxConcurrency:     10,
			RetryAttempts:      3,
			ToleranceThreshold: 0.01,
			EnableASIC:         false,
		}
	}

	return &CrossHardwareValidator{
		simulator: simulator,
		config:    config,
		stats: &ValidatorStats{
			LastValidationTime: time.Now(),
		},
		isRunning: false,
	}
}

func (chv *CrossHardwareValidator) Initialize() error {
	chv.mutex.Lock()
	defer chv.mutex.Unlock()

	if chv.simulator == nil {
		return fmt.Errorf("simulator is required")
	}

	chv.isRunning = true
	return nil
}

func (chv *CrossHardwareValidator) ValidateSeed(ctx context.Context, seed []byte, targetToken int32) (*ValidationResult, error) {
	if !chv.isRunning {
		return nil, fmt.Errorf("validator is not running")
	}

	result := &ValidationResult{
		SeedID:       0,
		IsValid:      false,
		IsConsistent: false,
	}

	ctx, cancel := context.WithTimeout(ctx, chv.config.Timeout)
	defer cancel()

	start := time.Now()
	gpuOutput, err := chv.simulator.SimulateHash(seed, 20)
	if err != nil {
		return nil, fmt.Errorf("GPU simulation failed: %w", err)
	}
	result.GPUOutput = gpuOutput
	result.GPULatency = time.Since(start).Seconds()

	if chv.config.EnableASIC {
		start = time.Now()
		asicOutput, err := chv.simulator.SimulateHash(seed, 20)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("ASIC simulation failed: %v", err)
			asicOutput = gpuOutput
		}
		result.ASICOutput = asicOutput
		result.ASILatency = time.Since(start).Seconds()
	} else {
		result.ASICOutput = gpuOutput
		result.ASILatency = result.GPULatency
	}

	result.IsConsistent = (result.GPUOutput == result.ASICOutput)

	if result.GPUOutput == uint32(targetToken) {
		result.IsValid = true
	}

	result.ErrorMargin = chv.calculateErrorMargin(result.GPUOutput, result.ASICOutput)

	chv.updateStats(result)

	return result, nil
}

func (chv *CrossHardwareValidator) BatchValidate(ctx context.Context, requests []SeedValidationRequest) ([]ValidationResult, error) {
	if len(requests) == 0 {
		return []ValidationResult{}, nil
	}

	semaphore := make(chan struct{}, chv.config.MaxConcurrency)
	results := make([]ValidationResult, len(requests))
	errors := make([]error, len(requests))

	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		go func(idx int, request SeedValidationRequest) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := chv.ValidateSeed(ctx, request.Seed, request.TargetToken)
			if err != nil {
				errors[idx] = err
				result = &ValidationResult{
					SeedID:       request.SeedID,
					ErrorMessage: err.Error(),
					IsValid:      false,
					IsConsistent: false,
				}
			} else {
				result.SeedID = request.SeedID
			}

			results[idx] = *result
		}(i, req)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("validation %d failed: %w", i, err)
		}
	}

	return results, nil
}

func (chv *CrossHardwareValidator) calculateErrorMargin(gpuOutput, asicOutput uint32) float64 {
	if gpuOutput == asicOutput {
		return 0.0
	}

	diff := abs(int64(gpuOutput) - int64(asicOutput))
	maxV := max(uint64(gpuOutput), uint64(asicOutput))

	if maxV == 0 {
		return 0.0
	}

	return float64(diff) / float64(maxV)
}

func (chv *CrossHardwareValidator) updateStats(result *ValidationResult) {
	chv.mutex.Lock()
	defer chv.mutex.Unlock()

	chv.stats.TotalValidations++

	if result.IsValid {
		chv.stats.SuccessfulMatches++
	} else {
		chv.stats.FailedMatches++
	}

	if result.GPULatency > 0 {
		chv.stats.AverageGPULatency = (chv.stats.AverageGPULatency*0.9 + result.GPULatency*0.1)
	}

	if result.ASILatency > 0 {
		chv.stats.AverageASILatency = (chv.stats.AverageASILatency*0.9 + result.ASILatency*0.1)
	}

	if chv.stats.TotalValidations > 0 {
		chv.stats.ConsistencyRate = float64(chv.stats.SuccessfulMatches) / float64(chv.stats.TotalValidations)
	}

	chv.stats.LastValidationTime = time.Now()
}

func (chv *CrossHardwareValidator) GetValidatorStats() (*ValidatorStats, error) {
	chv.mutex.RLock()
	defer chv.mutex.RUnlock()

	return &ValidatorStats{
		TotalValidations:   chv.stats.TotalValidations,
		SuccessfulMatches:  chv.stats.SuccessfulMatches,
		FailedMatches:      chv.stats.FailedMatches,
		AverageGPULatency:  chv.stats.AverageGPULatency,
		AverageASILatency:  chv.stats.AverageASILatency,
		ConsistencyRate:    chv.stats.ConsistencyRate,
		LastValidationTime: chv.stats.LastValidationTime,
	}, nil
}

func (chv *CrossHardwareValidator) Close() error {
	chv.mutex.Lock()
	defer chv.mutex.Unlock()

	chv.isRunning = false
	return nil
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func max(x, y uint64) uint64 {
	if x > y {
		return x
	}
	return y
}

type MockValidator struct {
	config *ValidatorConfig
	stats  *ValidatorStats
}

func NewMockValidator(config *ValidatorConfig) *MockValidator {
	if config == nil {
		config = &ValidatorConfig{}
	}
	return &MockValidator{
		config: config,
		stats: &ValidatorStats{
			LastValidationTime: time.Now(),
		},
	}
}

func (mv *MockValidator) ValidateSeed(ctx context.Context, seed []byte, targetToken int32) (*ValidationResult, error) {
	output := uint32(seed[0]) | uint32(seed[1])<<8 | uint32(seed[2])<<16 | uint32(seed[3])<<24

	result := &ValidationResult{
		SeedID:       0,
		GPUOutput:    output,
		ASICOutput:   output,
		IsConsistent: true,
		ErrorMargin:  0.0,
		IsValid:      output == uint32(targetToken),
		GPULatency:   0.001,
		ASILatency:   0.0005,
	}

	mv.updateStats(result)
	return result, nil
}

func (mv *MockValidator) BatchValidate(ctx context.Context, requests []SeedValidationRequest) ([]ValidationResult, error) {
	results := make([]ValidationResult, len(requests))

	for i, req := range requests {
		result, err := mv.ValidateSeed(ctx, req.Seed, req.TargetToken)
		if err != nil {
			return nil, err
		}
		result.SeedID = req.SeedID
		results[i] = *result
	}

	return results, nil
}

func (mv *MockValidator) updateStats(result *ValidationResult) {
	mv.stats.TotalValidations++

	if result.IsValid {
		mv.stats.SuccessfulMatches++
	} else {
		mv.stats.FailedMatches++
	}

	mv.stats.AverageGPULatency = (mv.stats.AverageGPULatency*0.9 + result.GPULatency*0.1)
	mv.stats.AverageASILatency = (mv.stats.AverageASILatency*0.9 + result.ASILatency*0.1)

	if mv.stats.TotalValidations > 0 {
		mv.stats.ConsistencyRate = float64(mv.stats.SuccessfulMatches) / float64(mv.stats.TotalValidations)
	}

	mv.stats.LastValidationTime = time.Now()
}

func (mv *MockValidator) GetValidatorStats() (*ValidatorStats, error) {
	return mv.stats, nil
}

func (mv *MockValidator) Close() error {
	return nil
}

func ValidateSeedsWithRetry(validator Validator, ctx context.Context, seed []byte, targetToken int32, maxRetries int) (*ValidationResult, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		result, err := validator.ValidateSeed(ctx, seed, targetToken)
		if err == nil {
			return result, nil
		}
		lastErr = err

		select {
		case <-time.After(time.Duration(i+1) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("validation failed after %d retries: %w", maxRetries, lastErr)
}
