package prefs

// A TOML 1.0 parser that mirrors toml_edit 0.22 (the parser behind the
// original tool's settings loader) closely enough to accept and reject the
// same documents and to report errors at the same positions with the same
// wording. The structure follows toml_edit's winnow grammar, including where
// the input position is left when a parser fails, because that position is
// what the error message points at.

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// --- errors ------------------------------------------------------------------

type ctxKind uint8

const (
	ctxLabel ctxKind = iota
	ctxChar
	ctxString
	ctxDesc
)

type strCtx struct {
	kind ctxKind
	s    string
	c    rune
}

func label(s string) strCtx     { return strCtx{kind: ctxLabel, s: s} }
func expChar(c rune) strCtx     { return strCtx{kind: ctxChar, c: c} }
func expStr(s string) strCtx    { return strCtx{kind: ctxString, s: s} }
func expDesc(s string) strCtx   { return strCtx{kind: ctxDesc, s: s} }
func (c strCtx) String() string { return c.display() }

func (c strCtx) display() string {
	switch c.kind {
	case ctxChar:
		switch {
		case c.c == '\n':
			return "newline"
		case c.c == '`':
			return "'`'"
		case c.c < 0x20 || c.c == 0x7f:
			return "`" + rustEscapeDebugChar(c.c) + "`"
		default:
			return "`" + string(c.c) + "`"
		}
	case ctxString:
		return "`" + c.s + "`"
	default:
		return c.s
	}
}

// perr is a parse failure: a backtrack error unless cut is set.
type perr struct {
	cut      bool
	ctx      []strCtx
	cause    string
	hasCause bool
}

func bt() *perr                 { return &perr{} }
func extErr(cause string) *perr { return &perr{cause: cause, hasCause: true} }

func cutErr(e *perr) *perr {
	if e != nil {
		e.cut = true
	}
	return e
}

func withCtx(e *perr, cs ...strCtx) *perr {
	if e != nil {
		e.ctx = append(e.ctx, cs...)
	}
	return e
}

func (e *perr) message() string {
	var b strings.Builder
	newline := false
	for _, c := range e.ctx {
		if c.kind == ctxLabel {
			newline = true
			b.WriteString("invalid " + c.s)
			break
		}
	}
	first := true
	for _, c := range e.ctx {
		if c.kind == ctxLabel {
			continue
		}
		if first {
			if newline {
				b.WriteString("\n")
			}
			newline = true
			b.WriteString("expected ")
			first = false
		} else {
			b.WriteString(", ")
		}
		b.WriteString(c.display())
	}
	if e.hasCause {
		if newline {
			b.WriteString("\n")
		}
		b.WriteString(e.cause)
	}
	return b.String()
}

const (
	causeOutOfRange = "value is out of range"
	causeRecursion  = "recursion limit exceeded"
	recursionLimit  = 80
)

// --- document model --------------------------------------------------------

type span struct{ start, end int }

type valKind uint8

const (
	valString valKind = iota
	valInteger
	valFloat
	valBool
	valDatetime
	valArray
	valInlineTable
)

var valTypeNames = [...]string{"string", "integer", "float", "boolean", "datetime", "array", "inline table"}

type tValue struct {
	kind    valKind
	str     string
	i       int64
	f       float64
	b       bool
	arr     []*tValue
	tbl     *tTable
	span    span
	hasSpan bool
}

type itemKind uint8

const (
	itemValue itemKind = iota
	itemTable
	itemArrayOfTables
)

type tItem struct {
	kind itemKind
	val  *tValue
	tbl  *tTable
	aot  *tArrayOfTables
}

func (it *tItem) span() (span, bool) {
	switch it.kind {
	case itemValue:
		if it.val.kind == valInlineTable {
			return it.val.tbl.span, it.val.tbl.hasSpan
		}
		return it.val.span, it.val.hasSpan
	case itemTable:
		return it.tbl.span, it.tbl.hasSpan
	default:
		return it.aot.span, it.aot.hasSpan
	}
}

func (it *tItem) typeName() string {
	switch it.kind {
	case itemValue:
		return valTypeNames[it.val.kind]
	case itemTable:
		return "table"
	default:
		return "array of tables"
	}
}

type tKey struct {
	name string
	span span
}

type tEntry struct {
	key  tKey
	item *tItem
}

// tTable is an insertion-ordered table (both [table]s and inline tables).
type tTable struct {
	entries  []tEntry
	span     span
	hasSpan  bool
	implicit bool
	dotted   bool
}

type tArrayOfTables struct {
	tables  []*tTable
	span    span
	hasSpan bool
}

func (t *tTable) find(name string) int {
	for i, e := range t.entries {
		if e.key.name == name {
			return i
		}
	}
	return -1
}

func (t *tTable) insert(key tKey, item *tItem) { t.entries = append(t.entries, tEntry{key, item}) }

func (t *tTable) shiftRemove(name string) *tItem {
	i := t.find(name)
	if i < 0 {
		return nil
	}
	item := t.entries[i].item
	t.entries = append(t.entries[:i], t.entries[i+1:]...)
	return item
}

// --- parser primitives -------------------------------------------------------

type parser struct {
	in    []byte
	pos   int
	depth int
	st    *parseState
}

