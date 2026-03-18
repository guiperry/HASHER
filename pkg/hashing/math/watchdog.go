package math

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"hasher/pkg/hashing/jitter"
)

// WatchdogRule describes a forbidden role-sequence loaded from a YAML schema.
type WatchdogRule struct {
	PrevRole      uint32   `yaml:"prev_role"`
	ForbiddenNext []uint32 `yaml:"forbidden_next"`
}

// watchdogSchemaFile is the minimal YAML structure needed to extract rules and domain.
type watchdogSchemaFile struct {
	Domain struct {
		Name string `yaml:"name"`
	} `yaml:"domain"`
	ValidationRules []WatchdogRule `yaml:"validation_rules"`
}

// LoadWatchdogRulesFromYAML reads validation_rules from a slot-schema YAML file.
func LoadWatchdogRulesFromYAML(path string) ([]WatchdogRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("watchdog schema load: %w", err)
	}
	var schema watchdogSchemaFile
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("watchdog schema parse: %w", err)
	}
	return schema.ValidationRules, nil
}

// LoadDomainFromSchema reads the domain.name field from a slot-schema YAML file.
// It is used by the host binary to decide which mode (HASHER / MATHASHER) to activate.
func LoadDomainFromSchema(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("schema load: %w", err)
	}
	var schema watchdogSchemaFile
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return "", fmt.Errorf("schema parse: %w", err)
	}
	return schema.Domain.Name, nil
}

type ValidationResult struct {
	Valid          bool
	Error          string
	LogicIntegrity float64
	Details        map[string]interface{}
}

type InferenceWatchdog struct {
	subdomain  uint32
	strictMode bool
	prevPOS    uint32
	rules      []WatchdogRule // nil → hard-coded fallback
}

func NewInferenceWatchdog(subdomain uint32) *InferenceWatchdog {
	if subdomain == 0 {
		subdomain = SUBDOMAIN_ARITHMETIC
	}
	return &InferenceWatchdog{
		subdomain:  subdomain,
		strictMode: true,
		prevPOS:    0,
	}
}

// NewWatchdogWithRules creates a watchdog using schema-loaded validation rules.
func NewWatchdogWithRules(subdomain uint32, rules []WatchdogRule) *InferenceWatchdog {
	if subdomain == 0 {
		subdomain = SUBDOMAIN_ARITHMETIC
	}
	return &InferenceWatchdog{
		subdomain:  subdomain,
		strictMode: true,
		prevPOS:    0,
		rules:      rules,
	}
}

func (w *InferenceWatchdog) ValidateMathStep(currentHash uint32, header [12]uint32) ValidationResult {
	result := ValidationResult{
		Valid:   true,
		Details: make(map[string]interface{}),
	}

	if header[10]&0xF000 != jitter.DOMAIN_MATH {
		result.Valid = false
		result.Error = "Domain not locked to Math mode (0x2000 range)"
		return result
	}

	currentPOS := header[4] & 0xFF

	if w.prevPOS != 0 {
		if len(w.rules) > 0 {
			// Schema-driven rules: check every loaded rule
			for _, rule := range w.rules {
				if w.prevPOS != rule.PrevRole {
					continue
				}
				for _, forbidden := range rule.ForbiddenNext {
					if currentPOS == forbidden {
						result.Valid = false
						result.Error = fmt.Sprintf(
							"HALLUCINATION DETECTED: role sequence %s → %s is forbidden",
							GetRoleName(w.prevPOS), GetRoleName(currentPOS),
						)
						result.Details["prevRole"] = GetRoleName(w.prevPOS)
						result.Details["currentRole"] = GetRoleName(currentPOS)
						result.LogicIntegrity = 0.0
						return result
					}
				}
			}
		} else {
			// Hard-coded fallback rules (used when no schema is loaded)
			if w.prevPOS == ROLE_OPERATOR && currentPOS != ROLE_VARIABLE && currentPOS != ROLE_INTEGER && currentPOS != ROLE_DECIMAL {
				result.Valid = false
				result.Error = "HALLUCINATION DETECTED: Operator cannot be followed by non-value"
				result.Details["prevRole"] = GetRoleName(w.prevPOS)
				result.Details["currentRole"] = GetRoleName(currentPOS)
				result.LogicIntegrity = 0.0
				return result
			}
			if w.prevPOS == ROLE_RELATION {
				if currentPOS == ROLE_OPERATOR || currentPOS == ROLE_FUNCTION {
					result.Valid = false
					result.Error = "HALLUCINATION DETECTED: Relation cannot be followed by operator/function"
					result.Details["prevRole"] = GetRoleName(w.prevPOS)
					result.Details["currentRole"] = GetRoleName(currentPOS)
					result.LogicIntegrity = 0.0
					return result
				}
			}
		}
	}

	w.prevPOS = currentPOS

	result.LogicIntegrity = 1.0
	result.Details["domain"] = GetSubDomainName(w.subdomain)
	result.Details["lastRole"] = GetRoleName(currentPOS)

	return result
}

func (w *InferenceWatchdog) ValidateDerivation(steps []DerivationStep) ValidationResult {
	result := ValidationResult{
		Valid:   true,
		Details: make(map[string]interface{}),
	}

	if len(steps) == 0 {
		result.Valid = false
		result.Error = "Empty derivation"
		return result
	}

	validSteps := 0
	for i, step := range steps {
		validation := w.ValidateMathStep(step.Hash, step.Header)
		if !validation.Valid {
			result.Valid = false
			result.Error = fmt.Sprintf("Step %d: %s", i, validation.Error)
			result.Details["failedStep"] = i
			break
		}
		validSteps++
	}

	result.Details["totalSteps"] = len(steps)
	result.Details["validSteps"] = validSteps
	result.LogicIntegrity = float64(validSteps) / float64(len(steps))

	return result
}

type DerivationStep struct {
	StepID   int
	Equation string
	Hash     uint32
	Header   [12]uint32
}

func DeriveMathSlots(subdomain uint32) (slots [12]uint32) {
	slots[10] = domainFromSubdomain(subdomain)
	slots[4] = ROLE_RELATION
	return
}

func BuildMathFrame(latex string, subdomain uint32, targetValue string) [12]uint32 {
	mapper := NewLaTeXMapper(subdomain)
	slots := mapper.MapLaTeXToTensor(latex, subdomain)

	if targetValue != "" {
		slots[4] = determineRole(targetValue)
	}

	return slots
}

func ValidateMathFrame(slots [12]uint32) error {
	domain := slots[10]
	if domain&0xF000 != jitter.DOMAIN_MATH {
		return fmt.Errorf("invalid domain: expected math mode (0x2000), got 0x%04x", domain)
	}

	role := slots[4] & 0xFF
	if role == 0 {
		return fmt.Errorf("invalid role: slot 4 is empty")
	}

	return nil
}

func IsMathDomain(domain uint32) bool {
	return domain&0xF000 == jitter.DOMAIN_MATH
}

func GetMathSubdomain(domain uint32) uint32 {
	if !IsMathDomain(domain) {
		return 0
	}
	return domain & 0x0F00
}
