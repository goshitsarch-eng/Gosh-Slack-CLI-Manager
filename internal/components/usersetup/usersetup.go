// Package usersetup implements the user setup tab: a form to create a user
// account, pick its supplementary groups and optionally switch the default
// runlevel to the graphical login.
package usersetup

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

// defaultGroups are the groups offered (and preselected) for new users.
var defaultGroups = [...]struct{ name, desc string }{
	{"wheel", "Administrative access (sudo)"},
	{"floppy", "Floppy disk access"},
	{"audio", "Audio devices"},
	{"video", "Video devices"},
	{"cdrom", "CD/DVD drives"},
	{"plugdev", "Pluggable devices"},
	{"power", "Power management"},
	{"netdev", "Network devices"},
	{"lp", "Printer access"},
	{"scanner", "Scanner access"},
}

type group struct {
	name     string
	selected bool
}

// Component is the tab component.
type Component struct {
	username        string
	password        string
	confirmPassword string
	groups          []group
	changeRunlevel  bool
	currentField    int
	isRunning       bool
	errorMessage    *string
	successMessage  *string
}

func newGroups() []group {
	groups := make([]group, 0, len(defaultGroups))
	for _, g := range defaultGroups {
		groups = append(groups, group{name: g.name, selected: true})
	}
	return groups
}

// New creates the component.
func New() *Component {
	return &Component{
		groups:         newGroups(),
		changeRunlevel: true,
	}
}

// Reset clears the form back to its initial state.
func (c *Component) Reset() {
	c.username = ""
	c.password = ""
	c.confirmPassword = ""
	c.groups = newGroups()
	c.changeRunlevel = true
	c.currentField = 0
	c.isRunning = false
	c.errorMessage = nil
	c.successMessage = nil
}

// Validate checks the form, returning the first problem found.
func (c *Component) Validate() error {
	if c.username == "" {
		return validationError("Username cannot be empty")
	}
	if strings.Contains(c.username, " ") {
		return validationError("Username cannot contain spaces")
	}
	if c.password == "" {
		return validationError("Password cannot be empty")
	}
	if c.password != c.confirmPassword {
		return validationError("Passwords do not match")
	}
	// Rust's String::len counts bytes.
	if len(c.password) < 4 {
		return validationError("Password must be at least 4 characters")
	}
	return nil
}

type validationError string

func (e validationError) Error() string { return string(e) }

// SelectedGroups returns the selected supplementary groups.
func (c *Component) SelectedGroups() []string {
	var out []string
	for _, g := range c.groups {
		if g.selected {
			out = append(out, g.name)
		}
	}
	return out
}

// Username returns the entered user name.
func (c *Component) Username() string { return c.username }

// Password returns the entered password.
func (c *Component) Password() string { return c.password }

// ShouldChangeRunlevel reports whether to switch the default runlevel to 4.
func (c *Component) ShouldChangeRunlevel() bool { return c.changeRunlevel }

// SetError shows an error.
func (c *Component) SetError(message string) {
	c.errorMessage = &message
	c.isRunning = false
}

// SetSuccess shows a success message.
func (c *Component) SetSuccess(message string) {
	c.successMessage = &message
	c.isRunning = false
}

// StartCreate marks the creation as running and clears old messages.
func (c *Component) StartCreate() {
	c.errorMessage = nil
	c.successMessage = nil
	c.isRunning = true
}

// totalFields counts username, password, confirm, the groups and runlevel.
func (c *Component) totalFields() int { return 3 + len(c.groups) + 1 }

func popRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	if c.isRunning {
		return nil
	}

	switch {
	case key.Code == tui.KeyTab || key.Code == tui.KeyDown:
		c.currentField = (c.currentField + 1) % c.totalFields()
		return nil
	case key.Code == tui.KeyBackTab || key.Code == tui.KeyUp:
		if c.currentField == 0 {
			c.currentField = c.totalFields() - 1
		} else {
			c.currentField--
		}
		return nil
	case key.IsChar(' ') && c.currentField >= 3:
		idx := c.currentField - 3
		if idx < len(c.groups) {
			c.groups[idx].selected = !c.groups[idx].selected
		} else if idx == len(c.groups) {
			c.changeRunlevel = !c.changeRunlevel
		}
		return nil
	case key.Code == tui.KeyChar && !key.Has(tui.ModControl):
		switch c.currentField {
		case 0:
			c.username += string(key.Rune)
		case 1:
			c.password += string(key.Rune)
		case 2:
			c.confirmPassword += string(key.Rune)
		}
		return nil
	case key.Code == tui.KeyBackspace:
		switch c.currentField {
		case 0:
			c.username = popRune(c.username)
		case 1:
			c.password = popRune(c.password)
		case 2:
			c.confirmPassword = popRune(c.confirmPassword)
		}
		return nil
	case key.Code == tui.KeyEnter:
		if err := c.Validate(); err != nil {
			e := err.Error()
			c.errorMessage = &e
			return nil
		}
		c.StartCreate()
		return msg.CreateUser{}
	case key.IsChar('r') && key.Has(tui.ModControl):
		c.Reset()
		return nil
	}
	return nil
}

