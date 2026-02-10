package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lab/hasher/data-trainer/pkg/storage"
)

type FlashConfig struct {
	BPFMapPath        string        `json:"bpf_map_path"`
	OpenWRTEndpoint   string        `json:"openwrt_endpoint"`
	DeploymentTimeout time.Duration `json:"deployment_timeout"`
	MaxRetries        int           `json:"max_retries"`
	RetryDelay        time.Duration `json:"retry_delay"`
	ValidationMode    string        `json:"validation_mode"`
	BackupEnabled     bool          `json:"backup_enabled"`
	BackupPath        string        `json:"backup_path"`
	RollbackEnabled   bool          `json:"rollback_enabled"`
}

type FlashStats struct {
	TotalDeployments   int64     `json:"total_deployments"`
	SuccessfulFlashes  int64     `json:"successful_flashes"`
	FailedFlashes      int64     `json:"failed_flashes"`
	RollbacksExecuted  int64     `json:"rollbacks_executed"`
	AverageFlashTime   float64   `json:"average_flash_time"`
	LastDeploymentTime time.Time `json:"last_deployment_time"`
	LastDeploymentID   string    `json:"last_deployment_id"`
}

type DeploymentRequest struct {
	LayerID       int32                   `json:"layer_id"`
	TargetFitness float64                 `json:"target_fitness"`
	TokenIDs      []int32                 `json:"token_ids,omitempty"`
	ContextHash   uint32                  `json:"context_hash,omitempty"`
	ValidationReq *ValidationRequirements `json:"validation_requirements,omitempty"`
	DryRun        bool                    `json:"dry_run"`
}

type ValidationRequirements struct {
	MinConsistencyRate float64 `json:"min_consistency_rate"`
	MaxErrorMargin     float64 `json:"max_error_margin"`
	RequiredSamples    int     `json:"required_samples"`
}

