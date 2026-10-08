package tui

// ListItem is one entry of a List.
type ListItem struct {
	Content Text
	style   Style
}

// NewListItem creates an item from text.
func NewListItem(content Text) ListItem { return ListItem{Content: content} }

// ListItemStr creates an item from a string (one line per '\n').
func ListItemStr(s string) ListItem { return NewListItem(TextStr(s)) }

// ListItemLine creates a single-line item.
func ListItemLine(line Line) ListItem { return NewListItem(TextLines(line)) }

// ListItemLines creates a multi-line item.
func ListItemLines(lines ...Line) ListItem { return NewListItem(TextLines(lines...)) }

// ListItemSpan creates a single-line item from a span.
func ListItemSpan(span Span) ListItem { return NewListItem(TextLines(LineSpan(span))) }

// Style sets the item's style.
func (i ListItem) Style(s Style) ListItem {
	i.style = s
	return i
}

// Height returns the number of lines of the item.
func (i ListItem) Height() int { return i.Content.Height() }

// Width returns the width of the widest line of the item.
func (i ListItem) Width() int { return i.Content.Width() }

// HighlightSpacing controls when the highlight symbol column is reserved.
type HighlightSpacing uint8

const (
	// HighlightWhenSelected reserves the column only while an item is
	// selected (the default).
	HighlightWhenSelected HighlightSpacing = iota
	HighlightAlways
	HighlightNever
)

func (h HighlightSpacing) shouldAdd(hasSelection bool) bool {
	switch h {
	case HighlightAlways:
		return true
	case HighlightNever:
		return false
	default:
		return hasSelection
	}
}

// ListState holds the scroll offset and selection of a List.
type ListState struct {
	offset      int
	selected    int
	hasSelected bool
}

// NewListState returns a state with no selection and offset 0.
func NewListState() ListState { return ListState{} }

// WithOffset returns the state with the offset set.
func (s ListState) WithOffset(offset int) ListState {
	s.offset = offset
	return s
}

// WithSelected returns the state with index selected.
func (s ListState) WithSelected(index int) ListState {
	s.selected, s.hasSelected = index, true
	return s
}

// WithSelectedNone returns the state without a selection (offset kept).
func (s ListState) WithSelectedNone() ListState {
	s.selected, s.hasSelected = 0, false
	return s
}

// Offset returns the index of the first visible item.
func (s ListState) Offset() int { return s.offset }

// SetOffset sets the offset (like *offset_mut() = n).
func (s *ListState) SetOffset(offset int) { s.offset = offset }

// Selected returns the selected index, if any.
func (s ListState) Selected() (int, bool) { return s.selected, s.hasSelected }

// Select selects index.
func (s *ListState) Select(index int) { s.selected, s.hasSelected = index, true }

// SelectNone clears the selection and resets the offset to 0.
func (s *ListState) SelectNone() {
	s.selected, s.hasSelected = 0, false
	s.offset = 0
}

// SelectOpt selects index when ok, otherwise clears the selection.
func (s *ListState) SelectOpt(index int, ok bool) {
	if ok {
		s.Select(index)
	} else {
		s.SelectNone()
	}
}

const maxIndexValue = int(^uint(0) >> 1)

// SelectNext selects the next item (or the first when none is selected).
func (s *ListState) SelectNext() {
	next := 0
	if s.hasSelected {
		next = saturatingAddInt(s.selected, 1)
	}
	s.Select(next)
}

// SelectPrevious selects the previous item (or the last when none is
// selected).
func (s *ListState) SelectPrevious() {
	prev := maxIndexValue
	if s.hasSelected {
		prev = satSub(s.selected, 1)
	}
	s.Select(prev)
}

// SelectFirst selects the first item.
func (s *ListState) SelectFirst() { s.Select(0) }

// SelectLast selects the last item (clamped on the next render).
func (s *ListState) SelectLast() { s.Select(maxIndexValue) }

// ScrollDownBy moves the selection down by amount.
func (s *ListState) ScrollDownBy(amount int) {
	sel := 0
	if s.hasSelected {
		sel = s.selected
	}
	s.Select(saturatingAddInt(sel, amount))
}

// ScrollUpBy moves the selection up by amount.
func (s *ListState) ScrollUpBy(amount int) {
	sel := 0
	if s.hasSelected {
		sel = s.selected
	}
	s.Select(satSub(sel, amount))
}

func saturatingAddInt(a, b int) int {
	if a > maxIndexValue-b {
		return maxIndexValue
	}
	return a + b
}

// List displays items top to bottom with an optional highlighted selection.
type List struct {
	block                 *Block
	items                 []ListItem
	style                 Style
	highlightStyle        Style
	highlightSymbol       string
	repeatHighlightSymbol bool
	highlightSpacing      HighlightSpacing
	scrollPadding         int
}

// NewList creates a list from items.
func NewList(items []ListItem) List { return List{items: items} }

// Items replaces the items.
func (l List) Items(items []ListItem) List {
	l.items = items
	return l
}

// Block surrounds the list with a block.
func (l List) Block(b Block) List {
	l.block = &b
	return l
}

// Style sets the base style of the widget area.
func (l List) Style(s Style) List {
	l.style = s
	return l
}

// HighlightSymbol sets the symbol drawn before the selected item.
func (l List) HighlightSymbol(s string) List {
	l.highlightSymbol = s
	return l
}

