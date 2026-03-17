package miner

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"strings"

	"hasher/pkg/hashing/jitter"
	"hasher/pkg/hashing/math"
)

type ProofRecord struct {
	Theorem   string `json:"theorem"`
	ProofStep string `json:"proof_step"`
	Domain    uint32 `json:"domain_id"`
}

// pipelineTrainingFrame mirrors pipeline/2_DATA_ENCODER/pkg/schema.TrainingFrame.
// JSON tags are identical so the pipeline's Arrow writer can consume this file directly.
type pipelineTrainingFrame struct {
	SourceFile    string `json:"source_file"`
	ChunkID       int32  `json:"chunk_id"`
	WindowStart   int32  `json:"window_start"`
	WindowEnd     int32  `json:"window_end"`
	ContextLength int32  `json:"context_length"`
	AsicSlots0    int32  `json:"asic_slot_0"`
	AsicSlots1    int32  `json:"asic_slot_1"`
	AsicSlots2    int32  `json:"asic_slot_2"`
	AsicSlots3    int32  `json:"asic_slot_3"`
	AsicSlots4    int32  `json:"asic_slot_4"`
	AsicSlots5    int32  `json:"asic_slot_5"`
	AsicSlots6    int32  `json:"asic_slot_6"`
	AsicSlots7    int32  `json:"asic_slot_7"`
	AsicSlots8    int32  `json:"asic_slot_8"`
	AsicSlots9    int32  `json:"asic_slot_9"`
	AsicSlots10   int32  `json:"asic_slot_10"`
	AsicSlots11   int32  `json:"asic_slot_11"`
	TargetTokenID int32  `json:"target_token_id"`
	BestSeed      []byte `json:"best_seed,omitempty"`
}

func toPipelineFrame(f jitter.TrainingFrame) pipelineTrainingFrame {
	s := f.AsicSlots
	return pipelineTrainingFrame{
		SourceFile:    f.SourceFile,
		ChunkID:       f.ChunkID,
		WindowStart:   f.WindowStart,
		WindowEnd:     f.WindowEnd,
		ContextLength: f.ContextLength,
		AsicSlots0:    int32(s[0]),
		AsicSlots1:    int32(s[1]),
		AsicSlots2:    int32(s[2]),
		AsicSlots3:    int32(s[3]),
		AsicSlots4:    int32(s[4]),
		AsicSlots5:    int32(s[5]),
		AsicSlots6:    int32(s[6]),
		AsicSlots7:    int32(s[7]),
		AsicSlots8:    int32(s[8]),
		AsicSlots9:    int32(s[9]),
		AsicSlots10:   int32(s[10]),
		AsicSlots11:   int32(s[11]),
		TargetTokenID: f.TargetTokenID,
		BestSeed:      f.BestSeed,
	}
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

	return jitter.TrainingFrame{
		AsicSlots:     slots,
		SourceFile:    p.Theorem,
		TargetTokenID: deriveTargetTokenID(p.ProofStep),
		Metadata: map[string]interface{}{
			"theorem":    p.Theorem,
			"proof_step": p.ProofStep,
			"domain":     math.GetSubDomainName(p.Domain),
		},
	}
}

// deriveTargetTokenID produces a stable non-zero token ID from the proof step text.
// Uses FNV-32a for speed and distribution; result is capped at 100,000 to stay within
// a realistic vocabulary range.
func deriveTargetTokenID(proofStep string) int32 {
	h := fnv.New32a()
	h.Write([]byte(proofStep))
	hash := h.Sum32()
	if hash == 0 {
		return 1
	}
	return int32(hash % 100000)
}

// saveFrames writes two output files:
//
//	<outputPath>.json         — pretty-printed debug format (jitter.TrainingFrame)
//	<outputPath>.pipeline.json — flat schema, JSON-tag-compatible with
//	                             pipeline/2_DATA_ENCODER/pkg/schema.TrainingFrame.
//	                             Feed this file to the 2_DATA_ENCODER pipeline stage
//	                             to produce the Arrow IPC file consumed by 3_DATA_TRAINER.
func (m *MathMiner) saveFrames(frames []jitter.TrainingFrame, outputPath string) error {
	// Strip any existing .json suffix so we control both output file names cleanly.
	base := strings.TrimSuffix(outputPath, ".json")

	// 1. Pretty-printed debug JSON
	debugData, err := json.MarshalIndent(frames, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal debug frames: %w", err)
	}
	if err := os.WriteFile(base+".json", debugData, 0644); err != nil {
		return fmt.Errorf("failed to write debug JSON: %w", err)
	}

	// 2. Pipeline-compatible flat JSON (Arrow-ready)
	flat := make([]pipelineTrainingFrame, len(frames))
	for i, f := range frames {
		flat[i] = toPipelineFrame(f)
	}
	pipelineData, err := json.Marshal(flat)
	if err != nil {
		return fmt.Errorf("failed to marshal pipeline frames: %w", err)
	}
	if err := os.WriteFile(base+".pipeline.json", pipelineData, 0644); err != nil {
		return fmt.Errorf("failed to write pipeline JSON: %w", err)
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
	return MineMathProofs(config.InputPath, config.OutputPath, config.Subdomain)
}
