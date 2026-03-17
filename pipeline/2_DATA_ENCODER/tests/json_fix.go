package tests

import (
	"regexp"
	"strconv"
	"strings"
)

// jsonHexEscapeRe matches \\xNN patterns: a JSON-encoded backslash followed by
// a hex escape (two backslashes in raw JSON text represent one literal backslash,
// so \\x00 in JSON source is a backslash + x00 — not valid standard JSON but
// produced by some encoders porting JavaScript hex escapes).
var jsonHexEscapeRe = regexp.MustCompile(`\\\\x([0-9a-fA-F]{2})`)

// jsonNullUnicodeRe matches JSON unicode escapes for control characters
// U+0000–U+001F (e.g. \u0000, \u0001) that produce null/control bytes when decoded.
var jsonNullUnicodeRe = regexp.MustCompile(`\\u00[01][0-9a-fA-F]`)

// rawControlRe matches raw (unescaped) control characters except \t, \n, \r.
var rawControlRe = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f]`)

// fixJSONString cleans s by:
//  1. Replacing \\xNN hex escapes with their decoded character (or "" for control chars).
//  2. Removing \u00XX JSON unicode escapes for control characters.
//  3. Stripping raw embedded control characters (0x00–0x1F, except \t/\n/\r).
func fixJSONString(s string) string {
	// 1. Replace \\xNN (two backslashes + hex) with the decoded character.
	s = jsonHexEscapeRe.ReplaceAllStringFunc(s, func(m string) string {
		// m is e.g. "\\x00" — capture group is the two hex digits.
		sub := jsonHexEscapeRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		n, err := strconv.ParseUint(sub[1], 16, 8)
		if err != nil {
			return ""
		}
		ch := rune(n)
		if ch < 0x20 && ch != '\t' && ch != '\n' && ch != '\r' {
			return "" // drop control characters
		}
		return string(ch)
	})

	// 2. Remove JSON unicode escapes for control characters (\u0000–\u001F).
	s = jsonNullUnicodeRe.ReplaceAllString(s, "")

	// 3. Remove raw embedded control characters (cannot appear unescaped in JSON).
	s = rawControlRe.ReplaceAllString(s, "")

	return s
}

// fixJSONContent applies fixJSONString to content and returns (fixed, changed).
func fixJSONContent(content string) (string, bool) {
	fixed := fixJSONString(content)
	return fixed, fixed != strings.NewReplacer().Replace(content) || fixed != content
}
