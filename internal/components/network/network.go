// Package network implements the network configuration tab.
package network

import (
	"errors"
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

// networkInterface describes one network interface.
type networkInterface struct {
	name       string
	ipAddress  string
	netmask    string
	gateway    string
	useDHCP    bool
	isUp       bool
	macAddress string
}

type networkMode uint8

const (
	modeOverview networkMode = iota
	modeDNS
)

// Component is the network configuration view.
type Component struct {
	interfaces     []networkInterface
	listState      tui.ListState
	mode           networkMode
	dnsServers     []string
	defaultGateway string
	hostname       string
	statusMessage  string
	statusIsError  bool
	hasStatus      bool
	showConfirm    bool
}

// New creates the component.
func New() *Component {
	c := &Component{mode: modeOverview, defaultGateway: "Not set"}
	c.loadNetworkInfo()
	if len(c.interfaces) > 0 {
		c.listState.Select(0)
	}
	return c
}

func (c *Component) loadNetworkInfo() {
	c.interfaces = c.interfaces[:0]
	c.loadInterfaces()
	c.loadDNS()
	c.loadDefaultGateway()
	c.loadHostname()
}

// readToString mirrors fs::read_to_string: it fails on invalid UTF-8.
func readToString(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(data) {
		return "", false
	}
	return string(data), true
}

// commandOutput mirrors std::process::Command::output: the command's stdout
// is available whenever it could be spawned, whatever its exit status.
func commandOutput(name string, args ...string) (string, bool) {
	out, err := exec.Command(name, args...).Output()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return "", false
	}
	return strings.ToValidUTF8(string(out), "�"), true
}

// trimStartMatches mirrors Rust's str::trim_start_matches with a string
// pattern.
func trimStartMatches(s, prefix string) string {
	if prefix == "" {
		return s
	}
	for strings.HasPrefix(s, prefix) {
		s = s[len(prefix):]
	}
	return s
}

func (c *Component) loadInterfaces() {
	// Read from /sys/class/net for interface list
	if f, err := os.Open("/sys/class/net"); err == nil {
		entries, _ := f.ReadDir(-1)
		f.Close()
		for _, entry := range entries {
			name := strings.ToValidUTF8(entry.Name(), "�")

			// Skip loopback
			if name == "lo" {
				continue
			}

			iface := networkInterface{name: name, useDHCP: true}

			// Read MAC address
			if mac, ok := readToString(fmt.Sprintf("/sys/class/net/%s/address", name)); ok {
				iface.macAddress = strings.TrimSpace(mac)
			}

			// Check if interface is up
			if flags, ok := readToString(fmt.Sprintf("/sys/class/net/%s/flags", name)); ok {
				if v, ok := parseHexU32(trimStartMatches(strings.TrimSpace(flags), "0x")); ok {
					iface.isUp = v&1 != 0 // IFF_UP
				}
			}

			// Try to get IP address using ip command
			if stdout, ok := commandOutput("ip", "addr", "show", name); ok {
				for _, line := range tui.RustLines(stdout) {
					line = strings.TrimSpace(line)
					if !strings.HasPrefix(line, "inet ") {
						continue
					}
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						if ip, cidr, ok := strings.Cut(parts[1], "/"); ok {
							iface.ipAddress = ip
							iface.netmask = cidrToNetmask(cidr)
						}
					}
				}
			}

			// Read from rc.inet1.conf for static config
			if config, ok := readToString("/etc/rc.d/rc.inet1.conf"); ok {
				iface.useDHCP = isDHCPEnabled(config, name)
				if !iface.useDHCP {
					if gw, ok := getConfigValue(config, "GATEWAY"); ok {
						iface.gateway = gw
					}
				}
			}

			c.interfaces = append(c.interfaces, iface)
		}
	}
	sort.SliceStable(c.interfaces, func(i, j int) bool { return c.interfaces[i].name < c.interfaces[j].name })
}

