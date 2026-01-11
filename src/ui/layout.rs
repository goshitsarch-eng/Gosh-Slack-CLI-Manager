use ratatui::layout::{Constraint, Direction, Layout, Rect};

/// Create a centered rectangle with given percentage width and height
pub fn centered_rect(percent_x: u16, percent_y: u16, area: Rect) -> Rect {
    let popup_layout = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Percentage((100 - percent_y) / 2),
            Constraint::Percentage(percent_y),
            Constraint::Percentage((100 - percent_y) / 2),
        ])
        .split(area);

    Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Percentage((100 - percent_x) / 2),
            Constraint::Percentage(percent_x),
            Constraint::Percentage((100 - percent_x) / 2),
        ])
        .split(popup_layout[1])[1]
}

/// Main application layout with header, content, and status bar
pub struct AppLayout {
    pub header: Rect,
    pub tabs: Rect,
    pub content: Rect,
    pub status_bar: Rect,
}

impl AppLayout {
    pub fn new(area: Rect) -> Self {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(2), // Header with title
                Constraint::Length(2), // Tabs (two rows for F1-F6 and F7-F12 + Ctrl shortcuts)
                Constraint::Min(10),   // Main content
                Constraint::Length(1), // Status bar
            ])
            .split(area);

        Self {
            header: chunks[0],
            tabs: chunks[1],
            content: chunks[2],
            status_bar: chunks[3],
        }
    }
}

/// Two-column layout for forms
pub struct FormLayout {
    pub left: Rect,
    pub right: Rect,
}

impl FormLayout {
    pub fn new(area: Rect) -> Self {
        let chunks = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
            .split(area);

        Self {
            left: chunks[0],
            right: chunks[1],
        }
    }
}

/// Layout with a main content area and output panel
pub struct ContentWithOutput {
    pub main: Rect,
    pub output: Rect,
}

impl ContentWithOutput {
    pub fn new(area: Rect, output_height: u16) -> Self {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Min(5), Constraint::Length(output_height)])
            .split(area);

        Self {
            main: chunks[0],
            output: chunks[1],
        }
    }
}
