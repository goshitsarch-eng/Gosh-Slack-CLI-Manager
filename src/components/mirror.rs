use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::Modifier,
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use tokio::sync::mpsc;

use super::{AsyncComponent, Component};
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
    progress_tx: Option<mpsc::UnboundedSender<String>>,
}

impl MirrorComponent {
    pub fn new(version: SlackwareVersion) -> Self {
        Self {
            mirrors: Vec::new(),
            list_state: ListState::default(),
            version,
            is_running: false,
            status_message: None,
            progress_tx: None,
        }
    }

    pub fn set_version(&mut self, version: SlackwareVersion) {
        self.version = version;
        self.load_mirrors();
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
                Constraint::Length(3), // Title
                Constraint::Length(3), // Version info
                Constraint::Min(10),   // Mirror list
                Constraint::Length(3), // Status
            ])
            .split(area);

        // Title
        let title = Paragraph::new(Line::from(vec![Span::styled(
            "Mirror Configuration",
            Theme::title(),
        )]))
        .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(title, chunks[0]);

        // Version info
        let version_info = Paragraph::new(Line::from(vec![
            Span::styled("Detected version: ", Theme::muted()),
            Span::styled(self.version.display_name(), Theme::default()),
        ]))
        .block(Block::default().borders(Borders::NONE));
        frame.render_widget(version_info, chunks[1]);

        // Mirror list
        let items: Vec<ListItem> = self
            .mirrors
            .iter()
            .map(|m| {
                let status = if m.is_active { "●" } else { "○" };
                let style = if m.is_active {
                    Theme::success()
                } else {
                    Theme::default()
                };

                ListItem::new(Line::from(vec![
                    Span::styled(format!("{} ", status), style),
                    Span::styled(&m.url, style),
                    Span::styled(format!(" ({})", m.region), Theme::muted()),
                ]))
            })
            .collect();

        let list = List::new(items)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(format!("Mirrors ({})", self.mirrors.len())),
            )
            .highlight_style(Theme::highlight().add_modifier(Modifier::BOLD))
            .highlight_symbol("→ ");

        frame.render_stateful_widget(list, chunks[2], &mut self.list_state.clone());

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
            status.block(Block::default().borders(Borders::TOP)),
            chunks[3],
        );
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("↑/↓", "Navigate"),
            ("Enter", "Select"),
            ("R", "Refresh"),
        ]
    }

    fn on_activate(&mut self) {
        self.load_mirrors();
    }
}

impl AsyncComponent for MirrorComponent {
    fn set_progress_channel(&mut self, tx: mpsc::UnboundedSender<String>) {
        self.progress_tx = Some(tx);
    }

    fn is_running(&self) -> bool {
        self.is_running
    }
}
