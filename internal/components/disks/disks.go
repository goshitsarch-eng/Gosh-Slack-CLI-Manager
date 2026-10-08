// Package disks implements the disk management tab (port of
// src/components/disks.rs). The component itself only reads disk
// information (df, lsblk, /etc/fstab); mount, unmount and filesystem checks
// are requested from the app via msg.DiskActionMsg.
package disks

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// diskInfo is disk/partition information.
type diskInfo struct {
	Name          string
	MountPoint    string
	HasMountPoint bool
	Filesystem    string
	Size          uint64
	Used          uint64
	Available     uint64
	UsePercent    uint8
	IsMounted     bool
	DevicePath    string
}

type diskMode uint8

const (
	modeOverview diskMode = iota
	modeDetails
)

type status struct {
	text    string
	isError bool
}

// Command runners and the fstab path; variables so tests can replace them.
var (
	runDf = func() ([]byte, error) {
		return exec.Command("df", "-B1", "--output=source,target,fstype,size,used,avail,pcent").Output()
	}
	runLsblk = func() ([]byte, error) {
		return exec.Command("lsblk", "-b", "-n", "-o", "NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT").Output()
	}
	fstabPath = "/etc/fstab"
)

// Component is the disk management component.
type Component struct {
	disks         []diskInfo
	listState     tui.ListState
	mode          diskMode
	statusMessage *status
	showConfirm   bool
	pendingAction *msg.DiskAction
}

// New creates the component and loads disk information.
func New() *Component {
	c := &Component{}
	c.loadDiskInfo()
	if len(c.disks) > 0 {
		c.listState.Select(0)
	}
	return c
}

// commandStdout mirrors `if let Ok(output) = Command::output()`: the output
// is used whenever the program could be spawned, whatever its exit status.
func commandStdout(run func() ([]byte, error)) (string, bool) {
	out, err := run()
	if err != nil {
		if _, isExit := err.(*exec.ExitError); !isExit {
			return "", false
		}
	}
	return lossy(out), true
}

// lossy mirrors String::from_utf8_lossy.
func lossy(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size <= 1 {
			sb.WriteRune(utf8.RuneError)
		} else {
			sb.Write(b[:size])
		}
		b = b[size:]
	}
	return sb.String()
}

// parseUint mirrors Rust's str::parse for unsigned integers: an optional
// leading '+' is accepted; anything else invalid or out of range fails.
func parseUint(s string, bits int) (uint64, bool) {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "+"), 10, bits)
	if err != nil || strings.HasPrefix(s, "+-") || strings.HasPrefix(s, "++") {
		return 0, false
	}
	return v, true
}

func parseU64(s string) uint64 {
	v, _ := parseUint(s, 64)
	return v
}

func trimStartMatches(s, prefix string) string {
	for strings.HasPrefix(s, prefix) {
		s = s[len(prefix):]
	}
	return s
}

func (c *Component) loadDiskInfo() {
	c.disks = c.disks[:0]

	// Use df to get mounted filesystems
	if stdout, ok := commandStdout(runDf); ok {
		lines := tui.RustLines(stdout)
		if len(lines) > 0 {
			lines = lines[1:] // Skip header
		}
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) < 7 {
				continue
			}
			// Skip pseudo filesystems
			source := parts[0]
			if !strings.HasPrefix(source, "/dev/") {
				continue
			}

			name := trimStartMatches(source, "/dev/")
			usePercent, _ := parseUint(strings.TrimRight(parts[6], "%"), 8)

			c.disks = append(c.disks, diskInfo{
				Name:          name,
				MountPoint:    parts[1],
				HasMountPoint: true,
				Filesystem:    parts[2],
				Size:          parseU64(parts[3]),
				Used:          parseU64(parts[4]),
				Available:     parseU64(parts[5]),
				UsePercent:    uint8(usePercent),
				IsMounted:     true,
				DevicePath:    source,
			})
		}
	}

	// Also scan for unmounted block devices
	c.scanBlockDevices()
}

