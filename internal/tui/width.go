package tui

import (
	"unicode"

	"github.com/rivo/uniseg"
)

// StrWidth returns the display width of s.
func StrWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// RuneWidth returns the display width of a single rune. Control characters
// have no width.
func RuneWidth(r rune) int {
	if r < 0x20 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	return uniseg.StringWidth(string(r))
}

// GraphemeWidth returns the display width of a single grapheme cluster.
func GraphemeWidth(g string) int {
	return uniseg.StringWidth(g)
}

// Graphemes splits s into extended grapheme clusters.
func Graphemes(s string) []string {
	var out []string
	gr := uniseg.NewGraphemes(s)
	for gr.Next() {
		out = append(out, gr.Str())
	}
	return out
}

// HasControl reports whether s contains a control character.
func HasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// IsWhitespace reports whether every rune in s is whitespace.
func IsWhitespace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
