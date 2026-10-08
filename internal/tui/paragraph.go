package tui

// Paragraph displays text, optionally wrapped, scrolled and inside a block.
type Paragraph struct {
	block     *Block
	style     Style
	wrap      bool
	trim      bool
	text      Text
	scrollX   int
	scrollY   int
	alignment Alignment
}

// NewParagraph creates a paragraph from text.
func NewParagraph(text Text) Paragraph { return Paragraph{text: text} }

// ParagraphStr creates a paragraph from a string (one line per '\n').
func ParagraphStr(s string) Paragraph { return NewParagraph(TextStr(s)) }

// ParagraphLines creates a paragraph from lines.
func ParagraphLines(lines ...Line) Paragraph { return NewParagraph(TextLines(lines...)) }

// ParagraphLine creates a paragraph from a single line.
func ParagraphLine(line Line) Paragraph { return NewParagraph(TextLines(line)) }

// ParagraphSpan creates a paragraph from a single span.
func ParagraphSpan(span Span) Paragraph { return NewParagraph(TextLines(LineSpan(span))) }

// Block surrounds the paragraph with a block.
func (p Paragraph) Block(b Block) Paragraph {
	p.block = &b
	return p
}

// Style sets the base style of the whole widget area.
func (p Paragraph) Style(s Style) Paragraph {
	p.style = s
	return p
}

// Wrap enables word wrapping; trim strips leading whitespace of wrapped
// lines.
func (p Paragraph) Wrap(trim bool) Paragraph {
	p.wrap, p.trim = true, trim
	return p
}

// Scroll sets the vertical (y) and horizontal (x) scroll offsets.
func (p Paragraph) Scroll(y, x int) Paragraph {
	p.scrollY, p.scrollX = y, x
	return p
}

// Alignment sets the default alignment of lines without their own.
func (p Paragraph) Alignment(a Alignment) Paragraph {
	p.alignment = a
	return p
}

// Centered is Alignment(AlignCenter).
func (p Paragraph) Centered() Paragraph { return p.Alignment(AlignCenter) }

// RightAligned is Alignment(AlignRight).
func (p Paragraph) RightAligned() Paragraph { return p.Alignment(AlignRight) }

// Render draws the paragraph.
func (p Paragraph) Render(area Rect, buf *Buffer) {
	buf.SetStyle(area, p.style)
	if p.block != nil {
		p.block.Render(area, buf)
	}
	inner := innerIfSome(p.block, area)
	p.renderParagraph(inner, buf)
}

func (p Paragraph) composerInput() []composerLine {
	lines := make([]composerLine, len(p.text.Lines))
	for i, line := range p.text.Lines {
		align := p.alignment
		if line.HasAlignment {
			align = line.Alignment
		}
		lines[i] = composerLine{graphemes: line.StyledGraphemes(p.text.Style), alignment: align}
	}
	return lines
}

func (p Paragraph) renderParagraph(area Rect, buf *Buffer) {
	if area.IsEmpty() {
		return
	}
	buf.SetStyle(area, p.style)
	input := p.composerInput()
	if p.wrap {
		p.renderText(newWordWrapper(input, area.Width, p.trim), area, buf)
	} else {
		t := newLineTruncator(input, area.Width)
		t.horizontalOffset = p.scrollX
		p.renderText(t, area, buf)
	}
}

func getLineOffset(lineWidth, areaWidth int, a Alignment) int {
	switch a {
	case AlignCenter:
		return satSub(areaWidth/2, lineWidth/2)
	case AlignRight:
		return satSub(areaWidth, lineWidth)
	default:
		return 0
	}
}

func (p Paragraph) renderText(c lineComposer, area Rect, buf *Buffer) {
	y := 0
	for {
		wl, ok := c.nextLine()
		if !ok {
			break
		}
		if y >= p.scrollY {
			x := getLineOffset(wl.width, area.Width, wl.alignment)
			for _, g := range wl.line {
				w := GraphemeWidth(g.Symbol)
				if w == 0 {
					continue
				}
				symbol := g.Symbol
				if symbol == "" {
					symbol = " "
				}
				buf.Cell(area.Left()+x, area.Top()+y-p.scrollY).SetSymbol(symbol).SetStyle(g.Style)
				x += w
			}
		}
		y++
		if y >= area.Height+p.scrollY {
			break
		}
	}
}

// LineCount returns the number of rows the paragraph needs at width,
// including the block's top and bottom rows.
func (p Paragraph) LineCount(width int) int {
	if width < 1 {
		return 0
	}
	top, bottom := 0, 0
	if p.block != nil {
		top, bottom = p.block.verticalSpace()
	}
	count := 0
	if p.wrap {
		lines := make([]composerLine, len(p.text.Lines))
		for i, line := range p.text.Lines {
			align := p.alignment
			if line.HasAlignment {
				align = line.Alignment
			}
			var gs []StyledGrapheme
			for _, s := range line.Spans {
				gs = append(gs, s.StyledGraphemes(p.style)...)
			}
			lines[i] = composerLine{graphemes: gs, alignment: align}
		}
		ww := newWordWrapper(lines, width, p.trim)
		for {
			if _, ok := ww.nextLine(); !ok {
				break
			}
			count++
		}
	} else {
		count = p.text.Height()
	}
	return count + top + bottom
}

// LineWidth returns the width needed to show the widest line unwrapped,
// including the block's left and right columns.
func (p Paragraph) LineWidth() int {
	w := p.text.Width()
	if p.block != nil {
		l, r := p.block.horizontalSpace()
		w += l + r
	}
	return w
}