func (c *Component) scanBlockDevices() {
	// Use lsblk for additional info
	if stdout, ok := commandStdout(runLsblk); ok {
		for _, line := range tui.RustLines(stdout) {
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			name := trimStartMatches(trimStartMatches(parts[0], "├─"), "└─")
			size := parseU64(parts[1])
			deviceType := parts[2]

			// Only interested in partitions and disks
			if deviceType != "part" && deviceType != "disk" {
				continue
			}

			filesystem := ""
			if len(parts) > 3 {
				filesystem = parts[3]
			}
			mountPoint, hasMountPoint := "", false
			if len(parts) > 4 {
				mountPoint, hasMountPoint = parts[4], true
			}

			// Check if already in list
			exists := false
			for i := range c.disks {
				if c.disks[i].Name == name {
					exists = true
					break
				}
			}
			if !exists && !strings.HasPrefix(name, "loop") {
				c.disks = append(c.disks, diskInfo{
					Name:          name,
					MountPoint:    mountPoint,
					HasMountPoint: hasMountPoint,
					Filesystem:    filesystem,
					Size:          size,
					Used:          0,
					Available:     size,
					UsePercent:    0,
					IsMounted:     false,
					DevicePath:    fmt.Sprintf("/dev/%s", name),
				})
			}
		}
	}

	// Sort by name
	sort.SliceStable(c.disks, func(i, j int) bool { return c.disks[i].Name < c.disks[j].Name })
}

func (c *Component) selectedDisk() *diskInfo {
	if i, ok := c.listState.Selected(); ok && i < len(c.disks) {
		return &c.disks[i]
	}
	return nil
}

// ActionStarted records that a disk action began.
func (c *Component) ActionStarted(action msg.DiskAction) {
	var message string
	switch action.Kind {
	case msg.DiskMount:
		message = fmt.Sprintf("Mounting %s...", action.Target)
	case msg.DiskUnmount:
		message = fmt.Sprintf("Unmounting %s...", action.Target)
	case msg.DiskCheckFilesystem:
		message = fmt.Sprintf("Checking %s...", action.Target)
	}
	c.statusMessage = &status{message, false}
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.statusMessage = &status{message, isError}
}

// RefreshDisks reloads disk information.
func (c *Component) RefreshDisks() { c.loadDiskInfo() }

// FindMountPoint returns the mount point to use for device: the one
// configured in /etc/fstab, or /mnt/<name>.
func (c *Component) FindMountPoint(device string) string {
	// Check fstab for configured mount point
	if data, err := os.ReadFile(fstabPath); err == nil && utf8.Valid(data) {
		for _, line := range tui.RustLines(string(data)) {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[0] == device {
				return parts[1]
			}
		}
	}

	// Create mount point based on device name
	name := trimStartMatches(device, "/dev/")
	return fmt.Sprintf("/mnt/%s", name)
}

