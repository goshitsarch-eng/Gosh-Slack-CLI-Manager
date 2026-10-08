// Package configeditor implements the configuration file editor tab: pick
// one of the managed Slackware config files and edit it in a text area.
package configeditor

import (
	"errors"
	"strings"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

// configFiles are the config files available for editing.
var configFiles = [...]struct{ path, desc string }{
	{"/etc/slackpkg/slackpkg.conf", "slackpkg configuration"},
	{"/etc/slackpkg/mirrors", "Package mirrors"},
	{"/etc/sbotools/sbotools.conf", "sbotools configuration"},
}

type editorMode uint8

const (
	modeFileSelect editorMode = iota
	modeEditing
)

type statusMessage struct {
	text    string
	isError bool
}

// Component is the tab component.
type Component struct {
	mode          editorMode
	fileListState tui.ListState
	currentFile   *string
	textarea      *tui.TextArea
	isModified    bool
	statusMessage *statusMessage
}

func idleTextArea() *tui.TextArea {
	ta := tui.NewTextArea(nil)
	ta.SetBlock(tui.NewBlock().
		Borders(tui.BordersAll).
		TitleStr("Editor").
		BorderStyle(ui.BorderStyle()))
	return ta
}

// New creates the component.
func New() *Component {
	return &Component{
		mode:          modeFileSelect,
		fileListState: tui.NewListState().WithSelected(0),
		textarea:      idleTextArea(),
	}
}

// IsEditing reports whether a file is open in the editor.
func (c *Component) IsEditing() bool { return c.mode == modeEditing }

func (c *Component) selectedFileIndex() (int, bool) {
	i, ok := c.fileListState.Selected()
	if !ok || i < 0 || i >= len(configFiles) {
		return 0, false
	}
	return i, true
}

// LoadFile opens path in the editor.
func (c *Component) LoadFile(path string) error {
	content, err := utils.ReadFileString(path)
	if err != nil {
		return errors.New("Unable to open " + path + ": " + utils.IOErrorString(err))
	}

	c.textarea = tui.NewTextArea(tui.RustLines(content))
	c.textarea.SetBlock(tui.NewBlock().
		Borders(tui.BordersAll).
		TitleStr("Editing: " + path).
		BorderStyle(ui.BorderFocusedStyle()))

	c.currentFile = &path
	c.mode = modeEditing
	c.isModified = false
	c.statusMessage = nil
	return nil
}

// CloseEditor leaves the editor and returns to the file list.
func (c *Component) CloseEditor() {
	c.mode = modeFileSelect
	c.currentFile = nil
	c.isModified = false
	c.textarea = idleTextArea()
}

// SelectedFile returns the path highlighted in the file list.
func (c *Component) SelectedFile() (string, bool) {
	i, ok := c.selectedFileIndex()
	if !ok {
		return "", false
	}
	return configFiles[i].path, true
}

// SetStatus sets the status message.
func (c *Component) SetStatus(message string, isError bool) {
	c.statusMessage = &statusMessage{text: message, isError: isError}
	if !isError {
		c.isModified = false
	}
}

// SaveRequest returns the open file's path and the content to write.
func (c *Component) SaveRequest() (path, content string, ok bool) {
	if c.currentFile == nil {
		return "", "", false
	}
	return *c.currentFile, strings.Join(c.textarea.Lines(), "\n") + "\n", true
}

// HandleInput implements components.Component.
func (c *Component) HandleInput(key tui.KeyEvent) msg.Message {
	switch c.mode {
	case modeFileSelect:
		switch {
		case key.Code == tui.KeyUp || key.IsChar('k'):
			if selected, ok := c.fileListState.Selected(); ok && selected > 0 {
				c.fileListState.Select(selected - 1)
			}
		case key.Code == tui.KeyDown || key.IsChar('j'):
			if selected, ok := c.fileListState.Selected(); ok && selected < len(configFiles)-1 {
				c.fileListState.Select(selected + 1)
			}
		case key.Code == tui.KeyEnter:
			if path, ok := c.SelectedFile(); ok {
				if err := c.LoadFile(path); err != nil {
					c.statusMessage = &statusMessage{text: "Error: " + err.Error(), isError: true}
				}
			}
		}
		return nil
	default:
		// Handle editor-specific keys
		if key.Has(tui.ModControl) && key.Code == tui.KeyChar {
			switch key.Rune {
			case 's':
				if path, content, ok := c.SaveRequest(); ok {
					return msg.SaveConfig{Path: path, Content: content}
				}
				return nil
			case 'q':
				if c.isModified {
					c.statusMessage = &statusMessage{
						text:    "Unsaved changes! Ctrl+S to save, Ctrl+X to discard",
						isError: true,
					}
				} else {
					c.CloseEditor()
				}
				return nil
			case 'x':
				// Force close without saving
				c.CloseEditor()
				return nil
			}
		}

		// Pass to textarea
		if c.textarea.Input(key) {
			c.isModified = true
		}
		return nil
	}
}

// Render implements components.Component.
func (c *Component) Render(f *tui.Frame, area tui.Rect) {
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(4), // Title
		tui.Min(10),   // Content
		tui.Length(3), // Status
	)

	header := tui.Split(chunks[0], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

	title := tui.ParagraphLines(
		tui.LineSpan(tui.Styled("Configuration Editor", ui.TitleStyle())),
		tui.LineSpan(tui.Styled(
			"Edit Slackware config files inside a safer, atomic-write workflow.",
			ui.SubtitleStyle(),
		)),
	).Block(ui.Panel(ui.PanelTitle("Editor")))
	f.RenderWidget(title, header[0])

	modeLabel := " SELECT "
	if c.mode == modeEditing {
		modeLabel = " EDIT "
	}
	modifiedBadge := tui.Styled(" NO ", ui.BadgeSuccess())
	if c.isModified {
		modifiedBadge = tui.Styled(" YES ", ui.BadgeWarning())
	}
	runtime := tui.ParagraphLines(
		tui.LineFrom(
			tui.Styled("Mode ", ui.LabelStyle()),
			tui.Styled(modeLabel, ui.BadgeNeutral()),
		),
		tui.LineFrom(
			tui.Styled("Modified ", ui.LabelStyle()),
			modifiedBadge,
		),
	).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(runtime, header[1])

	switch c.mode {
	case modeFileSelect:
		content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(58), tui.Percentage(42))

		items := make([]tui.ListItem, 0, len(configFiles))
		for _, file := range configFiles {
			items = append(items, tui.ListItemLines(
				tui.LineSpan(tui.Styled(file.path, ui.DefaultStyle().Add(tui.Bold))),
				tui.LineSpan(tui.Styled("  "+file.desc, ui.MutedStyle())),
			))
		}

		list := tui.NewList(items).
			Block(ui.Panel(ui.PanelTitle("Select file to edit"))).
			HighlightStyle(ui.HighlightStyle().Add(tui.Bold)).
			HighlightSymbol("▸ ")

		state := c.fileListState
		list.RenderStateful(content[0], f.Buffer(), &state)

		var inspectorLines []tui.Line
		if i, ok := c.selectedFileIndex(); ok {
			file := configFiles[i]
			inspectorLines = []tui.Line{
				tui.LineFrom(
					tui.Styled("TARGET", ui.BadgeInfo()),
					tui.Raw(" "),
					tui.Styled(file.path, ui.TitleStyle()),
				),
				tui.LineStr(""),
				tui.LineFrom(
					tui.Styled("Purpose ", ui.LabelStyle()),
					tui.Raw(file.desc),
				),
				tui.LineFrom(
					tui.Styled("Write path ", ui.LabelStyle()),
					tui.Raw("atomic replace"),
				),
				tui.LineStr(""),
				tui.LineSpan(tui.Styled(
					"Open the file, edit in place, then save with Ctrl+S.",
					ui.SubtitleStyle(),
				)),
			}
		} else {
			inspectorLines = []tui.Line{
				tui.LineSpan(tui.Styled("No file selected", ui.MutedStyle())),
				tui.LineStr(""),
				tui.LineStr("Pick a managed config file to inspect before editing."),
			}
		}
		f.RenderWidget(
			tui.ParagraphLines(inspectorLines...).
				Wrap(true).
				Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
			content[1],
		)
	case modeEditing:
		content := tui.Split(chunks[1], tui.Horizontal, tui.Percentage(70), tui.Percentage(30))

		// The text area keeps its viewport across renders, like the original
		// widget's interior mutability.
		c.textarea.Render(content[0], f.Buffer())

		currentFile := "unknown"
		if c.currentFile != nil {
			currentFile = *c.currentFile
		}
		stateBadge := tui.Styled(" CLEAN ", ui.BadgeSuccess())
		if c.isModified {
			stateBadge = tui.Styled(" MODIFIED ", ui.BadgeWarning())
		}
		inspectorLines := []tui.Line{
			tui.LineFrom(
				tui.Styled("FILE", ui.BadgeInfo()),
				tui.Raw(" "),
				tui.Styled(currentFile, ui.TitleStyle()),
			),
			tui.LineStr(""),
			tui.LineFrom(
				tui.Styled("State ", ui.LabelStyle()),
				stateBadge,
			),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled("Shortcuts", ui.EyebrowStyle())),
			tui.LineStr("Ctrl+S  save changes"),
			tui.LineStr("Ctrl+Q  close editor"),
			tui.LineStr("Ctrl+X  discard session"),
			tui.LineStr(""),
			tui.LineSpan(tui.Styled(
				"Edits are written through the app's atomic file-write path.",
				ui.SubtitleStyle(),
			)),
		}
		f.RenderWidget(
			tui.ParagraphLines(inspectorLines...).
				Wrap(true).
				Block(ui.PanelAltBlock(ui.PanelTitle("Inspector"))),
			content[1],
		)
	}

	// Status
	var statusText tui.Paragraph
	switch {
	case c.statusMessage != nil:
		style := ui.SuccessStyle()
		if c.statusMessage.isError {
			style = ui.ErrorStyle()
		}
		statusText = tui.ParagraphStr(c.statusMessage.text).Style(style)
	case c.mode == modeFileSelect:
		statusText = tui.ParagraphStr("Press Enter to edit file").Style(ui.MutedStyle())
	default:
		statusText = tui.ParagraphLine(tui.LineFrom(
			tui.Styled("Ctrl+S ", ui.KeyHint()),
			tui.Styled("save  ", ui.MutedStyle()),
			tui.Styled("Ctrl+Q ", ui.KeyHint()),
			tui.Styled("close  ", ui.MutedStyle()),
			tui.Styled("Ctrl+X ", ui.KeyHint()),
			tui.Styled("discard", ui.MutedStyle()),
		)).Style(ui.MutedStyle())
	}
	f.RenderWidget(statusText.Block(ui.PanelAltBlock(ui.PanelTitle("Status"))), chunks[2])
}

// HelpText implements components.Component.
func (c *Component) HelpText() []msg.KeyHelp {
	if c.mode == modeFileSelect {
		return []msg.KeyHelp{{Key: "↑/↓", Desc: "Navigate"}, {Key: "Enter", Desc: "Edit"}}
	}
	return []msg.KeyHelp{
		{Key: "Ctrl+S", Desc: "Save"},
		{Key: "Ctrl+Q", Desc: "Close"},
		{Key: "Ctrl+X", Desc: "Discard"},
	}
}

// OnActivate implements components.Component.
func (c *Component) OnActivate() {}

// OnDeactivate implements components.Component.
func (c *Component) OnDeactivate() {}
