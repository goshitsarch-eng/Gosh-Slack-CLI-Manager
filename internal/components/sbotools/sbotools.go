// Package sbotools implements the sbotools installer tab, which bootstraps
// sbopkg and sbotools step by step.
package sbotools

import (
	"fmt"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

const (
	sbopkgURL      = "https://github.com/sbopkg/sbopkg/releases/download/0.38.2/sbopkg-0.38.2-noarch-1_wsr.tgz"
	sbopkgFilename = "sbopkg-0.38.2-noarch-1_wsr.tgz"
	sboRepoURL     = "https://gitlab.com/SlackBuilds.org/slackbuilds.git"
)

// Component is the sbotools installer component.
type Component struct {
	steps       []ui.ProgressStep
	currentStep int
	outputLines []string
	isRunning   bool
}

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

func defaultSteps() []ui.ProgressStep {
	return []ui.ProgressStep{
		ui.NewProgressStep("Download sbopkg"),
		ui.NewProgressStep("Install sbopkg"),
		ui.NewProgressStep("Sync sbopkg repository"),
		ui.NewProgressStep("Install sbotools"),
		ui.NewProgressStep("Configure sbotools repository"),
		ui.NewProgressStep("Fetch SlackBuilds snapshot"),
	}
}

// New creates the component.
func New() *Component { return &Component{steps: defaultSteps()} }

// Reset clears all progress.
func (c *Component) Reset() {
	c.steps = defaultSteps()
	c.currentStep = 0
	c.outputLines = c.outputLines[:0]
	c.isRunning = false
}

func (c *Component) startInstall() {
	c.Reset()
	c.isRunning = true
	c.steps[0].Status = ui.StepRunning
}

// AddOutput appends a line of output.
func (c *Component) AddOutput(line string) {
	c.outputLines = append(c.outputLines, line)
}

// StepComplete finishes the current step. errMsg is used when !success.
func (c *Component) StepComplete(success bool, errMsg string) {
	if c.currentStep >= len(c.steps) {
		return
	}
	if success {
		c.steps[c.currentStep].Status = ui.StepComplete
	} else {
		c.steps[c.currentStep].Status = ui.StepFailed
		c.steps[c.currentStep].Error = errMsg
		c.isRunning = false
		return
	}

	c.currentStep++

	if c.currentStep < len(c.steps) {
		c.steps[c.currentStep].Status = ui.StepRunning
	} else {
		c.isRunning = false
		c.AddOutput("All steps completed successfully!")
	}
}

// CurrentCommand returns the command for the current step.
func (c *Component) CurrentCommand() (Command, bool) {
	if !c.isRunning {
		return Command{}, false
	}

	switch c.currentStep {
	case 0:
		return Command{Kind: CmdDownload, URL: sbopkgURL, Filename: sbopkgFilename}, true
	case 1:
		return Command{Kind: CmdInstallPkg, Path: fmt.Sprintf("/tmp/%s", sbopkgFilename)}, true
	case 2:
		return Command{Kind: CmdSbopkgSync}, true
	case 3:
		return Command{Kind: CmdSbopkgInstall, Package: "sbotools"}, true
	case 4:
		return Command{Kind: CmdSboconfigRepo, URL: sboRepoURL}, true
	case 5:
		return Command{Kind: CmdSbosnapFetch}, true
	}
	return Command{}, false
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.isRunning {
		return nil
	}
	switch {
	case key.Code == tui.KeyEnter:
		c.startInstall()
		return msg.StartSbotoolsInstall{}
	case key.IsChar('r'), key.IsChar('R'):
		c.Reset()
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(4),  // Title
		tui.Length(12), // Progress steps
		tui.Min(5),     // Output
	)

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(62), tui.Percentage(38))

	title := tui.ParagraphLines(
		tui.LineSpan(tui.Styled("sbotools Installer", ui.TitleStyle())),
		tui.LineSpan(tui.Styled(
			"Bootstrap the SlackBuilds toolchain and keep the UI responsive while it runs.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Bootstrap")))
	f.RenderWidget(title, header[0])

	complete := 0
	for _, step := range c.steps {
		if step.Status == ui.StepComplete {
			complete++
		}
	}
	state := tui.Styled(" READY ", ui.BadgeSuccess())
	if c.isRunning {
		state = tui.Styled(" RUNNING ", ui.BadgeWarning())
	}
	runtime := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Completed ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(complete), ui.BadgeSuccess()),
		),
		tui.LineFrom(
			tui.Styled("State ", ui.LabelStyle()),
			state,
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(runtime, header[1])

	// Description
	desc := tui.ParagraphLines(
		tui.LineStr("This will install sbopkg and sbotools for SlackBuilds.org packages."),
		tui.LineStr("The task runner stays interactive while each bootstrap step streams output."),
	).Style(ui.MutedStyle())

	progressChunks := tui.Split(chunks[1], tui.Vertical, tui.Length(3), tui.Min(5))

	f.RenderWidget(desc, progressChunks[0])

	// Progress steps
	progress := ui.NewProgressList(c.steps).Block(ui.Panel(ui.PanelTitle("Installation steps")))
	f.RenderWidget(progress, progressChunks[1])

	// Output
	outputBlock := ui.PanelAltBlock(ui.PanelTitle("Output"))
	inner := outputBlock.Inner(chunks[2])
	f.RenderWidget(outputBlock, chunks[2])

	visible := inner.Height
	start := max(len(c.outputLines)-visible, 0)
	lines := make([]tui.Line, 0, len(c.outputLines)-start)
	for _, s := range c.outputLines[start:] {
		lines = append(lines, tui.LineStr(s))
	}
	f.RenderWidget(tui.ParagraphLines(lines...).Style(ui.MutedStyle()), inner)
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	if c.isRunning {
		return nil
	}
	return []msg.KeyHelp{{Key: "Enter", Desc: "Start Installation"}, {Key: "R", Desc: "Reset"}}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() {}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
