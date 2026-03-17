package mapper

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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

type Domain struct {
	Name       string `yaml:"name"`
	Slot10Base uint32 `yaml:"slot10_base"`
}

type SlotSchema struct {
	Domain          Domain           `yaml:"domain"`
	Subdomains      []Subdomain      `yaml:"subdomains"`
	Slot4Roles      []SlotRole       `yaml:"slot4_roles"`
	ValidationRules []ValidationRule `yaml:"validation_rules"`
	Slot4Source     string           `yaml:"slot4_source"`
	POSMappings     []POSMapping     `yaml:"pos_mappings"`
}

type POSMapping struct {
	ID        uint32   `yaml:"id"`
	Name      string   `yaml:"name"`
	SpaCyTags []string `yaml:"spaCy_tags"`
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

func (s *SlotSchema) GetSubdomain(id uint32) *Subdomain {
	for _, sd := range s.Subdomains {
		if sd.ID == id {
			return &sd
		}
	}
	return nil
}

func (s *SlotSchema) GetSlot4Role(id uint32) *SlotRole {
	for _, role := range s.Slot4Roles {
		if role.ID == id {
			return &role
		}
	}
	return nil
}

func (s *SlotSchema) GetValidationRule(name string) *ValidationRule {
	for _, rule := range s.ValidationRules {
		if rule.Name == name {
			return &rule
		}
	}
	return nil
}
