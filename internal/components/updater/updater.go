// Package updater implements the system updater tab, which runs the slackpkg
// update sequence and guards the bootloader step.
package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

// Component is the system updater component - runs the slackpkg update
// sequence.
type Component struct {
	steps            []ui.ProgressStep
	currentStep      int
	outputLines      []string
	isRunning        bool
	showLiloConfirm  bool
	liloConfirmed    bool
	slackwareVersion slackware.Version
	bootloader       slackware.Bootloader
	kernelUpdated    bool
	skipInput        string
	liloSkipped      bool
	showSummary      bool
	blacklistEntries []string
	changelogPath    string // empty when no changelog was found
	newConfigFiles   []string
}

// New creates the component.
func New(version slackware.Version) *Component {
	c := &Component{
		steps:            defaultSteps(),
		slackwareVersion: version,
		bootloader:       slackware.DetectBootloader(),
	}
	c.refreshSystemContext()
	return c
}

func defaultSteps() []ui.ProgressStep {
	return []ui.ProgressStep{
		ui.NewProgressStep("Update package list"),
		ui.NewProgressStep("Install new packages"),
		ui.NewProgressStep("Upgrade all packages"),
		ui.NewProgressStep("Clean system"),
		ui.NewProgressStep("Update bootloader (lilo)"),
	}
}

// Reset clears all progress.
func (c *Component) Reset() {
	c.steps = defaultSteps()
	c.currentStep = 0
	c.outputLines = c.outputLines[:0]
	c.isRunning = false
	c.showLiloConfirm = false
	c.liloConfirmed = false
	c.kernelUpdated = false
	c.skipInput = ""
	c.liloSkipped = false
	c.showSummary = false
	c.refreshSystemContext()
}

func (c *Component) startUpdate() {
	c.Reset()
	c.isRunning = true
	c.steps[0].Status = ui.StepRunning
	c.outputLines = append(c.outputLines, fmt.Sprintf(
		"Release track: %s | Bootloader: %s",
		c.releaseTrackLabel(),
		c.bootloader.Name(),
	))
}

// AddOutput appends a line of output.
func (c *Component) AddOutput(line string) {
	c.outputLines = append(c.outputLines, line)
}

// LastOutput returns the most recent output line.
func (c *Component) LastOutput() (string, bool) {
	if len(c.outputLines) == 0 {
		return "", false
	}
	return c.outputLines[len(c.outputLines)-1], true
}

var kernelPatterns = []string{
	"kernel-generic",
	"kernel-huge",
	"kernel-modules",
	"kernel-source",
	"kernel-headers",
	"kernel-firmware",
}

// CheckForKernelUpdate reports whether upgrade output mentions a kernel.
func (c *Component) CheckForKernelUpdate(output string) bool {
	for _, pattern := range kernelPatterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

// SetKernelUpdated records whether a kernel was updated.
func (c *Component) SetKernelUpdated(updated bool) {
	c.kernelUpdated = updated
	if updated {
		c.AddOutput("*** KERNEL PACKAGES DETECTED - bootloader and config review required ***")
	}
}

// WasKernelUpdated reports whether a kernel package was upgraded.
func (c *Component) WasKernelUpdated() bool { return c.kernelUpdated }

// WasLiloSkipped reports whether the lilo step was skipped (for the exit
// warning).
func (c *Component) WasLiloSkipped() bool { return c.liloSkipped }

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
	}

	c.currentStep++

	if c.currentStep >= len(c.steps) {
		c.finish()
		return
	}

	if c.currentStep == 4 {
		switch c.bootloader {
		case slackware.BootloaderGrub:
			c.steps[c.currentStep].Status = ui.StepComplete
			c.AddOutput("GRUB detected - skipping lilo. Run 'grub-mkconfig -o /boot/grub/grub.cfg' if the kernel changed.")
			c.currentStep++
			c.finish()
			return
		case slackware.BootloaderUnknown:
			c.setFailed(c.currentStep, "No bootloader detected")
			c.AddOutput("WARNING: no bootloader configuration found. Update your bootloader manually before reboot if needed.")
			c.currentStep++
			c.finish()
			return
		case slackware.BootloaderLilo:
			if !c.liloConfirmed {
				c.showLiloConfirm = true
				c.skipInput = ""
				return
			}
		}
	}

	c.steps[c.currentStep].Status = ui.StepRunning
}

