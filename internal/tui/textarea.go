package tui

// This file ports the subset of the tui-textarea 0.7.0 crate the application
// uses: a multi-line editor widget with emacs-like default key bindings,
// undo/redo history, a yank buffer, selection and a scrolling viewport. The
// rendered output matches tui-textarea + ratatui 0.29 cell for cell.

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// BlockWidget is the subset of the Block widget API the text area needs to
// draw its surrounding block.
type BlockWidget interface {
	Widget
	Inner(area Rect) Rect
}

// taPos is a position in the text: row, char column and byte offset.
type taPos struct {
	row, col, offset int
}

// yankText is the content of the yank buffer: a single piece of text or a
// chunk spanning multiple lines.
type yankText struct {
	chunk bool
	lines []string // one element when !chunk
}

func yankPiece(s string) yankText { return yankText{lines: []string{s}} }

// yankFromLines mirrors `From<Vec<String>> for YankText`.
func yankFromLines(c []string) yankText {
	switch len(c) {
	case 0:
		return yankPiece("")
	case 1:
		return yankPiece(c[0])
	default:
		return yankText{chunk: true, lines: c}
	}
}

func (y yankText) String() string {
	if y.chunk {
		return strings.Join(y.lines, "\n")
	}
	return y.lines[0]
}

// TextArea is a multi-line text editor widget.
type TextArea struct {
	lines           []string
	block           BlockWidget
	style           Style
	cursor          [2]int // row, col (0-based, col counted in chars)
	tabLen          uint8
	hardTabIndent   bool
	history         taHistory
	cursorLineStyle Style
	viewport        taViewport
	cursorStyle     Style
	yank            yankText
	mask            rune
	hasMask         bool
	selectionStart  *[2]int
	selectStyle     Style
}

// NewTextArea creates a text area holding lines. A nil or empty slice yields
// a single empty line.
func NewTextArea(lines []string) *TextArea {
	ls := make([]string, len(lines))
	copy(ls, lines)
	if len(ls) == 0 {
		ls = append(ls, "")
	}
	return &TextArea{
		lines:           ls,
		tabLen:          4,
		history:         newTaHistory(50),
		cursorLineStyle: NewStyle().Add(Underlined),
		cursorStyle:     NewStyle().Add(Reversed),
		yank:            yankPiece(""),
		selectStyle:     NewStyle().BG(LightBlue),
	}
}

// --- input -----------------------------------------------------------------

type taKey uint8

const (
	taKeyNull taKey = iota
	taKeyChar
	taKeyF
	taKeyBackspace
	taKeyEnter
	taKeyLeft
	taKeyRight
	taKeyUp
	taKeyDown
	taKeyTab
	taKeyDelete
	taKeyHome
	taKeyEnd
	taKeyPageUp
	taKeyPageDown
	taKeyEsc
)

type taInput struct {
	key              taKey
	ch               rune
	f                int
	ctrl, alt, shift bool
}

// taInputFromKey mirrors tui-textarea's `From<crossterm::KeyEvent> for Input`.
func taInputFromKey(k KeyEvent) taInput {
	in := taInput{
		ctrl:  k.Modifiers&ModControl != 0,
		alt:   k.Modifiers&ModAlt != 0,
		shift: k.Modifiers&ModShift != 0,
	}
	switch k.Code {
	case KeyChar:
		in.key, in.ch = taKeyChar, k.Rune
	case KeyBackspace:
		in.key = taKeyBackspace
	case KeyEnter:
		in.key = taKeyEnter
	case KeyLeft:
		in.key = taKeyLeft
	case KeyRight:
		in.key = taKeyRight
	case KeyUp:
		in.key = taKeyUp
	case KeyDown:
		in.key = taKeyDown
	case KeyTab:
		in.key = taKeyTab
	case KeyDelete:
		in.key = taKeyDelete
	case KeyHome:
		in.key = taKeyHome
	case KeyEnd:
		in.key = taKeyEnd
	case KeyPageUp:
		in.key = taKeyPageUp
	case KeyPageDown:
		in.key = taKeyPageDown
	case KeyEsc:
		in.key = taKeyEsc
	case KeyF:
		in.key, in.f = taKeyF, k.F
	default:
		in.key = taKeyNull
	}
	return in
}

func (in taInput) isChar(c rune) bool { return in.key == taKeyChar && in.ch == c }

