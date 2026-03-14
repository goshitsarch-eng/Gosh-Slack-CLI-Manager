use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::Modifier,
    text::{Line, Span},
    widgets::{List, ListItem, ListState, Paragraph},
    Frame,
};

use super::Component;
use crate::app::Message;
use crate::slackware::config::MirrorEntry;
use crate::slackware::SlackwareVersion;
use crate::ui::theme::Theme;

/// Mirror management component
pub struct MirrorComponent {
    mirrors: Vec<MirrorEntry>,
    list_state: ListState,
    version: SlackwareVersion,
    is_running: bool,
    status_message: Option<(String, bool)>, // (message, is_error)
}

impl MirrorComponent {
    pub fn new(version: SlackwareVersion) -> Self {
        Self {
            mirrors: Vec::new(),
            list_state: ListState::default(),
            version,
            is_running: false,
            status_message: None,
        }
    }

    pub fn load_mirrors(&mut self) {
        use crate::slackware::config::SlackwareConfig;

        let version_filter = self.version.mirror_path();
        match SlackwareConfig::parse_mirrors(Some(version_filter)) {
            Ok(mirrors) => {
                self.mirrors = mirrors;
                if !self.mirrors.is_empty() {
                    self.list_state.select(Some(0));
                }
                self.status_message = None;
            }
            Err(e) => {
                self.status_message = Some((format!("Failed to load mirrors: {}", e), true));
            }
        }
    }

    pub fn get_selected_mirror(&self) -> Option<&MirrorEntry> {
        self.list_state.selected().and_then(|i| self.mirrors.get(i))
    }

    pub fn set_status(&mut self, message: String, is_error: bool) {
        self.status_message = Some((message, is_error));
        self.is_running = false;
    }

    pub fn start_update(&mut self) {
        self.is_running = true;
        self.status_message = None;
    }
}

impl Component for MirrorComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.is_running {
            return None;
        }

        match key.code {
            KeyCode::Up | KeyCode::Char('k') => {
                if let Some(selected) = self.list_state.selected() {
                    if selected > 0 {
                        self.list_state.select(Some(selected - 1));
                    }
                }
                None
            }
            KeyCode::Down | KeyCode::Char('j') => {
                if let Some(selected) = self.list_state.selected() {
                    if selected < self.mirrors.len().saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                } else if !self.mirrors.is_empty() {
                    self.list_state.select(Some(0));
                }
                None
            }
            KeyCode::Enter => {
                if let Some(mirror) = self.get_selected_mirror() {
                    let url = mirror.url.clone();
                    self.start_update();
                    return Some(Message::SetMirror(url));
                }
                None
            }
            KeyCode::Char('r') | KeyCode::Char('R') => {
                self.load_mirrors();
                None
            }
            _ => None,
        }
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(4), // Header
                Constraint::Min(10),   // Mirror list
                Constraint::Length(3), // Status
            ])
            .split(area);

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
            .split(chunks[0]);

        let title = Paragraph::new(vec![
            Line::from(Span::styled("Mirror Configuration", Theme::title())),
            Line::from(Span::styled(
                "Choose the active Slackware mirror for your detected release track.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel(Theme::panel_title("Mirrors")));
        frame.render_widget(title, header[0]);

        let active_count = self.mirrors.iter().filter(|m| m.is_active).count();
        let version_info = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Track ", Theme::label()),
                Span::styled(self.version.display_name(), Theme::badge_neutral()),
            ]),
            Line::from(vec![
                Span::styled("Active mirrors ", Theme::label()),
                Span::styled(active_count.to_string(), Theme::badge_success()),
                Span::raw(" "),
                Span::styled("Candidates ", Theme::label()),
                Span::styled(self.mirrors.len().to_string(), Theme::badge_neutral()),
            ]),
        ])
        .block(Theme::panel_alt(Theme::panel_title("Release track")));
        frame.render_widget(version_info, header[1]);

        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(64), Constraint::Percentage(36)])
            .split(chunks[1]);

        let items: Vec<ListItem> = self
            .mirrors
            .iter()
            .map(|m| {
                ListItem::new(vec![
                    Line::from(vec![
                        if m.is_active {
                            Span::styled(" ACTIVE ", Theme::badge_success())
                        } else {
                            Span::styled(" STANDBY ", Theme::badge_neutral())
                        },
                        Span::raw(" "),
                        Span::styled(
                            &m.url,
                            if m.is_active {
                                Theme::success()
                            } else {
                                Theme::value()
                            },
                        ),
                    ]),
                    Line::from(vec![
                        Span::styled("  Region ", Theme::muted()),
                        Span::raw(&m.region),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(Theme::panel(Theme::panel_title(format!(
                "Mirrors ({})",
                self.mirrors.len()
            ))))
            .highlight_style(Theme::highlight().add_modifier(Modifier::BOLD))
            .highlight_symbol("▸ ");

        frame.render_stateful_widget(list, content[0], &mut self.list_state.clone());

        let inspector_lines = if let Some(mirror) = self.get_selected_mirror() {
            vec![
                Line::from(vec![
                    Span::styled("SELECTION", Theme::badge_info()),
                    Span::raw(" "),
                    Span::styled(&mirror.region, Theme::title()),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled("State ", Theme::label()),
                    if mirror.is_active {
                        Span::styled(" ACTIVE ", Theme::badge_success())
                    } else {
                        Span::styled(" STANDBY ", Theme::badge_neutral())
                    },
                ]),
                Line::from(vec![
                    Span::styled("Track ", Theme::label()),
                    Span::raw(self.version.display_name()),
                ]),
                Line::from(Span::styled("URL", Theme::eyebrow())),
                Line::from(mirror.url.clone()),
                Line::from(""),
                Line::from(Span::styled(
                    "Selecting a mirror validates the exact target and then refreshes slackpkg metadata.",
                    Theme::subtitle(),
                )),
            ]
        } else {
            vec![
                Line::from(Span::styled("No mirror selected", Theme::muted())),
                Line::from(""),
                Line::from("Choose a mirror to inspect its region and activation state."),
            ]
        };
        frame.render_widget(
            Paragraph::new(inspector_lines)
                .block(Theme::panel_alt(Theme::panel_title("Inspector"))),
            content[1],
        );

        // Status
        let status = if let Some((ref msg, is_error)) = self.status_message {
            Paragraph::new(msg.as_str()).style(if is_error {
                Theme::error()
            } else {
                Theme::success()
            })
        } else if self.is_running {
            Paragraph::new("Updating mirror configuration...").style(Theme::warning())
        } else {
            Paragraph::new("Press Enter to select mirror, R to refresh list").style(Theme::muted())
        };
        frame.render_widget(
            status.block(Theme::panel_alt(Theme::panel_title("Status"))),
            chunks[2],
        );
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![("↑/↓", "Navigate"), ("Enter", "Select"), ("R", "Refresh")]
    }

    fn on_activate(&mut self) {
        self.load_mirrors();
    }
}
