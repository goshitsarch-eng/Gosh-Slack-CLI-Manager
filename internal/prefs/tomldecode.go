package prefs

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	posInf = math.Inf(1)
	nan    = math.NaN()
)

// tomlError mirrors toml_edit's TomlError and its Display output.
type tomlError struct {
	message string
	raw     string
	hasRaw  bool
	span    span
	hasSpan bool
}

func (e *tomlError) Error() string {
	var b strings.Builder
	if e.hasRaw && e.hasSpan {
		raw := []byte(e.raw)
		line, column := translatePosition(raw, e.span.start)
		lineNum, colNum := line+1, column+1
		gutter := len(strconv.Itoa(lineNum))
		content := strings.Split(e.raw, "\n")[line]
		highlight := e.span.end - e.span.start
		highlight = min(highlight, max(len(content)-column, 0))

		fmt.Fprintf(&b, "TOML parse error at line %d, column %d\n", lineNum, colNum)
		b.WriteString(strings.Repeat(" ", gutter+1) + "|\n")
		fmt.Fprintf(&b, "%d | %s\n", lineNum, content)
		b.WriteString(strings.Repeat(" ", gutter+1) + "|")
		b.WriteString(strings.Repeat(" ", column+1) + "^")
		if highlight > 1 {
			b.WriteString(strings.Repeat("^", highlight-1))
		}
		b.WriteString("\n")
	}
	b.WriteString(e.message + "\n")
	return b.String()
}

func translatePosition(input []byte, index int) (int, int) {
	if len(input) == 0 {
		return 0, index
	}
	safe := min(index, len(input)-1)
	columnOffset := index - safe
	index = safe

	lineStart := 0
	for i := index - 1; i >= 0; i-- {
		if input[i] == '\n' {
			lineStart = i + 1
			break
		}
	}
	line := strings.Count(string(input[:lineStart]), "\n")
	segment := input[lineStart : index+1]
	var column int
	if utf8.Valid(segment) {
		column = utf8.RuneCount(segment) - 1
	} else {
		column = index - lineStart
	}
	return line, column + columnOffset
}

// --- serde-style deserialization of AppSettings ------------------------------

type deError struct {
	message string
	span    span
	hasSpan bool
}

func deErr(msg string) *deError { return &deError{message: msg} }

func (e *deError) orSpan(sp span, ok bool) *deError {
	if e != nil && !e.hasSpan && ok {
		e.span, e.hasSpan = sp, true
	}
	return e
}

