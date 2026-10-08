package tui

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The golden data in testdata/textarea_golden.json.gz was produced by a Rust
// program driving tui-textarea 0.7.0 + ratatui 0.29.0 + crossterm 0.28.1:
// each scenario feeds scripted key events to a TextArea wrapped in
// Block::default().borders(Borders::ALL).title("Editor").border_style(fg Cyan)
// and records, after every step, the return value of input(), lines(),
// cursor() and every cell of a buffer the widget was rendered into (at
// x=2, y=1 inside a buffer pre-filled with "~").

type goldenBuf struct {
	Rows   [][]string        `json:"rows"`
	Styles []json.RawMessage `json:"styles"`
}

type goldenStep struct {
	Key       string     `json:"key"`
	W         int        `json:"w"`
	H         int        `json:"h"`
	Modified  bool       `json:"modified"`
	Lines     []string   `json:"lines"`
	Cursor    [2]int     `json:"cursor"`
	Yank      string     `json:"yank"`
	Selecting bool       `json:"selecting"`
	Empty     bool       `json:"empty"`
	Buf       *goldenBuf `json:"buf"`
}

type goldenScenario struct {
	Name  string       `json:"name"`
	Lines []string     `json:"lines"`
	W     int          `json:"w"`
	H     int          `json:"h"`
	Steps []goldenStep `json:"steps"`
}

