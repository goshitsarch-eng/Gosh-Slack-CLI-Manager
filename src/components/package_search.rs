use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::Modifier,
    text::{Line, Span},
    widgets::{List, ListItem, ListState, Paragraph},
    Frame,
};

use super::Component;
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

    fn render_selected_package(&self, frame: &mut Frame, area: Rect) {
        let lines = if let Some(pkg) = self.get_selected_package() {
            vec![
                Line::from(vec![
                    Span::styled("PACKAGE", Theme::badge_info()),
                    Span::raw(" "),
                    Span::styled(&pkg.name, Theme::title()),
                ]),
                Line::from(vec![
                    Span::styled("Category ", Theme::label()),
                    Span::styled(&pkg.category, Theme::badge_neutral()),
                ]),
                Line::from(""),
                Line::from(Span::styled("Description", Theme::eyebrow())),
                Line::from(pkg.description.clone()),
                Line::from(""),
                Line::from(Span::styled("Actions", Theme::eyebrow())),
                Line::from("Enter   search current query"),
                Line::from("Ctrl+I  install selected package"),
                Line::from("Tab     move between search hits"),
            ]
        } else {
            vec![
                Line::from(Span::styled("No package selected", Theme::muted())),
                Line::from(""),
                Line::from("Search for a package to inspect details here."),
            ]
        };

        let panel = Paragraph::new(lines).block(Theme::panel_alt(Theme::panel_title("Inspector")));
        frame.render_widget(panel, area);
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
                Constraint::Length(5),
                Constraint::Min(10),
                Constraint::Length(3), // Status
            ])
            .split(area);

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
            .split(chunks[0]);

        let title = Paragraph::new(vec![
            Line::from(Span::styled("SlackBuilds Package Search", Theme::title())),
            Line::from(Span::styled(
                "Query, inspect, and install packages without leaving the terminal.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel(Theme::panel_title("Search deck")));
        frame.render_widget(title, header[0]);

        let query_panel = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Query ", Theme::label()),
                Span::styled(
                    if self.search_query.is_empty() {
                        "<empty>"
                    } else {
                        self.search_query.as_str()
                    },
                    Theme::input_active(),
                ),
            ]),
            Line::from(vec![
                Span::styled("Results ", Theme::label()),
                Span::styled(self.results.len().to_string(), Theme::badge_neutral()),
                Span::raw(" "),
                if self.is_searching {
                    Span::styled(" SEARCHING ", Theme::badge_warning())
                } else if self.is_installing {
                    Span::styled(" INSTALLING ", Theme::badge_warning())
                } else {
                    Span::styled(" READY ", Theme::badge_success())
                },
            ]),
        ])
        .style(Theme::input_active())
        .block(Theme::panel_focused(Theme::panel_title("Query")));
        frame.render_widget(query_panel, header[1]);

        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(62), Constraint::Percentage(38)])
            .split(chunks[1]);

        let items: Vec<ListItem> = self
            .results
            .iter()
            .map(|pkg| {
                ListItem::new(Line::from(vec![
                    Span::styled(&pkg.name, Theme::title()),
                    Span::raw(" "),
                    Span::styled(format!(" {} ", pkg.category), Theme::badge_neutral()),
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
            .block(Theme::panel(Theme::panel_title(results_title)))
            .highlight_style(Theme::highlight().add_modifier(Modifier::BOLD))
            .highlight_symbol("▸ ");

        frame.render_stateful_widget(list, content[0], &mut self.list_state.clone());
        self.render_selected_package(frame, content[1]);

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
            status.block(Theme::panel_alt(Theme::panel_title("Status"))),
            chunks[2],
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
