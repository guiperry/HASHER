package math

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	variablePattern  = regexp.MustCompile(`(?i)([a-z]|alpha|beta|gamma|delta|epsilon|theta|lambda|mu|sigma|omega)`)
	operatorPattern  = regexp.MustCompile(`(?i)(\\+|-|\\times|\\div|\\cdot|\\int|\\sum|\\prod|\\partial|\\nabla)`)
	integerPattern   = regexp.MustCompile(`\b\d+\b`)
	decimalPattern   = regexp.MustCompile(`\d+\.\d+`)
	functionPattern  = regexp.MustCompile(`(?i)(\\sin|\\cos|\\tan|\\log|\\ln|\\exp|\\lim|\\sqrt|\\int|\\sum|\\prod)`)
	delimiterPattern = regexp.MustCompile(`[()\[\]\{\}]|\\{|\\}`)
	relationPattern  = regexp.MustCompile(`(?i)(=|<|>|\\leq|\\geq|\\approx|\\equiv|\\neq|\\in|\\subset)`)
	exponentPattern  = regexp.MustCompile(`\^(\d+|\{[^}]+\})|\\sqrt(\[[^\]]+\])?\{[^}]+\}`)
)

type Token struct {
	Text string
	Role uint32
}

type LaTeXMapper struct {
	subdomain uint32
}

func NewLaTeXMapper(subdomain uint32) *LaTeXMapper {
	if subdomain == 0 {
		subdomain = SUB_arithmetic
	}
	return &LaTeXMapper{subdomain: subdomain}
}

func (m *LaTeXMapper) MapLaTeXToTensor(latexExpr string, subdomain uint32) [12]uint32 {
	var slots [12]uint32

	if subdomain == 0 {
		subdomain = m.subdomain
	}

	slots[10] = domainFromSubdomain(subdomain)

	tokens := m.TokenizeLaTeX(latexExpr)
	if len(tokens) > 0 {
		lastToken := tokens[len(tokens)-1]
		slots[4] = lastToken.Role
	}

	slots[0], slots[1], slots[2], slots[3] = GetSemanticAnchors(latexExpr)

	slots[11] = GenerateTemporalLock(latexExpr)

	return slots
}

func (m *LaTeXMapper) TokenizeLaTeX(latex string) []Token {
	if latex == "" {
		return nil
	}

	var result []Token
	cmdRe := regexp.MustCompile(`\\[a-zA-Z]+`)
	lastEnd := 0

	for _, loc := range cmdRe.FindAllStringIndex(latex, -1) {
		// Tokenize plain text that precedes this command
		plain := latex[lastEnd:loc[0]]
		for _, t := range extractTokens(plain) {
			if t = strings.TrimSpace(t); t != "" {
				result = append(result, Token{Text: t, Role: determineRole(t)})
			}
		}
		// Tokenize the command itself (backslash retained — patterns match it)
		cmd := latex[loc[0]:loc[1]]
		result = append(result, Token{Text: cmd, Role: determineRole(cmd)})
		lastEnd = loc[1]
	}

	// Tokenize any remaining plain text after the last command
	if lastEnd < len(latex) {
		for _, t := range extractTokens(latex[lastEnd:]) {
			if t = strings.TrimSpace(t); t != "" {
				result = append(result, Token{Text: t, Role: determineRole(t)})
			}
		}
	}

	if len(result) == 0 {
		result = []Token{{Text: latex, Role: ROLE_VARIABLE}}
	}

	return result
}

func extractTokens(s string) []string {
	var tokens []string
	var current strings.Builder

	for i, r := range s {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		if strings.ContainsRune("()[]{}", r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
			continue
		}

		if r == '^' || r == '=' || r == '+' || r == '-' {
			if current.Len() > 0 {
				if i > 0 && (s[i-1] == '^' || s[i-1] == '=') {
					current.WriteRune(r)
				} else {
					tokens = append(tokens, current.String())
					current.Reset()
					current.WriteRune(r)
				}
			} else {
				current.WriteRune(r)
			}
			continue
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	if len(tokens) == 0 {
		tokens = []string{s}
	}

	return tokens
}

func determineRole(token string) uint32 {
	token = strings.TrimSpace(token)

	if functionPattern.MatchString(token) {
		return ROLE_FUNCTION
	}
	if operatorPattern.MatchString(token) || token == "+" || token == "-" {
		return ROLE_OPERATOR
	}
	if relationPattern.MatchString(token) || token == "=" {
		return ROLE_RELATION
	}
	if exponentPattern.MatchString(token) {
		return ROLE_EXPONENT
	}
	if delimiterPattern.MatchString(token) {
		return ROLE_DELIMITER
	}
	if decimalPattern.MatchString(token) {
		return ROLE_DECIMAL
	}
	if integerPattern.MatchString(token) {
		return ROLE_INTEGER
	}
	if variablePattern.MatchString(token) {
		return ROLE_VARIABLE
	}

	return ROLE_VARIABLE
}

func domainFromSubdomain(subdomain uint32) uint32 {
	return SubDomainToSlot10(subdomain)
}

// HashContext returns a deterministic polynomial hash of s for the given slot
// index. Used to fill Slots 0–3 (semantic anchors) until BGE-Base embeddings
// are wired in.
func HashContext(s string, slot int) uint32 {
	h := uint32(len(s))
	for i, c := range s {
		rot := uint32(c) << ((i + slot*3) % 24)
		h = h*31 + rot
	}
	return h
}

// GenerateTemporalLock returns a deterministic temporal lock value for s.
// Used to populate Slot 11.
func GenerateTemporalLock(s string) uint32 {
	h := uint32(0)
	for i, c := range s {
		h += uint32(c) * uint32(i+1)
	}
	return h ^ 0xCAFEBABE
}

// GetSemanticAnchors returns the four slot-0..3 hash values for latex.
func GetSemanticAnchors(latex string) (a, b, c, d uint32) {
	return HashContext(latex, 0), HashContext(latex, 1), HashContext(latex, 2), HashContext(latex, 3)
}
