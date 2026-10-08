package tui

// KeyCode identifies a key, mirroring crossterm's KeyCode.
type KeyCode uint8

const (
	KeyNull KeyCode = iota
	KeyChar
	KeyEnter
	KeyTab
	KeyBackTab
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyEsc
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyF
)

// KeyModifiers is a bitset of modifier keys.
type KeyModifiers uint8

const (
	ModNone    KeyModifiers = 0
	ModShift   KeyModifiers = 1 << 0
	ModControl KeyModifiers = 1 << 1
	ModAlt     KeyModifiers = 1 << 2
)

// KeyEvent is a key press. Rune is set for KeyChar; F holds the function key
// number for KeyF.
type KeyEvent struct {
	Code      KeyCode
	Rune      rune
	F         int
	Modifiers KeyModifiers
}

// NewKey builds a key event for a non-character key.
func NewKey(code KeyCode, mods KeyModifiers) KeyEvent {
	return KeyEvent{Code: code, Modifiers: mods}
}

// CharKey builds a character key event.
func CharKey(r rune, mods KeyModifiers) KeyEvent {
	return KeyEvent{Code: KeyChar, Rune: r, Modifiers: mods}
}

// FKey builds a function key event.
func FKey(n int, mods KeyModifiers) KeyEvent {
	return KeyEvent{Code: KeyF, F: n, Modifiers: mods}
}

// Has reports whether all of mods are held.
func (k KeyEvent) Has(mods KeyModifiers) bool { return k.Modifiers&mods == mods }

// IsChar reports whether the key is the character r (any modifiers).
func (k KeyEvent) IsChar(r rune) bool { return k.Code == KeyChar && k.Rune == r }

// IsF reports whether the key is function key n.
func (k KeyEvent) IsF(n int) bool { return k.Code == KeyF && k.F == n }
