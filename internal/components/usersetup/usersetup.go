// Package usersetup is a STUB awaiting the port of src/components/*.rs.
package usersetup

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// New creates the component.
func New() *Component { return &Component{} }

// SetError shows an error.
func (c *Component) SetError(message string) {}

// SetSuccess shows a success message.
func (c *Component) SetSuccess(message string) {}

// Username returns the entered user name.
func (c *Component) Username() string { return "" }

// Password returns the entered password.
func (c *Component) Password() string { return "" }

// SelectedGroups returns the selected supplementary groups.
func (c *Component) SelectedGroups() []string { return nil }

// ShouldChangeRunlevel reports whether to switch the default runlevel to 4.
func (c *Component) ShouldChangeRunlevel() bool { return false }

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message { return nil }

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp { return nil }

// OnActivate implements components.Component.
func (c *Component) OnActivate() {}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
