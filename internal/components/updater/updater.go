// Package updater is a STUB awaiting the port of src/components/*.rs.
package updater

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is the tab component.
type Component struct{}

// New creates the component.
func New(version slackware.Version) *Component { return &Component{} }

// Reset clears all progress.
func (c *Component) Reset() {}

// AddOutput appends a line of output.
func (c *Component) AddOutput(line string) {}

// ConfirmLilo records the user's choice to run lilo.
func (c *Component) ConfirmLilo(run bool) {}

// WasLiloSkipped reports whether the lilo step was skipped.
func (c *Component) WasLiloSkipped() bool { return false }

// WasKernelUpdated reports whether a kernel package was upgraded.
func (c *Component) WasKernelUpdated() bool { return false }

// IsRunning reports whether an update is in progress.
func (c *Component) IsRunning() bool { return false }

// NeedsLiloConfirm reports whether the lilo prompt is showing.
func (c *Component) NeedsLiloConfirm() bool { return false }

// IsShowingSummary reports whether the summary dialog is showing.
func (c *Component) IsShowingSummary() bool { return false }

// CheckForKernelUpdate reports whether upgrade output mentions a kernel.
func (c *Component) CheckForKernelUpdate(output string) bool { return false }

// SetKernelUpdated records whether a kernel was updated.
func (c *Component) SetKernelUpdated(updated bool) {}

// StepComplete finishes the current step. errMsg is used when !success.
func (c *Component) StepComplete(success bool, errMsg string) {}

// CurrentCommand returns the command for the current step.
func (c *Component) CurrentCommand() (cmd string, args []string, ok bool) { return "", nil, false }

// CurrentStep returns the index of the current step.
func (c *Component) CurrentStep() int { return 0 }

// LastOutput returns the most recent output line.
func (c *Component) LastOutput() (string, bool) { return "", false }

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