func isWSChar(b byte) bool { return b == ' ' || b == '\t' }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }
func isHexDig(b byte) bool { return isDigit(b) || b >= 'A' && b <= 'F' || b >= 'a' && b <= 'f' }
func isNonEOL(b byte) bool { return b == 0x09 || b >= 0x20 && b <= 0x7e || b >= 0x80 }
func isUnquoted(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || isDigit(b) || b == '-' || b == '_'
}
func isBasicUnescaped(b byte) bool {
	return isWSChar(b) || b == 0x21 || b >= 0x23 && b <= 0x5b || b >= 0x5d && b <= 0x7e || b >= 0x80
}
func isLiteralChar(b byte) bool {
	return b == 0x09 || b >= 0x20 && b <= 0x26 || b >= 0x28 && b <= 0x7e || b >= 0x80
}

func (p *parser) peek() (byte, bool) {
	if p.pos >= len(p.in) {
		return 0, false
	}
	return p.in[p.pos], true
}

// any consumes one byte.
func (p *parser) any() (byte, *perr) {
	if p.pos >= len(p.in) {
		return 0, bt()
	}
	b := p.in[p.pos]
	p.pos++
	return b, nil
}

// oneOf consumes one byte matching f.
func (p *parser) oneOf(f func(byte) bool) (byte, *perr) {
	if p.pos < len(p.in) && f(p.in[p.pos]) {
		p.pos++
		return p.in[p.pos-1], nil
	}
	return 0, bt()
}

func (p *parser) byteLit(c byte) *perr {
	_, e := p.oneOf(func(b byte) bool { return b == c })
	return e
}

func (p *parser) lit(s string) *perr {
	if strings.HasPrefix(string(p.in[p.pos:]), s) {
		p.pos += len(s)
		return nil
	}
	return bt()
}

// takeWhile consumes between lo and hi (hi<0: unbounded) bytes matching f.
func (p *parser) takeWhile(lo, hi int, f func(byte) bool) (string, *perr) {
	n := 0
	for p.pos+n < len(p.in) && (hi < 0 || n < hi) && f(p.in[p.pos+n]) {
		n++
	}
	if n < lo {
		return "", bt()
	}
	s := string(p.in[p.pos : p.pos+n])
	p.pos += n
	return s, nil
}

func (p *parser) ws() { _, _ = p.takeWhile(0, -1, isWSChar) }

// --- trivia ------------------------------------------------------------------

func (p *parser) comment() *perr {
	if e := p.byteLit('#'); e != nil {
		return e
	}
	_, _ = p.takeWhile(0, -1, isNonEOL)
	return nil
}

func (p *parser) newline() *perr {
	b, e := p.any()
	if e != nil {
		return e
	}
	switch b {
	case '\n':
		return nil
	case '\r':
		return p.byteLit('\n')
	default:
		return bt()
	}
}

func (p *parser) eof() *perr {
	if p.pos < len(p.in) {
		return bt()
	}
	return nil
}

// wsNewline is repeat(0.., alt((newline, take_while(1.., WSCHAR)))).
func (p *parser) wsNewline() *perr {
	for {
		start := p.pos
		e := p.newline()
		if e != nil && !e.cut {
			p.pos = start
			_, e = p.takeWhile(1, -1, isWSChar)
		}
		if e != nil {
			if e.cut {
				return e
			}
			p.pos = start
			return nil
		}
	}
}

func (p *parser) wsNewlines() *perr {
	if e := p.newline(); e != nil {
		return e
	}
	return p.wsNewline()
}

func (p *parser) wsCommentNewline() *perr {
	start := p.pos
	for {
		p.ws()
		b, ok := p.peek()
		if !ok {
			break
		}
		if b == '#' {
			if e := p.comment(); e != nil {
				return e
			}
			if e := p.newline(); e != nil {
				return e
			}
		} else if b == '\n' || b == '\r' {
			if e := p.newline(); e != nil {
				return e
			}
		} else {
			break
		}
		if p.pos == start {
			break
		}
		start = p.pos
	}
	return nil
}

func (p *parser) lineEnding() *perr {
	start := p.pos
	e := p.newline()
	if e == nil || e.cut {
		return e
	}
	p.pos = start
	return p.eof()
}

func (p *parser) lineTrailing() *perr {
	p.ws()
	start := p.pos
	if e := p.comment(); e != nil {
		p.pos = start
	}
	return p.lineEnding()
}

// --- keys --------------------------------------------------------------------

func (p *parser) simpleKey() (tKey, *perr) {
	start := p.pos
	b, ok := p.peek()
	if !ok {
		return tKey{}, bt()
	}
	var name string
	var e *perr
	switch b {
	case '"':
		name, e = p.basicString()
	case '\'':
		name, e = p.literalString()
	default:
		name, e = p.takeWhile(1, -1, isUnquoted)
	}
	if e != nil {
		return tKey{}, e
	}
	return tKey{name: name, span: span{start, p.pos}}, nil
}

func (p *parser) keyPart() (tKey, *perr) {
	p.ws()
	k, e := p.simpleKey()
	if e != nil {
		return k, e
	}
	p.ws()
	return k, nil
}

func (p *parser) key() ([]tKey, *perr) {
	start := p.pos
	first, e := p.keyPart()
	if e != nil {
		return nil, withCtx(e, label("key"))
	}
	keys := []tKey{first}
	for {
		s := p.pos
		if p.byteLit('.') != nil {
			p.pos = s
			break
		}
		k, e := p.keyPart()
		if e != nil {
			if e.cut {
				return nil, withCtx(e, label("key"))
			}
			p.pos = s
			break
		}
		keys = append(keys, k)
	}
	if recursionLimit <= len(keys) {
		p.pos = start
		return nil, extErr(causeRecursion)
	}
	return keys, nil
}

// --- strings -----------------------------------------------------------------

