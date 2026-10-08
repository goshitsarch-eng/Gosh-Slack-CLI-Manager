// Package logs implements the Log Viewer tab: an inventory of the files
// under /var/log and a pager with search and follow mode.
package logs

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

// logFile describes a log file.
type logFile struct {
	name     string
	path     string
	size     uint64
	modified string
}

type viewMode uint8

const (
	modeFileList viewMode = iota
	modeViewLog
)

type status struct {
	message string
	isError bool
}

// Component is the Log Viewer tab.
type Component struct {
	logFiles         []logFile
	fileListState    tui.ListState
	mode             viewMode
	logContent       []string
	contentScroll    int
	searchQuery      string
	isSearching      bool
	searchResults    []int
	currentSearchIdx int
	followMode       bool
	status           *status
}

var logDirs = []string{"/var/log"}

var importantLogs = []string{
	"messages",
	"syslog",
	"dmesg",
	"secure",
	"auth.log",
	"boot",
	"Xorg.0.log",
	"packages/",
	"slackpkg.log",
	"lastlog",
	"wtmp",
	"btmp",
}

const maxLines = 1000

// New creates the component.
func New() *Component {
	c := &Component{mode: modeFileList}
	c.loadLogFiles()
	if len(c.logFiles) > 0 {
		c.fileListState.Select(0)
	}
	return c
}

func isImportant(name string) bool {
	return slices.ContainsFunc(importantLogs, func(l string) bool { return strings.Contains(name, l) })
}

