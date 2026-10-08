package tui

// Borders is a bitflag set of the sides of a block that have a border.
type Borders uint8

// Border flags (same bit values as ratatui).
const (
	BordersNone   Borders = 0
	BordersTop    Borders = 1 << 0
	BordersRight  Borders = 1 << 1
	BordersBottom Borders = 1 << 2
	BordersLeft   Borders = 1 << 3
	BordersAll            = BordersTop | BordersRight | BordersBottom | BordersLeft
)

// Contains reports whether all flags in o are set.
func (b Borders) Contains(o Borders) bool { return b&o == o }

// Intersects reports whether any flag in o is set.
func (b Borders) Intersects(o Borders) bool { return b&o != 0 }

// BorderType selects the symbols used to draw a block's border.
type BorderType uint8

const (
	BorderPlain BorderType = iota
	BorderRounded
	BorderDouble
	BorderThick
	BorderQuadrantInside
	BorderQuadrantOutside
)

// Set returns the symbol set for the border type.
func (t BorderType) Set() BorderSet {
	switch t {
	case BorderRounded:
		return BorderSetRounded
	case BorderDouble:
		return BorderSetDouble
	case BorderThick:
		return BorderSetThick
	case BorderQuadrantInside:
		return BorderSetQuadrantInside
	case BorderQuadrantOutside:
		return BorderSetQuadrantOutside
	default:
		return BorderSetPlain
	}
}

// TitlePosition is the vertical position of a block title.
type TitlePosition uint8

const (
	TitleTop TitlePosition = iota
	TitleBottom
)

// Padding is the inner spacing of a block.
type Padding struct {
	Left, Right, Top, Bottom int
}

type blockTitle struct {
	position    TitlePosition
	hasPosition bool
	line        Line
}

// Block draws borders and titles around an area. The zero value (or
// NewBlock()) has no borders, no titles and plain border symbols.
type Block struct {
	titles          []blockTitle
	titlesStyle     Style
	titlesAlignment Alignment
	titlesPosition  TitlePosition
	borders         Borders
	borderStyle     Style
	borderSet       BorderSet
	hasBorderSet    bool
	style           Style
	padding         Padding
}

// NewBlock returns a block without borders.
func NewBlock() Block { return Block{} }

// BorderedBlock returns a block with all borders.
func BorderedBlock() Block { return Block{borders: BordersAll} }

// Title appends a title line. Its position is the block's title position;
// its alignment is the line's own alignment or the block's title alignment.
func (b Block) Title(line Line) Block {
	b.titles = append(cloneTitles(b.titles), blockTitle{line: line})
	return b
}

// TitleStr appends a title built from a string.
func (b Block) TitleStr(s string) Block { return b.Title(LineStr(s)) }

// TitleTop appends a title positioned at the top border.
func (b Block) TitleTop(line Line) Block {
	b.titles = append(cloneTitles(b.titles), blockTitle{line: line, position: TitleTop, hasPosition: true})
	return b
}

// TitleBottom appends a title positioned at the bottom border.
func (b Block) TitleBottom(line Line) Block {
	b.titles = append(cloneTitles(b.titles), blockTitle{line: line, position: TitleBottom, hasPosition: true})
	return b
}

// TitleStyle sets the style applied to the title areas before the titles
// are rendered.
func (b Block) TitleStyle(s Style) Block {
	b.titlesStyle = s
	return b
}

// TitleAlignment sets the default alignment of titles.
func (b Block) TitleAlignment(a Alignment) Block {
	b.titlesAlignment = a
	return b
}

// TitlePosition sets the default position of titles.
func (b Block) TitlePosition(p TitlePosition) Block {
	b.titlesPosition = p
	return b
}

// Borders sets which borders are drawn.
func (b Block) Borders(flags Borders) Block {
	b.borders = flags
	return b
}

// BorderType sets the border symbols.
func (b Block) BorderType(t BorderType) Block {
	b.borderSet, b.hasBorderSet = t.Set(), true
	return b
}

// BorderSet sets custom border symbols.
func (b Block) BorderSet(set BorderSet) Block {
	b.borderSet, b.hasBorderSet = set, true
	return b
}

// BorderStyle sets the style of the borders.
func (b Block) BorderStyle(s Style) Block {
	b.borderStyle = s
	return b
}

