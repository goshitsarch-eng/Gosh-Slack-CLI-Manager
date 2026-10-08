package configeditor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func ctrl(r rune) tui.KeyEvent { return tui.CharKey(r, tui.ModControl) }

func TestLoadEditSaveAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slackpkg.conf")
	if err := os.WriteFile(path, []byte("A=1\r\nB=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New()
	if err := c.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if !c.IsEditing() || c.isModified {
		t.Fatal("expected clean editing state")
	}
	if p, content, ok := c.SaveRequest(); !ok || p != path || content != "A=1\nB=2\n" {
		t.Fatalf("save request %q %q %v", p, content, ok)
	}

	c.HandleInput(tui.CharKey('x', tui.ModNone))
	if !c.isModified {
		t.Fatal("typing should mark the buffer modified")
	}
	if m := c.HandleInput(ctrl('q')); m != nil || !c.IsEditing() {
		t.Fatal("ctrl+q with changes must not close")
	}
	if c.statusMessage == nil || c.statusMessage.text != "Unsaved changes! Ctrl+S to save, Ctrl+X to discard" || !c.statusMessage.isError {
		t.Fatalf("status %+v", c.statusMessage)
	}

	m := c.HandleInput(ctrl('s'))
	save, ok := m.(msg.SaveConfig)
	if !ok || save.Path != path || save.Content != "xA=1\nB=2\n" {
		t.Fatalf("got %#v", m)
	}
	c.SetStatus("saved", false)
	if c.isModified {
		t.Fatal("successful save should clear the modified flag")
	}
	c.HandleInput(ctrl('q'))
	if c.IsEditing() {
		t.Fatal("ctrl+q on a clean buffer should close")
	}
}

func TestDiscardAndMissingFile(t *testing.T) {
	c := New()
	path := filepath.Join(t.TempDir(), "missing")
	err := c.LoadFile(path)
	if err == nil || err.Error() != "Unable to open "+path+": No such file or directory (os error 2)" {
		t.Fatalf("got %v", err)
	}

	// Enter on a (missing) managed file shows the error in the status line.
	c.HandleInput(tui.NewKey(tui.KeyDown, tui.ModNone))
	if f, _ := c.SelectedFile(); f != "/etc/slackpkg/mirrors" {
		t.Fatalf("selected %q", f)
	}
	if _, statErr := os.Stat("/etc/slackpkg/mirrors"); statErr != nil {
		c.HandleInput(tui.NewKey(tui.KeyEnter, tui.ModNone))
		if c.statusMessage == nil || c.statusMessage.text != "Error: Unable to open /etc/slackpkg/mirrors: No such file or directory (os error 2)" {
			t.Fatalf("status %+v", c.statusMessage)
		}
	}

	ok := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(ok, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadFile(ok); err != nil {
		t.Fatal(err)
	}
	c.HandleInput(tui.CharKey('y', tui.ModNone))
	c.HandleInput(ctrl('x'))
	if c.IsEditing() || c.isModified {
		t.Fatal("ctrl+x should discard and close")
	}
}

func TestFileListNavigationClamps(t *testing.T) {
	c := New()
	for i := 0; i < 5; i++ {
		c.HandleInput(tui.CharKey('j', tui.ModNone))
	}
	if f, _ := c.SelectedFile(); f != "/etc/sbotools/sbotools.conf" {
		t.Fatalf("got %q", f)
	}
	for i := 0; i < 5; i++ {
		c.HandleInput(tui.CharKey('k', tui.ModNone))
	}
	if f, _ := c.SelectedFile(); f != "/etc/slackpkg/slackpkg.conf" {
		t.Fatalf("got %q", f)
	}
}