func (p *parser) str() (string, *perr) {
	start := p.pos
	parsers := []func() (string, *perr){p.mlBasicString, p.basicString, p.mlLiteralString, p.literalString}
	var err *perr
	for _, f := range parsers {
		p.pos = start
		s, e := f()
		if e == nil || e.cut {
			return s, e
		}
		err = e
	}
	return "", err
}

func (p *parser) basicString() (string, *perr) {
	if e := p.byteLit('"'); e != nil {
		return "", e
	}
	var b strings.Builder
	for {
		s := p.pos
		chunk, e := p.basicChars()
		if e != nil {
			if e.cut {
				return "", e
			}
			p.pos = s
			break
		}
		b.WriteString(chunk)
	}
	if e := p.byteLit('"'); e != nil {
		return "", withCtx(cutErr(e), label("basic string"))
	}
	return b.String(), nil
}

func (p *parser) basicChars() (string, *perr) {
	start := p.pos
	if s, e := p.takeWhile(1, -1, isBasicUnescaped); e == nil {
		return s, nil
	}
	p.pos = start
	r, e := p.escaped()
	if e != nil {
		return "", e
	}
	return string(r), nil
}

func (p *parser) escaped() (rune, *perr) {
	if e := p.byteLit('\\'); e != nil {
		return 0, e
	}
	return p.escapeSeqChar()
}

func (p *parser) escapeSeqChar() (rune, *perr) {
	b, e := p.any()
	if e != nil {
		return 0, e
	}
	switch b {
	case 'b':
		return '\b', nil
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'u':
		r, e := p.hexEscape(4)
		return r, withCtx(cutErr(e), label("unicode 4-digit hex code"))
	case 'U':
		r, e := p.hexEscape(8)
		return r, withCtx(cutErr(e), label("unicode 8-digit hex code"))
	case '\\':
		return '\\', nil
	case '"':
		return '"', nil
	default:
		return 0, withCtx(cutErr(bt()), label("escape sequence"),
			expChar('b'), expChar('f'), expChar('n'), expChar('r'), expChar('t'),
			expChar('u'), expChar('U'), expChar('\\'), expChar('"'))
	}
}

func (p *parser) hexEscape(n int) (rune, *perr) {
	start := p.pos
	s, _ := p.takeWhile(0, n, isHexDig)
	if len(s) != n {
		p.pos = start
		return 0, bt()
	}
	v, _ := strconv.ParseUint(s, 16, 32)
	if v > utf8.MaxRune || v >= 0xd800 && v <= 0xdfff {
		p.pos = start
		return 0, extErr(causeOutOfRange)
	}
	return rune(v), nil
}

func (p *parser) mlBasicString() (string, *perr) {
	if e := p.lit(`"""`); e != nil {
		return "", e
	}
	s := p.pos
	if p.newline() != nil {
		p.pos = s
	}
	body, e := p.mlBasicBody()
	if e != nil {
		return "", withCtx(cutErr(e), label("multiline basic string"))
	}
	if e := p.lit(`"""`); e != nil {
		return "", withCtx(cutErr(e), label("multiline basic string"))
	}
	return body, nil
}

// optRun runs f; a backtrack resets the position and reports ok=false.
func optRun[T any](p *parser, f func() (T, *perr)) (T, bool, *perr) {
	s := p.pos
	v, e := f()
	if e != nil {
		if e.cut {
			return v, false, e
		}
		p.pos = s
		return v, false, nil
	}
	return v, true, nil
}

func (p *parser) mlBasicBody() (string, *perr) {
	var b strings.Builder
	for {
		c, ok, e := optRun(p, p.mlbContent)
		if e != nil {
			return "", e
		}
		if !ok {
			break
		}
		b.WriteString(c)
	}
	notQuote := func() *perr {
		_, e := p.oneOf(func(b byte) bool { return b != '"' })
		return e
	}
	for {
		q, ok, e := optRun(p, func() (string, *perr) { return p.quotes('"', notQuote) })
		if e != nil {
			return "", e
		}
		if !ok {
			break
		}
		c, ok, e := optRun(p, p.mlbContent)
		if e != nil {
			return "", e
		}
		if !ok {
			break
		}
		b.WriteString(q)
		b.WriteString(c)
		for {
			c, ok, e := optRun(p, p.mlbContent)
			if e != nil {
				return "", e
			}
			if !ok {
				break
			}
			b.WriteString(c)
		}
	}
	delim := func() *perr { return p.lit(`"""`) }
	q, ok, e := optRun(p, func() (string, *perr) { return p.quotes('"', delim) })
	if e != nil {
		return "", e
	}
	if ok {
		b.WriteString(q)
	}
	return b.String(), nil
}

// quotes is mlb-quotes / mll-quotes: two (else one) quote characters that
// must be followed by term (peeked, not consumed).
func (p *parser) quotes(q byte, term func() *perr) (string, *perr) {
	start := p.pos
	two := string([]byte{q, q})
	e := p.lit(two)
	if e == nil {
		s := p.pos
		e = term()
		p.pos = s
		if e == nil {
			return two, nil
		}
	}
	if e.cut {
		return "", e
	}
	p.pos = start
	if e := p.byteLit(q); e != nil {
		return "", e
	}
	s := p.pos
	e = term()
	p.pos = s
	if e != nil {
		return "", e
	}
	return string(q), nil
}

func (p *parser) mlbContent() (string, *perr) {
	start := p.pos
	if s, e := p.takeWhile(1, -1, isBasicUnescaped); e == nil {
		return s, nil
	}
	p.pos = start
	e := p.mlbEscapedNL()
	if e == nil {
		return "", nil
	}
	if e.cut {
		return "", e
	}
	p.pos = start
	r, e := p.escaped()
	if e == nil {
		return string(r), nil
	}
	if e.cut {
		return "", e
	}
	p.pos = start
	if e := p.newline(); e != nil {
		return "", e
	}
	return "\n", nil
}

