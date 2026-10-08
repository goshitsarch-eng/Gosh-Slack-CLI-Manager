package ui

import (
	"fmt"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// StepStatus is the state of a progress step.
type StepStatus uint8

const (
	StepPending StepStatus = iota
	StepRunning
	StepComplete
	StepFailed
)

// ProgressStep is a step in a progress wizard. Error is set when Failed.
type ProgressStep struct {
	Name   string
	Status StepStatus
	Error  string
}

// NewProgressStep returns a pending step.
func NewProgressStep(name string) ProgressStep {
	return ProgressStep{Name: name, Status: StepPending}
}

// ProgressList draws a list of progress steps.
type ProgressList struct {
	Steps    []ProgressStep
	block    tui.Block
	hasBlock bool
}

// NewProgressList returns a progress list widget.
func NewProgressList(steps []ProgressStep) ProgressList { return ProgressList{Steps: steps} }

// Block wraps the list in a block.
func (p ProgressList) Block(b tui.Block) ProgressList {
	p.block, p.hasBlock = b, true
	return p
}

// Render implements tui.Widget.
func (p ProgressList) Render(area tui.Rect, buf *tui.Buffer) {
	inner := area
	if p.hasBlock {
		inner = p.block.Inner(area)
		p.block.Render(area, buf)
	}

	for y := inner.Top(); y < inner.Bottom(); y++ {
		for x := inner.Left(); x < inner.Right(); x++ {
			buf.Cell(x, y).SetStyle(SurfaceStyle())
		}
	}

	for i, step := range p.Steps {
		if i >= inner.Height {
			break
		}
		var icon, detail string
		var st tui.Style
		switch step.Status {
		case StepPending:
			icon, st, detail = "·", ProgressPending(), "Queued"
		case StepRunning:
			icon, st, detail = "◆", ProgressRunning(), "Running"
		case StepComplete:
			icon, st, detail = "■", ProgressComplete(), "Done"
		default:
			icon, st, detail = "×", ErrorStyle(), "Failed"
		}
		line := tui.LineFrom(
			tui.Styled(" ", SurfaceStyle()),
			tui.Styled(fmt.Sprintf(" %s ", icon), st),
			tui.Styled(step.Name, st),
			tui.Styled("  ", SurfaceStyle()),
			tui.Styled(detail, MutedStyle()),
		)
		buf.SetLine(inner.X, inner.Y+i, line, inner.Width)
	}
}

// StatusBar is the bottom controls bar.
type StatusBar struct {
	message string
	keys    []msg.KeyHelp
}

// NewStatusBar returns a status bar showing message when there are no keys.
func NewStatusBar(message string) StatusBar { return StatusBar{message: message} }

// Keys sets the key hints.
func (s StatusBar) Keys(keys []msg.KeyHelp) StatusBar {
	s.keys = keys
	return s
}

// Render implements tui.Widget.
func (s StatusBar) Render(area tui.Rect, buf *tui.Buffer) {
	if area.IsEmpty() {
		return
	}
	for y := area.Top(); y < area.Bottom(); y++ {
		for x := area.Left(); x < area.Right(); x++ {
			buf.Cell(x, y).SetStyle(StatusBarStyle())
		}
	}

	var spans []tui.Span
	if len(s.keys) > 0 {
		spans = append(spans, tui.Styled(" CONTROLS ", BadgeInfo()), tui.Styled("  ", StatusBarStyle()))
	}
	for _, k := range s.keys {
		spans = append(spans,
			tui.Styled(fmt.Sprintf(" %s ", k.Key), KeyHint()),
			tui.Styled(" ", StatusBarStyle()),
			tui.Styled(k.Desc, StatusBarStyle()),
			tui.Styled("  •  ", MutedStyle()),
		)
	}
	if len(s.keys) > 0 {
		spans = spans[:len(spans)-1]
	}
	if len(s.keys) == 0 && s.message != "" {
		if len(spans) > 0 {
			spans = append(spans, tui.Styled("  //  ", MutedStyle()))
		}
		spans = append(spans, tui.Styled(s.message, StatusBarStyle()))
	}

	tui.NewParagraph(tui.TextLines(tui.LineFrom(spans...))).Wrap(true).Render(area, buf)
}
