// Package services implements the rc.d service manager tab.
package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// rcDPath is the directory holding the init scripts.
const rcDPath = "/etc/rc.d"

// serviceInfo describes one rc.d script.
type serviceInfo struct {
	name        string
	path        string
	isRunning   bool
	isEnabled   bool
	description string
}

func (s serviceInfo) statusDisplay() (string, tui.Color) {
	switch {
	case s.isRunning && s.isEnabled:
		return "● Running", tui.Green
	case s.isRunning:
		return "● Running (disabled)", tui.Yellow
	case s.isEnabled:
		return "○ Stopped", tui.Red
	default:
		return "○ Stopped (disabled)", tui.DarkGray
	}
}

type serviceFilter uint8

const (
	filterAll serviceFilter = iota
	filterRunning
	filterStopped
	filterEnabled
)

// Component is the service manager.
type Component struct {
	services      []serviceInfo
	listState     tui.ListState
	filter        serviceFilter
	statusMessage string
	statusIsError bool
	hasStatus     bool
	showConfirm   bool
	pendingAction *msg.ServiceAction
}

// New creates the component.
func New() *Component {
	c := &Component{filter: filterAll}
	c.LoadServices()
	if len(c.services) > 0 {
		c.listState.Select(0)
	}
	return c
}

// LoadServices reloads the service list.
func (c *Component) LoadServices() {
	c.services = loadServicesFrom(rcDPath)
}

func loadServicesFrom(dir string) []serviceInfo {
	var services []serviceInfo
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			name := strings.ToValidUTF8(entry.Name(), "�")
			path := filepath.Join(dir, entry.Name())

			// Only include rc.* scripts (init scripts)
			if !strings.HasPrefix(name, "rc.") || strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".new") {
				continue
			}
			// Skip rc.M, rc.K, rc.S, etc. (runlevel scripts)
			if len(name) == 4 {
				if r := []rune(name); len(r) > 3 && unicode.IsUpper(r[3]) {
					continue
				}
			}
			// Skip rc.local_shutdown and similar
			if name == "rc.local_shutdown" {
				continue
			}

			isEnabled := false
			if info, err := os.Stat(path); err == nil {
				isEnabled = info.Mode().Perm()&0o111 != 0
			}

			services = append(services, serviceInfo{
				name:        name,
				path:        strings.ToValidUTF8(path, "�"),
				isRunning:   checkIfRunning(name),
				isEnabled:   isEnabled,
				description: serviceDescription(path),
			})
		}
	}
	sort.SliceStable(services, func(i, j int) bool { return services[i].name < services[j].name })
	return services
}

func checkIfRunning(serviceName string) bool {
	// Try to determine if service is running based on common patterns
	daemonName := strings.ReplaceAll(trimStartMatches(serviceName, "rc."), "_", "")

	// Check for PID file
	pidFiles := []string{
		fmt.Sprintf("/var/run/%s.pid", daemonName),
		fmt.Sprintf("/var/run/%s/%s.pid", daemonName, daemonName),
		fmt.Sprintf("/run/%s.pid", daemonName),
	}
	for _, pidFile := range pidFiles {
		if _, err := os.Stat(pidFile); err != nil {
			continue
		}
		data, err := os.ReadFile(pidFile)
		if err != nil || !utf8.Valid(data) {
			continue
		}
		pid, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 32)
		if err != nil {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
			return true
		}
	}

	// Check for common process patterns
	out, err := exec.Command("pgrep", "-x", daemonName).Output()
	if err == nil && len(out) > 0 {
		return true
	}
	return false
}

// trimStartMatches mirrors Rust's str::trim_start_matches with a string
// pattern: every leading repetition is removed.
func trimStartMatches(s, prefix string) string {
	if prefix == "" {
		return s
	}
	for strings.HasPrefix(s, prefix) {
		s = s[len(prefix):]
	}
	return s
}

func serviceDescription(path string) string {
	if data, err := os.ReadFile(path); err == nil && utf8.Valid(data) {
		// Look for description in script comments
		lines := tui.RustLines(string(data))
		if len(lines) > 20 {
			lines = lines[:20]
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#!") {
				desc := strings.TrimSpace(strings.TrimLeft(line, "#"))
				if desc != "" && len(desc) > 10 && !strings.Contains(desc, "/") {
					r := []rune(desc)
					if len(r) > 60 {
						r = r[:60]
					}
					return string(r)
				}
			}
		}
	}
	return "No description available"
}

