use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    text::{Line, Span},
    widgets::{List, ListItem, ListState, Paragraph, Wrap},
    Frame,
};
use std::fs;
use std::path::Path;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// Installed package information
#[derive(Debug, Clone)]
pub struct InstalledPackage {
    pub name: String,
    pub version: String,
    pub arch: String,
    pub build: String,
    pub full_name: String,
    pub description: String,
    pub size_compressed: String,
    pub size_uncompressed: String,
}

/// Package Browser/Manager Component
pub struct PackageBrowserComponent {
    packages: Vec<InstalledPackage>,
    filtered_packages: Vec<usize>,
    list_state: ListState,
    search_query: String,
    is_searching: bool,
    selected_package: Option<InstalledPackage>,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
    view_mode: ViewMode,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum ViewMode {
    List,
    Details,
}

impl PackageBrowserComponent {
    pub fn new() -> Self {
        let mut component = Self {
            packages: Vec::new(),
            filtered_packages: Vec::new(),
            list_state: ListState::default(),
            search_query: String::new(),
            is_searching: false,
            selected_package: None,
            status_message: None,
            show_confirm: false,
            view_mode: ViewMode::List,
        };
        component.load_packages();
        component.apply_filter();
        if !component.filtered_packages.is_empty() {
            component.list_state.select(Some(0));
        }
        component
    }

    pub fn load_packages(&mut self) {
        let packages_dir = Path::new("/var/log/packages");
        let mut packages = Vec::new();

        if let Ok(entries) = fs::read_dir(packages_dir) {
            for entry in entries.filter_map(|e| e.ok()) {
                let filename = entry.file_name().to_string_lossy().to_string();

                if let Some(pkg) = Self::parse_package_name(&filename) {
                    let mut pkg = pkg;

                    // Read package info file for description
                    if let Ok(content) = fs::read_to_string(entry.path()) {
                        pkg.description = Self::extract_description(&content);
                        pkg.size_compressed =
                            Self::extract_size(&content, "COMPRESSED PACKAGE SIZE:");
                        pkg.size_uncompressed =
                            Self::extract_size(&content, "UNCOMPRESSED PACKAGE SIZE:");
                    }

                    packages.push(pkg);
                }
            }
        }

        packages.sort_by(|a, b| a.name.to_lowercase().cmp(&b.name.to_lowercase()));
        self.packages = packages;
    }

    fn parse_package_name(filename: &str) -> Option<InstalledPackage> {
        // Slackware package naming: name-version-arch-build
        // Examples: bash-5.1.008-x86_64-1, kernel-generic-5.15.19-x86_64-1
        let parts: Vec<&str> = filename.rsplitn(4, '-').collect();

        if parts.len() >= 4 {
            Some(InstalledPackage {
                build: parts[0].to_string(),
                arch: parts[1].to_string(),
                version: parts[2].to_string(),
                name: parts[3].to_string(),
                full_name: filename.to_string(),
                description: String::new(),
                size_compressed: String::new(),
                size_uncompressed: String::new(),
            })
        } else {
            None
        }
    }

    fn extract_description(content: &str) -> String {
        let mut in_description = false;
        let mut description = String::new();

        for line in content.lines() {
            if line.starts_with("PACKAGE DESCRIPTION:") {
                in_description = true;
                continue;
            }
            if in_description {
                if line.starts_with("FILE LIST:") {
                    break;
                }
                // Skip the package name line (usually first line after PACKAGE DESCRIPTION)
                let trimmed = line.trim();
                if !trimmed.is_empty() && !trimmed.ends_with(':') {
                    // Remove package name prefix if present
                    let desc_line = if let Some(pos) = trimmed.find(':') {
                        trimmed[pos + 1..].trim()
                    } else {
                        trimmed
                    };
                    if !desc_line.is_empty() {
                        if !description.is_empty() {
                            description.push(' ');
                        }
                        description.push_str(desc_line);
                    }
                }
            }
        }

        if description.len() > 200 {
            description.truncate(200);
            description.push_str("...");
        }

        description
    }

    fn extract_size(content: &str, prefix: &str) -> String {
        for line in content.lines() {
            if line.starts_with(prefix) {
                return line.trim_start_matches(prefix).trim().to_string();
            }
        }
        "Unknown".to_string()
    }

    fn apply_filter(&mut self) {
        self.filtered_packages = self
            .packages
            .iter()
            .enumerate()
            .filter(|(_, pkg)| {
                if self.search_query.is_empty() {
                    true
                } else {
                    let query = self.search_query.to_lowercase();
                    pkg.name.to_lowercase().contains(&query)
                        || pkg.description.to_lowercase().contains(&query)
                }
            })
            .map(|(i, _)| i)
            .collect();

        if self.filtered_packages.is_empty() {
            self.list_state.select(None);
        } else {
            self.list_state.select(Some(0));
        }
    }

    fn selected_package(&self) -> Option<&InstalledPackage> {
        self.list_state
            .selected()
            .and_then(|i| self.filtered_packages.get(i))
            .and_then(|&idx| self.packages.get(idx))
    }