// Input handles a key event with the default key bindings and reports
// whether the text was modified.
func (t *TextArea) Input(key KeyEvent) bool {
	in := taInputFromKey(key)
	c, a, sh := in.ctrl, in.alt, in.shift
	k := in.key

	switch {
	case (in.isChar('m') && c && !a) ||
		((in.isChar('\n') || in.isChar('\r')) && !c && !a) ||
		k == taKeyEnter:
		t.InsertNewline()
		return true
	case k == taKeyChar && !c && !a:
		t.InsertChar(in.ch)
		return true
	case k == taKeyTab && !c && !a:
		return t.InsertTab()
	case (in.isChar('h') && c && !a) || (k == taKeyBackspace && !c && !a):
		return t.DeleteChar()
	case (in.isChar('d') && c && !a) || (k == taKeyDelete && !c && !a):
		return t.DeleteNextChar()
	case in.isChar('k') && c && !a:
		return t.DeleteLineByEnd()
	case in.isChar('j') && c && !a:
		return t.DeleteLineByHead()
	case (in.isChar('w') && c && !a) || (in.isChar('h') && !c && a) ||
		(k == taKeyBackspace && !c && a):
		return t.DeleteWord()
	case (k == taKeyDelete && !c && a) || (in.isChar('d') && !c && a):
		return t.DeleteNextWord()
	case (in.isChar('n') && c && !a) || (k == taKeyDown && !c && !a):
		t.moveCursorWithShift(CursorDown, sh)
	case (in.isChar('p') && c && !a) || (k == taKeyUp && !c && !a):
		t.moveCursorWithShift(CursorUp, sh)
	case (in.isChar('f') && c && !a) || (k == taKeyRight && !c && !a):
		t.moveCursorWithShift(CursorForward, sh)
	case (in.isChar('b') && c && !a) || (k == taKeyLeft && !c && !a):
		t.moveCursorWithShift(CursorBack, sh)
	case (in.isChar('a') && c && !a) || k == taKeyHome ||
		((k == taKeyLeft || in.isChar('b')) && c && a):
		t.moveCursorWithShift(CursorHead, sh)
	case (in.isChar('e') && c && !a) || k == taKeyEnd ||
		((k == taKeyRight || in.isChar('f')) && c && a):
		t.moveCursorWithShift(CursorEnd, sh)
	case (in.isChar('<') && !c && a) || ((k == taKeyUp || in.isChar('p')) && c && a):
		t.moveCursorWithShift(CursorTop, sh)
	case (in.isChar('>') && !c && a) || ((k == taKeyDown || in.isChar('n')) && c && a):
		t.moveCursorWithShift(CursorBottom, sh)
	case (in.isChar('f') && !c && a) || (k == taKeyRight && c && !a):
		t.moveCursorWithShift(CursorWordForward, sh)
	case (in.isChar('b') && !c && a) || (k == taKeyLeft && c && !a):
		t.moveCursorWithShift(CursorWordBack, sh)
	case (in.isChar(']') && !c && a) || (in.isChar('n') && !c && a) ||
		(k == taKeyDown && c && !a):
		t.moveCursorWithShift(CursorParagraphForward, sh)
	case (in.isChar('[') && !c && a) || (in.isChar('p') && !c && a) ||
		(k == taKeyUp && c && !a):
		t.moveCursorWithShift(CursorParagraphBack, sh)
	case in.isChar('u') && c && !a:
		return t.Undo()
	case in.isChar('r') && c && !a:
		return t.Redo()
	case in.isChar('y') && c && !a:
		return t.Paste()
	case in.isChar('x') && c && !a:
		return t.Cut()
	case in.isChar('c') && c && !a:
		t.Copy()
	case (in.isChar('v') && c && !a) || k == taKeyPageDown:
		t.scrollWithShift(taScrollPageDown, sh)
	case (in.isChar('v') && !c && a) || k == taKeyPageUp:
		t.scrollWithShift(taScrollPageUp, sh)
	}
	return false
}

// --- editing ---------------------------------------------------------------

// taCharOffset returns the byte offset of the col-th char of line, or len(line).
func taCharOffset(line string, col int) int {
	n := 0
	for i := range line {
		if n == col {
			return i
		}
		n++
	}
	return len(line)
}

// taCharAt returns the byte offset and rune of the col-th char of line.
func taCharAt(line string, col int) (int, rune, bool) {
	n := 0
	for i, r := range line {
		if n == col {
			return i, r, true
		}
		n++
	}
	return 0, 0, false
}

func taCharCount(s string) int { return utf8.RuneCountInString(s) }

func (t *TextArea) pushHistory(kind taEditKind, before taPos, afterOffset int) {
	after := taPos{t.cursor[0], t.cursor[1], afterOffset}
	t.history.push(taEdit{kind: kind, before: before, after: after})
}

// InsertChar inserts a character at the cursor.
func (t *TextArea) InsertChar(c rune) {
	if c == '\n' || c == '\r' {
		t.InsertNewline()
		return
	}
	t.deleteSelection(false)
	row, col := t.cursor[0], t.cursor[1]
	line := t.lines[row]
	i := taCharOffset(line, col)
	t.lines[row] = line[:i] + string(c) + line[i:]
	t.cursor[1]++
	t.pushHistory(taEditKind{op: taEditInsertChar, ch: c}, taPos{row, col, i}, i+utf8.RuneLen(c))
}

// InsertStr inserts a string (which may contain newlines) at the cursor.
func (t *TextArea) InsertStr(s string) bool {
	modified := t.deleteSelection(false)
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = strings.TrimSuffix(p, "\r")
	}
	switch len(parts) {
	case 0:
		return modified
	case 1:
		return t.insertPiece(parts[0])
	default:
		return t.insertChunk(parts)
	}
}

func (t *TextArea) insertChunk(chunk []string) bool {
	row, col := t.cursor[0], t.cursor[1]
	i := taCharOffset(t.lines[row], col)
	before := taPos{row, col, i}

	row, col = row+len(chunk)-1, taCharCount(chunk[len(chunk)-1])
	t.cursor = [2]int{row, col}

	endOffset := len(chunk[len(chunk)-1])
	c := make([]string, len(chunk))
	copy(c, chunk)
	edit := taEditKind{op: taEditInsertChunk, chunk: c}
	edit.apply(&t.lines, before, taPos{row, col, endOffset})
	t.pushHistory(edit, before, endOffset)
	return true
}

func (t *TextArea) insertPiece(s string) bool {
	if s == "" {
		return false
	}
	row, col := t.cursor[0], t.cursor[1]
	line := t.lines[row]
	i := taCharOffset(line, col)
	t.lines[row] = line[:i] + s + line[i:]
	endOffset := i + len(s)
	t.cursor[1] += taCharCount(s)
	t.pushHistory(taEditKind{op: taEditInsertStr, str: s}, taPos{row, col, i}, endOffset)
	return true
}

