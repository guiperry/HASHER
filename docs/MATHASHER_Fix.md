# MATHASHER Implementation Fix Report
**Branch:** `feature/math-logic` | **Audit Date:** 2026-03-17
**Scope:** Post-implementation audit of `docs/MATHASHER_Implementation.md` requirements

---

## Quick Answer: Are We Achieving the Two Core Architectural Goals?

> *"Abstract the Mapper: Create a generic `Map(input string) NeuralFrame` interface."*
> **NO.** No shared interface exists. The four mapper types (`Service`, `VarianceMapper`, `TensorPacker`, `LaTeXMapper`) each have different signatures and cannot be swapped.

> *"Schema-Driven Bits: Move the Bitmask Specifications into a configuration layer rather than hard-coding them in the Go source."*
> **NO.** POS IDs, domain signatures, and signal indices are all hard-coded as Go constants in `bitmask.go` and `packer.go`. Runtime schema loading does not exist.

Until these two goals are met, the HASHER cannot behave as either a General HASHER or a MATHASHER "depending on which schema it loads at runtime." Everything else in this report is a prerequisite for getting there.

---

## Part 1: Critical Bugs in the New `pkg/hashing/math/` Code

These bugs were introduced by the implementing agent and must be fixed regardless of the migration plan.

---

### BUG-1 — Watchdog `prevPOS` and `currentPOS` read the same slot (hallucination detection is non-functional)
**File:** `pkg/hashing/math/watchdog.go:44–45`

```go
// BROKEN: both variables point to the same slot in the same header
prevPOS     := header[4] & 0xFF
currentPOS  := header[4] & 0xFF
```

The spec's `ValidateMathStep` is meant to detect invalid *sequences* — e.g., OPERATOR followed by OPERATOR. But a single call receives only one header. There is no state retained between calls, so the check comparing `prevPOS` to `currentPOS` is always comparing a slot to itself. The hallucination guard will never fire for a cross-step sequence.

**Required Fix:** `InferenceWatchdog` must hold the previous POS as mutable state:
```go
type InferenceWatchdog struct {
    subdomain  uint32
    strictMode bool
    prevPOS    uint32  // ADD: retained across calls
}
```
Then `ValidateMathStep` reads `w.prevPOS` for the prior role, checks `currentPOS` from `header[4]`, then sets `w.prevPOS = currentPOS` before returning.

---

### BUG-2 — `RegisterMathVerifierService` is a no-op (gRPC server never wired)
**File:** `pkg/hashing/api/math_api.go:182–185`

```go
func RegisterMathVerifierService(grpcServer *grpc.Server, service MathVerifierService) {
    _ = service   // discarded
    _ = grpcServer // discarded
}
```

`StartServer()` calls this function after creating the gRPC server, but the function does nothing. No generated proto handler is registered. Any client calling `VerifyMath` will receive `codes.Unimplemented`.

**Required Fix:** Run `protoc` with `protoc-gen-go-grpc` to generate the server interface from `internal/proto/hasher/v1/hasher.proto`. Implement the generated `HasherServiceServer` interface on `MathVerifierServer`, then call `pb.RegisterHasherServiceServer(grpcServer, server)`.

---

### BUG-3 — `normalizeLatex` strips backslashes before role detection (all math functions misclassified)
**File:** `pkg/hashing/math/latex_mapper.go:78–85`

```go
func normalizeLatex(s string) string {
    s = strings.ReplaceAll(s, "\\left", "")
    s = strings.ReplaceAll(s, "\\right", "")
    s = strings.ReplaceAll(s, "\\", "")   // ← strips ALL remaining backslashes
    ...
}
```

`\int x^2 dx` normalizes to `int x^2 dx`. In `determineRole`, `variablePattern` (`[a-z]`) is checked **first** and matches `int`, classifying it as `ROLE_VARIABLE` (0x01) instead of `ROLE_FUNCTION` (0x05). All LaTeX commands (`\sin`, `\log`, `\sum`, `\partial`) suffer the same fate. The function and operator patterns that include backslashes become unreachable for all standard LaTeX inputs.

**Required Fix (two-part):**
1. Perform pattern matching **before** stripping backslashes, or rewrite the detection patterns to match backslash-free forms (`sin`, `log`, `int`).
2. Reorder `determineRole` to check more-specific patterns (function, operator) **before** the catch-all `variablePattern`.

---