func (p *parser) mlbEscapedNL() *perr {
	one := func() *perr {
		if e := p.byteLit('\\'); e != nil {
			return e
		}
		p.ws()
		return p.wsNewlines()
	}
	if e := one(); e != nil {
		return e
	}
	for {
		s := p.pos
		if e := one(); e != nil {
			if e.cut {
				return e
			}
			p.pos = s
			return nil
		}
	}
}

func (p *parser) literalString() (string, *perr) {
	if e := p.byteLit('\''); e != nil {
		return "", withCtx(e, label("literal string"))
	}
	s, _ := p.takeWhile(0, -1, isLiteralChar)
	if e := p.byteLit('\''); e != nil {
		return "", withCtx(cutErr(e), label("literal string"))
	}
	return s, nil
}

func (p *parser) mlLiteralString() (string, *perr) {
	if e := p.lit("'''"); e != nil {
		return "", e
	}
	s := p.pos
	if p.newline() != nil {
		p.pos = s
	}
	body, e := p.mlLiteralBody()
	if e != nil {
		return "", withCtx(cutErr(e), label("multiline literal string"))
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if e := p.lit("'''"); e != nil {
		return "", withCtx(cutErr(e), label("multiline literal string"))
	}
	return body, nil
}

func (p *parser) mllContent() (struct{}, *perr) {
	start := p.pos
	if _, e := p.oneOf(isLiteralChar); e == nil {
		return struct{}{}, nil
	}
	p.pos = start
	return struct{}{}, p.newline()
}

func (p *parser) mlLiteralBody() (string, *perr) {
	start := p.pos
	repeatContent := func() *perr {
		for {
			_, ok, e := optRun(p, p.mllContent)
			if e != nil {
				return e
			}
			if !ok {
				return nil
			}
		}
	}
	if e := repeatContent(); e != nil {
		return "", e
	}
	notApos := func() *perr {
		_, e := p.oneOf(func(b byte) bool { return b != '\'' })
		return e
	}
	for {
		s := p.pos
		if _, e := p.quotes('\'', notApos); e != nil {
			if e.cut {
				return "", e
			}
			p.pos = s
			break
		}
		if _, e := p.mllContent(); e != nil {
			if e.cut {
				return "", e
			}
			p.pos = s
			break
		}
		if e := repeatContent(); e != nil {
			return "", e
		}
	}
	delim := func() *perr { return p.lit("'''") }
	if _, _, e := optRun(p, func() (string, *perr) { return p.quotes('\'', delim) }); e != nil {
		return "", e
	}
	return string(p.in[start:p.pos]), nil
}

// --- numbers -----------------------------------------------------------------

// digitRun parses first-digit then *( DIGIT / "_" DIGIT ) where the digit
// after an underscore is cut with ctx.
func (p *parser) digitTail(isD func(byte) bool, afterUnderscore ...strCtx) *perr {
	for {
		s := p.pos
		if _, e := p.oneOf(isD); e == nil {
			continue
		}
		p.pos = s
		if p.byteLit('_') != nil {
			p.pos = s
			return nil
		}
		if _, e := p.oneOf(isD); e != nil {
			return withCtx(cutErr(e), afterUnderscore...)
		}
	}
}

func (p *parser) decInt() (string, *perr) {
	start := p.pos
	s := p.pos
	if _, e := p.oneOf(func(b byte) bool { return b == '+' || b == '-' }); e != nil {
		p.pos = s
	}
	s = p.pos
	if _, e := p.oneOf(func(b byte) bool { return b >= '1' && b <= '9' }); e == nil {
		if e := p.digitTail(isDigit, expDesc("digit")); e != nil {
			return "", withCtx(e, label("integer"))
		}
	} else {
		p.pos = s
		if _, e := p.oneOf(isDigit); e != nil {
			return "", withCtx(e, label("integer"))
		}
	}
	return string(p.in[start:p.pos]), nil
}

func (p *parser) prefixedInt(prefix string, isD func(byte) bool, name string) (string, *perr) {
	if e := p.lit(prefix); e != nil {
		return "", withCtx(e, label(name))
	}
	s := p.pos
	if _, e := p.oneOf(isD); e != nil {
		return "", withCtx(cutErr(e), label(name))
	}
	if e := p.digitTail(isD, expDesc("digit")); e != nil {
		return "", withCtx(cutErr(e), label(name))
	}
	return string(p.in[s:p.pos]), nil
}

func intRangeCause(negative bool) string {
	if negative {
		return "number too small to fit in target type"
	}
	return "number too large to fit in target type"
}

func (p *parser) integer() (int64, *perr) {
	start := p.pos
	var two string
	if p.pos+2 <= len(p.in) {
		two = string(p.in[p.pos : p.pos+2])
	}
	radix := 0
	switch two {
	case "0x":
		radix = 16
	case "0o":
		radix = 8
	case "0b":
		radix = 2
	}
	if radix != 0 {
		var s string
		var e *perr
		switch radix {
		case 16:
			s, e = p.prefixedInt("0x", isHexDig, "hexadecimal integer")
		case 8:
			s, e = p.prefixedInt("0o", func(b byte) bool { return b >= '0' && b <= '7' }, "octal integer")
		default:
			s, e = p.prefixedInt("0b", func(b byte) bool { return b == '0' || b == '1' }, "binary integer")
		}
		if e != nil {
			return 0, cutErr(e)
		}
		v, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), radix, 64)
		if err != nil {
			p.pos = start
			return 0, cutErr(extErr(intRangeCause(false)))
		}
		return v, nil
	}
	s, e := p.decInt()
	if e != nil {
		return 0, e
	}
	clean := strings.ReplaceAll(s, "_", "")
	v, err := strconv.ParseInt(clean, 10, 64)
	if err != nil {
		p.pos = start
		return 0, cutErr(extErr(intRangeCause(strings.HasPrefix(clean, "-"))))
	}
	return v, nil
}

