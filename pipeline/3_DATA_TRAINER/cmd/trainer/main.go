package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lab/hasher/data-trainer/internal/config"
	"github.com/lab/hasher/data-trainer/internal/logging"
	"github.com/lab/hasher/data-trainer/pkg/deployment"
	"github.com/lab/hasher/data-trainer/pkg/simulator"
	"github.com/lab/hasher/data-trainer/pkg/storage"
	"github.com/lab/hasher/data-trainer/pkg/training"
	"github.com/lab/hasher/data-trainer/pkg/validator"
)

var (
	configFile = flag.String("config", "", "Path to configuration file")
	dataPath   = flag.String("data", "data", "Path to data directory")
	maxEpochs  = flag.Int("epochs", 10, "Maximum number of training epochs")
	population = flag.Int("population", 32, "Population size for evolution")
	verbose    = flag.Bool("verbose", false, "Enable verbose logging")
)

type TrainingOrchestrator struct {
	logger        *logging.Logger
	simulator     simulator.HashSimulator
	storage       *storage.CSVStorage
	harness       *training.EvolutionaryHarness
	validator     validator.Validator
	flashManager  *deployment.FlashManager
	checkpointMgr *storage.CheckpointManager
	dataIngestor  *storage.DataIngestor
	trainingData  []*training.TrainingRecord
}

func main() {
	flag.Parse()

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	loggingConfig := &logging.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}

	logger, err := logging.NewLogger(loggingConfig)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	logger.Info("Starting HASHER Data Trainer v1.0")

	orchestrator, err := NewTrainingOrchestrator(logger)
	if err != nil {
		logger.Fatal("Failed to initialize orchestrator: %v", err)
	}
	defer orchestrator.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("Received signal %v, shutting down...", sig)
		cancel()
	}()

	if err := orchestrator.Run(ctx, *maxEpochs, *population); err != nil {
		logger.Fatal("Training failed: %v", err)
	}

	logger.Info("Training completed successfully")
}

func NewTrainingOrchestrator(logger *logging.Logger) (*TrainingOrchestrator, error) {
	orchestrator := &TrainingOrchestrator{
		logger: logger,
	}

	if err := orchestrator.initializeComponents(); err != nil {
		return nil, err
	}

	return orchestrator, nil
}

func (to *TrainingOrchestrator) initializeComponents() error {
	to.logger.Info("Initializing simulator...")
	simConfig := &simulator.SimulatorConfig{
		DeviceType:     "vhasher",
		MaxConcurrency: 100,
		TargetHashRate: 500000000,
		CacheSize:      10000,
		GPUDevice:      0,
		Timeout:        30,
	}
	sim := simulator.NewvHasherSimulator(simConfig)
	if err := sim.Initialize(simConfig); err != nil {
		return fmt.Errorf("failed to initialize simulator: %w", err)
	}
	to.simulator = sim

	to.logger.Info("Initializing storage...")
	storagePath := filepath.Join(*dataPath, "weights")
	csvStorage := storage.NewCSVStorage(storagePath, 1000)
	if err := csvStorage.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	to.storage = csvStorage

	to.logger.Info("Initializing checkpoint manager...")
	checkpointPath := filepath.Join(*dataPath, "checkpoints")
	if err := os.MkdirAll(checkpointPath, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}
	checkpointMgr := storage.NewCheckpointManager(filepath.Join(checkpointPath, "checkpoints.db"))
	if err := checkpointMgr.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize checkpoint manager: %w", err)
	}
	to.checkpointMgr = checkpointMgr

	// Initialize data ingestion
	to.logger.Info("Initializing data ingestion...")
	trainingDataPath := filepath.Join(os.Getenv("HOME"), ".local", "share", "data-encoder", "training_frames.parquet")
	if _, err := os.Stat(trainingDataPath); os.IsNotExist(err) {
		to.logger.Warn("Training data not found at %s, will use synthetic data", trainingDataPath)
		trainingDataPath = ""
	} else {
		dataIngestor := storage.NewDataIngestor(filepath.Dir(trainingDataPath))
		to.dataIngestor = dataIngestor

		to.logger.Info("Ingesting training data from %s...", trainingDataPath)
		trainingRecords, err := dataIngestor.ProcessAllFiles(nil)
		if err != nil {
			to.logger.Warn("Failed to ingest training data: %v", err)
			to.logger.Info("Proceeding with synthetic training data")
		} else {
			to.trainingData = trainingRecords
			to.logger.Info("Successfully ingested %d training records", len(trainingRecords))

			// Validate training data
			report, err := dataIngestor.ValidateTrainingData(trainingRecords)
			if err != nil {
				to.logger.Warn("Failed to validate training data: %v", err)
			} else {
				to.logger.Info("Training data validation: %s", report.GetSummary())
				if !report.Valid {
					to.logger.Warn("Training data has issues, proceeding anyway")
				}
			}
		}
	}

	to.logger.Info("Initializing evolutionary harness...")
	harness := training.NewEvolutionaryHarness(*population)
	to.harness = harness

	to.logger.Info("Initializing validator...")
	validatorConfig := &validator.ValidatorConfig{
		Timeout:            30 * time.Second,
		MaxConcurrency:     10,
		RetryAttempts:      3,
		ToleranceThreshold: 0.01,
		EnableASIC:         false,
	}
	validator := validator.NewCrossHardwareValidator(sim, validatorConfig)
	if err := validator.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize validator: %w", err)
	}
	to.validator = validator

	to.logger.Info("Initializing flash manager...")
	flashConfig := &deployment.FlashConfig{
		BPFMapPath:        "/sys/fs/bpf/hasher_weights",
		DeploymentTimeout: 300 * time.Second,
		MaxRetries:        3,
		RetryDelay:        5 * time.Second,
		ValidationMode:    "strict",
		BackupEnabled:     true,
		BackupPath:        filepath.Join(*dataPath, "backups"),
		RollbackEnabled:   true,
	}
	flashManager := deployment.NewFlashManager(csvStorage, flashConfig)
	if err := flashManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize flash manager: %w", err)
	}
	to.flashManager = flashManager

	return nil
}

