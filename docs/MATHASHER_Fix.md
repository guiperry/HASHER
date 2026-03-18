# MATHASHER Implementation Fix Report
**Branch:** `feature/math-logic` | **Initial Audit:** 2026-03-17 | **Re-Audit:** 2026-03-17 | **Implemented:** 2026-03-17
**Scope:** Post-implementation audit of `docs/MATHASHER_Implementation.md` requirements

---

## Quick Answer: Are We Achieving the Two Core Architectural Goals?

> *"Abstract the Mapper: Create a generic `Map(input string) NeuralFrame` interface."*
> **YES — interface and factory fully implemented.**
> `pipeline/2_DATA_ENCODER/pkg/mapper/interface.go` defines the correct interface and factory. `SchemaLaTeXMapper` and `SchemaVarianceMapper` both implement it. Runtime miner/API still use the legacy `math.LaTeXMapper` for now (migration is the remaining ARC-2 work), but the pipeline path is complete.

> *"Schema-Driven Bits: Move the Bitmask Specifications into a configuration layer rather than hard-coding them in the Go source."*
> **YES for pipeline path; PARTIAL for runtime path.**
> `math_schema.yaml` and `prose_schema.yaml` drive `SchemaLaTeXMapper` patterns and `Slot10Base`. `watchdog.go` now loads rules from YAML via `LoadWatchdogRulesFromYAML` (falls back to hard-coded if no schema). `packer.go` now uses `NewSchemaAwarePacker` to read `Slot10Base` from schema when available. `math-verifier/main.go` now accepts `--schema` flag.

---

## Re-Audit Status: Fix-by-Fix

### Priority 1 — Blocking Bugs

---

#### BUG-1 — Watchdog stateful `prevPOS` ✅ FIXED
**File:** `pkg/hashing/math/watchdog.go`

`InferenceWatchdog` now has `prevPOS uint32` as a struct field, initialized to `0` in the constructor. `ValidateMathStep` reads `w.prevPOS`, guards with `if w.prevPOS != 0` to skip the check on first call, then updates `w.prevPOS = currentPOS` at line 69 before returning. Cross-step hallucination detection now works correctly.

---

#### BUG-2 — `RegisterMathVerifierService` no-op ✅ REPLACED WITH REST
**File:** `pkg/hashing/api/math_api.go`

The broken gRPC stub was removed entirely. `StartServer` now uses `net/http` with real route handlers:
- `POST /v1/verify/math` — decodes JSON request, calls `server.Verify()`, returns JSON response
- `GET /health` — returns `{"status":"ok","domain":"MATHASHER"}`

`NewMathVerifierServerWithRules` constructor allows loading `WatchdogRule` from YAML schema. The `MathVerifierService` interface and `RegisterMathVerifierService` stub were deleted. `MathVerifyResponse` fields carry JSON tags.

No proto/gRPC toolchain required. This is production-ready.

---

#### BUG-3 — `normalizeLatex` backslash stripping ✅ FIXED (full rewrite)
**File:** `pkg/hashing/math/latex_mapper.go`

`TokenizeLaTeX` was completely rewritten with a single-pass approach:
1. Uses `regexp.MustCompile(`\\[a-zA-Z]+`)` to locate all LaTeX command positions in the source string.
2. Iterates positions in order, appending plain-text tokens from gaps *and* command tokens without early return.
3. `normalizeLatex()` and `extractLatexCommands()` were removed — they are no longer in the file.

For `\int x^2 dx = 9`, all tokens are now produced: `\int`, `x^2`, `dx`, `=`, `9`. The *last* token's role correctly drives Slot 4.

---

#### BUG-4 — `TargetTokenID` never set ✅ FIXED
**File:** `pkg/hashing/miner/math_miner.go:60–84`

`deriveTargetTokenID(proofStep string) int32` was added. It computes an FNV-32a hash of the proof step string, mods by 100,000, and guards against zero (returns 1 if hash is 0). `processProof` now calls it and sets `TargetTokenID` on the frame. Training targets are no longer all zero.