func formatSize(bytes uint64) string {
	const (
		kb uint64 = 1024
		mb        = kb * 1024
		gb        = mb * 1024
		tb        = gb * 1024
	)

	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func usageColor(percent uint8) tui.Color {
	switch {
	case percent >= 90:
		return tui.Red
	case percent >= 75:
		return tui.Yellow
	default:
		return tui.Green
	}
}

func (d *diskInfo) mountOr(def string) string {
	if d.HasMountPoint {
		return d.MountPoint
	}
	return def
}

func (c *Component) renderSelectedDisk(f *tui.Frame, area tui.Rect) {
	var lines []tui.Line
	if disk := c.selectedDisk(); disk != nil {
		state := "unmounted"
		if disk.IsMounted {
			state = "mounted"
		}
		lines = []tui.Line{
			tui.LineSpan(tui.Styled(disk.Name, ui.TitleStyle())),
			tui.LineFrom(
				tui.Styled("State ", ui.MutedStyle()),
				tui.Styled(state, ui.KeyHintSecondary()),
			),
			tui.LineFrom(
				tui.Styled("Filesystem ", ui.MutedStyle()),
				tui.Raw(disk.Filesystem),
			),
			tui.LineFrom(
				tui.Styled("Device ", ui.MutedStyle()),
				tui.Raw(disk.DevicePath),
			),
			tui.LineFrom(
				tui.Styled("Mount ", ui.MutedStyle()),
				tui.Raw(disk.mountOr("-")),
			),
			tui.LineFrom(
				tui.Styled("Capacity ", ui.MutedStyle()),
				tui.Raw(formatSize(disk.Size)),
			),
			tui.LineStr(""),
			tui.LineStr("Actions: m mount, u unmount, f check fs"),
		}
	} else {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled("No disk selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Pick a disk to inspect its mount and capacity data."),
		}
	}

	f.RenderWidget(
		tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
		area,
	)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.showConfirm {
		switch {
		case key.IsChar('y') || key.IsChar('Y'):
			c.showConfirm = false
			if c.pendingAction != nil {
				action := *c.pendingAction
				c.pendingAction = nil
				return msg.DiskActionMsg{Action: action}
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
		if selected, ok := c.listState.Selected(); ok && selected < max(len(c.disks)-1, 0) {
			c.listState.Select(selected + 1)
		}
	case key.IsChar('m'):
		if disk := c.selectedDisk(); disk != nil && !disk.IsMounted {
			c.pendingAction = &msg.DiskAction{Kind: msg.DiskMount, Target: disk.DevicePath}
			c.showConfirm = true
		}
	case key.IsChar('u'):
		if disk := c.selectedDisk(); disk != nil && disk.IsMounted && disk.HasMountPoint {
			c.pendingAction = &msg.DiskAction{Kind: msg.DiskUnmount, Target: disk.MountPoint}
			c.showConfirm = true
		}
	case key.IsChar('f'):
		if disk := c.selectedDisk(); disk != nil {
			c.pendingAction = &msg.DiskAction{Kind: msg.DiskCheckFilesystem, Target: disk.DevicePath}
			c.showConfirm = true
		}
	case key.Code == tui.KeyEnter:
		if c.mode == modeOverview {
			c.mode = modeDetails
		} else {
			c.mode = modeOverview
		}
	case key.IsF(5):
		c.loadDiskInfo()
		c.statusMessage = &status{"Disk info refreshed", false}
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(6), tui.Min(10), tui.Length(3))

	c.renderSummary(f, chunks[0])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(64), tui.Percentage(36))

	switch c.mode {
	case modeOverview:
		c.renderList(f, content[0])
		c.renderSelectedDisk(f, content[1])
	case modeDetails:
		if disk := c.selectedDisk(); disk != nil {
			c.renderDetails(f, content[0], disk)
		} else {
			c.renderList(f, content[0])
		}
		c.renderSelectedDisk(f, content[1])
	}

	// Status bar
	var statusContent tui.Line
	if c.showConfirm {
		actionDesc := "Confirm action?"
		if a := c.pendingAction; a != nil {
			switch a.Kind {
			case msg.DiskMount:
				actionDesc = fmt.Sprintf("Mount %s?", a.Target)
			case msg.DiskUnmount:
				actionDesc = fmt.Sprintf("Unmount %s?", a.Target)
			case msg.DiskCheckFilesystem:
				actionDesc = fmt.Sprintf("Check %s?", a.Target)
			}
		}
		statusContent = tui.LineFrom(
			tui.Styled(actionDesc, ui.WarningStyle()),
			tui.Raw(" [Y]es / [N]o"),
		)
	} else if c.statusMessage != nil {
		style := ui.SuccessStyle()
		if c.statusMessage.isError {
			style = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.statusMessage.text, style))
	} else if disk := c.selectedDisk(); disk != nil {
		statusContent = tui.LineFrom(
			tui.Styled("Device: ", ui.MutedStyle()),
			tui.Raw(disk.DevicePath),
		)
	} else {
		statusContent = tui.LineSpan(tui.Styled("Select a disk", ui.MutedStyle()))
	}

	statusPara := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(statusPara, chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{
		{Key: "m", Desc: "Mount"},
		{Key: "u", Desc: "Unmount"},
		{Key: "f", Desc: "Check FS"},
		{Key: "Enter", Desc: "Details"},
		{Key: "F5", Desc: "Refresh"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.loadDiskInfo() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}

func (c *Component) renderSummary(f *tui.Frame, area tui.Rect) {
	block := ui.Panel(ui.PanelTitle("Disk management"))

	inner := block.Inner(area)
	f.RenderWidget(block, area)

	// Calculate totals
	var totalSize, totalUsed uint64
	mountedCount, unmountedCount := 0, 0
	for i := range c.disks {
		d := &c.disks[i]
		if d.IsMounted {
			totalSize += d.Size
			totalUsed += d.Used
			mountedCount++
		} else {
			unmountedCount++
		}
	}

	overallPercent := 0
	if totalSize > 0 {
		overallPercent = int(uint16(min(max((float64(totalUsed)/float64(totalSize))*100.0, 0), 65535)))
	}

	chunks := tui.Split(inner, tui.Horizontal, tui.Percentage(38), tui.Percentage(62))

	modeText := " DETAILS "
	if c.mode == modeOverview {
		modeText = " OVERVIEW "
	}
	stats := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Mounted ", ui.LabelStyle()),
			tui.Styled(fmt.Sprintf(" %d ", mountedCount), ui.BadgeSuccess()),
			tui.Styled("  Unmounted ", ui.LabelStyle()),
			tui.Styled(fmt.Sprintf(" %d ", unmountedCount), ui.BadgeWarning()),
		),
		tui.LineFrom(
			tui.Styled("Mode ", ui.LabelStyle()),
			tui.Styled(modeText, ui.BadgeNeutral()),
		),
		tui.LineSpan(tui.Styled(
			"Inspect mount state and confirm mutating actions before execution.",
			ui.SubtitleStyle(),
		)),
	)
	f.RenderWidget(stats, chunks[0])

	// `{:.1}` on an integer ignores the precision in Rust.
	gauge := tui.NewGauge().
		Block(ui.PanelAltBlock(ui.PanelTitle("Total usage"))).
		GaugeStyle(tui.NewStyle().FG(usageColor(uint8(overallPercent)))).
		Percent(overallPercent).
		LabelStr(fmt.Sprintf("%s / %s (%d%%)", formatSize(totalUsed), formatSize(totalSize), overallPercent))
	f.RenderWidget(gauge, chunks[1])
}

func (c *Component) renderList(f *tui.Frame, area tui.Rect) {
	items := make([]tui.ListItem, 0, len(c.disks))
	for i := range c.disks {
		disk := &c.disks[i]
		mountStr := disk.mountOr("-")

		st := tui.Styled("○", ui.MutedStyle())
		if disk.IsMounted {
			st = tui.Styled("●", ui.SuccessStyle())
		}

		usage := tui.Raw("     ")
		if disk.IsMounted {
			usage = tui.Styled(fmt.Sprintf(" %3d%%", disk.UsePercent), tui.NewStyle().FG(usageColor(disk.UsePercent)))
		}

		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				st,
				tui.Raw(" "),
				tui.Styled(fmt.Sprintf("%-12s", disk.Name), ui.TitleStyle()),
				tui.Styled(fmt.Sprintf("%-10s", disk.Filesystem), ui.AccentStyle()),
				tui.Styled(fmt.Sprintf("%10s", formatSize(disk.Size)), ui.WarningStyle()),
				usage,
			),
			tui.LineFrom(
				tui.Styled("  Mount ", ui.MutedStyle()),
				tui.Raw(mountStr),
			),
		))
	}

	list := tui.NewList(items).
		Block(ui.PanelAltBlock(ui.PanelTitle(fmt.Sprintf("Partitions (%d)", len(c.disks))))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")

	state := c.listState
	list.RenderStateful(area, f.Buffer(), &state)
}