func (c *Component) filteredServices() []*serviceInfo {
	var out []*serviceInfo
	for i := range c.services {
		s := &c.services[i]
		var keep bool
		switch c.filter {
		case filterAll:
			keep = true
		case filterRunning:
			keep = s.isRunning
		case filterStopped:
			keep = !s.isRunning
		case filterEnabled:
			keep = s.isEnabled
		}
		if keep {
			out = append(out, s)
		}
	}
	return out
}

func (c *Component) selectedService() *serviceInfo {
	filtered := c.filteredServices()
	if i, ok := c.listState.Selected(); ok && i < len(filtered) {
		return filtered[i]
	}
	return nil
}

// ActionStarted records that a service action began.
func (c *Component) ActionStarted(action msg.ServiceAction) {
	var message string
	switch action.Kind {
	case msg.ServiceStart:
		message = fmt.Sprintf("Starting %s...", action.Name)
	case msg.ServiceStop:
		message = fmt.Sprintf("Stopping %s...", action.Name)
	case msg.ServiceRestart:
		message = fmt.Sprintf("Restarting %s...", action.Name)
	case msg.ServiceToggle:
		message = fmt.Sprintf("Toggling %s...", action.Name)
	}
	c.statusMessage, c.statusIsError, c.hasStatus = message, false, true
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.statusMessage, c.statusIsError, c.hasStatus = message, isError, true
}

func (c *Component) renderServiceDetails(f *tui.Frame, area tui.Rect) {
	var lines []tui.Line
	if service := c.selectedService(); service != nil {
		state := tui.Styled(" STOPPED ", ui.BadgeWarning())
		if service.isRunning {
			state = tui.Styled(" RUNNING ", ui.BadgeSuccess())
		}
		enabled := tui.Styled(" NO ", ui.BadgeNeutral())
		if service.isEnabled {
			enabled = tui.Styled(" YES ", ui.BadgeSuccess())
		}
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("SERVICE", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(service.name, ui.TitleStyle()),
			),
			tui.LineFrom(tui.Styled("State ", ui.LabelStyle()), state),
			tui.LineFrom(tui.Styled("Path ", ui.LabelStyle()), tui.Raw(service.path)),
			tui.LineFrom(tui.Styled("Enabled ", ui.LabelStyle()), enabled),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Description", ui.EyebrowStyle())),
			tui.LineStr(service.description),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Actions", ui.EyebrowStyle())),
			tui.LineStr("s start   x stop"),
			tui.LineStr("r restart e toggle exec bit"),
		}
	} else {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled("No service selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Pick a service to inspect its current state."),
		}
	}
	panel := tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector")))
	f.RenderWidget(panel, area)
}