### BUG-4 — `TrainingFrame.TargetTokenID` is never set by the miner (all training targets are 0 = `!`)
**File:** `pkg/hashing/miner/math_miner.go:56–70`

```go
func (m *MathMiner) processProof(p ProofRecord) jitter.TrainingFrame {
    slots := m.mapper.MapLaTeXToTensor(p.ProofStep, p.Domain)
    frame := jitter.TrainingFrame{
        AsicSlots:  slots,
        SourceFile: p.Theorem,
        // TargetTokenID is never set — defaults to 0
    }
    return frame
}
```

Token ID 0 maps to `!` in the cl100k_base vocabulary (confirmed in prior sessions — this is the same root cause as the `!!` output bug). Every mined training frame will have a target of 0, poisoning the Evo-GRPO trainer.

**Required Fix:** Derive a non-zero `TargetTokenID` from the proof step. The simplest defensible approach: tokenize the target result value using the same tokenizer used at inference time and take the first token ID. At minimum, any non-zero deterministic hash of the proof step is better than the implicit zero.

---

### BUG-5 — `SUB_*` constants collide with jitter domain constants (naming confusion causes incorrect domain routing)
**Files:** `pkg/hashing/math/bitmask.go` vs `pkg/hashing/jitter/types.go`

| Constant | Value | Collision |
|---|---|---|
| `math.SUB_arithmetic` | `0x2000` | = `jitter.DOMAIN_MATH` |
| `math.SUB_algebra` | `0x2100` | = `jitter.DOMAIN_LOGIC` |

Any code path that passes `SUB_algebra` into jitter machinery without first routing through `domainFromSubdomain()` will misidentify algebra frames as Logic/Code domain frames. The constants should be offsets or renamed to make their scope unambiguous.

**Required Fix:** Rename to `SUBDOMAIN_ARITHMETIC`, `SUBDOMAIN_ALGEBRA`, etc., and define them as pure offsets (`0x00`, `0x01`, `0x02`...) separate from the full domain signature space. `domainFromSubdomain()` remains the single place that assembles the full Slot 10 value.

---

## Part 2: Missed Architectural Context — The Existing Mapper Lives in `2_DATA_ENCODER`

The implementing agent placed the new math mapper in `pkg/hashing/math/` — which is **inside the hasher runtime**, not the data pipeline. This is architecturally incorrect.

The original variance-based mapper was built in **`pipeline/2_DATA_ENCODER/pkg/mapper/`** and it contains three distinct components:

| File | Type | Responsibility |
|---|---|---|
| `mapper.go` | `Service` | Random-projection of 768-dim BGE embeddings to 12 slots |
| `variance_mapper.go` | `VarianceMapper` | High-variance dimension selection from BGE embeddings |
| `packer.go` | `TensorPacker` | Full 12-slot frame construction: Slots 0–3 (identity), Slot 4 (POS/Tense), Slot 5 (dep hash), Slots 6–8 (memory), Slot 9 (intent flags), Slot 10 (domain), Slot 11 (token position) |

The `LaTeXMapper` in `pkg/hashing/math/latex_mapper.go` is a **Stage 2 data pipeline component** — it transforms raw input into training frames. It has no business living in the hasher runtime package. Its correct home is alongside `TensorPacker` in `pipeline/2_DATA_ENCODER/pkg/mapper/`.

---

## Part 3: Required Migration Plan

### Step 1 — Rename the pipeline stage directory

The `2_DATA_ENCODER` directory should be understood as the **Data Mapper** stage in the context of MATHASHER work. Consider renaming to `1_DATA_MAPPER` or treating it as such in documentation, since its primary output is the encoded/mapped training frames fed to `3_DATA_TRAINER`. The existing `1_DATA_MINER` feeds raw text into it.

> **Note:** If a physical directory rename is disruptive, a symlink or documentation alias is acceptable. The key point is that all mapper implementations belong here, not in `pkg/hashing/`.

### Step 2 — Move `LaTeXMapper` to the pipeline mapper package

Move these files:
```
FROM: pkg/hashing/math/latex_mapper.go
TO:   pipeline/2_DATA_ENCODER/pkg/mapper/latex_mapper.go

FROM: pkg/hashing/math/bitmask.go (constants only)
TO:   pipeline/2_DATA_ENCODER/pkg/mapper/schema_loader.go
      (replaced by YAML-loaded config — see Step 3)
```