func (to *TrainingOrchestrator) Run(ctx context.Context, maxEpochs, populationSize int) error {
	to.logger.Info("Starting training with %d epochs, population size %d", maxEpochs, populationSize)

	// Use ingested data if available, otherwise create synthetic data
	if len(to.trainingData) > 0 {
		to.logger.Info("Training with %d ingested records", len(to.trainingData))
		return to.runTrainingWithData(ctx, maxEpochs, populationSize, to.trainingData)
	} else {
		to.logger.Info("No ingested data available, using synthetic training")
		tokenMap := to.createTokenMap()
		return to.runSyntheticTraining(ctx, maxEpochs, populationSize, tokenMap)
	}
}

func (to *TrainingOrchestrator) runSyntheticTraining(ctx context.Context, maxEpochs, populationSize int, tokenMap map[int32]bool) error {
	for epoch := 0; epoch < maxEpochs; epoch++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		to.logger.Info("Starting epoch %d/%d", epoch+1, maxEpochs)

		if err := to.runEpoch(ctx, epoch, tokenMap); err != nil {
			to.logger.Error("Epoch %d failed: %v", epoch+1, err)
			return err
		}

		if epoch%5 == 0 {
			if err := to.saveProgress(epoch); err != nil {
				to.logger.Warn("Failed to save progress: %v", err)
			}
		}
	}

	to.logger.Info("Synthetic training completed all epochs")
	return to.finalizeTraining()
}

func (to *TrainingOrchestrator) runTrainingWithData(ctx context.Context, maxEpochs, populationSize int, records []*training.TrainingRecord) error {
	// Shuffle records for training
	for epoch := 0; epoch < maxEpochs; epoch++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		to.logger.Info("Starting epoch %d/%d with %d records", epoch+1, maxEpochs, len(records))

		// Process records in batches
		batchSize := populationSize
		for i := 0; i < len(records); i += batchSize {
			end := i + batchSize
			if end > len(records) {
				end = len(records)
			}

			batch := records[i:end]
			if err := to.trainBatch(ctx, batch); err != nil {
				to.logger.Error("Batch %d-%d failed: %v", i, end-1, err)
				return err
			}

			if i%(batchSize*10) == 0 {
				to.logger.Debug("Processed %d/%d records", i, len(records))
			}
		}

		if epoch%2 == 0 {
			if err := to.saveProgress(epoch); err != nil {
				to.logger.Warn("Failed to save progress: %v", err)
			}
		}
	}

	to.logger.Info("Data training completed all epochs")
	return to.finalizeTraining()
}

