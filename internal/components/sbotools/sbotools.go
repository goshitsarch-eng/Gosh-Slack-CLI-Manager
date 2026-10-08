// Package sbotools is a STUB awaiting the port of src/components/*.rs.
package sbotools

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// CommandKind enumerates install steps.
type CommandKind uint8

const (
	CmdDownload CommandKind = iota
	CmdInstallPkg
	CmdSbopkgSync
	CmdSbopkgInstall
	CmdSboconfigRepo
	CmdSbosnapFetch
)

// Command is an install step. Download uses URL and Filename, InstallPkg
// uses Path, SbopkgInstall uses Package and SboconfigRepo uses URL.
type Command struct {
	Kind     CommandKind
	URL      string
	Filename string
	Path     string
	Package  string
}

// New creates the component.
func New() *Component { return &Component{} }

// Reset clears all progress.
func (c *Component) Reset() {}

// AddOutput appends a line of output.
func (c *Component) AddOutput(line string) {}

// StepComplete finishes the current step. errMsg is used when !success.
func (c *Component) StepComplete(success bool, errMsg string) {}

// CurrentCommand returns the command for the current step.
func (c *Component) CurrentCommand() (Command, bool) { return Command{}, false }

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