The `watchdog.go` and API-layer files remain in `pkg/hashing/` because they are runtime validation components, not pipeline ETL components.

### Step 3 — Implement the YAML schema loader

Create `pipeline/2_DATA_ENCODER/pkg/mapper/schema_loader.go`. The schema file controls the "Meaning of Slot 4" and Slot 10 domain signatures at runtime.

**Schema file:** `pipeline/2_DATA_ENCODER/config/math_schema.yaml`
```yaml
# MATHASHER Slot Schema - Math Domain
# Controls the meaning of Slot 4 (Grammar/Role) and Slot 10 (Domain Signature)

domain:
  name: "MATHASHER"
  slot10_base: 0x2000  # Math domain base signature

subdomains:
  - id: 0x00
    name: "Arithmetic"
    slot10: 0x2000
  - id: 0x01
    name: "Algebra"
    slot10: 0x2100
  - id: 0x02
    name: "Calculus"
    slot10: 0x2200
  - id: 0x03
    name: "Statistics"
    slot10: 0x2300
  - id: 0x04
    name: "Logic/Set"
    slot10: 0x2400

slot4_roles:
  - id: 0x01
    name: "VARIABLE"
    description: "Symbolic placeholders"
    patterns: ["^[a-z]$", "alpha", "beta", "gamma", "theta", "lambda", "mu", "sigma", "omega"]
  - id: 0x02
    name: "OPERATOR"
    description: "Arithmetic or logical actions"
    patterns: ["\\+", "-", "\\times", "\\div", "\\cdot", "\\int", "\\sum", "\\prod", "\\partial", "\\nabla"]
  - id: 0x03
    name: "INTEGER"
    description: "Constant whole numbers"
    patterns: ["^\\d+$"]
  - id: 0x04
    name: "DECIMAL"
    description: "Floating-point or fractional values"
    patterns: ["^\\d+\\.\\d+$"]
  - id: 0x05
    name: "FUNCTION"
    description: "Named mathematical operations"
    patterns: ["\\sin", "\\cos", "\\tan", "\\log", "\\ln", "\\exp", "\\lim", "\\sqrt"]
  - id: 0x06
    name: "DELIMITER"
    description: "Structural boundaries"
    patterns: ["\\(", "\\)", "\\[", "\\]", "\\{", "\\}"]
  - id: 0x07
    name: "RELATION"
    description: "Comparative logic"
    patterns: ["=", "<", ">", "\\leq", "\\geq", "\\approx", "\\equiv", "\\neq"]
  - id: 0x08
    name: "EXPONENT"
    description: "Power or root indicators"
    patterns: ["\\^", "\\sqrt\\{"]

validation_rules:
  - name: "operator_cannot_follow_operator"
    prev_role: 0x02
    forbidden_next: [0x02, 0x07]
  - name: "relation_cannot_follow_operator"
    prev_role: 0x07
    forbidden_next: [0x02, 0x05]
```

**General HASHER prose schema:** `pipeline/2_DATA_ENCODER/config/prose_schema.yaml`
```yaml
# General HASHER Slot Schema - Prose Domain
domain:
  name: "HASHER"
  slot10_base: 0x1000

subdomains:
  - id: 0x00
    name: "Prose"
    slot10: 0x1000
  - id: 0x01
    name: "Academic"
    slot10: 0x1100

# Slot 4 uses spaCy POS tags (loaded from packer.go — no change needed here)
slot4_source: "spacy_pos"
```

**Schema loader interface in Go:**
```go
// pipeline/2_DATA_ENCODER/pkg/mapper/schema_loader.go

type SlotRole struct {
    ID          uint32   `yaml:"id"`
    Name        string   `yaml:"name"`
    Description string   `yaml:"description"`
    Patterns    []string `yaml:"patterns"`
}

type Subdomain struct {
    ID     uint32 `yaml:"id"`
    Name   string `yaml:"name"`
    Slot10 uint32 `yaml:"slot10"`
}

type ValidationRule struct {
    Name          string   `yaml:"name"`
    PrevRole      uint32   `yaml:"prev_role"`
    ForbiddenNext []uint32 `yaml:"forbidden_next"`
}

type SlotSchema struct {
    Domain struct {
        Name       string `yaml:"name"`
        Slot10Base uint32 `yaml:"slot10_base"`
    } `yaml:"domain"`
    Subdomains      []Subdomain      `yaml:"subdomains"`
    Slot4Roles      []SlotRole       `yaml:"slot4_roles"`
    ValidationRules []ValidationRule `yaml:"validation_rules"`
}

func LoadSchema(path string) (*SlotSchema, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("schema load: %w", err)
    }
    var schema SlotSchema
    if err := yaml.Unmarshal(data, &schema); err != nil {
        return nil, fmt.Errorf("schema parse: %w", err)
    }
    return &schema, nil
}
```