func (t *TextArea) deleteRange(start, end taPos, shouldYank bool) {
	t.cursor = [2]int{start.row, start.col}

	if start.row == end.row {
		line := t.lines[start.row]
		removed := line[start.offset:end.offset]
		t.lines[start.row] = line[:start.offset] + line[end.offset:]
		if shouldYank {
			t.yank = yankPiece(removed)
		}
		t.pushHistory(taEditKind{op: taEditDeleteStr, str: removed}, end, start.offset)
		return
	}

	first := t.lines[start.row]
	deleted := []string{first[start.offset:]}
	t.lines[start.row] = first[:start.offset]
	// drain start.row+1 .. end.row
	deleted = append(deleted, t.lines[start.row+1:end.row]...)
	t.lines = append(t.lines[:start.row+1:start.row+1], t.lines[end.row:]...)
	if start.row+1 < len(t.lines) {
		lastLine := t.lines[start.row+1]
		t.lines = append(t.lines[:start.row+1:start.row+1], t.lines[start.row+2:]...)
		t.lines[start.row] += lastLine[end.offset:]
		deleted = append(deleted, lastLine[:end.offset])
	}

	if shouldYank {
		y := make([]string, len(deleted))
		copy(y, deleted)
		t.yank = yankText{chunk: true, lines: y}
	}

	var edit taEditKind
	if len(deleted) == 1 {
		edit = taEditKind{op: taEditDeleteStr, str: deleted[0]}
	} else {
		edit = taEditKind{op: taEditDeleteChunk, chunk: deleted}
	}
	t.pushHistory(edit, end, start.offset)
}

func (t *TextArea) deletePiece(col, chars int) bool {
	if chars == 0 {
		return false
	}
	bytesAndChars := func(claimed int, s string) (int, int) {
		lastCol := 0
		c := 0
		for b := range s {
			if c == claimed {
				return b, claimed
			}
			lastCol = c
			c++
		}
		return len(s), lastCol + 1
	}

	row := t.cursor[0]
	line := t.lines[row]
	i, _, ok := taCharAt(line, col)
	if !ok {
		return false
	}
	nbytes, nchars := bytesAndChars(chars, line[i:])
	removed := line[i : i+nbytes]
	t.lines[row] = line[:i] + line[i+nbytes:]

	t.cursor = [2]int{row, col}
	t.pushHistory(taEditKind{op: taEditDeleteStr, str: removed}, taPos{row, col + nchars, i + nbytes}, i)
	t.yank = yankPiece(removed)
	return true
}

// InsertTab inserts spaces (or a hard tab) up to the next tab stop.
func (t *TextArea) InsertTab() bool {
	modified := t.deleteSelection(false)
	if t.tabLen == 0 {
		return modified
	}
	if t.hardTabIndent {
		t.InsertChar('\t')
		return true
	}
	row, col := t.cursor[0], t.cursor[1]
	width := 0
	n := 0
	for _, r := range t.lines[row] {
		if n == col {
			break
		}
		width += RuneWidth(r)
		n++
	}
	l := int(t.tabLen) - width%int(t.tabLen)
	return t.insertPiece(strings.Repeat(" ", l))
}

// InsertNewline splits the line at the cursor.
func (t *TextArea) InsertNewline() {
	t.deleteSelection(false)
	row, col := t.cursor[0], t.cursor[1]
	line := t.lines[row]
	offset := taCharOffset(line, col)
	next := line[offset:]
	t.lines[row] = line[:offset]
	t.lines = taInsertLine(t.lines, row+1, next)
	t.cursor = [2]int{row + 1, 0}
	t.pushHistory(taEditKind{op: taEditInsertNewline}, taPos{row, col, offset}, 0)
}

// DeleteNewline joins the cursor line onto the previous one.
func (t *TextArea) DeleteNewline() bool {
	if t.deleteSelection(false) {
		return true
	}
	row := t.cursor[0]
	if row == 0 {
		return false
	}
	line := t.lines[row]
	t.lines = taRemoveLine(t.lines, row)
	prev := t.lines[row-1]
	prevEnd := len(prev)
	t.cursor = [2]int{row - 1, taCharCount(prev)}
	t.lines[row-1] = prev + line
	t.pushHistory(taEditKind{op: taEditDeleteNewline}, taPos{row, 0, 0}, prevEnd)
	return true
}

// DeleteChar deletes the character before the cursor.
func (t *TextArea) DeleteChar() bool {
	if t.deleteSelection(false) {
		return true
	}
	row, col := t.cursor[0], t.cursor[1]
	if col == 0 {
		return t.DeleteNewline()
	}
	line := t.lines[row]
	offset, c, ok := taCharAt(line, col-1)
	if !ok {
		return false
	}
	n := utf8.RuneLen(c)
	t.lines[row] = line[:offset] + line[offset+n:]
	t.cursor[1]--
	t.pushHistory(taEditKind{op: taEditDeleteChar, ch: c}, taPos{row, col, offset + n}, offset)
	return true
}

// DeleteNextChar deletes the character under the cursor.
func (t *TextArea) DeleteNextChar() bool {
	if t.deleteSelection(false) {
		return true
	}
	before := t.cursor
	t.moveCursorWithShift(CursorForward, false)
	if before == t.cursor {
		return false
	}
	return t.DeleteChar()
}

