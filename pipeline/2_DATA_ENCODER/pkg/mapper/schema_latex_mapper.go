package mapper

import (
	"fmt"
	"regexp"

	hashermath "hasher/pkg/hashing/math"
)

// compiledRole holds a pre-compiled set of patterns for a single Slot-4 role.
type compiledRole struct {
	id       uint32
	patterns []*regexp.Regexp
}

// SchemaLaTeXMapper implements Mapper for the MATHASHER domain.
//
// Tokenization is delegated to hasher/pkg/hashing/math.LaTeXMapper so the
// single-pass splitting logic lives in one place.  Role detection is
// schema-driven: patterns from the loaded SlotSchema override the runtime
// hard-coded patterns, letting the same binary serve different math domains
// by swapping YAML files.
type SchemaLaTeXMapper struct {
	schema        *SlotSchema
	compiledRoles []compiledRole
	latexMapper   *hashermath.LaTeXMapper // canonical tokenizer from the runtime package
}

func NewSchemaLaTeXMapper(schema *SlotSchema) *SchemaLaTeXMapper {
	m := &SchemaLaTeXMapper{
		schema:      schema,
		latexMapper: hashermath.NewLaTeXMapper(0),
	}

	for _, role := range schema.Slot4Roles {
		cr := compiledRole{id: role.ID}
		for _, pat := range role.Patterns {
			re, err := regexp.Compile(pat)
			if err != nil {
				// Skip invalid patterns — log for visibility rather than panicking.
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

	// Tokenize using the canonical runtime tokenizer (single source of truth).
	tokens := m.latexMapper.TokenizeLaTeX(input)
	if len(tokens) > 0 {
		last := tokens[len(tokens)-1]
		// Re-classify the last token with schema-driven patterns.
		slots[4] = m.detectRole(last.Text)
	}

	// Slots 0–3: semantic anchors (hash-based until BGE-Base embeddings are wired).
	slots[0], slots[1], slots[2], slots[3] = hashermath.GetSemanticAnchors(input)

	// Slot 11: temporal lock.
	slots[11] = hashermath.GenerateTemporalLock(input)

	return NeuralFrame{
		Slots:     slots,
		SourceRef: input,
		Metadata:  map[string]interface{}{"domain": m.schema.Domain.Name},
	}, nil
}

func (m *SchemaLaTeXMapper) Schema() *SlotSchema {
	return m.schema
}

// detectRole returns the Slot-4 role ID for token using pre-compiled schema patterns.
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

// SchemaVarianceMapper implements Mapper for the general HASHER prose domain.
// Slots 0–3 use the same deterministic hash as the runtime LaTeXMapper until the
// BGE-Base embedding service is integrated.
type SchemaVarianceMapper struct {
	schema         *SlotSchema
	varianceMapper *VarianceMapper
}

func NewSchemaVarianceMapper(schema *SlotSchema) *SchemaVarianceMapper {
	return &SchemaVarianceMapper{
		schema:         schema,
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

	// Slots 0–3: hash-based semantic anchors (BGE-Base integration pending).
	slots[0], slots[1], slots[2], slots[3] = hashermath.GetSemanticAnchors(input)

	// Slot 10: domain from schema.
	slots[10] = m.schema.Domain.Slot10Base

	// Slot 11: temporal lock.
	slots[11] = hashermath.GenerateTemporalLock(input)

	return NeuralFrame{
		Slots:     slots,
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
