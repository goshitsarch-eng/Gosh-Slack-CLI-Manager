// Package configeditor is a STUB awaiting the port of src/components/*.rs.
package configeditor

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// New creates the component.
func New() *Component { return &Component{} }

// IsEditing reports whether a file is open in the editor.
func (c *Component) IsEditing() bool { return false }

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {}

// LoadFile opens path in the editor.
func (c *Component) LoadFile(path string) error { return nil }

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