// DeleteLineByEnd deletes from the cursor to the end of the line.
func (t *TextArea) DeleteLineByEnd() bool {
	if t.deleteSelection(false) {
		return true
	}
	if t.deletePiece(t.cursor[1], int(^uint(0)>>1)) {
		return true
	}
	return t.DeleteNextChar()
}

// DeleteLineByHead deletes from the head of the line to the cursor.
func (t *TextArea) DeleteLineByHead() bool {
	if t.deleteSelection(false) {
		return true
	}
	if t.deletePiece(0, t.cursor[1]) {
		return true
	}
	return t.DeleteNewline()
}

// DeleteWord deletes the word before the cursor.
func (t *TextArea) DeleteWord() bool {
	if t.deleteSelection(false) {
		return true
	}
	r, c := t.cursor[0], t.cursor[1]
	if col, ok := taFindWordStartBackward(t.lines[r], c); ok {
		return t.deletePiece(col, c-col)
	} else if c > 0 {
		return t.deletePiece(0, c)
	}
	return t.DeleteNewline()
}

// DeleteNextWord deletes the word after the cursor.
func (t *TextArea) DeleteNextWord() bool {
	if t.deleteSelection(false) {
		return true
	}
	r, c := t.cursor[0], t.cursor[1]
	line := t.lines[r]
	if col, ok := taFindWordExclusiveEndForward(line, c); ok {
		return t.deletePiece(c, col-c)
	}
	endCol := taCharCount(line)
	if c < endCol {
		return t.deletePiece(c, endCol-c)
	} else if r+1 < len(t.lines) {
		t.cursor = [2]int{r + 1, 0}
		return t.DeleteNewline()
	}
	return false
}

// Paste inserts the yanked text at the cursor.
func (t *TextArea) Paste() bool {
	t.deleteSelection(false)
	y := t.yank
	if y.chunk {
		return t.insertChunk(y.lines)
	}
	return t.insertPiece(y.lines[0])
}

// StartSelection starts selecting text at the cursor.
func (t *TextArea) StartSelection() {
	p := t.cursor
	t.selectionStart = &p
}

// CancelSelection stops selecting text.
func (t *TextArea) CancelSelection() { t.selectionStart = nil }

// IsSelecting reports whether a selection is active.
func (t *TextArea) IsSelecting() bool { return t.selectionStart != nil }

func (t *TextArea) lineOffset(row, col int) int {
	var line string
	if row < len(t.lines) {
		line = t.lines[row]
	} else {
		line = t.lines[len(t.lines)-1]
	}
	return taCharOffset(line, col)
}

func (t *TextArea) selectionPositions() (taPos, taPos, bool) {
	if t.selectionStart == nil {
		return taPos{}, taPos{}, false
	}
	sr, sc := t.selectionStart[0], t.selectionStart[1]
	er, ec := t.cursor[0], t.cursor[1]
	so, eo := t.lineOffset(sr, sc), t.lineOffset(er, ec)
	s := taPos{sr, sc, so}
	e := taPos{er, ec, eo}
	switch {
	case sr < er || (sr == er && so < eo):
		return s, e, true
	case sr == er && so == eo:
		return taPos{}, taPos{}, false
	default:
		return e, s, true
	}
}

func (t *TextArea) takeSelectionPositions() (taPos, taPos, bool) {
	s, e, ok := t.selectionPositions()
	t.CancelSelection()
	return s, e, ok
}

// Copy copies the selected text into the yank buffer.
func (t *TextArea) Copy() {
	start, end, ok := t.takeSelectionPositions()
	if !ok {
		return
	}
	if start.row == end.row {
		t.yank = yankPiece(t.lines[start.row][start.offset:end.offset])
		return
	}
	chunk := []string{t.lines[start.row][start.offset:]}
	chunk = append(chunk, t.lines[start.row+1:end.row]...)
	chunk = append(chunk, t.lines[end.row][:end.offset])
	t.yank = yankText{chunk: true, lines: chunk}
}

// Cut deletes the selected text into the yank buffer.
func (t *TextArea) Cut() bool { return t.deleteSelection(true) }

func (t *TextArea) deleteSelection(shouldYank bool) bool {
	if s, e, ok := t.takeSelectionPositions(); ok {
		t.deleteRange(s, e, shouldYank)
		return true
	}
	return false
}

// MoveCursor moves the cursor, extending the selection if one is active.
func (t *TextArea) MoveCursor(m CursorMove) {
	t.moveCursorWithShift(m, t.selectionStart != nil)
}

func (t *TextArea) moveCursorWithShift(m CursorMove, shift bool) {
	if row, col, ok := m.nextCursor(t.cursor[0], t.cursor[1], t.lines, &t.viewport); ok {
		if shift {
			if t.selectionStart == nil {
				t.StartSelection()
			}
		} else {
			t.CancelSelection()
		}
		t.cursor = [2]int{row, col}
	}
}

// Undo undoes the last edit.
func (t *TextArea) Undo() bool {
	if row, col, ok := t.history.undo(&t.lines); ok {
		t.CancelSelection()
		t.cursor = [2]int{row, col}
		return true
	}
	return false
}

// Redo redoes the last undone edit.
func (t *TextArea) Redo() bool {
	if row, col, ok := t.history.redo(&t.lines); ok {
		t.CancelSelection()
		t.cursor = [2]int{row, col}
		return true
	}
	return false
}

