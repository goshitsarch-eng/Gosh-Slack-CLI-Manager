// Package cron implements the cron job manager tab (port of
// src/components/cron.rs). It only reads cron configuration; it never
// modifies crontabs.
package cron

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// sourceKind distinguishes system and user cron sources.
type sourceKind uint8

const (
	sourceSystem sourceKind = iota // Value is a path to a file in /etc/cron.*
	sourceUser                     // Value is a username
)

// cronSource is where a job was defined.
type cronSource struct {
	Kind  sourceKind
	Value string
}

// cronJob is a cron job entry.
type cronJob struct {
	Minute  string
	Hour    string
	Day     string
	Month   string
	Weekday string
	Command string
	Source  cronSource
	Enabled bool
	RawLine string
}

// cronFilter selects which jobs are visible.
type cronFilter uint8

const (
	filterAll cronFilter = iota
	filterSystem
	filterUser
	filterHourly
	filterDaily
	filterWeekly
	filterMonthly
)

// Paths read by the component; variables so tests can point them elsewhere.
var (
	cronHourlyDir   = "/etc/cron.hourly"
	cronDailyDir    = "/etc/cron.daily"
	cronWeeklyDir   = "/etc/cron.weekly"
	cronMonthlyDir  = "/etc/cron.monthly"
	systemCrontab   = "/etc/crontab"
	cronDDir        = "/etc/cron.d"
	userCrontabsDir = "/var/spool/cron/crontabs"
)

type status struct {
	text    string
	isError bool
}

// Component is the cron job manager component.
type Component struct {
	jobs          []cronJob
	listState     tui.ListState
	filter        cronFilter
	statusMessage *status
}

// New creates the component and loads the cron jobs.
func New() *Component {
	c := &Component{}
	c.loadCronJobs()
	if len(c.jobs) > 0 {
		c.listState.Select(0)
	}
	return c
}

func (c *Component) loadCronJobs() {
	c.jobs = c.jobs[:0]

	// Load system cron directories
	c.loadCronDir(cronHourlyDir, "hourly")
	c.loadCronDir(cronDailyDir, "daily")
	c.loadCronDir(cronWeeklyDir, "weekly")
	c.loadCronDir(cronMonthlyDir, "monthly")

	// Load /etc/crontab
	c.loadCrontab(systemCrontab)

	// Load /etc/cron.d/*
	for _, e := range readDirUnsorted(cronDDir) {
		path := filepath.Join(cronDDir, e.Name())
		if isFile(path) {
			c.loadCrontabFile(path)
		}
	}

	// Load user crontabs
	c.loadUserCrontabs()
}

// readDirUnsorted lists a directory in the order the OS returns entries,
// like Rust's fs::read_dir (os.ReadDir would sort by name).
func readDirUnsorted(dir string) []os.DirEntry {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	entries, _ := f.ReadDir(-1)
	return entries
}

// isFile follows symlinks, like Path::is_file.
func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func (c *Component) loadCronDir(dir, period string) {
	if _, err := os.Stat(dir); err != nil {
		return
	}

	for _, e := range readDirUnsorted(dir) {
		entryPath := filepath.Join(dir, e.Name())
		if !isFile(entryPath) {
			continue
		}
		name := strings.ToValidUTF8(e.Name(), "�")

		// Skip backup files
		if strings.HasSuffix(name, "~") || strings.HasPrefix(name, ".") {
			continue
		}

		// DirEntry::metadata does not follow symlinks.
		isExecutable := false
		if info, err := e.Info(); err == nil {
			isExecutable = info.Mode().Perm()&0o111 != 0
		}

		hour := "0"
		if period == "hourly" {
			hour = "*"
		}
		weekday := "*"
		if period == "weekly" {
			weekday = "0"
		}
		c.jobs = append(c.jobs, cronJob{
			Minute:  "*",
			Hour:    hour,
			Day:     "*",
			Month:   "*",
			Weekday: weekday,
			Command: name,
			Source:  cronSource{sourceSystem, fmt.Sprintf("%s/%s", dir, name)},
			Enabled: isExecutable,
			RawLine: fmt.Sprintf("@%s %s", period, name),
		})
	}
}