    pub fn set_status(&mut self, message: String, is_error: bool) {
        self.status_message = Some((message, is_error));
    }

    pub fn refresh_packages(&mut self) {
        self.load_packages();
        self.apply_filter();
    }

    fn render_selected_package(&self, frame: &mut Frame, area: Rect) {
        let lines = if let Some(pkg) = self.selected_package() {
            vec![
                Line::from(Span::styled(&pkg.name, Theme::title())),
                Line::from(vec![
                    Span::styled("Version ", Theme::muted()),
                    Span::raw(&pkg.version),
                ]),
                Line::from(vec![
                    Span::styled("Arch ", Theme::muted()),
                    Span::raw(&pkg.arch),
                ]),
                Line::from(vec![
                    Span::styled("Sizes ", Theme::muted()),
                    Span::raw(format!(
                        "{} / {}",
                        pkg.size_compressed, pkg.size_uncompressed
                    )),
                ]),
                Line::from(""),
                Line::from(Span::styled("Description", Theme::muted())),
                Line::from(pkg.description.clone()),
            ]
        } else {
            vec![
                Line::from(Span::styled("No package selected", Theme::muted())),
                Line::from(""),
                Line::from("Select an installed package to inspect metadata."),
            ]
        };

        frame.render_widget(
            Paragraph::new(lines).block(Theme::panel_alt(Theme::panel_title("Inspector"))),
            area,
        );
    }
}

impl Component for PackageBrowserComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    if let Some(pkg) = self.selected_package.take() {
                        return Some(Message::RemoveInstalledPackage(pkg.full_name));
                    }
                }
                KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                    self.show_confirm = false;
                    self.selected_package = None;
                }
                _ => {}
            }
            return None;
        }

        if self.is_searching {
            match key.code {
                KeyCode::Enter | KeyCode::Esc => {
                    self.is_searching = false;
                }
                KeyCode::Backspace => {
                    self.search_query.pop();
                    self.apply_filter();
                }
                KeyCode::Char(c) => {
                    self.search_query.push(c);
                    self.apply_filter();
                }
                _ => {}
            }
            return None;
        }

        match key.code {
            KeyCode::Up | KeyCode::Char('k') => {
                if let Some(selected) = self.list_state.selected() {
                    if selected > 0 {
                        self.list_state.select(Some(selected - 1));
                    }
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                if let Some(selected) = self.list_state.selected() {
                    if selected < self.filtered_packages.len().saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                } else if !self.filtered_packages.is_empty() {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::Home => {
                if !self.filtered_packages.is_empty() {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::End => {
                if !self.filtered_packages.is_empty() {
                    self.list_state
                        .select(Some(self.filtered_packages.len() - 1));
                }
            }
            KeyCode::PageUp => {
                if let Some(selected) = self.list_state.selected() {
                    self.list_state.select(Some(selected.saturating_sub(10)));
                }
            }
            KeyCode::PageDown => {
                if let Some(selected) = self.list_state.selected() {
                    let new_idx =
                        (selected + 10).min(self.filtered_packages.len().saturating_sub(1));
                    self.list_state.select(Some(new_idx));
                }
            }
            KeyCode::Char('/') => {
                self.is_searching = true;
            }
            KeyCode::Enter => {
                self.view_mode = match self.view_mode {
                    ViewMode::List => ViewMode::Details,
                    ViewMode::Details => ViewMode::List,
                };
            }
            KeyCode::Char('d') => {
                if let Some(pkg) = self.selected_package() {
                    self.selected_package = Some(pkg.clone());
                    self.show_confirm = true;
                }
            }
            KeyCode::Char('c') => {
                self.search_query.clear();
                self.apply_filter();
            }
            KeyCode::F(5) => {
                self.load_packages();
                self.apply_filter();
                self.status_message = Some(("Package list refreshed".to_string(), false));
            }
            _ => {}
        }
        None
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(4),
                Constraint::Min(10),
                Constraint::Length(3),
            ])
            .split(area);

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
            .split(chunks[0]);

        let search_style = if self.is_searching {
            Theme::accent()
        } else {
            Theme::value()
        };
        let search_bar = Paragraph::new(Line::from(vec![
            Span::styled("Query ", Theme::label()),
            Span::styled(&self.search_query, search_style),
            if self.is_searching {
                Span::styled(" _", Theme::accent())
            } else {
                Span::raw("")
            },
            Span::styled(
                format!(
                    "  {}/{} packages",
                    self.filtered_packages.len(),
                    self.packages.len()
                ),
                Theme::badge_neutral(),
            ),
        ]))
        .block(Theme::panel(Theme::panel_title("Installed packages")));
        frame.render_widget(search_bar, header[0]);

        let runtime = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Mode ", Theme::muted()),
                Span::styled(
                    if self.view_mode == ViewMode::List {
                        " LIST "
                    } else {
                        " DETAILS "
                    },
                    Theme::badge_neutral(),
                ),
                Span::raw(" "),
                if self.is_searching {
                    Span::styled(" SEARCH ", Theme::badge_warning())
                } else {
                    Span::styled(" IDLE ", Theme::badge_success())
                },
            ]),
            Line::from(""),
            Line::from(Span::styled(
                "Browse installed packages, inspect metadata, then remove with confirmation.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel_alt(Theme::panel_title("Runtime")));
        frame.render_widget(runtime, header[1]);

        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(62), Constraint::Percentage(38)])
            .split(chunks[1]);

        if self.view_mode == ViewMode::Details {
            if let Some(pkg) = self.selected_package() {
                self.render_details(frame, content[0], pkg);
                self.render_selected_package(frame, content[1]);
            }
        } else {
            self.render_list(frame, content[0]);
            self.render_selected_package(frame, content[1]);
        }

        // Status bar
        let status_content = if self.show_confirm {
            Line::from(vec![
                Span::styled(
                    format!(
                        "Remove package '{}'? ",
                        self.selected_package
                            .as_ref()
                            .map(|p| p.name.as_str())
                            .unwrap_or("?")
                    ),
                    Theme::warning(),
                ),
                Span::raw("[Y]es / [N]o"),
            ])
        } else if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                if *is_error {
                    Theme::error()
                } else {
                    Theme::success()
                },
            ))
        } else if let Some(pkg) = self.selected_package() {
            Line::from(vec![
                Span::styled("Footprint ", Theme::label()),
                Span::raw(format!(
                    "{} compressed, {} installed",
                    pkg.size_compressed, pkg.size_uncompressed
                )),
            ])
        } else {
            Line::from(Span::styled("No package selected", Theme::muted()))
        };

        let status =
            Paragraph::new(status_content).block(Theme::panel_alt(Theme::panel_title("Status")));
        frame.render_widget(status, chunks[2]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        if self.is_searching {
            vec![("Enter/Esc", "Done"), ("Type", "Search")]
        } else {
            vec![
                ("/", "Search"),
                ("Enter", "Details"),
                ("d", "Remove"),
                ("c", "Clear"),
            ]
        }
    }

    fn on_activate(&mut self) {
        self.load_packages();
        self.apply_filter();
    }
}