func (c *Component) renderDetails(f *tui.Frame, area tui.Rect, disk *diskInfo) {
	block := ui.Panel(ui.PanelTitle(fmt.Sprintf("%s details", disk.Name)))

	inner := block.Inner(area)
	f.RenderWidget(block, area)

	chunks := tui.Split(inner, tui.Vertical, tui.Length(8), tui.Min(3))

	// Info
	info := []tui.Line{
		tui.LineSpan(tui.Styled("Inventory", ui.EyebrowStyle())),
		tui.LineFrom(
			tui.Styled("Device     ", ui.LabelStyle()),
			tui.Raw(disk.DevicePath),
		),
		tui.LineFrom(
			tui.Styled("Filesystem ", ui.LabelStyle()),
			tui.Raw(disk.Filesystem),
		),
		tui.LineFrom(
			tui.Styled("Mount      ", ui.LabelStyle()),
			tui.Raw(disk.mountOr("Not mounted")),
		),
		tui.LineStr(""),
		tui.LineSpan(tui.Styled("Capacity", ui.EyebrowStyle())),
		tui.LineFrom(
			tui.Styled("Size       ", ui.LabelStyle()),
			tui.Raw(formatSize(disk.Size)),
		),
		tui.LineFrom(
			tui.Styled("Used       ", ui.LabelStyle()),
			tui.Raw(formatSize(disk.Used)),
		),
		tui.LineFrom(
			tui.Styled("Available  ", ui.LabelStyle()),
			tui.Raw(formatSize(disk.Available)),
		),
	}

	f.RenderWidget(tui.ParagraphLines(info...), chunks[0])

	// Usage gauge
	if disk.IsMounted {
		gauge := tui.NewGauge().
			Block(ui.PanelAltBlock(ui.PanelTitle("Usage"))).
			GaugeStyle(tui.NewStyle().FG(usageColor(disk.UsePercent))).
			Percent(int(disk.UsePercent)).
			LabelStr(fmt.Sprintf("%d%%", disk.UsePercent))
		f.RenderWidget(gauge, chunks[1])
	}
}