// readToString mirrors fs::read_to_string, which fails on invalid UTF-8.
func readToString(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	s := string(data)
	if !utf8.ValidString(s) {
		return "", false
	}
	return s, true
}

func (c *Component) loadCrontab(path string) {
	if content, ok := readToString(path); ok {
		for _, line := range tui.RustLines(content) {
			if job, ok := parseCronLine(line, cronSource{sourceSystem, path}); ok {
				c.jobs = append(c.jobs, job)
			}
		}
	}
}

func (c *Component) loadCrontabFile(path string) {
	if content, ok := readToString(path); ok {
		source := cronSource{sourceSystem, strings.ToValidUTF8(path, "�")}
		for _, line := range tui.RustLines(content) {
			if job, ok := parseCronLine(line, source); ok {
				c.jobs = append(c.jobs, job)
			}
		}
	}
}

func (c *Component) loadUserCrontabs() {
	if _, err := os.Stat(userCrontabsDir); err != nil {
		return
	}

	for _, e := range readDirUnsorted(userCrontabsDir) {
		username := strings.ToValidUTF8(e.Name(), "�")
		if content, ok := readToString(filepath.Join(userCrontabsDir, e.Name())); ok {
			for _, line := range tui.RustLines(content) {
				if job, ok := parseCronLine(line, cronSource{sourceUser, username}); ok {
					c.jobs = append(c.jobs, job)
				}
			}
		}
	}
}

func parseCronLine(line string, source cronSource) (cronJob, bool) {
	line = strings.TrimSpace(line)

	// Skip empty lines and comments
	if line == "" || strings.HasPrefix(line, "#") {
		return cronJob{}, false
	}

	// Skip variable assignments
	if strings.Contains(line, "=") && !strings.Contains(line, " ") {
		return cronJob{}, false
	}

	parts := strings.Fields(line)

	// Handle special time specifications
	if strings.HasPrefix(parts[0], "@") {
		var minute, hour, day, month, weekday string
		switch parts[0] {
		case "@reboot":
			minute, hour, day, month, weekday = "@reboot", "-", "-", "-", "-"
		case "@yearly", "@annually":
			minute, hour, day, month, weekday = "0", "0", "1", "1", "*"
		case "@monthly":
			minute, hour, day, month, weekday = "0", "0", "1", "*", "*"
		case "@weekly":
			minute, hour, day, month, weekday = "0", "0", "*", "*", "0"
		case "@daily", "@midnight":
			minute, hour, day, month, weekday = "0", "0", "*", "*", "*"
		case "@hourly":
			minute, hour, day, month, weekday = "0", "*", "*", "*", "*"
		default:
			return cronJob{}, false
		}

		return cronJob{
			Minute:  minute,
			Hour:    hour,
			Day:     day,
			Month:   month,
			Weekday: weekday,
			Command: strings.Join(parts[1:], " "),
			Source:  source,
			Enabled: true,
			RawLine: line,
		}, true
	}

	// Standard cron format: min hour day month weekday command
	if len(parts) >= 6 {
		// Check if 6th field is a username (system crontab format)
		cmdStart := 5
		if source.Kind == sourceSystem && (source.Value == "/etc/crontab" || strings.HasPrefix(source.Value, "/etc/cron.d")) {
			cmdStart = 6 // Skip username field
		}

		if len(parts) > cmdStart {
			return cronJob{
				Minute:  parts[0],
				Hour:    parts[1],
				Day:     parts[2],
				Month:   parts[3],
				Weekday: parts[4],
				Command: strings.Join(parts[cmdStart:], " "),
				Source:  source,
				Enabled: true,
				RawLine: line,
			}, true
		}
	}

	return cronJob{}, false
}

func (c *Component) filteredJobs() []*cronJob {
	var out []*cronJob
	for i := range c.jobs {
		job := &c.jobs[i]
		var keep bool
		switch c.filter {
		case filterAll:
			keep = true
		case filterSystem:
			keep = job.Source.Kind == sourceSystem
		case filterUser:
			keep = job.Source.Kind == sourceUser
		case filterHourly:
			keep = strings.Contains(job.RawLine, "hourly") || (job.Minute == "0" && job.Hour == "*")
		case filterDaily:
			keep = strings.Contains(job.RawLine, "daily") || (job.Hour == "0" && job.Day == "*")
		case filterWeekly:
			keep = strings.Contains(job.RawLine, "weekly") || job.Weekday != "*"
		case filterMonthly:
			keep = strings.Contains(job.RawLine, "monthly") || (job.Day == "1" && job.Month == "*")
		}
		if keep {
			out = append(out, job)
		}
	}
	return out
}

