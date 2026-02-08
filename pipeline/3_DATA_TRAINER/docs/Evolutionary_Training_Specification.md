# Evolutionary Training Specification (ETS)

**Project:** HASHER

**Component:** Training Harness (vHasher Simulator Bridge)

**Version:** 1.0

**Status:** Draft - Specification Phase

---

## 1. Scope & Objective

This document specifies the **Evolutionary Training Harness**, a system designed to optimize SHA-256 "Seeds" (weights) for the HASHER architecture. Since cryptographic hash functions are non-differentiable, traditional backpropagation is discarded in favor of **Group Relative Policy Optimization (GRPO)** logic implemented via **Evolutionary Strategies (ES)**.

The primary objective is to discover/establish a high-entropy mapping between input feature vectors and targeted linguistic tokens through a 21-pass recursive hashing loop.

---

## 2. Data Pipeline Integration (Input)

The training pipeline ingests data processed by the **Data Structuring Engine** (PDF-to-Parquet).

### 2.1 The Training Record

Standard `DocumentRecord` objects are transformed into `TrainingRecord` objects for the vHasher.

```go
type TrainingRecord struct {
    TokenSequence []int32     // Generated via Tiktoken/BPE from PDF content
    FeatureVector [12]uint32  // Normalized semantic embeddings (from Ollama)
    TargetToken   int32       // The "Label" (Next token in the sequence)
    ContextHash   uint32      // Rolling hash of the previous 5 tokens
}

```

### 2.2 Feature Encoding

Semantic embeddings (floats) from the ingestion engine are quantized into the 12-slot `uint32` array required by the `neural_frame`. This encoding acts as the "Encoder" stage, preparing the prompt for the ASIC.

---

## 3. The Evo-GRPO Harness Logic

We utilize the "Group Relative" principle to eliminate the need for a separate Critic model.

### 3.1 Group Sampling (Rollout)

For every training step (one prompt), the **vHasher** (HASHER Simulator) executes a "Group" of candidates:

* **Group Size ():** 64 to 128 seeds.
* **Input:** A single `FeatureVector` and `TargetToken`.
* **Execution:** Parallel 21-pass recursion across  seeds on the GPU.

### 3.2 Reward Function ()

The Orchestrator evaluates the output of each seed in the group based on three criteria:

1. **Alignment Reward ():** High if the Golden Nonce matches the `TargetToken` lookup.
2. **Stability Reward ():** Measured by the Hamming distance between the outputs of pass 20 and pass 21 (encouraging convergence).
3. **Format Reward ():** +1 if the nonce resolves to a valid entry in the `token_map`.

### 3.3 Advantage Calculation ()

Following GRPO principles, the advantage of a seed is its relative performance against the group mean ():


* **Positive Advantage:** The seed is a "Winner" (Parent for next generation).
* **Negative Advantage:** The seed is "Noise" (Discarded).


```go
package training

import (
    "math"
    "sort"
)

type SeedResult struct {
    SeedID   uint32
    Reward   float64
    Advantage float64 // The "Surprise" factor
}

// CalculateAdvantage implements the core logic of GRPO for our Evolutionary Harness.
func CalculateAdvantage(results []SeedResult) []SeedResult {
    if len(results) == 0 {
        return results
    }

    // 1. Calculate Group Mean Reward
    var totalReward float64
    for _, res := range results {
        totalReward += res.Reward
    }
    mean := totalReward / float64(len(results))

    // 2. Calculate Standard Deviation for Normalization (Optional but recommended)
    var varianceSum float64
    for _, res := range results {
        varianceSum += math.Pow(res.Reward-mean, 2)
    }
    stdDev := math.Sqrt(varianceSum / float64(len(results)))

    // 3. Calculate Relative Advantage
    // Higher than mean = Positive Advantage (Keep/Mutate)
    // Lower than mean = Negative Advantage (Discard)
    for i := range results {
        if stdDev > 0 {
            results[i].Advantage = (results[i].Reward - mean) / stdDev
        } else {
            results[i].Advantage = results[i].Reward - mean
        }
    }

    return results
}

// SelectAndMutate determines which seeds "win" and how to flip their bits.
func SelectAndMutate(results []SeedResult, currentSeeds map[uint32][]byte) map[uint32][]byte {
    // Sort by advantage (best first)
    sort.Slice(results, func(i, j int) bool {
        return results[i].Advantage > results[j].Advantage
    })

    newGeneration := make(map[uint32][]byte)
    topCount := len(results) / 4 // Keep top 25%

    for i := 0; i < len(results); i++ {
        if i < topCount {
            // WINNER: Keep this seed and create a mutated copy
            originalSeed := currentSeeds[results[i].SeedID]
            newGeneration[results[i].SeedID] = originalSeed // Keep original
            
            // Add a slightly mutated offspring
            mutatedID := results[i].SeedID + uint32(len(results))
            newGeneration[mutatedID] = BitwiseMutation(originalSeed, results[i].Advantage)
        }
    }
    return newGeneration
}
```
---

