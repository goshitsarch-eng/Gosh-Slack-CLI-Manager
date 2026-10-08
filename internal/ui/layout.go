package ui

import "github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"

// CenteredRect returns a rect of the given percentage size centered in area.
func CenteredRect(percentX, percentY int, area tui.Rect) tui.Rect {
	popup := tui.Split(area, tui.Vertical,
		tui.Percentage((100-percentY)/2),
		tui.Percentage(percentY),
		tui.Percentage((100-percentY)/2),
	)
	return tui.Split(popup[1], tui.Horizontal,
		tui.Percentage((100-percentX)/2),
		tui.Percentage(percentX),
		tui.Percentage((100-percentX)/2),
	)[1]
}

// AppLayout is the main screen layout: header, navigation, content and the
// wrapped controls bar.
type AppLayout struct {
	Header, Tabs, Content, StatusBar tui.Rect
}

// NewAppLayout splits area into the main layout.
func NewAppLayout(area tui.Rect, statusHeight int) AppLayout {
	chunks := tui.Split(area, tui.Vertical,
		tui.Length(5), // Header: three lines and borders
		tui.Length(5), // Navigation: three shortcut lanes and borders
		tui.Min(10),   // Main content
		tui.Length(statusHeight),
	)
	return AppLayout{Header: chunks[0], Tabs: chunks[1], Content: chunks[2], StatusBar: chunks[3]}
}