func (c *Component) selectedJob() *cronJob {
	filtered := c.filteredJobs()
	if i, ok := c.listState.Selected(); ok && i < len(filtered) {
		return filtered[i]
	}
	return nil
}

func formatSchedule(job *cronJob) string {
	if job.Minute == "@reboot" {
		return "At reboot"
	}

	// Try to create human-readable schedule
	var parts []string

	if job.Minute != "*" && job.Minute != "0" {
		parts = append(parts, fmt.Sprintf(":%s", job.Minute))
	}

	if job.Hour == "*" {
		parts = append(parts, "Every hour")
	} else {
		parts = append(parts, fmt.Sprintf("%s:00", job.Hour))
	}

	if job.Day != "*" {
		parts = append(parts, fmt.Sprintf("day %s", job.Day))
	}

	if job.Weekday != "*" {
		var dayName string
		switch job.Weekday {
		case "0", "7":
			dayName = "Sun"
		case "1":
			dayName = "Mon"
		case "2":
			dayName = "Tue"
		case "3":
			dayName = "Wed"
		case "4":
			dayName = "Thu"
		case "5":
			dayName = "Fri"
		case "6":
			dayName = "Sat"
		default:
			dayName = job.Weekday
		}
		parts = append(parts, dayName)
	}

	if len(parts) == 0 {
		return fmt.Sprintf("%s %s %s %s %s", job.Minute, job.Hour, job.Day, job.Month, job.Weekday)
	}
	return strings.Join(parts, " ")
}

func sourceDisplay(source cronSource) (string, tui.Color) {
	if source.Kind == sourceUser {
		return source.Value, tui.White
	}
	path := source.Value
	switch {
	case strings.Contains(path, "hourly"):
		return "hourly", tui.Cyan
	case strings.Contains(path, "daily"):
		return "daily", tui.Green
	case strings.Contains(path, "weekly"):
		return "weekly", tui.Yellow
	case strings.Contains(path, "monthly"):
		return "monthly", tui.Magenta
	default:
		return "system", tui.Blue
	}
}

func sourcePath(source cronSource) string {
	if source.Kind == sourceUser {
		return fmt.Sprintf("/var/spool/cron/crontabs/%s", source.Value)
	}
	return source.Value
}

// truncateCommand mirrors `if len > 60 { format!("{}...", &cmd[..57]) }`.
// Rust slices by byte and would panic inside a multi-byte character; here
// the cut is moved back to the previous character boundary instead.
func truncateCommand(cmd string) string {
	if len(cmd) <= 60 {
		return cmd
	}
	cut := 57
	for cut > 0 && !utf8.RuneStart(cmd[cut]) {
		cut--
	}
	return cmd[:cut] + "..."
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	filteredLen := len(c.filteredJobs())

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
	case key.Code == tui.KeyTab:
		switch c.filter {
		case filterAll:
			c.filter = filterSystem
		case filterSystem:
			c.filter = filterUser
		case filterUser:
			c.filter = filterHourly
		case filterHourly:
			c.filter = filterDaily
		case filterDaily:
			c.filter = filterWeekly
		case filterWeekly:
			c.filter = filterMonthly
		case filterMonthly:
			c.filter = filterAll
		}
		c.listState.Select(0)
	case key.IsF(5):
		c.loadCronJobs()
		c.statusMessage = &status{"Cron jobs refreshed", false}
	}
	return nil
}

