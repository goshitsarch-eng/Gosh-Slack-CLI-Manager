package tui

// The expected output in testdata/widgets_ratatui.txt.gz was produced by a
// small Rust program rendering the very same cases with ratatui 0.29.0 and
// dumping every cell (symbol|fg|bg|modifier bits). Each case below mirrors
// one case of that program.

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

var (
	tBG              = RGB(12, 16, 24)
	tSurface         = RGB(20, 27, 38)
	tSurfaceElevated = RGB(28, 36, 48)
	tPanelAlt        = RGB(16, 22, 32)
	tFG              = RGB(233, 228, 214)
	tAccent          = RGB(94, 204, 187)
	tAccentAlt       = RGB(235, 178, 74)
	tSuccess         = RGB(116, 204, 149)
	tMuted           = RGB(109, 120, 136)
	tBorder          = RGB(63, 78, 96)
)

func tStyleBorder() Style        { return NewStyle().FG(tBorder).BG(tSurface) }
func tStyleBorderFocused() Style { return NewStyle().FG(tAccent).BG(tSurface) }
func tStyleSurface() Style       { return NewStyle().FG(tFG).BG(tSurface) }
func tStyleSurfaceAlt() Style    { return NewStyle().FG(tFG).BG(tPanelAlt) }
func tStyleApp() Style           { return NewStyle().FG(tFG).BG(tBG) }
func tStyleMuted() Style         { return NewStyle().FG(tMuted).BG(tBG) }
func tStyleAccent() Style        { return NewStyle().FG(tAccent).BG(tBG) }
func tStyleLabel() Style         { return NewStyle().FG(tMuted).BG(tBG) }
func tStyleSuccess() Style       { return NewStyle().FG(tSuccess).BG(tBG) }
func tStyleHighlight() Style {
	return NewStyle().FG(tFG).BG(tSurfaceElevated).Add(Bold)
}
func tStyleListSelected() Style { return NewStyle().FG(tBG).BG(tAccentAlt).Add(Bold) }
func tStyleWeird() Style {
	return NewStyle().FG(Red).Add(Italic | Underlined).Remove(Dim)
}

func tPanelTitle(label string) Line {
	return LineFrom(
		Styled(" ", NewStyle().BG(tSurface)),
		Styled("● ", NewStyle().FG(tAccent).BG(tSurface)),
		Styled(strings.ToUpper(label)+" ", NewStyle().FG(tAccentAlt).BG(tSurface).Add(Bold)),
	)
}

func tPanel(title Line) Block {
	return NewBlock().Borders(BordersAll).BorderType(BorderRounded).BorderStyle(tStyleBorder()).Style(tStyleSurface()).Title(title)
}

func tPanelFocused(title Line) Block {
	return NewBlock().Borders(BordersAll).BorderType(BorderRounded).BorderStyle(tStyleBorderFocused()).Style(tStyleSurface()).Title(title)
}

func tPanelAltBlock(title Line) Block {
	return NewBlock().Borders(BordersAll).BorderType(BorderRounded).BorderStyle(tStyleBorder()).Style(tStyleSurfaceAlt()).Title(title)
}

// ---------- dump ----------

