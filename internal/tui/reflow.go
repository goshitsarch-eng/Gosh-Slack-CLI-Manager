package tui

import "unicode"

const (
	nbsp = " "
	zwsp = "​"
)

// isWhitespaceGrapheme mirrors StyledGrapheme::is_whitespace: a zero width
// space, or a grapheme made only of whitespace that is not a non-breaking
// space. Go's unicode.IsSpace matches Rust's char::is_whitespace (the
// Unicode White_Space property).
func isWhitespaceGrapheme(symbol string) bool {
	if symbol == zwsp {
		return true
	}
	for _, r := range symbol {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return symbol != nbsp
}

// composerLine is one input line for a line composer.
type composerLine struct {
	graphemes []StyledGrapheme
	alignment Alignment
}

// wrappedLine is one output line of a line composer.
type wrappedLine struct {
	line      []StyledGrapheme
	width     int
	alignment Alignment
}

type lineComposer interface {
	nextLine() (wrappedLine, bool)
}

// wordWrapper wraps lines on word boundaries (ratatui reflow::WordWrapper).
type wordWrapper struct {
	input            []composerLine
	pos              int
	maxLineWidth     int
	wrappedLines     [][]StyledGrapheme
	currentAlignment Alignment
	trim             bool
}

func newWordWrapper(lines []composerLine, maxLineWidth int, trim bool) *wordWrapper {
	return &wordWrapper{input: lines, maxLineWidth: maxLineWidth, trim: trim}
}

func (w *wordWrapper) processInput(lineSymbols []StyledGrapheme) {
	var pendingLine, pendingWord, pendingWhitespace []StyledGrapheme
	lineWidth, wordWidth, whitespaceWidth := 0, 0, 0
	nonWhitespacePrevious := false
	maxW := w.maxLineWidth

	for _, g := range lineSymbols {
		isWS := isWhitespaceGrapheme(g.Symbol)
		symbolWidth := GraphemeWidth(g.Symbol)
		// ignore symbols wider than line limit
		if symbolWidth > maxW {
			continue
		}

		wordFound := nonWhitespacePrevious && isWS
		trimmedOverflow := len(pendingLine) == 0 && w.trim && wordWidth+symbolWidth > maxW
		whitespaceOverflow := len(pendingLine) == 0 && w.trim && whitespaceWidth+symbolWidth > maxW
		untrimmedOverflow := len(pendingLine) == 0 && !w.trim &&
			wordWidth+whitespaceWidth+symbolWidth > maxW

		// append finished segment to current line
		if wordFound || trimmedOverflow || whitespaceOverflow || untrimmedOverflow {
			if len(pendingLine) > 0 || !w.trim {
				pendingLine = append(pendingLine, pendingWhitespace...)
				lineWidth += whitespaceWidth
			}
			pendingLine = append(pendingLine, pendingWord...)
			pendingWord = pendingWord[:0]
			lineWidth += wordWidth

			pendingWhitespace = pendingWhitespace[:0]
			whitespaceWidth = 0
			wordWidth = 0
		}

		lineFull := lineWidth >= maxW
		pendingWordOverflow := symbolWidth > 0 && lineWidth+whitespaceWidth+wordWidth >= maxW

		// add finished wrapped line to remaining lines
		if lineFull || pendingWordOverflow {
			remainingWidth := satSub(maxW, lineWidth)
			w.wrappedLines = append(w.wrappedLines, pendingLine)
			pendingLine = nil
			lineWidth = 0

			// remove whitespace up to the end of line
			for len(pendingWhitespace) > 0 {
				width := GraphemeWidth(pendingWhitespace[0].Symbol)
				if width > remainingWidth {
					break
				}
				whitespaceWidth -= width
				remainingWidth -= width
				pendingWhitespace = pendingWhitespace[1:]
			}

			// don't count first whitespace toward next word
			if isWS && len(pendingWhitespace) == 0 {
				continue
			}
		}

		if isWS {
			whitespaceWidth += symbolWidth
			pendingWhitespace = append(pendingWhitespace, g)
		} else {
			wordWidth += symbolWidth
			pendingWord = append(pendingWord, g)
		}
		nonWhitespacePrevious = !isWS
	}

	// append remaining text parts
	if len(pendingLine) == 0 && len(pendingWord) == 0 && len(pendingWhitespace) > 0 {
		w.wrappedLines = append(w.wrappedLines, nil)
	}
	if len(pendingLine) > 0 || !w.trim {
		pendingLine = append(pendingLine, pendingWhitespace...)
	}
	pendingLine = append(pendingLine, pendingWord...)

	if len(pendingLine) > 0 {
		w.wrappedLines = append(w.wrappedLines, pendingLine)
	}
	if len(w.wrappedLines) == 0 {
		w.wrappedLines = append(w.wrappedLines, nil)
	}
}

func (w *wordWrapper) nextLine() (wrappedLine, bool) {
	if w.maxLineWidth == 0 {
		return wrappedLine{}, false
	}
	for {
		if len(w.wrappedLines) > 0 {
			line := w.wrappedLines[0]
			w.wrappedLines = w.wrappedLines[1:]
			width := 0
			for _, g := range line {
				width += GraphemeWidth(g.Symbol)
			}
			return wrappedLine{line: line, width: width, alignment: w.currentAlignment}, true
		}
		if w.pos >= len(w.input) {
			return wrappedLine{}, false
		}
		in := w.input[w.pos]
		w.pos++
		w.currentAlignment = in.alignment
		w.processInput(in.graphemes)
	}
}

// lineTruncator cuts lines at the maximum width (ratatui
// reflow::LineTruncator).
type lineTruncator struct {
	input            []composerLine
	pos              int
	maxLineWidth     int
	horizontalOffset int
}

func newLineTruncator(lines []composerLine, maxLineWidth int) *lineTruncator {
	return &lineTruncator{input: lines, maxLineWidth: maxLineWidth}
}

func (t *lineTruncator) nextLine() (wrappedLine, bool) {
	if t.maxLineWidth == 0 {
		return wrappedLine{}, false
	}
	if t.pos >= len(t.input) {
		return wrappedLine{}, false
	}
	in := t.input[t.pos]
	t.pos++

	var current []StyledGrapheme
	currentWidth := 0
	horizontalOffset := t.horizontalOffset
	for _, g := range in.graphemes {
		symbol := g.Symbol
		sw := GraphemeWidth(symbol)
		// Ignore characters wider than the total max width.
		if sw > t.maxLineWidth {
			continue
		}
		if currentWidth+sw > t.maxLineWidth {
			// Truncate line
			break
		}
		if horizontalOffset != 0 && in.alignment == AlignLeft {
			if sw > horizontalOffset {
				symbol = trimOffset(symbol, horizontalOffset)
				horizontalOffset = 0
			} else {
				horizontalOffset -= sw
				symbol = ""
			}
		}
		currentWidth += GraphemeWidth(symbol)
		current = append(current, StyledGrapheme{Symbol: symbol, Style: g.Style})
	}
	return wrappedLine{line: current, width: currentWidth, alignment: in.alignment}, true
}

// trimOffset drops leading graphemes of src whose total width fits within
// offset.
func trimOffset(src string, offset int) string {
	start := 0
	for _, g := range Graphemes(src) {
		w := GraphemeWidth(g)
		if w <= offset {
			offset -= w
			start += len(g)
		} else {
			break
		}
	}
	return src[start:]
}