func (c *Component) setFailed(step int, errMsg string) {
	c.steps[step].Status = ui.StepFailed
	c.steps[step].Error = errMsg
}

func (c *Component) finish() {
	c.isRunning = false
	c.showSummary = true
	c.refreshSystemContext()
}

// ConfirmLilo records the user's choice to run lilo.
func (c *Component) ConfirmLilo(run bool) {
	c.showLiloConfirm = false
	c.liloConfirmed = run

	if run {
		c.steps[c.currentStep].Status = ui.StepRunning
		return
	}

	c.liloSkipped = true
	if c.kernelUpdated {
		c.setFailed(c.currentStep, "SKIPPED - KERNEL WAS UPDATED!")
	} else {
		c.setFailed(c.currentStep, "Skipped by user")
	}
	c.finish()
}

func (c *Component) dismissSummary() { c.showSummary = false }

// IsShowingSummary reports whether the summary dialog is showing.
func (c *Component) IsShowingSummary() bool { return c.showSummary }

// IsRunning reports whether an update is in progress.
func (c *Component) IsRunning() bool { return c.isRunning }

// CurrentCommand returns the command for the current step.
func (c *Component) CurrentCommand() (cmd string, args []string, ok bool) {
	if !c.isRunning {
		return "", nil, false
	}

	switch c.currentStep {
	case 0:
		return "slackpkg", []string{"update"}, true
	case 1:
		return "slackpkg", []string{"install-new"}, true
	case 2:
		return "slackpkg", []string{"upgrade-all"}, true
	case 3:
		return "slackpkg", []string{"clean-system"}, true
	case 4:
		if c.liloConfirmed {
			return "lilo", []string{}, true
		}
	}
	return "", nil, false
}

// CurrentStep returns the index of the current step.
func (c *Component) CurrentStep() int { return c.currentStep }

// NeedsLiloConfirm reports whether the lilo prompt is showing.
func (c *Component) NeedsLiloConfirm() bool { return c.showLiloConfirm }

func (c *Component) releaseTrackLabel() string {
	if c.slackwareVersion.Kind == slackware.Current {
		return "-current"
	}
	return "stable"
}

func (c *Component) refreshSystemContext() {
	c.bootloader = slackware.DetectBootloader()
	c.blacklistEntries = readBlacklistEntries()
	c.changelogPath = detectChangelogPath()
	c.newConfigFiles = scanNewConfigFiles("/etc", 12)
}

// readToString reads a file the way Rust's fs::read_to_string does,
// rejecting content that is not valid UTF-8.
func readToString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errInvalidUTF8
	}
	return string(data), nil
}

type invalidUTF8Error struct{}

func (invalidUTF8Error) Error() string { return "stream did not contain valid UTF-8" }

var errInvalidUTF8 error = invalidUTF8Error{}

func readBlacklistEntries() []string {
	content, err := readToString("/etc/slackpkg/blacklist")
	if err != nil {
		return nil
	}
	return parseBlacklistEntries(content)
}

func parseBlacklistEntries(content string) []string {
	var entries []string
	for _, line := range tui.RustLines(content) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, line)
		}
	}
	return entries
}

