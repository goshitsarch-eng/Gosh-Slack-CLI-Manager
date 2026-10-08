// Package backup implements the Backup & Restore tab: choosing which
// configuration files go into a snapshot, listing existing snapshots, and
// creating, restoring and deleting them.
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

const backupDir = "/var/backups/slackware-cli-manager"

var sensitiveFiles = []string{"/etc/shadow"}

// configFiles are the predefined config files to back up.
var configFiles = [][2]string{
	{"/etc/slackpkg/slackpkg.conf", "Slackpkg configuration"},
	{"/etc/slackpkg/mirrors", "Slackpkg mirrors"},
	{"/etc/sbotools/sbotools.conf", "sbotools configuration"},
	{"/etc/lilo.conf", "LILO bootloader configuration"},
	{"/etc/fstab", "Filesystem table"},
	{"/etc/rc.d/rc.local", "Local startup script"},
	{"/etc/rc.d/rc.inet1.conf", "Network configuration"},
	{"/etc/inittab", "Init configuration"},
	{"/etc/passwd", "User accounts"},
	{"/etc/group", "Group definitions"},
	{"/etc/shadow", "Password hashes"},
	{"/etc/sudoers", "Sudo configuration"},
	{"/etc/hosts", "Host mappings"},
	{"/etc/resolv.conf", "DNS configuration"},
}

// ConfigFile is a file that can be included in backups.
type ConfigFile struct {
	Path        string
	Description string
	Include     bool
}

// entry describes one backup snapshot directory.
type entry struct {
	name      string
	path      string
	timestamp stamp
	size      uint64
	fileCount int
}

type mode uint8

const (
	modeCreate mode = iota
	modeRestore
)

type status struct {
	message string
	isError bool
}

// Component is the Backup & Restore tab.
type Component struct {
	mode          mode
	configFiles   []ConfigFile
	backups       []entry
	listState     tui.ListState
	status        *status
	showConfirm   bool
	pendingAction *msg.BackupAction
}

// New creates the component. Like the original it ensures the backup
// directory exists (creating it with mode 0700) while loading snapshots.
func New() *Component {
	files := make([]ConfigFile, 0, len(configFiles))
	for _, f := range configFiles {
		files = append(files, ConfigFile{
			Path:        f[0],
			Description: f[1],
			Include:     !isSensitive(f[0]),
		})
	}
	c := &Component{
		mode:        modeCreate,
		configFiles: files,
	}
	c.loadBackups()
	return c
}

func ensureBackupDir() error {
	if err := os.MkdirAll(backupDir, 0o777); err != nil {
		return err
	}
	return os.Chmod(backupDir, 0o700)
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

func (c *Component) loadBackups() {
	c.backups = c.backups[:0]

	if ensureBackupDir() != nil {
		return
	}

	if entries, err := readDirOrder(backupDir); err == nil {
		for _, e := range entries {
			path := filepath.Join(backupDir, e.Name())
			if info, err := os.Stat(path); err != nil || !info.IsDir() {
				continue
			}
			name := e.Name()

			// Parse timestamp from directory name (format: backup_YYYYMMDD_HHMMSS)
			var ts stamp
			if strings.HasPrefix(name, "backup_") {
				tsStr := trimStartMatches(name, "backup_")
				if parsed, ok := parseBackupStamp(tsStr); ok {
					ts = parsed
				} else {
					ts = nowStamp()
				}
			} else {
				ts = nowStamp()
			}

			count, size := calculateBackupStats(path)
			c.backups = append(c.backups, entry{
				name:      name,
				path:      path,
				timestamp: ts,
				size:      size,
				fileCount: count,
			})
		}
	}

	sort.SliceStable(c.backups, func(i, j int) bool {
		return c.backups[j].timestamp.less(c.backups[i].timestamp)
	})
}

func calculateBackupStats(path string) (int, uint64) {
	count := 0
	var size uint64
	if entries, err := readDirOrder(path); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil {
				count++
				size += uint64(info.Size())
			}
		}
	}
	return count, size
}