func (p *parser) zeroPrefixableInt() *perr {
	if _, e := p.oneOf(isDigit); e != nil {
		return e
	}
	return p.digitTail(isDigit, expDesc("digit"))
}

func (p *parser) frac() *perr {
	if e := p.byteLit('.'); e != nil {
		return e
	}
	return withCtx(cutErr(p.zeroPrefixableInt()), expDesc("digit"))
}

func (p *parser) exp() *perr {
	if _, e := p.oneOf(func(b byte) bool { return b == 'e' || b == 'E' }); e != nil {
		return e
	}
	s := p.pos
	if _, e := p.oneOf(func(b byte) bool { return b == '+' || b == '-' }); e != nil {
		p.pos = s
	}
	return cutErr(p.zeroPrefixableInt())
}

func (p *parser) floatRaw() (string, *perr) {
	start := p.pos
	if _, e := p.decInt(); e != nil {
		return "", e
	}
	s := p.pos
	e := p.exp()
	if e == nil {
		return string(p.in[start:p.pos]), nil
	}
	if e.cut {
		return "", e
	}
	p.pos = s
	if e := p.frac(); e != nil {
		return "", e
	}
	s = p.pos
	if e := p.exp(); e != nil {
		if e.cut {
			return "", e
		}
		p.pos = s
	}
	return string(p.in[start:p.pos]), nil
}

func (p *parser) float() (float64, *perr) {
	f, e := p.float_()
	return f, withCtx(e, label("floating-point number"))
}

func (p *parser) float_() (float64, *perr) {
	start := p.pos
	s, e := p.floatRaw()
	if e == nil {
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, "_", ""), 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			p.pos = start
			return 0, cutErr(extErr(err.Error()))
		}
		if math.IsInf(f, 1) {
			p.pos = start
			return 0, cutErr(bt())
		}
		return f, nil
	}
	if e.cut {
		return 0, e
	}
	p.pos = start
	return p.specialFloat()
}

func (p *parser) specialFloat() (float64, *perr) {
	s := p.pos
	sign, e := p.oneOf(func(b byte) bool { return b == '+' || b == '-' })
	if e != nil {
		p.pos = s
		sign = '+'
	}
	afterSign := p.pos
	var f float64
	if e := p.lit("inf"); e == nil {
		f = posInf
	} else {
		p.pos = afterSign
		if e := p.lit("nan"); e != nil {
			return 0, e
		}
		f = nan
	}
	if sign == '-' {
		f = -f
	}
	return f, nil
}

// --- date-time -----------------------------------------------------------------

func (p *parser) digits2(lo, hi int) (int, *perr) {
	start := p.pos
	s, e := p.takeWhile(2, 2, isDigit)
	if e != nil {
		return 0, e
	}
	v, _ := strconv.Atoi(s)
	if v < lo || v > hi {
		p.pos = start
		return 0, extErr(causeOutOfRange)
	}
	return v, nil
}

func (p *parser) fullDate() *perr {
	ys, e := p.takeWhile(4, 4, isDigit)
	if e != nil {
		return e
	}
	if e := p.byteLit('-'); e != nil {
		return e
	}
	month, e := p.digits2(1, 12)
	if e != nil {
		return cutErr(e)
	}
	if e := p.byteLit('-'); e != nil {
		return cutErr(e)
	}
	dayStart := p.pos
	day, e := p.digits2(1, 31)
	if e != nil {
		return cutErr(e)
	}
	year, _ := strconv.Atoi(ys)
	leap := year%4 == 0 && (year%100 != 0 || year%400 == 0)
	maxDays := 31
	switch month {
	case 2:
		maxDays = 28
		if leap {
			maxDays = 29
		}
	case 4, 6, 9, 11:
		maxDays = 30
	}
	if maxDays < day {
		p.pos = dayStart
		return cutErr(extErr(causeOutOfRange))
	}
	return nil
}

func (p *parser) partialTime() *perr {
	if _, e := p.digits2(0, 23); e != nil {
		return e
	}
	if e := p.byteLit(':'); e != nil {
		return e
	}
	if _, e := p.digits2(0, 59); e != nil {
		return cutErr(e)
	}
	if e := p.byteLit(':'); e != nil {
		return cutErr(e)
	}
	if _, e := p.digits2(0, 60); e != nil {
		return cutErr(e)
	}
	s := p.pos
	if p.byteLit('.') != nil {
		p.pos = s
	} else if _, e := p.takeWhile(1, -1, isDigit); e != nil {
		p.pos = s
	}
	return nil
}

func (p *parser) timeOffset() *perr {
	start := p.pos
	if _, e := p.oneOf(func(b byte) bool { return b == 'Z' || b == 'z' }); e == nil {
		return nil
	}
	p.pos = start
	if _, e := p.oneOf(func(b byte) bool { return b == '+' || b == '-' }); e != nil {
		return withCtx(e, label("time offset"))
	}
	if _, e := p.digits2(0, 23); e != nil {
		return withCtx(cutErr(e), label("time offset"))
	}
	if e := p.byteLit(':'); e != nil {
		return withCtx(cutErr(e), label("time offset"))
	}
	if _, e := p.digits2(0, 59); e != nil {
		return withCtx(cutErr(e), label("time offset"))
	}
	return nil
}

