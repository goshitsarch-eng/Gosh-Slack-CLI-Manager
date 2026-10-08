// Package kernel implements the Kernel Manager tab: installed kernel images
// under /boot, the running kernel, the detected bootloader, and setting the
// LILO default.
package kernel

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

const liloConf = "/etc/lilo.conf"

// kernelInfo describes an installed kernel image.
type kernelInfo struct {
	version   string
	variant   string // generic, huge, etc.
	path      string
	isCurrent bool
	isDefault bool
	size      uint64
}

// BootloaderType is the detected bootloader.
type BootloaderType uint8

const (
	BootloaderLilo BootloaderType = iota
	BootloaderGrub
	BootloaderUnknown
)

type status struct {
	message string
	isError bool
}

// Component is the Kernel Manager tab.
type Component struct {
	kernels       []kernelInfo
	listState     tui.ListState
	currentKernel string
	bootloader    BootloaderType
	status        *status
	showConfirm   bool
	pendingAction *msg.KernelAction
}

// New creates the component.
func New() *Component {
	c := &Component{bootloader: BootloaderUnknown}
	c.loadKernelInfo()
	if len(c.kernels) > 0 {
		c.listState.Select(0)
	}
	return c
}

func (c *Component) loadKernelInfo() {
	c.kernels = c.kernels[:0]

	// Get current running kernel
	cmd := exec.Command("uname", "-r")
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) {
		c.currentKernel = strings.TrimSpace(strings.ToValidUTF8(string(out), "�"))
	}

	// Detect bootloader
	c.bootloader = detectBootloader()

	// Scan for installed kernels
	c.scanKernels()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectBootloader() BootloaderType {
	switch {
	case pathExists(liloConf):
		return BootloaderLilo
	case pathExists("/boot/grub/grub.cfg") || pathExists("/boot/grub2/grub.cfg"):
		return BootloaderGrub
	default:
		return BootloaderUnknown
	}
}

// readDirOrder lists a directory in the order the OS returns entries (the
// order Rust's fs::read_dir yields), unlike os.ReadDir which sorts.
func readDirOrder(path string) ([]os.DirEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

// trimStartMatches removes every leading repetition of prefix, like Rust's
// str::trim_start_matches.
func trimStartMatches(s, prefix string) string {
	for strings.HasPrefix(s, prefix) {
		s = s[len(prefix):]
	}
	return s
}

func (c *Component) scanKernels() {
	if entries, err := readDirOrder("/boot"); err == nil {
		for _, e := range entries {
			name := e.Name()

			// Look for vmlinuz-* files
			if !strings.HasPrefix(name, "vmlinuz-") {
				continue
			}
			version := trimStartMatches(name, "vmlinuz-")

			// Determine variant
			variant := "custom"
			if strings.Contains(version, "-generic") {
				variant = "generic"
			} else if strings.Contains(version, "-huge") {
				variant = "huge"
			}

			var size uint64
			if info, err := e.Info(); err == nil {
				size = uint64(info.Size())
			}

			bare := strings.ReplaceAll(strings.ReplaceAll(version, "-generic", ""), "-huge", "")
			c.kernels = append(c.kernels, kernelInfo{
				version:   version,
				variant:   variant,
				path:      filepath.Join("/boot", name),
				isCurrent: strings.Contains(c.currentKernel, bare),
				isDefault: c.isDefaultKernel(name),
				size:      size,
			})
		}
	}

	// Sort by version (newest first)
	sort.SliceStable(c.kernels, func(i, j int) bool { return c.kernels[i].version > c.kernels[j].version })
}

// readToString mirrors fs::read_to_string, including its error for files
// that are not valid UTF-8.
func readToString(path string) (string, error) {
	content, err := utils.ReadFileString(path)
	if err != nil {
		return "", utils.PlainIOError(err)
	}
	return content, nil
}

// strLines mirrors Rust's str::lines: split on '\n', removing a "\r" only
// when it precedes the '\n', with no final empty line.
func strLines(s string) []string {
	var lines []string
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, strings.TrimSuffix(s[:i], "\r"))
		s = s[i+1:]
	}
	return lines
}

// secondField mirrors `line.split('=').nth(1)`.
func secondField(line string) (string, bool) {
	parts := strings.SplitN(line, "=", 3)
	if len(parts) < 2 {
		return "", false
	}
	return parts[1], true
}