func detectChangelogPath() string {
	for _, path := range []string{
		"/var/lib/slackpkg/ChangeLog.txt",
		"/var/log/ChangeLog.txt",
		"/patches/ChangeLog.txt",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// scanNewConfigFiles walks root (in directory order, following symlinked
// directories) collecting up to maxResults files whose name ends in ".new".
func scanNewConfigFiles(root string, maxResults int) []string {
	var found []string
	var walk func(dir string)
	walk = func(dir string) {
		if len(found) >= maxResults {
			return
		}

		f, err := os.Open(dir)
		if err != nil {
			return
		}
		entries, _ := f.ReadDir(-1)
		f.Close()

		for _, entry := range entries {
			if len(found) >= maxResults {
				break
			}

			path := filepath.Join(dir, entry.Name())
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				walk(path)
			} else if name := entry.Name(); utf8.ValidString(name) && strings.HasSuffix(name, ".new") {
				found = append(found, path)
			}
		}
	}
	walk(root)
	return found
}

func (c *Component) advisoryLines() []tui.Line {
	changelog := "Review changelog: no local ChangeLog.txt found"
	if c.changelogPath != "" {
		changelog = "Review changelog: " + c.changelogPath
	}
	lines := []tui.Line{
		tui.LineStr("Release track: " + c.releaseTrackLabel()),
		tui.LineStr(changelog),
	}

	if len(c.blacklistEntries) == 0 {
		lines = append(lines, tui.LineStr("Blacklist: no active entries"))
	} else {
		suffix := "ies"
		if len(c.blacklistEntries) == 1 {
			suffix = "y"
		}
		lines = append(lines, tui.LineStr(fmt.Sprintf(
			"Blacklist: %d active entr%s", len(c.blacklistEntries), suffix)))
	}

	if len(c.newConfigFiles) == 0 {
		lines = append(lines, tui.LineStr("Config updates: no .new files detected under /etc"))
	} else {
		preview := c.newConfigFiles
		if len(preview) > 2 {
			preview = preview[:2]
		}
		lines = append(lines,
			tui.LineStr(fmt.Sprintf("Config updates: %d .new file(s) detected", len(c.newConfigFiles))),
			tui.LineStr("Examples: "+strings.Join(preview, ", ")),
		)
	}

	switch c.bootloader {
	case slackware.BootloaderLilo:
		lines = append(lines, tui.LineStr("Bootloader: LILO detected. Run lilo after kernel changes."))
	case slackware.BootloaderGrub:
		lines = append(lines, tui.LineStr("Bootloader: GRUB detected. Regenerate grub.cfg after kernel changes."))
	default:
		lines = append(lines, tui.LineStr("Bootloader: no known bootloader configuration detected."))
	}

	if c.releaseTrackLabel() == "-current" {
		lines = append(lines, tui.LineStr(
			"Warning: -current requires closer review of changelog and config changes."))
	}

	return lines
}

func (c *Component) stepTotals() (complete, running, failed int) {
	for _, step := range c.steps {
		switch step.Status {
		case ui.StepComplete:
			complete++
		case ui.StepRunning:
			running++
		case ui.StepFailed:
			failed++
		}
	}
	return complete, running, failed
}

func (c *Component) renderSummaryPanel(f *tui.Frame, area tui.Rect) {
	complete, running, failed := c.stepTotals()
	var nextAction string
	switch {
	case c.showLiloConfirm:
		nextAction = "Awaiting bootloader confirmation"
	case c.showSummary:
		nextAction = "Review summary before leaving"
	case c.isRunning:
		nextAction = "Streaming package manager output"
	default:
		nextAction = "Ready to start maintenance cycle"
	}

	panel := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Done ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(complete), ui.BadgeSuccess()),
			tui.Raw(" "),
			tui.Styled("Running ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(running), ui.BadgeNeutral()),
			tui.Raw(" "),
			tui.Styled("Failed ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(failed), ui.BadgeWarning()),
		),
		tui.LineFrom(
			tui.Styled("Next ", ui.LabelStyle()),
			tui.Styled(nextAction, ui.SubtitleStyle()),
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Run state")))
	f.RenderWidget(panel, area)
}

func (c *Component) showChangelogPreview() {
	if c.changelogPath == "" {
		c.AddOutput("No local ChangeLog.txt found.")
		return
	}
	path := c.changelogPath

	c.AddOutput("Showing changelog preview from " + path)
	content, err := readToString(path)
	if err != nil {
		c.AddOutput(fmt.Sprintf("Failed to read %s: %s", path, utils.IOErrorString(err)))
		return
	}
	lines := tui.RustLines(content)
	if len(lines) > 12 {
		lines = lines[:12]
	}
	for _, line := range lines {
		c.AddOutput(line)
	}
}

func (c *Component) renderLiloConfirm(f *tui.Frame, area tui.Rect) {
	dialogArea := ui.CenteredRect(60, 50, area)
	f.RenderWidget(tui.Clear{}, dialogArea)

	if c.kernelUpdated {
		dialog := ui.Panel(ui.PanelTitle("Kernel updated - bootloader required")).
			BorderStyle(ui.ErrorStyle())
		inner := dialog.Inner(dialogArea)
		f.RenderWidget(dialog, dialogArea)

		skipDisplay := strings.Repeat("*", utf8.RuneCountInString(c.skipInput))
		remaining := max(4-len(c.skipInput), 0)
		underscores := strings.Repeat("_", remaining)

		text := tui.ParagraphLines(
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Your kernel was updated.", ui.WarningStyle())),
			tui.LineStr(""),
			tui.LineStr("You must update the bootloader or your"),
			tui.LineStr("system may not boot after reboot."),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("[Y]", ui.KeyHint()),
				tui.Raw(" Update bootloader now (recommended)"),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Type SKIP", ui.ErrorStyle()),
				tui.Raw(" to bypass at your own risk"),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Raw("Input: "),
				tui.Styled(skipDisplay+underscores, ui.MutedStyle()),
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("[Backspace] to correct", ui.MutedStyle())),
		).Style(ui.DefaultStyle())
		f.RenderWidget(text, inner)
		return
	}

	dialog := ui.Panel(ui.PanelTitle("Update bootloader")).BorderStyle(ui.WarningStyle())
	inner := dialog.Inner(dialogArea)
	f.RenderWidget(dialog, dialogArea)

	text := tui.ParagraphLines(
		tui.LineStr(""),
		tui.LineStr("Run 'lilo' to update the bootloader?"),
		tui.LineStr(""),
		tui.LineSpan(tui.Styled("No kernel changes detected - safe to skip.", ui.MutedStyle())),
		tui.LineStr(""),
		tui.LineFrom(
			tui.Styled("[Y]", ui.KeyHint()),
			tui.Raw(" Yes - update bootloader"),
		),
		tui.LineFrom(
			tui.Styled("[N]", ui.KeyHint()),
			tui.Raw(" No - skip this step"),
		),
	).Style(ui.DefaultStyle())
	f.RenderWidget(text, inner)
}

func (c *Component) renderSummary(f *tui.Frame, area tui.Rect) {
	dialogArea := ui.CenteredRect(60, 60, area)
	f.RenderWidget(tui.Clear{}, dialogArea)

	warn := c.liloSkipped && c.kernelUpdated
	title := " Update Complete "
	borderStyle := ui.SuccessStyle()
	if warn {
		title = " !! UPDATE COMPLETE - WARNING !! "
		borderStyle = ui.ErrorStyle()
	}

	dialog := ui.Panel(ui.PanelTitle(title)).BorderStyle(borderStyle)
	inner := dialog.Inner(dialogArea)
	f.RenderWidget(dialog, dialogArea)

	lines := []tui.Line{tui.LineStr("")}

	for _, step := range c.steps {
		var symbol string
		var st tui.Style
		switch {
		case step.Status == ui.StepComplete:
			symbol, st = "OK", ui.SuccessStyle()
		case step.Status == ui.StepFailed && strings.Contains(step.Error, "SKIPPED"):
			symbol, st = "!!", ui.ErrorStyle()
		case step.Status == ui.StepFailed:
			symbol, st = "X", ui.ErrorStyle()
		default:
			symbol, st = "?", ui.MutedStyle()
		}

		lines = append(lines, tui.LineFrom(
			tui.Styled(fmt.Sprintf(" [%s] ", symbol), st),
			tui.Raw(step.Name),
		))
	}

	lines = append(lines, tui.LineStr(""))

	if warn {
		lines = append(lines,
			tui.LineSpan(tui.Styled("  !! WARNING !!", ui.ErrorStyle())),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("  Bootloader was not updated after kernel change.", ui.ErrorStyle())),
			tui.LineSpan(tui.Styled("  Run 'lilo' manually before rebooting.", ui.ErrorStyle())),
			tui.LineStr(""),
		)
	} else if c.liloSkipped {
		lines = append(lines,
			tui.LineSpan(tui.Styled("  Note: bootloader update was skipped.", ui.WarningStyle())),
			tui.LineStr(""),
		)
	}

	changelog := "not found locally"
	if c.changelogPath != "" {
		changelog = c.changelogPath
	}
	lines = append(lines,
		tui.LineStr("Review before reboot:"),
		tui.LineStr("  - release track: "+c.releaseTrackLabel()),
		tui.LineStr("  - changelog: "+changelog),
		tui.LineStr(fmt.Sprintf("  - blacklist entries: %d", len(c.blacklistEntries))),
		tui.LineStr(fmt.Sprintf("  - .new files under /etc: %d", len(c.newConfigFiles))),
		tui.LineStr(""),
		tui.LineFrom(
			tui.Styled("[Enter]", ui.KeyHint()),
			tui.Raw(" Acknowledge"),
		),
	)

	f.RenderWidget(tui.ParagraphLines(lines...).Style(ui.DefaultStyle()), inner)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.showSummary {
		if key.Code == tui.KeyEnter {
			c.dismissSummary()
		}
		return nil
	}

	if c.showLiloConfirm {
		if c.kernelUpdated {
			switch key.Code {
			case tui.KeyChar:
				if key.Rune == 'y' || key.Rune == 'Y' {
					c.ConfirmLilo(true)
					return msg.ContinueUpdate{}
				}
				upper := key.Rune
				if upper >= 'a' && upper <= 'z' {
					upper -= 'a' - 'A'
				}
				const expected = "SKIP"
				next := len(c.skipInput)
				if next < len(expected) && upper == rune(expected[next]) {
					c.skipInput += string(upper)
					if c.skipInput == "SKIP" {
						c.ConfirmLilo(false)
					}
				} else {
					c.skipInput = ""
				}
			case tui.KeyBackspace:
				if c.skipInput != "" {
					_, size := utf8.DecodeLastRuneInString(c.skipInput)
					c.skipInput = c.skipInput[:len(c.skipInput)-size]
				}
			case tui.KeyEsc:
				c.skipInput = ""
			}
			return nil
		}

		switch {
		case key.IsChar('y'), key.IsChar('Y'):
			c.ConfirmLilo(true)
			return msg.ContinueUpdate{}
		case key.IsChar('n'), key.IsChar('N'), key.Code == tui.KeyEsc:
			c.ConfirmLilo(false)
		}
		return nil
	}

	if c.isRunning {
		return nil
	}
	switch {
	case key.Code == tui.KeyEnter:
		c.startUpdate()
		return msg.StartUpdate{}
	case key.IsChar('c'), key.IsChar('C'):
		c.showChangelogPreview()
	case key.IsChar('r'), key.IsChar('R'):
		c.Reset()
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(5),
		tui.Length(12),
		tui.Min(5),
	)

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(62), tui.Percentage(38))

	title := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Slackware System Updater", ui.TitleStyle()),
			tui.Raw(" "),
			tui.Styled(fmt.Sprintf(" %s ", c.releaseTrackLabel()), ui.BadgeNeutral()),
			tui.Raw(" "),
			tui.Styled(fmt.Sprintf(" %s ", c.bootloader.Name()), ui.BadgeInfo()),
		),
		tui.LineSpan(tui.Styled("Review → update → bootloader → config", ui.SubtitleStyle())),
	).Block(ui.Panel(ui.PanelTitle("Update runway")))
	f.RenderWidget(title, header[0])
	c.renderSummaryPanel(f, header[1])

	middle := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(42), tui.Percentage(58))

	progress := ui.NewProgressList(c.steps).Block(ui.Panel(ui.PanelTitle("Progress")))
	f.RenderWidget(progress, middle[0])

	advisories := tui.ParagraphLines(c.advisoryLines()...).
		Wrap(true).
		Style(ui.SubtitleStyle()).
		Block(ui.PanelAltBlock(ui.PanelTitle("Maintenance notes")))
	f.RenderWidget(advisories, middle[1])

	outputBlock := ui.PanelAltBlock(ui.PanelTitle("Output"))
	inner := outputBlock.Inner(chunks[2])
	f.RenderWidget(outputBlock, chunks[2])

	visible := inner.Height
	start := max(len(c.outputLines)-visible, 0)
	lines := make([]tui.Line, 0, len(c.outputLines)-start)
	for _, line := range c.outputLines[start:] {
		if strings.Contains(line, "KERNEL") || strings.Contains(line, "Warning:") {
			lines = append(lines, tui.LineSpan(tui.Styled(line, ui.WarningStyle())))
		} else {
			lines = append(lines, tui.LineStr(line))
		}
	}
	f.RenderWidget(tui.ParagraphLines(lines...).Style(ui.DefaultStyle()), inner)

	if c.showLiloConfirm {
		c.renderLiloConfirm(f, area)
	} else if c.showSummary {
		c.renderSummary(f, area)
	}
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	switch {
	case c.showSummary:
		return []msg.KeyHelp{{Key: "Enter", Desc: "Acknowledge"}}
	case c.showLiloConfirm:
		if c.kernelUpdated {
			return []msg.KeyHelp{{Key: "Y", Desc: "Update"}, {Key: "Type SKIP", Desc: "Bypass"}}
		}
		return []msg.KeyHelp{{Key: "Y", Desc: "Yes"}, {Key: "N", Desc: "No"}}
	case c.isRunning:
		return nil
	default:
		return []msg.KeyHelp{
			{Key: "Enter", Desc: "Start Update"},
			{Key: "C", Desc: "Show Changelog"},
			{Key: "R", Desc: "Reset"},
		}
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() {}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
