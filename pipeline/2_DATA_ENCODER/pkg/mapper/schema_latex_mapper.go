package mapper

import (
	"regexp"
)

const (
	DOMAIN_MATH uint32 = 0x2000
)

type SchemaLaTeXMapper struct {
	schema *SlotSchema
}

func NewSchemaLaTeXMapper(schema *SlotSchema) *SchemaLaTeXMapper {
	return &SchemaLaTeXMapper{schema: schema}
}

func (m *SchemaLaTeXMapper) Map(input string) (NeuralFrame, error) {
	var slots [12]uint32

	slots[10] = m.schema.Domain.Slot10Base

	tokens := tokenizeMathInput(input)
	if len(tokens) > 0 {
		lastToken := tokens[len(tokens)-1]
		slots[4] = detectRoleFromSchema(lastToken, m.schema)
	}

	for i := 0; i < 4; i++ {
		slots[i] = hashContext(input, i)
	}

	slots[11] = generateTemporalLock(input)

	return NeuralFrame{
		Slots:         slots,
		TargetTokenID: 0,
		SourceRef:     "",
		Metadata:      map[string]interface{}{"domain": m.schema.Domain.Name},
	}, nil
}

func (m *SchemaLaTeXMapper) Schema() *SlotSchema {
	return m.schema
}

func tokenizeMathInput(s string) []string {
	var tokens []string
	var current string

	for _, r := range s {
		if r == ' ' || r == '\t' {
			if current != "" {
				tokens = append(tokens, current)
				current = ""
			}
			continue
		}
		current += string(r)
	}

	if current != "" {
		tokens = append(tokens, current)
	}

	return tokens
}

func detectRoleFromSchema(token string, schema *SlotSchema) uint32 {
	for _, role := range schema.Slot4Roles {
		for _, pattern := range role.Patterns {
			re := regexp.MustCompile(pattern)
			if re.MatchString(token) {
				return role.ID
			}
		}
	}
	return 0x01
}

func hashContext(s string, slot int) uint32 {
	h := uint32(len(s))
	for i, c := range s {
		rot := (uint32(c) << ((i + slot*3) % 24))
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

type SchemaVarianceMapper struct {
	schema *SlotSchema
}

func NewSchemaVarianceMapper(schema *SlotSchema) *SchemaVarianceMapper {
	return &SchemaVarianceMapper{schema: schema}
}

func (m *SchemaVarianceMapper) Map(input string) (NeuralFrame, error) {
	var slots [12]uint32

	slots[10] = m.schema.Domain.Slot10Base

	return NeuralFrame{
		Slots:         slots,
		TargetTokenID: 0,
		SourceRef:     "",
		Metadata:      map[string]interface{}{"domain": m.schema.Domain.Name},
	}, nil
}

func (m *SchemaVarianceMapper) Schema() *SlotSchema {
	return m.schema
}