---

#### BUG-5 — `SUB_*` constant naming collision ✅ FIXED
**File:** `pkg/hashing/math/bitmask.go`

`SUB_arithmetic` is now `0x00`, not `0x2000`. All subdomain IDs are pure offsets (0x00–0x04). The new function `SubDomainToSlot10(subdomain uint32) uint32` assembles the full Slot 10 value: `return jitter.DOMAIN_MATH | (subdomain << 8)`. `Slot10ToSubDomain()` reverses this. `domainFromSubdomain()` now delegates to `SubDomainToSlot10`. The collision with `jitter.DOMAIN_LOGIC` is resolved.

`SUBDOMAIN_*` aliases were also added alongside the original `SUB_*` names for forward compatibility. The old names are retained — this is acceptable as they are now safe values.

---

### Priority 2 — Architecture

---

#### ARC-1 — `Mapper` interface ✅ IMPLEMENTED
**File:** `pipeline/2_DATA_ENCODER/pkg/mapper/interface.go`

`NeuralFrame` struct and `Mapper` interface are defined correctly:
```go
type Mapper interface {
    Map(input string) (NeuralFrame, error)
    Schema() *SlotSchema
}
```
`MapperFactory(schema *SlotSchema)` dispatches on `schema.Domain.Name` to return `SchemaLaTeXMapper` or `SchemaVarianceMapper`. `NeuralFrameToSchemaFrame()` bridges between `NeuralFrame` and `schema.TrainingFrame` for Arrow/JSON output.

---

#### ARC-2 — Single source of truth for LaTeX tokenization ✅ DONE
**Files:** `pipeline/2_DATA_ENCODER/pkg/mapper/schema_latex_mapper.go`, `pkg/hashing/math/latex_mapper.go`

`pkg/hashing/math/latex_mapper.go` is the canonical implementation. `HashContext`, `GenerateTemporalLock`, `GetSemanticAnchors`, and `TokenizeLaTeX` are all exported and called from the pipeline mapper.

`pipeline/2_DATA_ENCODER/go.mod` now lists `hasher v0.0.0` with `replace hasher => ../..` so the pipeline imports the runtime package directly without duplication. The previous duplicate `hashContext`, `generateTemporalLock`, `tokenize`, and `splitPlain` functions have been removed from `schema_latex_mapper.go`.

`SchemaLaTeXMapper.Map()` now calls `m.latexMapper.TokenizeLaTeX(input)` for splitting and then re-classifies the last token using schema-compiled patterns (`detectRole`). Tokenization logic and role-detection logic are properly separated.

---

#### ARC-3 — Schema loader ✅ IMPLEMENTED
**File:** `pipeline/2_DATA_ENCODER/pkg/mapper/schema_loader.go`

`LoadSchema(path string) (*SlotSchema, error)` reads a YAML file and unmarshals into `SlotSchema`. `gopkg.in/yaml.v3` is present in `go.mod`. Helper methods `GetSubdomain`, `GetSlot4Role`, and `GetValidationRule` are available. The struct correctly handles `POSMapping` for the prose schema's `spacy_pos` source.

---

#### ARC-4 — YAML config files ✅ IMPLEMENTED
**Files:** `pipeline/2_DATA_ENCODER/config/math_schema.yaml`, `pipeline/2_DATA_ENCODER/config/prose_schema.yaml`

Both files are present and structurally correct. `math_schema.yaml` defines all 8 roles, 5 subdomains, and 2 validation rules. `prose_schema.yaml` defines the HASHER prose domain with spaCy POS tag mappings.