func (c *Component) action(kind msg.ServiceActionKind, name string) *msg.ServiceAction {
	return &msg.ServiceAction{Kind: kind, Name: name}
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.showConfirm {
		switch {
		case key.IsChar('y') || key.IsChar('Y'):
			c.showConfirm = false
			if action := c.pendingAction; action != nil {
				c.pendingAction = nil
				return msg.ServiceActionMsg{Action: *action}
			}
		case key.IsChar('n') || key.IsChar('N') || key.Code == tui.KeyEsc:
			c.showConfirm = false
			c.pendingAction = nil
		}
		return nil
	}

	filteredLen := len(c.filteredServices())

	switch {
	case key.Code == tui.KeyUp || key.IsChar('k'):
		if selected, ok := c.listState.Selected(); ok && selected > 0 {
			c.listState.Select(selected - 1)
		}
	case key.Code == tui.KeyDown || key.IsChar('j'):
		if selected, ok := c.listState.Selected(); ok {
			if selected < max(filteredLen-1, 0) {
				c.listState.Select(selected + 1)
			}
		} else if filteredLen > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyHome:
		if filteredLen > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyEnd:
		if filteredLen > 0 {
			c.listState.Select(filteredLen - 1)
		}
	case key.IsChar('s'):
		if service := c.selectedService(); service != nil {
			c.pendingAction = c.action(msg.ServiceStart, service.name)
			c.showConfirm = true
		}
	case key.IsChar('x'):
		if service := c.selectedService(); service != nil {
			c.pendingAction = c.action(msg.ServiceStop, service.name)
			c.showConfirm = true
		}
	case key.IsChar('r'):
		if service := c.selectedService(); service != nil {
			c.pendingAction = c.action(msg.ServiceRestart, service.name)
			c.showConfirm = true
		}
	case key.IsChar('e'):
		if service := c.selectedService(); service != nil {
			return msg.ServiceActionMsg{Action: msg.ServiceAction{Kind: msg.ServiceToggle, Name: service.name}}
		}
	case key.Code == tui.KeyTab:
		switch c.filter {
		case filterAll:
			c.filter = filterRunning
		case filterRunning:
			c.filter = filterStopped
		case filterStopped:
			c.filter = filterEnabled
		case filterEnabled:
			c.filter = filterAll
		}
		c.listState.Select(0)
	case key.IsF(5):
		c.LoadServices()
		c.statusMessage, c.statusIsError, c.hasStatus = "Services refreshed", false, true
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(4), tui.Min(10), tui.Length(3))
	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(64), tui.Percentage(36))

	var filterText string
	switch c.filter {
	case filterAll:
		filterText = "[All]  Running  Stopped  Enabled"
	case filterRunning:
		filterText = " All  [Running]  Stopped  Enabled"
	case filterStopped:
		filterText = " All   Running  [Stopped]  Enabled"
	case filterEnabled:
		filterText = " All   Running   Stopped  [Enabled]"
	}
	filterBar := tui.ParagraphLines(
		tui.LineFrom(tui.Styled("Filter ", ui.LabelStyle()), tui.Raw(filterText)),
		tui.LineSpan(tui.Styled(
			fmt.Sprintf("%d services visible in current filter", len(c.filteredServices())),
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Services")))
	f.RenderWidget(filterBar, header[0])

	running, enabled := 0, 0
	for _, s := range c.services {
		if s.isRunning {
			running++
		}
		if s.isEnabled {
			enabled++
		}
	}
	summary := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Running ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(running), ui.BadgeSuccess()),
			tui.Raw(" "),
			tui.Styled("Enabled ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(enabled), ui.BadgeNeutral()),
		),
		tui.LineSpan(tui.Styled(
			"Use filter tabs to narrow the roster before taking action.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(summary, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(64), tui.Percentage(36))

	filtered := c.filteredServices()
	items := make([]tui.ListItem, 0, len(filtered))
	for _, service := range filtered {
		status, color := service.statusDisplay()
		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				tui.Styled(fmt.Sprintf("%-20s", service.name), ui.TitleStyle()),
				tui.Styled(fmt.Sprintf(" %-20s", status), tui.NewStyle().FG(color)),
			),
			tui.LineFrom(tui.Styled("  "+service.description, ui.MutedStyle())),
		))
	}
	list := tui.NewList(items).
		Block(ui.PanelAltBlock(ui.PanelTitle("Service roster"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")
	state := c.listState
	list.RenderStateful(content[0], f.Buffer(), &state)
	c.renderServiceDetails(f, content[1])

	var statusContent tui.Line
	switch {
	case c.showConfirm:
		actionDesc := "Confirm action?"
		if a := c.pendingAction; a != nil {
			switch a.Kind {
			case msg.ServiceStart:
				actionDesc = fmt.Sprintf("Start %s?", a.Name)
			case msg.ServiceStop:
				actionDesc = fmt.Sprintf("Stop %s?", a.Name)
			case msg.ServiceRestart:
				actionDesc = fmt.Sprintf("Restart %s?", a.Name)
			case msg.ServiceToggle:
				actionDesc = fmt.Sprintf("Toggle %s?", a.Name)
			}
		}
		statusContent = tui.LineFrom(
			tui.Styled(actionDesc, ui.WarningStyle()),
			tui.Raw(" [Y]es / [N]o"),
		)
	case c.hasStatus:
		style := ui.SuccessStyle()
		if c.statusIsError {
			style = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.statusMessage, style))
	default:
		if service := c.selectedService(); service != nil {
			statusContent = tui.LineFrom(tui.Styled("Path ", ui.LabelStyle()), tui.Raw(service.path))
		} else {
			statusContent = tui.LineSpan(tui.Styled("Select a service", ui.MutedStyle()))
		}
	}
	status := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(status, chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{
		{Key: "s", Desc: "Start"},
		{Key: "x", Desc: "Stop"},
		{Key: "r", Desc: "Restart"},
		{Key: "e", Desc: "Enable/Disable"},
		{Key: "Tab", Desc: "Filter"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.LoadServices() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