// Execute performs a backup action. It runs off the UI goroutine.
func Execute(action msg.BackupAction, files []ConfigFile) (string, error) {
	switch action.Kind {
	case msg.BackupRestore:
		return restoreBackup(action.Path)
	case msg.BackupDelete:
		return deleteBackup(action.Path)
	default:
		return createBackup(files)
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }

func ioErr(err error) error { return utils.PlainIOError(err) }

func createBackup(files []ConfigFile) (string, error) {
	if err := ensureBackupDir(); err != nil {
		return "", ioErr(err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, "backup_"+timestamp)

	if err := os.MkdirAll(backupPath, 0o777); err != nil {
		return "", ioErr(err)
	}
	if err := os.Chmod(backupPath, 0o700); err != nil {
		return "", ioErr(err)
	}

	backedUp, failed := 0, 0
	for _, f := range files {
		if !f.Include {
			continue
		}
		if _, err := os.Stat(f.Path); err != nil {
			continue
		}

		// Create destination path preserving directory structure
		destName := trimStartMatches(strings.ReplaceAll(f.Path, "/", "_"), "_")
		dest := filepath.Join(backupPath, destName)

		if err := copyFile(f.Path, dest); err == nil {
			backedUp++
			_ = os.Chmod(dest, 0o600)
		} else {
			failed++
		}
	}

	if backedUp > 0 {
		return fmt.Sprintf("Backup created: %d files backed up, %d failed", backedUp, failed), nil
	}
	_ = os.Remove(backupPath)
	return "", stringError("No files were backed up")
}

func restoreBackup(backupPath string) (string, error) {
	restored, failed := 0, 0

	if entries, err := readDirOrder(backupPath); err == nil {
		for _, e := range entries {
			filename := e.Name()

			// Convert filename back to path
			originalPath := "/" + strings.ReplaceAll(filename, "_", "/")

			// Verify this is a known config file
			if slices.ContainsFunc(configFiles, func(f [2]string) bool { return f[0] == originalPath }) {
				if err := copyFile(filepath.Join(backupPath, filename), originalPath); err == nil {
					restored++
				} else {
					failed++
				}
			}
		}
	}

	if restored == 0 && failed == 0 {
		return "", stringError("Backup did not contain any restorable files")
	}

	return fmt.Sprintf("Restore complete: %d files restored, %d failed", restored, failed), nil
}

func deleteBackup(backupPath string) (string, error) {
	if err := removeDirAll(backupPath); err != nil {
		return "", ioErr(err)
	}
	return "Backup deleted successfully", nil
}

func formatSize(bytes uint64) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.status = &status{message: message, isError: isError}
}

// RefreshBackups reloads the list of backups.
func (c *Component) RefreshBackups() { c.loadBackups() }

// ConfigFiles returns the backup file selection.
func (c *Component) ConfigFiles() []ConfigFile { return c.configFiles }

func isSensitive(path string) bool { return slices.Contains(sensitiveFiles, path) }

func (c *Component) listLen() int {
	if c.mode == modeCreate {
		return len(c.configFiles)
	}
	return len(c.backups)
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.showConfirm {
		switch {
		case key.IsChar('y') || key.IsChar('Y'):
			c.showConfirm = false
			if action := c.pendingAction; action != nil {
				c.pendingAction = nil
				return msg.BackupActionMsg{Action: *action}
			}
		case key.IsChar('n') || key.IsChar('N') || key.Code == tui.KeyEsc:
			c.showConfirm = false
			c.pendingAction = nil
		}
		return nil
	}

	switch {
	case key.Code == tui.KeyTab:
		if c.mode == modeCreate {
			c.mode = modeRestore
		} else {
			c.mode = modeCreate
		}
		c.listState.Select(0)
	case key.Code == tui.KeyUp || key.IsChar('k'):
		n := c.listLen()
		if selected, ok := c.listState.Selected(); ok {
			if selected > 0 {
				c.listState.Select(selected - 1)
			}
		} else if n > 0 {
			c.listState.Select(0)
		}
	case key.Code == tui.KeyDown || key.IsChar('j'):
		n := c.listLen()
		if selected, ok := c.listState.Selected(); ok {
			if selected < max(n-1, 0) {
				c.listState.Select(selected + 1)
			}
		} else if n > 0 {
			c.listState.Select(0)
		}
	case key.IsChar(' ') && c.mode == modeCreate:
		if selected, ok := c.listState.Selected(); ok && selected < len(c.configFiles) {
			c.configFiles[selected].Include = !c.configFiles[selected].Include
		}
	case key.IsChar('a') && c.mode == modeCreate:
		allSelected := true
		for _, f := range c.configFiles {
			if !f.Include {
				allSelected = false
				break
			}
		}
		for i := range c.configFiles {
			c.configFiles[i].Include = !allSelected
		}
	case key.Code == tui.KeyEnter:
		if c.mode == modeCreate {
			c.pendingAction = &msg.BackupAction{Kind: msg.BackupCreate}
			c.showConfirm = true
		} else if selected, ok := c.listState.Selected(); ok && selected < len(c.backups) {
			c.pendingAction = &msg.BackupAction{Kind: msg.BackupRestore, Path: c.backups[selected].path}
			c.showConfirm = true
		}
	case key.IsChar('d') && c.mode == modeRestore:
		if selected, ok := c.listState.Selected(); ok && selected < len(c.backups) {
			c.pendingAction = &msg.BackupAction{Kind: msg.BackupDelete, Path: c.backups[selected].path}
			c.showConfirm = true
		}
	case key.IsF(5):
		c.loadBackups()
		c.status = &status{message: "Backup list refreshed"}
	}
	return nil
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical, tui.Length(3), tui.Min(10), tui.Length(3))

	// Mode tabs
	modeText := "[Create Backup]  Restore Backup"
	if c.mode == modeRestore {
		modeText = " Create Backup  [Restore Backup]"
	}
	modeBar := tui.ParagraphLine(tui.LineFrom(
		tui.Styled("Mode ", ui.LabelStyle()),
		tui.Raw(modeText),
		tui.Raw(" "),
		tui.Styled(fmt.Sprintf(" %d snapshots ", len(c.backups)), ui.BadgeNeutral()),
	)).Block(ui.Panel(ui.PanelTitle("Vault")))
	f.RenderWidget(modeBar, chunks[0])

	// Content
	if c.mode == modeCreate {
		c.renderCreateMode(f, chunks[1])
	} else {
		c.renderRestoreMode(f, chunks[1])
	}

	// Status bar
	var statusContent tui.Line
	switch {
	case c.showConfirm:
		actionDesc := "Confirm action?"
		if c.pendingAction != nil {
			switch c.pendingAction.Kind {
			case msg.BackupCreate:
				actionDesc = "Create backup?"
			case msg.BackupRestore:
				actionDesc = "Restore this backup?"
			case msg.BackupDelete:
				actionDesc = "Delete this backup?"
			}
		}
		statusContent = tui.LineFrom(
			tui.Styled(actionDesc, ui.WarningStyle()),
			tui.Raw(" [Y]es / [N]o"),
		)
	case c.status != nil:
		st := ui.SuccessStyle()
		if c.status.isError {
			st = ui.ErrorStyle()
		}
		statusContent = tui.LineSpan(tui.Styled(c.status.message, st))
	default:
		statusContent = tui.LineSpan(tui.Styled(
			fmt.Sprintf("Backup directory: %s | sensitive files are opt-in", backupDir),
			ui.MutedStyle(),
		))
	}

	statusBar := tui.ParagraphLine(statusContent).Block(ui.PanelAltBlock(ui.PanelTitle("Status")))
	f.RenderWidget(statusBar, chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	if c.mode == modeCreate {
		return []msg.KeyHelp{
			{Key: "Tab", Desc: "Switch Mode"},
			{Key: "Space", Desc: "Toggle"},
			{Key: "a", Desc: "Select All"},
			{Key: "Enter", Desc: "Backup"},
		}
	}
	return []msg.KeyHelp{
		{Key: "Tab", Desc: "Switch Mode"},
		{Key: "Enter", Desc: "Restore"},
		{Key: "d", Desc: "Delete"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() { c.loadBackups() }

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}

func (c *Component) selectedConfigFile() (ConfigFile, bool) {
	if c.mode != modeCreate {
		return ConfigFile{}, false
	}
	if idx, ok := c.listState.Selected(); ok && idx < len(c.configFiles) {
		return c.configFiles[idx], true
	}
	return ConfigFile{}, false
}

func (c *Component) selectedBackup() (entry, bool) {
	if c.mode != modeRestore {
		return entry{}, false
	}
	if idx, ok := c.listState.Selected(); ok && idx < len(c.backups) {
		return c.backups[idx], true
	}
	return entry{}, false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (c *Component) renderCreateMode(f *tui.Frame, area tui.Rect) {
	content := tui.Split(area, tui.Horizontal, tui.Percentage(62), tui.Percentage(38))

	items := make([]tui.ListItem, 0, len(c.configFiles))
	for _, file := range c.configFiles {
		checkbox := "   "
		checkboxStyle := ui.BadgeNeutral()
		if file.Include {
			checkbox = " ✓ "
			checkboxStyle = ui.BadgeSuccess()
		}
		exists := pathExists(file.Path)
		existsBadge := tui.Styled(" MISSING ", ui.BadgeWarning())
		pathStyle := ui.ValueStyle().Add(tui.Dim)
		if exists {
			existsBadge = tui.Styled(" READY ", ui.BadgeSuccess())
			pathStyle = ui.ValueStyle()
		}
		gap, sensitivity := tui.Raw(""), tui.Raw("")
		if isSensitive(file.Path) {
			gap = tui.Raw(" ")
			sensitivity = tui.Styled(" SENSITIVE ", ui.BadgeWarning())
		}

		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				tui.Styled(checkbox, checkboxStyle),
				tui.Raw(" "),
				tui.Styled(file.Path, pathStyle),
				tui.Raw(" "),
				existsBadge,
				gap,
				sensitivity,
			),
			tui.LineSpan(tui.Styled("    "+file.Description, ui.MutedStyle())),
		))
	}

	list := tui.NewList(items).
		Block(ui.Panel(ui.PanelTitle("Selected files"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")

	st := c.listState
	list.RenderStateful(content[0], f.Buffer(), &st)

	total := len(c.configFiles)
	selected, sensitive := 0, 0
	for _, file := range c.configFiles {
		if file.Include {
			selected++
			if isSensitive(file.Path) {
				sensitive++
			}
		}
	}

	var lines []tui.Line
	if file, ok := c.selectedConfigFile(); ok {
		included := tui.Styled(" NO ", ui.BadgeWarning())
		if file.Include {
			included = tui.Styled(" YES ", ui.BadgeSuccess())
		}
		sensitivity := tui.Styled(" NORMAL ", ui.BadgeNeutral())
		if isSensitive(file.Path) {
			sensitivity = tui.Styled(" HIGH ", ui.BadgeWarning())
		}
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("SELECTION", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(file.Path, ui.TitleStyle()),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Purpose ", ui.LabelStyle()),
				tui.Raw(file.Description),
			),
			tui.LineFrom(
				tui.Styled("Included ", ui.LabelStyle()),
				included,
			),
			tui.LineFrom(
				tui.Styled("Sensitivity ", ui.LabelStyle()),
				sensitivity,
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Selected set ", ui.LabelStyle()),
				tui.Styled(fmt.Sprintf(" %d / %d ", selected, total), ui.BadgeNeutral()),
			),
			tui.LineFrom(
				tui.Styled("Sensitive included ", ui.LabelStyle()),
				tui.Styled(fmt.Sprintf(" %d ", sensitive), ui.BadgeWarning()),
			),
		}
	} else {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled("No file selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Move through the vault roster to inspect backup coverage."),
		}
	}

	summary := tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector")))
	f.RenderWidget(summary, content[1])
}

func (c *Component) renderRestoreMode(f *tui.Frame, area tui.Rect) {
	if len(c.backups) == 0 {
		empty := tui.ParagraphLine(tui.LineSpan(tui.Styled("No backups found", ui.MutedStyle()))).
			Block(ui.Panel(ui.PanelTitle("Available backups")))
		f.RenderWidget(empty, area)
		return
	}

	content := tui.Split(area, tui.Horizontal, tui.Percentage(60), tui.Percentage(40))

	items := make([]tui.ListItem, 0, len(c.backups))
	for _, b := range c.backups {
		items = append(items, tui.ListItemLines(
			tui.LineFrom(
				tui.Styled(b.timestamp.format(), ui.AccentStyle().Add(tui.Bold)),
				tui.Styled("  ", tui.NewStyle()),
				tui.Styled(b.name, ui.MutedStyle()),
			),
			tui.LineFrom(
				tui.Styled("    Files ", ui.MutedStyle()),
				tui.Raw(fmt.Sprintf("%d", b.fileCount)),
				tui.Styled("  Size ", ui.MutedStyle()),
				tui.Raw(formatSize(b.size)),
			),
		))
	}

	list := tui.NewList(items).
		Block(ui.Panel(ui.PanelTitle("Available backups"))).
		HighlightStyle(ui.ListSelected()).
		HighlightSymbol("▶ ")

	st := c.listState
	list.RenderStateful(content[0], f.Buffer(), &st)

	var lines []tui.Line
	if b, ok := c.selectedBackup(); ok {
		lines = []tui.Line{
			tui.LineFrom(
				tui.Styled("SNAPSHOT", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(b.name, ui.TitleStyle()),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Created ", ui.LabelStyle()),
				tui.Raw(b.timestamp.format()),
			),
			tui.LineFrom(
				tui.Styled("Files ", ui.LabelStyle()),
				tui.Styled(fmt.Sprintf("%d", b.fileCount), ui.BadgeNeutral()),
				tui.Raw(" "),
				tui.Styled("Size ", ui.LabelStyle()),
				tui.Styled(formatSize(b.size), ui.BadgeSuccess()),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("Path ", ui.LabelStyle()),
				tui.Raw(b.path),
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled(
				"Restore replaces known managed files; delete removes the snapshot directory.",
				ui.SubtitleStyle(),
			)),
		}
	} else {
		lines = []tui.Line{
			tui.LineSpan(tui.Styled("No backup selected", ui.MutedStyle())),
			tui.LineStr(""),
			tui.LineStr("Choose a snapshot to inspect restore scope."),
		}
	}

	f.RenderWidget(
		tui.ParagraphLines(lines...).Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
		content[1],
	)
}
