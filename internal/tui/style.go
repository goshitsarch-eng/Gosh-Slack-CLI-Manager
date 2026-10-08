// Package tui is a small cell-buffer rendering toolkit. It mirrors the
// semantics of the ratatui widgets the original application was built on so
// that every screen renders cell-for-cell the same.
package tui

// ColorKind distinguishes the different color encodings.
type ColorKind uint8

const (
	ColorKindReset ColorKind = iota
	ColorKindIndexed
	ColorKindRGB
)

// Color is a terminal color: the terminal default (Reset), an ANSI palette
// index, or a 24-bit RGB value.
type Color struct {
	Kind    ColorKind
	Index   uint8
	R, G, B uint8
}

// Reset is the terminal's default color.
var Reset = Color{Kind: ColorKindReset}

// Named ANSI colors, using the same palette indexes crossterm emits.
var (
	Black        = Indexed(0)
	Red          = Indexed(1)
	Green        = Indexed(2)
	Yellow       = Indexed(3)
	Blue         = Indexed(4)
	Magenta      = Indexed(5)
	Cyan         = Indexed(6)
	Gray         = Indexed(7)
	DarkGray     = Indexed(8)
	LightRed     = Indexed(9)
	LightGreen   = Indexed(10)
	LightYellow  = Indexed(11)
	LightBlue    = Indexed(12)
	LightMagenta = Indexed(13)
	LightCyan    = Indexed(14)
	White        = Indexed(15)
)

// Indexed returns an ANSI palette color.
func Indexed(i uint8) Color { return Color{Kind: ColorKindIndexed, Index: i} }

// RGB returns a 24-bit color.
func RGB(r, g, b uint8) Color { return Color{Kind: ColorKindRGB, R: r, G: g, B: b} }

// Modifier is a bitset of text attributes.
type Modifier uint16

const (
	Bold Modifier = 1 << iota
	Dim
	Italic
	Underlined
	SlowBlink
	RapidBlink
	Reversed
	Hidden
	CrossedOut
)

// Style holds optional foreground/background colors plus modifiers to add
// and remove. Unset colors leave whatever is underneath untouched.
type Style struct {
	Fg, Bg       Color
	HasFg, HasBg bool
	AddModifier  Modifier
	SubModifier  Modifier
}

// NewStyle returns an empty style.
func NewStyle() Style { return Style{} }

// FG sets the foreground color.
func (s Style) FG(c Color) Style {
	s.Fg, s.HasFg = c, true
	return s
}

// BG sets the background color.
func (s Style) BG(c Color) Style {
	s.Bg, s.HasBg = c, true
	return s
}

// Add adds modifiers.
func (s Style) Add(m Modifier) Style {
	s.SubModifier &^= m
	s.AddModifier |= m
	return s
}

// Remove removes modifiers.
func (s Style) Remove(m Modifier) Style {
	s.AddModifier &^= m
	s.SubModifier |= m
	return s
}

// Patch layers other on top of s.
func (s Style) Patch(other Style) Style {
	if other.HasFg {
		s.Fg, s.HasFg = other.Fg, true
	}
	if other.HasBg {
		s.Bg, s.HasBg = other.Bg, true
	}
	s.AddModifier &^= other.SubModifier
	s.AddModifier |= other.AddModifier
	s.SubModifier &^= other.AddModifier
	s.SubModifier |= other.SubModifier
	return s
}
