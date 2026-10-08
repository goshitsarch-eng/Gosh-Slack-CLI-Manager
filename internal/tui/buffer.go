package tui

// Cell is a single terminal cell.
type Cell struct {
	Symbol   string
	Fg, Bg   Color
	Modifier Modifier
	Skip     bool
}

// EmptyCell is a blank cell with default colors.
var EmptyCell = Cell{Symbol: " ", Fg: Reset, Bg: Reset}

// Reset clears the cell.
func (c *Cell) Reset() { *c = EmptyCell }

// SetSymbol replaces the cell's symbol.
func (c *Cell) SetSymbol(s string) *Cell {
	c.Symbol = s
	return c
}

// AppendSymbol appends to the cell's symbol (zero-width graphemes).
func (c *Cell) AppendSymbol(s string) *Cell {
	c.Symbol += s
	return c
}

// SetStyle applies a style on top of the cell's current attributes.
func (c *Cell) SetStyle(s Style) *Cell {
	if s.HasFg {
		c.Fg = s.Fg
	}
	if s.HasBg {
		c.Bg = s.Bg
	}
	c.Modifier |= s.AddModifier
	c.Modifier &^= s.SubModifier
	return c
}

// Style returns the cell's style.
func (c *Cell) Style() Style {
	return Style{Fg: c.Fg, Bg: c.Bg, HasFg: true, HasBg: true, AddModifier: c.Modifier}
}

// Buffer is a grid of cells covering Area.
type Buffer struct {
	Area    Rect
	Content []Cell
}

// NewBuffer returns an empty buffer for area.
func NewBuffer(area Rect) *Buffer {
	b := &Buffer{Area: area, Content: make([]Cell, area.Area())}
	for i := range b.Content {
		b.Content[i] = EmptyCell
	}
	return b
}

// Cell returns the cell at (x, y). It panics if out of bounds, like the
// original indexing operator.
func (b *Buffer) Cell(x, y int) *Cell {
	if x < b.Area.Left() || x >= b.Area.Right() || y < b.Area.Top() || y >= b.Area.Bottom() {
		panic("tui: buffer index out of bounds")
	}
	return &b.Content[(y-b.Area.Y)*b.Area.Width+(x-b.Area.X)]
}

// Resize resizes the buffer to area, resetting its content.
func (b *Buffer) Resize(area Rect) {
	n := area.Area()
	if cap(b.Content) >= n {
		b.Content = b.Content[:n]
	} else {
		b.Content = make([]Cell, n)
	}
	b.Area = area
	b.ResetAll()
}

// ResetAll resets every cell.
func (b *Buffer) ResetAll() {
	for i := range b.Content {
		b.Content[i] = EmptyCell
	}
}

// SetStyle applies style to every cell in area.
func (b *Buffer) SetStyle(area Rect, style Style) {
	area = b.Area.Intersection(area)
	for y := area.Top(); y < area.Bottom(); y++ {
		for x := area.Left(); x < area.Right(); x++ {
			b.Cell(x, y).SetStyle(style)
		}
	}
}

// SetString prints s at (x, y) until the end of the line.
func (b *Buffer) SetString(x, y int, s string, style Style) (int, int) {
	return b.SetStringN(x, y, s, 1<<16-1, style)
}

// SetStringN prints at most maxWidth columns of s at (x, y). Zero-width
// graphemes and control characters are skipped.
func (b *Buffer) SetStringN(x, y int, s string, maxWidth int, style Style) (int, int) {
	if maxWidth > 1<<16-1 {
		maxWidth = 1<<16 - 1
	}
	remaining := min(satSub(b.Area.Right(), x), maxWidth)
	for _, g := range Graphemes(s) {
		if HasControl(g) {
			continue
		}
		w := GraphemeWidth(g)
		if w == 0 {
			continue
		}
		if remaining < w {
			break
		}
		remaining -= w
		b.Cell(x, y).SetSymbol(g).SetStyle(style)
		next := x + w
		x++
		for x < next {
			b.Cell(x, y).Reset()
			x++
		}
	}
	return x, y
}

// SetLine prints a line at (x, y) using at most maxWidth columns.
func (b *Buffer) SetLine(x, y int, line Line, maxWidth int) (int, int) {
	remaining := maxWidth
	for _, span := range line.Spans {
		if remaining == 0 {
			break
		}
		px, _ := b.SetStringN(x, y, span.Content, remaining, line.Style.Patch(span.Style))
		w := satSub(px, x)
		x = px
		remaining = satSub(remaining, w)
	}
	return x, y
}

// SetSpan prints a span at (x, y) using at most maxWidth columns.
func (b *Buffer) SetSpan(x, y int, span Span, maxWidth int) (int, int) {
	return b.SetStringN(x, y, span.Content, maxWidth, span.Style)
}

// Frame is the drawing surface handed to render functions.
type Frame struct {
	Buf *Buffer
}

// Area returns the full frame area.
func (f *Frame) Area() Rect { return f.Buf.Area }

// Widget is anything that can draw itself into a buffer.
type Widget interface {
	Render(area Rect, buf *Buffer)
}

// RenderWidget draws w into area.
func (f *Frame) RenderWidget(w Widget, area Rect) { w.Render(area, f.Buf) }

// Buffer returns the frame buffer.
func (f *Frame) Buffer() *Buffer { return f.Buf }
