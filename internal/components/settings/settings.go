// Package settings is a STUB awaiting the port of src/components/*.rs.
package settings

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/prefs"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// New creates the component, loading saved settings.
func New() *Component { return &Component{} }

// Settings returns the current settings.
func (c *Component) Settings() prefs.AppSettings { return prefs.Default() }

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {}

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
