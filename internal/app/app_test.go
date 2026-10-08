package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func TestReadOnlyModeBlocksUpdateStart(t *testing.T) {
	app := New(slackware.Version{Kind: slackware.Current}, false)

	app.Update(msg.StartUpdate{})

	if app.Updater.IsRunning() {
		t.Error("updater should not be running in read-only mode")
	}
	if got, ok := app.Updater.LastOutput(); !ok || got != "Root privileges are required for system updates." {
		t.Errorf("last output = %q, %v", got, ok)
	}
}

func TestUpdaterProgressRoutesEvenWhenOtherTabIsActive(t *testing.T) {
	app := New(slackware.Version{Kind: slackware.Current}, true)
	app.SwitchToTab(components.TabLogs)

	app.Update(msg.TaskProgress{Target: msg.TargetUpdater, Line: "slackpkg update output"})

	if got, ok := app.Updater.LastOutput(); !ok || got != "slackpkg update output" {
		t.Errorf("last output = %q, %v", got, ok)
	}
}

func TestCompactUserFormKeepsFieldsErrorsAndLastToggleVisible(t *testing.T) {
	app := New(slackware.Version{Kind: slackware.Current}, false)
	app.SwitchToTab(components.TabUserSetup)
	app.UserSetup.HandleInput(tui.NewKey(tui.KeyEnter, tui.ModNone))
	for i := 0; i < 13; i++ {
		app.UserSetup.HandleInput(tui.NewKey(tui.KeyTab, tui.ModNone))
	}
	buf := tui.NewBuffer(tui.Rect{Width: 80, Height: 24})
	app.Render(&tui.Frame{Buf: buf})
	var text strings.Builder
	for y := 0; y < buf.Area.Height; y++ {
		for x := 0; x < buf.Area.Width; x++ {
			text.WriteString(buf.Cell(x, y).Symbol)
		}
	}
	for _, label := range []string{
		"User:",
		"Password:",
		"Confirm:",
		"Username cannot be empty",
		"Change runlevel",
	} {
		if !strings.Contains(text.String(), label) {
			t.Errorf("missing %s", label)
		}
	}
}

func TestEditorShortcutsTakePrecedenceOverGlobalShortcuts(t *testing.T) {
	app := New(slackware.Version{Kind: slackware.Current}, false)
	app.SwitchToTab(components.TabConfig)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("slackware-editor-%d", os.Getpid()))
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.ConfigEditor.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	ctrl := func(r rune) tui.KeyEvent { return tui.CharKey(r, tui.ModControl) }

	app.HandleInput(tui.CharKey('x', tui.ModNone))
	m := app.HandleInput(ctrl('s'))
	if _, ok := m.(msg.SaveConfig); !ok {
		t.Fatalf("ctrl+s returned %#v, want SaveConfig", m)
	}
	if app.CurrentTab != components.TabConfig {
		t.Fatalf("current tab = %v", app.CurrentTab)
	}
	if m := app.HandleInput(ctrl('q')); m != nil {
		t.Fatalf("ctrl+q returned %#v, want nil", m)
	}
	if !app.ConfigEditor.IsEditing() {
		t.Fatal("editor should still be open")
	}
	if !app.Running {
		t.Fatal("app should still be running")
	}
	app.HandleInput(ctrl('x'))
	if app.ConfigEditor.IsEditing() {
		t.Fatal("ctrl+x should close the editor")
	}
	m = app.HandleInput(ctrl('q'))
	if _, ok := m.(msg.Quit); !ok {
		t.Fatalf("ctrl+q returned %#v, want Quit", m)
	}
}
