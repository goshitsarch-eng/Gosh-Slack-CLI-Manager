package settings

import (
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/prefs"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func newTest() *Component {
	return &Component{settings: prefs.Default(), listState: tui.NewListState().WithSelected(0)}
}

func press(c *Component, keys ...tui.KeyEvent) msg.Message {
	var m msg.Message
	for _, k := range keys {
		m = c.HandleInput(k)
	}
	return m
}

var (
	tab   = tui.NewKey(tui.KeyTab, tui.ModNone)
	down  = tui.NewKey(tui.KeyDown, tui.ModNone)
	left  = tui.NewKey(tui.KeyLeft, tui.ModNone)
	right = tui.NewKey(tui.KeyRight, tui.ModNone)
)

func TestThemeCyclesBothWays(t *testing.T) {
	c := newTest()
	press(c, left)
	if c.settings.Theme != prefs.ThemeDracula || !c.unsavedChanges {
		t.Fatalf("got %v", c.settings.Theme)
	}
	press(c, right, right)
	if c.settings.Theme != prefs.ThemeDark {
		t.Fatalf("got %v", c.settings.Theme)
	}
}

func TestRefreshIntervalOnlyWhenAutoRefresh(t *testing.T) {
	c := newTest()
	press(c, tab, down, down, right)
	if c.settings.RefreshInterval != 5 || c.unsavedChanges {
		t.Fatal("disabled option must not change")
	}
	press(c, tui.NewKey(tui.KeyUp, tui.ModNone), right, down)
	for i := 0; i < 100; i++ {
		press(c, right)
	}
	if c.settings.RefreshInterval != 60 {
		t.Fatalf("got %d", c.settings.RefreshInterval)
	}
	for i := 0; i < 100; i++ {
		press(c, left)
	}
	if c.settings.RefreshInterval != 1 {
		t.Fatalf("got %d", c.settings.RefreshInterval)
	}
}

func TestDisplayOptions(t *testing.T) {
	c := newTest()
	press(c, tab, tab, down)
	for i := 0; i < 200; i++ {
		press(c, right)
	}
	if c.settings.LogLines != 10000 {
		t.Fatalf("got %d", c.settings.LogLines)
	}
	for i := 0; i < 200; i++ {
		press(c, left)
	}
	if c.settings.LogLines != 100 {
		t.Fatalf("got %d", c.settings.LogLines)
	}
	press(c, down, left)
	if c.settings.DefaultTab != "config" {
		t.Fatalf("got %q", c.settings.DefaultTab)
	}
	c.settings.DefaultTab = "unknown"
	press(c, right)
	if c.settings.DefaultTab != "sbotools" {
		t.Fatalf("unknown tab should cycle from the first entry, got %q", c.settings.DefaultTab)
	}
}

func TestSaveResetAndStatus(t *testing.T) {
	c := newTest()
	press(c, right)
	m := press(c, tui.CharKey('s', tui.ModNone))
	save, ok := m.(msg.SaveSettings)
	if !ok || save.Settings.Theme != prefs.ThemeDark {
		t.Fatalf("got %#v", m)
	}
	c.SetStatus("Settings saved to x", false)
	if c.unsavedChanges {
		t.Fatal("successful save clears unsaved flag")
	}
	press(c, tui.CharKey('r', tui.ModNone))
	if c.settings != prefs.Default() || !c.unsavedChanges || c.statusMessage.text != "Settings reset to defaults" {
		t.Fatal("reset failed")
	}
	c.SetStatus("boom", true)
	if !c.unsavedChanges {
		t.Fatal("error status keeps unsaved flag")
	}
}
