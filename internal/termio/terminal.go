// Package termio connects the tui cell buffer to a real terminal through
// tcell: raw mode, alternate screen, mouse capture, truecolor output and key
// events translated to the crossterm-style KeyEvent the app expects.
package termio

import (
	"os"
	"unicode"

	"github.com/gdamore/tcell/v2"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Terminal is an initialized full-screen terminal.
type Terminal struct {
	screen tcell.Screen
	buf    *tui.Buffer
	events chan Event
}

// Event is a terminal input event. Key is nil for non-key events such as
// resizes.
type Event struct {
	Key *tui.KeyEvent
}

// Open switches the terminal into full-screen raw mode.
func Open() (*Terminal, error) {
	// Always emit 24-bit color sequences, as the original crossterm backend
	// did, unless the user explicitly opted out.
	if os.Getenv("TCELL_TRUECOLOR") == "" {
		_ = os.Setenv("TCELL_TRUECOLOR", "enable")
	}
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := screen.Init(); err != nil {
		return nil, err
	}
	screen.EnableMouse()
	screen.HideCursor()
	screen.Clear()

	t := &Terminal{screen: screen, buf: tui.NewBuffer(tui.Rect{}), events: make(chan Event, 64)}
	go t.pump()
	return t, nil
}

// Close restores the terminal.
func (t *Terminal) Close() {
	t.screen.DisableMouse()
	t.screen.Fini()
}

// Events delivers input events.
func (t *Terminal) Events() <-chan Event { return t.events }

func (t *Terminal) pump() {
	for {
		ev := t.screen.PollEvent()
		if ev == nil {
			close(t.events)
			return
		}
		switch e := ev.(type) {
		case *tcell.EventKey:
			if key, ok := translateKey(e); ok {
				t.events <- Event{Key: &key}
			}
		case *tcell.EventResize:
			t.events <- Event{}
		}
	}
}

// Draw renders one frame. Cells are pushed to tcell, which only writes the
// ones that changed since the previous frame.
func (t *Terminal) Draw(render func(f *tui.Frame)) {
	w, h := t.screen.Size()
	area := tui.Rect{Width: w, Height: h}
	if area != t.buf.Area {
		t.buf = tui.NewBuffer(area)
		t.screen.Clear()
	} else {
		t.buf.ResetAll()
	}

	render(&tui.Frame{Buf: t.buf})

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := t.buf.Cell(x, y)
			if cell.Skip {
				continue
			}
			runes := []rune(cell.Symbol)
			mainc := ' '
			var comb []rune
			if len(runes) > 0 {
				mainc, comb = runes[0], runes[1:]
			}
			t.screen.SetContent(x, y, mainc, comb, toTcellStyle(cell))
		}
	}
	t.screen.HideCursor()
	t.screen.Show()
}

func toTcellColor(c tui.Color) tcell.Color {
	switch c.Kind {
	case tui.ColorKindIndexed:
		return tcell.PaletteColor(int(c.Index))
	case tui.ColorKindRGB:
		return tcell.NewRGBColor(int32(c.R), int32(c.G), int32(c.B))
	default:
		return tcell.ColorReset
	}
}

func toTcellStyle(cell *tui.Cell) tcell.Style {
	style := tcell.StyleDefault.Foreground(toTcellColor(cell.Fg)).Background(toTcellColor(cell.Bg))
	m := cell.Modifier
	return style.
		Bold(m&tui.Bold != 0).
		Dim(m&tui.Dim != 0).
		Italic(m&tui.Italic != 0).
		Underline(m&tui.Underlined != 0).
		Blink(m&(tui.SlowBlink|tui.RapidBlink) != 0).
		Reverse(m&tui.Reversed != 0).
		StrikeThrough(m&tui.CrossedOut != 0)
}

func translateMods(m tcell.ModMask) tui.KeyModifiers {
	var mods tui.KeyModifiers
	if m&tcell.ModShift != 0 {
		mods |= tui.ModShift
	}
	if m&tcell.ModCtrl != 0 {
		mods |= tui.ModControl
	}
	if m&tcell.ModAlt != 0 {
		mods |= tui.ModAlt
	}
	return mods
}

var simpleKeys = map[tcell.Key]tui.KeyCode{
	tcell.KeyEnter:     tui.KeyEnter,
	tcell.KeyTab:       tui.KeyTab,
	tcell.KeyBackspace: tui.KeyBackspace,
	tcell.KeyDelete:    tui.KeyDelete,
	tcell.KeyInsert:    tui.KeyInsert,
	tcell.KeyEsc:       tui.KeyEsc,
	tcell.KeyUp:        tui.KeyUp,
	tcell.KeyDown:      tui.KeyDown,
	tcell.KeyLeft:      tui.KeyLeft,
	tcell.KeyRight:     tui.KeyRight,
	tcell.KeyHome:      tui.KeyHome,
	tcell.KeyEnd:       tui.KeyEnd,
	tcell.KeyPgUp:      tui.KeyPageUp,
	tcell.KeyPgDn:      tui.KeyPageDown,
}

// translateKey maps tcell keys to the crossterm KeyEvent conventions the
// application logic was written against.
func translateKey(e *tcell.EventKey) (tui.KeyEvent, bool) {
	mods := translateMods(e.Modifiers())
	k := e.Key()

	if k == tcell.KeyRune {
		r := e.Rune()
		if unicode.IsUpper(r) {
			mods |= tui.ModShift
		}
		return tui.CharKey(r, mods), true
	}
	if k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		return tui.CharKey(rune('a'+(k-tcell.KeyCtrlA)), mods|tui.ModControl), true
	}
	if k >= tcell.KeyF1 && k <= tcell.KeyF64 {
		return tui.FKey(int(k-tcell.KeyF1)+1, mods), true
	}

	if code, ok := simpleKeys[k]; ok {
		return tui.NewKey(code, mods), true
	}

	switch k {
	case tcell.KeyBacktab:
		return tui.NewKey(tui.KeyBackTab, mods|tui.ModShift), true
	case tcell.KeyCtrlSpace:
		return tui.CharKey(' ', mods|tui.ModControl), true
	case tcell.KeyCtrlBackslash, tcell.KeyCtrlRightSq, tcell.KeyCtrlCarat, tcell.KeyCtrlUnderscore:
		return tui.CharKey(rune('4'+(k-tcell.KeyCtrlBackslash)), mods|tui.ModControl), true
	}
	return tui.KeyEvent{}, false
}