func (c *Component) loadLogFiles() {
	c.logFiles = c.logFiles[:0]

	for _, dir := range logDirs {
		c.scanDirectory(dir, 0)
	}

	// Sort by importance and name
	sort.SliceStable(c.logFiles, func(i, j int) bool {
		a, b := c.logFiles[i], c.logFiles[j]
		ai, bi := isImportant(a.name), isImportant(b.name)
		if ai != bi {
			return ai
		}
		return a.name < b.name
	})
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

func (c *Component) scanDirectory(path string, depth int) {
	if depth > 2 {
		return
	}

	entries, err := readDirOrder(path)
	if err != nil {
		return
	}
	for _, e := range entries {
		entryPath := filepath.Join(path, e.Name())
		target, statErr := os.Stat(entryPath)

		switch {
		case statErr == nil && target.IsDir():
			// Skip certain directories
			name := e.Name()
			if strings.HasPrefix(name, ".") || name == "journal" {
				continue
			}
			c.scanDirectory(entryPath, depth+1)
		case statErr == nil && target.Mode().IsRegular():
			name := e.Name()

			// Skip compressed logs and certain files
			if strings.HasSuffix(name, ".gz") ||
				strings.HasSuffix(name, ".xz") ||
				strings.HasSuffix(name, ".old") ||
				strings.HasPrefix(name, ".") {
				continue
			}

			info, err := e.Info()
			if err != nil {
				continue
			}
			modified := info.ModTime().Local().Format("2006-01-02 15:04")

			// Create display name with relative path
			displayName := name
			if rel, ok := stripPrefix(entryPath, "/var/log"); ok {
				displayName = rel
			}

			c.logFiles = append(c.logFiles, logFile{
				name:     displayName,
				path:     entryPath,
				size:     uint64(info.Size()),
				modified: modified,
			})
		}
	}
}

// stripPrefix mirrors Path::strip_prefix for the clean absolute paths built
// by the scanner.
func stripPrefix(path, prefix string) (string, bool) {
	if path == prefix {
		return "", true
	}
	if rest, ok := strings.CutPrefix(path, prefix+"/"); ok {
		return rest, true
	}
	return "", false
}

// readLines mirrors BufRead::lines().map_while(Result::ok): lines split on
// '\n' with a trailing "\n" or "\r\n" removed, stopping at the first line
// that is not valid UTF-8 or at a read error.
func readLines(r io.Reader) []string {
	br := bufio.NewReader(r)
	var lines []string
	for {
		raw, err := br.ReadBytes('\n')
		if len(raw) == 0 && err != nil {
			return lines
		}
		if err != nil && err != io.EOF {
			return lines
		}
		if !utf8.Valid(raw) {
			return lines
		}
		if bytes.HasSuffix(raw, []byte{'\n'}) {
			raw = raw[:len(raw)-1]
			raw = bytes.TrimSuffix(raw, []byte{'\r'})
		}
		lines = append(lines, string(raw))
		if err != nil {
			return lines
		}
	}
}

func (c *Component) loadLogContent(path string) {
	c.logContent = nil
	c.contentScroll = 0
	c.searchResults = c.searchResults[:0]

	f, err := os.Open(path)
	if err != nil {
		c.logContent = []string{"Error reading file: " + utils.IOErrorString(err)}
		return
	}
	defer f.Close()

	lines := readLines(f)
	// Keep only last maxLines
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	c.logContent = lines

	// Scroll to end if in follow mode
	if c.followMode && len(c.logContent) > 0 {
		c.contentScroll = max(len(c.logContent)-1, 0)
	}
}

func (c *Component) refreshLog() {
	if log, ok := c.selectedLog(); ok {
		c.loadLogContent(log.path)
	}
}

func (c *Component) selectedLog() (logFile, bool) {
	if i, ok := c.fileListState.Selected(); ok && i < len(c.logFiles) {
		return c.logFiles[i], true
	}
	return logFile{}, false
}

func (c *Component) performSearch() {
	c.searchResults = c.searchResults[:0]
	c.currentSearchIdx = 0

	if c.searchQuery == "" {
		return
	}

	query := rustToLower(c.searchQuery)
	for i, line := range c.logContent {
		if strings.Contains(rustToLower(line), query) {
			c.searchResults = append(c.searchResults, i)
		}
	}

	// Jump to first result
	if len(c.searchResults) > 0 {
		c.contentScroll = c.searchResults[0]
	}
}

func (c *Component) nextSearchResult() {
	if len(c.searchResults) == 0 {
		return
	}
	c.currentSearchIdx = (c.currentSearchIdx + 1) % len(c.searchResults)
	c.contentScroll = c.searchResults[c.currentSearchIdx]
}

func (c *Component) prevSearchResult() {
	if len(c.searchResults) == 0 {
		return
	}
	if c.currentSearchIdx == 0 {
		c.currentSearchIdx = len(c.searchResults) - 1
	} else {
		c.currentSearchIdx--
	}
	c.contentScroll = c.searchResults[c.currentSearchIdx]
}

func formatSize(bytes uint64) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func logLevelColor(line string) tui.Color {
	lower := rustToLower(line)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "fail") || strings.Contains(lower, "crit"):
		return tui.Red
	case strings.Contains(lower, "warn"):
		return tui.Yellow
	case strings.Contains(lower, "info"):
		return tui.Cyan
	case strings.Contains(lower, "debug"):
		return tui.DarkGray
	default:
		return tui.White
	}
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.isSearching {
		switch key.Code {
		case tui.KeyEnter:
			c.isSearching = false
			c.performSearch()
		case tui.KeyEsc:
			c.isSearching = false
			c.searchQuery = ""
			c.searchResults = c.searchResults[:0]
		case tui.KeyBackspace:
			if _, size := utf8.DecodeLastRuneInString(c.searchQuery); size > 0 {
				c.searchQuery = c.searchQuery[:len(c.searchQuery)-size]
			}
		case tui.KeyChar:
			c.searchQuery += string(key.Rune)
		}
		return nil
	}

	switch c.mode {
	case modeFileList:
		switch {
		case key.Code == tui.KeyUp || key.IsChar('k'):
			if selected, ok := c.fileListState.Selected(); ok && selected > 0 {
				c.fileListState.Select(selected - 1)
			}
		case key.Code == tui.KeyDown || key.IsChar('j'):
			if selected, ok := c.fileListState.Selected(); ok && selected < max(len(c.logFiles)-1, 0) {
				c.fileListState.Select(selected + 1)
			}
		case key.Code == tui.KeyEnter:
			if log, ok := c.selectedLog(); ok {
				c.loadLogContent(log.path)
				c.mode = modeViewLog
			}
		case key.Code == tui.KeyHome:
			c.fileListState.Select(0)
		case key.Code == tui.KeyEnd:
			if len(c.logFiles) > 0 {
				c.fileListState.Select(len(c.logFiles) - 1)
			}
		case key.IsF(5):
			c.loadLogFiles()
			c.status = &status{message: "Log list refreshed"}
		}
	case modeViewLog:
		last := max(len(c.logContent)-1, 0)
		switch {
		case key.Code == tui.KeyEsc || key.IsChar('q'):
			c.mode = modeFileList
			c.logContent = nil
			c.searchQuery = ""
			c.searchResults = c.searchResults[:0]
		case key.Code == tui.KeyUp || key.IsChar('k'):
			if c.contentScroll > 0 {
				c.contentScroll--
				c.followMode = false
			}
		case key.Code == tui.KeyDown || key.IsChar('j'):
			if c.contentScroll < last {
				c.contentScroll++
			}
		case key.Code == tui.KeyPageUp:
			c.contentScroll = max(c.contentScroll-20, 0)
			c.followMode = false
		case key.Code == tui.KeyPageDown:
			c.contentScroll = min(c.contentScroll+20, last)
		case key.Code == tui.KeyHome || key.IsChar('g'):
			c.contentScroll = 0
			c.followMode = false
		case key.Code == tui.KeyEnd || key.IsChar('G'):
			c.contentScroll = last
		case key.IsChar('/'):
			c.isSearching = true
			c.searchQuery = ""
		case key.IsChar('n'):
			c.nextSearchResult()
		case key.IsChar('N'):
			c.prevSearchResult()
		case key.IsChar('f'):
			c.followMode = !c.followMode
			if c.followMode {
				c.refreshLog()
				c.status = &status{message: "Follow mode enabled"}
			} else {
				c.status = &status{message: "Follow mode disabled"}
			}
		case key.IsF(5):
			c.refreshLog()
			c.status = &status{message: "Log refreshed"}
		}
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	if c.mode == modeViewLog {
		c.renderLogView(f, area)
	} else {
		c.renderFileList(f, area)
	}
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	if c.mode == modeViewLog {
		return []msg.KeyHelp{
			{Key: "q/Esc", Desc: "Back"},
			{Key: "/", Desc: "Search"},
			{Key: "n/N", Desc: "Next/Prev"},
			{Key: "f", Desc: "Follow"},
		}
	}
	return []msg.KeyHelp{{Key: "Enter", Desc: "Open"}, {Key: "↑/↓", Desc: "Navigate"}, {Key: "F5", Desc: "Refresh"}}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.loadLogFiles() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}

