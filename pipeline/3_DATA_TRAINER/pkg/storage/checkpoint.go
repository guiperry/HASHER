package storage

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/lab/hasher/data-trainer/pkg/training"
)

type CheckpointManager struct {
	dbPath           string
	db               map[string][]byte
	mutex            sync.RWMutex
	batchSize        int
	dirty            bool
	lastSave         time.Time
	autoSaveInterval time.Duration
}

type CheckpointConfig struct {
	DBPath             string        `json:"db_path"`
	AutoCheckpoint     bool          `json:"auto_checkpoint"`
	CheckpointInterval time.Duration `json:"checkpoint_interval"`
	BatchSize          int           `json:"batch_size"`
	MaxCheckpoints     int           `json:"max_checkpoints"`
}

type CheckpointSummary struct {
	LastCheckpointTime time.Time `json:"last_checkpoint_time"`
	TotalCheckpoints   int       `json:"total_checkpoints"`
	TotalTokens        int       `json:"total_tokens"`
	DatabaseSize       int64     `json:"database_size"`
	AverageFitness     float64   `json:"average_fitness"`
	BestFitness        float64   `json:"best_fitness"`
}

func NewCheckpointManager(dbPath string) *CheckpointManager {
	return &CheckpointManager{
		dbPath:           dbPath,
		db:               make(map[string][]byte),
		batchSize:        100,
		autoSaveInterval: 30 * time.Second,
	}
}

func (cm *CheckpointManager) Initialize() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Load checkpoints from disk if file exists
	if cm.dbPath != "" {
		data, err := os.ReadFile(cm.dbPath)
		if err == nil {
			var savedDB map[string][]byte
			if err := json.Unmarshal(data, &savedDB); err == nil {
				cm.db = savedDB
				fmt.Printf("[CHECKPOINT] Loaded %d checkpoints from %s\n", len(cm.db), cm.dbPath)
			}
		} else if !os.IsNotExist(err) {
			fmt.Printf("[CHECKPOINT] Warning: could not load checkpoints: %v\n", err)
		}
	}

	cm.lastSave = time.Now()
	return nil
}

func (cm *CheckpointManager) Close() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	return cm.saveToDiskUnlocked()
}

func (cm *CheckpointManager) saveToDiskUnlocked() error {
	if cm.dbPath == "" || !cm.dirty {
		return nil
	}

	data, err := json.Marshal(cm.db)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoints: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tmpPath := cm.dbPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoints: %w", err)
	}

	if err := os.Rename(tmpPath, cm.dbPath); err != nil {
		return fmt.Errorf("failed to atomic-save checkpoints: %w", err)
	}

	fmt.Printf("[CHECKPOINT] Saved %d checkpoints to %s\n", len(cm.db), cm.dbPath)
	cm.dirty = false
	cm.lastSave = time.Now()
	return nil
}

func (cm *CheckpointManager) maybeAutoSave() {
	if time.Since(cm.lastSave) >= cm.autoSaveInterval {
		cm.saveToDiskUnlocked()
	}
}

func (cm *CheckpointManager) SaveCheckpoint(entry training.CheckpointEntry) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	tokenKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(tokenKey, uint32(entry.TokenID))

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint entry: %w", err)
	}

	cm.db[string(tokenKey)] = data
	cm.dirty = true

	// Immediately persist to disk for durability
	if err := cm.saveToDiskUnlocked(); err != nil {
		fmt.Printf("[CHECKPOINT] Warning: failed to persist checkpoint: %v\n", err)
	}

	return nil
}

func (cm *CheckpointManager) LoadCheckpoint(tokenID int32) (*training.CheckpointEntry, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	tokenKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(tokenKey, uint32(tokenID))

	data, exists := cm.db[string(tokenKey)]
	if !exists {
		return nil, fmt.Errorf("checkpoint for token %d not found", tokenID)
	}

	var checkpoint training.CheckpointEntry
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint entry: %w", err)
	}

	return &checkpoint, nil
}

func (cm *CheckpointManager) LoadAllCheckpoints() ([]training.CheckpointEntry, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var checkpoints []training.CheckpointEntry
	for _, data := range cm.db {
		var checkpoint training.CheckpointEntry
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			continue
		}
		checkpoints = append(checkpoints, checkpoint)
	}

	return checkpoints, nil
}

