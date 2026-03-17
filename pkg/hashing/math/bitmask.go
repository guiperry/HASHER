package math

import (
	"hasher/pkg/hashing/jitter"
)

const (
	ROLE_VARIABLE  uint32 = 0x01
	ROLE_OPERATOR  uint32 = 0x02
	ROLE_INTEGER   uint32 = 0x03
	ROLE_DECIMAL   uint32 = 0x04
	ROLE_FUNCTION  uint32 = 0x05
	ROLE_DELIMITER uint32 = 0x06
	ROLE_RELATION  uint32 = 0x07
	ROLE_EXPONENT  uint32 = 0x08
)

const (
	SUB_arithmetic uint32 = 0x00
	SUB_algebra    uint32 = 0x01
	SUB_calculus   uint32 = 0x02
	SUB_statistics uint32 = 0x03
	SUB_logic      uint32 = 0x04

	SUBDOMAIN_ARITHMETIC uint32 = 0x00
	SUBDOMAIN_ALGEBRA    uint32 = 0x01
	SUBDOMAIN_CALCULUS   uint32 = 0x02
	SUBDOMAIN_STATISTICS uint32 = 0x03
	SUBDOMAIN_LOGIC      uint32 = 0x04
)

type MathRole struct {
	ID          uint32
	Name        string
	Description string
	Examples    []string
}

var MathRoles = map[uint32]MathRole{
	ROLE_VARIABLE:  {ROLE_VARIABLE, "VARIABLE", "Symbolic placeholders", []string{"x", "y", "theta", "alpha"}},
	ROLE_OPERATOR:  {ROLE_OPERATOR, "OPERATOR", "Arithmetic or logical actions", []string{"+", "-", "\\int", "\\sum"}},
	ROLE_INTEGER:   {ROLE_INTEGER, "INTEGER", "Constant whole numbers", []string{"1", "2", "42"}},
	ROLE_DECIMAL:   {ROLE_DECIMAL, "DECIMAL", "Floating-point or fractional values", []string{"3.14", "0.5", "2/3"}},
	ROLE_FUNCTION:  {ROLE_FUNCTION, "FUNCTION", "Named mathematical operations", []string{"sin", "log", "lim", "exp"}},
	ROLE_DELIMITER: {ROLE_DELIMITER, "DELIMITER", "Structural boundaries", []string{"(", ")", "[", "]", "{", "}"}},
	ROLE_RELATION:  {ROLE_RELATION, "RELATION", "Comparative logic", []string{"=", "<", ">", "\\approx", "\\equiv"}},
	ROLE_EXPONENT:  {ROLE_EXPONENT, "EXPONENT", "Power or root indicators", []string{"^2", "\\sqrt{}", "pow"}},
}

var SubDomains = map[uint32]string{
	0x00: "Arithmetic",
	0x01: "Algebra",
	0x02: "Calculus",
	0x03: "Statistics",
	0x04: "Logic/Set",
}

func SubDomainToSlot10(subdomain uint32) uint32 {
	return jitter.DOMAIN_MATH | (subdomain << 8)
}

func Slot10ToSubDomain(slot10 uint32) uint32 {
	if slot10&0xF000 != jitter.DOMAIN_MATH {
		return 0
	}
	return (slot10 & 0x0F00) >> 8
}

func GetRoleName(role uint32) string {
	if r, ok := MathRoles[role]; ok {
		return r.Name
	}
	return "UNKNOWN"
}

func GetSubDomainName(subdomain uint32) string {
	normalizedSubdomain := subdomain & 0x0F
	if name, ok := SubDomains[normalizedSubdomain]; ok {
		return name
	}
	return "Unknown"
}