// HighlightStyle sets the style patched over the selected row.
func (l List) HighlightStyle(s Style) List {
	l.highlightStyle = s
	return l
}

// RepeatHighlightSymbol draws the symbol on every line of a multi-line
// selected item.
func (l List) RepeatHighlightSymbol(repeat bool) List {
	l.repeatHighlightSymbol = repeat
	return l
}

// HighlightSpacing sets when the highlight column is reserved.
func (l List) HighlightSpacing(h HighlightSpacing) List {
	l.highlightSpacing = h
	return l
}

// ScrollPadding keeps this many items visible around the selection.
func (l List) ScrollPadding(n int) List {
	l.scrollPadding = n
	return l
}

// Len returns the number of items.
func (l List) Len() int { return len(l.items) }

// Render draws the list with a fresh default state.
func (l List) Render(area Rect, buf *Buffer) {
	var state ListState
	l.RenderStateful(area, buf, &state)
}

// RenderStateful draws the list, updating the state's offset (and clamping
// its selection) like the original.
func (l List) RenderStateful(area Rect, buf *Buffer, state *ListState) {
	buf.SetStyle(area, l.style)
	if l.block != nil {
		l.block.Render(area, buf)
	}
	listArea := innerIfSome(l.block, area)
	if listArea.IsEmpty() {
		return
	}
	if len(l.items) == 0 {
		state.SelectNone()
		return
	}

	// If the selected index is out of bounds, set it to the last item.
	if state.hasSelected && state.selected >= len(l.items) {
		state.Select(satSub(len(l.items), 1))
	}

	listHeight := listArea.Height
	first, last := l.getItemsBounds(state.selected, state.hasSelected, state.offset, listHeight)
	state.offset = first

	highlightSymbol := l.highlightSymbol
	symbolWidth := StrWidth(highlightSymbol)
	blankSymbol := ""
	for i := 0; i < symbolWidth; i++ {
		blankSymbol += " "
	}

	currentHeight := 0
	selectionSpacing := l.highlightSpacing.shouldAdd(state.hasSelected)
	for i := state.offset; i < len(l.items) && i < state.offset+(last-first); i++ {
		item := l.items[i]
		x := listArea.Left()
		y := listArea.Top() + currentHeight
		currentHeight += item.Height()

		rowArea := Rect{X: x, Y: y, Width: listArea.Width, Height: item.Height()}
		itemStyle := l.style.Patch(item.style)
		buf.SetStyle(rowArea, itemStyle)

		isSelected := state.hasSelected && state.selected == i
		itemArea := rowArea
		if selectionSpacing {
			itemArea = Rect{
				X:      rowArea.X + symbolWidth,
				Y:      rowArea.Y,
				Width:  satSub(rowArea.Width, symbolWidth),
				Height: rowArea.Height,
			}
		}
		item.Content.Render(itemArea, buf)

		for j := 0; j < item.Content.Height(); j++ {
			symbol := blankSymbol
			if isSelected && (j == 0 || l.repeatHighlightSymbol) {
				symbol = highlightSymbol
			}
			if selectionSpacing {
				buf.SetStringN(x, y+j, symbol, listArea.Width, itemStyle)
			}
		}

		if isSelected {
			buf.SetStyle(rowArea, l.highlightStyle)
		}
	}
}

func (l List) getItemsBounds(selected int, hasSelected bool, offset, maxHeight int) (int, int) {
	offset = min(offset, satSub(len(l.items), 1))

	first := offset
	last := offset
	heightFromOffset := 0

	for _, item := range l.items[offset:] {
		if heightFromOffset+item.Height() > maxHeight {
			break
		}
		heightFromOffset += item.Height()
		last++
	}

	indexToDisplay := offset
	if hasSelected {
		indexToDisplay = l.applyScrollPaddingToSelectedIndex(selected, maxHeight, first, last)
	}

	for indexToDisplay >= last {
		heightFromOffset = saturatingAddInt(heightFromOffset, l.items[last].Height())
		last++
		for heightFromOffset > maxHeight {
			heightFromOffset = satSub(heightFromOffset, l.items[first].Height())
			first++
		}
	}

	for indexToDisplay < first {
		first--
		heightFromOffset = saturatingAddInt(heightFromOffset, l.items[first].Height())
		for heightFromOffset > maxHeight {
			last--
			heightFromOffset = satSub(heightFromOffset, l.items[last].Height())
		}
	}

	return first, last
}

func (l List) applyScrollPaddingToSelectedIndex(selected, maxHeight, first, last int) int {
	lastValid := satSub(len(l.items), 1)
	selected = min(selected, lastValid)

	scrollPadding := l.scrollPadding
	for scrollPadding > 0 {
		height := 0
		hi := min(saturatingAddInt(selected, scrollPadding), lastValid)
		for index := satSub(selected, scrollPadding); index <= hi; index++ {
			height += l.items[index].Height()
		}
		if height <= maxHeight {
			break
		}
		scrollPadding--
	}

	var r int
	switch {
	case min(selected+scrollPadding, lastValid) >= last:
		r = selected + scrollPadding
	case satSub(selected, scrollPadding) < first:
		r = satSub(selected, scrollPadding)
	default:
		r = selected
	}
	return min(r, lastValid)
}
