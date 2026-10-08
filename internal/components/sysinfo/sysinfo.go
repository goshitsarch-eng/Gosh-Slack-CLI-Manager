// Package sysinfo implements the system information dashboard tab.
package sysinfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// Component is the system information dashboard.
type Component struct {
	cpus            cpusWrapper
	memory          memInfo
	diskCount       int
	networks        []netData
	lastRefresh     time.Time
	selectedSection int
	scrollOffset    int
}

// New creates the component.
func New() *Component {
	c := &Component{}
	c.memory.refresh()
	c.cpus.refresh()
	c.diskCount = countDisks()
	c.networks = listNetworks()
	c.lastRefresh = time.Now()
	return c
}

// refresh re-reads CPU, memory and network counters. CPU times are only
// re-read when the previous read is older than the crate's minimum interval.
func (c *Component) refresh() {
	c.cpus.refresh()
	c.memory.refresh()
	// Disks::refresh only updates available space, which is not displayed.
	refreshNetworks(c.networks)
	c.lastRefresh = time.Now()
}

func formatBytes(bytes uint64) string {
	const (
		kb = uint64(1024)
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatUptime(seconds uint64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// asU16 mirrors Rust's saturating `f as u16` cast.
func asU16(f float64) int {
	if math.IsNaN(f) || f <= 0 {
		return 0
	}
	if f >= math.MaxUint16 {
		return math.MaxUint16
	}
	return int(f)
}

func usageColor(percent int) tui.Color {
	switch {
	case percent >= 90:
		return tui.Red
	case percent >= 70:
		return tui.Yellow
	default:
		return tui.Green
	}
}

func (c *Component) panel(section int, title string) tui.Block {
	if c.selectedSection == section {
		return ui.PanelFocused(ui.PanelTitle(title))
	}
	return ui.Panel(ui.PanelTitle(title))
}

func orUnknown(s string, ok bool) string {
	if !ok {
		return "Unknown"
	}
	return s
}

func (c *Component) renderSystemInfo(f *tui.Frame, area tui.Rect) {
	block := c.panel(0, "System information")
	inner := block.Inner(area)
	f.RenderWidget(block, area)

	name := orUnknown(osName())
	version, _ := osVersion()
	infoLines := []tui.Line{
		tui.LineFrom(tui.Styled("Hostname ", ui.LabelStyle()), tui.Raw(orUnknown(hostName()))),
		tui.LineFrom(tui.Styled("OS       ", ui.LabelStyle()), tui.Raw(name+" "+version)),
		tui.LineFrom(tui.Styled("Kernel   ", ui.LabelStyle()), tui.Raw(orUnknown(kernelVersion()))),
		tui.LineFrom(tui.Styled("Uptime   ", ui.LabelStyle()), tui.Raw(formatUptime(uptime()))),
		tui.LineFrom(tui.Styled("Arch     ", ui.LabelStyle()), tui.Raw(orUnknown(cpuArch()))),
	}
	f.RenderWidget(tui.ParagraphLines(infoLines...), inner)
}

func (c *Component) renderCPU(f *tui.Frame, area tui.Rect) {
	block := c.panel(1, "CPU")
	inner := block.Inner(area)
	f.RenderWidget(block, area)

	cpus := c.cpus.cpus
	var overall float32
	if len(cpus) > 0 {
		var sum float32
		for _, cpu := range cpus {
			sum += cpu.percent
		}
		overall = sum / float32(len(cpus))
	}

	chunks := tui.Split(inner, tui.Vertical, tui.Length(2), tui.Min(1))

	gauge := tui.NewGauge().
		Block(tui.NewBlock()).
		GaugeStyle(tui.NewStyle().FG(usageColor(asU16(float64(overall))))).
		Percent(asU16(float64(overall))).
		LabelStr(fmt.Sprintf("Overall: %.1f%%", overall))
	f.RenderWidget(gauge, chunks[0])

	var items []tui.ListItem
	for i := c.scrollOffset; i < len(cpus) && len(items) < chunks[1].Height; i++ {
		usage := cpus[i].percent
		const barWidth = 20
		filled := int(asU16(float64(usage / 100.0 * barWidth)))
		bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
		items = append(items, tui.ListItemLine(tui.LineFrom(
			tui.Styled(fmt.Sprintf("CPU%2d ", i), ui.LabelStyle()),
			tui.Styled(bar, tui.NewStyle().FG(usageColor(asU16(float64(usage))))),
			tui.Raw(fmt.Sprintf(" %5.1f%%", usage)),
		)))
	}
	f.RenderWidget(tui.NewList(items), chunks[1])
}

func (c *Component) renderRuntimeSummary(f *tui.Frame, area tui.Rect) {
	lines := []tui.Line{
		tui.LineFrom(
			tui.Styled("Uptime ", ui.LabelStyle()),
			tui.Styled(formatUptime(uptime()), ui.BadgeNeutral()),
			tui.Raw(" "),
			tui.Styled("CPUs ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.cpus.cpus)), ui.BadgeSuccess()),
		),
		tui.LineFrom(
			tui.Styled("Disks ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(c.diskCount), ui.BadgeNeutral()),
			tui.Raw(" "),
			tui.Styled("NICs ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.networks)), ui.BadgeNeutral()),
		),
		tui.LineStr(""),
		tui.LineSpan(tui.Styled("Cycle sections with Tab and refresh snapshots with r.", ui.SubtitleStyle())),
	}
	panel := tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(panel, area)
}

func (c *Component) renderMemory(f *tui.Frame, area tui.Rect) {
	block := c.panel(2, "Memory")
	inner := block.Inner(area)
	f.RenderWidget(block, area)

	totalMem := c.memory.total
	usedMem := c.memory.usedMemory()
	totalSwap := c.memory.swapTotal
	usedSwap := c.memory.usedSwap()

	memPercent := 0
	if totalMem > 0 {
		memPercent = asU16(float64(usedMem) / float64(totalMem) * 100.0)
	}
	swapPercent := 0
	if totalSwap > 0 {
		swapPercent = asU16(float64(usedSwap) / float64(totalSwap) * 100.0)
	}

	chunks := tui.Split(inner, tui.Vertical, tui.Length(3), tui.Length(3))

	// Rust formats the integer percentages with `{:.1}`, which integers ignore.
	ramGauge := tui.NewGauge().
		Block(tui.NewBlock().TitleStr("RAM")).
		GaugeStyle(tui.NewStyle().FG(usageColor(memPercent))).
		Percent(memPercent).
		LabelStr(fmt.Sprintf("%s / %s (%d%%)", formatBytes(usedMem), formatBytes(totalMem), memPercent))
	f.RenderWidget(ramGauge, chunks[0])

	swapGauge := tui.NewGauge().
		Block(tui.NewBlock().TitleStr("Swap")).
		GaugeStyle(tui.NewStyle().FG(usageColor(swapPercent))).
		Percent(swapPercent).
		LabelStr(fmt.Sprintf("%s / %s (%d%%)", formatBytes(usedSwap), formatBytes(totalSwap), swapPercent))
	f.RenderWidget(swapGauge, chunks[1])
}

func (c *Component) renderNetwork(f *tui.Frame, area tui.Rect) {
	block := c.panel(3, "Network")
	inner := block.Inner(area)
	f.RenderWidget(block, area)

	items := make([]tui.ListItem, 0, len(c.networks))
	for _, n := range c.networks {
		items = append(items, tui.ListItemLines(
			tui.LineFrom(tui.Styled(n.name+": ", tui.NewStyle().FG(tui.Cyan).Add(tui.Bold))),
			tui.LineFrom(
				tui.Styled("  ↓ ", tui.NewStyle().FG(tui.Green)),
				tui.Raw(formatBytes(n.received())+"/s  "),
				tui.Styled("↑ ", tui.NewStyle().FG(tui.Red)),
				tui.Raw(formatBytes(n.transmitted())+"/s"),
			),
		))
	}
	f.RenderWidget(tui.NewList(items), inner)
}

func (c *Component) renderProcesses(f *tui.Frame, area tui.Rect) {
	block := c.panel(4, "Top processes")
	inner := block.Inner(area)
	f.RenderWidget(block, area)

	processes := listProcesses()
	// Stable sort by CPU usage, highest first (partial_cmp, NaN as equal).
	sortByCPUDesc(processes)

	var items []tui.ListItem
	for i := 0; i < len(processes) && i < inner.Height; i++ {
		p := processes[i]
		name := []rune(p.name)
		if len(name) > 30 {
			name = name[:30]
		}
		items = append(items, tui.ListItemLine(tui.LineFrom(
			tui.Styled(fmt.Sprintf("%6d ", p.pid), tui.NewStyle().FG(tui.DarkGray)),
			tui.Styled(fmt.Sprintf("%5.1f%% ", p.cpuUsage), tui.NewStyle().FG(usageColor(asU16(float64(p.cpuUsage))))),
			tui.Styled(fmt.Sprintf("%8s ", formatBytes(p.memory)), tui.NewStyle().FG(tui.Yellow)),
			tui.Raw(string(name)),
		)))
	}
	f.RenderWidget(tui.NewList(items), inner)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	switch {
	case key.Code == tui.KeyTab:
		c.selectedSection = (c.selectedSection + 1) % 5
		c.scrollOffset = 0
	case key.Code == tui.KeyBackTab:
		if c.selectedSection == 0 {
			c.selectedSection = 4
		} else {
			c.selectedSection--
		}
		c.scrollOffset = 0
	case key.Code == tui.KeyUp || key.IsChar('k'):
		if c.scrollOffset > 0 {
			c.scrollOffset--
		}
	case key.Code == tui.KeyDown || key.IsChar('j'):
		c.scrollOffset++
	case key.IsChar('r'):
		c.refresh()
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(7), tui.Min(10), tui.Length(10))

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))
	c.renderSystemInfo(f, header[0])
	c.renderRuntimeSummary(f, header[1])

	middle := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(60), tui.Percentage(40))
	c.renderCPU(f, middle[0])
	c.renderMemory(f, middle[1])

	bottom := tui.Split(chunks[2], tui.Horizontal, tui.Percentage(40), tui.Percentage(60))
	c.renderNetwork(f, bottom[0])
	c.renderProcesses(f, bottom[1])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{{Key: "Tab", Desc: "Next Section"}, {Key: "↑/↓", Desc: "Scroll"}, {Key: "r", Desc: "Refresh"}}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.refresh() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
