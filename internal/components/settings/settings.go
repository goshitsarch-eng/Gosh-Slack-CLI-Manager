// Package settings implements the settings tab: theme, behavior and display
// preferences, saved through the prefs package.
package settings

import (
	"fmt"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/prefs"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// section is a settings page.
type section uint8

const (
	sectionTheme section = iota
	sectionBehavior
	sectionDisplay
)

type statusMessage struct {
	text    string
	isError bool
}

// Component is the tab component.
type Component struct {
	settings       prefs.AppSettings
	listState      tui.ListState
	section        section
	statusMessage  *statusMessage
	unsavedChanges bool
}

// New creates the component, loading saved settings.
func New() *Component {
	settings, errMessage := prefs.Load()
	c := &Component{
		settings:  settings,
		listState: tui.NewListState().WithSelected(0),
		section:   sectionTheme,
	}
	if errMessage != "" {
		c.statusMessage = &statusMessage{text: errMessage, isError: true}
	}
	return c
}

type sectionItem struct {
	name    string
	value   string
	enabled bool
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func (c *Component) sectionItems() []sectionItem {
	switch c.section {
	case sectionBehavior:
		return []sectionItem{
			{"Confirm Actions", yesNo(c.settings.ConfirmActions), true},
			{"Auto Refresh", yesNo(c.settings.AutoRefresh), true},
			{"Refresh Interval", fmt.Sprintf("%d seconds", c.settings.RefreshInterval), c.settings.AutoRefresh},
		}
	case sectionDisplay:
		return []sectionItem{
			{"Show Hidden Files", yesNo(c.settings.ShowHiddenFiles), true},
			{"Log Buffer Size", fmt.Sprintf("%d lines", c.settings.LogLines), true},
			{"Default Tab", c.settings.DefaultTab, true},
		}
	default:
		return []sectionItem{{"Color Theme", c.settings.Theme.Name(), true}}
	}
}

var defaultTabs = []string{"updater", "sbotools", "user_setup", "mirror", "packages", "config"}

func cycleIndex(current, n int, forward bool) int {
	if forward {
		return (current + 1) % n
	}
	return (current + n - 1) % n
}

func (c *Component) cycleCurrentOption(forward bool) {
	items := c.sectionItems()
	selected, ok := c.listState.Selected()
	if !ok {
		selected = 0
	}
	if selected >= len(items) {
		return
	}
	item := items[selected]
	if !item.enabled {
		return
	}

	switch c.section {
	case sectionTheme:
		themes := prefs.AllThemes()
		current := 0
		for i, t := range themes {
			if t == c.settings.Theme {
				current = i
				break
			}
		}
		c.settings.Theme = themes[cycleIndex(current, len(themes), forward)]
	case sectionBehavior:
		switch item.name {
		case "Confirm Actions":
			c.settings.ConfirmActions = !c.settings.ConfirmActions
		case "Auto Refresh":
			c.settings.AutoRefresh = !c.settings.AutoRefresh
		case "Refresh Interval":
			if forward {
				c.settings.RefreshInterval = min(c.settings.RefreshInterval+1, 60)
			} else {
				v := c.settings.RefreshInterval
				if v > 0 {
					v--
				}
				c.settings.RefreshInterval = max(v, 1)
			}
		}
	case sectionDisplay:
		switch item.name {
		case "Show Hidden Files":
			c.settings.ShowHiddenFiles = !c.settings.ShowHiddenFiles
		case "Log Buffer Size":
			if forward {
				// usize arithmetic: values loaded from a file can be as large as i64::MAX.
				c.settings.LogLines = int(min(uint64(c.settings.LogLines)+100, 10000))
			} else {
				c.settings.LogLines = max(max(c.settings.LogLines-100, 0), 100)
			}
		case "Default Tab":
			current := 0
			for i, t := range defaultTabs {
				if t == c.settings.DefaultTab {
					current = i
					break
				}
			}
			c.settings.DefaultTab = defaultTabs[cycleIndex(current, len(defaultTabs), forward)]
		}
	}

	c.unsavedChanges = true
}

// Settings returns the current settings.
func (c *Component) Settings() prefs.AppSettings { return c.settings }

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.statusMessage = &statusMessage{text: message, isError: isError}
	if !isError {
		c.unsavedChanges = false
	}
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	itemsLen := len(c.sectionItems())

	switch {
	case key.Code == tui.KeyTab:
		switch c.section {
		case sectionTheme:
			c.section = sectionBehavior
		case sectionBehavior:
			c.section = sectionDisplay
		default:
			c.section = sectionTheme
		}
		c.listState.Select(0)
	case key.Code == tui.KeyUp || key.IsChar('k'):
		if selected, ok := c.listState.Selected(); ok && selected > 0 {
			c.listState.Select(selected - 1)
		}
	case key.Code == tui.KeyDown || key.IsChar('j'):
		if selected, ok := c.listState.Selected(); ok && selected < max(itemsLen-1, 0) {
			c.listState.Select(selected + 1)
		}
	case key.Code == tui.KeyLeft || key.IsChar('h'):
		c.cycleCurrentOption(false)
	case key.Code == tui.KeyRight || key.IsChar('l') || key.Code == tui.KeyEnter:
		c.cycleCurrentOption(true)
	case key.IsChar('s'):
		return msg.SaveSettings{Settings: c.Settings()}
	case key.IsChar('r'):
		c.settings = prefs.Default()
		c.unsavedChanges = true
		c.statusMessage = &statusMessage{text: "Settings reset to defaults", isError: false}
	}
	return nil
}

func tabStyle(active bool) tui.Style {
	if active {
		return ui.TabActive()
	}
	return ui.TabInactive()
}

func enabledBadge(enabled bool) tui.Span {
	if enabled {
		return tui.Styled(" enabled ", ui.BadgeSuccess())
	}
	return tui.Styled(" disabled ", ui.BadgeWarning())
}

func (c *Component) renderSectionSummary(f *tui.Frame, area tui.Rect) {
	var lines []tui.Line
	switch c.section {
	case sectionTheme:
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("THEME", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled("Visual profile", ui.TitleStyle()),
			),
			tui.LineStr("Choose the palette and contrast profile for the console."),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Current ", ui.LabelStyle()),
				tui.Styled(c.settings.Theme.Name(), ui.BadgeNeutral()),
			),
		}
	case sectionBehavior:
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("BEHAVIOR", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled("Operator flow", ui.TitleStyle()),
			),
			tui.LineStr("Control confirmations and refresh cadence."),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Confirm actions ", ui.LabelStyle()),
				enabledBadge(c.settings.ConfirmActions),
			),
			tui.LineFrom(
				tui.Styled("Auto refresh ", ui.LabelStyle()),
				enabledBadge(c.settings.AutoRefresh),
			),
		}
	case sectionDisplay:
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("DISPLAY", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled("Density defaults", ui.TitleStyle()),
			),
			tui.LineStr("Tune the visible defaults for file and log-heavy workflows."),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Default tab ", ui.LabelStyle()),
				tui.Styled(c.settings.DefaultTab, ui.BadgeNeutral()),
			),
			tui.LineFrom(
				tui.Styled("Log buffer ", ui.LabelStyle()),
				tui.Raw(fmt.Sprintf("%d lines", c.settings.LogLines)),
			),
		}
	}

	f.RenderWidget(
		tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Section summary"))),
		area,
	)
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(4),
		tui.Min(10),
		tui.Length(3),
	)

	// Section tabs
	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	syncBadge := tui.Styled("  SYNCED  ", ui.BadgeSuccess())
	if c.unsavedChanges {
		syncBadge = tui.Styled("  UNSAVED  ", ui.BadgeWarning())
	}
	sectionBar := tui.ParagraphLine(tui.LineFrom(
		tui.Styled(" THEME ", tabStyle(c.section == sectionTheme)),
		tui.Raw(" "),
		tui.Styled(" BEHAVIOR ", tabStyle(c.section == sectionBehavior)),
		tui.Raw(" "),
		tui.Styled(" DISPLAY ", tabStyle(c.section == sectionDisplay)),
		tui.Styled("  //  Tab cycles sections", ui.MutedStyle()),
		syncBadge,
	)).Block(ui.Panel(ui.PanelTitle("Settings")))
	f.RenderWidget(sectionBar, header[0])
	c.renderSectionSummary(f, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(54), tui.Percentage(46))

	sectionItems := c.sectionItems()
	items := make([]tui.ListItem, 0, len(sectionItems))
	for _, item := range sectionItems {
		style := tui.NewStyle()
		if !item.enabled {
			style = tui.NewStyle().FG(tui.DarkGray)
		}
		valueStyle := style
		if item.enabled {
			valueStyle = ui.BadgeInfo()
		}
		items = append(items, tui.ListItemLine(tui.LineFrom(
			tui.Styled(fmt.Sprintf("%-18s", item.name), style),
			tui.Styled("  ", ui.SurfaceAltStyle()),
			tui.Styled(" "+item.value+" ", valueStyle),
		)))
	}

	list := tui.NewList(items).
		Block(ui.PanelAltBlock(ui.PanelTitle("Controls"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")

	state := c.listState
	list.RenderStateful(content[0], f.Buffer(), &state)

	c.renderThemePreview(f, content[1])

	// Status bar
	var statusContent tui.Line
	if c.statusMessage != nil {
		style := ui.SuccessStyle()
		if c.statusMessage.isError {
			style = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.statusMessage.text, style))
	} else {
		statusContent = tui.LineSpan(tui.Styled(
			"Use ←/→ to change values, 's' to save, 'r' to reset",
			ui.MutedStyle(),
		))
	}

	status := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(status, chunks[2])
}

func (c *Component) renderThemePreview(f *tui.Frame, area tui.Rect) {
	colors := c.settings.Theme.Colors()

	block := ui.Panel(ui.PanelTitle(c.settings.Theme.Name() + " theme preview")).
		BorderStyle(tui.NewStyle().FG(colors.Primary))

	inner := block.Inner(area)
	f.RenderWidget(block, area)

	chip := func(text string, fg, bg tui.Color) tui.Span {
		return tui.Styled(text, tui.NewStyle().FG(fg).BG(bg))
	}
	preview := []tui.Line{
		tui.LineFrom(
			chip(" PRIMARY ", colors.Background, colors.Primary),
			tui.Raw(" "),
			chip(" SECONDARY ", colors.Background, colors.Secondary),
		),
		tui.LineFrom(
			chip(" SUCCESS ", colors.Background, colors.Success),
			tui.Raw(" "),
			chip(" WARNING ", colors.Background, colors.Warning),
			tui.Raw(" "),
			chip(" ERROR ", colors.Foreground, colors.Error),
		),
		tui.LineSpan(tui.Styled(
			"The quick brown fox jumps over the lazy dog",
			tui.NewStyle().FG(colors.Foreground),
		)),
		tui.LineSpan(chip(" Background sample ", colors.Foreground, colors.Background)),
		tui.LineStr(""),
		tui.LineFrom(
			tui.Styled("Muted text ", tui.NewStyle().FG(colors.Muted)),
			chip("Selected chip", colors.Foreground, colors.Primary),
		),
	}

	f.RenderWidget(tui.ParagraphLines(preview...), inner)
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{
		{Key: "Tab", Desc: "Section"},
		{Key: "←/→", Desc: "Change"},
		{Key: "s", Desc: "Save"},
		{Key: "r", Desc: "Reset"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() {}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
