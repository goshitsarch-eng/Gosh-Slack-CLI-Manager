// Package mirror implements the mirror management tab, which selects the
// active slackpkg mirror.
package mirror

import (
	"fmt"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// Component is the mirror management component.
type Component struct {
	mirrors   []slackware.MirrorEntry
	listState tui.ListState
	version   slackware.Version
	isRunning bool

	hasStatus     bool
	statusMessage string
	statusIsError bool
}

// New creates the component.
func New(version slackware.Version) *Component {
	return &Component{version: version}
}

// LoadMirrors reloads the mirror list.
func (c *Component) LoadMirrors() {
	mirrors, err := slackware.ParseMirrors(c.version.MirrorPath())
	if err != nil {
		c.setStatusOnly(fmt.Sprintf("Failed to load mirrors: %s", err.Error()), true)
		return
	}
	c.mirrors = mirrors
	if len(c.mirrors) > 0 {
		c.listState.Select(0)
	}
	c.hasStatus = false
}

func (c *Component) selectedMirror() (slackware.MirrorEntry, bool) {
	i, ok := c.listState.Selected()
	if !ok || i < 0 || i >= len(c.mirrors) {
		return slackware.MirrorEntry{}, false
	}
	return c.mirrors[i], true
}

func (c *Component) setStatusOnly(message string, isError bool) {
	c.hasStatus, c.statusMessage, c.statusIsError = true, message, isError
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.setStatusOnly(message, isError)
	c.isRunning = false
}

func (c *Component) startUpdate() {
	c.isRunning = true
	c.hasStatus = false
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.isRunning {
		return nil
	}

	switch {
	case key.Code == tui.KeyUp, key.IsChar('k'):
		if selected, ok := c.listState.Selected(); ok && selected > 0 {
			c.listState.Select(selected - 1)
		}
	case key.Code == tui.KeyDown, key.IsChar('j'):
		if selected, ok := c.listState.Selected(); ok {
			if selected < max(len(c.mirrors)-1, 0) {
				c.listState.Select(selected + 1)
			}
		} else if len(c.mirrors) > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyEnter:
		if mirror, ok := c.selectedMirror(); ok {
			c.startUpdate()
			return msg.SetMirror{URL: mirror.URL}
		}
	case key.IsChar('r'), key.IsChar('R'):
		c.LoadMirrors()
	}
	return nil
}

func activeBadge(active bool) tui.Span {
	if active {
		return tui.Styled(" ACTIVE ", ui.BadgeSuccess())
	}
	return tui.Styled(" STANDBY ", ui.BadgeNeutral())
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(4), // Header
		tui.Min(10),   // Mirror list
		tui.Length(3), // Status
	)

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	title := tui.ParagraphLines(
		tui.LineSpan(tui.Styled("Mirror Configuration", ui.TitleStyle())),
		tui.LineSpan(tui.Styled(
			"Choose the active Slackware mirror for your detected release track.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Mirrors")))
	f.RenderWidget(title, header[0])

	activeCount := 0
	for _, m := range c.mirrors {
		if m.IsActive {
			activeCount++
		}
	}
	versionInfo := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Track ", ui.LabelStyle()),
			tui.Styled(c.version.DisplayName(), ui.BadgeNeutral()),
		),
		tui.LineFrom(
			tui.Styled("Active mirrors ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(activeCount), ui.BadgeSuccess()),
			tui.Raw(" "),
			tui.Styled("Candidates ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(len(c.mirrors)), ui.BadgeNeutral()),
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Release track")))
	f.RenderWidget(versionInfo, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(64), tui.Percentage(36))

	items := make([]tui.ListItem, 0, len(c.mirrors))
	for _, m := range c.mirrors {
		urlStyle := ui.ValueStyle()
		if m.IsActive {
			urlStyle = ui.SuccessStyle()
		}
		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				activeBadge(m.IsActive),
				tui.Raw(" "),
				tui.Styled(m.URL, urlStyle),
			),
			tui.LineFrom(
				tui.Styled("  Region ", ui.MutedStyle()),
				tui.Raw(m.Region),
			),
		))
	}

	list := tui.NewList(items).
		Block(ui.Panel(ui.PanelTitle(fmt.Sprintf("Mirrors (%d)", len(c.mirrors))))).
		HighlightStyle(ui.HighlightStyle().Add(tui.Bold)).
		HighlightSymbol("▸ ")

	st := c.listState
	list.RenderStateful(content[0], f.Buffer(), &st)

	var inspectorLines []tui.Line
	if mirror, ok := c.selectedMirror(); ok {
		inspectorLines = []tui.Line{
			tui.LineFrom(
				tui.Styled("SELECTION", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(mirror.Region, ui.TitleStyle()),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("State ", ui.LabelStyle()),
				activeBadge(mirror.IsActive),
			),
			tui.LineFrom(
				tui.Styled("Track ", ui.LabelStyle()),
				tui.Raw(c.version.DisplayName()),
			),
			tui.LineSpan(tui.Styled("URL", ui.EyebrowStyle())),
			tui.LineStr(mirror.URL),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled(
				"Selecting a mirror validates the exact target and then refreshes slackpkg metadata.",
				ui.SubtitleStyle(),
			)),
		}
	} else {
		inspectorLines = []tui.Line{
			tui.LineSpan(tui.Styled("No mirror selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Choose a mirror to inspect its region and activation state."),
		}
	}
	f.RenderWidget(
		tui.ParagraphLines(inspectorLines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
		content[1],
	)

	// Status
	var status tui.Paragraph
	switch {
	case c.hasStatus:
		style := ui.SuccessStyle()
		if c.statusIsError {
			style = ui.ErrorStyle()
		}
		status = tui.ParagraphStr(c.statusMessage).Style(style)
	case c.isRunning:
		status = tui.ParagraphStr("Updating mirror configuration...").Style(ui.WarningStyle())
	default:
		status = tui.ParagraphStr("Press Enter to select mirror, R to refresh list").Style(ui.MutedStyle())
	}
	f.RenderWidget(status.Block(ui.PanelAltBlock(ui.PanelTitle("Status"))), chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{{Key: "↑/↓", Desc: "Navigate"}, {Key: "Enter", Desc: "Select"}, {Key: "R", Desc: "Refresh"}}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.LoadMirrors() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