func (c *Component) isDefaultKernel(kernelName string) bool {
	if c.bootloader != BootloaderLilo {
		return false
	}
	content, err := readToString(liloConf)
	if err != nil {
		return false
	}
	// Find the default entry and check if it matches this kernel
	defaultLabel, currentImage := "", ""
	for _, line := range strLines(content) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "default") {
			if label, ok := secondField(line); ok {
				defaultLabel = strings.TrimSpace(label)
			}
		}
		if strings.HasPrefix(line, "image") {
			if path, ok := secondField(line); ok {
				currentImage = strings.TrimSpace(path)
			}
		}
		if strings.HasPrefix(line, "label") {
			if label, ok := secondField(line); ok {
				if strings.TrimSpace(label) == defaultLabel && strings.Contains(currentImage, kernelName) {
					return true
				}
			}
		}
	}
	return false
}

func (c *Component) selectedKernel() (kernelInfo, bool) {
	if i, ok := c.listState.Selected(); ok && i < len(c.kernels) {
		return c.kernels[i], true
	}
	return kernelInfo{}, false
}

// BuildLiloDefaultConfig returns /etc/lilo.conf with version as default.
func BuildLiloDefaultConfig(version string) (string, error) {
	content, err := readToString(liloConf)
	if err != nil {
		return "", err
	}
	return liloWithDefault(content, version)
}

// liloWithDefault rewrites lilo.conf content so the image matching version
// becomes the default entry.
func liloWithDefault(content, version string) (string, error) {
	foundLabel := ""

	currentImage := ""
	for _, line := range strLines(content) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "image") {
			if path, ok := secondField(trimmed); ok {
				currentImage = strings.TrimSpace(path)
			}
		}
		if strings.HasPrefix(trimmed, "label") && strings.Contains(currentImage, version) {
			if label, ok := secondField(trimmed); ok {
				foundLabel = strings.TrimSpace(label)
				break
			}
		}
	}

	if foundLabel == "" {
		return "", errors.New("Kernel not found in lilo.conf")
	}

	var b strings.Builder
	defaultSet := false
	for _, line := range strLines(content) {
		if strings.HasPrefix(strings.TrimSpace(line), "default") {
			fmt.Fprintf(&b, "default = %s\n", foundLabel)
			defaultSet = true
		} else {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	newContent := b.String()
	if !defaultSet {
		newContent = fmt.Sprintf("default = %s\n%s", foundLabel, newContent)
	}
	return newContent, nil
}

// Refresh reloads kernel information.
func (c *Component) Refresh() { c.loadKernelInfo() }

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.status = &status{message: message, isError: isError}
}

// Bootloader returns the detected bootloader.
func (c *Component) Bootloader() BootloaderType { return c.bootloader }

func formatSize(bytes uint64) string {
	const mb = 1024 * 1024
	return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
}

