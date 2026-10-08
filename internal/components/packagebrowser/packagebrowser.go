// Package packagebrowser implements the installed package browser tab.
package packagebrowser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// packagesDir holds the installed package database.
const packagesDir = "/var/log/packages"

// installedPackage describes one installed package.
type installedPackage struct {
	name             string
	version          string
	arch             string
	build            string
	fullName         string
	description      string
	sizeCompressed   string
	sizeUncompressed string
}

type viewMode uint8

const (
	viewList viewMode = iota
	viewDetails
)

// Component is the package browser.
type Component struct {
	packages         []installedPackage
	filteredPackages []int
	listState        tui.ListState
	searchQuery      string
	isSearching      bool
	pendingRemoval   *installedPackage
	statusMessage    string
	statusIsError    bool
	hasStatus        bool
	showConfirm      bool
	viewMode         viewMode
}

// New creates the component.
func New() *Component {
	c := &Component{viewMode: viewList}
	c.loadPackages()
	c.applyFilter()
	if len(c.filteredPackages) > 0 {
		c.listState.Select(0)
	}
	return c
}

// readDirUnsorted lists a directory in the order the OS returns entries,
// like Rust's fs::read_dir.
func readDirUnsorted(dir string) ([]os.DirEntry, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

func (c *Component) loadPackages() {
	c.packages = loadPackagesFrom(packagesDir)
}

func loadPackagesFrom(dir string) []installedPackage {
	var packages []installedPackage
	if entries, err := readDirUnsorted(dir); err == nil {
		for _, entry := range entries {
			filename := strings.ToValidUTF8(entry.Name(), "�")
			pkg, ok := parsePackageName(filename)
			if !ok {
				continue
			}
			// Read package info file for description
			if data, err := os.ReadFile(filepath.Join(dir, entry.Name())); err == nil && utf8.Valid(data) {
				content := string(data)
				pkg.description = extractDescription(content)
				pkg.sizeCompressed = extractSize(content, "COMPRESSED PACKAGE SIZE:")
				pkg.sizeUncompressed = extractSize(content, "UNCOMPRESSED PACKAGE SIZE:")
			}
			packages = append(packages, pkg)
		}
	}
	sort.SliceStable(packages, func(i, j int) bool {
		return strings.ToLower(packages[i].name) < strings.ToLower(packages[j].name)
	})
	return packages
}

// parsePackageName splits a Slackware package name (name-version-arch-build)
// the way `rsplitn(4, '-')` does.
func parsePackageName(filename string) (installedPackage, bool) {
	parts := make([]string, 0, 4)
	rest := filename
	for len(parts) < 3 {
		i := strings.LastIndexByte(rest, '-')
		if i < 0 {
			break
		}
		parts = append(parts, rest[i+1:])
		rest = rest[:i]
	}
	parts = append(parts, rest)
	if len(parts) < 4 {
		return installedPackage{}, false
	}
	return installedPackage{
		build:    parts[0],
		arch:     parts[1],
		version:  parts[2],
		name:     parts[3],
		fullName: filename,
	}, true
}

func extractDescription(content string) string {
	inDescription := false
	var description strings.Builder

	for _, line := range tui.RustLines(content) {
		if strings.HasPrefix(line, "PACKAGE DESCRIPTION:") {
			inDescription = true
			continue
		}
		if !inDescription {
			continue
		}
		if strings.HasPrefix(line, "FILE LIST:") {
			break
		}
		// Skip the package name line (usually first line after PACKAGE DESCRIPTION)
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasSuffix(trimmed, ":") {
			// Remove package name prefix if present
			descLine := trimmed
			if pos := strings.IndexByte(trimmed, ':'); pos >= 0 {
				descLine = strings.TrimSpace(trimmed[pos+1:])
			}
			if descLine != "" {
				if description.Len() > 0 {
					description.WriteByte(' ')
				}
				description.WriteString(descLine)
			}
		}
	}

	out := description.String()
	if len(out) > 200 {
		if !utf8.RuneStart(out[200]) {
			panic("assertion failed: self.is_char_boundary(new_len)")
		}
		out = out[:200] + "..."
	}
	return out
}

func extractSize(content, prefix string) string {
	for _, line := range tui.RustLines(content) {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(trimStartMatches(line, prefix))
		}
	}
	return "Unknown"
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

func (c *Component) applyFilter() {
	c.filteredPackages = c.filteredPackages[:0]
	query := strings.ToLower(c.searchQuery)
	for i, pkg := range c.packages {
		if c.searchQuery == "" ||
			strings.Contains(strings.ToLower(pkg.name), query) ||
			strings.Contains(strings.ToLower(pkg.description), query) {
			c.filteredPackages = append(c.filteredPackages, i)
		}
	}
	if len(c.filteredPackages) == 0 {
		c.listState.SelectNone()
	} else {
		c.listState.Select(0)
	}
}

func (c *Component) selectedPackage() *installedPackage {
	i, ok := c.listState.Selected()
	if !ok || i >= len(c.filteredPackages) {
		return nil
	}
	idx := c.filteredPackages[i]
	if idx >= len(c.packages) {
		return nil
	}
	return &c.packages[idx]
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.statusMessage, c.statusIsError, c.hasStatus = message, isError, true
}

// RefreshPackages reloads installed packages.
func (c *Component) RefreshPackages() {
	c.loadPackages()
	c.applyFilter()
}

func (c *Component) renderSelectedPackage(f *tui.Frame, area tui.Rect) {
	var lines []tui.Line
	if pkg := c.selectedPackage(); pkg != nil {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled(pkg.name, ui.TitleStyle())),
			tui.LineFrom(tui.Styled("Version ", ui.MutedStyle()), tui.Raw(pkg.version)),
			tui.LineFrom(tui.Styled("Arch ", ui.MutedStyle()), tui.Raw(pkg.arch)),
			tui.LineFrom(
				tui.Styled("Sizes ", ui.MutedStyle()),
				tui.Raw(fmt.Sprintf("%s / %s", pkg.sizeCompressed, pkg.sizeUncompressed)),
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Description", ui.MutedStyle())),
			tui.LineStr(pkg.description),
		}
	} else {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled("No package selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Select an installed package to inspect metadata."),
		}
	}
	f.RenderWidget(tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))), area)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.showConfirm {
		switch {
		case key.IsChar('y') || key.IsChar('Y'):
			c.showConfirm = false
			if pkg := c.pendingRemoval; pkg != nil {
				c.pendingRemoval = nil
				return msg.RemoveInstalledPackage{Name: pkg.fullName}
			}
		case key.IsChar('n') || key.IsChar('N') || key.Code == tui.KeyEsc:
			c.showConfirm = false
			c.pendingRemoval = nil
		}
		return nil
	}

	if c.isSearching {
		switch key.Code {
		case tui.KeyEnter, tui.KeyEsc:
			c.isSearching = false
		case tui.KeyBackspace:
			if _, size := utf8.DecodeLastRuneInString(c.searchQuery); size > 0 {
				c.searchQuery = c.searchQuery[:len(c.searchQuery)-size]
			}
			c.applyFilter()
		case tui.KeyChar:
			c.searchQuery += string(key.Rune)
			c.applyFilter()
		}
		return nil
	}

	n := len(c.filteredPackages)
	switch {
	case key.Code == tui.KeyUp || key.IsChar('k'):
		if selected, ok := c.listState.Selected(); ok && selected > 0 {
			c.listState.Select(selected - 1)
		}
	case key.Code == tui.KeyDown || key.IsChar('j'):
		if selected, ok := c.listState.Selected(); ok {
			if selected < max(n-1, 0) {
				c.listState.Select(selected + 1)
			}
		} else if n > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyHome:
		if n > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyEnd:
		if n > 0 {
			c.listState.Select(n - 1)
		}
	case key.Code == tui.KeyPageUp:
		if selected, ok := c.listState.Selected(); ok {
			c.listState.Select(max(selected-10, 0))
		}
	case key.Code == tui.KeyPageDown:
		if selected, ok := c.listState.Selected(); ok {
			c.listState.Select(min(selected+10, max(n-1, 0)))
		}
	case key.IsChar('/'):
		c.isSearching = true
	case key.Code == tui.KeyEnter:
		if c.viewMode == viewList {
			c.viewMode = viewDetails
		} else {
			c.viewMode = viewList
		}
	case key.IsChar('d'):
		if pkg := c.selectedPackage(); pkg != nil {
			clone := *pkg
			c.pendingRemoval = &clone
			c.showConfirm = true
		}
	case key.IsChar('c'):
		c.searchQuery = ""
		c.applyFilter()
	case key.IsF(5):
		c.loadPackages()
		c.applyFilter()
		c.statusMessage, c.statusIsError, c.hasStatus = "Package list refreshed", false, true
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(4), tui.Min(10), tui.Length(3))
	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	searchStyle := ui.ValueStyle()
	cursor := tui.Raw("")
	if c.isSearching {
		searchStyle = ui.AccentStyle()
		cursor = tui.Styled(" _", ui.AccentStyle())
	}
	searchBar := tui.ParagraphLine(tui.LineFrom(
		tui.Styled("Query ", ui.LabelStyle()),
		tui.Styled(c.searchQuery, searchStyle),
		cursor,
		tui.Styled(fmt.Sprintf("  %d/%d packages", len(c.filteredPackages), len(c.packages)), ui.BadgeNeutral()),
	)).Block(ui.Panel(ui.PanelTitle("Installed packages")))
	f.RenderWidget(searchBar, header[0])

	mode := " DETAILS "
	if c.viewMode == viewList {
		mode = " LIST "
	}
	activity := tui.Styled(" IDLE ", ui.BadgeSuccess())
	if c.isSearching {
		activity = tui.Styled(" SEARCH ", ui.BadgeWarning())
	}
	runtime := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Mode ", ui.MutedStyle()),
			tui.Styled(mode, ui.BadgeNeutral()),
			tui.Raw(" "),
			activity,
		),
		tui.LineStr(""),
		tui.LineSpan(tui.Styled(
			"Browse installed packages, inspect metadata, then remove with confirmation.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(runtime, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(62), tui.Percentage(38))

	if c.viewMode == viewDetails {
		if pkg := c.selectedPackage(); pkg != nil {
			c.renderDetails(f, content[0], pkg)
			c.renderSelectedPackage(f, content[1])
		}
	} else {
		c.renderList(f, content[0])
		c.renderSelectedPackage(f, content[1])
	}

	// Status bar
	var statusContent tui.Line
	switch {
	case c.showConfirm:
		name := "?"
		if c.pendingRemoval != nil {
			name = c.pendingRemoval.name
		}
		statusContent = tui.LineFrom(
			tui.Styled(fmt.Sprintf("Remove package '%s'? ", name), ui.WarningStyle()),
			tui.Raw("[Y]es / [N]o"),
		)
	case c.hasStatus:
		style := ui.SuccessStyle()
		if c.statusIsError {
			style = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.statusMessage, style))
	default:
		if pkg := c.selectedPackage(); pkg != nil {
			statusContent = tui.LineFrom(
				tui.Styled("Footprint ", ui.LabelStyle()),
				tui.Raw(fmt.Sprintf("%s compressed, %s installed", pkg.sizeCompressed, pkg.sizeUncompressed)),
			)
		} else {
			statusContent = tui.LineSpan(tui.Styled("No package selected", ui.MutedStyle()))
		}
	}
	status := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(status, chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	if c.isSearching {
		return []msg.KeyHelp{{Key: "Enter/Esc", Desc: "Done"}, {Key: "Type", Desc: "Search"}}
	}
	return []msg.KeyHelp{
		{Key: "/", Desc: "Search"},
		{Key: "Enter", Desc: "Details"},
		{Key: "d", Desc: "Remove"},
		{Key: "c", Desc: "Clear"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() {
	c.loadPackages()
	c.applyFilter()
}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}

func (c *Component) renderList(f *tui.Frame, area tui.Rect) {
	items := make([]tui.ListItem, 0, len(c.filteredPackages))
	for _, idx := range c.filteredPackages {
		if idx >= len(c.packages) {
			continue
		}
		pkg := c.packages[idx]
		desc := pkg.description
		if len(desc) > 70 {
			if !utf8.RuneStart(desc[67]) {
				panic(fmt.Sprintf("byte index 67 is not a char boundary in `%s`", desc))
			}
			desc = desc[:67] + "..."
		}
		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				tui.Styled(fmt.Sprintf("%-28s", pkg.name), ui.TitleStyle()),
				tui.Styled(fmt.Sprintf(" %-14s", pkg.version), ui.SuccessStyle()),
				tui.Styled(fmt.Sprintf(" %-10s", pkg.arch), ui.AccentStyle()),
			),
			tui.LineSpan(tui.Styled("  "+desc, ui.MutedStyle())),
		))
	}

	list := tui.NewList(items).
		Block(ui.PanelAltBlock(ui.PanelTitle("Package roster"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")
	state := c.listState
	list.RenderStateful(area, f.Buffer(), &state)
}

func (c *Component) renderDetails(f *tui.Frame, area tui.Rect, pkg *installedPackage) {
	block := ui.Panel(ui.PanelTitle("Package " + pkg.name))
	inner := block.Inner(area)
	f.RenderWidget(block, area)

	lines := []tui.Line{
		tui.LineFrom(tui.Styled("Identity", ui.EyebrowStyle())),
		tui.LineFrom(tui.Styled("Name         ", ui.LabelStyle()), tui.Raw(pkg.name)),
		tui.LineFrom(tui.Styled("Version      ", ui.LabelStyle()), tui.Raw(pkg.version)),
		tui.LineFrom(tui.Styled("Architecture ", ui.LabelStyle()), tui.Raw(pkg.arch)),
		tui.LineFrom(tui.Styled("Build        ", ui.LabelStyle()), tui.Raw(pkg.build)),
		tui.LineFrom(tui.Styled("Full name    ", ui.LabelStyle()), tui.Raw(pkg.fullName)),
		tui.LineStr(""),
		tui.LineFrom(tui.Styled("Payload", ui.EyebrowStyle())),
		tui.LineFrom(tui.Styled("Compressed   ", ui.LabelStyle()), tui.Raw(pkg.sizeCompressed)),
		tui.LineFrom(tui.Styled("Uncompressed ", ui.LabelStyle()), tui.Raw(pkg.sizeUncompressed)),
		tui.LineStr(""),
		tui.LineSpan(tui.Styled("Description", ui.EyebrowStyle())),
		tui.LineSpan(tui.Styled("Package notes from /var/log/packages", ui.SubtitleStyle())),
		tui.LineStr(""),
	}

	// Wrap description text in chunks of `inner.width - 2` chars. The
	// original computes this with usize arithmetic: a width below 2 wraps
	// around to a huge chunk size (release build) and a width of exactly 2
	// panics on a zero chunk size.
	chunk := inner.Width - 2
	if chunk == 0 {
		panic("chunk size must be non-zero")
	}
	desc := []rune(pkg.description)
	if chunk < 0 {
		chunk = len(desc) + 1
	}
	for start := 0; start < len(desc); start += chunk {
		end := min(start+chunk, len(desc))
		lines = append(lines, tui.LineSpan(tui.Raw(string(desc[start:end]))))
	}

	f.RenderWidget(tui.ParagraphLines(lines...).Wrap(true), inner)
}