func (t *TextArea) scrollWithShift(s taScrolling, shift bool) {
	if shift && t.selectionStart == nil {
		p := t.cursor
		t.selectionStart = &p
	}
	s.scroll(&t.viewport)
	t.moveCursorWithShift(CursorInViewport, shift)
}

// Scroll scrolls the viewport by the given number of rows and columns,
// keeping the cursor inside it.
func (t *TextArea) Scroll(rows, cols int16) {
	t.scrollWithShift(taScrolling{kind: taScrollDelta, rows: rows, cols: cols}, t.selectionStart != nil)
}

// --- accessors -------------------------------------------------------------

// SetBlock sets the block drawn around the text.
func (t *TextArea) SetBlock(b BlockWidget) { t.block = b }

// RemoveBlock removes the block.
func (t *TextArea) RemoveBlock() { t.block = nil }

// SetStyle sets the base text style.
func (t *TextArea) SetStyle(s Style) { t.style = s }

// SetCursorStyle sets the style of the cursor cell.
func (t *TextArea) SetCursorStyle(s Style) { t.cursorStyle = s }

// SetCursorLineStyle sets the style of the line holding the cursor.
func (t *TextArea) SetCursorLineStyle(s Style) { t.cursorLineStyle = s }

// SetSelectionStyle sets the style of selected text.
func (t *TextArea) SetSelectionStyle(s Style) { t.selectStyle = s }

// SetTabLength sets the tab width.
func (t *TextArea) SetTabLength(n uint8) { t.tabLen = n }

// SetHardTabIndent makes Tab insert a hard tab character.
func (t *TextArea) SetHardTabIndent(on bool) { t.hardTabIndent = on }

// SetMaxHistories resets the undo history with a new capacity.
func (t *TextArea) SetMaxHistories(n int) { t.history = newTaHistory(n) }

// SetMaskChar masks every displayed character with c.
func (t *TextArea) SetMaskChar(c rune) { t.mask, t.hasMask = c, true }

// ClearMaskChar disables masking.
func (t *TextArea) ClearMaskChar() { t.hasMask = false }

// Lines returns the text lines. The slice must not be modified.
func (t *TextArea) Lines() []string { return t.lines }

// Cursor returns the cursor position (row, char column).
func (t *TextArea) Cursor() (row, col int) { return t.cursor[0], t.cursor[1] }

// IsEmpty reports whether the text area holds a single empty line.
func (t *TextArea) IsEmpty() bool { return len(t.lines) == 1 && t.lines[0] == "" }

// YankText returns the yank buffer as a string.
func (t *TextArea) YankText() string { return t.yank.String() }

// SetYankText replaces the yank buffer.
func (t *TextArea) SetYankText(s string) {
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = strings.TrimSuffix(p, "\r")
	}
	t.yank = yankFromLines(parts)
}

func taInsertLine(lines []string, at int, s string) []string {
	lines = append(lines, "")
	copy(lines[at+1:], lines[at:])
	lines[at] = s
	return lines
}

func taRemoveLine(lines []string, at int) []string {
	copy(lines[at:], lines[at+1:])
	return lines[:len(lines)-1]
}

// --- cursor movement -------------------------------------------------------

// CursorMove is a cursor movement.
type CursorMove uint8

const (
	CursorForward CursorMove = iota
	CursorBack
	CursorUp
	CursorDown
	CursorHead
	CursorEnd
	CursorTop
	CursorBottom
	CursorWordForward
	CursorWordEnd
	CursorWordBack
	CursorParagraphForward
	CursorParagraphBack
	CursorInViewport
)

func (m CursorMove) nextCursor(row, col int, lines []string, vp *taViewport) (int, int, bool) {
	fitCol := func(col int, line string) int { return min(col, taCharCount(line)) }

	switch m {
	case CursorForward:
		if col >= taCharCount(lines[row]) {
			if row+1 < len(lines) {
				return row + 1, 0, true
			}
			return 0, 0, false
		}
		return row, col + 1, true
	case CursorBack:
		if col == 0 {
			if row == 0 {
				return 0, 0, false
			}
			return row - 1, taCharCount(lines[row-1]), true
		}
		return row, col - 1, true
	case CursorUp:
		if row == 0 {
			return 0, 0, false
		}
		return row - 1, fitCol(col, lines[row-1]), true
	case CursorDown:
		if row+1 >= len(lines) {
			return 0, 0, false
		}
		return row + 1, fitCol(col, lines[row+1]), true
	case CursorHead:
		return row, 0, true
	case CursorEnd:
		return row, taCharCount(lines[row]), true
	case CursorTop:
		return 0, fitCol(col, lines[0]), true
	case CursorBottom:
		r := len(lines) - 1
		return r, fitCol(col, lines[r]), true
	case CursorWordEnd:
		if c, ok := taFindWordInclusiveEndForward(lines[row], col+1); ok {
			return row, c, true
		}
		r := row
		for {
			if r == len(lines)-1 {
				return r, taCharCount(lines[r]), true
			}
			r++
			if c, ok := taFindWordInclusiveEndForward(lines[r], 0); ok {
				return r, c, true
			}
		}
	case CursorWordForward:
		if c, ok := taFindWordStartForward(lines[row], col); ok {
			return row, c, true
		} else if row+1 < len(lines) {
			return row + 1, 0, true
		}
		return row, taCharCount(lines[row]), true
	case CursorWordBack:
		if c, ok := taFindWordStartBackward(lines[row], col); ok {
			return row, c, true
		} else if row > 0 {
			return row - 1, taCharCount(lines[row-1]), true
		}
		return row, 0, true
	case CursorParagraphForward:
		prevIsEmpty := lines[row] == ""
		for r := row + 1; r < len(lines); r++ {
			isEmpty := lines[r] == ""
			if !isEmpty && prevIsEmpty {
				return r, fitCol(col, lines[r]), true
			}
			prevIsEmpty = isEmpty
		}
		r := len(lines) - 1
		return r, fitCol(col, lines[r]), true
	case CursorParagraphBack:
		if row == 0 {
			return 0, 0, false
		}
		row--
		prevIsEmpty := lines[row] == ""
		for r := row - 1; r >= 0; r-- {
			isEmpty := lines[r] == ""
			if isEmpty && !prevIsEmpty {
				return r + 1, fitCol(col, lines[r+1]), true
			}
			prevIsEmpty = isEmpty
		}
		return 0, fitCol(col, lines[0]), true
	case CursorInViewport:
		rowTop, colTop, rowBottom, colBottom := vp.position()
		r := min(max(row, int(rowTop)), int(rowBottom))
		r = min(r, len(lines)-1)
		c := min(max(col, int(colTop)), int(colBottom))
		c = fitCol(c, lines[r])
		return r, c, true
	}
	return 0, 0, false
}