## 4. The Evolutionary Update (Policy Optimization)

### 4.1 Selection & Culling

* **Elite Preservation:** The top 25% of seeds by Advantage are saved directly to the **Parquet Weights Database**.
* **Culling:** The bottom 75% are deleted from the current simulation buffer.

### 4.2 Bitwise Mutation (The "Gradient")

New candidates are generated by mutating the winners. The mutation rate is inversely proportional to the Advantage:

* **High Advantage:** Low mutation (0.1% bit-flip) for fine-tuning.
* **Low Advantage:** High mutation (5% bit-flip) for broad exploration.

```go
func BitwiseMutation(seed []byte, advantage float64) []byte {
    mutated := make([]byte, len(seed))
    copy(mutated, seed)

    // Scaling mutation rate by inverse of advantage
    // High advantage = Low mutation (Stay near the winning path)
    mutationRate := int(10 / (math.Abs(advantage) + 1)) 

    for i := 0; i < mutationRate; i++ {
        byteIdx := rand.Intn(len(mutated))
        bitIdx := uint(rand.Intn(8))
        mutated[byteIdx] ^= (1 << bitIdx) // Flip a single bit
    }

    return mutated
}
```

---


## 5. Weights Persistence & Parquet Schema

Unlike traditional neural networks that store weights as floating-point tensors, the **HASHER** stores weights as **Target-Seed Pairs**.

### 5.1 The Evolutionary Parquet Schema

The training harness outputs a specialized Parquet file that maps Token IDs to the "Golden Seeds" discovered by the Evo-GRPO process.

| Column | Type | Description |
| --- | --- | --- |
| `token_id` | `INT32` | The target linguistic token (e.g., from Tiktoken). |
| `best_seed` | `BYTE_ARRAY` | The 32-byte seed that successfully triggered the token. |
| `fitness_score` | `FLOAT` | The final reward value at the time of selection. |
| `generation` | `INT32` | The evolutionary epoch when this seed was found. |
| `context_key` | `UINT32` | Optional: The 4-byte rolling hash of preceding tokens. |

### 5.2 Layered Weight Files

Because the Antminer S3 has limited RAM (64MB), we do not load all weights at once. Weights are partitioned into "Layer Files" (e.g., `layer_0.parquet`, `layer_1.parquet`). The Orchestrator "hot-swaps" these into the ASIC's BPF maps based on the conversation topic or depth.

---

## 6. Checkpointing & Validation

Training via evolution is non-linear; the system may find a "super-seed" that works for 90% of tokens but fails on logic. We implement a dual-check validation system.

### 6.1 Bbolt Checkpointing

Using the same `bbolt` infrastructure from your **Data Structuring Engine**, the harness tracks "Genetic Progress."

* **Key:** `token_id`
* **Value:** `hash_of_best_seed`
* **Purpose:** If the simulator crashes, the harness resumes training by loading the last known winners from the Parquet store and repopulating the vHasher group.

### 6.2 The Cross-Hardware Validator

Before a seed set is marked as "Production Ready," it must pass the **Validator Gate**:

1. The seed is tested in the **vHasher** (GPU).
2. The same seed is sent via gRPC to the **Antminer S3** (Physical ASIC).
3. The outputs are compared. If the Golden Nonces differ (due to hardware jitter or PIC timing), the seed is discarded as "Non-Deterministic" and marked for re-training.

---

## 7. The "Flash" Routine (Deployment)

Once a generation of seeds reaches the target fitness (e.g., 98% accuracy on the PDF training set), they are deployed to the edge.

1. **Optimization:** The Go Orchestrator reads the `best_seeds` from Parquet.
2. **Map Update:** It performs a bulk `bpf_map_update_elem` syscall on the OpenWRT kernel.
3. **Activation:** The eBPF XDP hook is notified of the map update, and the next incoming inference packet is processed using the new "Intelligence."

---

## 8. Summary of the Training Cycle

| Phase | Component | Speed | Goal |
| --- | --- | --- | --- |
| **Ingestion** | Data Engine (Go) | High | PDF  Tokenized Parquet |
| **Rollout** | vHasher (CUDA) | Massive | Test 128 seeds per prompt |
| **Selection** | Evo-GRPO Harness | High | Calculate Advantage & Mutate |
| **Persistence** | Parquet / bbolt | Persistent | Save winning seeds |
| **Inference** | ASIC (eBPF) | Ultra-Low Latency | Real-time Chat Completion |

---

### Final Implementation Note

This specification bridges your existing **Data Structuring Engine** with the **HASHER_SDD** hardware requirements. By treating the "Weights" as a searchable token-space in SHA-256, you bypass the need for gradients and turn the Antminer S3 into a functional, learning-capable neural node.