type DeploymentResult struct {
	DeploymentID      string                 `json:"deployment_id"`
	LayerID           int32                  `json:"layer_id"`
	Success           bool                   `json:"success"`
	WeightsFlashed    int                    `json:"weights_flashed"`
	FlashTime         time.Duration          `json:"flash_time"`
	ValidationResults map[string]interface{} `json:"validation_results"`
	BackupPath        string                 `json:"backup_path,omitempty"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
	Metrics           map[string]interface{} `json:"metrics"`
}

type FlashManager struct {
	storage   *storage.CSVStorage
	config    *FlashConfig
	mutex     sync.RWMutex
	stats     *FlashStats
	isRunning bool
}

type BPFDummyInterface struct {
	mapData map[string][]byte
	mutex   sync.RWMutex
}

func NewFlashManager(storage *storage.CSVStorage, config *FlashConfig) *FlashManager {
	if config == nil {
		config = &FlashConfig{
			BPFMapPath:        "/sys/fs/bpf/hasher_weights",
			DeploymentTimeout: 300 * time.Second,
			MaxRetries:        3,
			RetryDelay:        5 * time.Second,
			ValidationMode:    "strict",
			BackupEnabled:     true,
			BackupPath:        "data/backups",
			RollbackEnabled:   true,
		}
	}

	return &FlashManager{
		storage: storage,
		config:  config,
		stats: &FlashStats{
			LastDeploymentTime: time.Now(),
		},
		isRunning: false,
	}
}

func (fm *FlashManager) Initialize() error {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()

	if fm.storage == nil {
		return fmt.Errorf("storage is required")
	}

	if fm.config.BackupEnabled && fm.config.BackupPath != "" {
		if err := os.MkdirAll(fm.config.BackupPath, 0755); err != nil {
			return fmt.Errorf("failed to create backup directory: %w", err)
		}
	}

	fm.isRunning = true
	return nil
}

func (fm *FlashManager) DeployWeights(ctx context.Context, request *DeploymentRequest) (*DeploymentResult, error) {
	if !fm.isRunning {
		return nil, fmt.Errorf("flash manager is not running")
	}

	deploymentID := fmt.Sprintf("deploy_%d_%d", request.LayerID, time.Now().Unix())

	result := &DeploymentResult{
		DeploymentID:      deploymentID,
		LayerID:           request.LayerID,
		Success:           false,
		WeightsFlashed:    0,
		FlashTime:         0,
		ValidationResults: make(map[string]interface{}),
		Metrics:           make(map[string]interface{}),
	}

	start := time.Now()
	defer func() {
		result.FlashTime = time.Since(start)
		fm.updateStats(result)
	}()

	weights, err := fm.loadWeightsForDeployment(request)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to load weights: %v", err)
		return result, err
	}

	if request.DryRun {
		result.Success = true
		result.WeightsFlashed = len(weights)
		result.Metrics["dry_run"] = true
		return result, nil
	}

	if fm.config.BackupEnabled {
		backupPath, err := fm.createBackup(weights)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("backup failed: %v", err)
			return result, err
		}
		result.BackupPath = backupPath
	}

	if err := fm.flashWeights(ctx, weights); err != nil {
		result.ErrorMessage = fmt.Sprintf("flash operation failed: %v", err)

		if fm.config.RollbackEnabled && result.BackupPath != "" {
			if rollbackErr := fm.rollbackWeights(result.BackupPath); rollbackErr != nil {
				result.ErrorMessage += fmt.Sprintf(" (rollback also failed: %v)", rollbackErr)
			} else {
				result.ErrorMessage += " (rollback successful)"
			}
		}

		return result, err
	}

	if err := fm.validateDeployment(ctx, weights, request.ValidationReq); err != nil {
		result.ErrorMessage = fmt.Sprintf("validation failed: %v", err)

		if fm.config.RollbackEnabled && result.BackupPath != "" {
			if rollbackErr := fm.rollbackWeights(result.BackupPath); rollbackErr != nil {
				result.ErrorMessage += fmt.Sprintf(" (rollback also failed: %v)", rollbackErr)
			} else {
				result.ErrorMessage += " (rollback successful)"
			}
		}

		return result, err
	}

	result.Success = true
	result.WeightsFlashed = len(weights)
	result.Metrics["deployment_time"] = result.FlashTime.Seconds()
	result.Metrics["weights_per_second"] = float64(len(weights)) / result.FlashTime.Seconds()

	return result, nil
}

func (fm *FlashManager) loadWeightsForDeployment(request *DeploymentRequest) ([]storage.WeightRecord, error) {
	query := storage.WeightQuery{
		LayerID:     request.LayerID,
		MinFitness:  request.TargetFitness,
		TokenIDs:    request.TokenIDs,
		ContextHash: request.ContextHash,
	}

	weights, err := fm.storage.LoadWeights(query)
	if err != nil {
		return nil, err
	}

	result := make([]storage.WeightRecord, len(weights))
	for i, w := range weights {
		result[i] = storage.WeightRecord{
			TokenID:      w.TokenID,
			BestSeed:     w.BestSeed,
			FitnessScore: w.FitnessScore,
			Generation:   w.Generation,
			ContextKey:   w.ContextKey,
		}
	}

	return result, nil
}

func (fm *FlashManager) flashWeights(ctx context.Context, weights []storage.WeightRecord) error {
	bpf := &BPFDummyInterface{
		mapData: make(map[string][]byte),
	}

	for _, weight := range weights {
		key := fmt.Sprintf("token_%d", weight.TokenID)

		if err := bpf.updateElement(key, []byte(weight.BestSeed)); err != nil {
			return fmt.Errorf("failed to update BPF element for token %d: %w", weight.TokenID, err)
		}
	}

	return nil
}

func (bpf *BPFDummyInterface) updateElement(key string, value []byte) error {
	bpf.mutex.Lock()
	defer bpf.mutex.Unlock()

	bpf.mapData[key] = make([]byte, len(value))
	copy(bpf.mapData[key], value)

	return nil
}

func (bpf *BPFDummyInterface) getElement(key string) ([]byte, error) {
	bpf.mutex.RLock()
	defer bpf.mutex.RUnlock()

	value, exists := bpf.mapData[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

func (fm *FlashManager) validateDeployment(ctx context.Context, weights []storage.WeightRecord, validationReq *ValidationRequirements) error {
	if validationReq == nil {
		return nil
	}

	if len(weights) == 0 {
		return fmt.Errorf("no weights to validate")
	}

	sampleSize := validationReq.RequiredSamples
	if sampleSize <= 0 || sampleSize > len(weights) {
		sampleSize = len(weights)
	}

	successCount := 0
	var totalErrorMargin float64

	bpf := &BPFDummyInterface{mapData: make(map[string][]byte)}

	for _, weight := range weights[:sampleSize] {
		key := fmt.Sprintf("token_%d", weight.TokenID)
		retrievedSeed, err := bpf.getElement(key)
		if err != nil {
			return fmt.Errorf("failed to retrieve weight for token %d: %w", weight.TokenID, err)
		}

		errorMargin := fm.calculateErrorMargin([]byte(weight.BestSeed), retrievedSeed)
		totalErrorMargin += errorMargin

		if errorMargin <= validationReq.MaxErrorMargin {
			successCount++
		}
	}

	consistencyRate := float64(successCount) / float64(sampleSize)
	averageErrorMargin := totalErrorMargin / float64(sampleSize)

	if consistencyRate < validationReq.MinConsistencyRate {
		return fmt.Errorf("consistency rate %.2f is below required %.2f",
			consistencyRate, validationReq.MinConsistencyRate)
	}

	if averageErrorMargin > validationReq.MaxErrorMargin {
		return fmt.Errorf("average error margin %.4f exceeds threshold %.4f",
			averageErrorMargin, validationReq.MaxErrorMargin)
	}

	return nil
}

func (fm *FlashManager) calculateErrorMargin(original, retrieved []byte) float64 {
	if len(original) != len(retrieved) {
		return 1.0
	}

	var diffCount int
	for i := range original {
		if original[i] != retrieved[i] {
			diffCount++
		}
	}

	return float64(diffCount) / float64(len(original))
}

func (fm *FlashManager) createBackup(weights []storage.WeightRecord) (string, error) {
	if !fm.config.BackupEnabled {
		return "", nil
	}

	timestamp := time.Now().Format("20060102_150405")
	backupFilename := fmt.Sprintf("backup_%s_%d.json", timestamp, time.Now().Unix())
	backupPath := filepath.Join(fm.config.BackupPath, backupFilename)

	backupData := map[string]interface{}{
		"timestamp":   time.Now().Unix(),
		"weights":     weights,
		"count":       len(weights),
		"backup_type": "pre_deployment",
	}

	data, err := json.MarshalIndent(backupData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal backup data: %w", err)
	}

	if err := ioutil.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupPath, nil
}

func (fm *FlashManager) rollbackWeights(backupPath string) error {
	if !fm.config.RollbackEnabled {
		return fmt.Errorf("rollback is disabled")
	}

	data, err := ioutil.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	var backupData map[string]interface{}
	if err := json.Unmarshal(data, &backupData); err != nil {
		return fmt.Errorf("failed to unmarshal backup data: %w", err)
	}

	weightsInterface, ok := backupData["weights"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid backup format: weights not found")
	}

	bpf := &BPFDummyInterface{mapData: make(map[string][]byte)}

	for i, weightInterface := range weightsInterface {
		weightMap, ok := weightInterface.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid weight format at index %d", i)
		}

		tokenIDFloat, ok := weightMap["token_id"].(float64)
		if !ok {
			return fmt.Errorf("invalid token_id at index %d", i)
		}

		bestSeedInterface, ok := weightMap["best_seed"].(string)
		if !ok {
			return fmt.Errorf("invalid best_seed at index %d", i)
		}

		key := fmt.Sprintf("token_%d", int32(tokenIDFloat))
		bpf.updateElement(key, []byte(bestSeedInterface))
	}

	return nil
}

func (fm *FlashManager) updateStats(result *DeploymentResult) {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()

	fm.stats.TotalDeployments++
	fm.stats.LastDeploymentTime = time.Now()
	fm.stats.LastDeploymentID = result.DeploymentID

	if result.Success {
		fm.stats.SuccessfulFlashes++
	} else {
		fm.stats.FailedFlashes++
	}

	if result.FlashTime > 0 {
		fm.stats.AverageFlashTime = (fm.stats.AverageFlashTime*0.9 + result.FlashTime.Seconds()*0.1)
	}
}

func (fm *FlashManager) GetFlashStats() (*FlashStats, error) {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()

	return &FlashStats{
		TotalDeployments:   fm.stats.TotalDeployments,
		SuccessfulFlashes:  fm.stats.SuccessfulFlashes,
		FailedFlashes:      fm.stats.FailedFlashes,
		RollbacksExecuted:  fm.stats.RollbacksExecuted,
		AverageFlashTime:   fm.stats.AverageFlashTime,
		LastDeploymentTime: fm.stats.LastDeploymentTime,
		LastDeploymentID:   fm.stats.LastDeploymentID,
	}, nil
}

func (fm *FlashManager) Close() error {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()

	fm.isRunning = false
	return nil
}

func (fm *FlashManager) ListBackups() ([]string, error) {
	if !fm.config.BackupEnabled || fm.config.BackupPath == "" {
		return nil, fmt.Errorf("backup is disabled")
	}

	files, err := ioutil.ReadDir(fm.config.BackupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			backups = append(backups, file.Name())
		}
	}

	return backups, nil
}

func (fm *FlashManager) DeleteBackup(filename string) error {
	if !fm.config.BackupEnabled {
		return fmt.Errorf("backup is disabled")
	}

	backupPath := filepath.Join(fm.config.BackupPath, filename)
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}

	return nil
}
