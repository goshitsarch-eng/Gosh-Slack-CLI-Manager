use ratatui::style::{Color, Modifier, Style};

/// Application theme colors and styles
pub struct Theme;

impl Theme {
    // Colors
    pub const BG: Color = Color::Reset;
    pub const FG: Color = Color::White;
    pub const ACCENT: Color = Color::Cyan;
    pub const SUCCESS: Color = Color::Green;
    pub const ERROR: Color = Color::Red;
    pub const WARNING: Color = Color::Yellow;
    pub const MUTED: Color = Color::DarkGray;
    pub const HIGHLIGHT_BG: Color = Color::DarkGray;

    // Styles
    pub fn default() -> Style {
        Style::default().fg(Self::FG).bg(Self::BG)
    }

    pub fn title() -> Style {
        Style::default()
            .fg(Self::ACCENT)
            .add_modifier(Modifier::BOLD)
    }

    pub fn highlight() -> Style {
        Style::default()
            .bg(Self::HIGHLIGHT_BG)
            .add_modifier(Modifier::BOLD)
    }

    pub fn selected() -> Style {
        Style::default()
            .fg(Color::Black)
            .bg(Self::ACCENT)
            .add_modifier(Modifier::BOLD)
    }

    pub fn tab_active() -> Style {
        Style::default()
            .fg(Self::ACCENT)
            .add_modifier(Modifier::BOLD | Modifier::UNDERLINED)
    }

    pub fn tab_inactive() -> Style {
        Style::default().fg(Self::MUTED)
    }

    pub fn success() -> Style {
        Style::default().fg(Self::SUCCESS)
    }

    pub fn error() -> Style {
        Style::default().fg(Self::ERROR)
    }

    pub fn warning() -> Style {
        Style::default().fg(Self::WARNING)
    }

    pub fn muted() -> Style {
        Style::default().fg(Self::MUTED)
    }

    pub fn status_bar() -> Style {
        Style::default().bg(Color::DarkGray).fg(Color::White)
    }

    pub fn key_hint() -> Style {
        Style::default()
            .fg(Self::ACCENT)
            .add_modifier(Modifier::BOLD)
    }

    pub fn progress_complete() -> Style {
        Style::default().fg(Self::SUCCESS)
    }

    pub fn progress_pending() -> Style {
        Style::default().fg(Self::MUTED)
    }

    pub fn progress_running() -> Style {
        Style::default()
            .fg(Self::WARNING)
            .add_modifier(Modifier::BOLD)
    }

    pub fn input_active() -> Style {
        Style::default()
            .fg(Self::FG)
            .bg(Color::DarkGray)
    }

    pub fn input_inactive() -> Style {
        Style::default().fg(Self::MUTED)
    }

    pub fn border() -> Style {
        Style::default().fg(Self::MUTED)
    }

    pub fn border_focused() -> Style {
        Style::default().fg(Self::ACCENT)
    }

    pub fn list_selected() -> Style {
        Style::default()
            .fg(Color::Black)
            .bg(Self::ACCENT)
            .add_modifier(Modifier::BOLD)
    }
}
