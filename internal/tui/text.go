package tui

import "strings"

// Alignment is horizontal text alignment.
type Alignment uint8

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

// Span is a styled piece of text.
type Span struct {
	Content string
	Style   Style
}

// Raw returns an unstyled span.
func Raw(s string) Span { return Span{Content: s} }

// Styled returns a styled span.
func Styled(s string, style Style) Span { return Span{Content: s, Style: style} }

// Width returns the display width of the span.
func (s Span) Width() int { return StrWidth(s.Content) }

// StyledGrapheme is a grapheme with its effective style.
type StyledGrapheme struct {
	Symbol string
	Style  Style
}

// StyledGraphemes yields the span's graphemes patched over base.
func (s Span) StyledGraphemes(base Style) []StyledGrapheme {
	return s.appendStyledGraphemes(make([]StyledGrapheme, 0, len(s.Content)), base)
}

func (s Span) appendStyledGraphemes(out []StyledGrapheme, base Style) []StyledGrapheme {
	style := base.Patch(s.Style)
	rest, state := s.Content, -1
	for rest != "" {
		var g string
		g, rest, state = NextGrapheme(rest, state)
		if g == "\n" {
			continue
		}
		out = append(out, StyledGrapheme{Symbol: g, Style: style})
	}
	return out
}

// Render draws the span into area on a single row.
func (s Span) Render(area Rect, buf *Buffer) {
	area = area.Intersection(buf.Area)
	if area.IsEmpty() {
		return
	}
	x, y := area.X, area.Y
	style := Style{}.Patch(s.Style)
	rest, state := s.Content, -1
	for i := 0; rest != ""; {
		var sym string
		sym, rest, state = NextGrapheme(rest, state)
		if sym == "\n" {
			continue
		}
		g := StyledGrapheme{Symbol: sym, Style: style}
		w := GraphemeWidth(g.Symbol)
		next := x + w
		if next > area.Right() {
			break
		}
		switch {
		case i == 0:
			buf.Cell(x, y).SetSymbol(g.Symbol).SetStyle(g.Style)
		case x == area.X:
			buf.Cell(x, y).AppendSymbol(g.Symbol).SetStyle(g.Style)
		case w == 0:
			buf.Cell(x-1, y).AppendSymbol(g.Symbol).SetStyle(g.Style)
		default:
			buf.Cell(x, y).SetSymbol(g.Symbol).SetStyle(g.Style)
		}
		for hx := x + 1; hx < next; hx++ {
			buf.Cell(hx, y).Reset()
		}
		x = next
		i++
	}
}

// Line is a single row of spans.
type Line struct {
	Spans        []Span
	Style        Style
	Alignment    Alignment
	HasAlignment bool
}

// RustLines splits s the way Rust's str::lines does: on '\n', stripping a
// trailing '\r', without a final empty line.
func RustLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	last := len(parts) - 1
	if parts[last] == "" {
		parts = parts[:last]
	}
	for i := range parts {
		// Only "\r\n" is a line ending; a final "\r" without "\n" stays.
		if i < last {
			parts[i] = strings.TrimSuffix(parts[i], "\r")
		}
	}
	return parts
}

// LineStr builds an unstyled line from a string (newlines start new spans,
// matching the original conversion).
func LineStr(s string) Line {
	var spans []Span
	for _, l := range RustLines(s) {
		spans = append(spans, Raw(l))
	}
	return Line{Spans: spans}
}

// LineFrom builds a line from spans.
func LineFrom(spans ...Span) Line { return Line{Spans: spans} }

// LineSpan builds a line from a single span.
func LineSpan(span Span) Line { return Line{Spans: []Span{span}} }

// StyledLine builds a styled line from a string.
func StyledLine(s string, style Style) Line {
	l := LineStr(s)
	l.Style = style
	return l
}

// WithStyle returns the line with style set.
func (l Line) WithStyle(style Style) Line {
	l.Style = style
	return l
}

// WithAlignment returns the line with an explicit alignment.
func (l Line) WithAlignment(a Alignment) Line {
	l.Alignment, l.HasAlignment = a, true
	return l
}

// Width returns the display width of the line.
func (l Line) Width() int {
	w := 0
	for _, s := range l.Spans {
		w += s.Width()
	}
	return w
}