func (c *Component) renderSelectedKernel(f *tui.Frame, area tui.Rect) {
	var lines []tui.Line
	if k, ok := c.selectedKernel(); ok {
		running := "Not the running image"
		if k.isCurrent {
			running = "RUNNING kernel image"
		}
		def := "Not the configured default"
		if k.isDefault {
			def = "Configured default boot target"
		}
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("KERNEL", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(k.version, ui.TitleStyle()),
			),
			tui.LineFrom(
				tui.Styled("Variant ", ui.LabelStyle()),
				tui.Styled(k.variant, ui.BadgeNeutral()),
			),
			tui.LineFrom(
				tui.Styled("Size ", ui.LabelStyle()),
				tui.Raw(formatSize(k.size)),
			),
			tui.LineFrom(
				tui.Styled("Path ", ui.LabelStyle()),
				tui.Raw(k.path),
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Flags", ui.EyebrowStyle())),
			tui.LineStr(running),
			tui.LineStr(def),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Actions", ui.EyebrowStyle())),
			tui.LineStr("d/Enter  set default"),
			tui.LineStr("l        run lilo"),
		}
	} else {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled("No kernel selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Choose a kernel image to inspect its boot role."),
		}
	}

	panel := tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector")))
	f.RenderWidget(panel, area)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.showConfirm {
		switch {
		case key.IsChar('y') || key.IsChar('Y'):
			c.showConfirm = false
			if action := c.pendingAction; action != nil {
				c.pendingAction = nil
				return msg.KernelActionMsg{Action: *action}
			}
		case key.IsChar('n') || key.IsChar('N') || key.Code == tui.KeyEsc:
			c.showConfirm = false
			c.pendingAction = nil
		}
		return nil
	}

	switch {
	case key.Code == tui.KeyUp || key.IsChar('k'):
		if selected, ok := c.listState.Selected(); ok && selected > 0 {
			c.listState.Select(selected - 1)
		}
	case key.Code == tui.KeyDown || key.IsChar('j'):
		if selected, ok := c.listState.Selected(); ok && selected < max(len(c.kernels)-1, 0) {
			c.listState.Select(selected + 1)
		}
	case key.Code == tui.KeyEnter || key.IsChar('d'):
		if k, ok := c.selectedKernel(); ok {
			c.pendingAction = &msg.KernelAction{Kind: msg.KernelSetDefault, Version: k.version}
			c.showConfirm = true
		}
	case key.IsChar('l'):
		if c.bootloader == BootloaderLilo {
			c.pendingAction = &msg.KernelAction{Kind: msg.KernelRunLilo}
			c.showConfirm = true
		}
	case key.IsF(5):
		c.loadKernelInfo()
		c.status = &status{message: "Kernel list refreshed"}
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(5), tui.Min(10), tui.Length(3))

	// Info header
	bootloaderStr := "Unknown"
	switch c.bootloader {
	case BootloaderLilo:
		bootloaderStr = "LILO"
	case BootloaderGrub:
		bootloaderStr = "GRUB"
	}

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(56), tui.Percentage(44))

	info := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Running ", ui.LabelStyle()),
			tui.Styled(c.currentKernel, ui.BadgeSuccess()),
		),
		tui.LineFrom(
			tui.Styled("Bootloader ", ui.LabelStyle()),
			tui.Styled(bootloaderStr, ui.BadgeNeutral()),
		),
	).Block(ui.Panel(ui.PanelTitle("Kernel manager")))
	f.RenderWidget(info, header[0])

	bootPath := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Installed ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.kernels)), ui.BadgeSuccess()),
			tui.Raw(" "),
			tui.Styled("Bootloader ", ui.LabelStyle()),
			tui.Styled(bootloaderStr, ui.BadgeNeutral()),
		),
		tui.LineSpan(tui.Styled(
			"Default changes rewrite config first; run lilo afterward if LILO is in play.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Boot path")))
	f.RenderWidget(bootPath, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(64), tui.Percentage(36))

	items := make([]tui.ListItem, 0, len(c.kernels))
	for _, k := range c.kernels {
		spans := []tui.Span{
			tui.Styled(fmt.Sprintf("%-40s", k.version), ui.TitleStyle()),
			tui.Styled(fmt.Sprintf("%-10s", k.variant), ui.AccentStyle()),
		}
		if k.isCurrent {
			spans = append(spans, tui.Styled(" RUNNING ", ui.BadgeSuccess()))
		}
		if k.isDefault {
			spans = append(spans, tui.Styled(" DEFAULT ", ui.BadgeWarning()))
		}
		items = append(items, tui.ListItemLines(
			tui.LineFrom(spans...),
			tui.LineFrom(
				tui.Styled("    Path ", ui.MutedStyle()),
				tui.Raw(k.path),
				tui.Styled("  Size ", ui.MutedStyle()),
				tui.Raw(formatSize(k.size)),
			),
		))
	}

	list := tui.NewList(items).
		Block(ui.PanelAltBlock(ui.PanelTitle(fmt.Sprintf("Installed kernels (%d)", len(c.kernels))))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")

	st := c.listState
	list.RenderStateful(content[0], f.Buffer(), &st)
	c.renderSelectedKernel(f, content[1])

	// Status bar
	var statusContent tui.Line
	switch {
	case c.showConfirm:
		actionDesc := "Confirm action?"
		if c.pendingAction != nil {
			switch c.pendingAction.Kind {
			case msg.KernelSetDefault:
				actionDesc = fmt.Sprintf("Set %s as default?", c.pendingAction.Version)
			case msg.KernelRunLilo:
				actionDesc = "Run lilo to update bootloader?"
			}
		}
		statusContent = tui.LineFrom(
			tui.Styled(actionDesc, ui.WarningStyle()),
			tui.Raw(" [Y]es / [N]o"),
		)
	case c.status != nil:
		st := ui.SuccessStyle()
		if c.status.isError {
			st = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.status.message, st))
	default:
		statusContent = tui.LineSpan(tui.Styled("Press 'd' to set default, 'l' to run lilo", ui.MutedStyle()))
	}

	statusBar := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(statusBar, chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{
		{Key: "d/Enter", Desc: "Set Default"},
		{Key: "l", Desc: "Run LILO"},
		{Key: "F5", Desc: "Refresh"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.loadKernelInfo() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