var filterTexts = [...]string{
	filterAll:     "[All] System User Hourly Daily Weekly Monthly",
	filterSystem:  " All [System] User Hourly Daily Weekly Monthly",
	filterUser:    " All System [User] Hourly Daily Weekly Monthly",
	filterHourly:  " All System User [Hourly] Daily Weekly Monthly",
	filterDaily:   " All System User Hourly [Daily] Weekly Monthly",
	filterWeekly:  " All System User Hourly Daily [Weekly] Monthly",
	filterMonthly: " All System User Hourly Daily Weekly [Monthly]",
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(4), tui.Min(10), tui.Length(3))

	// Filter bar
	filterText := filterTexts[c.filter]

	filteredJobs := c.filteredJobs()
	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(60), tui.Percentage(40))

	filterBar := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Filter ", ui.LabelStyle()),
			tui.Raw(filterText),
		),
		tui.LineSpan(tui.Styled(
			"Rotate filters with Tab to isolate periodic or source-based jobs.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Cron job manager")))
	f.RenderWidget(filterBar, header[0])

	systemCount, userCount := 0, 0
	for i := range c.jobs {
		if c.jobs[i].Source.Kind == sourceSystem {
			systemCount++
		} else {
			userCount++
		}
	}
	runtime := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Visible ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(len(filteredJobs)), ui.BadgeNeutral()),
		),
		tui.LineFrom(
			tui.Styled("System ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(systemCount), ui.BadgeSuccess()),
			tui.Raw(" "),
			tui.Styled("User ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(userCount), ui.BadgeNeutral()),
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(runtime, header[1])

	content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(64), tui.Percentage(36))

	items := make([]tui.ListItem, 0, len(filteredJobs))
	for _, job := range filteredJobs {
		sourceName, sourceColor := sourceDisplay(job.Source)
		dot, dotColor := "○", tui.Red
		if job.Enabled {
			dot, dotColor = "●", tui.Green
		}
		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				tui.Styled(fmt.Sprintf("%-10s", sourceName), tui.NewStyle().FG(sourceColor)),
				tui.Styled(dot, tui.NewStyle().FG(dotColor)),
				tui.Raw(" "),
				tui.Styled(fmt.Sprintf("%-20s", formatSchedule(job)), tui.NewStyle().FG(tui.Yellow)),
			),
			tui.LineFrom(
				tui.Styled("    ", tui.NewStyle()),
				tui.Raw(truncateCommand(job.Command)),
			),
		))
	}

	list := tui.NewList(items).
		Block(ui.PanelAltBlock(ui.PanelTitle("Scheduled jobs"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")

	state := c.listState
	list.RenderStateful(content[0], f.Buffer(), &state)

	var inspectorLines []tui.Line
	if job := c.selectedJob(); job != nil {
		enabled := tui.Styled(" NO ", ui.BadgeWarning())
		if job.Enabled {
			enabled = tui.Styled(" YES ", ui.BadgeSuccess())
		}
		inspectorLines = []tui.Line{
			tui.LineFrom(
				tui.Styled("JOB", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(formatSchedule(job), ui.TitleStyle()),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Enabled ", ui.LabelStyle()),
				enabled,
			),
			tui.LineFrom(
				tui.Styled("Command ", ui.LabelStyle()),
				tui.Raw(job.Command),
			),
			tui.LineFrom(
				tui.Styled("Source ", ui.LabelStyle()),
				tui.Raw(sourcePath(job.Source)),
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Raw expression", ui.EyebrowStyle())),
			tui.LineStr(job.RawLine),
		}
	} else {
		inspectorLines = []tui.Line{
			tui.LineSpan(tui.Styled("No job selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Choose a cron entry to inspect its source and raw expression."),
		}
	}
	f.RenderWidget(
		tui.ParagraphLines(inspectorLines...).
			Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
		content[1],
	)

	var statusContent tui.Line
	if c.statusMessage != nil {
		style := ui.SuccessStyle()
		if c.statusMessage.isError {
			style = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.statusMessage.text, style))
	} else if job := c.selectedJob(); job != nil {
		statusContent = tui.LineFrom(
			tui.Styled("Source: ", ui.MutedStyle()),
			tui.Raw(sourcePath(job.Source)),
		)
	} else {
		statusContent = tui.LineSpan(tui.Styled("No job selected", ui.MutedStyle()))
	}

	statusPara := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(statusPara, chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{{Key: "Tab", Desc: "Filter"}, {Key: "↑/↓", Desc: "Navigate"}, {Key: "F5", Desc: "Refresh"}}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.loadCronJobs() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