func loadTextAreaGolden(t *testing.T) []goldenScenario {
	t.Helper()
	f, err := os.Open("testdata/textarea_golden.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var out []goldenScenario
	if err := json.NewDecoder(zr).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// parseKeySpec parses the key notation used by the golden generator:
// optional "C-", "M-", "S-" prefixes followed by a key name or a character.
func parseKeySpec(t *testing.T, spec string) KeyEvent {
	t.Helper()
	var mods KeyModifiers
	rest := spec
	for len(rest) > 2 {
		switch {
		case strings.HasPrefix(rest, "C-"):
			mods |= ModControl
		case strings.HasPrefix(rest, "M-"):
			mods |= ModAlt
		case strings.HasPrefix(rest, "S-"):
			mods |= ModShift
		default:
			goto done
		}
		rest = rest[2:]
	}
done:
	named := map[string]KeyCode{
		"Enter": KeyEnter, "Tab": KeyTab, "BackTab": KeyBackTab, "Backspace": KeyBackspace,
		"Delete": KeyDelete, "Insert": KeyInsert, "Esc": KeyEsc, "Up": KeyUp, "Down": KeyDown,
		"Left": KeyLeft, "Right": KeyRight, "Home": KeyHome, "End": KeyEnd,
		"PageUp": KeyPageUp, "PageDown": KeyPageDown, "Null": KeyNull,
	}
	if code, ok := named[rest]; ok {
		return NewKey(code, mods)
	}
	if len(rest) > 1 && rest[0] == 'F' {
		n, err := strconv.Atoi(rest[1:])
		if err == nil {
			return FKey(n, mods)
		}
	}
	rs := []rune(rest)
	if len(rs) != 1 {
		t.Fatalf("bad key spec %q", spec)
	}
	return CharKey(rs[0], mods)
}

func colorName(c Color) string {
	switch c.Kind {
	case ColorKindReset:
		return "reset"
	case ColorKindIndexed:
		return strconv.Itoa(int(c.Index))
	default:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
}

// ratatui's Modifier bits.
func ratatuiModBits(m Modifier) int {
	bits := 0
	pairs := []struct {
		m   Modifier
		bit int
	}{
		{Bold, 1 << 0}, {Dim, 1 << 1}, {Italic, 1 << 2}, {Underlined, 1 << 3},
		{SlowBlink, 1 << 4}, {RapidBlink, 1 << 5}, {Reversed, 1 << 6},
		{Hidden, 1 << 7}, {CrossedOut, 1 << 8},
	}
	for _, p := range pairs {
		if m&p.m != 0 {
			bits |= p.bit
		}
	}
	return bits
}

func dumpBuffer(buf *Buffer) goldenBuf {
	var gb goldenBuf
	gb.Rows = [][]string{}
	gb.Styles = []json.RawMessage{}
	for y := buf.Area.Top(); y < buf.Area.Bottom(); y++ {
		row := []string{}
		for x := buf.Area.Left(); x < buf.Area.Right(); x++ {
			c := buf.Cell(x, y)
			row = append(row, c.Symbol)
			if c.Fg != Reset || c.Bg != Reset || c.Modifier != 0 {
				s, _ := json.Marshal([]any{x, y, colorName(c.Fg), colorName(c.Bg), ratatuiModBits(c.Modifier)})
				gb.Styles = append(gb.Styles, s)
			}
		}
		gb.Rows = append(gb.Rows, row)
	}
	return gb
}

func renderTextAreaForTest(ta *TextArea, w, h int) *Buffer {
	buf := NewBuffer(NewRect(0, 0, w+3, h+2))
	for i := range buf.Content {
		buf.Content[i].Symbol = "~"
	}
	ta.Render(NewRect(2, 1, w, h), buf)
	return buf
}

func compareBuf(want *goldenBuf, got goldenBuf) string {
	if !reflect.DeepEqual(want.Rows, got.Rows) {
		var b strings.Builder
		for y := range max(len(want.Rows), len(got.Rows)) {
			var wr, gr []string
			if y < len(want.Rows) {
				wr = want.Rows[y]
			}
			if y < len(got.Rows) {
				gr = got.Rows[y]
			}
			mark := " "
			if !reflect.DeepEqual(wr, gr) {
				mark = "!"
			}
			fmt.Fprintf(&b, "%s want %q\n  got  %q\n", mark, strings.Join(wr, ""), strings.Join(gr, ""))
		}
		return "symbols differ:\n" + b.String()
	}
	ws, gs := make([]string, len(want.Styles)), make([]string, len(got.Styles))
	for i, s := range want.Styles {
		ws[i] = string(s)
	}
	for i, s := range got.Styles {
		gs[i] = string(s)
	}
	if !reflect.DeepEqual(ws, gs) {
		return fmt.Sprintf("styles differ:\nwant %v\ngot  %v", ws, gs)
	}
	return ""
}

// fakeBlock renders like ratatui's Block with Borders::ALL, plain borders,
// a left-aligned top title and a border style.
type fakeBlock struct {
	title       string
	borderStyle Style
}

func (b fakeBlock) Inner(area Rect) Rect {
	in := area
	in.X = min(in.X+1, in.Right())
	in.Width = satSub(in.Width, 1)
	in.Y = min(in.Y+1, in.Bottom())
	in.Height = satSub(in.Height, 1)
	in.Width = satSub(in.Width, 1)
	in.Height = satSub(in.Height, 1)
	return in
}

func (b fakeBlock) Render(area Rect, buf *Buffer) {
	area = area.Intersection(buf.Area)
	if area.IsEmpty() {
		return
	}
	set := func(x, y int, s string) { buf.Cell(x, y).SetSymbol(s).SetStyle(b.borderStyle) }
	for y := area.Top(); y < area.Bottom(); y++ {
		set(area.Left(), y, "│")
	}
	for x := area.Left(); x < area.Right(); x++ {
		set(x, area.Top(), "─")
	}
	for y := area.Top(); y < area.Bottom(); y++ {
		set(area.Right()-1, y, "│")
	}
	for x := area.Left(); x < area.Right(); x++ {
		set(x, area.Bottom()-1, "─")
	}
	set(area.Right()-1, area.Bottom()-1, "┘")
	set(area.Right()-1, area.Top(), "┐")
	set(area.Left(), area.Bottom()-1, "└")
	set(area.Left(), area.Top(), "┌")
	titles := Rect{X: area.X + 1, Y: area.Y, Width: satSub(area.Width, 2), Height: 1}
	if titles.IsEmpty() {
		return
	}
	line := LineStr(b.title)
	titles.Width = min(line.Width(), titles.Width)
	line.Render(titles, buf)
}

// textAreaAPIOp applies the non-key operations of the golden scenarios.
func textAreaAPIOp(t *testing.T, ta *TextArea, op string) bool {
	t.Helper()
	name, arg, _ := strings.Cut(op, ":")
	arg = strings.ReplaceAll(arg, `\n`, "\n")
	atoi := func(s string) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	switch name {
	case "wordend":
		ta.MoveCursor(CursorWordEnd)
	case "insert_str":
		return ta.InsertStr(arg)
	case "set_yank":
		ta.SetYankText(arg)
	case "mask":
		ta.SetMaskChar([]rune(arg)[0])
	case "clearmask":
		ta.ClearMaskChar()
	case "hardtab":
		ta.SetHardTabIndent(arg == "on")
	case "tablen":
		ta.SetTabLength(uint8(atoi(arg)))
	case "maxhist":
		ta.SetMaxHistories(atoi(arg))
	case "scroll":
		r, c, _ := strings.Cut(arg, ",")
		ta.Scroll(int16(atoi(r)), int16(atoi(c)))
	case "styles":
		ta.SetStyle(NewStyle().FG(Yellow))
		ta.SetCursorStyle(NewStyle().BG(Red))
		ta.SetCursorLineStyle(NewStyle().Add(Italic))
		ta.SetSelectionStyle(NewStyle().BG(Green).FG(Black))
	case "noblock":
		ta.RemoveBlock()
	case "start_sel":
		ta.StartSelection()
	case "cancel_sel":
		ta.CancelSelection()
	case "insert_tab":
		return ta.InsertTab()
	case "delete_newline":
		return ta.DeleteNewline()
	default:
		t.Fatalf("unknown op %q", op)
	}
	return false
}

func runTextAreaGolden(t *testing.T, block func() BlockWidget) {
	for _, sc := range loadTextAreaGolden(t) {
		t.Run(sc.Name, func(t *testing.T) {
			ta := NewTextArea(sc.Lines)
			ta.SetBlock(block())
			for i, st := range sc.Steps {
				ctx := fmt.Sprintf("step %d (%q)", i, st.Key)
				modified := false
				render := true
				spec := st.Key
				if s, ok := strings.CutSuffix(spec, "!norender"); ok {
					spec, render = s, false
				}
				switch {
				case i == 0 || strings.HasPrefix(spec, "@size="):
				case strings.HasPrefix(spec, "#"):
					modified = textAreaAPIOp(t, ta, spec[1:])
				default:
					modified = ta.Input(parseKeySpec(t, spec))
				}
				if modified != st.Modified {
					t.Fatalf("%s: modified = %v, want %v", ctx, modified, st.Modified)
				}
				if !reflect.DeepEqual(ta.Lines(), st.Lines) {
					t.Fatalf("%s: lines = %q, want %q", ctx, ta.Lines(), st.Lines)
				}
				if r, c := ta.Cursor(); r != st.Cursor[0] || c != st.Cursor[1] {
					t.Fatalf("%s: cursor = (%d, %d), want %v", ctx, r, c, st.Cursor)
				}
				if ta.YankText() != st.Yank || ta.IsSelecting() != st.Selecting || ta.IsEmpty() != st.Empty {
					t.Fatalf("%s: yank/selecting/empty = %q/%v/%v, want %q/%v/%v", ctx,
						ta.YankText(), ta.IsSelecting(), ta.IsEmpty(), st.Yank, st.Selecting, st.Empty)
				}
				if !render {
					if st.Buf != nil {
						t.Fatalf("%s: golden has a buffer for a no-render step", ctx)
					}
					continue
				}
				got := dumpBuffer(renderTextAreaForTest(ta, st.W, st.H))
				if diff := compareBuf(st.Buf, got); diff != "" {
					t.Fatalf("%s: %s", ctx, diff)
				}
			}
		})
	}
}

func TestTextAreaGolden(t *testing.T) {
	runTextAreaGolden(t, func() BlockWidget {
		return fakeBlock{title: "Editor", borderStyle: NewStyle().FG(Cyan)}
	})
}

func TestTextAreaGoldenWithBlock(t *testing.T) {
	runTextAreaGolden(t, func() BlockWidget {
		return NewBlock().Borders(BordersAll).Title(LineStr("Editor")).BorderStyle(NewStyle().FG(Cyan))
	})
}

func TestTextAreaDefaults(t *testing.T) {
	for _, lines := range [][]string{nil, {}} {
		ta := NewTextArea(lines)
		if !reflect.DeepEqual(ta.Lines(), []string{""}) {
			t.Fatalf("lines = %q", ta.Lines())
		}
		if r, c := ta.Cursor(); r != 0 || c != 0 {
			t.Fatalf("cursor = %d,%d", r, c)
		}
	}
	src := []string{"a", "b"}
	ta := NewTextArea(src)
	ta.Input(CharKey('x', ModNone))
	if src[0] != "a" {
		t.Fatal("NewTextArea must not alias the caller's slice")
	}
}