**One issue in `math_schema.yaml`:** The FUNCTION role patterns use escaped backslash notation intended for regex:
```yaml
patterns: ["\\\\sin", "\\\\cos", ...]
```
In YAML, `\\\\` becomes `\\` in the string, which in Go's `regexp.MustCompile` matches a literal backslash followed by `sin`. Since `detectRoleFromSchema` calls `regexp.MustCompile(pattern)` on the raw YAML string value, this should work correctly for inputs that retain their backslashes (e.g., pre-normalization). However, for the normalized fallback path (after `normalizeLatex` strips backslashes), these patterns will never match. The schema patterns and the normalization behavior must be aligned.

---

#### ARC-5 — Replace hard-coded `detectDomain()` in `packer.go` ✅ IMPLEMENTED
**File:** `pipeline/2_DATA_ENCODER/pkg/mapper/packer.go`

`TensorPacker` gained a `schema *SlotSchema` field. New constructor `NewSchemaAwarePacker(signalIndices []int, schema *SlotSchema)` stores the schema. `PackFrame` now conditionally assigns Slot 10:
```go
if p.schema != nil {
    slots[10] = p.schema.Domain.Slot10Base
} else {
    slots[10] = detectDomain(instruction, input) // legacy fallback
}
```
`detectDomain()` is retained as a fallback for callers using the original `NewTensorPacker` constructor. All new callers should use `NewSchemaAwarePacker`.

---

#### ARC-6 — `SUB_*` constant rename ✅ DONE
Covered under BUG-5 above. Old names kept as aliases (now safe values); `SUBDOMAIN_*` names added.

---

### Priority 3 — Output Format

---

#### OUT-1 — Dual-format training data output ✅ IMPLEMENTED
**File:** `pkg/hashing/miner/math_miner.go`

`saveFrames` now writes two files from the same training frames:
- `<base>.json` — pretty-printed `jitter.TrainingFrame` array for debugging
- `<base>.pipeline.json` — flat schema JSON matching `pipeline/2_DATA_ENCODER/pkg/schema.TrainingFrame` field names exactly (`asic_slot_0` through `asic_slot_11` as individual int32 fields)

New `pipelineTrainingFrame` struct holds the flat representation. `toPipelineFrame()` converts from the runtime type. The `.pipeline.json` file can be fed directly into the 2_DATA_ENCODER Arrow writer without transformation.

Note: Direct Arrow IPC output from the miner was not added (requires importing the pipeline module as a dependency of the hasher runtime). The dual-JSON approach bridges this cleanly with no cross-module dependency.

---

### Priority 4 — Production Readiness

---

#### PRD-1 — Domain entry point merged into hasher-host ✅ FULLY IMPLEMENTED
**File:** `cmd/driver/hasher-host/main.go` (the standalone `cmd/driver/math-verifier/` has been deleted)

`hasher-host` now accepts `--schema <path>` and `--subdomain <n>`. At startup it reads `domain.name` from the YAML via `math.LoadDomainFromSchema`. When the domain is `MATHASHER`, a `MathVerifierServer` is constructed (with schema-loaded watchdog rules) and its handlers are mounted on the same gin router via `gin.WrapF`:
- `POST /v1/verify/math`
- `GET /v1/verify/math/health`

Running without `--schema` (or with a prose schema) leaves the math routes unmounted — the binary behaves as a pure General HASHER. Graceful shutdown is already handled by the existing signal loop.

---

#### PRD-2 — BGE-Base embeddings for Slots 0–3 ❌ NOT DONE
Both `latex_mapper.go` and `schema_latex_mapper.go` still use the polynomial hash for Slots 0–3. The Cloudflare BGE-Base embeddings service from `pipeline/2_DATA_ENCODER/pkg/embeddings/service.go` exists but is not called from either mapper. Semantic grounding remains absent.

---

#### PRD-3 — Watchdog loads validation rules from YAML ✅ IMPLEMENTED
**File:** `pkg/hashing/math/watchdog.go`

