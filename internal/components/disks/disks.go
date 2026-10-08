// Package disks is a STUB awaiting the port of src/components/*.rs.
package disks

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// New creates the component.
func New() *Component { return &Component{} }

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {}

// ActionStarted records that a disk action began.
func (c *Component) ActionStarted(action msg.DiskAction) {}

// RefreshDisks reloads disk information.
func (c *Component) RefreshDisks() {}

// FindMountPoint returns the mount point to use for device.
func (c *Component) FindMountPoint(device string) string { return "" }

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