func (cm *CheckpointManager) BatchSaveCheckpoints(entries []training.CheckpointEntry) error {
	if len(entries) == 0 {
		return nil
	}

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for _, entry := range entries {
		tokenKey := make([]byte, 4)
		binary.LittleEndian.PutUint32(tokenKey, uint32(entry.TokenID))

		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal checkpoint entry: %w", err)
		}

		cm.db[string(tokenKey)] = data
	}

	cm.dirty = true

	// Immediately persist to disk for durability
	if err := cm.saveToDiskUnlocked(); err != nil {
		fmt.Printf("[CHECKPOINT] Warning: failed to persist batch: %v\n", err)
	}

	return nil
}

func (cm *CheckpointManager) DeleteCheckpoint(tokenID int32) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	tokenKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(tokenKey, uint32(tokenID))

	delete(cm.db, string(tokenKey))
	return nil
}

func (cm *CheckpointManager) HasCheckpoint(tokenID int32) (bool, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	tokenKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(tokenKey, uint32(tokenID))

	_, exists := cm.db[string(tokenKey)]
	return exists, nil
}

func (cm *CheckpointManager) GetCheckpointsByFitness(minFitness, maxFitness float64) ([]training.CheckpointEntry, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var checkpoints []training.CheckpointEntry
	for _, data := range cm.db {
		var checkpoint training.CheckpointEntry
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			continue
		}

		if checkpoint.FitnessScore >= minFitness &&
			(maxFitness <= 0 || checkpoint.FitnessScore <= maxFitness) {
			checkpoints = append(checkpoints, checkpoint)
		}
	}

	return checkpoints, nil
}

func (cm *CheckpointManager) SaveGeneration(generation int32, seedResults []training.SeedResult) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	genKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(genKey, uint32(generation))

	data, err := json.Marshal(seedResults)
	if err != nil {
		return fmt.Errorf("failed to marshal generation data: %w", err)
	}

	cm.db[string(genKey)] = data
	return nil
}

func (cm *CheckpointManager) LoadGeneration(generation int32) ([]training.SeedResult, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	genKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(genKey, uint32(generation))

	data, exists := cm.db[string(genKey)]
	if !exists {
		return nil, fmt.Errorf("generation %d not found", generation)
	}

	var results []training.SeedResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal generation data: %w", err)
	}

	return results, nil
}

func (cm *CheckpointManager) GetLatestGeneration() (int32, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var latestGen int32 = -1
	for key := range cm.db {
		if len(key) == 4 {
			gen := binary.LittleEndian.Uint32([]byte(key))
			if int32(gen) > latestGen {
				latestGen = int32(gen)
			}
		}
	}

	return latestGen, nil
}

func (cm *CheckpointManager) GetCheckpointSummary() (*CheckpointSummary, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	summary := &CheckpointSummary{
		LastCheckpointTime: time.Now(),
	}

	var totalFitness, bestFitness float64
	var tokenCount int

	for key, data := range cm.db {
		if len(key) == 4 {
			tokenCount++

			var checkpoint training.CheckpointEntry
			if err := json.Unmarshal(data, &checkpoint); err == nil {
				totalFitness += checkpoint.FitnessScore
				if checkpoint.FitnessScore > bestFitness {
					bestFitness = checkpoint.FitnessScore
				}
			}
		}
	}

	summary.TotalTokens = tokenCount
	if tokenCount > 0 {
		summary.AverageFitness = totalFitness / float64(tokenCount)
	}
	summary.BestFitness = bestFitness
	summary.TotalCheckpoints = tokenCount

	var totalSize int64
	for _, data := range cm.db {
		totalSize += int64(len(data))
	}
	summary.DatabaseSize = totalSize

	return summary, nil
}

func (cm *CheckpointManager) Compact() error {
	return nil
}

func (cm *CheckpointManager) VerifyIntegrity() error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for key, data := range cm.db {
		if len(key) == 4 {
			var checkpoint training.CheckpointEntry
			if err := json.Unmarshal(data, &checkpoint); err != nil {
				return fmt.Errorf("invalid checkpoint data for key %x: %w", []byte(key), err)
			}

			if checkpoint.SeedHash == nil || len(checkpoint.SeedHash) == 0 {
				return fmt.Errorf("invalid seed hash for token %d", checkpoint.TokenID)
			}
		}
	}

	return nil
}

func ComputeSeedHash(seed []byte) []byte {
	hash := sha256.Sum256(seed)
	return hash[:]
}
