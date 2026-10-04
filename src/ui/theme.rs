use ratatui::{
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, BorderType, Borders},
};

/// Application theme colors and styles
pub struct Theme;

impl Theme {
    pub const BG: Color = Color::Rgb(12, 16, 24);
    pub const SURFACE: Color = Color::Rgb(20, 27, 38);
    pub const SURFACE_ELEVATED: Color = Color::Rgb(28, 36, 48);
    pub const PANEL_ALT: Color = Color::Rgb(16, 22, 32);
    pub const FG: Color = Color::Rgb(233, 228, 214);
    pub const SOFT_FG: Color = Color::Rgb(190, 186, 176);
    pub const ACCENT: Color = Color::Rgb(94, 204, 187);
    pub const ACCENT_ALT: Color = Color::Rgb(235, 178, 74);
    pub const SUCCESS: Color = Color::Rgb(116, 204, 149);
    pub const ERROR: Color = Color::Rgb(224, 101, 84);
    pub const WARNING: Color = Color::Rgb(240, 198, 96);
    pub const MUTED: Color = Color::Rgb(109, 120, 136);
    pub const BORDER: Color = Color::Rgb(63, 78, 96);
    pub const BORDER_FOCUSED: Color = Color::Rgb(94, 204, 187);

    pub fn app() -> Style {
        Style::default().fg(Self::FG).bg(Self::BG)
    }

    pub fn default() -> Style {
        Self::app()
    }

    pub fn surface() -> Style {
        Style::default().fg(Self::FG).bg(Self::SURFACE)
    }

    pub fn surface_alt() -> Style {
        Style::default().fg(Self::FG).bg(Self::PANEL_ALT)
    }

    pub fn title() -> Style {
        Style::default()
            .fg(Self::FG)
            .bg(Self::BG)
            .add_modifier(Modifier::BOLD)
    }

    pub fn hero() -> Style {
        Style::default()
            .fg(Self::ACCENT_ALT)
            .bg(Self::BG)
            .add_modifier(Modifier::BOLD)
    }

    pub fn subtitle() -> Style {
        Style::default().fg(Self::SOFT_FG).bg(Self::BG)
    }

    pub fn eyebrow() -> Style {
        Style::default()
            .fg(Self::ACCENT_ALT)
            .bg(Self::BG)
            .add_modifier(Modifier::BOLD)
    }

    pub fn label() -> Style {
        Style::default().fg(Self::MUTED).bg(Self::BG)
    }

    pub fn value() -> Style {
        Style::default().fg(Self::FG).bg(Self::BG)
    }

    pub fn accent() -> Style {
        Style::default().fg(Self::ACCENT).bg(Self::BG)
    }

    pub fn highlight() -> Style {
        Style::default()
            .fg(Self::FG)
            .bg(Self::SURFACE_ELEVATED)
            .add_modifier(Modifier::BOLD)
    }

    pub fn tab_active() -> Style {
        Style::default()
            .fg(Self::BG)
            .bg(Self::ACCENT)
            .add_modifier(Modifier::BOLD)
    }

    pub fn tab_inactive() -> Style {
        Style::default().fg(Self::SOFT_FG).bg(Self::SURFACE)
    }

    pub fn success() -> Style {
        Style::default().fg(Self::SUCCESS).bg(Self::BG)
    }

    pub fn error() -> Style {
        Style::default().fg(Self::ERROR).bg(Self::BG)
    }

    pub fn warning() -> Style {
        Style::default().fg(Self::WARNING).bg(Self::BG)
    }

    pub fn muted() -> Style {
        Style::default().fg(Self::MUTED).bg(Self::BG)
    }

    pub fn status_bar() -> Style {
        Style::default().fg(Self::FG).bg(Self::SURFACE_ELEVATED)
    }

    pub fn key_hint() -> Style {
        Style::default()
            .fg(Self::BG)
            .bg(Self::ACCENT_ALT)
            .add_modifier(Modifier::BOLD)
    }

    pub fn key_hint_secondary() -> Style {
        Style::default()
            .fg(Self::FG)
            .bg(Self::SURFACE)
            .add_modifier(Modifier::BOLD)
    }

    pub fn progress_complete() -> Style {
        Style::default().fg(Self::SUCCESS).bg(Self::SURFACE)
    }

    pub fn progress_pending() -> Style {
        Style::default().fg(Self::MUTED).bg(Self::SURFACE)
    }

    pub fn progress_running() -> Style {
        Style::default()
            .fg(Self::ACCENT_ALT)
            .bg(Self::SURFACE)
            .add_modifier(Modifier::BOLD)
    }

    pub fn input_active() -> Style {
        Style::default().fg(Self::FG).bg(Self::SURFACE_ELEVATED)
    }

    pub fn input_inactive() -> Style {
        Style::default().fg(Self::SOFT_FG).bg(Self::SURFACE)
    }

    pub fn border() -> Style {
        Style::default().fg(Self::BORDER).bg(Self::SURFACE)
    }

    pub fn border_focused() -> Style {
        Style::default().fg(Self::BORDER_FOCUSED).bg(Self::SURFACE)
    }

    pub fn list_selected() -> Style {
        Style::default()
            .fg(Self::BG)
            .bg(Self::ACCENT_ALT)
            .add_modifier(Modifier::BOLD)
    }

    pub fn badge_warning() -> Style {
        Style::default()
            .fg(Self::BG)
            .bg(Self::WARNING)
            .add_modifier(Modifier::BOLD)
    }

    pub fn badge_success() -> Style {
        Style::default()
            .fg(Self::BG)
            .bg(Self::SUCCESS)
            .add_modifier(Modifier::BOLD)
    }

    pub fn badge_info() -> Style {
        Style::default()
            .fg(Self::BG)
            .bg(Self::ACCENT)
            .add_modifier(Modifier::BOLD)
    }

    pub fn badge_neutral() -> Style {
        Style::default()
            .fg(Self::FG)
            .bg(Self::SURFACE_ELEVATED)
            .add_modifier(Modifier::BOLD)
    }

    pub fn panel<'a>(title: Line<'a>) -> Block<'a> {
        Block::default()
            .borders(Borders::ALL)
            .border_type(BorderType::Rounded)
            .border_style(Self::border())
            .style(Self::surface())
            .title(title)
    }

    pub fn panel_focused<'a>(title: Line<'a>) -> Block<'a> {
        Block::default()
            .borders(Borders::ALL)
            .border_type(BorderType::Rounded)
            .border_style(Self::border_focused())
            .style(Self::surface())
            .title(title)
    }

    pub fn panel_alt<'a>(title: Line<'a>) -> Block<'a> {
        Block::default()
            .borders(Borders::ALL)
            .border_type(BorderType::Rounded)
            .border_style(Self::border())
            .style(Self::surface_alt())
            .title(title)
    }

    pub fn panel_title<T: Into<String>>(label: T) -> Line<'static> {
        Line::from(vec![
            Span::styled(" ", Style::default().bg(Self::SURFACE)),
            Span::styled("● ", Style::default().fg(Self::ACCENT).bg(Self::SURFACE)),
            Span::styled(
                format!("{} ", label.into().to_uppercase()),
                Style::default()
                    .fg(Self::ACCENT_ALT)
                    .bg(Self::SURFACE)
                    .add_modifier(Modifier::BOLD),
            ),
        ])
    }
}
