package mapper

import "strings"

// TensorPacker orchestrates bit-level placement of metadata into the 12-slot format
type TensorPacker struct {
	signalIndices []int
}

// NewTensorPacker creates a new packer with variance-selected indices for slots 0-3
func NewTensorPacker(signalIndices []int) *TensorPacker {
	return &TensorPacker{
		signalIndices: signalIndices,
	}
}

// PackFrame creates the 12-slot bitmask tensor according to the HASHER-MAPPER spec
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

	// Zone 1: Identity (Slots 0-3)
	// We use the top 4 dimensions from our importance/variance map.
	// We quantize them to 32-bit uint32 to preserve maximal entropy for the hash search.
	for i := 0; i < 4 && i < len(p.signalIndices); i++ {
		dimIdx := p.signalIndices[i]
		slots[i] = quantizeFloatToUint32(embedding[dimIdx])
	}

	// Slot 4: Grammar (POS Tag ID, Tense / Mood ID)
	// Bits 0-7: POS | Bits 8-15: Tense
	slots[4] = uint32(pos) | (uint32(tense) << 8)

	// Slot 5: Syntax (Dependency Link Hash)
	slots[5] = depHash

	// Slots 6-8: Memory (Recursive Summary - Last 10 Headers XORed)
	for i := 0; i < 3 && i < len(lastHeaders); i++ {
		slots[6+i] = lastHeaders[i]
	}

	// Slot 9: Intent (Flags: Question, Command, Code)
	// Bit 0: IS_QUESTION | Bit 1: IS_CODE
	if isQuestion(instruction) {
		slots[9] |= 0x1
	}
	if isCode(input) || isCode(instruction) {
		slots[9] |= 0x2
	}

	// Slot 10: Domain Signature
	slots[10] = detectDomain(instruction, input)

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
	// Scale -1.0...1.0 to 0...4294967295
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
	codeIndicators := []string{"func ", "var ", "import ", "{", "}", "[]", "const ", "def ", "class "}
	for _, ind := range codeIndicators {
		if strings.Contains(s, ind) {
			return true
		}
	}
	return false
}

func detectDomain(instr, input string) uint32 {
	instr = strings.ToLower(instr)
	input = strings.ToLower(input)
	
	// Math Mode
	mathIndicators := []string{"calculate", "math", "equation", "solve", "sum", "multiply", "divide", "+", "-", "*", "/"}
	for _, ind := range mathIndicators {
		if strings.Contains(instr, ind) || strings.Contains(input, ind) {
			return 0x2000
		}
	}
	
	// Code Mode
	if isCode(input) || strings.Contains(instr, "code") || strings.Contains(instr, "program") || strings.Contains(instr, "function") {
		return 0x3000
	}
	
	// Default Prose Mode
	return 0x1000
}

func quantizeFloatToUint16(val float32) uint16 {
	if val < -1.0 { val = -1.0 }
	if val > 1.0 { val = 1.0 }
	scaled := (val + 1.0) / 2.0 * 65535.0
	return uint16(scaled + 0.5)
}