// parseHexU32 mirrors u32::from_str_radix(s, 16), which accepts an optional
// leading '+'.
func parseHexU32(s string) (uint32, bool) {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "+"), 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// cidrToNetmask converts a prefix length to a dotted netmask; unparsable
// input defaults to /24.
func cidrToNetmask(cidr string) string {
	bits := uint32(24)
	if v, err := strconv.ParseUint(strings.TrimPrefix(cidr, "+"), 10, 32); err == nil {
		bits = uint32(v)
	}
	var mask uint32
	if bits != 0 {
		// The shift amount wraps like the release build of the original.
		mask = ^uint32(0) << ((32 - bits) & 31)
	}
	return fmt.Sprintf("%d.%d.%d.%d", (mask>>24)&255, (mask>>16)&255, (mask>>8)&255, mask&255)
}

func isDHCPEnabled(config, iface string) bool {
	// Check for USE_DHCP[n]="yes" where n is interface number
	ifaceNum := "0"
	if strings.HasPrefix(iface, "eth") {
		ifaceNum = trimStartMatches(iface, "eth")
	}
	prefix := fmt.Sprintf("USE_DHCP[%s]", ifaceNum)
	for _, line := range tui.RustLines(config) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.Contains(line, "yes") || strings.Contains(line, "YES")
		}
	}
	return true // Default to DHCP
}

func getConfigValue(config, key string) (string, bool) {
	for _, line := range tui.RustLines(config) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) && strings.Contains(line, "=") {
			parts := strings.Split(line, "=")
			if len(parts) < 2 {
				return "", false
			}
			return strings.Trim(strings.TrimSpace(parts[1]), `"`), true
		}
	}
	return "", false
}

func (c *Component) loadDNS() {
	c.dnsServers = c.dnsServers[:0]
	if content, ok := readToString("/etc/resolv.conf"); ok {
		for _, line := range tui.RustLines(content) {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "nameserver ") {
				server := strings.TrimSpace(trimStartMatches(line, "nameserver "))
				c.dnsServers = append(c.dnsServers, server)
			}
		}
	}
}

func (c *Component) loadHostname() {
	if hostname, ok := readToString("/etc/HOSTNAME"); ok {
		c.hostname = strings.TrimSpace(hostname)
	} else if hostname, ok := readToString("/etc/hostname"); ok {
		c.hostname = strings.TrimSpace(hostname)
	}
}

func (c *Component) loadDefaultGateway() {
	c.defaultGateway = "Not set"
	stdout, ok := commandOutput("ip", "route", "show", "default")
	if !ok {
		return
	}
	lines := tui.RustLines(stdout)
	if len(lines) == 0 {
		return
	}
	words := strings.Fields(lines[0])
	for i, w := range words {
		if w == "via" {
			if i+1 < len(words) {
				c.defaultGateway = words[i+1]
			}
			return
		}
	}
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.statusMessage, c.statusIsError, c.hasStatus = message, isError, true
}

// RestartStarted records that a network restart began.
func (c *Component) RestartStarted() {
	c.statusMessage, c.statusIsError, c.hasStatus = "Restarting network...", false, true
}

// Refresh reloads network state.
func (c *Component) Refresh() { c.loadNetworkInfo() }

