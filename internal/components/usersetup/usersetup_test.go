package usersetup

import (
	"reflect"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func typeText(c *Component, s string) {
	for _, r := range s {
		c.HandleInput(tui.CharKey(r, tui.ModNone))
	}
}

func key(code tui.KeyCode) tui.KeyEvent { return tui.NewKey(code, tui.ModNone) }

func TestValidationMessages(t *testing.T) {
	c := New()
	steps := []struct {
		field int
		text  string
		want  string
	}{
		{0, "", "Username cannot be empty"},
		{0, "a b", "Username cannot contain spaces"},
	}
	for _, s := range steps {
		c.Reset()
		typeText(c, s.text)
		if err := c.Validate(); err == nil || err.Error() != s.want {
			t.Fatalf("%q: got %v", s.text, err)
		}
	}

	c.Reset()
	typeText(c, "bob")
	if err := c.Validate(); err == nil || err.Error() != "Password cannot be empty" {
		t.Fatal(err)
	}
	c.HandleInput(key(tui.KeyTab))
	typeText(c, "abc")
	c.HandleInput(key(tui.KeyTab))
	typeText(c, "abd")
	if err := c.Validate(); err == nil || err.Error() != "Passwords do not match" {
		t.Fatal(err)
	}
	c.HandleInput(key(tui.KeyBackspace))
	typeText(c, "c")
	if err := c.Validate(); err == nil || err.Error() != "Password must be at least 4 characters" {
		t.Fatal(err)
	}
}

func TestPasswordLengthCountsBytes(t *testing.T) {
	c := New()
	typeText(c, "bob")
	c.HandleInput(key(tui.KeyTab))
	typeText(c, "éé") // 2 chars, 4 bytes
	c.HandleInput(key(tui.KeyTab))
	typeText(c, "éé")
	if err := c.Validate(); err != nil {
		t.Fatalf("expected byte-length validation to pass, got %v", err)
	}
}

func TestEnterCreatesUserAndLocksInput(t *testing.T) {
	c := New()
	if m := c.HandleInput(key(tui.KeyEnter)); m != nil {
		t.Fatalf("invalid form returned %#v", m)
	}
	if c.errorMessage == nil || *c.errorMessage != "Username cannot be empty" {
		t.Fatal("missing validation error")
	}
	typeText(c, "bob")
	c.HandleInput(key(tui.KeyTab))
	typeText(c, "secret")
	c.HandleInput(key(tui.KeyTab))
	typeText(c, "secret")
	if m := c.HandleInput(key(tui.KeyEnter)); m != (msg.CreateUser{}) {
		t.Fatalf("got %#v", m)
	}
	if !c.isRunning || c.errorMessage != nil {
		t.Fatal("expected running state with cleared error")
	}
	typeText(c, "x") // ignored while running
	if c.Password() != "secret" || c.Username() != "bob" {
		t.Fatalf("input changed while running: %q %q", c.Username(), c.Password())
	}
	c.SetSuccess("done")
	if c.isRunning || c.successMessage == nil {
		t.Fatal("success not recorded")
	}
}

func TestGroupAndRunlevelToggles(t *testing.T) {
	c := New()
	all := []string{"wheel", "floppy", "audio", "video", "cdrom", "plugdev", "power", "netdev", "lp", "scanner"}
	if !reflect.DeepEqual(c.SelectedGroups(), all) || !c.ShouldChangeRunlevel() {
		t.Fatal("unexpected defaults")
	}
	c.HandleInput(key(tui.KeyBackTab)) // wraps to runlevel
	c.HandleInput(tui.CharKey(' ', tui.ModNone))
	if c.ShouldChangeRunlevel() {
		t.Fatal("runlevel not toggled")
	}
	c.HandleInput(key(tui.KeyUp)) // scanner
	c.HandleInput(tui.CharKey(' ', tui.ModNone))
	if got := c.SelectedGroups(); len(got) != 9 || got[8] != "lp" {
		t.Fatalf("got %v", got)
	}
	c.HandleInput(tui.CharKey('r', tui.ModControl))
	if !reflect.DeepEqual(c.SelectedGroups(), all) || !c.ShouldChangeRunlevel() || c.currentField != 0 {
		t.Fatal("reset did not restore defaults")
	}
}

func TestSpaceOnTextFieldIsTyped(t *testing.T) {
	c := New()
	typeText(c, "a b")
	if c.Username() != "a b" {
		t.Fatalf("got %q", c.Username())
	}
}
