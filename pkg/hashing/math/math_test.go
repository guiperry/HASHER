package math

import (
	"testing"

	"hasher/pkg/hashing/jitter"
)

func TestMathRoles(t *testing.T) {
	tests := []struct {
		name     string
		latex    string
		expected uint32
	}{
		{"Variable x", "x", ROLE_VARIABLE},
		{"Variable y", "y", ROLE_VARIABLE},
		{"Variable theta", "theta", ROLE_VARIABLE},
		{"Addition operator", "+", ROLE_OPERATOR},
		{"Subtraction operator", "-", ROLE_OPERATOR},
		{"Integer", "42", ROLE_INTEGER},
		{"Decimal", "3.14", ROLE_DECIMAL},
		{"Integer", "100", ROLE_INTEGER},
		{"Delimiter (", "(", ROLE_DELIMITER},
		{"Relation =", "=", ROLE_RELATION},
		{"Exponent ^2", "^2", ROLE_EXPONENT},
	}

	mapper := NewLaTeXMapper(SUB_arithmetic)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := mapper.TokenizeLaTeX(tt.latex)
			if len(tokens) == 0 {
				t.Errorf("No tokens found for %s", tt.latex)
				return
			}

			lastToken := tokens[len(tokens)-1]
			if lastToken.Role != tt.expected {
				t.Errorf("Expected role %d, got %d for %s", tt.expected, lastToken.Role, tt.latex)
			}
		})
	}
}

func TestMapLaTeXToTensor(t *testing.T) {
	mapper := NewLaTeXMapper(SUB_arithmetic)

	slots := mapper.MapLaTeXToTensor("x + y", SUB_arithmetic)

	if slots[10] != jitter.DOMAIN_MATH {
		t.Errorf("Expected domain 0x%08x, got 0x%08x", jitter.DOMAIN_MATH, slots[10])
	}

	if slots[4] == 0 {
		t.Error("Slot 4 should not be zero")
	}
}

func TestSubDomainMapping(t *testing.T) {
	tests := []struct {
		name      string
		subdomain uint32
		expected  string
	}{
		{"Arithmetic", SUB_arithmetic, "Arithmetic"},
		{"Algebra", SUB_algebra, "Algebra"},
		{"Calculus", SUB_calculus, "Calculus"},
		{"Statistics", SUB_statistics, "Statistics"},
		{"Logic", SUB_logic, "Logic/Set"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSubDomainName(tt.subdomain)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetRoleName(t *testing.T) {
	tests := []struct {
		role     uint32
		expected string
	}{
		{ROLE_VARIABLE, "VARIABLE"},
		{ROLE_OPERATOR, "OPERATOR"},
		{ROLE_INTEGER, "INTEGER"},
		{ROLE_DECIMAL, "DECIMAL"},
		{ROLE_FUNCTION, "FUNCTION"},
		{ROLE_DELIMITER, "DELIMITER"},
		{ROLE_RELATION, "RELATION"},
		{ROLE_EXPONENT, "EXPONENT"},
		{0xFF, "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := GetRoleName(tt.role)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestWatchdogValidation(t *testing.T) {
	watchdog := NewInferenceWatchdog(SUB_arithmetic)

	validSlots := [12]uint32{
		0, 0, 0, 0,
		ROLE_VARIABLE,
		0, 0, 0, 0, 0,
		jitter.DOMAIN_MATH,
		0xCAFEBABE,
	}

	result := watchdog.ValidateMathStep(0, validSlots)
	if !result.Valid {
		t.Errorf("Expected valid result, got: %s", result.Error)
	}

	operatorSlots := [12]uint32{
		0, 0, 0, 0,
		ROLE_OPERATOR,
		0, 0, 0, 0, 0,
		jitter.DOMAIN_MATH,
		0xCAFEBABE,
	}

	result2 := watchdog.ValidateMathStep(0, operatorSlots)
	if !result2.Valid {
		t.Errorf("First step with operator should be valid, got: %s", result2.Error)
	}

	result3 := watchdog.ValidateMathStep(0, operatorSlots)
	if result3.Valid {
		t.Error("Expected invalid result for operator followed by operator")
	}
}

func TestValidateMathFrame(t *testing.T) {
	validFrame := [12]uint32{
		0x11111111, 0x22222222, 0x33333333, 0x44444444,
		ROLE_INTEGER,
		0, 0, 0, 0, 0,
		jitter.DOMAIN_MATH,
		0xCAFEBABE,
	}

	err := ValidateMathFrame(validFrame)
	if err != nil {
		t.Errorf("Expected valid frame, got: %v", err)
	}

	invalidFrame := [12]uint32{
		0, 0, 0, 0,
		0,
		0, 0, 0, 0, 0,
		0x00001000,
		0,
	}

	err = ValidateMathFrame(invalidFrame)
	if err == nil {
		t.Error("Expected error for empty role")
	}
}

func TestIsMathDomain(t *testing.T) {
	tests := []struct {
		domain   uint32
		expected bool
	}{
		{jitter.DOMAIN_MATH, true},
		{jitter.DOMAIN_MATH | 0x0100, true},
		{jitter.DOMAIN_PROSE, false},
		{0x00001000, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := IsMathDomain(tt.domain)
			if result != tt.expected {
				t.Errorf("Expected %v for domain 0x%04x", tt.expected, tt.domain)
			}
		})
	}
}

func TestBuildMathFrame(t *testing.T) {
	slots := BuildMathFrame("x + y = z", SUB_algebra, "z")

	if slots[10]&0xF000 != jitter.DOMAIN_MATH {
		t.Errorf("Expected math domain, got 0x%08x", slots[10])
	}

	if slots[4] != ROLE_VARIABLE {
		t.Errorf("Expected VARIABLE role for 'z', got %d", slots[4])
	}
}

func TestDeriveMathSlots(t *testing.T) {
	slots := DeriveMathSlots(SUB_calculus)

	if slots[10] != jitter.DOMAIN_MATH|0x0200 {
		t.Errorf("Expected Calculus domain, got 0x%08x", slots[10])
	}

	if slots[4] != ROLE_RELATION {
		t.Errorf("Expected RELATION role, got %d", slots[4])
	}
}

func TestTokenizationEdgeCases(t *testing.T) {
	mapper := NewLaTeXMapper(SUB_arithmetic)

	tests := []string{
		"",
		"x",
		"x + 1",
		"f(x) = y",
		"\\int_0^1 x dx",
		"\\sum_{i=1}^n i",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			tokens := mapper.TokenizeLaTeX(tt)
			if len(tokens) == 0 && tt != "" {
				t.Error("Expected tokens for non-empty input")
			}
		})
	}
}