### Step 4 — Implement the abstract `Mapper` interface

**File:** `pipeline/2_DATA_ENCODER/pkg/mapper/interface.go`

```go
package mapper

// NeuralFrame is the canonical 12-slot ASIC-ready tensor.
// This is the single output type for all mapper implementations.
type NeuralFrame struct {
    Slots         [12]uint32
    TargetTokenID int32
    SourceRef     string // theorem, source file, etc.
    Metadata      map[string]interface{}
}

// Mapper is the generic interface all domain mappers must satisfy.
// The HASHER uses VarianceMapper; the MATHASHER uses LaTeXMapper.
// Both are selected at runtime by loading the appropriate schema.
type Mapper interface {
    Map(input string) (NeuralFrame, error)
    Schema() *SlotSchema
}

// MapperFactory returns the correct Mapper for a given schema.
func MapperFactory(schema *SlotSchema) (Mapper, error) {
    switch schema.Domain.Name {
    case "MATHASHER":
        return NewSchemaLatexMapper(schema), nil
    case "HASHER":
        return NewSchemaVarianceMapper(schema), nil
    default:
        return nil, fmt.Errorf("unknown domain: %s", schema.Domain.Name)
    }
}
```

`LaTeXMapper` and `VarianceMapper` both implement `Mapper` by receiving a `*SlotSchema` at construction time instead of hard-coded constants. Role detection patterns are compiled from `schema.Slot4Roles[*].Patterns` at startup.

---

## Part 4: Output Format — JSON and Arrow (Parquet Deprecated)

The miner currently outputs JSON only. The pipeline's `2_DATA_ENCODER/pkg/schema/arrow.go` already has a fully working `WriteTrainingFramesToArrowIPC` function. The math miner must use it.

**Required change to `math_miner.go`:**

```go
// saveFrames writes both JSON (human-readable) and Arrow IPC (trainer-ready)
func (m *MathMiner) saveFrames(frames []schema.TrainingFrame, basePath string) error {
    // 1. JSON output
    jsonPath := basePath + ".json"
    data, err := json.MarshalIndent(frames, "", "  ")
    if err != nil {
        return fmt.Errorf("json marshal: %w", err)
    }
    if err := os.WriteFile(jsonPath, data, 0644); err != nil {
        return fmt.Errorf("json write: %w", err)
    }

    // 2. Arrow IPC output (consumed by 3_DATA_TRAINER)
    arrowPath := basePath + ".arrow"
    if err := schema.WriteTrainingFramesToArrowIPC(arrowPath, frames); err != nil {
        return fmt.Errorf("arrow write: %w", err)
    }

    return nil
}
```

**Important:** The miner must use `pipeline/2_DATA_ENCODER/pkg/schema.TrainingFrame` (the flat struct with individual `AsicSlots0`...`AsicSlots11` fields and Arrow tags), **not** `pkg/hashing/jitter.TrainingFrame` (the embedded struct used by the runtime). These are two different types serving different pipeline stages. The miner bridges between them.

---

## Part 5: Spec Deviations (Non-Blocking but Required Before Production)

### DEV-1 — Semantic Anchors (Slots 0–3) use polynomial hash, not BGE-Base embeddings

**File:** `pkg/hashing/math/latex_mapper.go:186–192`

The spec mandates Cloudflare BGE-Base worker embeddings for semantic grounding. The implementation uses `h = h*31 + rot` — a content-addressable hash with no semantic meaning. Two mathematically distinct but syntactically similar expressions (e.g., `x + 2` and `y + 2`) will produce completely different slot values, which is correct, but two semantically equivalent expressions (`2 + x` and `x + 2`) will also produce different values, defeating the purpose of semantic anchoring.

**Recommended Fix:** The migrated `LaTeXMapper` should call the same `embeddings.Service` used by `VarianceMapper` in `2_DATA_ENCODER/pkg/embeddings/service.go` to obtain a real BGE-Base vector, then apply variance-selected dimensions to Slots 0–3. This is only possible after the migration in Part 3 is complete, since the embeddings service is not available in the hasher runtime package.