func dumpColor(c Color) string {
	switch c.Kind {
	case ColorKindReset:
		return "-"
	case ColorKindIndexed:
		return "i" + strconv.Itoa(int(c.Index))
	default:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
}

func newTestBuf(w, h int) *Buffer {
	buf := NewBuffer(NewRect(0, 0, w+2, h+2))
	pre := NewStyle().FG(Yellow).BG(Blue).Add(Dim)
	for i := range buf.Content {
		buf.Content[i].SetSymbol(".").SetStyle(pre)
	}
	return buf
}

var dumpEscaper = strings.NewReplacer(`\`, `\\`, "\r", `\r`, "\n", `\n`, "\t", `\t`)

type caseSink struct {
	names []string
	dumps map[string]string
}

func (s *caseSink) dump(name string, buf *Buffer) {
	var sb strings.Builder
	a := buf.Area
	for y := a.Top(); y < a.Bottom(); y++ {
		for x := a.Left(); x < a.Right(); x++ {
			c := buf.Cell(x, y)
			if x > a.Left() {
				sb.WriteByte('\t')
			}
			fmt.Fprintf(&sb, "%s|%s|%s|%d", dumpEscaper.Replace(c.Symbol), dumpColor(c.Fg), dumpColor(c.Bg), c.Modifier)
		}
		sb.WriteByte('\n')
	}
	s.names = append(s.names, name)
	s.dumps[name] = sb.String()
}

func rectStr(r Rect) string { return fmt.Sprintf("%d %d %d %d", r.X, r.Y, r.Width, r.Height) }

// ---------- blocks ----------

type namedBlock struct {
	name  string
	block Block
}

func testBlocks() []namedBlock {
	return []namedBlock{
		{"panel", tPanelAltBlock(tPanelTitle("Inspector"))},
		{"panel_focused", tPanelFocused(tPanelTitle("Status"))},
		{"plain_title", NewBlock().Borders(BordersAll).TitleStr("Editor").BorderStyle(tStyleBorder())},
		{"notitle_rounded", NewBlock().Borders(BordersAll).BorderType(BorderRounded).BorderStyle(tStyleBorder())},
		{"title_only", NewBlock().TitleStr("RAM")},
		{"default", NewBlock()},
		{"style_only", NewBlock().Style(tStyleApp())},
		{"double", NewBlock().Borders(BordersAll).BorderType(BorderDouble).TitleStr("Double Title").Style(tStyleWeird())},
		{"thick", NewBlock().Borders(BordersAll).BorderType(BorderThick).TitleStr("Thick")},
		{"quad_in", NewBlock().Borders(BordersAll).BorderType(BorderQuadrantInside).TitleStr("q")},
		{"quad_out", NewBlock().Borders(BordersAll).BorderType(BorderQuadrantOutside).TitleStr("q")},
		{"top_left", NewBlock().Borders(BordersTop | BordersLeft).TitleStr("abc").BorderStyle(tStyleBorder())},
		{"bottom_right", NewBlock().Borders(BordersBottom | BordersRight).TitleStr("abc")},
		{"left_right", NewBlock().Borders(BordersLeft | BordersRight).TitleStr("xyz")},
		{"top_only", NewBlock().Borders(BordersTop).TitleStr("top")},
		{"multi", NewBlock().
			Borders(BordersAll).
			TitleStr("L1").
			Title(LineSpan(Styled("L2", tStyleWeird()))).
			Title(LineStr("C1").WithAlignment(AlignCenter)).
			Title(LineStr("C2").WithAlignment(AlignCenter)).
			Title(LineStr("R1").WithAlignment(AlignRight)).
			Title(LineStr("R2").WithAlignment(AlignRight)).
			TitleBottom(LineStr("B1")).
			TitleBottom(LineStr("BR").WithAlignment(AlignRight)).
			TitleTop(LineStr("T").WithAlignment(AlignCenter))},
		{"title_style", NewBlock().
			Borders(BordersAll).
			TitleStyle(NewStyle().FG(Yellow).Add(Italic)).
			Title(LineFrom(Raw("Sty"), Styled("led", NewStyle().FG(Red))))},
		{"title_center_default", NewBlock().Borders(BordersAll).TitleAlignment(AlignCenter).TitleStr("one").TitleStr("two")},
		{"title_right_default", NewBlock().Borders(BordersAll).TitleAlignment(AlignRight).TitleStr("one").TitleStr("two")},
		{"title_pos_bottom", NewBlock().Borders(BordersAll).TitlePosition(TitleBottom).TitleStr("bot")},
		{"wide_title", NewBlock().Borders(BordersAll).TitleStr("日本語タイトル")},
		{"right_long", NewBlock().Borders(BordersAll).Title(LineStr("this is a very long right title").WithAlignment(AlignRight))},
		{"center_long", NewBlock().Borders(BordersAll).Title(LineFrom(Raw("centered "), Styled("long title text", tStyleAccent())).WithAlignment(AlignCenter))},
		{"line_styled_title", NewBlock().Borders(BordersAll).Title(
			LineFrom(Raw("ab"), Styled("cd", NewStyle().FG(Green))).WithStyle(NewStyle().BG(Magenta).Add(Bold)))},
		{"padding", BorderedBlock().Padding(Padding{Left: 1, Right: 2, Top: 1, Bottom: 0}).TitleStr("pad")},
		{"empty_title", BorderedBlock().TitleStr("").TitleStr("x")},
	}
}

func blockCases(s *caseSink) {
	var widths []int
	for w := 0; w <= 12; w++ {
		widths = append(widths, w)
	}
	widths = append(widths, 20, 33)
	for _, nb := range testBlocks() {
		for _, w := range widths {
			for _, h := range []int{0, 1, 2, 3} {
				buf := newTestBuf(w, h)
				area := NewRect(1, 1, w, h)
				nb.block.Render(area, buf)
				s.dump(fmt.Sprintf("block/%s/%dx%d inner=%s", nb.name, w, h, rectStr(nb.block.Inner(area))), buf)
			}
		}
	}
}

func clearCases(s *caseSink) {
	for _, c := range [][4]int{{0, 0, 0, 0}, {2, 1, 3, 2}, {0, 0, 12, 5}, {5, 3, 20, 20}, {11, 6, 1, 1}} {
		buf := newTestBuf(10, 5)
		tPanel(tPanelTitle("Dialog")).Render(NewRect(1, 1, 10, 5), buf)
		Clear{}.Render(NewRect(c[0], c[1], c[2], c[3]).Intersection(buf.Area), buf)
		s.dump(fmt.Sprintf("clear/%d %d %d %d", c[0], c[1], c[2], c[3]), buf)
	}
}

// ---------- paragraphs ----------

func longLines() []Line {
	return []Line{
		LineStr("Hello world, this is a long line with averyveryverylongwordthatexceedswidth and more."),
		LineStr(""),
		LineStr("   leading spaces here"),
		LineStr("multiple    spaces   between  words"),
		LineFrom(
			Styled(" ", NewStyle().BG(tSurface)),
			Styled("● ", tStyleAccent()),
			Styled("STATUS ", NewStyle().FG(Yellow).Add(Bold)),
			Raw("running • ok – fine"),
		),
		LineStr("▸ item ▶ next ■ done ◆ x · y × z ← back"),
		LineStr("trailing   "),
		LineStr("     "),
		LineStr("a b c d e f g h i j k l m n o p"),
		LineStr("日本語のテキストです wide chars"),
		StyledLine("line-styled text here", NewStyle().FG(Cyan).BG(Black).Add(Italic)),
		LineFrom(Styled("weird", tStyleWeird()), Raw(" x"), Styled("", tStyleAccent()), Raw("y")),
		LineStr("end"),
	}
}

var wrapNames = []string{"nowrap", "trim", "notrim"}

func applyWrap(p Paragraph, m int) Paragraph {
	switch m {
	case 1:
		return p.Wrap(true)
	case 2:
		return p.Wrap(false)
	}
	return p
}

var alignNames = map[Alignment]string{AlignLeft: "left", AlignCenter: "center", AlignRight: "right"}

func paragraphCases(s *caseSink) {
	for _, w := range []int{0, 1, 2, 3, 4, 5, 7, 10, 13, 17, 25, 40} {
		for m := 0; m < 3; m++ {
			aligns := []Alignment{AlignLeft}
			if w == 7 || w == 13 || w == 25 {
				aligns = append(aligns, AlignCenter, AlignRight)
			}
			for _, a := range aligns {
				h := 14
				buf := newTestBuf(w, h)
				applyWrap(ParagraphLines(longLines()...), m).Alignment(a).Render(NewRect(1, 1, w, h), buf)
				s.dump(fmt.Sprintf("para/long/%s/%s/%dx%d", wrapNames[m], alignNames[a], w, h), buf)
			}
		}
	}
	for _, h := range []int{0, 1, 2, 3} {
		for m := 0; m < 3; m++ {
			buf := newTestBuf(9, h)
			applyWrap(ParagraphLines(longLines()...), m).Render(NewRect(1, 1, 9, h), buf)
			s.dump(fmt.Sprintf("para/tiny/%s/9x%d", wrapNames[m], h), buf)
		}
	}
	for _, sc := range [][2]int{{0, 0}, {3, 0}, {0, 4}, {2, 5}, {20, 0}, {0, 100}, {1, 1}} {
		for m := 0; m < 3; m++ {
			for _, a := range []Alignment{AlignLeft, AlignCenter} {
				buf := newTestBuf(13, 5)
				applyWrap(ParagraphLines(longLines()...), m).Alignment(a).Scroll(sc[0], sc[1]).Render(NewRect(1, 1, 13, 5), buf)
				s.dump(fmt.Sprintf("para/scroll/%s/%s/%d %d", wrapNames[m], alignNames[a], sc[0], sc[1]), buf)
			}
		}
	}
	for _, wh := range [][2]int{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {5, 3}, {10, 4}, {20, 6}, {30, 10}} {
		w, h := wh[0], wh[1]
		for m := 0; m < 2; m++ {
			buf := newTestBuf(w, h)
			applyWrap(ParagraphLines(longLines()...), m).
				Block(tPanelAltBlock(tPanelTitle("Inspector"))).
				Style(tStyleSurface()).
				Render(NewRect(1, 1, w, h), buf)
			s.dump(fmt.Sprintf("para/block/%s/%dx%d", wrapNames[m], w, h), buf)
		}
	}
	{
		text := Text{
			Lines: []Line{
				LineStr("text styled line"),
				LineStr("right").WithAlignment(AlignRight),
				LineSpan(Styled("span", NewStyle().BG(Red))),
				LineStr("left").WithAlignment(AlignLeft),
				LineStr("centered line").WithAlignment(AlignCenter),
			},
			Style:        NewStyle().FG(Green).Add(Underlined),
			Alignment:    AlignCenter,
			HasAlignment: true,
		}
		for m := 0; m < 3; m++ {
			for _, w := range []int{4, 11, 20} {
				buf := newTestBuf(w, 8)
				applyWrap(NewParagraph(text), m).Alignment(AlignCenter).Style(tStyleMuted()).Render(NewRect(1, 1, w, 8), buf)
				s.dump(fmt.Sprintf("para/textstyle/%s/%d", wrapNames[m], w), buf)
			}
		}
	}
	{
		line := LineFrom(
			Styled(" CONTROLS ", tStyleListSelected()),
			Styled("  ", tStyleHighlight()),
			Styled(" q ", tStyleListSelected()),
			Styled(" ", tStyleHighlight()),
			Styled("Quit", tStyleHighlight()),
			Styled("  •  ", tStyleMuted()),
			Styled(" ↑↓ ", tStyleListSelected()),
			Styled(" ", tStyleHighlight()),
			Styled("Navigate", tStyleHighlight()),
		)
		for _, w := range []int{10, 20, 30, 45, 60} {
			for _, h := range []int{1, 2} {
				buf := newTestBuf(w, h)
				ParagraphLine(line).Wrap(true).Render(NewRect(1, 1, w, h), buf)
				s.dump(fmt.Sprintf("para/status/%dx%d", w, h), buf)
			}
		}
	}
	for i, str := range []string{"first line\nsecond\n\nfourth", "", "single", "trailing newline\n", "crlf\r\nline"} {
		for m := 0; m < 3; m++ {
			buf := newTestBuf(8, 5)
			applyWrap(ParagraphStr(str), m).Style(tStyleWeird()).Render(NewRect(1, 1, 8, 5), buf)
			s.dump(fmt.Sprintf("para/str/%d/%s", i, wrapNames[m]), buf)
		}
	}
	ws := []string{
		"   ",
		" a",
		"a    b",
		"  ab  cd  ",
		"abcdefghij",
		"ab cdefghijklm no",
		"x y z long nbsp words",
		"zero​width​space",
		"     word",
		"word     ",
		"日本 語 日本語日本語",
	}
	for i, str := range ws {
		for _, w := range []int{1, 2, 3, 4, 6} {
			for m := 1; m < 3; m++ {
				buf := newTestBuf(w, 9)
				applyWrap(ParagraphStr(str), m).Render(NewRect(1, 1, w, 9), buf)
				s.dump(fmt.Sprintf("para/ws/%d/%s/%d", i, wrapNames[m], w), buf)
			}
		}
	}
}

// ---------- lists ----------

func items1() []ListItem {
	var out []ListItem
	for i := 0; i < 10; i++ {
		out = append(out, ListItemLine(LineFrom(
			Styled(fmt.Sprintf(" %-8s", fmt.Sprintf("svc%d", i)), tStyleLabel()),
			Styled("ok", tStyleSuccess()),
			Raw(" ·"),
		)))
	}
	return out
}

func items2() []ListItem {
	var out []ListItem
	for i := 0; i < 6; i++ {
		lines := []Line{
			LineSpan(Styled(fmt.Sprintf("Title %d", i), NewStyle().Add(Bold))),
			LineSpan(Styled(fmt.Sprintf("  detail %d", i), tStyleMuted())),
		}
		if i == 3 {
			lines = append(lines, LineStr("  third line").WithAlignment(AlignRight))
		}
		it := ListItemLines(lines...)
		if i == 4 {
			it = it.Style(NewStyle().BG(Blue).FG(White))
		}
		out = append(out, it)
	}
	return out
}

type namedList struct {
	name string
	list List
}

func testLists() []namedList {
	return []namedList{
		{"L1", NewList(items1()).Block(tPanel(tPanelTitle("Services"))).HighlightStyle(tStyleListSelected()).HighlightSymbol("▶ ")},
		{"L2", NewList(items1()).HighlightStyle(tStyleHighlight().Add(Bold)).HighlightSymbol("▸ ")},
		{"L3", NewList(items2()).HighlightStyle(tStyleListSelected()).HighlightSymbol("▶ ").RepeatHighlightSymbol(true)},
		{"L4", NewList(items2()).HighlightSymbol(">>").HighlightSpacing(HighlightAlways).Style(tStyleSurface())},
		{"L5", NewList(items1()).HighlightSymbol("▶ ").HighlightSpacing(HighlightNever).HighlightStyle(tStyleWeird())},
		{"L6", NewList(items2()).Block(BorderedBlock()).Style(tStyleSurfaceAlt()).HighlightStyle(tStyleWeird())},
	}
}

func optStr(v int, ok bool) string {
	if !ok {
		return "None"
	}
	return fmt.Sprintf("Some(%d)", v)
}

func stateStr(s ListState) string {
	sel, ok := s.Selected()
	return fmt.Sprintf("STATE offset=%d selected=%s", s.Offset(), optStr(sel, ok))
}

func listCases(s *caseSink) {
	states := []struct {
		name  string
		state ListState
	}{
		{"none", NewListState()},
		{"sel0", NewListState().WithSelected(0)},
		{"sel4", NewListState().WithSelected(4)},
		{"sel5", NewListState().WithSelected(5)},
		{"sel9", NewListState().WithSelected(9)},
		{"sel99", NewListState().WithSelected(99)},
		{"off7", NewListState().WithOffset(7)},
		{"off7sel2", NewListState().WithOffset(7).WithSelected(2)},
		{"off50", NewListState().WithOffset(50)},
		{"off3sel4", NewListState().WithOffset(3).WithSelected(4)},
	}
	for _, nl := range testLists() {
		for _, st := range states {
			for _, w := range []int{3, 20} {
				for _, h := range []int{0, 1, 2, 3, 5, 12} {
					buf := newTestBuf(w, h)
					state := st.state
					nl.list.RenderStateful(NewRect(1, 1, w, h), buf, &state)
					s.dump(fmt.Sprintf("list/%s/%s/%dx%d %s", nl.name, st.name, w, h, stateStr(state)), buf)
				}
			}
		}
		buf := newTestBuf(12, 4)
		nl.list.Render(NewRect(1, 1, 12, 4), buf)
		s.dump(fmt.Sprintf("list/%s/stateless", nl.name), buf)
	}
	for _, nl := range testLists() {
		var state ListState
		ops := []string{
			"first", "next", "next", "next", "next", "next", "next", "next", "next", "next", "next", "next", "prev", "prev",
			"prev", "prev", "prev", "prev", "prev", "prev", "none", "last", "prev", "up3", "down2", "sel1", "none", "prevnone",
		}
		for i, op := range ops {
			switch op {
			case "first":
				state.SelectFirst()
			case "next":
				state.SelectNext()
			case "prev", "prevnone":
				state.SelectPrevious()
			case "none":
				state.SelectNone()
			case "last":
				state.SelectLast()
			case "up3":
				state.ScrollUpBy(3)
			case "down2":
				state.ScrollDownBy(2)
			case "sel1":
				state.Select(1)
			}
			buf := newTestBuf(16, 5)
			nl.list.RenderStateful(NewRect(1, 1, 16, 5), buf, &state)
			s.dump(fmt.Sprintf("listseq/%s/%d:%s %s", nl.name, i, op, stateStr(state)), buf)
		}
	}
	{
		state := NewListState().WithSelected(3).WithOffset(2)
		buf := newTestBuf(6, 3)
		NewList(nil).HighlightSymbol("▶ ").RenderStateful(NewRect(1, 1, 6, 3), buf, &state)
		s.dump(fmt.Sprintf("list/empty %s", stateStr(state)), buf)
	}
	for _, h := range []int{1, 2, 3, 4} {
		items := []ListItem{ListItemStr("a\nb\nc"), ListItemStr("d"), ListItemLines(LineStr("e"), LineStr("f"), LineStr("g"))}
		for _, sel := range []int{-1, 0, 1, 2} {
			state := NewListState()
			if sel >= 0 {
				state = state.WithSelected(sel)
			}
			buf := newTestBuf(6, h)
			NewList(items).HighlightSymbol("▸ ").RenderStateful(NewRect(1, 1, 6, h), buf, &state)
			s.dump(fmt.Sprintf("list/tall/%d/%s %s", h, optStr(sel, sel >= 0), stateStr(state)), buf)
		}
	}
	{
		items := []ListItem{
			ListItemStr("plain string"),
			ListItemSpan(Styled("span item", tStyleAccent())),
			ListItemStr("multi\nline string"),
			ListItemStr(""),
			ListItemStr("wide 日本語").Style(tStyleWeird()),
		}
		state := NewListState().WithSelected(4)
		buf := newTestBuf(10, 6)
		NewList(items).HighlightSymbol("→").HighlightStyle(tStyleListSelected()).RenderStateful(NewRect(1, 1, 10, 6), buf, &state)
		s.dump(fmt.Sprintf("list/kinds %s", stateStr(state)), buf)
	}
}

// ---------- gauges ----------

func gaugeCases(s *caseSink) {
	for _, p := range []int{0, 1, 33, 50, 67, 99, 100} {
		for _, w := range []int{0, 1, 2, 3, 5, 10, 21} {
			for _, h := range []int{1, 2, 3} {
				g1 := NewGauge().
					Block(NewBlock()).
					GaugeStyle(NewStyle().FG(Green)).
					Percent(p).
					LabelStr("Overall: 45.3%")
				g2 := NewGauge().
					Block(NewBlock().TitleStr("RAM")).
					GaugeStyle(NewStyle().FG(Yellow)).
					Percent(p).
					LabelStr("8.0 GiB / 16.0 GiB (50%)")
				g3 := NewGauge().
					Block(tPanel(tPanelTitle("Disk"))).
					GaugeStyle(NewStyle().FG(tAccent).BG(tSurface).Add(Bold)).
					Style(tStyleSurface()).
					Percent(p)
				g4 := NewGauge().Percent(p).Label(Styled("lbl", tStyleWeird()))
				for _, ng := range []struct {
					name string
					g    Gauge
				}{{"G1", g1}, {"G2", g2}, {"G3", g3}, {"G4", g4}} {
					buf := newTestBuf(w, h)
					ng.g.Render(NewRect(1, 1, w, h), buf)
					s.dump(fmt.Sprintf("gauge/%s/%d/%dx%d", ng.name, p, w, h), buf)
				}
			}
		}
	}
	for _, r := range []float64{0.0, 0.005, 0.07, 0.123, 0.125, 0.5, 0.875, 0.999, 0.9951, 1.0} {
		for _, w := range []int{1, 3, 7, 10, 21} {
			for _, h := range []int{1, 3} {
				for _, uni := range []bool{false, true} {
					buf := newTestBuf(w, h)
					NewGauge().
						GaugeStyle(NewStyle().FG(Cyan).BG(Black)).
						Ratio(r).
						UseUnicode(uni).
						Render(NewRect(1, 1, w, h), buf)
					s.dump(fmt.Sprintf("gauge/ratio/%s/%t/%dx%d", strconv.FormatFloat(r, 'f', -1, 64), uni, w, h), buf)
				}
			}
		}
	}
}

// ---------- unicode / control characters ----------

func uniStrs() []string {
	return []string{
		"tab\there\tx",
		"ctl\u0007bell\u001besc",
		"fam \U0001f468\u200d\U0001f469\u200d\U0001f467 flag \U0001f1fa\U0001f1f8 ok",
		"key 1\ufe0f\u20e3 #\ufe0f\u20e3 \u25b6\ufe0f \u26a0\ufe0f \u2764\ufe0f",
		"thumbs \U0001f44d\U0001f3fd done",
		"e\u0301cole cafe\u0301",
		"\u0939\u093f\u0928\u094d\u0926\u0940 \u0915\u093e \u092a\u093e\u0920",
		"\ud55c\uae00 \ud14d\uc2a4\ud2b8 \u1100\u1161\u11a8",
		"ls\u2028sep",
		"cr\r\nlf",
		"\u2191\u2193 \u2190\u2192 \u2714 \u2713 \u2026",
	}
}

func uniCases(s *caseSink) {
	for i, str := range uniStrs() {
		for _, w := range []int{3, 6, 10, 16, 40} {
			for m := 0; m < 3; m++ {
				buf := newTestBuf(w, 6)
				applyWrap(ParagraphLine(LineSpan(Styled(str, tStyleAccent()))), m).Render(NewRect(1, 1, w, 6), buf)
				s.dump(fmt.Sprintf("uni/para/%d/%s/%d", i, wrapNames[m], w), buf)
			}
			buf := newTestBuf(w, 3)
			BorderedBlock().Title(LineSpan(Raw(str))).Render(NewRect(1, 1, w, 3), buf)
			s.dump(fmt.Sprintf("uni/title/%d/%d", i, w), buf)

			buf = newTestBuf(w, 2)
			state := NewListState().WithSelected(0)
			NewList([]ListItem{ListItemLine(LineSpan(Raw(str))), ListItemLine(LineSpan(Raw(str)).WithAlignment(AlignRight))}).
				HighlightSymbol("\u25b6 ").
				RenderStateful(NewRect(1, 1, w, 2), buf, &state)
			s.dump(fmt.Sprintf("uni/list/%d/%d", i, w), buf)

			buf = newTestBuf(w, 1)
			NewGauge().Percent(40).Label(Raw(str)).Render(NewRect(1, 1, w, 1), buf)
			s.dump(fmt.Sprintf("uni/gauge/%d/%d", i, w), buf)
		}
	}
}

// ---------- comparison ----------

func loadExpected(t *testing.T) ([]string, map[string]string) {
	t.Helper()
	f, err := os.Open("testdata/widgets_ratatui.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	var names []string
	dumps := map[string]string{}
	var cur string
	var sb strings.Builder
	flush := func() {
		if cur != "" {
			dumps[cur] = sb.String()
		}
		sb.Reset()
	}
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutPrefix(line, "=== "); ok {
			flush()
			cur = name
			names = append(names, name)
			continue
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	flush()
	return names, dumps
}

func firstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	for i := 0; i < max(len(wl), len(gl)); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w == g {
			continue
		}
		wc := strings.Split(w, "\t")
		gc := strings.Split(g, "\t")
		for x := 0; x < max(len(wc), len(gc)); x++ {
			var a, b string
			if x < len(wc) {
				a = wc[x]
			}
			if x < len(gc) {
				b = gc[x]
			}
			if a != b {
				return fmt.Sprintf("row %d col %d: want %q got %q", i, x, a, b)
			}
		}
	}
	return "identical"
}

func renderSymbols(dump string) string {
	var sb strings.Builder
	for _, row := range strings.Split(strings.TrimSuffix(dump, "\n"), "\n") {
		for _, c := range strings.Split(row, "\t") {
			sb.WriteString(strings.SplitN(c, "|", 2)[0])
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// knownDivergence lists cases that differ for reasons outside the widgets:
// uniStrs()[6] is Devanagari text whose virama conjuncts form a single
// grapheme under Unicode 15.1 rule GB9c (unicode-segmentation 1.12) but are
// split by uniseg 0.4.7 (Unicode 15.0).
func knownDivergence(name string) bool {
	for _, kind := range []string{"para", "title", "list", "gauge"} {
		if strings.HasPrefix(name, "uni/"+kind+"/6/") {
			return true
		}
	}
	return false
}

func TestWidgetsMatchRatatui(t *testing.T) {
	wantNames, want := loadExpected(t)
	s := &caseSink{dumps: map[string]string{}}
	blockCases(s)
	clearCases(s)
	paragraphCases(s)
	listCases(s)
	gaugeCases(s)
	uniCases(s)

	if len(wantNames) != len(s.names) {
		t.Errorf("case count: want %d got %d", len(wantNames), len(s.names))
	}
	failures, skipped := 0, 0
	for i, name := range wantNames {
		if knownDivergence(name) {
			skipped++
			continue
		}
		got, ok := s.dumps[name]
		if !ok {
			gotName := "<none>"
			if i < len(s.names) {
				gotName = s.names[i]
			}
			t.Errorf("missing case %q (case #%d here is %q)", name, i, gotName)
			failures++
		} else if got != want[name] {
			t.Errorf("case %q differs: %s\nwant:\n%sgot:\n%s", name, firstDiff(want[name], got),
				renderSymbols(want[name]), renderSymbols(got))
			failures++
		}
		if failures >= 15 {
			t.Fatalf("too many failures")
		}
	}
	t.Logf("compared %d cases (%d skipped as known divergences)", len(wantNames)-skipped, skipped)
}