// --- viewport & scrolling --------------------------------------------------

// taViewport records the scroll position and size of the last render, using
// the same u16 arithmetic as the original.
type taViewport struct {
	row, col, width, height uint16
}

func (v *taViewport) position() (rowTop, colTop, rowBottom, colBottom uint16) {
	rowTop, colTop = v.row, v.col
	rowBottom = taSatSub16(taSatAdd16(rowTop, v.height), 1)
	colBottom = taSatSub16(taSatAdd16(colTop, v.width), 1)
	return rowTop, colTop, max(rowTop, rowBottom), max(colTop, colBottom)
}

func (v *taViewport) scroll(rows, cols int16) {
	apply := func(pos uint16, delta int16) uint16 {
		if delta >= 0 {
			return taSatAdd16(pos, uint16(delta))
		}
		return taSatSub16(pos, uint16(-delta))
	}
	v.row = apply(v.row, rows)
	v.col = apply(v.col, cols)
}

func taSatAdd16(a, b uint16) uint16 {
	if s := uint32(a) + uint32(b); s <= 0xffff {
		return uint16(s)
	}
	return 0xffff
}

func taSatSub16(a, b uint16) uint16 {
	if a < b {
		return 0
	}
	return a - b
}

type taScrollKind uint8

const (
	taScrollDelta taScrollKind = iota
	taScrollPageDownKind
	taScrollPageUpKind
)

type taScrolling struct {
	kind       taScrollKind
	rows, cols int16
}

var (
	taScrollPageDown = taScrolling{kind: taScrollPageDownKind}
	taScrollPageUp   = taScrolling{kind: taScrollPageUpKind}
)

func (s taScrolling) scroll(v *taViewport) {
	rows, cols := s.rows, s.cols
	switch s.kind {
	case taScrollPageDownKind:
		rows, cols = int16(v.height), 0
	case taScrollPageUpKind:
		rows, cols = -int16(v.height), 0
	}
	v.scroll(rows, cols)
}

func taNextScrollTop(prevTop, cursor, length uint16) uint16 {
	switch {
	case cursor < prevTop:
		return cursor
	case prevTop+length <= cursor:
		return cursor + 1 - length
	default:
		return prevTop
	}
}

// --- rendering -------------------------------------------------------------

// Render draws the text area (and its block) into area, updating the
// viewport so that the cursor stays visible.
func (t *TextArea) Render(area Rect, buf *Buffer) {
	inner := area
	if t.block != nil {
		inner = t.block.Inner(area)
	}
	width, height := uint16(inner.Width), uint16(inner.Height)

	topRow := taNextScrollTop(t.viewport.row, uint16(t.cursor[0]), height)
	topCol := taNextScrollTop(t.viewport.col, uint16(t.cursor[1]), width)

	// Build the visible lines.
	top := int(topRow)
	bottom := min(top+int(height), len(t.lines))
	var lines []Line
	for i := top; i < bottom; i++ {
		lines = append(lines, t.lineSpans(t.lines[i], i))
	}

	textArea := area
	if t.block != nil {
		textArea = t.block.Inner(area)
		t.block.Render(area, buf)
	}

	t.viewport = taViewport{row: topRow, col: topCol, width: width, height: height}

	taRenderParagraph(lines, t.style, int(topCol), textArea, buf)
}

// taRenderParagraph renders lines the way ratatui's Paragraph does
// without wrapping, with a horizontal scroll offset and left alignment.
func taRenderParagraph(lines []Line, style Style, xOffset int, area Rect, buf *Buffer) {
	buf.SetStyle(area, style)
	if area.IsEmpty() {
		return
	}
	buf.SetStyle(area, style)
	maxWidth := area.Width
	for y, line := range lines {
		if y >= area.Height {
			break
		}
		offset := xOffset
		width := 0
		x := 0
		for _, g := range line.StyledGraphemes(Style{}) {
			w := GraphemeWidth(g.Symbol)
			if w > maxWidth {
				continue
			}
			if width+w > maxWidth {
				break
			}
			sym := g.Symbol
			if offset != 0 {
				if w > offset {
					sym = taTrimOffset(sym, offset)
					offset = 0
				} else {
					offset -= w
					sym = ""
				}
			}
			sw := GraphemeWidth(sym)
			width += sw
			if sw == 0 {
				continue
			}
			buf.Cell(area.Left()+x, area.Top()+y).SetSymbol(sym).SetStyle(g.Style)
			x += sw
		}
	}
}

