package mapper

import "strings"

// TensorPacker orchestrates bit-level placement of metadata into the 12-slot format.
// When a SlotSchema is provided via NewSchemaAwarePacker, Slot 10 is taken directly
// from schema.Domain.Slot10Base instead of being auto-detected from content keywords.
type TensorPacker struct {
	signalIndices []int
	schema        *SlotSchema // optional; nil = keyword-based auto-detection
}

// NewTensorPacker creates a packer that auto-detects the domain from content keywords.
func NewTensorPacker(signalIndices []int) *TensorPacker {
	return &TensorPacker{signalIndices: signalIndices}
}

// NewSchemaAwarePacker creates a packer whose Slot 10 domain value is driven by the schema.
// Use this when the domain is known at construction time (e.g. loaded from a YAML schema file).
func NewSchemaAwarePacker(signalIndices []int, schema *SlotSchema) *TensorPacker {
	return &TensorPacker{signalIndices: signalIndices, schema: schema}
}

// PackFrame creates the 12-slot bitmask tensor according to the HASHER-MAPPER spec.
// When the packer was created with NewSchemaAwarePacker, Slot 10 uses the schema's
// domain base; otherwise it falls back to keyword-based detection.
func (p *TensorPacker) PackFrame(
	embedding []float32,
	pos uint8,
	tense uint8,
	depHash uint32,
	lastHeaders []uint32,
	tokenPos uint16,
	instruction string,
	input string,
) [12]uint32 {
	var slots [12]uint32

	// Zone 1: Identity (Slots 0–3)
	for i := 0; i < 4 && i < len(p.signalIndices); i++ {
		dimIdx := p.signalIndices[i]
		slots[i] = quantizeFloatToUint32(embedding[dimIdx])
	}

	// Slot 4: Grammar (POS Tag ID | Tense/Mood ID)
	slots[4] = uint32(pos) | (uint32(tense) << 8)

	// Slot 5: Syntax (Dependency Link Hash)
	slots[5] = depHash

	// Slots 6–8: Memory (Recursive Summary — last headers XORed)
	for i := 0; i < 3 && i < len(lastHeaders); i++ {
		slots[6+i] = lastHeaders[i]
	}

	// Slot 9: Intent flags
	if isQuestion(instruction) {
		slots[9] |= 0x1
	}
	if isCode(input) || isCode(instruction) {
		slots[9] |= 0x2
	}

	// Slot 10: Domain Signature — schema-driven when available, keyword fallback otherwise
	if p.schema != nil {
		slots[10] = p.schema.Domain.Slot10Base
	} else {
		slots[10] = detectDomain(instruction, input)
	}

	// Slot 11: Lock (Token Position Index)
	slots[11] = uint32(tokenPos)

	return slots
}

func quantizeFloatToUint32(val float32) uint32 {
	if val < -1.0 {
		val = -1.0
	}
	if val > 1.0 {
		val = 1.0
	}
	scaled := (float64(val) + 1.0) / 2.0 * 4294967295.0
	return uint32(scaled + 0.5)
}

func isQuestion(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "?") ||
		strings.HasPrefix(s, "what") ||
		strings.HasPrefix(s, "how") ||
		strings.HasPrefix(s, "why") ||
		strings.HasPrefix(s, "can you")
}

func isCode(s string) bool {
	s = strings.ToLower(s)
	for _, ind := range []string{"func ", "var ", "import ", "{", "}", "[]", "const ", "def ", "class "} {
		if strings.Contains(s, ind) {
			return true
		}
	}
	return false
}

// detectDomain is the legacy keyword-based fallback used when no schema is loaded.
// Prefer NewSchemaAwarePacker for explicit domain control.
func detectDomain(instr, input string) uint32 {
	instr = strings.ToLower(instr)
	input = strings.ToLower(input)

	for _, ind := range []string{"calculate", "math", "equation", "solve", "sum", "multiply", "divide", "+", "-", "*", "/"} {
		if strings.Contains(instr, ind) || strings.Contains(input, ind) {
			return 0x2000 // DOMAIN_MATH
		}
	}

	if isCode(input) || strings.Contains(instr, "code") || strings.Contains(instr, "program") || strings.Contains(instr, "function") {
		return 0x3000 // DOMAIN_CODE
	}

	return 0x1000 // DOMAIN_PROSE
}

func quantizeFloatToUint16(val float32) uint16 {
	if val < -1.0 {
		val = -1.0
	}
	if val > 1.0 {
		val = 1.0
	}
	scaled := (val + 1.0) / 2.0 * 65535.0
	return uint16(scaled + 0.5)
}
