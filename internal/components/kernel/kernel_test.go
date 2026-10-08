package kernel

import (
	"strings"
	"testing"
)

func TestStrLines(t *testing.T) {
	got := strLines("a\r\nb\rc\n\nlast\r")
	want := []string{"a", "b\rc", "", "last\r"}
	if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
		t.Errorf("got %q want %q", got, want)
	}
	if strLines("") != nil {
		t.Error("empty input has no lines")
	}
}

func TestSecondField(t *testing.T) {
	if v, ok := secondField("label = a = b"); !ok || v != " a " {
		t.Errorf("got %q %v", v, ok)
	}
	if _, ok := secondField("default"); ok {
		t.Error("no '=' means no field")
	}
}

func TestTrimStartMatches(t *testing.T) {
	if got := trimStartMatches("vmlinuz-vmlinuz-x", "vmlinuz-"); got != "x" {
		t.Errorf("got %q", got)
	}
}

func TestFormatSize(t *testing.T) {
	if got := formatSize(5 * 1024 * 1024); got != "5.0 MB" {
		t.Errorf("got %q", got)
	}
	if got := formatSize(0); got != "0.0 MB" {
		t.Errorf("got %q", got)
	}
}