// StyledGraphemes yields every grapheme of the line patched over base.
func (l Line) StyledGraphemes(base Style) []StyledGrapheme {
	style := base.Patch(l.Style)
	n := 0
	for _, s := range l.Spans {
		n += len(s.Content)
	}
	out := make([]StyledGrapheme, 0, n)
	for _, s := range l.Spans {
		out = s.appendStyledGraphemes(out, style)
	}
	return out
}

// Render draws the line into area.
func (l Line) Render(area Rect, buf *Buffer) {
	l.RenderWithAlignment(area, buf, AlignLeft, false)
}

// RenderWithAlignment draws the line, using the parent alignment when the
// line has none of its own.
func (l Line) RenderWithAlignment(area Rect, buf *Buffer, parent Alignment, hasParent bool) {
	area = area.Intersection(buf.Area)
	if area.IsEmpty() {
		return
	}
	area.Height = 1
	lineWidth := l.Width()
	if lineWidth == 0 {
		return
	}
	buf.SetStyle(area, l.Style)

	alignment, hasAlignment := l.Alignment, l.HasAlignment
	if !hasAlignment {
		alignment, hasAlignment = parent, hasParent
	}
	if !hasAlignment {
		alignment = AlignLeft
	}

	if lineWidth <= area.Width {
		indent := 0
		switch alignment {
		case AlignCenter:
			indent = satSub(area.Width, lineWidth) / 2
		case AlignRight:
			indent = satSub(area.Width, lineWidth)
		}
		renderSpans(l.Spans, area.IndentX(indent), buf, 0)
	} else {
		skip := 0
		switch alignment {
		case AlignCenter:
			skip = satSub(lineWidth, area.Width) / 2
		case AlignRight:
			skip = satSub(lineWidth, area.Width)
		}
		renderSpans(l.Spans, area, buf, skip)
	}
}

func renderSpans(spans []Span, area Rect, buf *Buffer, skip int) {
	for _, span := range spans {
		spanWidth := span.Width()
		if skip >= spanWidth {
			skip -= spanWidth
			continue
		}
		available := spanWidth - skip
		skip = 0
		offset := 0
		if spanWidth > available {
			content, actual := truncateStart(span.Content, available)
			offset = satSub(available, actual)
			span = Span{Content: content, Style: span.Style}
			spanWidth = actual
		}
		area = area.IndentX(offset)
		if area.IsEmpty() {
			break
		}
		span.Render(area, buf)
		area = area.IndentX(spanWidth)
	}
}

// truncateStart keeps the trailing graphemes of s that fit within width
// (unicode-truncate's unicode_truncate_start). That crate measures with
// unicode-width 0.1, which differs from 0.2 only in giving '\n' (and so
// "\r\n") zero width.
func truncateStart(s string, width int) (string, int) {
	gs := Graphemes(s)
	used := 0
	i := len(gs)
	for i > 0 {
		w := GraphemeWidth(gs[i-1])
		if gs[i-1] == "\n" || gs[i-1] == "\r\n" {
			w = 0
		}
		if used+w > width {
			break
		}
		used += w
		i--
	}
	return strings.Join(gs[i:], ""), used
}

// Text is a multi-line block of styled text.
type Text struct {
	Lines        []Line
	Style        Style
	Alignment    Alignment
	HasAlignment bool
}

// TextStr converts a string into text, one line per '\n'. An empty string
// yields a single empty line.
func TextStr(s string) Text {
	if s == "" {
		return Text{Lines: []Line{{}}}
	}
	var lines []Line
	for _, l := range RustLines(s) {
		lines = append(lines, LineStr(l))
	}
	return Text{Lines: lines}
}

// TextLines builds text from lines.
func TextLines(lines ...Line) Text { return Text{Lines: lines} }

// Height returns the number of lines.
func (t Text) Height() int { return len(t.Lines) }

// Width returns the width of the widest line.
func (t Text) Width() int {
	w := 0
	for _, l := range t.Lines {
		w = max(w, l.Width())
	}
	return w
}

// Render draws the text, one line per row of area.
func (t Text) Render(area Rect, buf *Buffer) {
	area = area.Intersection(buf.Area)
	buf.SetStyle(area, t.Style)
	for i, line := range t.Lines {
		if i >= area.Height {
			break
		}
		row := Rect{X: area.X, Y: area.Y + i, Width: area.Width, Height: 1}
		line.RenderWithAlignment(row, buf, t.Alignment, t.HasAlignment)
	}
}
