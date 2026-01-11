use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
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
use crate::slackware::packages::PackageInfo;
use crate::ui::theme::Theme;

/// Package search component
pub struct PackageSearchComponent {
    search_query: String,
    results: Vec<PackageInfo>,
    list_state: ListState,
    is_searching: bool,
    is_installing: bool,
    status_message: Option<(String, bool)>,
    progress_tx: Option<mpsc::UnboundedSender<String>>,
}

impl PackageSearchComponent {
    pub fn new() -> Self {
        Self {
            search_query: String::new(),
            results: Vec::new(),
            list_state: ListState::default(),
            is_searching: false,
            is_installing: false,
            status_message: None,
            progress_tx: None,
        }
    }

    pub fn set_results(&mut self, results: Vec<PackageInfo>) {
        self.results = results;
        self.is_searching = false;
        if !self.results.is_empty() {
            self.list_state.select(Some(0));
        } else {
            self.list_state.select(None);
        }
    }

    pub fn get_selected_package(&self) -> Option<&PackageInfo> {
        self.list_state.selected().and_then(|i| self.results.get(i))
    }

    pub fn get_query(&self) -> &str {
        &self.search_query
    }

    pub fn start_search(&mut self) {
        self.is_searching = true;
        self.status_message = None;
    }

    pub fn start_install(&mut self) {
        self.is_installing = true;
        self.status_message = None;
    }

    pub fn set_status(&mut self, message: String, is_error: bool) {
        self.status_message = Some((message, is_error));
        self.is_searching = false;
        self.is_installing = false;
    }
}

impl Default for PackageSearchComponent {
    fn default() -> Self {
        Self::new()
    }
}

impl Component for PackageSearchComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.is_searching || self.is_installing {
            return None;
        }

        match key.code {
            KeyCode::Char(c) if !key.modifiers.contains(KeyModifiers::CONTROL) => {
                self.search_query.push(c);
                None
            }
            KeyCode::Backspace => {
                self.search_query.pop();
                None
            }
            KeyCode::Enter if !self.search_query.is_empty() => {
                self.start_search();
                Some(Message::SearchPackages(self.search_query.clone()))
            }
            KeyCode::Up | KeyCode::Char('k') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                if let Some(selected) = self.list_state.selected() {
                    if selected > 0 {
                        self.list_state.select(Some(selected - 1));
                    }
                }
                None
            }
            KeyCode::Down | KeyCode::Char('j') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                if let Some(selected) = self.list_state.selected() {
                    if selected < self.results.len().saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                } else if !self.results.is_empty() {
                    self.list_state.select(Some(0));
                }
                None
            }
            KeyCode::Tab => {
                // Cycle through results
                if !self.results.is_empty() {
                    let next = self
                        .list_state
                        .selected()
                        .map(|i| (i + 1) % self.results.len())
                        .unwrap_or(0);
                    self.list_state.select(Some(next));
                }
                None
            }
            KeyCode::Char('i') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                if let Some(pkg) = self.get_selected_package() {
                    let name = pkg.name.clone();
                    self.start_install();
                    return Some(Message::InstallPackage(name));
                }
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
                Constraint::Length(3), // Search input
                Constraint::Min(10),   // Results
                Constraint::Length(3), // Status
            ])
            .split(area);

        // Title
        let title = Paragraph::new(Line::from(vec![Span::styled(
            "Package Search (SlackBuilds.org)",
            Theme::title(),
        )]))
        .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(title, chunks[0]);

        // Search input
        let search_block = Block::default()
            .borders(Borders::ALL)
            .title("Search")
            .border_style(Theme::border_focused());
        let search = Paragraph::new(self.search_query.as_str())
            .style(Theme::input_active())
            .block(search_block);
        frame.render_widget(search, chunks[1]);

        // Results list
        let items: Vec<ListItem> = self
            .results
            .iter()
            .map(|pkg| {
                ListItem::new(Line::from(vec![
                    Span::styled(&pkg.name, Theme::default().add_modifier(Modifier::BOLD)),
                    Span::styled(format!(" ({})", pkg.category), Theme::muted()),
                    Span::raw(" - "),
                    Span::styled(
                        if pkg.description.len() > 50 {
                            format!("{}...", &pkg.description[..50])
                        } else {
                            pkg.description.clone()
                        },
                        Theme::muted(),
                    ),
                ]))
            })
            .collect();

        let results_title = if self.is_searching {
            "Searching...".to_string()
        } else {
            format!("Results ({})", self.results.len())
        };

        let list = List::new(items)
            .block(Block::default().borders(Borders::ALL).title(results_title))
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
        } else if self.is_installing {
            Paragraph::new("Installing package...").style(Theme::warning())
        } else if self.is_searching {
            Paragraph::new("Searching...").style(Theme::warning())
        } else {
            Paragraph::new("Type to search, Enter to submit, Ctrl+I to install selected")
                .style(Theme::muted())
        };
        frame.render_widget(
            status.block(Block::default().borders(Borders::TOP)),
            chunks[3],
        );
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("Enter", "Search"),
            ("Tab", "Next result"),
            ("Ctrl+I", "Install"),
        ]
    }
}

impl AsyncComponent for PackageSearchComponent {
    fn set_progress_channel(&mut self, tx: mpsc::UnboundedSender<String>) {
        self.progress_tx = Some(tx);
    }

    fn is_running(&self) -> bool {
        self.is_searching || self.is_installing
    }
}
