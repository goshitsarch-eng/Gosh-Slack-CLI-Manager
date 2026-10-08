// Package kernel is a STUB awaiting the port of src/components/*.rs.
package kernel

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// BootloaderType is the detected bootloader.
type BootloaderType uint8

const (
	BootloaderLilo BootloaderType = iota
	BootloaderGrub
	BootloaderUnknown
)

// New creates the component.
func New() *Component { return &Component{} }

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {}

// Refresh reloads kernel information.
func (c *Component) Refresh() {}

// Bootloader returns the detected bootloader.
func (c *Component) Bootloader() BootloaderType { return BootloaderUnknown }

// BuildLiloDefaultConfig returns /etc/lilo.conf with version as default.
func BuildLiloDefaultConfig(version string) (string, error) { return "", nil }

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