// rustDebugString formats s like Rust's `{:?}` for str.
func rustDebugString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\'':
			b.WriteByte('\'')
		default:
			if r == 0 || unicode.In(r, unicode.Mn, unicode.Me) || !unicode.IsGraphic(r) {
				fmt.Fprintf(&b, `\u{%x}`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func rustEscapeDebugChar(c rune) string {
	s := rustDebugString(string(c))
	return s[1 : len(s)-1]
}

// rustFloat formats like serde's WithDecimalPoint(f64).
func rustFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// unexpected describes an item the way serde's Unexpected does when a
// visitor rejects it.
func unexpected(it *tItem) string {
	if it.kind != itemValue {
		if it.kind == itemTable {
			return "map"
		}
		return "sequence"
	}
	v := it.val
	switch v.kind {
	case valString:
		return "string " + rustDebugString(v.str)
	case valInteger:
		return "integer `" + strconv.FormatInt(v.i, 10) + "`"
	case valFloat:
		return "floating point `" + rustFloat(v.f) + "`"
	case valBool:
		return "boolean `" + strconv.FormatBool(v.b) + "`"
	case valArray:
		return "sequence"
	default: // datetime, inline table
		return "map"
	}
}

func invalidType(it *tItem, expected string) *deError {
	return deErr("invalid type: " + unexpected(it) + ", expected " + expected)
}

func deBool(it *tItem) (bool, *deError) {
	if it.kind == itemValue && it.val.kind == valBool {
		return it.val.b, nil
	}
	return false, invalidType(it, "a boolean")
}

func deString(it *tItem) (string, *deError) {
	if it.kind == itemValue && it.val.kind == valString {
		return it.val.str, nil
	}
	return "", invalidType(it, "a string")
}

func deUnsigned(it *tItem, maxValue uint64, name string) (uint64, *deError) {
	if it.kind == itemValue && it.val.kind == valInteger {
		v := it.val.i
		if v >= 0 && uint64(v) <= maxValue {
			return uint64(v), nil
		}
		return 0, deErr("invalid value: integer `" + strconv.FormatInt(v, 10) + "`, expected " + name)
	}
	return 0, invalidType(it, name)
}

func themeByName(name string) (ThemeChoice, *deError) {
	for i, n := range themeNames {
		if n == name {
			return ThemeChoice(i), nil
		}
	}
	quoted := make([]string, len(themeNames))
	for i, n := range themeNames {
		quoted[i] = "`" + n + "`"
	}
	return 0, deErr("unknown variant `" + name + "`, expected one of " + strings.Join(quoted, ", "))
}

// deTheme is toml_edit's deserialize_enum for the unit-only ThemeChoice.
func deTheme(it *tItem) (ThemeChoice, *deError) {
	itSpan, itHasSpan := it.span()
	var table *tTable
	switch {
	case it.kind == itemValue && it.val.kind == valString:
		t, err := themeByName(it.val.str)
		return t, err.orSpan(itSpan, itHasSpan)
	case it.kind == itemValue && it.val.kind == valInlineTable:
		table = it.val.tbl
	case it.kind == itemTable:
		table = it.tbl
	default:
		return 0, deErr("wanted string or table").orSpan(itSpan, itHasSpan)
	}

	switch len(table.entries) {
	case 0:
		return 0, deErr("wanted exactly 1 element, found 0 elements").orSpan(table.span, table.hasSpan).orSpan(itSpan, itHasSpan)
	case 1:
	default:
		return 0, deErr("wanted exactly 1 element, more than 1 element").orSpan(table.span, table.hasSpan).orSpan(itSpan, itHasSpan)
	}

	entry := table.entries[0]
	theme, err := themeByName(entry.key.name)
	if err != nil {
		return 0, err.orSpan(entry.key.span, true).orSpan(itSpan, itHasSpan)
	}
	if err := unitVariant(entry.item); err != nil {
		return 0, err.orSpan(itSpan, itHasSpan)
	}
	return theme, nil
}

func unitVariant(v *tItem) *deError {
	sp, ok := v.span()
	switch {
	case v.kind == itemArrayOfTables:
		if len(v.aot.tables) == 0 {
			return nil
		}
		return deErr("expected empty array").orSpan(sp, ok)
	case v.kind == itemValue && v.val.kind == valArray:
		if len(v.val.arr) == 0 {
			return nil
		}
		return deErr("expected empty table").orSpan(sp, ok)
	case v.kind == itemTable:
		if len(v.tbl.entries) == 0 {
			return nil
		}
		return deErr("expected empty table").orSpan(sp, ok)
	case v.kind == itemValue && v.val.kind == valInlineTable:
		if len(v.val.tbl.entries) == 0 {
			return nil
		}
		return deErr("expected empty table").orSpan(sp, ok)
	default:
		return deErr("expected table, found "+v.typeName()).orSpan(sp, ok)
	}
}

var settingsFields = []string{
	"theme", "confirm_actions", "show_hidden_files", "auto_refresh",
	"refresh_interval", "default_tab", "log_lines",
}

// decodeSettings is toml::from_str::<AppSettings>: every field is required,
// unknown keys are ignored.
func decodeSettings(raw string) (AppSettings, error) {
	root, perr := parseDocument(raw)
	if perr != nil {
		return AppSettings{}, perr
	}

	var s AppSettings
	seen := map[string]bool{}
	for _, e := range root.entries {
		var err *deError
		switch e.key.name {
		case "theme":
			s.Theme, err = deTheme(e.item)
		case "confirm_actions":
			s.ConfirmActions, err = deBool(e.item)
		case "show_hidden_files":
			s.ShowHiddenFiles, err = deBool(e.item)
		case "auto_refresh":
			s.AutoRefresh, err = deBool(e.item)
		case "refresh_interval":
			var v uint64
			v, err = deUnsigned(e.item, math.MaxUint32, "u32")
			s.RefreshInterval = uint32(v)
		case "default_tab":
			s.DefaultTab, err = deString(e.item)
		case "log_lines":
			var v uint64
			v, err = deUnsigned(e.item, math.MaxInt64, "usize")
			s.LogLines = int(v)
		default:
			continue
		}
		if err != nil {
			sp, ok := e.item.span()
			err = err.orSpan(sp, ok).orSpan(e.key.span, true)
			return AppSettings{}, &tomlError{message: err.message, raw: raw, hasRaw: true, span: err.span, hasSpan: err.hasSpan}
		}
		seen[e.key.name] = true
	}
	for _, field := range settingsFields {
		if !seen[field] {
			return AppSettings{}, &tomlError{
				message: "missing field `" + field + "`",
				raw:     raw, hasRaw: true,
				span: root.span, hasSpan: root.hasSpan,
			}
		}
	}
	return s, nil
}