func (to *TrainingOrchestrator) runEpoch(ctx context.Context, epoch int, tokenMap map[int32]bool) error {
	targetTokens := to.getTargetTokens(epoch)

	for i, targetToken := range targetTokens {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if i%50 == 0 {
			to.logger.Debug("Processing token %d/%d in epoch %d", i+1, len(targetTokens), epoch+1)
		}

		if err := to.trainToken(ctx, targetToken, tokenMap); err != nil {
			to.logger.Warn("Failed to train token %d: %v", targetToken, err)
			continue
		}
	}

	return nil
}

func (to *TrainingOrchestrator) trainBatch(ctx context.Context, records []*training.TrainingRecord) error {
	for _, record := range records {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := to.trainRecord(ctx, record); err != nil {
			to.logger.Warn("Failed to train record for token %d: %v", record.TargetToken, err)
			continue
		}
	}

	return nil
}

func (to *TrainingOrchestrator) trainRecord(ctx context.Context, record *training.TrainingRecord) error {
	contextHash := training.ComputeContextHash(record.TokenSequence, 5)
	population := training.NewSeedPopulation(record.TargetToken, contextHash, 32)

	tokenMap := map[int32]bool{record.TargetToken: true}

	for gen := 0; gen < 20; gen++ { // Reduced generations for batch processing
		results, err := to.harness.EvaluatePopulation(population, record.TargetToken, tokenMap, to.simulator)
		if err != nil {
			return fmt.Errorf("failed to evaluate population: %w", err)
		}

		eliteSeeds := to.harness.GetEliteSeeds(results)
		if len(eliteSeeds) > 0 && eliteSeeds[0].Advantage > 0.5 {
			to.logger.Debug("Found winning seed for token %d in generation %d", record.TargetToken, gen)
			return to.saveWinningSeed(record.TargetToken, eliteSeeds[0], gen)
		}

		population.Seeds = to.harness.SelectAndMutate(results, population.Seeds)
		population.Generation++

		if gen%10 == 0 {
			to.logger.Debug("Token %d generation %d: fitness=%.4f", record.TargetToken, gen, population.Fitness)
		}
	}

	to.logger.Warn("Failed to find winning seed for token %d after 20 generations", record.TargetToken)
	return nil
}

func (to *TrainingOrchestrator) trainToken(ctx context.Context, targetToken int32, tokenMap map[int32]bool) error {
	hasCheckpoint, err := to.checkpointMgr.HasCheckpoint(targetToken)
	if err != nil {
		return fmt.Errorf("failed to check checkpoint: %w", err)
	}

	if hasCheckpoint {
		to.logger.Debug("Token %d already has checkpoint, skipping", targetToken)
		return nil
	}

	contextHash := training.ComputeContextHash([]int32{targetToken}, 5)
	population := training.NewSeedPopulation(targetToken, contextHash, *population)

	for gen := 0; gen < 50; gen++ {
		results, err := to.harness.EvaluatePopulation(population, targetToken, tokenMap, to.simulator)
		if err != nil {
			return fmt.Errorf("failed to evaluate population: %w", err)
		}

		eliteSeeds := to.harness.GetEliteSeeds(results)
		if len(eliteSeeds) > 0 && eliteSeeds[0].Advantage > 1.0 {
			to.logger.Debug("Found winning seed for token %d in generation %d", targetToken, gen)
			return to.saveWinningSeed(targetToken, eliteSeeds[0], gen)
		}

		population.Seeds = to.harness.SelectAndMutate(results, population.Seeds)
		population.Generation++

		if gen%25 == 0 {
			to.logger.Debug("Token %d generation %d: fitness=%.4f", targetToken, gen, population.Fitness)
		}
	}

	to.logger.Warn("Failed to find winning seed for token %d after 50 generations", targetToken)
	return nil
}

