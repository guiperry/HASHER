package mapper

import (
	"fmt"
	"regexp"
)

// compiledRole holds a pre-compiled set of patterns for a single Slot-4 role.
type compiledRole struct {
	id       uint32
	patterns []*regexp.Regexp
}

// SchemaLaTeXMapper implements Mapper for the MATHASHER domain.
// Role detection patterns are compiled once at construction from the loaded SlotSchema.
type SchemaLaTeXMapper struct {
	schema         *SlotSchema
	compiledRoles  []compiledRole
	cmdRe          *regexp.Regexp
}

func NewSchemaLaTeXMapper(schema *SlotSchema) *SchemaLaTeXMapper {
	m := &SchemaLaTeXMapper{
		schema: schema,
		cmdRe:  regexp.MustCompile(`\\[a-zA-Z]+`),
	}

	for _, role := range schema.Slot4Roles {
		cr := compiledRole{id: role.ID}
		for _, pat := range role.Patterns {
			re, err := regexp.Compile(pat)
			if err != nil {
				// Skip invalid patterns rather than panicking — log for visibility
				fmt.Printf("Warning: invalid pattern %q for role %s: %v\n", pat, role.Name, err)
				continue
			}
			cr.patterns = append(cr.patterns, re)
		}
		m.compiledRoles = append(m.compiledRoles, cr)
	}

	return m
}

// Map converts a LaTeX expression to a NeuralFrame using schema-defined roles and domain.
func (m *SchemaLaTeXMapper) Map(input string) (NeuralFrame, error) {
	var slots [12]uint32

	slots[10] = m.schema.Domain.Slot10Base

	tokens := m.tokenize(input)
	if len(tokens) > 0 {
		last := tokens[len(tokens)-1]
		slots[4] = m.detectRole(last)
	}

	for i := range 4 {
		slots[i] = hashContext(input, i)
	}
	slots[11] = generateTemporalLock(input)

	return NeuralFrame{
		Slots:    slots,
		SourceRef: input,
		Metadata: map[string]interface{}{"domain": m.schema.Domain.Name},
	}, nil
}

func (m *SchemaLaTeXMapper) Schema() *SlotSchema {
	return m.schema
}

// tokenize performs a single-pass extraction: LaTeX commands are identified
// before normalization so their backslash prefixes are available for pattern matching.
func (m *SchemaLaTeXMapper) tokenize(input string) []string {
	if input == "" {
		return nil
	}

	var tokens []string
	lastEnd := 0

	for _, loc := range m.cmdRe.FindAllStringIndex(input, -1) {
		// Plain text before this command
		for _, t := range splitPlain(input[lastEnd:loc[0]]) {
			if t != "" {
				tokens = append(tokens, t)
			}
		}
		// The command itself (backslash retained)
		tokens = append(tokens, input[loc[0]:loc[1]])
		lastEnd = loc[1]
	}

	// Remaining plain text
	for _, t := range splitPlain(input[lastEnd:]) {
		if t != "" {
			tokens = append(tokens, t)
		}
	}

	if len(tokens) == 0 && input != "" {
		tokens = []string{input}
	}

	return tokens
}

// splitPlain breaks a plain (backslash-free) string on whitespace and operator boundaries.
func splitPlain(s string) []string {
	if s == "" {
		return nil
	}
	// Reuse the same boundary characters as the runtime tokenizer
	var out []string
	cur := ""
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n':
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		case r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}':
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			out = append(out, string(r))
		default:
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// detectRole returns the Slot-4 role ID for a token using pre-compiled schema patterns.
// Patterns are checked in schema order (FUNCTION before VARIABLE avoids misclassification).
func (m *SchemaLaTeXMapper) detectRole(token string) uint32 {
	for _, cr := range m.compiledRoles {
		for _, re := range cr.patterns {
			if re.MatchString(token) {
				return cr.id
			}
		}
	}
	return 0x01 // fallback: VARIABLE
}

// hashContext and generateTemporalLock are defined here to avoid importing the
// runtime math package (separate module boundary).
func hashContext(s string, slot int) uint32 {
	h := uint32(len(s))
	for i, c := range s {
		rot := uint32(c) << ((i + slot*3) % 24)
		h = h*31 + rot
	}
	return h
}

func generateTemporalLock(s string) uint32 {
	h := uint32(0)
	for i, c := range s {
		h += uint32(c) * uint32(i+1)
	}
	return h ^ 0xCAFEBABE
}

// SchemaVarianceMapper implements Mapper for the general HASHER prose domain.
// When a real BGE-Base embedding is not available, Slots 0–3 are filled with a
// deterministic hash of the input — consistent but not semantically grounded.
// Wire to VarianceMapper.MapToSlots once the embeddings service is integrated.
type SchemaVarianceMapper struct {
	schema        *SlotSchema
	varianceMapper *VarianceMapper
}

func NewSchemaVarianceMapper(schema *SlotSchema) *SchemaVarianceMapper {
	return &SchemaVarianceMapper{
		schema:        schema,
		varianceMapper: NewDefaultVarianceMapper(),
	}
}

// Map produces a NeuralFrame for prose input.
// Slots 0–3: deterministic hash (polynomial) until BGE-Base service is wired in.
// Slot 4:    0 (POS tag requires spaCy; use TensorPacker.PackFrame for full prose frames).
// Slot 10:   schema.Domain.Slot10Base (e.g. 0x1000 for HASHER prose).
// Slot 11:   temporal lock.
func (m *SchemaVarianceMapper) Map(input string) (NeuralFrame, error) {
	var slots [12]uint32

	// Slots 0–3: hash-based semantic anchors (BGE-Base integration pending)
	for i := range 4 {
		slots[i] = hashContext(input, i)
	}

	// Slot 10: domain from schema
	slots[10] = m.schema.Domain.Slot10Base

	// Slot 11: temporal lock
	slots[11] = generateTemporalLock(input)

	return NeuralFrame{
		Slots:    slots,
		SourceRef: input,
		Metadata: map[string]interface{}{
			"domain": m.schema.Domain.Name,
			"note":   "Slot 4 requires spaCy POS — use TensorPacker.PackFrame for full frames",
		},
	}, nil
}

func (m *SchemaVarianceMapper) Schema() *SlotSchema {
	return m.schema
}