func (c *Component) renderFileList(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(4), tui.Min(10), tui.Length(3))

	var totalSize uint64
	importantCount := 0
	for _, log := range c.logFiles {
		totalSize += log.size
		if isImportant(log.name) {
			importantCount++
		}
	}

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	titlePanel := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("LOG INDEX", ui.BadgeInfo()),
			tui.Raw(" "),
			tui.Styled("Filesystem log inventory", ui.TitleStyle()),
		),
		tui.LineSpan(tui.Styled(
			"Browse curated system logs before opening a file for line-by-line inspection.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Log viewer")))
	f.RenderWidget(titlePanel, header[0])

	runtimePanel := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Files ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.logFiles)), ui.BadgeNeutral()),
			tui.Raw(" "),
			tui.Styled("Important ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(importantCount), ui.BadgeSuccess()),
		),
		tui.LineFrom(
			tui.Styled("Visible size ", ui.LabelStyle()),
			tui.Styled(formatSize(totalSize), ui.BadgeNeutral()),
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(runtimePanel, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(66), tui.Percentage(34))

	items := make([]tui.ListItem, 0, len(c.logFiles))
	for _, log := range c.logFiles {
		items = append(items, tui.ListItemLine(tui.LineFrom(
			tui.Styled(fmt.Sprintf("%-38s", log.name), ui.TitleStyle()),
			tui.Styled(fmt.Sprintf("%8s", formatSize(log.size)), ui.AccentStyle()),
			tui.Styled("  "+log.modified, ui.MutedStyle()),
		)))
	}

	list := tui.NewList(items).
		Block(ui.Panel(ui.PanelTitle(fmt.Sprintf("Log files (%d)", len(c.logFiles))))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")

	st := c.fileListState
	list.RenderStateful(content[0], f.Buffer(), &st)

	var inspectorLines []tui.Line
	if log, ok := c.selectedLog(); ok {
		inspectorLines = []tui.Line{
			tui.LineSpan(tui.Styled(log.name, ui.TitleStyle())),
			tui.LineFrom(
				tui.Styled("Size ", ui.LabelStyle()),
				tui.Styled(formatSize(log.size), ui.BadgeNeutral()),
			),
			tui.LineFrom(
				tui.Styled("Modified ", ui.LabelStyle()),
				tui.Raw(log.modified),
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Path", ui.EyebrowStyle())),
			tui.LineStr(log.path),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled(
				"Press Enter to open and inspect live content.",
				ui.SubtitleStyle(),
			)),
		}
	} else {
		inspectorLines = []tui.Line{
			tui.LineSpan(tui.Styled("No log selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Pick a log file to preview its metadata."),
		}
	}
	inspector := tui.ParagraphLines(inspectorLines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector")))
	f.RenderWidget(inspector, content[1])

	// Status bar
	var statusContent tui.Line
	if c.status != nil {
		st := ui.SuccessStyle()
		if c.status.isError {
			st = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.status.message, st))
	} else if log, ok := c.selectedLog(); ok {
		statusContent = tui.LineFrom(
			tui.Styled("Path: ", ui.MutedStyle()),
			tui.Raw(log.path),
		)
	} else {
		statusContent = tui.LineSpan(tui.Styled("Select a log file", ui.MutedStyle()))
	}

	statusBar := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(statusBar, chunks[2])
}

func (c *Component) renderLogView(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(4), tui.Min(10), tui.Length(3))

	// Header with search
	title := "Log"
	if log, ok := c.selectedLog(); ok {
		title = log.name
	}

	searchDisplay := ""
	switch {
	case c.isSearching:
		searchDisplay = fmt.Sprintf("Search: %s█", c.searchQuery)
	case c.searchQuery != "":
		current := 0
		if len(c.searchResults) > 0 {
			current = c.currentSearchIdx + 1
		}
		searchDisplay = fmt.Sprintf("Search: %s (%d/%d)", c.searchQuery, current, len(c.searchResults))
	}

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(62), tui.Percentage(38))

	titlePanel := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("LIVE FILE", ui.BadgeInfo()),
			tui.Raw(" "),
			tui.Styled(title, ui.TitleStyle()),
		),
		tui.LineSpan(tui.Styled(
			"Navigate by line, search in-place, or follow the live tail.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Log viewer")))
	f.RenderWidget(titlePanel, header[0])

	searchText := searchDisplay
	if searchText == "" {
		searchText = "idle"
	}
	searchStyle := ui.KeyHintSecondary()
	if c.isSearching {
		searchStyle = ui.WarningStyle()
	}
	modeBadge := tui.Styled(" STATIC ", ui.BadgeNeutral())
	if c.followMode {
		modeBadge = tui.Styled(" FOLLOW ", ui.BadgeSuccess())
	}
	metaPanel := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Search ", ui.LabelStyle()),
			tui.Styled(searchText, searchStyle),
		),
		tui.LineFrom(
			tui.Styled("Mode ", ui.LabelStyle()),
			modeBadge,
		),
		tui.LineFrom(
			tui.Styled("Lines ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.logContent)), ui.BadgeNeutral()),
			tui.Raw(" "),
			tui.Styled("Matches ", ui.LabelStyle()),
			tui.Styled(strconv.Itoa(len(c.searchResults)), ui.BadgeSuccess()),
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(metaPanel, header[1])

	// Log content
	visibleHeight := max(chunks[1].Height-2, 0)
	start := c.contentScroll
	end := min(start+visibleHeight, len(c.logContent))

	lines := make([]tui.Line, 0, max(end-start, 0))
	for i, line := range c.logContent[start:end] {
		lineNum := start + i
		var style tui.Style
		if slices.Contains(c.searchResults, lineNum) {
			style = tui.NewStyle().BG(tui.Yellow).FG(tui.Black)
		} else {
			style = tui.NewStyle().FG(logLevelColor(line))
		}
		lines = append(lines, tui.LineFrom(
			tui.Styled(fmt.Sprintf("%6d ", lineNum+1), ui.MutedStyle()),
			tui.Styled(line, style),
		))
	}

	contentPara := tui.ParagraphLines(lines...).
		Block(ui.PanelAltBlock(ui.PanelTitle("Stream"))).
		Wrap(false)
	f.RenderWidget(contentPara, chunks[1])

	// Status bar
	statusSpan := tui.Raw("")
	if c.status != nil {
		color := tui.Green
		if c.status.isError {
			color = tui.Red
		}
		statusSpan = tui.Styled("  "+c.status.message, tui.NewStyle().FG(color))
	}
	statusBar := tui.ParagraphLine(tui.LineFrom(
		tui.Styled("Line: ", ui.MutedStyle()),
		tui.Raw(fmt.Sprintf("%d/%d", c.contentScroll+1, len(c.logContent))),
		statusSpan,
	)).Block(ui.PanelAltBlock(ui.PanelTitle("Position")))
	f.RenderWidget(statusBar, chunks[2])
}
