// Package packagesearch implements the SlackBuilds package search tab.
package packagesearch

import (
	"fmt"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// Component is the package search component.
type Component struct {
	searchQuery  string
	results      []slackware.PackageInfo
	listState    tui.ListState
	isSearching  bool
	isInstalling bool

	hasStatus     bool
	statusMessage string
	statusIsError bool
}

// New creates the component.
func New() *Component { return &Component{} }

// SetResults shows search results.
func (c *Component) SetResults(results []slackware.PackageInfo) {
	c.results = results
	c.isSearching = false
	if len(c.results) > 0 {
		c.listState.Select(0)
	} else {
		c.listState.SelectNone()
	}
}

func (c *Component) selectedPackage() (slackware.PackageInfo, bool) {
	i, ok := c.listState.Selected()
	if !ok || i < 0 || i >= len(c.results) {
		return slackware.PackageInfo{}, false
	}
	return c.results[i], true
}

func (c *Component) startSearch() {
	c.isSearching = true
	c.hasStatus = false
}

func (c *Component) startInstall() {
	c.isInstalling = true
	c.hasStatus = false
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.hasStatus, c.statusMessage, c.statusIsError = true, message, isError
	c.isSearching = false
	c.isInstalling = false
}

func (c *Component) renderSelectedPackage(f *tui.Frame, area tui.Rect) {
	var lines []tui.Line
	if pkg, ok := c.selectedPackage(); ok {
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("PACKAGE", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(pkg.Name, ui.TitleStyle()),
			),
			tui.LineFrom(
				tui.Styled("Category ", ui.LabelStyle()),
				tui.Styled(pkg.Category, ui.BadgeNeutral()),
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Description", ui.EyebrowStyle())),
			tui.LineStr(pkg.Description),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Actions", ui.EyebrowStyle())),
			tui.LineStr("Enter   search current query"),
			tui.LineStr("Ctrl+I  install selected package"),
			tui.LineStr("Tab     move between search hits"),
		}
	} else {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled("No package selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Search for a package to inspect details here."),
		}
	}

	panel := tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector")))
	f.RenderWidget(panel, area)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.isSearching || c.isInstalling {
		return nil
	}

	ctrl := key.Has(tui.ModControl)
	switch {
	case key.Code == tui.KeyChar && !ctrl:
		c.searchQuery += string(key.Rune)
	case key.Code == tui.KeyBackspace:
		if c.searchQuery != "" {
			_, size := utf8.DecodeLastRuneInString(c.searchQuery)
			c.searchQuery = c.searchQuery[:len(c.searchQuery)-size]
		}
	case key.Code == tui.KeyEnter && c.searchQuery != "":
		c.startSearch()
		return msg.SearchPackages{Query: c.searchQuery}
	case (key.Code == tui.KeyUp || key.IsChar('k')) && ctrl:
		if selected, ok := c.listState.Selected(); ok && selected > 0 {
			c.listState.Select(selected - 1)
		}
	case (key.Code == tui.KeyDown || key.IsChar('j')) && ctrl:
		if selected, ok := c.listState.Selected(); ok {
			if selected < max(len(c.results)-1, 0) {
				c.listState.Select(selected + 1)
			}
		} else if len(c.results) > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyTab:
		// Cycle through results
		if len(c.results) > 0 {
			next := 0
			if selected, ok := c.listState.Selected(); ok {
				next = (selected + 1) % len(c.results)
			}
			c.listState.Select(next)
		}
	case key.IsChar('i') && ctrl:
		if pkg, ok := c.selectedPackage(); ok {
			c.startInstall()
			return msg.InstallPackage{Name: pkg.Name}
		}
	}
	return nil
}

// truncateDescription mirrors the original `&description[..50]` byte slice.
// The original panics when byte 50 is not a char boundary; here the cut
// moves back to the previous boundary instead.
func truncateDescription(description string) string {
	if len(description) <= 50 {
		return description
	}
	end := 50
	for end > 0 && !utf8.RuneStart(description[end]) {
		end--
	}
	return description[:end] + "..."
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(5),
		tui.Min(10),
		tui.Length(3), // Status
	)

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	title := tui.ParagraphLines(
		tui.LineSpan(tui.Styled("SlackBuilds Package Search", ui.TitleStyle())),
		tui.LineSpan(tui.Styled(
			"Query, inspect, and install packages without leaving the terminal.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Search deck")))
	f.RenderWidget(title, header[0])

	query := c.searchQuery
	if query == "" {
		query = "<empty>"
	}
	var state tui.Span
	switch {
	case c.isSearching:
		state = tui.Styled(" SEARCHING ", ui.BadgeWarning())
	case c.isInstalling:
		state = tui.Styled(" INSTALLING ", ui.BadgeWarning())
	default:
		state = tui.Styled(" READY ", ui.BadgeSuccess())
	}
	queryPanel := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Query ", ui.LabelStyle()),
			tui.Styled(query, ui.InputActive()),
		),
		tui.LineFrom(
			tui.Styled("Results ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(len(c.results)), ui.BadgeNeutral()),
			tui.Raw(" "),
			state,
		),
	).Style(ui.InputActive()).Block(ui.PanelFocused(ui.PanelTitle("Query")))
	f.RenderWidget(queryPanel, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(62), tui.Percentage(38))

	items := make([]tui.ListItem, 0, len(c.results))
	for _, pkg := range c.results {
		items = append(items, tui.ListItemLine(tui.LineFrom(
			tui.Styled(pkg.Name, ui.TitleStyle()),
			tui.Raw(" "),
			tui.Styled(fmt.Sprintf(" %s ", pkg.Category), ui.BadgeNeutral()),
			tui.Raw(" - "),
			tui.Styled(truncateDescription(pkg.Description), ui.MutedStyle()),
		)))
	}

	resultsTitle := fmt.Sprintf("Results (%d)", len(c.results))
	if c.isSearching {
		resultsTitle = "Searching..."
	}

	list := tui.NewList(items).
		Block(ui.Panel(ui.PanelTitle(resultsTitle))).
		HighlightStyle(ui.HighlightStyle().Add(tui.Bold)).
		HighlightSymbol("▸ ")

	st := c.listState
	list.RenderStateful(content[0], f.Buffer(), &st)
	c.renderSelectedPackage(f, content[1])

	var status tui.Paragraph
	switch {
	case c.hasStatus:
		style := ui.SuccessStyle()
		if c.statusIsError {
			style = ui.ErrorStyle()
		}
		status = tui.ParagraphStr(c.statusMessage).Style(style)
	case c.isInstalling:
		status = tui.ParagraphStr("Installing package...").Style(ui.WarningStyle())
	case c.isSearching:
		status = tui.ParagraphStr("Searching...").Style(ui.WarningStyle())
	default:
		status = tui.ParagraphStr("Type to search, Enter to submit, Ctrl+I to install selected").
			Style(ui.MutedStyle())
	}
	f.RenderWidget(status.Block(ui.PanelAltBlock(ui.PanelTitle("Status"))), chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{
		{Key: "Enter", Desc: "Search"},
		{Key: "Tab", Desc: "Next result"},
		{Key: "Ctrl+I", Desc: "Install"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() {}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
