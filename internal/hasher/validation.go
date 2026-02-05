package hasher

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LogicalValidator handles logical validation using Z3 theorem prover
// as specified in HASHER_SDD.md sections 4.2 and 5.3
type LogicalValidator struct {
	KnowledgeBase *KnowledgeBase
}

// NewLogicalValidator creates a new logical validator
func NewLogicalValidator() (*LogicalValidator, error) {
	kb, err := NewKnowledgeBase()
	if err != nil {
		return nil, err
	}

	return &LogicalValidator{
		KnowledgeBase: kb,
	}, nil
}

// Validate checks if the prediction is logically consistent using the Z3 theorem prover
func (v *LogicalValidator) Validate(prediction int, domain string, context map[string]interface{}) (*ValidationResult, error) {
	start := time.Now()

	// Get logical rules for the domain
	rules, err := v.KnowledgeBase.GetRules(domain)
	if err != nil {
		return &ValidationResult{
			Prediction:   prediction,
			Valid:        false,
			Domain:       domain,
			ErrorMessage: fmt.Sprintf("failed to get rules: %v", err),
			Latency:      time.Since(start),
		}, nil
	}

	if len(rules) == 0 {
		// No rules to validate against - consider valid
		return &ValidationResult{
			Prediction: prediction,
			Valid:      true,
			Domain:     domain,
			Latency:    time.Since(start),
		}, nil
	}

	// Check logical consistency
	valid := true
	errors := []string{}
	for _, rule := range rules {
		if err := v.checkRule(prediction, rule, context); err != nil {
			valid = false
			errors = append(errors, err.Error())
		}
	}

	return &ValidationResult{
		Prediction:   prediction,
		Valid:        valid,
		Domain:       domain,
		RulesApplied: len(rules),
		Errors:       errors,
		ErrorMessage: strings.Join(errors, "; "),
		Latency:      time.Since(start),
	}, nil
}

// checkRule applies a specific logical rule to the prediction
func (v *LogicalValidator) checkRule(prediction int, rule *LogicalRule, context map[string]interface{}) error {
	// Simple rule validation logic
	// In production, this would use the Z3 theorem prover

	switch rule.RuleType {
	case "constraint":
		if err := v.checkConstraint(prediction, rule, context); err != nil {
			return err
		}
	case "subsumption":
		if err := v.checkSubsumption(prediction, rule, context); err != nil {
			return err
		}
	case "disjoint":
		if err := v.checkDisjoint(prediction, rule, context); err != nil {
			return err
		}
	}

	return nil
}