func (c *Component) currentLen() int {
	if c.mode == modeOverview {
		return len(c.interfaces)
	}
	return len(c.dnsServers)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.showConfirm {
		switch {
		case key.IsChar('y') || key.IsChar('Y'):
			c.showConfirm = false
			return msg.RestartNetwork{}
		case key.IsChar('n') || key.IsChar('N') || key.Code == tui.KeyEsc:
			c.showConfirm = false
		}
		return nil
	}

	switch {
	case key.Code == tui.KeyTab:
		if c.mode == modeOverview {
			c.mode = modeDNS
		} else {
			c.mode = modeOverview
		}
	case key.Code == tui.KeyUp || key.IsChar('k'):
		n := c.currentLen()
		if selected, ok := c.listState.Selected(); ok {
			if selected > 0 {
				c.listState.Select(selected - 1)
			}
		} else if n > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyDown || key.IsChar('j'):
		n := c.currentLen()
		if selected, ok := c.listState.Selected(); ok {
			if selected < max(n-1, 0) {
				c.listState.Select(selected + 1)
			}
		} else if n > 0 {
			c.listState.Select(0)
		}
	case key.IsChar('r'):
		c.showConfirm = true
	case key.IsF(5):
		c.loadNetworkInfo()
		c.statusMessage, c.statusIsError, c.hasStatus = "Network info refreshed", false, true
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(3), tui.Min(10), tui.Length(8), tui.Length(3))

	// Mode bar
	modeText := "[Interfaces]  DNS"
	if c.mode == modeDNS {
		modeText = " Interfaces  [DNS]"
	}
	modeBar := tui.ParagraphLine(tui.LineFrom(
		tui.Styled("View ", ui.LabelStyle()),
		tui.Raw(modeText),
		tui.Raw(" "),
		tui.Styled(" HOST ", ui.BadgeNeutral()),
		tui.Raw(" "),
		tui.Styled(c.hostname, ui.SubtitleStyle()),
	)).Block(ui.Panel(ui.PanelTitle("Network fabric")))
	f.RenderWidget(modeBar, chunks[0])

	// Main content
	if c.mode == modeOverview {
		c.renderInterfaces(f, chunks[1])
	} else {
		c.renderDNS(f, chunks[1])
	}

	// Info panel
	c.renderInfo(f, chunks[2])

	// Status bar
	var statusContent tui.Line
	switch {
	case c.showConfirm:
		statusContent = tui.LineFrom(
			tui.Styled("Restart network? ", ui.WarningStyle()),
			tui.Raw("[Y]es / [N]o"),
		)
	case c.hasStatus:
		style := ui.SuccessStyle()
		if c.statusIsError {
			style = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.statusMessage, style))
	default:
		statusContent = tui.LineSpan(tui.Styled("Press 'r' to restart network", ui.MutedStyle()))
	}
	status := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(status, chunks[3])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{
		{Key: "Tab", Desc: "Switch View"},
		{Key: "r", Desc: "Restart Network"},
		{Key: "F5", Desc: "Refresh"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.loadNetworkInfo() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}

func (c *Component) selectedInterface() *networkInterface {
	if i, ok := c.listState.Selected(); ok && i < len(c.interfaces) {
		return &c.interfaces[i]
	}
	return nil
}

func (c *Component) selectedDNS() (string, bool) {
	if i, ok := c.listState.Selected(); ok && i < len(c.dnsServers) {
		return c.dnsServers[i], true
	}
	return "", false
}

func linkBadge(up bool) tui.Span {
	if up {
		return tui.Styled(" UP ", ui.BadgeSuccess())
	}
	return tui.Styled(" DOWN ", ui.BadgeWarning())
}

func (c *Component) renderInterfaces(f *tui.Frame, area tui.Rect) {
	content := tui.Split(area, tui.Horizontal, tui.Percentage(62), tui.Percentage(38))

	items := make([]tui.ListItem, 0, len(c.interfaces))
	for _, iface := range c.interfaces {
		dhcp := "Static"
		if iface.useDHCP {
			dhcp = "DHCP"
		}
		address := "Not assigned"
		if iface.ipAddress != "" {
			address = fmt.Sprintf("%s/%s", iface.ipAddress, iface.netmask)
		}
		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				tui.Styled(fmt.Sprintf("%-12s", iface.name), ui.TitleStyle()),
				linkBadge(iface.isUp),
				tui.Raw(" "),
				tui.Styled(fmt.Sprintf(" %s ", dhcp), ui.BadgeNeutral()),
				tui.Raw(" "+iface.macAddress),
			),
			tui.LineFrom(
				tui.Styled("    Address ", ui.MutedStyle()),
				tui.Raw(address),
			),
		))
	}

	list := tui.NewList(items).
		Block(ui.Panel(ui.PanelTitle("Interfaces"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")
	state := c.listState
	list.RenderStateful(content[0], f.Buffer(), &state)

	var inspectorLines []tui.Line
	if iface := c.selectedInterface(); iface != nil {
		address := "not assigned"
		if iface.ipAddress != "" {
			address = fmt.Sprintf("%s/%s", iface.ipAddress, iface.netmask)
		}
		mode := " STATIC "
		if iface.useDHCP {
			mode = " DHCP "
		}
		gateway := iface.gateway
		if gateway == "" {
			gateway = c.defaultGateway
		}
		inspectorLines = []tui.Line{
			tui.LineFrom(
				tui.Styled("INTERFACE", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(iface.name, ui.TitleStyle()),
			),
			tui.LineStr(""),
			tui.LineFrom(tui.Styled("Link ", ui.LabelStyle()), linkBadge(iface.isUp)),
			tui.LineFrom(tui.Styled("Address ", ui.LabelStyle()), tui.Raw(address)),
			tui.LineFrom(tui.Styled("Mode ", ui.LabelStyle()), tui.Styled(mode, ui.BadgeNeutral())),
			tui.LineFrom(tui.Styled("Gateway ", ui.LabelStyle()), tui.Raw(gateway)),
			tui.LineFrom(tui.Styled("MAC ", ui.LabelStyle()), tui.Raw(iface.macAddress)),
		}
	} else {
		inspectorLines = []tui.Line{
			tui.LineSpan(tui.Styled("No interface selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Choose an interface to inspect link state and addressing."),
		}
	}
	f.RenderWidget(
		tui.ParagraphLines(inspectorLines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
		content[1],
	)
}

func (c *Component) renderDNS(f *tui.Frame, area tui.Rect) {
	content := tui.Split(area, tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	items := make([]tui.ListItem, 0, len(c.dnsServers))
	for i, server := range c.dnsServers {
		items = append(items, tui.ListItemLine(tui.LineFrom(
			tui.Styled(fmt.Sprintf("DNS %d ", i+1), ui.AccentStyle()),
			tui.Raw(server),
		)))
	}
	if len(items) == 0 {
		items = []tui.ListItem{tui.ListItemSpan(tui.Styled("No DNS servers configured", ui.MutedStyle()))}
	}
	list := tui.NewList(items).
		Block(ui.Panel(ui.PanelTitle("DNS servers"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")
	state := c.listState
	list.RenderStateful(content[0], f.Buffer(), &state)

	var inspectorLines []tui.Line
	if server, ok := c.selectedDNS(); ok {
		order := "n/a"
		if idx, ok := c.listState.Selected(); ok {
			order = fmt.Sprintf("#%d", idx+1)
		}
		inspectorLines = []tui.Line{
			tui.LineFrom(
				tui.Styled("RESOLVER", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(server, ui.TitleStyle()),
			),
			tui.LineStr(""),
			tui.LineFrom(tui.Styled("Order ", ui.LabelStyle()), tui.Raw(order)),
			tui.LineFrom(tui.Styled("Source ", ui.LabelStyle()), tui.Raw("/etc/resolv.conf")),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled(
				"Higher entries are consulted first during name resolution.",
				ui.SubtitleStyle(),
			)),
		}
	} else {
		inspectorLines = []tui.Line{
			tui.LineSpan(tui.Styled("No DNS server selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Select a resolver entry to inspect its order and source."),
		}
	}
	f.RenderWidget(
		tui.ParagraphLines(inspectorLines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
		content[1],
	)
}

func (c *Component) renderInfo(f *tui.Frame, area tui.Rect) {
	sections := tui.Split(area, tui.Horizontal, tui.Percentage(54), tui.Percentage(46))

	routeInfo := []tui.Line{
		tui.LineFrom(tui.Styled("Gateway ", ui.LabelStyle()), tui.Styled(c.defaultGateway, ui.BadgeNeutral())),
		tui.LineFrom(tui.Styled("Hostname ", ui.LabelStyle()), tui.Raw(c.hostname)),
		tui.LineFrom(
			tui.Styled("Interfaces ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.interfaces)), ui.BadgeSuccess()),
		),
	}
	f.RenderWidget(
		tui.ParagraphLines(routeInfo...).Block(ui.PanelAltBlock(ui.PanelTitle("Route summary"))),
		sections[0],
	)

	configInfo := []tui.Line{
		tui.LineFrom(
			tui.Styled("Resolver count ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.dnsServers)), ui.BadgeNeutral()),
		),
		tui.LineFrom(tui.Styled("Config ", ui.LabelStyle()), tui.Raw("/etc/rc.d/rc.inet1.conf")),
		tui.LineSpan(tui.Styled("Use Tab to switch between interface and DNS views.", ui.SubtitleStyle())),
	}
	f.RenderWidget(
		tui.ParagraphLines(configInfo...).Block(ui.PanelAltBlock(ui.PanelTitle("Control plane"))),
		sections[1],
	)
}