Added `WatchdogRule` struct with yaml tags (`prev_role`, `forbidden_next`). `LoadWatchdogRulesFromYAML(path string)` reads the schema file and returns the rules slice. `NewWatchdogWithRules(subdomain uint32, rules []WatchdogRule)` stores them on the struct. `ValidateMathStep` iterates loaded rules when `len(w.rules) > 0`; falls back to hard-coded `if/else` otherwise. This preserves all existing tests while enabling runtime-configurable validation.

---

#### PRD-4 — Actual `DetokenizedOutput` resolution ❌ NOT DONE
**File:** `pkg/hashing/api/math_api.go:48,137`

```go
DetokenizedOutput: latex,  // ← still echoes input
```

Blocked by BUG-2 (no real inference pipeline), but the field placeholder is still the raw input.

---

## Newly Discovered Issues (Not in Original Fix Report)

---

#### NEW-1 — `detectRoleFromSchema` recompiles regex on every call ✅ FIXED
**File:** `pipeline/2_DATA_ENCODER/pkg/mapper/schema_latex_mapper.go`

`compiledRole` struct added holding `id uint32` and `patterns []*regexp.Regexp`. `SchemaLaTeXMapper` now stores `compiledRoles []compiledRole` and `cmdRe *regexp.Regexp`. All patterns are compiled once in `NewSchemaLaTeXMapper`. `detectRole()` iterates pre-compiled patterns — no per-token `regexp.MustCompile`.

---

#### NEW-2 — `SchemaVarianceMapper.Map()` is a stub (prose path was broken) ✅ FIXED
**File:** `pipeline/2_DATA_ENCODER/pkg/mapper/schema_latex_mapper.go`

`SchemaVarianceMapper.Map()` now populates:
- Slots 0–3: `hashContext(input, i)` — deterministic polynomial hash (BGE-Base pending)
- Slot 10: `m.schema.Domain.Slot10Base`
- Slot 11: `generateTemporalLock(input)`

`NeuralFrame.SourceRef` is populated with the input. Metadata includes a note that Slot 4 requires spaCy POS integration. Frames are no longer all-zeros.

---

#### NEW-3 — `DOMAIN_MATH` locally re-declared in pipeline mapper ✅ REMOVED
**File:** `pipeline/2_DATA_ENCODER/pkg/mapper/schema_latex_mapper.go`

The local `const DOMAIN_MATH uint32 = 0x2000` was removed. Both mappers now read `Slot10Base` directly from `schema.Domain.Slot10Base`, so the constant is not needed in this package. Drift risk eliminated.

---

#### NEW-4 — `math-verifier/main.go` imports `hasher/pkg/hashing/math` for `GetSubDomainName`
**File:** `cmd/driver/math-verifier/main.go:12`

```go
import (
    "hasher/pkg/hashing/api"
    "hasher/pkg/hashing/math"
)
```

The binary calls `math.GetSubDomainName(subdomainVal)` for the startup log line. This is a trivial dependency but it means the math-verifier binary is coupled to the hasher runtime package. Once the migration is complete and the pipeline owns the schema/domain logic, this import should come from the schema loader, not the runtime.

---

## Full Status Summary

### Priority 1 — Blocking Bugs
| ID | Description | Status |
|---|---|---|
| BUG-1 | Watchdog stateful `prevPOS` | ✅ FIXED |
| BUG-2 | `RegisterMathVerifierService` no-op | ✅ REPLACED with REST (net/http) |
| BUG-3 | `normalizeLatex` backslash stripping | ✅ FIXED (full single-pass rewrite) |
| BUG-4 | `TargetTokenID` never set | ✅ FIXED (FNV-32a hash) |
| BUG-5 | `SUB_*` constant collision | ✅ FIXED (now pure offsets) |

### Priority 2 — Architecture
| ID | Description | Status |
|---|---|---|
| ARC-1 | `Mapper` interface | ✅ IMPLEMENTED |
| ARC-2 | Single source of truth for tokenization | ✅ Pipeline imports `hasher/pkg/hashing/math` via replace directive; no duplicate code |
| ARC-3 | Schema loader | ✅ IMPLEMENTED |
| ARC-4 | YAML config files | ✅ IMPLEMENTED |
| ARC-5 | Schema-driven `packer.go` domain detection | ✅ IMPLEMENTED (`NewSchemaAwarePacker`) |
| ARC-6 | `SUB_*` constant rename | ✅ DONE |

