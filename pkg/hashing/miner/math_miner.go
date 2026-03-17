package miner

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"

	"hasher/pkg/hashing/jitter"
	"hasher/pkg/hashing/math"
)

type ProofRecord struct {
	Theorem   string `json:"theorem"`
	ProofStep string `json:"proof_step"`
	Domain    uint32 `json:"domain_id"`
}

type MathMiner struct {
	mapper       *math.LaTeXMapper
	outputFormat string
}

func NewMathMiner(subdomain uint32) *MathMiner {
	return &MathMiner{
		mapper:       math.NewLaTeXMapper(subdomain),
		outputFormat: "json",
	}
}

func (m *MathMiner) MineFromFile(inputPath string, outputPath string) (int, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read input file: %w", err)
	}

	var proofs []ProofRecord
	if err := json.Unmarshal(data, &proofs); err != nil {
		return 0, fmt.Errorf("failed to parse JSON: %w", err)
	}

	frames := make([]jitter.TrainingFrame, 0, len(proofs))

	for i, p := range proofs {
		frame := m.processProof(p)
		frame.ChunkID = int32(i)
		frames = append(frames, frame)
	}

	if err := m.saveFrames(frames, outputPath); err != nil {
		return 0, fmt.Errorf("failed to save frames: %w", err)
	}

	return len(frames), nil
}

func (m *MathMiner) processProof(p ProofRecord) jitter.TrainingFrame {
	slots := m.mapper.MapLaTeXToTensor(p.ProofStep, p.Domain)

	targetTokenID := deriveTargetTokenID(p.ProofStep)

	frame := jitter.TrainingFrame{
		AsicSlots:     slots,
		SourceFile:    p.Theorem,
		TargetTokenID: targetTokenID,
		Metadata: map[string]interface{}{
			"theorem":    p.Theorem,
			"proof_step": p.ProofStep,
			"domain":     math.GetSubDomainName(p.Domain),
		},
	}

	return frame
}

func deriveTargetTokenID(proofStep string) int32 {
	h := fnv.New32a()
	h.Write([]byte(proofStep))
	hash := h.Sum32()
	if hash == 0 {
		return 1
	}
	return int32(hash % 100000)
}

func (m *MathMiner) saveFrames(frames []jitter.TrainingFrame, outputPath string) error {
	data, err := json.MarshalIndent(frames, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal frames: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

func MineMathProofs(inputPath, outputPath string, subdomain uint32) (int, error) {
	miner := NewMathMiner(subdomain)
	return miner.MineFromFile(inputPath, outputPath)
}

type MathDatasetConfig struct {
	InputPath      string
	OutputPath     string
	Subdomain      uint32
	BatchSize      int
	ShouldValidate bool
}

func (c *MathDatasetConfig) ValidateConfig() error {
	if c.InputPath == "" {
		return fmt.Errorf("InputPath is required")
	}
	if c.OutputPath == "" {
		return fmt.Errorf("OutputPath is required")
	}
	if c.Subdomain == 0 {
		c.Subdomain = math.SUB_algebra
	}
	if c.BatchSize == 0 {
		c.BatchSize = 1000
	}
	return nil
}

func ProcessMathDataset(config *MathDatasetConfig) (int, error) {
	if err := config.ValidateConfig(); err != nil {
		return 0, err
	}

	miner := NewMathMiner(config.Subdomain)
	return miner.MineFromFile(config.InputPath, config.OutputPath)
}