func (p *parser) dateTime() *perr {
	start := p.pos
	e := func() *perr {
		if e := p.fullDate(); e != nil {
			return e
		}
		s := p.pos
		if _, e := p.oneOf(func(b byte) bool { return b == 'T' || b == 't' || b == ' ' }); e != nil {
			p.pos = s
			return nil
		}
		if e := p.partialTime(); e != nil {
			if e.cut {
				return e
			}
			p.pos = s
			return nil
		}
		s2 := p.pos
		if e := p.timeOffset(); e != nil {
			if e.cut {
				return e
			}
			p.pos = s2
		}
		return nil
	}()
	if e == nil {
		return nil
	}
	e = withCtx(e, label("date-time"))
	if e.cut {
		return e
	}
	p.pos = start
	return withCtx(p.partialTime(), label("time"))
}

// --- values --------------------------------------------------------------------

func valueTypoCtx(e *perr) *perr {
	return withCtx(e, label("string"), expChar('"'), expChar('\''))
}

func (p *parser) value() (*tValue, *perr) {
	start := p.pos
	b, ok := p.peek()
	if !ok {
		return nil, bt()
	}
	v := &tValue{}
	var e *perr
	switch {
	case b == '"' || b == '\'':
		v.kind = valString
		v.str, e = p.str()
	case b == '[':
		v.kind = valArray
		e = p.checkRecursion(func() *perr {
			var e *perr
			v.arr, e = p.array()
			return e
		})
	case b == '{':
		v.kind = valInlineTable
		e = p.checkRecursion(func() *perr {
			var e *perr
			v.tbl, e = p.inlineTable()
			return e
		})
	case b == '+' || b == '-' || isDigit(b):
		e = p.dateTime()
		if e == nil {
			v.kind = valDatetime
			break
		}
		if e.cut {
			break
		}
		p.pos = start
		v.f, e = p.float()
		if e == nil {
			v.kind = valFloat
			break
		}
		if e.cut {
			break
		}
		p.pos = start
		v.kind = valInteger
		v.i, e = p.integer()
	case b == '_':
		v.kind = valInteger
		v.i, e = p.integer()
		e = withCtx(e, expDesc("leading digit"))
	case b == '.':
		v.kind = valFloat
		v.f, e = p.float()
		e = withCtx(e, expDesc("leading digit"))
	case b == 't' || b == 'f':
		v.kind = valBool
		word := "true"
		if b == 'f' {
			word = "false"
		}
		v.b = b == 't'
		e = valueTypoCtx(cutErr(p.lit(word)))
	case b == 'i':
		v.kind = valFloat
		v.f = posInf
		e = valueTypoCtx(p.lit("inf"))
	case b == 'n':
		v.kind = valFloat
		v.f = nan
		e = valueTypoCtx(p.lit("nan"))
	default:
		e = valueTypoCtx(bt())
	}
	if e != nil {
		return nil, e
	}
	v.span, v.hasSpan = span{start, p.pos}, true
	if v.kind == valInlineTable {
		v.tbl.span, v.tbl.hasSpan = v.span, true
	}
	return v, nil
}

func (p *parser) checkRecursion(f func() *perr) *perr {
	p.depth++
	if recursionLimit <= p.depth {
		return cutErr(extErr(causeRecursion))
	}
	e := f()
	p.depth--
	return e
}

func (p *parser) array() ([]*tValue, *perr) {
	if e := p.byteLit('['); e != nil {
		return nil, e
	}
	vals, e := p.arrayValues()
	if e != nil {
		return nil, cutErr(e)
	}
	if e := p.byteLit(']'); e != nil {
		return nil, withCtx(cutErr(e), label("array"), expChar(']'))
	}
	return vals, nil
}

func (p *parser) arrayValues() ([]*tValue, *perr) {
	s := p.pos
	if p.byteLit(']') == nil {
		p.pos = s
		return nil, nil
	}
	p.pos = s
	var vals []*tValue
	first, ok, e := optRun(p, p.arrayValue)
	if e != nil {
		return nil, e
	}
	if ok {
		vals = append(vals, first)
		for {
			s := p.pos
			if p.byteLit(',') != nil {
				p.pos = s
				break
			}
			v, e := p.arrayValue()
			if e != nil {
				if e.cut {
					return nil, e
				}
				p.pos = s
				break
			}
			vals = append(vals, v)
		}
		s := p.pos
		if p.byteLit(',') != nil {
			p.pos = s
		}
	}
	if e := p.wsCommentNewline(); e != nil {
		return nil, e
	}
	return vals, nil
}

func (p *parser) arrayValue() (*tValue, *perr) {
	if e := p.wsCommentNewline(); e != nil {
		return nil, e
	}
	v, e := p.value()
	if e != nil {
		return nil, e
	}
	if e := p.wsCommentNewline(); e != nil {
		return nil, e
	}
	return v, nil
}

type keyval struct {
	path []tKey
	key  tKey
	val  *tValue
}

func (p *parser) inlineKeyval() (keyval, *perr) {
	path, e := p.key()
	if e != nil {
		return keyval{}, e
	}
	if e := p.byteLit('='); e != nil {
		return keyval{}, withCtx(cutErr(e), expChar('.'), expChar('='))
	}
	p.ws()
	v, e := p.value()
	if e != nil {
		return keyval{}, cutErr(e)
	}
	p.ws()
	return keyval{path: path[:len(path)-1], key: path[len(path)-1], val: v}, nil
}

