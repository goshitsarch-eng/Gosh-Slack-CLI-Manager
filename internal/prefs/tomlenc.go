package prefs

import (
	"fmt"
	"strconv"
	"strings"
)

// encodeSettings renders settings exactly like toml 0.8's to_string_pretty
// for the original serde struct.
func encodeSettings(s AppSettings) string {
	var b strings.Builder
	b.WriteString("theme = " + encodeTomlString(s.Theme.Name()) + "\n")
	b.WriteString("confirm_actions = " + strconv.FormatBool(s.ConfirmActions) + "\n")
	b.WriteString("show_hidden_files = " + strconv.FormatBool(s.ShowHiddenFiles) + "\n")
	b.WriteString("auto_refresh = " + strconv.FormatBool(s.AutoRefresh) + "\n")
	b.WriteString("refresh_interval = " + strconv.FormatUint(uint64(s.RefreshInterval), 10) + "\n")
	b.WriteString("default_tab = " + encodeTomlString(s.DefaultTab) + "\n")
	b.WriteString("log_lines = " + strconv.Itoa(s.LogLines) + "\n")
	return b.String()
}

type tomlEncoding uint8

const (
	encNone tomlEncoding = iota
	encLiteral
	encBasic
	encMlLiteral
	encMlBasic
)

type valueMetrics struct {
	maxSeqSingle, maxSeqDouble int
	escapeCodes, escape        bool
	newline                    bool
}

func calcValueMetrics(s string) valueMetrics {
	var m valueMetrics
	single, double := 0, 0
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == '\'' {
			single++
			m.maxSeqSingle = max(m.maxSeqSingle, single)
		} else {
			single = 0
		}
		if b == '"' {
			double++
			m.maxSeqDouble = max(m.maxSeqDouble, double)
		} else {
			double = 0
		}
		switch {
		case b == '\\':
			m.escape = true
		case b == '\t':
		case b == '\n':
			m.newline = true
		case b <= 0x1f || b == 0x7f:
			m.escapeCodes = true
		}
	}
	return m
}

// encodeTomlString picks the string representation the same way
// toml_write's TomlStringBuilder::as_default does.
func encodeTomlString(s string) string {
	m := calcValueMetrics(s)
	switch {
	case !m.escapeCodes && !m.escape && m.maxSeqDouble == 0 && !m.newline:
		return writeTomlString(s, encBasic, m.newline)
	case !m.escapeCodes && m.maxSeqSingle == 0 && !m.newline:
		return writeTomlString(s, encLiteral, m.newline)
	case !m.escapeCodes && !m.escape && m.maxSeqDouble <= 2:
		return writeTomlString(s, encMlBasic, m.newline)
	case !m.escapeCodes && m.maxSeqSingle <= 2:
		return writeTomlString(s, encMlLiteral, m.newline)
	case m.newline:
		return writeTomlString(s, encMlBasic, m.newline)
	default:
		return writeTomlString(s, encBasic, m.newline)
	}
}

// encodeTomlKey picks a key representation like TomlKeyBuilder::as_default.
func encodeTomlKey(s string) string {
	unquoted := s != ""
	var single, double, escapeCodes, escape bool
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_') {
			unquoted = false
		}
		switch {
		case b == '\'':
			single = true
		case b == '"':
			double = true
		case b == '\\':
			escape = true
		case b == '\t':
		case b <= 0x1f || b == 0x7f:
			escapeCodes = true
		}
	}
	switch {
	case unquoted:
		return s
	case !escapeCodes && !escape && !double:
		return writeTomlString(s, encBasic, false)
	case !escapeCodes && !single:
		return writeTomlString(s, encLiteral, false)
	default:
		return writeTomlString(s, encBasic, false)
	}
}

func writeTomlString(decoded string, enc tomlEncoding, newline bool) string {
	var delimiter string
	escaped, isML := false, false
	switch enc {
	case encLiteral:
		delimiter = "'"
	case encBasic:
		delimiter, escaped = `"`, true
	case encMlLiteral:
		delimiter, isML = "'''", true
	case encMlBasic:
		delimiter, escaped, isML = `"""`, true, true
	}

	var w strings.Builder
	w.WriteString(delimiter)
	if newline && isML {
		w.WriteString("\n")
	}
	if !escaped {
		w.WriteString(decoded)
		w.WriteString(delimiter)
		return w.String()
	}

	maxSeqDouble := 0
	if isML {
		maxSeqDouble = 2
	}
	stream := decoded
	for stream != "" {
		unescapedEnd := 0
		esc := ""
		hasEsc := false
		seqDouble := 0
	scan:
		for i := 0; i < len(stream); i++ {
			b := stream[i]
			if b == '"' {
				seqDouble++
				if maxSeqDouble < seqDouble {
					esc, hasEsc = `\"`, true
					break scan
				}
			} else {
				seqDouble = 0
			}
			switch {
			case b == 0x8:
				esc, hasEsc = `\b`, true
				break scan
			case b == 0x9:
				esc, hasEsc = `\t`, true
				break scan
			case b == 0xa:
				if !isML {
					esc, hasEsc = `\n`, true
					break scan
				}
			case b == 0xc:
				esc, hasEsc = `\f`, true
				break scan
			case b == 0xd:
				esc, hasEsc = `\r`, true
				break scan
			case b == '"':
			case b == '\\':
				esc, hasEsc = `\\`, true
				break scan
			case b <= 0x1f || b == 0x7f:
				break scan
			}
			unescapedEnd = i + 1
		}
		w.WriteString(stream[:unescapedEnd])
		w.WriteString(esc)
		end := unescapedEnd
		if hasEsc {
			end++
		}
		stream = stream[end:]
		if !hasEsc && stream != "" {
			fmt.Fprintf(&w, "\\u%04X", stream[0])
			stream = stream[1:]
		}
	}
	w.WriteString(delimiter)
	return w.String()
}
