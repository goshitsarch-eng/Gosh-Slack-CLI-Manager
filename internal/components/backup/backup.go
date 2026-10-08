// Package backup is a STUB awaiting the port of src/components/*.rs.
package backup

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// ConfigFile is a file that can be included in backups.
type ConfigFile struct {
	Path        string
	Description string
	Include     bool
}

// New creates the component.
func New() *Component { return &Component{} }

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {}

// ConfigFiles returns the backup file selection.
func (c *Component) ConfigFiles() []ConfigFile { return nil }

// RefreshBackups reloads the list of backups.
func (c *Component) RefreshBackups() {}

// Execute performs a backup action. It runs off the UI goroutine.
func Execute(action msg.BackupAction, files []ConfigFile) (string, error) { return "", nil }

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
