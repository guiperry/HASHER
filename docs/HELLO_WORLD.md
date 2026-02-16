# HASHER "Hello World" Chat Demo

This document outlines the steps to perform a successful "Hello World" demonstration using the HASHER inference system.

## 1. Optimal Data Set Generation

A synthetic dataset has been generated in `~/.local/share/hasher/data/frames/training_frames.json`. This dataset specifically maps conversational patterns into the 80-byte Bitcoin header format used by the trainer, optimized for the default 1000-token vocabulary.

### Training Patterns
- **Greeting**: "Hello" → " world" → "!"
- **Identity**: "What is" → " your" → " name" → "?"

## 2. Execution Steps for the Demo

To perform the demonstration, follow these steps in order:

### Step 1: Train the Demo Model
Run the evolutionary trainer on the generated frames to discover the "Golden Seeds" for the target transitions.
```bash
cd pipeline/3_DATA_TRAINER
./bin/data-trainer -epochs 20 -difficulty-bits 12
```
*Note: You should see "[WIN]" logs for token transitions like 906 → 917.*

### Step 2: Start the Inference Host
In a separate terminal, start the orchestrator with crypto-transformer enabled.
```bash
./bin/hasher-host --crypto=true --vocab-size=1000
```

### Step 3: Perform the Chat Demo
Launch the HASHER CLI, navigate to **Test Chat** (Option 4), and test the following interactions:

| User Prompt | Expected System Response |
|-------------|-------------------------|
| `Hello` | ` world!` |
| `What is your name?` | ` Hasher` |

## 3. Scaling Beyond "Hello World"

For more advanced training, use the **TinyStories** dataset (Microsoft Research). This dataset contains grammatically simple stories that are ideal for the HASHER "Crypto-Transformer" architecture and its unique RAM/hardware constraints.

- **Dataset**: [HuggingFace - TinyStories](https://huggingface.co/datasets/roneneldan/TinyStories)
- **Tooling**: Use the `1_DATA_MINER` sub-module to ingest and structure larger datasets for full-scale inference deployment.