// taTrimOffset drops leading graphemes of s whose cumulative width fits in
// offset (ratatui's reflow::trim_offset).
func taTrimOffset(s string, offset int) string {
	start := 0
	for _, g := range Graphemes(s) {
		w := GraphemeWidth(g)
		if w <= offset {
			offset -= w
			start += len(g)
		} else {
			break
		}
	}
	return s[start:]
}

// --- line highlighting -----------------------------------------------------

type taBoundaryKind uint8

const (
	taBoundaryEnd taBoundaryKind = iota
	taBoundarySelect
	taBoundaryCursor
)

type taBoundary struct {
	kind   taBoundaryKind
	style  Style
	offset int
}

func (t *TextArea) lineSpans(line string, row int) Line {
	var (
		boundaries   []taBoundary
		styleBegin   Style
		cursorAtEnd  bool
		selectAtEnd  bool
		spans        []Span
		builderWidth int
	)

	if row == t.cursor[0] {
		if start, c, ok := taCharAt(line, t.cursor[1]); ok {
			boundaries = append(boundaries,
				taBoundary{kind: taBoundaryCursor, style: t.cursorStyle, offset: start},
				taBoundary{kind: taBoundaryEnd, offset: start + utf8.RuneLen(c)})
		} else {
			cursorAtEnd = true
		}
		styleBegin = t.cursorLineStyle
	}

	if s, e, ok := t.selectionPositions(); ok {
		var start, end int
		include := true
		switch {
		case row == s.row:
			if s.row == e.row {
				start, end = s.offset, e.offset
			} else {
				selectAtEnd = true
				start, end = s.offset, len(line)
			}
		case row == e.row:
			start, end = 0, e.offset
		case s.row < row && row < e.row:
			selectAtEnd = true
			start, end = 0, len(line)
		default:
			include = false
		}
		if include && start != end {
			boundaries = append(boundaries,
				taBoundary{kind: taBoundarySelect, style: t.selectStyle, offset: start},
				taBoundary{kind: taBoundaryEnd, offset: end})
		}
	}

	build := func(s string) string {
		if t.hasMask {
			return strings.Repeat(string(t.mask), taCharCount(s))
		}
		// Like the original, the builder only starts copying once a tab
		// has been expanded; until then the input is returned as is.
		var b strings.Builder
		for i, c := range s {
			if c == '\t' {
				if b.Len() == 0 {
					b.WriteString(s[:i])
				}
				if t.tabLen > 0 {
					l := int(t.tabLen) - builderWidth%int(t.tabLen)
					b.WriteString(strings.Repeat(" ", l))
					builderWidth += l
				}
			} else {
				if b.Len() > 0 {
					b.WriteRune(c)
				}
				builderWidth += RuneWidth(c)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
		return s
	}

	if len(boundaries) == 0 {
		if built := build(line); built != "" {
			spans = append(spans, Styled(built, styleBegin))
		}
	} else {
		sort.SliceStable(boundaries, func(i, j int) bool {
			if boundaries[i].offset != boundaries[j].offset {
				return boundaries[i].offset < boundaries[j].offset
			}
			return boundaries[i].kind < boundaries[j].kind
		})
		style := styleBegin
		start := 0
		var stack []Style
		for _, b := range boundaries {
			if start < b.offset {
				spans = append(spans, Styled(build(line[start:b.offset]), style))
			}
			if b.kind != taBoundaryEnd {
				stack = append(stack, style)
				style = b.style
			} else if n := len(stack); n > 0 {
				style = stack[n-1]
				stack = stack[:n-1]
			} else {
				style = styleBegin
			}
			start = b.offset
		}
		if start != len(line) {
			spans = append(spans, Styled(build(line[start:]), style))
		}
	}

	if cursorAtEnd {
		spans = append(spans, Styled(" ", t.cursorStyle))
	} else if selectAtEnd {
		spans = append(spans, Styled(" ", t.selectStyle))
	}
	return Line{Spans: spans}
}

// --- word boundaries -------------------------------------------------------

type taCharKind uint8

const (
	taCharKindSpace taCharKind = iota
	taCharKindPunct
	taCharKindOther
)

func newTaCharKind(c rune) taCharKind {
	switch {
	case taIsWhitespace(c):
		return taCharKindSpace
	case c < 0x80 && strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c):
		return taCharKindPunct
	default:
		return taCharKindOther
	}
}

// taIsWhitespace mirrors char::is_whitespace (the White_Space property).
func taIsWhitespace(c rune) bool {
	switch {
	case c == ' ' || (c >= '\t' && c <= '\r'):
		return true
	case c < 0x80:
		return false
	}
	switch c {
	case 0x85, 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return c >= 0x2000 && c <= 0x200a
}

func taFindWordStartForward(line string, startCol int) (int, bool) {
	rs := []rune(line)
	if startCol >= len(rs) {
		return 0, false
	}
	prev := newTaCharKind(rs[startCol])
	for col := startCol + 1; col < len(rs); col++ {
		cur := newTaCharKind(rs[col])
		if cur != taCharKindSpace && prev != cur {
			return col, true
		}
		prev = cur
	}
	return 0, false
}

func taFindWordExclusiveEndForward(line string, startCol int) (int, bool) {
	rs := []rune(line)
	if startCol >= len(rs) {
		return 0, false
	}
	prev := newTaCharKind(rs[startCol])
	for col := startCol + 1; col < len(rs); col++ {
		cur := newTaCharKind(rs[col])
		if prev != taCharKindSpace && prev != cur {
			return col, true
		}
		prev = cur
	}
	return 0, false
}

func taFindWordInclusiveEndForward(line string, startCol int) (int, bool) {
	rs := []rune(line)
	if startCol >= len(rs) {
		return 0, false
	}
	lastCol := startCol
	prev := newTaCharKind(rs[startCol])
	for col := startCol + 1; col < len(rs); col++ {
		cur := newTaCharKind(rs[col])
		if prev != taCharKindSpace && cur != prev {
			return max(col-1, 0), true
		}
		prev = cur
		lastCol = col
	}
	if prev != taCharKindSpace {
		return lastCol, true
	}
	return 0, false
}

func taFindWordStartBackward(line string, startCol int) (int, bool) {
	rs := []rune(line)
	end := min(startCol, len(rs))
	if end == 0 {
		return 0, false
	}
	cur := newTaCharKind(rs[end-1])
	for i := 1; i < end; i++ {
		next := newTaCharKind(rs[end-1-i])
		if cur != taCharKindSpace && next != cur {
			return startCol - i, true
		}
		cur = next
	}
	if cur != taCharKindSpace {
		return 0, true
	}
	return 0, false
}

// --- history ---------------------------------------------------------------

type taEditOp uint8

const (
	taEditInsertChar taEditOp = iota
	taEditDeleteChar
	taEditInsertNewline
	taEditDeleteNewline
	taEditInsertStr
	taEditDeleteStr
	taEditInsertChunk
	taEditDeleteChunk
)

type taEditKind struct {
	op    taEditOp
	ch    rune
	str   string
	chunk []string
}

func (k taEditKind) apply(lines *[]string, before, after taPos) {
	ls := *lines
	switch k.op {
	case taEditInsertChar:
		l := ls[before.row]
		ls[before.row] = l[:before.offset] + string(k.ch) + l[before.offset:]
	case taEditDeleteChar:
		l := ls[before.row]
		_, n := utf8.DecodeRuneInString(l[after.offset:])
		ls[before.row] = l[:after.offset] + l[after.offset+n:]
	case taEditInsertNewline:
		l := ls[before.row]
		ls[before.row] = l[:before.offset]
		ls = taInsertLine(ls, before.row+1, l[before.offset:])
	case taEditDeleteNewline:
		l := ls[before.row]
		ls = taRemoveLine(ls, before.row)
		ls[before.row-1] += l
	case taEditInsertStr:
		l := ls[before.row]
		ls[before.row] = l[:before.offset] + k.str + l[before.offset:]
	case taEditDeleteStr:
		l := ls[after.row]
		ls[after.row] = l[:after.offset] + l[after.offset+len(k.str):]
	case taEditInsertChunk:
		c := k.chunk
		first := ls[before.row]
		lastLine := first[before.offset:]
		ls[before.row] = first[:before.offset] + c[0]
		next := before.row + 1
		lastLine = c[len(c)-1] + lastLine
		ls = taInsertLine(ls, next, lastLine)
		mid := c[1 : len(c)-1]
		if len(mid) > 0 {
			nl := make([]string, 0, len(ls)+len(mid))
			nl = append(nl, ls[:next]...)
			nl = append(nl, mid...)
			nl = append(nl, ls[next:]...)
			ls = nl
		}
	case taEditDeleteChunk:
		c := k.chunk
		lastLine := ls[after.row+len(c)-1]
		ls = append(ls[:after.row+1], ls[after.row+len(c):]...)
		lastLine = lastLine[len(c[len(c)-1]):]
		ls[after.row] = ls[after.row][:after.offset] + lastLine
	}
	*lines = ls
}

func (k taEditKind) invert() taEditKind {
	switch k.op {
	case taEditInsertChar:
		k.op = taEditDeleteChar
	case taEditDeleteChar:
		k.op = taEditInsertChar
	case taEditInsertNewline:
		k.op = taEditDeleteNewline
	case taEditDeleteNewline:
		k.op = taEditInsertNewline
	case taEditInsertStr:
		k.op = taEditDeleteStr
	case taEditDeleteStr:
		k.op = taEditInsertStr
	case taEditInsertChunk:
		k.op = taEditDeleteChunk
	case taEditDeleteChunk:
		k.op = taEditInsertChunk
	}
	return k
}

type taEdit struct {
	kind          taEditKind
	before, after taPos
}

type taHistory struct {
	index    int
	maxItems int
	edits    []taEdit
}

func newTaHistory(maxItems int) taHistory { return taHistory{maxItems: maxItems} }

func (h *taHistory) push(e taEdit) {
	if h.maxItems == 0 {
		return
	}
	if len(h.edits) == h.maxItems {
		h.edits = h.edits[1:]
		h.index = max(h.index-1, 0)
	}
	if h.index < len(h.edits) {
		h.edits = h.edits[:h.index]
	}
	h.index++
	h.edits = append(h.edits, e)
}

func (h *taHistory) redo(lines *[]string) (int, int, bool) {
	if h.index == len(h.edits) {
		return 0, 0, false
	}
	e := h.edits[h.index]
	e.kind.apply(lines, e.before, e.after)
	h.index++
	return e.after.row, e.after.col, true
}

func (h *taHistory) undo(lines *[]string) (int, int, bool) {
	if h.index == 0 {
		return 0, 0, false
	}
	h.index--
	e := h.edits[h.index]
	e.kind.invert().apply(lines, e.after, e.before)
	return e.before.row, e.before.col, true
}