func (p *parser) inlineTable() (*tTable, *perr) {
	if e := p.byteLit('{'); e != nil {
		return nil, e
	}
	start := p.pos
	var kvs []keyval
	first, ok, e := optRun(p, p.inlineKeyval)
	if e != nil {
		return nil, cutErr(e)
	}
	if ok {
		kvs = append(kvs, first)
		for {
			s := p.pos
			if p.byteLit(',') != nil {
				p.pos = s
				break
			}
			kv, e := p.inlineKeyval()
			if e != nil {
				if e.cut {
					return nil, e
				}
				p.pos = s
				break
			}
			kvs = append(kvs, kv)
		}
	}
	p.ws()
	tbl, msg := tableFromPairs(kvs)
	if msg != "" {
		p.pos = start
		return nil, cutErr(extErr(msg))
	}
	if e := p.byteLit('}'); e != nil {
		return nil, withCtx(cutErr(e), label("inline table"), expChar('}'))
	}
	return tbl, nil
}

func dupKeyMsg(key string) string { return "duplicate key `" + key + "`" }

func joinKeyNames(keys []tKey) string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.name
	}
	return strings.Join(names, ".")
}

func extendWrongTypeMsg(path []tKey, i int, actual string) string {
	return "dotted key `" + joinKeyNames(path[:i+1]) + "` attempted to extend non-table type (" + actual + ")"
}

func tableFromPairs(kvs []keyval) (*tTable, string) {
	root := &tTable{}
	for _, kv := range kvs {
		table, msg := descendInline(root, kv.path)
		if msg != "" {
			return nil, msg
		}
		if table.dotted == (len(kv.path) == 0) {
			return nil, dupKeyMsg(kv.key.name)
		}
		if i := table.find(kv.key.name); i >= 0 {
			return nil, dupKeyMsg(table.entries[i].key.name)
		}
		table.insert(kv.key, &tItem{kind: itemValue, val: kv.val})
	}
	return root, ""
}

func descendInline(table *tTable, path []tKey) (*tTable, string) {
	for i, key := range path {
		idx := table.find(key.name)
		if idx < 0 {
			child := &tTable{implicit: true, dotted: true}
			table.insert(key, &tItem{kind: itemValue, val: &tValue{kind: valInlineTable, tbl: child}})
			table = child
			continue
		}
		item := table.entries[idx].item
		if item.val.kind != valInlineTable {
			return nil, extendWrongTypeMsg(path, i, item.typeName())
		}
		child := item.val.tbl
		if !child.implicit {
			return nil, dupKeyMsg(key.name)
		}
		table = child
	}
	return table, ""
}

// --- document ------------------------------------------------------------------

func (p *parser) keyvalLine() *perr {
	start := p.pos
	path, e := p.key()
	if e != nil {
		return e
	}
	if e := p.byteLit('='); e != nil {
		return withCtx(cutErr(e), expChar('.'), expChar('='))
	}
	p.ws()
	v, e := p.value()
	if e != nil {
		return cutErr(e)
	}
	if e := p.lineTrailing(); e != nil {
		return withCtx(cutErr(e), expChar('\n'), expChar('#'))
	}
	if msg := p.st.onKeyval(path[:len(path)-1], path[len(path)-1], v); msg != "" {
		p.pos = start
		return extErr(msg)
	}
	return nil
}

func (p *parser) tableHeader(open, close string, array bool) *perr {
	start := p.pos
	if e := p.lit(open); e != nil {
		return e
	}
	path, e := p.key()
	if e != nil {
		return cutErr(e)
	}
	if e := p.lit(close); e != nil {
		return withCtx(cutErr(e), expChar('.'), expStr(close))
	}
	sp := span{start, p.pos}
	if e := p.lineTrailing(); e != nil {
		return withCtx(cutErr(e), expChar('\n'), expChar('#'))
	}
	var msg string
	if array {
		msg = p.st.onArrayHeader(path, sp)
	} else {
		msg = p.st.onStdHeader(path, sp)
	}
	if msg != "" {
		p.pos = start
		return extErr(msg)
	}
	return nil
}

func (p *parser) table() *perr {
	var e *perr
	switch {
	case p.pos+2 > len(p.in):
		e = bt()
	case string(p.in[p.pos:p.pos+2]) == "[[":
		e = p.tableHeader("[[", "]]", true)
	default:
		e = p.tableHeader("[", "]", false)
	}
	return withCtx(e, label("table header"))
}

func (p *parser) document() *perr {
	if strings.HasPrefix(string(p.in), "\xef\xbb\xbf") {
		p.pos = 3
	}
	p.ws()
	for {
		s := p.pos
		b, ok := p.peek()
		if !ok {
			break
		}
		var e *perr
		switch b {
		case '#':
			e = p.comment()
			if e == nil {
				e = p.lineEnding()
			}
			e = cutErr(e)
		case '[':
			e = cutErr(p.table())
		case '\n', '\r':
			e = p.newline()
		default:
			e = cutErr(p.keyvalLine())
		}
		if e != nil {
			if e.cut {
				return e
			}
			p.pos = s
			break
		}
		p.ws()
	}
	return p.eof()
}

// --- parse state -------------------------------------------------------------

type parseState struct {
	root           *tTable
	currentTable   *tTable
	currentIsArray bool
	currentPath    []tKey
}

func newParseState() *parseState {
	return &parseState{
		root:         &tTable{},
		currentTable: &tTable{hasSpan: true},
	}
}