impl PackageBrowserComponent {
    fn render_list(&self, frame: &mut Frame, area: Rect) {
        let items: Vec<ListItem> = self
            .filtered_packages
            .iter()
            .filter_map(|&idx| self.packages.get(idx))
            .map(|pkg| {
                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(format!("{:<28}", pkg.name), Theme::title()),
                        Span::styled(format!(" {:<14}", pkg.version), Theme::success()),
                        Span::styled(format!(" {:<10}", pkg.arch), Theme::accent()),
                    ]),
                    Line::from(Span::styled(
                        format!(
                            "  {}",
                            if pkg.description.len() > 70 {
                                format!("{}...", &pkg.description[..67])
                            } else {
                                pkg.description.clone()
                            }
                        ),
                        Theme::muted(),
                    )),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(Theme::panel_alt(Theme::panel_title("Package roster")))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, area, &mut state);
    }

    fn render_details(&self, frame: &mut Frame, area: Rect, pkg: &InstalledPackage) {
        let block = Theme::panel(Theme::panel_title(format!("Package {}", pkg.name)));

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let details = vec![
            Line::from(vec![Span::styled("Identity", Theme::eyebrow())]),
            Line::from(vec![
                Span::styled("Name         ", Theme::label()),
                Span::raw(&pkg.name),
            ]),
            Line::from(vec![
                Span::styled("Version      ", Theme::label()),
                Span::raw(&pkg.version),
            ]),
            Line::from(vec![
                Span::styled("Architecture ", Theme::label()),
                Span::raw(&pkg.arch),
            ]),
            Line::from(vec![
                Span::styled("Build        ", Theme::label()),
                Span::raw(&pkg.build),
            ]),
            Line::from(vec![
                Span::styled("Full name    ", Theme::label()),
                Span::raw(&pkg.full_name),
            ]),
            Line::from(""),
            Line::from(vec![Span::styled("Payload", Theme::eyebrow())]),
            Line::from(vec![
                Span::styled("Compressed   ", Theme::label()),
                Span::raw(&pkg.size_compressed),
            ]),
            Line::from(vec![
                Span::styled("Uncompressed ", Theme::label()),
                Span::raw(&pkg.size_uncompressed),
            ]),
            Line::from(""),
            Line::from(Span::styled("Description", Theme::eyebrow())),
            Line::from(Span::styled(
                "Package notes from /var/log/packages",
                Theme::subtitle(),
            )),
            Line::from(""),
        ];

        let mut lines = details;
        // Wrap description text
        for line in pkg
            .description
            .chars()
            .collect::<Vec<_>>()
            .chunks(inner.width as usize - 2)
        {
            lines.push(Line::from(Span::raw(line.iter().collect::<String>())));
        }

        let paragraph = Paragraph::new(lines).wrap(Wrap { trim: true });
        frame.render_widget(paragraph, inner);
    }
}