func (to *TrainingOrchestrator) saveWinningSeed(targetToken int32, seed training.SeedResult, generation int) error {
	weightRecord := storage.WeightRecord{
		TokenID:      targetToken,
		BestSeed:     seed.Seed,
		FitnessScore: seed.Reward,
		Generation:   int32(generation),
		ContextKey:   training.ComputeContextHash([]int32{targetToken}, 5),
	}

	layerID := int32(targetToken / 100)
	if err := to.storage.SaveWeights([]storage.WeightRecord{weightRecord}, layerID); err != nil {
		return fmt.Errorf("failed to save weight: %w", err)
	}

	checkpointEntry := training.CheckpointEntry{
		TokenID:      targetToken,
		SeedHash:     storage.ComputeSeedHash(seed.Seed),
		BestSeed:     seed.Seed,
		FitnessScore: seed.Reward,
		LastUpdated:  time.Now().Unix(),
	}

	if err := to.checkpointMgr.SaveCheckpoint(checkpointEntry); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	to.logger.Info("Saved winning seed for token %d (fitness=%.4f)", targetToken, seed.Reward)
	return nil
}

func (to *TrainingOrchestrator) saveProgress(epoch int) error {
	summary, err := to.checkpointMgr.GetCheckpointSummary()
	if err != nil {
		return err
	}

	to.logger.Info("Progress checkpoint at epoch %d: %d tokens, avg_fitness=%.4f",
		epoch, summary.TotalTokens, summary.AverageFitness)

	return nil
}

func (to *TrainingOrchestrator) finalizeTraining() error {
	to.logger.Info("Finalizing training...")

	layers, err := to.storage.ListLayers()
	if err != nil {
		to.logger.Warn("Failed to list layers: %v", err)
	} else {
		to.logger.Info("Created %d weight layers", len(layers))
	}

	stats, err := to.validator.GetValidatorStats()
	if err != nil {
		to.logger.Warn("Failed to get validator stats: %v", err)
	} else {
		to.logger.Info("Validator stats: consistency=%.2f%%, validations=%d",
			stats.ConsistencyRate*100, stats.TotalValidations)
	}

	return nil
}

func (to *TrainingOrchestrator) Shutdown() {
	to.logger.Info("Shutting down training orchestrator...")

	if to.validator != nil {
		to.validator.Close()
	}

	if to.checkpointMgr != nil {
		to.checkpointMgr.Close()
	}

	if to.simulator != nil {
		to.simulator.Shutdown()
	}

	if to.logger != nil {
		to.logger.Info("Shutdown complete")
	}
}

func loadConfig(filename string) (*config.Config, error) {
	config := &config.Config{
		Simulator: &config.SimulatorConfig{
			DeviceType:     "vhasher",
			MaxConcurrency: 100,
			TargetHashRate: 500000000,
			CacheSize:      10000,
			GPUDevice:      0,
			Timeout:        30,
		},
		Storage: &config.StorageConfig{
			BasePath:  "data/weights",
			LayerSize: 1000,
		},
		Training: &config.TrainingConfig{
			PopulationSize:  128,
			MaxGenerations:  500,
			EliteRatio:      0.25,
			MutationRate:    0.05,
			TargetFitness:   0.95,
			ValidationSplit: 0.1,
		},
		Deployment: &config.DeploymentConfig{
			BPFMapPath:        "/sys/fs/bpf/hasher_weights",
			DeploymentTimeout: "300s",
			MaxRetries:        3,
			RetryDelay:        "5s",
			ValidationMode:    "strict",
			BackupEnabled:     true,
			BackupPath:        "data/backups",
			RollbackEnabled:   true,
		},
		Validation: &config.ValidationConfig{
			Timeout:            "30s",
			MaxConcurrency:     10,
			RetryAttempts:      3,
			ToleranceThreshold: 0.01,
			EnableASIC:         false,
		},
		Logging: &config.LoggingConfig{
			Level:      "info",
			Format:     "text",
			Output:     "stdout",
			MaxSize:    100,
			MaxBackups: 10,
			MaxAge:     30,
		},
	}

	if filename == "" {
		return config, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}

func (to *TrainingOrchestrator) createTokenMap() map[int32]bool {
	tokenMap := make(map[int32]bool)

	for i := int32(1); i <= 1000; i++ {
		tokenMap[i] = true
	}

	return tokenMap
}

func (to *TrainingOrchestrator) getTargetTokens(epoch int) []int32 {
	tokens := make([]int32, 0, 50) // Smaller set for testing

	start := int32(epoch*50 + 1)
	end := start + 49
	if end > 500 {
		end = 500
	}

	for i := start; i <= end; i++ {
		tokens = append(tokens, i)
	}

	return tokens
}