func (s *parseState) onKeyval(path []tKey, key tKey, v *tValue) string {
	if s.currentTable.hasSpan {
		s.currentTable.span.end = v.span.end
	}
	table, msg := descendPath(s.currentTable, path, true)
	if msg != "" {
		return msg
	}
	if table.dotted == (len(path) == 0) {
		return dupKeyMsg(key.name)
	}
	if i := table.find(key.name); i >= 0 {
		return dupKeyInTableMsg(table.entries[i].key.name, s.currentPath)
	}
	table.insert(key, &tItem{kind: itemValue, val: v})
	return ""
}

func dupKeyInTableMsg(key string, table []tKey) string {
	if len(table) == 0 {
		return "duplicate key `" + key + "` in document root"
	}
	return "duplicate key `" + key + "` in table `" + joinKeyNames(table) + "`"
}

// duplicateKeyAt is CustomError::duplicate_key: the key in its default
// representation, in the table named by the preceding path.
func duplicateKeyAt(path []tKey, i int) string {
	return dupKeyInTableMsg(encodeTomlKey(path[i].name), path[:i])
}

func descendPath(table *tTable, path []tKey, dotted bool) (*tTable, string) {
	for i, key := range path {
		idx := table.find(key.name)
		if idx < 0 {
			child := &tTable{implicit: true, dotted: dotted}
			table.insert(key, &tItem{kind: itemTable, tbl: child})
			table = child
			continue
		}
		item := table.entries[idx].item
		switch item.kind {
		case itemValue:
			return nil, extendWrongTypeMsg(path, i, item.typeName())
		case itemArrayOfTables:
			table = item.aot.tables[len(item.aot.tables)-1]
		default:
			if dotted && !item.tbl.implicit {
				return nil, dupKeyMsg(key.name)
			}
			table = item.tbl
		}
	}
	return table, ""
}

func (s *parseState) startTable(path []tKey, sp span, array bool) string {
	n := len(path)
	parent, msg := descendPath(s.root, path[:n-1], false)
	if msg != "" {
		return msg
	}
	key := path[n-1]
	if array {
		idx := parent.find(key.name)
		if idx < 0 {
			parent.insert(key, &tItem{kind: itemArrayOfTables, aot: &tArrayOfTables{}})
		} else if parent.entries[idx].item.kind != itemArrayOfTables {
			return duplicateKeyAt(path, n-1)
		}
	} else if item := parent.shiftRemove(key.name); item != nil {
		if item.kind == itemTable && item.tbl.implicit && !item.tbl.dotted {
			s.currentTable = item.tbl
		} else {
			return duplicateKeyAt(path, n-1)
		}
	}
	s.currentTable.implicit = false
	s.currentTable.dotted = false
	s.currentTable.span, s.currentTable.hasSpan = sp, true
	s.currentIsArray = array
	s.currentPath = path
	return ""
}

func (s *parseState) finalizeTable() string {
	table := s.currentTable
	s.currentTable = &tTable{}
	path := s.currentPath
	s.currentPath = nil
	if len(path) == 0 {
		s.root = table
		return ""
	}
	n := len(path)
	parent, msg := descendPath(s.root, path[:n-1], false)
	if msg != "" {
		return msg
	}
	key := path[n-1]
	idx := parent.find(key.name)
	if s.currentIsArray {
		if idx < 0 {
			parent.insert(key, &tItem{kind: itemArrayOfTables, aot: &tArrayOfTables{}})
			idx = len(parent.entries) - 1
		}
		item := parent.entries[idx].item
		if item.kind != itemArrayOfTables {
			return duplicateKeyAt(path, n-1)
		}
		aot := item.aot
		aot.tables = append(aot.tables, table)
		first, last := aot.tables[0], aot.tables[len(aot.tables)-1]
		aot.hasSpan = first.hasSpan && last.hasSpan
		aot.span = span{first.span.start, last.span.end}
		return ""
	}
	if idx < 0 {
		parent.insert(key, &tItem{kind: itemTable, tbl: table})
		return ""
	}
	item := parent.entries[idx].item
	if item.kind == itemTable && item.tbl.implicit {
		item.tbl = table
		return ""
	}
	return duplicateKeyAt(path, n-1)
}

func (s *parseState) onStdHeader(path []tKey, sp span) string {
	if msg := s.finalizeTable(); msg != "" {
		return msg
	}
	return s.startTable(path, sp, false)
}

func (s *parseState) onArrayHeader(path []tKey, sp span) string {
	if msg := s.finalizeTable(); msg != "" {
		return msg
	}
	return s.startTable(path, sp, true)
}

// parseDocument parses raw into its root table, or returns the error the
// original parser would display.
func parseDocument(raw string) (*tTable, *tomlError) {
	p := &parser{in: []byte(raw), st: newParseState()}
	if e := p.document(); e != nil {
		start, end := charBoundary(p.in, p.pos)
		return nil, &tomlError{message: e.message(), raw: raw, hasRaw: true, span: span{start, end}, hasSpan: true}
	}
	if msg := p.st.finalizeTable(); msg != "" {
		return nil, &tomlError{message: msg}
	}
	return p.st.root, nil
}

func charBoundary(in []byte, offset int) (int, int) {
	n := len(in)
	if offset == n {
		return offset, offset
	}
	isBoundary := func(b byte) bool { return int8(b) >= -0x40 }
	start := 0
	for i := min(offset+1, n) - 1; i >= 0; i-- {
		if isBoundary(in[i]) {
			start = i
			break
		}
	}
	end := n
	for i := offset + 1; i < n; i++ {
		if isBoundary(in[i]) {
			end = i
			break
		}
	}
	return start, end
}