### DEV-2 — `DetokenizedOutput` in API response echoes input instead of the computed result

**File:** `pkg/hashing/api/math_api.go:45,135`

```go
DetokenizedOutput: latex,  // ← returns input unchanged
```

The spec example shows `"detokenized_output": "9"` for `∫₀³ x² dx`. This field is supposed to contain the verified *result*. Currently it is meaningless. Fixing this requires the inference pipeline to produce an actual result value, which depends on BUG-2 and BUG-4 being resolved first.

### DEV-3 — Strategy Pattern for domain-switching watchdog not implemented

The spec calls for the `InferenceWatchdog` to use a Strategy Pattern so the hasher-host can switch between `MathValidationStrategy` and `GeneralProseStrategy` based on Slot 10. Currently the watchdog is math-only with no integration into the general inference path. Once schema loading (Part 3) is in place, the watchdog's validation rules should be loaded from `schema.ValidationRules` rather than hard-coded, making the strategy implicit in the schema itself.

### DEV-4 — No `cmd` entry point for the Math Verification API

There is no `cmd/driver/math-verifier/main.go`. The API package cannot be deployed as a standalone service. This is a blocking gap for the HashNet Plugin API described in the spec. A minimal `main.go` that calls `api.StartServer(api.DefaultServerConfig())` is needed, plus a `Makefile` target to build it.

---

## Part 6: Summary of All Required Changes

### Priority 1 — Blocking (Code is broken)
| # | File | Action |
|---|---|---|
| BUG-2 | `pkg/hashing/api/math_api.go` | Wire gRPC handler via generated proto code |
| BUG-3 | `pkg/hashing/math/latex_mapper.go` | Fix backslash stripping / role detection order |
| BUG-1 | `pkg/hashing/math/watchdog.go` | Add stateful `prevPOS` field |
| BUG-4 | `pkg/hashing/miner/math_miner.go` | Set `TargetTokenID` in `processProof()` |

### Priority 2 — Architecture (Spec goals not met)
| # | Location | Action |
|---|---|---|
| ARC-1 | `pipeline/2_DATA_ENCODER/pkg/mapper/` | Create `interface.go` with `Mapper` interface and `NeuralFrame` type |
| ARC-2 | `pipeline/2_DATA_ENCODER/pkg/mapper/` | Migrate `latex_mapper.go` from `pkg/hashing/math/` |
| ARC-3 | `pipeline/2_DATA_ENCODER/pkg/mapper/` | Create `schema_loader.go` with YAML struct + `LoadSchema()` |
| ARC-4 | `pipeline/2_DATA_ENCODER/config/` | Create `math_schema.yaml` and `prose_schema.yaml` |
| ARC-5 | `pipeline/2_DATA_ENCODER/pkg/mapper/packer.go` | Replace hard-coded domain detection with schema-driven dispatch |
| ARC-6 | `pkg/hashing/math/bitmask.go` | Rename `SUB_*` constants to avoid jitter collision (BUG-5) |

### Priority 3 — Output Format
| # | File | Action |
|---|---|---|
| OUT-1 | `pkg/hashing/miner/math_miner.go` | Add Arrow IPC output alongside JSON; use `schema.TrainingFrame` |

### Priority 4 — Production Readiness
| # | Item | Action |
|---|---|---|
| PRD-1 | `cmd/driver/math-verifier/` | Create standalone server entry point |
| PRD-2 | `LaTeXMapper` | Replace polynomial hash in Slots 0–3 with BGE-Base embeddings |
| PRD-3 | `InferenceWatchdog` | Load validation rules from YAML schema |
| PRD-4 | API response | Implement actual `DetokenizedOutput` resolution |

---

## Appendix: The HASHER-as-OS Mental Model

The refactored system should work as follows at runtime:

```
# Run as General HASHER (prose mode)
./hasher-host --schema config/prose_schema.yaml

# Run as MATHASHER (math verification mode)
./hasher-host --schema config/math_schema.yaml
```

The single binary remains unchanged. All that changes is which schema file is loaded. The `MapperFactory` reads the schema's `domain.name` field and returns the correct `Mapper` implementation. The `InferenceWatchdog` reads `validation_rules` from the same schema. No recompilation, no build tags, no separate repos.

This is the correct end state. None of the current code achieves it.