// Style sets the base style of the whole block area.
func (b Block) Style(s Style) Block {
	b.style = s
	return b
}

// Padding sets the inner padding.
func (b Block) Padding(p Padding) Block {
	b.padding = p
	return b
}

// GetStyle returns the block's base style.
func (b Block) GetStyle() Style { return b.style }

func cloneTitles(t []blockTitle) []blockTitle {
	return append([]blockTitle(nil), t...)
}

func (b Block) set() BorderSet {
	if b.hasBorderSet {
		return b.borderSet
	}
	return BorderSetPlain
}

func (b Block) titlePos(t blockTitle) TitlePosition {
	if t.hasPosition {
		return t.position
	}
	return b.titlesPosition
}

func (b Block) hasTitleAt(p TitlePosition) bool {
	for _, t := range b.titles {
		if b.titlePos(t) == p {
			return true
		}
	}
	return false
}

// Inner returns the area inside the borders, titles and padding.
func (b Block) Inner(area Rect) Rect {
	inner := area
	if b.borders.Intersects(BordersLeft) {
		inner.X = min(inner.X+1, inner.Right())
		inner.Width = satSub(inner.Width, 1)
	}
	if b.borders.Intersects(BordersTop) || b.hasTitleAt(TitleTop) {
		inner.Y = min(inner.Y+1, inner.Bottom())
		inner.Height = satSub(inner.Height, 1)
	}
	if b.borders.Intersects(BordersRight) {
		inner.Width = satSub(inner.Width, 1)
	}
	if b.borders.Intersects(BordersBottom) || b.hasTitleAt(TitleBottom) {
		inner.Height = satSub(inner.Height, 1)
	}
	inner.X += b.padding.Left
	inner.Y += b.padding.Top
	inner.Width = satSub(inner.Width, b.padding.Left+b.padding.Right)
	inner.Height = satSub(inner.Height, b.padding.Top+b.padding.Bottom)
	return inner
}

// innerIfSome mirrors Option<Block>::inner_if_some.
func innerIfSome(b *Block, area Rect) Rect {
	if b == nil {
		return area
	}
	return b.Inner(area)
}

// Render draws the block.
func (b Block) Render(area Rect, buf *Buffer) {
	area = area.Intersection(buf.Area)
	if area.IsEmpty() {
		return
	}
	buf.SetStyle(area, b.style)
	b.renderBorders(area, buf)
	b.renderTitlePosition(TitleTop, area, buf)
	b.renderTitlePosition(TitleBottom, area, buf)
}

func (b Block) renderBorders(area Rect, buf *Buffer) {
	set := b.set()
	if b.borders.Contains(BordersLeft) {
		for y := area.Top(); y < area.Bottom(); y++ {
			buf.Cell(area.Left(), y).SetSymbol(set.VerticalLeft).SetStyle(b.borderStyle)
		}
	}
	if b.borders.Contains(BordersTop) {
		for x := area.Left(); x < area.Right(); x++ {
			buf.Cell(x, area.Top()).SetSymbol(set.HorizontalTop).SetStyle(b.borderStyle)
		}
	}
	if b.borders.Contains(BordersRight) {
		x := area.Right() - 1
		for y := area.Top(); y < area.Bottom(); y++ {
			buf.Cell(x, y).SetSymbol(set.VerticalRight).SetStyle(b.borderStyle)
		}
	}
	if b.borders.Contains(BordersBottom) {
		y := area.Bottom() - 1
		for x := area.Left(); x < area.Right(); x++ {
			buf.Cell(x, y).SetSymbol(set.HorizontalBottom).SetStyle(b.borderStyle)
		}
	}
	if b.borders.Contains(BordersRight | BordersBottom) {
		buf.Cell(area.Right()-1, area.Bottom()-1).SetSymbol(set.BottomRight).SetStyle(b.borderStyle)
	}
	if b.borders.Contains(BordersRight | BordersTop) {
		buf.Cell(area.Right()-1, area.Top()).SetSymbol(set.TopRight).SetStyle(b.borderStyle)
	}
	if b.borders.Contains(BordersLeft | BordersBottom) {
		buf.Cell(area.Left(), area.Bottom()-1).SetSymbol(set.BottomLeft).SetStyle(b.borderStyle)
	}
	if b.borders.Contains(BordersLeft | BordersTop) {
		buf.Cell(area.Left(), area.Top()).SetSymbol(set.TopLeft).SetStyle(b.borderStyle)
	}
}

