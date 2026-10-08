package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// TestGoldenDump renders scripted scenarios and writes every cell to a dump
// file so screens can be compared against a reference rendering. It only
// runs when GOLDEN_SCENARIOS and GOLDEN_OUT are set.
//
// Scenario lines: name|tabIndex|width|height|root(0/1)|waitMs|keys...
func TestGoldenDump(t *testing.T) {
	scenarioPath, outPath := os.Getenv("GOLDEN_SCENARIOS"), os.Getenv("GOLDEN_OUT")
	if scenarioPath == "" || outPath == "" {
		t.Skip("GOLDEN_SCENARIOS/GOLDEN_OUT not set")
	}
	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 7)
		tabIndex, _ := strconv.Atoi(parts[1])
		width, _ := strconv.Atoi(parts[2])
		height, _ := strconv.Atoi(parts[3])
		waitMs, _ := strconv.Atoi(parts[5])
		keys := ""
		if len(parts) > 6 {
			keys = parts[6]
		}

		a := New(slackware.Version{Kind: slackware.Current}, parts[4] == "1")
		a.SwitchToTab(components.AllTabs()[tabIndex])
		for _, tok := range strings.Fields(keys) {
			if m := a.HandleInput(parseGoldenKey(t, tok)); m != nil {
				a.Update(m)
			}
		}
		if waitMs > 0 {
			time.Sleep(time.Duration(waitMs) * time.Millisecond)
		}
		for {
			m, ok := a.TryRecv()
			if !ok {
				break
			}
			a.Update(m)
		}

		buf := tui.NewBuffer(tui.Rect{Width: width, Height: height})
		a.Render(&tui.Frame{Buf: buf})
		dumpBuffer(&out, parts[0], buf)
	}
	if err := os.WriteFile(outPath, []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func parseGoldenKey(t *testing.T, tok string) tui.KeyEvent {
	mods := tui.ModNone
	rest := tok
	if r, ok := strings.CutPrefix(tok, "C-"); ok {
		mods, rest = tui.ModControl, r
	} else if r, ok := strings.CutPrefix(tok, "A-"); ok {
		mods, rest = tui.ModAlt, r
	}
	codes := map[string]tui.KeyCode{
		"Enter": tui.KeyEnter, "Tab": tui.KeyTab, "Esc": tui.KeyEsc, "Backspace": tui.KeyBackspace,
		"Delete": tui.KeyDelete, "Up": tui.KeyUp, "Down": tui.KeyDown, "Left": tui.KeyLeft,
		"Right": tui.KeyRight, "Home": tui.KeyHome, "End": tui.KeyEnd, "PageUp": tui.KeyPageUp,
		"PageDown": tui.KeyPageDown,
	}
	if code, ok := codes[rest]; ok {
		return tui.NewKey(code, mods)
	}
	switch {
	case rest == "BackTab":
		return tui.NewKey(tui.KeyBackTab, tui.ModShift)
	case rest == "Space":
		return tui.CharKey(' ', mods)
	case len(rest) > 1 && rest[0] == 'F':
		if n, err := strconv.Atoi(rest[1:]); err == nil {
			return tui.FKey(n, mods)
		}
	case utf8.RuneCountInString(rest) == 1:
		r, _ := utf8.DecodeRuneInString(rest)
		if unicode.IsUpper(r) {
			mods |= tui.ModShift
		}
		return tui.CharKey(r, mods)
	}
	t.Fatalf("unknown key token %q", tok)
	return tui.KeyEvent{}
}

func goldenColor(c tui.Color) string {
	switch c.Kind {
	case tui.ColorKindRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	case tui.ColorKindIndexed:
		return fmt.Sprintf("i%d", c.Index)
	default:
		return "-"
	}
}

func dumpBuffer(out *strings.Builder, name string, buf *tui.Buffer) {
	// Mirror what a terminal backend receives: the cells hidden behind a
	// wide character are never drawn, so they keep the blank default cell.
	buf = hideWideTails(buf)
	fmt.Fprintf(out, "=== %s\n", name)
	for y := 0; y < buf.Area.Height; y++ {
		for x := 0; x < buf.Area.Width; x++ {
			out.WriteString(buf.Cell(x, y).Symbol)
		}
		out.WriteString("\n")
	}
	out.WriteString("--- styles\n")
	for y := 0; y < buf.Area.Height; y++ {
		type run struct {
			n   int
			key string
		}
		var runs []run
		for x := 0; x < buf.Area.Width; x++ {
			c := buf.Cell(x, y)
			key := fmt.Sprintf("%s/%s/%x", goldenColor(c.Fg), goldenColor(c.Bg), uint16(c.Modifier))
			if len(runs) > 0 && runs[len(runs)-1].key == key {
				runs[len(runs)-1].n++
			} else {
				runs = append(runs, run{1, key})
			}
		}
		parts := make([]string, len(runs))
		for i, r := range runs {
			parts[i] = fmt.Sprintf("%d:%s", r.n, r.key)
		}
		fmt.Fprintf(out, "%d: %s\n", y, strings.Join(parts, " "))
	}
}

func hideWideTails(src *tui.Buffer) *tui.Buffer {
	buf := &tui.Buffer{Area: src.Area, Content: append([]tui.Cell(nil), src.Content...)}
	for y := 0; y < buf.Area.Height; y++ {
		for x := 0; x < buf.Area.Width; x++ {
			w := tui.GraphemeWidth(buf.Cell(x, y).Symbol)
			for i := 1; i < w && x+i < buf.Area.Width; i++ {
				*buf.Cell(x+i, y) = tui.EmptyCell
			}
			if w > 1 {
				x += w - 1
			}
		}
	}
	return buf
}