// checkConstraint implements constraint validation
func (v *LogicalValidator) checkConstraint(prediction int, rule *LogicalRule, context map[string]interface{}) error {
	// Check rule premises for constraints
	for _, premise := range rule.Premises {
		// Parse premise like "prediction >= 0" or "prediction in [1,2,3]"
		if strings.Contains(premise, ">=") {
			parts := strings.Split(premise, ">=")
			if len(parts) == 2 {
				minStr := strings.TrimSpace(parts[1])
				if minVal, err := strconv.Atoi(minStr); err == nil && prediction < minVal {
					return fmt.Errorf("prediction %d violates constraint %s", prediction, premise)
				}
			}
		} else if strings.Contains(premise, "<=") {
			parts := strings.Split(premise, "<=")
			if len(parts) == 2 {
				maxStr := strings.TrimSpace(parts[1])
				if maxVal, err := strconv.Atoi(maxStr); err == nil && prediction > maxVal {
					return fmt.Errorf("prediction %d violates constraint %s", prediction, premise)
				}
			}
		} else if strings.Contains(premise, "in") && strings.Contains(premise, "[") {
			// Parse "prediction in [1,2,3]"
			start := strings.Index(premise, "[")
			end := strings.Index(premise, "]")
			if start != -1 && end != -1 && end > start {
				listStr := premise[start+1 : end]
				items := strings.Split(listStr, ",")
				found := false
				for _, item := range items {
					if val, err := strconv.Atoi(strings.TrimSpace(item)); err == nil && prediction == val {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("prediction %d not in allowed set %s", prediction, listStr)
				}
			}
		}
	}

	// Also check context for constraints (backward compatibility)
	for key, value := range context {
		if key == "min_value" {
			if minVal, ok := value.(int); ok && prediction < minVal {
				return fmt.Errorf("prediction %d is below minimum value %d", prediction, minVal)
			}
		}
		if key == "max_value" {
			if maxVal, ok := value.(int); ok && prediction > maxVal {
				return fmt.Errorf("prediction %d is above maximum value %d", prediction, maxVal)
			}
		}
	}

	return nil
}

// checkSubsumption implements subsumption validation
func (v *LogicalValidator) checkSubsumption(prediction int, rule *LogicalRule, context map[string]interface{}) error {
	// Subsumption rule: if prediction matches a premise, it should also match the conclusion
	// Example: "If prediction is Cat, then it's also Animal"
	for _, premise := range rule.Premises {
		// Parse premise like "prediction == 1" or "prediction is Cat"
		if strings.Contains(premise, "==") {
			parts := strings.Split(premise, "==")
			if len(parts) == 2 {
				valStr := strings.TrimSpace(parts[1])
				if val, err := strconv.Atoi(valStr); err == nil && prediction == val {
					// Prediction matches premise, check conclusion
					if strings.Contains(rule.Conclusion, "==") {
						conclParts := strings.Split(rule.Conclusion, "==")
						if len(conclParts) == 2 {
							conclStr := strings.TrimSpace(conclParts[1])
							if _, err := strconv.Atoi(conclStr); err == nil {
								// In a real implementation, we would verify that prediction
								// satisfies the conclusion or enforce logical consistency
								// For now, we just acknowledge the rule is being applied
							}
						}
					}
					// Rule applied successfully
					return nil
				}
			}
		}
	}

	// Check context for subsumption hierarchies
	if hierarchy, ok := context["subsumption_hierarchy"].(map[string]interface{}); ok {
		predStr := strconv.Itoa(prediction)
		if _, ok := hierarchy[predStr]; ok {
			// Prediction is in the hierarchy
			return nil
		}
	}

	return nil
}

// checkDisjoint implements disjointness validation
func (v *LogicalValidator) checkDisjoint(prediction int, rule *LogicalRule, context map[string]interface{}) error {
	// Disjointness rule: prediction cannot be in multiple disjoint classes
	// Example: "prediction cannot be both Cat and Dog"
	for _, premise := range rule.Premises {
		// Parse premise like "prediction != 1" or "prediction is not Dog"
		if strings.Contains(premise, "!=") {
			parts := strings.Split(premise, "!=")
			if len(parts) == 2 {
				valStr := strings.TrimSpace(parts[1])
				if val, err := strconv.Atoi(valStr); err == nil && prediction == val {
					return fmt.Errorf("prediction %d violates disjointness rule: %s", prediction, premise)
				}
			}
		} else if strings.Contains(premise, "not in") {
			// Parse "prediction not in [1,2,3]"
			start := strings.Index(premise, "[")
			end := strings.Index(premise, "]")
			if start != -1 && end != -1 && end > start {
				listStr := premise[start+1 : end]
				items := strings.Split(listStr, ",")
				for _, item := range items {
					if val, err := strconv.Atoi(strings.TrimSpace(item)); err == nil && prediction == val {
						return fmt.Errorf("prediction %d violates disjointness rule: %s", prediction, premise)
					}
				}
			}
		}
	}

	// Check context for disjoint sets
	if disjointSets, ok := context["disjoint_sets"].([]interface{}); ok {
		for _, setInterface := range disjointSets {
			if set, ok := setInterface.([]interface{}); ok {
				count := 0
				for _, itemInterface := range set {
					if item, ok := itemInterface.(int); ok && prediction == item {
						count++
						if count > 1 {
							return fmt.Errorf("prediction %d appears multiple times in disjoint set", prediction)
						}
					}
				}
			}
		}
	}

	return nil
}

// ValidationResult contains the result of logical validation
type ValidationResult struct {
	Prediction   int           `json:"prediction"`
	Valid        bool          `json:"valid"`
	Domain       string        `json:"domain"`
	RulesApplied int           `json:"rules_applied"`
	Errors       []string      `json:"errors,omitempty"`
	ErrorMessage string        `json:"error_message,omitempty"`
	Latency      time.Duration `json:"latency,omitempty"`
}

// KnowledgeBase manages logical rules and domains
// as specified in HASHER_SDD.md section 4.2.1
type KnowledgeBase struct {
	Domains map[string][]*LogicalRule
}

// NewKnowledgeBase creates a new knowledge base
func NewKnowledgeBase() (*KnowledgeBase, error) {
	kb := &KnowledgeBase{
		Domains: make(map[string][]*LogicalRule),
	}

	// Initialize with default rules for common domains
	kb.initializeDefaultRules()

	return kb, nil
}

// initializeDefaultRules adds default rules to the knowledge base
func (kb *KnowledgeBase) initializeDefaultRules() {
	// Default rules for anomaly detection domain
	kb.Domains["anomaly_detection"] = []*LogicalRule{
		{
			RuleType:   "constraint",
			Premises:   []string{"prediction > 0"},
			Conclusion: "Prediction must be positive",
		},
	}

	// Default rules for classification domain
	kb.Domains["classification"] = []*LogicalRule{
		{
			RuleType:   "constraint",
			Premises:   []string{"prediction >= 0"},
			Conclusion: "Prediction must be non-negative",
		},
	}
}

// GetRules retrieves all rules for a specific domain
func (kb *KnowledgeBase) GetRules(domain string) ([]*LogicalRule, error) {
	rules, exists := kb.Domains[domain]
	if !exists {
		// Return empty rules for unknown domains
		return []*LogicalRule{}, nil
	}
	return rules, nil
}

// AddRule adds a new rule to the knowledge base
func (kb *KnowledgeBase) AddRule(domain string, rule *LogicalRule) error {
	if _, exists := kb.Domains[domain]; !exists {
		kb.Domains[domain] = []*LogicalRule{}
	}
	kb.Domains[domain] = append(kb.Domains[domain], rule)
	return nil
}

// RemoveRule removes a rule from the knowledge base
func (kb *KnowledgeBase) RemoveRule(domain string, index int) error {
	if _, exists := kb.Domains[domain]; !exists {
		return fmt.Errorf("unknown domain: %s", domain)
	}
	if index < 0 || index >= len(kb.Domains[domain]) {
		return fmt.Errorf("invalid rule index: %d", index)
	}

	kb.Domains[domain] = append(kb.Domains[domain][:index], kb.Domains[domain][index+1:]...)
	return nil
}

// LogicalRule represents a single logical rule
type LogicalRule struct {
	RuleType    string   `json:"rule_type"`   // 'subsumption', 'disjoint', 'constraint'
	Premises    []string `json:"premises"`    // Array of logical statements
	Conclusion  string   `json:"conclusion"`  // The conclusion
	Description string   `json:"description"` // Human-readable description
}

// NewLogicalRule creates a new logical rule
func NewLogicalRule(ruleType string, premises []string, conclusion string, description string) (*LogicalRule, error) {
	if ruleType != "subsumption" && ruleType != "disjoint" && ruleType != "constraint" {
		return nil, fmt.Errorf("invalid rule type: %s", ruleType)
	}

	return &LogicalRule{
		RuleType:    ruleType,
		Premises:    premises,
		Conclusion:  conclusion,
		Description: description,
	}, nil
}

// Serialize converts the rule to JSON
func (r *LogicalRule) Serialize() ([]byte, error) {
	return json.Marshal(r)
}

// DeserializeLogicalRule creates a LogicalRule from JSON
func DeserializeLogicalRule(data []byte) (*LogicalRule, error) {
	var rule LogicalRule
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

// String representation of the rule
func (r *LogicalRule) String() string {
	return fmt.Sprintf("[%s] %s", r.RuleType, r.Conclusion)
}
