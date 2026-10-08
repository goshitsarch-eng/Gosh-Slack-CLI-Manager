package tui

import (
	"reflect"
	"testing"
)

func TestRustLinesMatchesStrLines(t *testing.T) {
	cases := map[string][]string{
		"":           nil,
		"a":          {"a"},
		"a\n":        {"a"},
		"a\r\nb\r\n": {"a", "b"},
		"a\rb\r":     {"a\rb\r"},
		"a\n\nb":     {"a", "", "b"},
		"\n":         {""},
	}
	for in, want := range cases {
		if got := RustLines(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
