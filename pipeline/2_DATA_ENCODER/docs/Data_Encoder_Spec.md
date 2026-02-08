# Specification: Data Encoder (Stage 2)

**Version:** 1.0

**Status:** Draft

**Role:** Intermediate Data Pipeline (Bridging "Miner" to "Trainer")

---

## 1. Executive Summary

The **Data Encoder** is a high-throughput Go utility designed to transform raw, unstructured embeddings (from the Data Miner's application data directory) into hardware-ready "Neural Frames." It performs the critical translation between human-readable text and the **Antminer S3**'s specific SHA-256 requirements.

This tool picks up exactly where the Data Miner left off: it reads the JSON output containing text and embeddings, applies the **12-Slot Feature Mapping**, tokenizes the text for **Next-Token Prediction**, and initializes the Parquet schema for **Seed Persistence**.

---

## 2. System Architecture

### 2.1 Data Flow

1. **Input:** `ai_knowledge_base.json` (Stream of {Text, Embedding} objects from Stage 1).
2. **Process:**
* **Tokenizer:** Converts Text  `[]int` (Token IDs).
* **Mapper:** Converts Embedding (`[]float32`) to `[12]uint32` (ASIC Frame).
* **Windowing:** Slides across the text to create (Frame, Target) pairs.


3. **Output:** `training_frames.parquet` (The "Fuel" for the Evo-GRPO Trainer).

---

## 3. Component Specifications & Code

### 3.1 The Input Schema

We assume the Data Miner outputs a stream of JSON objects.

```go
// input_schema.go
package schema

type MinedRecord struct {
    FileName  string    `json:"file_name"`
    ChunkID   int       `json:"chunk_id"`
    Text      string    `json:"text"`
    Embedding []float32 `json:"embedding"` // The 768-dim vector from Ollama
}

```
---

### 3.2 The Tokenizer Service

This module wraps `tiktoken` to convert text into integer targets.

```go
// pkg/tokenizer/tokenizer.go
package tokenizer

import (
	"github.com/pkoukk/tiktoken-go"
	"log"
)

type Service struct {
	model *tiktoken.Tiktoken
}

func New() *Service {
	// usage of standard cl100k_base (GPT-4) encoding
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		log.Fatalf("Failed to initialize tokenizer: %v", err)
	}
	return &Service{model: tkm}
}

// Encode converts string to token IDs
func (s *Service) Encode(text string) []int {
	return s.model.Encode(text, nil, nil)
}

```

---

### 3.3 The Feature Mapper (24-Feature Packed)

This performs the critical **Feature Mapping** logic, packing 24 semantic features into the 12 hardware slots.

```go
// pkg/mapper/mapper.go
package mapper

import (
	"math"
	"math/rand"
)

type Service struct {
	projectionMatrix [][]float32
}

func New(seed int64) *Service {
	m := &Service{
		projectionMatrix: make([][]float32, 24), // 24 features
	}
	// Deterministic random projection
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < 24; i++ {
		m.projectionMatrix[i] = make([]float32, 768) // Assuming 768-dim embedding
		for j := 0; j < 768; j++ {
			m.projectionMatrix[i][j] = r.Float32()*2 - 1
		}
	}
	return m
}

func (s *Service) MapToSlots(embedding []float32) [12]uint32 {
	var slots [12]uint32
	intermediate := make([]int16, 24)

	// 1. Project & Quantize to int16
	for i := 0; i < 24; i++ {
		var sum float32
		for j := 0; j < 768; j++ {
			// Safety check for embedding length
			if j < len(embedding) {
				sum += embedding[j] * s.projectionMatrix[i][j]
			}
		}
		// Sigmoid squash to fit int16 range
		normalized := 1.0 / (1.0 + math.Exp(float64(-sum)))
		intermediate[i] = int16((normalized * 65535) - 32768)
	}

	// 2. Bit Packing (2x int16 -> 1x uint32)
	for i := 0; i < 12; i++ {
		high := uint32(uint16(intermediate[i*2]))
		low := uint32(uint16(intermediate[i*2+1]))
		slots[i] = (high << 16) | low
	}

	return slots
}

```

---

### 3.4 The Output Schema (Parquet)

This schema defines the **Training Data** structure. It includes the `BestSeed` field, which is initialized to empty/zero but acts as the "Seed Persistence" layer for the next stage.

```go
// output_schema.go
package schema

type TrainingFrame struct {
    // 1. Metadata for traceability
    SourceFile string `parquet:"name=source_file, type=BYTE_ARRAY, convertedtype=UTF8"`
    ChunkID    int32  `parquet:"name=chunk_id, type=INT32"`

    // 2. The Input (What the ASIC sees)
    // 12 slots * 4 bytes = 48 bytes total
    AsicSlots [12]uint32 `parquet:"name=asic_slots, type=FIXED_LEN_BYTE_ARRAY, length=48"`

    // 3. The Target (What the ASIC must predict)
    // This is the "Golden Nonce" we are hunting for
    TargetTokenID int32 `parquet:"name=target_token_id, type=INT32"`

    // 4. Seed Persistence (Placeholder for Stage 3)
    // The Evo-GRPO trainer will fill this. 32 bytes = 256 bits (SHA-256)
    BestSeed []byte `parquet:"name=best_seed, type=FIXED_LEN_BYTE_ARRAY, length=32, repetitiontype=OPTIONAL"`
}

```

---



## 4. The Orchestrator (Main Logic)

This bridges the components. It reads the JSON, tokenizes, maps, and writes the Parquet file.

```go
// main.go
package main

import (
	"encoding/json"
	"log"
	"os"

	"data-encoder/pkg/mapper"
	"data-encoder/pkg/schema"
	"data-encoder/pkg/tokenizer"

	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
	"github.com/xitongsys/parquet-go-source/local"
)

func main() {
	inputFile := "ai_knowledge_base.json"
	outputFile := "training_frames.parquet"

	// 1. Initialize Services
	tk := tokenizer.New()
	mp := mapper.New(1337) // Fixed seed for reproducibility

	// 2. Setup Parquet Writer
	fw, err := local.NewLocalFileWriter(outputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer fw.Close()

	// 4 concurrent writers for speed
	pw, err := writer.NewParquetWriter(fw, new(schema.TrainingFrame), 4) 
	if err != nil {
		log.Fatal(err)
	}
	pw.CompressionType = parquet.CompressionCodec_SNAPPY
	defer pw.WriteStop()

	// 3. Open JSON Input Stream
	jsonFile, _ := os.Open(inputFile)
	defer jsonFile.Close()
	decoder := json.NewDecoder(jsonFile)

	// 4. Processing Loop
	log.Println("Starting Data Encoding...")
	
	for decoder.More() {
		var record schema.MinedRecord
		if err := decoder.Decode(&record); err != nil {
			log.Printf("Skipping bad record: %v", err)
			continue
		}

		// A. Map the Embedding -> Hardware Slots
		// This embedding represents the "Context" of the chunk
		asicSlots := mp.MapToSlots(record.Embedding)

		// B. Tokenize the Text
		tokens := tk.Encode(record.Text)

		// C. Target Definition Strategy:
		// We treat the *entire* chunk as a sequence of targets for this context.
		// (Alternatively, you can implement sliding windows here)
		for _, tokenID := range tokens {
			frame := schema.TrainingFrame{
				SourceFile:    record.FileName,
				ChunkID:       int32(record.ChunkID),
				AsicSlots:     asicSlots,    // The "Input" (Context)
				TargetTokenID: int32(tokenID), // The "Target" (Next Word)
				BestSeed:      nil,          // To be discovered by Trainer
			}

			if err := pw.Write(frame); err != nil {
				log.Printf("Parquet write error: %v", err)
			}
		}
	}

	log.Println("Encoding Complete. Ready for Evo-GRPO.")
}

```

---

## 5. Deployment Instructions

### 5.1 Directory Structure

Ensure your project looks like this before running:

```text
/DATA_ENCODER
├── main.go                # The entry point
├── go.mod                 # Dependencies
├── ~/.local/share/dataminer/json/ai_knowledge_base.json        # Input from Stage 1
├── pkg/
│   ├── mapper/
│   │   └── mapper.go      # Feature Mapping Logic
│   ├── tokenizer/
│   │   └── tokenizer.go   # Tiktoken Wrapper
│   └── schema/
│       ├── input.go       # JSON Structs
│       └── output.go      # Parquet Structs

```

### 5.2 Dependencies

Run the following to grab the required libraries:

```bash
go mod init data-encoder
go get github.com/xitongsys/parquet-go
go get github.com/pkoukk/tiktoken-go
go get github.com/xitongsys/parquet-go-source

```

### 5.3 Execution

1. The `ai_knowledge_base.json` should be automatically detected in the ~/.local/share/dataminer/json/ folder.
2. Run `go run main.go`.
3. The result will be `training_frames.parquet`.

This file is now the **"Fuel Tank"** for your vHasher Simulator. It contains the exact 12-slot inputs the ASIC needs and the exact Token IDs the trainer must solve for.

