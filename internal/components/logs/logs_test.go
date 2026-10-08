package logs

import (
	"strings"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func TestReadLinesMatchesBufReadLines(t *testing.T) {
	in := "a\r\nb\rc\n\nlast\r"
	got := readLines(strings.NewReader(in))
	want := []string{"a", "b\rc", "", "last\r"}
	if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
		t.Errorf("got %q want %q", got, want)
	}
	got = readLines(strings.NewReader("ok\nbad \xff\nafter\n"))
	if len(got) != 1 || got[0] != "ok" {
		t.Errorf("must stop at invalid UTF-8: %q", got)
	}
}

func TestRustToLower(t *testing.T) {
	cases := map[string]string{"İnfo": "i̇nfo", "ΟΔΟΣ": "οδος", "ΣΑ": "σα", "A Σ B": "a σ b", "ABC": "abc"}
	for in, want := range cases {
		if got := rustToLower(in); got != want {
			t.Errorf("rustToLower(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogLevelColor(t *testing.T) {
	cases := map[string]tui.Color{"ERROR x": tui.Red, "failed": tui.Red, "Warn": tui.Yellow, "INFO": tui.Cyan, "debug": tui.DarkGray, "plain": tui.White, "İnfo": tui.White}
	for in, want := range cases {
		if got := logLevelColor(in); got != want {
			t.Errorf("logLevelColor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	cases := map[uint64]string{0: "0B", 1024: "1.0K", 1536: "1.5K", 1048576: "1.0M"}
	for in, want := range cases {
		if got := formatSize(in); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSearchNavigation(t *testing.T) {
	c := &Component{mode: modeViewLog, logContent: []string{"a", "Apple", "b", "APPLE pie", "c"}}
	ch := func(r rune) tui.KeyEvent { return tui.CharKey(r, tui.ModNone) }
	c.HandleInput(ch('/'))
	for _, r := range "apple" {
		c.HandleInput(ch(r))
	}
	c.HandleInput(tui.NewKey(tui.KeyEnter, tui.ModNone))
	if len(c.searchResults) != 2 || c.contentScroll != 1 {
		t.Fatalf("results %v scroll %d", c.searchResults, c.contentScroll)
	}
	c.HandleInput(ch('n'))
	if c.contentScroll != 3 {
		t.Fatalf("n -> %d", c.contentScroll)
	}
	c.HandleInput(ch('n'))
	if c.contentScroll != 1 {
		t.Fatalf("n wraps -> %d", c.contentScroll)
	}
	c.HandleInput(ch('N'))
	if c.contentScroll != 3 {
		t.Fatalf("N wraps -> %d", c.contentScroll)
	}
	c.HandleInput(tui.NewKey(tui.KeyPageDown, tui.ModNone))
	if c.contentScroll != 4 {
		t.Fatalf("PageDown clamps -> %d", c.contentScroll)
	}
}