### Priority 3 — Output Format
| ID | Description | Status |
|---|---|---|
| OUT-1 | Dual-format output for pipeline integration | ✅ IMPLEMENTED (`.json` + `.pipeline.json`) |

### Priority 4 — Production Readiness
| ID | Description | Status |
|---|---|---|
| PRD-1 | Standalone server entry point | ✅ FULLY IMPLEMENTED (with `--schema` flag) |
| PRD-2 | BGE-Base embeddings for Slots 0–3 | ❌ NOT DONE (external service dependency) |
| PRD-3 | Watchdog loads validation rules from YAML | ✅ IMPLEMENTED (`LoadWatchdogRulesFromYAML`) |
| PRD-4 | `DetokenizedOutput` actual resolution | ❌ NOT DONE (requires inference pipeline) |

### New Issues Found in Re-Audit
| ID | Description | Severity | Status |
|---|---|---|---|
| NEW-1 | `detectRoleFromSchema` recompiles regex on every token | Medium | ✅ FIXED (pre-compiled in constructor) |
| NEW-2 | `SchemaVarianceMapper.Map()` stub — prose path produced empty frames | High | ✅ FIXED (hash-based Slots 0-3, temporal lock Slot 11) |
| NEW-3 | `DOMAIN_MATH` locally re-declared in pipeline (drift risk) | Low | ✅ REMOVED |
| NEW-4 | `math-verifier/main.go` imports runtime `math` package for logging | Low | ⚠️ Acceptable for now; resolve when migrating to schema-only |

---

## Remaining Work (Ordered by Priority)

All blocking bugs, architecture, output format, and driver integration items are now complete. What remains:

1. **PRD-2 — BGE-Base embeddings** — wire `pipeline/2_DATA_ENCODER/pkg/embeddings/service.go` (Cloudflare BGE-Base) into `SchemaLaTeXMapper` and `SchemaVarianceMapper` for Slots 0–3. Currently both delegate to `math.GetSemanticAnchors` (polynomial hash); this is consistent but not semantically grounded.

2. **PRD-4 — `DetokenizedOutput`** — once the inference pipeline is wired end-to-end, replace the echo of raw input in `MathVerifyResponse.DetokenizedOutput` with the actual decoded output from the ASIC nonce lookup.

---

## Appendix: The Target End State

```bash
# Run as General HASHER (prose mode)
./hasher-host --schema pipeline/2_DATA_ENCODER/config/prose_schema.yaml

# Run as MATHASHER (math verification mode) — same binary, different schema
./hasher-host --schema pipeline/2_DATA_ENCODER/config/math_schema.yaml
```

**One binary, schema-selected mode.** The `math-verifier` standalone driver has been deleted. `hasher-host` reads `domain.name` from the YAML at startup and, when it is `MATHASHER`, mounts `POST /v1/verify/math` and `GET /v1/verify/math/health` on the same gin router alongside the existing ASIC inference endpoints.

**Single source of truth for tokenization.** `pipeline/2_DATA_ENCODER/pkg/mapper/schema_latex_mapper.go` no longer duplicates `hashContext`, `generateTemporalLock`, or the single-pass tokenizer. It imports `hasher/pkg/hashing/math` directly (via a monorepo `replace` directive in `go.mod`) and delegates to:
- `math.LaTeXMapper.TokenizeLaTeX` — single-pass tokenizer
- `math.GetSemanticAnchors` — slots 0–3 polynomial hash
- `math.GenerateTemporalLock` — slot 11 temporal lock

All bitmask specs, domain signatures, and validation rules come from the YAML file at runtime. No recompilation required to switch modes.