func fieldStyle(active bool) tui.Style {
	if active {
		return ui.InputActive()
	}
	return ui.InputInactive()
}

func borderStyle(active bool) tui.Style {
	if active {
		return ui.BorderFocusedStyle()
	}
	return ui.BorderStyle()
}

func inputField(title, value string, active bool) tui.Paragraph {
	block := tui.NewBlock().
		Borders(tui.BordersAll).
		TitleStr(title).
		BorderStyle(borderStyle(active))
	return tui.ParagraphStr(value).Style(fieldStyle(active)).Block(block)
}

func stars(s string) string { return strings.Repeat("*", utf8.RuneCountInString(s)) }

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	titleHeight := 4
	if area.Height < 20 {
		titleHeight = 0
	}
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(titleHeight), // Title
		tui.Min(0),              // Form; reserve space for validation feedback
		tui.Length(3),           // Status/Error
	)

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	title := tui.ParagraphLines(
		tui.LineSpan(tui.Styled("User Setup", ui.TitleStyle())),
		tui.LineSpan(tui.Styled("Create accounts and assign groups.", ui.SubtitleStyle())),
	).Block(ui.Panel(ui.PanelTitle("Identity")))
	f.RenderWidget(title, header[0])

	selectedGroups := 0
	for _, g := range c.groups {
		if g.selected {
			selectedGroups++
		}
	}
	runlevelBadge := tui.Styled(" TEXT LOGIN ", ui.BadgeNeutral())
	if c.changeRunlevel {
		runlevelBadge = tui.Styled(" GUI LOGIN ", ui.BadgeSuccess())
	}
	runtime := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Selected groups ", ui.LabelStyle()),
			tui.Styled(fmt.Sprint(selectedGroups), ui.BadgeNeutral()),
		),
		tui.LineFrom(
			tui.Styled("Runlevel ", ui.LabelStyle()),
			runlevelBadge,
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(runtime, header[1])

	formChunks := tui.Split(chunks[1], tui.Horizontal,
		tui.Percentage(38),
		tui.Percentage(36),
		tui.Percentage(26),
	)

	// Left side - text inputs
	inputBlock := ui.Panel(ui.PanelTitle("User info"))
	inputInner := inputBlock.Inner(formChunks[0])
	f.RenderWidget(inputBlock, formChunks[0])

	inputChunks := tui.Split(inputInner, tui.Vertical,
		tui.Length(3),
		tui.Length(3),
		tui.Length(3),
	)

	f.RenderWidget(inputField("Username", c.username, c.currentField == 0), inputChunks[0])
	f.RenderWidget(inputField("Password", stars(c.password), c.currentField == 1), inputChunks[1])
	f.RenderWidget(inputField("Confirm Password", stars(c.confirmPassword), c.currentField == 2), inputChunks[2])

	if inputInner.Height < 9 {
		f.RenderWidget(tui.Clear{}, inputInner)
		values := []string{
			"User: " + c.username,
			"Password: " + stars(c.password),
			"Confirm: " + stars(c.confirmPassword),
		}
		lines := make([]tui.Line, 0, len(values))
		for index, value := range values {
			lines = append(lines, tui.StyledLine(value, fieldStyle(c.currentField == index)))
		}
		f.RenderWidget(tui.ParagraphLines(lines...), inputInner)
	}

	// Middle - groups checkboxes
	groupsBlock := ui.Panel(ui.PanelTitle("Groups"))
	groupsInner := groupsBlock.Inner(formChunks[1])
	f.RenderWidget(groupsBlock, formChunks[1])

	var lines []tui.Line
	for i, g := range c.groups {
		checkbox := "[ ]"
		if g.selected {
			checkbox = "[x]"
		}
		desc := ""
		for _, d := range defaultGroups {
			if d.name == g.name {
				desc = d.desc
				break
			}
		}

		style := ui.DefaultStyle()
		if c.currentField == 3+i {
			style = ui.HighlightStyle()
		}

		lines = append(lines, tui.LineFrom(
			tui.Styled(checkbox+" ", style.Add(tui.Bold)),
			tui.Styled(fmt.Sprintf("%-10s", g.name), style),
			tui.Styled(desc, ui.MutedStyle()),
		))
	}

	// Runlevel option
	runlevelCheckbox := "[ ]"
	if c.changeRunlevel {
		runlevelCheckbox = "[x]"
	}
	runlevelStyle := ui.DefaultStyle()
	if c.currentField == 3+len(c.groups) {
		runlevelStyle = ui.HighlightStyle()
	}
	lines = append(lines, tui.LineStr(""))
	lines = append(lines, tui.LineFrom(
		tui.Styled(runlevelCheckbox+" ", runlevelStyle.Add(tui.Bold)),
		tui.Styled("Change runlevel 3→4 (GUI)", runlevelStyle),
	))

	selectedLine := max(c.currentField-3, 0)
	if c.currentField == 3+len(c.groups) {
		selectedLine++
	}
	groupScroll := max(selectedLine+1-groupsInner.Height, 0)
	groupsPara := tui.ParagraphLines(lines...).Scroll(groupScroll&0xffff, 0)
	f.RenderWidget(groupsPara, groupsInner)

	var currentGroup *group
	if c.currentField >= 3 && c.currentField < 3+len(c.groups) {
		currentGroup = &c.groups[c.currentField-3]
	}

	profileName := c.username
	if profileName == "" {
		profileName = "<new user>"
	}

	var passwordBadge tui.Span
	switch {
	case c.password == "":
		passwordBadge = tui.Styled(" EMPTY ", ui.BadgeWarning())
	case c.password == c.confirmPassword:
		passwordBadge = tui.Styled(" MATCH ", ui.BadgeSuccess())
	default:
		passwordBadge = tui.Styled(" MISMATCH ", ui.BadgeWarning())
	}

	var fieldName string
	switch {
	case c.currentField == 0:
		fieldName = "username"
	case c.currentField == 1:
		fieldName = "password"
	case c.currentField == 2:
		fieldName = "confirm password"
	case c.currentField == 3+len(c.groups):
		fieldName = "runlevel"
	default:
		fieldName = "group selection"
	}

	var focusLine tui.Line
	if currentGroup != nil {
		badge := tui.Styled(" EXCLUDED ", ui.BadgeNeutral())
		if currentGroup.selected {
			badge = tui.Styled(" INCLUDED ", ui.BadgeSuccess())
		}
		focusLine = tui.LineFrom(
			tui.Styled(currentGroup.name, ui.TitleStyle()),
			tui.Raw(" "),
			badge,
		)
	} else {
		focusLine = tui.LineSpan(tui.Styled(
			"Move through fields and groups with Tab/Shift+Tab.",
			ui.SubtitleStyle(),
		))
	}

	inspectorLines := []tui.Line{
		tui.LineFrom(
			tui.Styled("PROFILE", ui.BadgeInfo()),
			tui.Raw(" "),
			tui.Styled(profileName, ui.TitleStyle()),
		),
		tui.LineStr(""),
		tui.LineFrom(
			tui.Styled("Password ", ui.LabelStyle()),
			passwordBadge,
		),
		tui.LineFrom(
			tui.Styled("Current field ", ui.LabelStyle()),
			tui.Raw(fieldName),
		),
		tui.LineStr(""),
		tui.LineSpan(tui.Styled("Focus", ui.EyebrowStyle())),
		focusLine,
		tui.LineStr(""),
		tui.LineSpan(tui.Styled(
			"Press Enter only after validation passes and the account plan looks right.",
			ui.SubtitleStyle(),
		)),
	}
	f.RenderWidget(
		tui.ParagraphLines(inspectorLines...).
			Wrap(true).
			Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
		formChunks[2],
	)

	// Status/Error message
	var status tui.Paragraph
	switch {
	case c.errorMessage != nil:
		status = tui.ParagraphStr(*c.errorMessage).Style(ui.ErrorStyle())
	case c.successMessage != nil:
		status = tui.ParagraphStr(*c.successMessage).Style(ui.SuccessStyle())
	case c.isRunning:
		status = tui.ParagraphStr("Creating user...").Style(ui.WarningStyle())
	default:
		status = tui.ParagraphStr("Press Enter to create user, Ctrl+R to reset").Style(ui.MutedStyle())
	}
	f.RenderWidget(status.Block(ui.PanelAltBlock(ui.PanelTitle("Status"))), chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	return []msg.KeyHelp{
		{Key: "Tab", Desc: "Next field"},
		{Key: "Space", Desc: "Toggle"},
		{Key: "Enter", Desc: "Create"},
		{Key: "Ctrl+R", Desc: "Reset"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() {}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
