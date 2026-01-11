use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Wrap},
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
                        pkg.size_compressed = Self::extract_size(&content, "COMPRESSED PACKAGE SIZE:");
                        pkg.size_uncompressed = Self::extract_size(&content, "UNCOMPRESSED PACKAGE SIZE:");
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

    fn remove_package(&mut self, name: &str) -> Option<Message> {
        match std::process::Command::new("removepkg")
            .arg(name)
            .output()
        {
            Ok(output) => {
                if output.status.success() {
                    self.status_message = Some((
                        format!("Package '{}' removed successfully", name),
                        false,
                    ));
                    self.load_packages();
                    self.apply_filter();
                } else {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    self.status_message = Some((
                        format!("Failed to remove package: {}", stderr),
                        true,
                    ));
                }
            }
            Err(e) => {
                self.status_message = Some((format!("Error executing removepkg: {}", e), true));
            }
        }
        None
    }
}

impl Component for PackageBrowserComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    if let Some(pkg) = self.selected_package.take() {
                        return self.remove_package(&pkg.full_name);
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
                    self.list_state.select(Some(self.filtered_packages.len() - 1));
                }
            }
            KeyCode::PageUp => {
                if let Some(selected) = self.list_state.selected() {
                    self.list_state.select(Some(selected.saturating_sub(10)));
                }
            }
            KeyCode::PageDown => {
                if let Some(selected) = self.list_state.selected() {
                    let new_idx = (selected + 10).min(self.filtered_packages.len().saturating_sub(1));
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
                Constraint::Length(3),
                Constraint::Min(10),
                Constraint::Length(3),
            ])
            .split(area);

        // Search bar
        let search_style = if self.is_searching {
            Style::default().fg(Color::Yellow)
        } else {
            Style::default()
        };
        let search_bar = Paragraph::new(Line::from(vec![
            Span::styled("Search: ", Style::default().fg(Color::Cyan)),
            Span::styled(&self.search_query, search_style),
            if self.is_searching {
                Span::styled("_", Style::default().fg(Color::Yellow))
            } else {
                Span::raw("")
            },
            Span::styled(
                format!(
                    "  ({}/{} packages)",
                    self.filtered_packages.len(),
                    self.packages.len()
                ),
                Style::default().fg(Color::DarkGray),
            ),
        ]))
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title(" Installed Packages "),
        );
        frame.render_widget(search_bar, chunks[0]);

        // Main content area
        if self.view_mode == ViewMode::Details {
            if let Some(pkg) = self.selected_package() {
                self.render_details(frame, chunks[1], pkg);
            }
        } else {
            self.render_list(frame, chunks[1]);
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
                    Style::default().fg(Color::Yellow),
                ),
                Span::raw("[Y]es / [N]o"),
            ])
        } else if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                Style::default().fg(if *is_error { Color::Red } else { Color::Green }),
            ))
        } else if let Some(pkg) = self.selected_package() {
            Line::from(vec![
                Span::styled("Size: ", Style::default().fg(Color::Cyan)),
                Span::raw(format!(
                    "{} compressed, {} installed",
                    pkg.size_compressed, pkg.size_uncompressed
                )),
            ])
        } else {
            Line::from(Span::raw("No package selected"))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
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
                        Span::styled(
                            format!("{:<30}", pkg.name),
                            Style::default().add_modifier(Modifier::BOLD),
                        ),
                        Span::styled(
                            format!(" {:<15}", pkg.version),
                            Style::default().fg(Color::Green),
                        ),
                        Span::styled(
                            format!(" {:<10}", pkg.arch),
                            Style::default().fg(Color::Cyan),
                        ),
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
                        Style::default().fg(Color::DarkGray),
                    )),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(Block::default().borders(Borders::ALL))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, area, &mut state);
    }

    fn render_details(&self, frame: &mut Frame, area: Rect, pkg: &InstalledPackage) {
        let block = Block::default()
            .borders(Borders::ALL)
            .title(format!(" Package: {} ", pkg.name));

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let details = vec![
            Line::from(vec![
                Span::styled("Name:         ", Style::default().fg(Color::Cyan)),
                Span::raw(&pkg.name),
            ]),
            Line::from(vec![
                Span::styled("Version:      ", Style::default().fg(Color::Cyan)),
                Span::raw(&pkg.version),
            ]),
            Line::from(vec![
                Span::styled("Architecture: ", Style::default().fg(Color::Cyan)),
                Span::raw(&pkg.arch),
            ]),
            Line::from(vec![
                Span::styled("Build:        ", Style::default().fg(Color::Cyan)),
                Span::raw(&pkg.build),
            ]),
            Line::from(vec![
                Span::styled("Full Name:    ", Style::default().fg(Color::Cyan)),
                Span::raw(&pkg.full_name),
            ]),
            Line::from(""),
            Line::from(vec![
                Span::styled("Compressed:   ", Style::default().fg(Color::Cyan)),
                Span::raw(&pkg.size_compressed),
            ]),
            Line::from(vec![
                Span::styled("Uncompressed: ", Style::default().fg(Color::Cyan)),
                Span::raw(&pkg.size_uncompressed),
            ]),
            Line::from(""),
            Line::from(Span::styled("Description:", Style::default().fg(Color::Cyan))),
            Line::from(""),
        ];

        let mut lines = details;
        // Wrap description text
        for line in pkg.description.chars().collect::<Vec<_>>().chunks(inner.width as usize - 2) {
            lines.push(Line::from(Span::raw(line.iter().collect::<String>())));
        }

        let paragraph = Paragraph::new(lines).wrap(Wrap { trim: true });
        frame.render_widget(paragraph, inner);
    }
}