// renderTitlePosition renders right, then center, then left titles; the
// order defines how overlapping titles are drawn.
func (b Block) renderTitlePosition(pos TitlePosition, area Rect, buf *Buffer) {
	b.renderRightTitles(pos, area, buf)
	b.renderCenterTitles(pos, area, buf)
	b.renderLeftTitles(pos, area, buf)
}

func (b Block) filteredTitles(pos TitlePosition, a Alignment) []Line {
	var out []Line
	for _, t := range b.titles {
		if b.titlePos(t) != pos {
			continue
		}
		align := b.titlesAlignment
		if t.line.HasAlignment {
			align = t.line.Alignment
		}
		if align != a {
			continue
		}
		out = append(out, t.line)
	}
	return out
}

func (b Block) titlesArea(area Rect, pos TitlePosition) Rect {
	left, right := 0, 0
	if b.borders.Contains(BordersLeft) {
		left = 1
	}
	if b.borders.Contains(BordersRight) {
		right = 1
	}
	y := area.Top()
	if pos == TitleBottom {
		y = area.Bottom() - 1
	}
	return Rect{
		X:      area.Left() + left,
		Y:      y,
		Width:  satSub(satSub(area.Width, left), right),
		Height: 1,
	}
}

func (b Block) renderRightTitles(pos TitlePosition, area Rect, buf *Buffer) {
	titles := b.filteredTitles(pos, AlignRight)
	ta := b.titlesArea(area, pos)
	for i := len(titles) - 1; i >= 0; i-- {
		if ta.IsEmpty() {
			break
		}
		title := titles[i]
		tw := title.Width()
		titleArea := Rect{
			X:      max(satSub(ta.Right(), tw), ta.Left()),
			Y:      ta.Y,
			Width:  min(tw, ta.Width),
			Height: ta.Height,
		}
		buf.SetStyle(titleArea, b.titlesStyle)
		title.Render(titleArea, buf)
		ta.Width = satSub(satSub(ta.Width, tw), 1)
	}
}

func (b Block) renderCenterTitles(pos TitlePosition, area Rect, buf *Buffer) {
	titles := b.filteredTitles(pos, AlignCenter)
	total := 0
	for _, t := range titles {
		total += t.Width() + 1
	}
	total = satSub(total, 1)
	ta := b.titlesArea(area, pos)
	ta.X = ta.Left() + satSub(ta.Width, total)/2
	for _, title := range titles {
		if ta.IsEmpty() {
			break
		}
		tw := title.Width()
		titleArea := Rect{X: ta.X, Y: ta.Y, Width: min(tw, ta.Width), Height: ta.Height}
		buf.SetStyle(titleArea, b.titlesStyle)
		title.Render(titleArea, buf)
		ta.X += tw + 1
		ta.Width = satSub(ta.Width, tw+1)
	}
}

func (b Block) renderLeftTitles(pos TitlePosition, area Rect, buf *Buffer) {
	titles := b.filteredTitles(pos, AlignLeft)
	ta := b.titlesArea(area, pos)
	for _, title := range titles {
		if ta.IsEmpty() {
			break
		}
		tw := title.Width()
		titleArea := Rect{X: ta.X, Y: ta.Y, Width: min(tw, ta.Width), Height: ta.Height}
		buf.SetStyle(titleArea, b.titlesStyle)
		title.Render(titleArea, buf)
		ta.X += tw + 1
		ta.Width = satSub(ta.Width, tw+1)
	}
}

func (b Block) horizontalSpace() (int, int) {
	left, right := b.padding.Left, b.padding.Right
	if b.borders.Contains(BordersLeft) {
		left++
	}
	if b.borders.Contains(BordersRight) {
		right++
	}
	return left, right
}

func (b Block) verticalSpace() (int, int) {
	top, bottom := b.padding.Top, b.padding.Bottom
	if b.borders.Contains(BordersTop) || b.hasTitleAt(TitleTop) {
		top++
	}
	if b.borders.Contains(BordersBottom) || b.hasTitleAt(TitleBottom) {
		bottom++
	}
	return top, bottom
}
