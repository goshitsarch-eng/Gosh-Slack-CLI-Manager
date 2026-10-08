package logs

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// rustToLower mirrors Rust's str::to_lowercase, which applies the full
// Unicode lowercase mapping (U+0130 becomes "i̇") and the Final_Sigma
// rule, unlike strings.ToLower's simple per-rune mapping.
func rustToLower(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		switch r {
		case 'İ':
			b.WriteString("i̇")
		case 'Σ':
			if caseIgnorableThenCased(s[:i], true) && !caseIgnorableThenCased(s[i+len("Σ"):], false) {
				b.WriteRune('ς')
			} else {
				b.WriteRune('σ')
			}
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// caseIgnorableThenCased skips Case_Ignorable characters (scanning s
// backwards when reverse is set) and reports whether the next one is Cased.
func caseIgnorableThenCased(s string, reverse bool) bool {
	for len(s) > 0 {
		var r rune
		var size int
		if reverse {
			r, size = utf8.DecodeLastRuneInString(s)
			s = s[:len(s)-size]
		} else {
			r, size = utf8.DecodeRuneInString(s)
			s = s[size:]
		}
		if isCaseIgnorable(r) {
			continue
		}
		return isCased(r)
	}
	return false
}

func isCased(r rune) bool {
	return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

func isCaseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '^', '`', 0x00A8, 0x00AD, 0x00AF, 0x00B4, 0x00B7, 0x00B8,
		0x0387, 0x055F, 0x05F4, 0x2018, 0x2019, 0x2024, 0xFE13, 0xFE52, 0xFE55, 0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}
