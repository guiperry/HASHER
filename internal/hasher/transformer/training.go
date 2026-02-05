package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hasher/internal/hasher"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"time"
)

// TrainingConfig defines training hyperparameters
type TrainingConfig struct {
	Epochs         int
	BatchSize      int
	LearningRate   float32
	WeightDecay    float32
	WarmupSteps    int
	ValidationFreq int
	SaveFreq       int
	ModelPath      string
	DataPath       string
}

// TrainingState tracks training progress
type TrainingState struct {
	Epoch          int
	Step           int
	TotalLoss      float32
	Accuracy       float32
	ValidationLoss float32
	ValidationAcc  float32
	BestAccuracy   float32
	LearningRate   float32
	GradientNorm   float32
}

// DataSample represents a single training sample
type DataSample struct {
	InputTokens   []int
	OutputTokens  []int
	AttentionMask []bool
}

// DataLoader handles training data loading and batching
type DataLoader struct {
	Samples    []DataSample
	BatchSize  int
	Shuffle    bool
	currentIdx int
	rand       *rand.Rand
}

// NewDataLoader creates a new data loader
func NewDataLoader(samples []DataSample, batchSize int, shuffle bool) *DataLoader {
	return &DataLoader{
		Samples:    samples,
		BatchSize:  batchSize,
		Shuffle:    shuffle,
		currentIdx: 0,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NextBatch returns the next batch of data
func (dl *DataLoader) NextBatch() ([]DataSample, bool) {
	if dl.currentIdx >= len(dl.Samples) {
		if dl.Shuffle {
			dl.shuffle()
		}
		dl.currentIdx = 0
		return nil, false
	}

	end := dl.currentIdx + dl.BatchSize
	if end > len(dl.Samples) {
		end = len(dl.Samples)
	}

	batch := dl.Samples[dl.currentIdx:end]
	dl.currentIdx = end

	return batch, true
}

// shuffle randomizes sample order
func (dl *DataLoader) shuffle() {
	for i := len(dl.Samples) - 1; i > 0; i-- {
		j := dl.rand.Intn(i + 1)
		dl.Samples[i], dl.Samples[j] = dl.Samples[j], dl.Samples[i]
	}
}

// Trainer handles cryptographic transformer training
type Trainer struct {
	Model            *HasherTransformer
	Config           *TrainingConfig
	State            *TrainingState
	Loader           *DataLoader
	ValidationLoader *DataLoader
	SeedEncoder      *hasher.MatrixSeedEncoder
	Surrogate        *hasher.SurrogateGradient
	Optimizer        *HasherOptimizer
}

// HasherOptimizer implements optimization for hash-based neural networks
type HasherOptimizer struct {
	LearningRate float32
	WeightDecay  float32
	Momentum     float32
	Beta1        float32
	Beta2        float32
	Epsilon      float32
	t            int
	velocityMap  map[string][][]float32
	momentumMap  map[string][][]float32
}

// NewHasherOptimizer creates a new optimizer for hash-based networks
func NewHasherOptimizer(learningRate, weightDecay float32) *HasherOptimizer {
	return &HasherOptimizer{
		LearningRate: learningRate,
		WeightDecay:  weightDecay,
		Momentum:     0.9,
		Beta1:        0.9,
		Beta2:        0.999,
		Epsilon:      1e-8,
		t:            0,
		velocityMap:  make(map[string][][]float32),
		momentumMap:  make(map[string][][]float32),
	}
}

// NewTrainer creates a new trainer for cryptographic transformer
func NewTrainer(model *HasherTransformer, config *TrainingConfig, samples []DataSample) *Trainer {
	// Split data into train/validation (80/20)
	splitIdx := int(float64(len(samples)) * 0.8)
	trainSamples := samples[:splitIdx]
	valSamples := samples[splitIdx:]

	loader := NewDataLoader(trainSamples, config.BatchSize, true)
	valLoader := NewDataLoader(valSamples, config.BatchSize, false)

	seedEncoder := hasher.NewMatrixSeedEncoder()
	surrogate := hasher.NewSurrogateGradient("ste")
	optimizer := NewHasherOptimizer(config.LearningRate, config.WeightDecay)

	return &Trainer{
		Model:            model,
		Config:           config,
		State:            &TrainingState{BestAccuracy: 0},
		Loader:           loader,
		ValidationLoader: valLoader,
		SeedEncoder:      seedEncoder,
		Surrogate:        surrogate,
		Optimizer:        optimizer,
	}
}

// Train starts the training process
func (t *Trainer) Train() error {
	log.Printf("Starting training for %d epochs with batch size %d", t.Config.Epochs, t.Config.BatchSize)

	for epoch := 0; epoch < t.Config.Epochs; epoch++ {
		t.State.Epoch = epoch

		// Training loop
		epochLoss := float32(0)
		epochSteps := 0

		for {
			batch, _ := t.Loader.NextBatch()
			if batch == nil {
				break
			}

			// Process batch
			batchLoss, accuracy := t.processBatch(batch)
			epochLoss += batchLoss
			epochSteps++

			t.State.Step++
			t.State.TotalLoss = epochLoss / float32(epochSteps)
			t.State.Accuracy = accuracy

			// Log progress
			if t.State.Step%100 == 0 {
				log.Printf("Epoch %d, Step %d: Loss=%.4f, Acc=%.4f",
					epoch, t.State.Step, t.State.TotalLoss, t.State.Accuracy)
			}
		}

		// Validation
		if epoch%t.Config.ValidationFreq == 0 {
			t.validate()

			// Save best model
			if t.State.ValidationAcc > t.State.BestAccuracy {
				t.State.BestAccuracy = t.State.ValidationAcc
				if err := t.saveModel("best"); err != nil {
					log.Printf("Failed to save best model: %v", err)
				}
			}
		}

		// Save checkpoint
		if epoch%t.Config.SaveFreq == 0 {
			if err := t.saveModel(fmt.Sprintf("epoch_%d", epoch)); err != nil {
				log.Printf("Failed to save checkpoint: %v", err)
			}
		}
	}

	log.Printf("Training completed. Best validation accuracy: %.4f", t.State.BestAccuracy)
	return nil
}

// processBatch processes a single batch of data
func (t *Trainer) processBatch(batch []DataSample) (float32, float32) {
	totalLoss := float32(0)
	correct := 0
	total := 0

	for _, sample := range batch {
		// Forward pass
		output := t.Model.Forward(sample.InputTokens)

		// Calculate loss (simplified cross-entropy)
		loss := t.calculateLoss(output, sample.OutputTokens[0])
		totalLoss += loss

		// Check accuracy
		predicted, _ := t.Model.GenerateToken(sample.InputTokens, 0) // Temperature = 0 for greedy
		if predicted == sample.OutputTokens[0] {
			correct++
		}
		total++

		// Backward pass
		lossGrad := t.calculateLossGradient(output, sample.OutputTokens[0])
		t.Model.Backward(lossGrad, t.Config.LearningRate)
	}

	return totalLoss / float32(len(batch)), float32(correct) / float32(total)
}

// calculateLoss computes cross-entropy loss
func (t *Trainer) calculateLoss(output []float32, target int) float32 {
	// Simplified: use first dimension as logits
	logits := []float32{output[0]} // Simplified for single output

	// Apply log softmax
	maxLogit := float32(-math.MaxFloat32)
	for _, logit := range logits {
		if logit > maxLogit {
			maxLogit = logit
		}
	}

	sumExp := float32(0)
	for _, logit := range logits {
		sumExp += float32(math.Exp(float64(logit - maxLogit)))
	}

	targetLogit := float32(math.Exp(float64(logits[target] - maxLogit)))
	logProb := float32(math.Log(float64(targetLogit / sumExp)))
	return -logProb
}

// calculateLossGradient computes gradient of cross-entropy loss
func (t *Trainer) calculateLossGradient(output []float32, target int) []float32 {
	// Simplified gradient computation
	grad := make([]float32, len(output))

	// Gradient for cross-entropy with softmax
	maxLogit := float32(-math.MaxFloat32)
	for _, v := range output {
		if v > maxLogit {
			maxLogit = v
		}
	}

	sumExp := float32(0)
	for _, v := range output {
		sumExp += float32(math.Exp(float64(v - maxLogit)))
	}

	for i, v := range output {
		softmax := float32(math.Exp(float64(v-maxLogit))) / sumExp
		if i == target {
			grad[i] = softmax - 1
		} else {
			grad[i] = softmax
		}
	}

	return grad
}

// validate evaluates model on validation set
func (t *Trainer) validate() {
	log.Println("Running validation...")

	totalLoss := float32(0)
	correct := 0
	total := 0

	// Reset validation loader
	t.ValidationLoader.currentIdx = 0

	for {
		batch, _ := t.ValidationLoader.NextBatch()
		if batch == nil {
			break
		}

		for _, sample := range batch {
			// Forward pass
			output := t.Model.Forward(sample.InputTokens)

			// Calculate loss
			loss := t.calculateLoss(output, sample.OutputTokens[0])
			totalLoss += loss

			// Check accuracy
			predicted, _ := t.Model.GenerateToken(sample.InputTokens, 0)
			if predicted == sample.OutputTokens[0] {
				correct++
			}
			total++
		}
	}

	t.State.ValidationLoss = totalLoss / float32(total)
	t.State.ValidationAcc = float32(correct) / float32(total)

	log.Printf("Validation: Loss=%.4f, Acc=%.4f", t.State.ValidationLoss, t.State.ValidationAcc)
}

// saveModel saves the current model state
func (t *Trainer) saveModel(suffix string) error {
	// Simplified model saving
	// In production, would save all weight seeds and training state

	log.Printf("Saving model checkpoint: %s", suffix)

	// Save configuration and seeds for each layer
	// This is a placeholder - actual implementation would serialize all model parameters

	return nil
}

// loadModel loads a saved model
func (t *Trainer) loadModel(path string) error {
	// Placeholder for model loading
	log.Printf("Loading model from: %s", path)
	return nil
}

// =============================================================================
// API Client Functions for Training
// =============================================================================

// TrainRequest represents a training request to hasher-host
type TrainRequest struct {
	Epochs       int      `json:"epochs"`
	LearningRate float32  `json:"learning_rate"`
	BatchSize    int      `json:"batch_size"`
	DataSamples  []string `json:"data_samples"`
}

// TrainResponse represents the training API response
type TrainResponse struct {
	Epoch     int     `json:"epoch"`
	Loss      float32 `json:"loss"`
	Accuracy  float32 `json:"accuracy"`
	LatencyMs float64 `json:"latency_ms"`
	UsingASIC bool    `json:"using_asic"`
}

// findOpenPort finds an available port starting from a default port
func findOpenPort(startPort int) int {
	if startPort <= 0 {
		startPort = 8080
	}

	for port := startPort; port <= 9090; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			return port
		}
	}
	return startPort // fallback
}

// CallTrainingAPI calls the hasher-host training API
func CallTrainingAPI(epochs int, learningRate float32, batchSize int, dataSamples []string) (*TrainResponse, error) {
	// Find hasher-host API port
	apiPort := findOpenPort(8080)

	// Create training request
	req := TrainRequest{
		Epochs:       epochs,
		LearningRate: learningRate,
		BatchSize:    batchSize,
		DataSamples:  dataSamples,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal training request: %w", err)
	}

	// Send training request
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/api/v1/train", apiPort),
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call training API: %w", err)
	}
	defer resp.Body.Close()

	// Parse training response
	var trainResp TrainResponse
	if err := json.NewDecoder(resp.Body).Decode(&trainResp); err != nil {
		return nil, fmt.Errorf("failed to parse training response: %w", err)
	}

	return &trainResp, nil
}
