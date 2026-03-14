use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Style},
    text::{Line, Span},
    widgets::{List, ListItem, ListState, Paragraph, Wrap},
    Frame,
};
use std::fs::{self, File};
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// Log file information
#[derive(Debug, Clone)]
pub struct LogFile {
    pub name: String,
    pub path: PathBuf,
    pub size: u64,
    pub modified: String,
}

/// Log Viewer Component
pub struct LogViewerComponent {
    log_files: Vec<LogFile>,
    file_list_state: ListState,
    mode: LogViewMode,
    log_content: Vec<String>,
    content_scroll: usize,
    search_query: String,
    is_searching: bool,
    search_results: Vec<usize>,
    current_search_idx: usize,
    follow_mode: bool,
    status_message: Option<(String, bool)>,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum LogViewMode {
    FileList,
    ViewLog,
}

const LOG_DIRS: &[&str] = &["/var/log"];

const IMPORTANT_LOGS: &[&str] = &[
    "messages",
    "syslog",
    "dmesg",
    "secure",
    "auth.log",
    "boot",
    "Xorg.0.log",
    "packages/",
    "slackpkg.log",
    "lastlog",
    "wtmp",
    "btmp",
];

impl LogViewerComponent {
    pub fn new() -> Self {
        let mut component = Self {
            log_files: Vec::new(),
            file_list_state: ListState::default(),
            mode: LogViewMode::FileList,
            log_content: Vec::new(),
            content_scroll: 0,
            search_query: String::new(),
            is_searching: false,
            search_results: Vec::new(),
            current_search_idx: 0,
            follow_mode: false,
            status_message: None,
        };
        component.load_log_files();
        if !component.log_files.is_empty() {
            component.file_list_state.select(Some(0));
        }
        component
    }

    fn load_log_files(&mut self) {
        self.log_files.clear();

        for log_dir in LOG_DIRS {
            self.scan_directory(Path::new(log_dir), 0);
        }

        // Sort by importance and name
        self.log_files.sort_by(|a, b| {
            let a_important = IMPORTANT_LOGS.iter().any(|&l| a.name.contains(l));
            let b_important = IMPORTANT_LOGS.iter().any(|&l| b.name.contains(l));

            match (a_important, b_important) {
                (true, false) => std::cmp::Ordering::Less,
                (false, true) => std::cmp::Ordering::Greater,
                _ => a.name.cmp(&b.name),
            }
        });
    }

    fn scan_directory(&mut self, path: &Path, depth: usize) {
        if depth > 2 {
            return;
        }

        if let Ok(entries) = fs::read_dir(path) {
            for entry in entries.filter_map(|e| e.ok()) {
                let entry_path = entry.path();

                if entry_path.is_dir() {
                    // Skip certain directories
                    let name = entry.file_name().to_string_lossy().to_string();
                    if name.starts_with('.') || name == "journal" {
                        continue;
                    }
                    self.scan_directory(&entry_path, depth + 1);
                } else if entry_path.is_file() {
                    let name = entry.file_name().to_string_lossy().to_string();

                    // Skip compressed logs and certain files
                    if name.ends_with(".gz")
                        || name.ends_with(".xz")
                        || name.ends_with(".old")
                        || name.starts_with('.')
                    {
                        continue;
                    }

                    if let Ok(metadata) = entry.metadata() {
                        let modified = metadata
                            .modified()
                            .ok()
                            .map(|t| {
                                let datetime: chrono::DateTime<chrono::Local> = t.into();
                                datetime.format("%Y-%m-%d %H:%M").to_string()
                            })
                            .unwrap_or_else(|| "Unknown".to_string());

                        // Create display name with relative path
                        let display_name = entry_path
                            .strip_prefix("/var/log")
                            .map(|p| p.to_string_lossy().to_string())
                            .unwrap_or_else(|_| name);

                        self.log_files.push(LogFile {
                            name: display_name,
                            path: entry_path,
                            size: metadata.len(),
                            modified,
                        });
                    }
                }
            }
        }
    }

    fn load_log_content(&mut self, path: &Path) {
        self.log_content.clear();
        self.content_scroll = 0;
        self.search_results.clear();

        // Read last N lines (tail behavior)
        const MAX_LINES: usize = 1000;

        match File::open(path) {
            Ok(file) => {
                let reader = BufReader::new(file);
                let mut lines: Vec<String> = reader.lines().map_while(Result::ok).collect();

                // Keep only last MAX_LINES
                if lines.len() > MAX_LINES {
                    lines = lines.split_off(lines.len() - MAX_LINES);
                }

                self.log_content = lines;

                // Scroll to end if in follow mode
                if self.follow_mode && !self.log_content.is_empty() {
                    self.content_scroll = self.log_content.len().saturating_sub(1);
                }
            }
            Err(e) => {
                self.log_content = vec![format!("Error reading file: {}", e)];
            }
        }
    }

    fn refresh_log(&mut self) {
        if let Some(selected) = self.file_list_state.selected() {
            if let Some(log) = self.log_files.get(selected) {
                let path = log.path.clone();
                self.load_log_content(&path);
            }
        }
    }

    fn selected_log(&self) -> Option<&LogFile> {
        self.file_list_state
            .selected()
            .and_then(|i| self.log_files.get(i))
    }

    fn perform_search(&mut self) {
        self.search_results.clear();
        self.current_search_idx = 0;

        if self.search_query.is_empty() {
            return;
        }

        let query = self.search_query.to_lowercase();
        for (i, line) in self.log_content.iter().enumerate() {
            if line.to_lowercase().contains(&query) {
                self.search_results.push(i);
            }
        }

        // Jump to first result
        if !self.search_results.is_empty() {
            self.content_scroll = self.search_results[0];
        }
    }

    fn next_search_result(&mut self) {
        if self.search_results.is_empty() {
            return;
        }

        self.current_search_idx = (self.current_search_idx + 1) % self.search_results.len();
        self.content_scroll = self.search_results[self.current_search_idx];
    }

    fn prev_search_result(&mut self) {
        if self.search_results.is_empty() {
            return;
        }

        self.current_search_idx = if self.current_search_idx == 0 {
            self.search_results.len() - 1
        } else {
            self.current_search_idx - 1
        };
        self.content_scroll = self.search_results[self.current_search_idx];
    }

    fn format_size(bytes: u64) -> String {
        const KB: u64 = 1024;
        const MB: u64 = KB * 1024;

        if bytes >= MB {
            format!("{:.1}M", bytes as f64 / MB as f64)
        } else if bytes >= KB {
            format!("{:.1}K", bytes as f64 / KB as f64)
        } else {
            format!("{}B", bytes)
        }
    }

    fn get_log_level_color(line: &str) -> Color {
        let lower = line.to_lowercase();
        if lower.contains("error") || lower.contains("fail") || lower.contains("crit") {
            Color::Red
        } else if lower.contains("warn") {
            Color::Yellow
        } else if lower.contains("info") {
            Color::Cyan
        } else if lower.contains("debug") {
            Color::DarkGray
        } else {
            Color::White
        }
    }
}

impl Component for LogViewerComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.is_searching {
            match key.code {
                KeyCode::Enter => {
                    self.is_searching = false;
                    self.perform_search();
                }
                KeyCode::Esc => {
                    self.is_searching = false;
                    self.search_query.clear();
                    self.search_results.clear();
                }
                KeyCode::Backspace => {
                    self.search_query.pop();
                }
                KeyCode::Char(c) => {
                    self.search_query.push(c);
                }
                _ => {}
            }
            return None;
        }

        match self.mode {
            LogViewMode::FileList => match key.code {
                KeyCode::Up | KeyCode::Char('k') => {
                    if let Some(selected) = self.file_list_state.selected() {
                        if selected > 0 {
                            self.file_list_state.select(Some(selected - 1));
                        }
                    }
                }
                KeyCode::Down | KeyCode::Char('j') => {
                    if let Some(selected) = self.file_list_state.selected() {
                        if selected < self.log_files.len().saturating_sub(1) {
                            self.file_list_state.select(Some(selected + 1));
                        }
                    }
                }
                KeyCode::Enter => {
                    if let Some(log) = self.selected_log() {
                        let path = log.path.clone();
                        self.load_log_content(&path);
                        self.mode = LogViewMode::ViewLog;
                    }
                }
                KeyCode::Home => {
                    self.file_list_state.select(Some(0));
                }
                KeyCode::End => {
                    if !self.log_files.is_empty() {
                        self.file_list_state.select(Some(self.log_files.len() - 1));
                    }
                }
                KeyCode::F(5) => {
                    self.load_log_files();
                    self.status_message = Some(("Log list refreshed".to_string(), false));
                }
                _ => {}
            },
            LogViewMode::ViewLog => match key.code {
                KeyCode::Esc | KeyCode::Char('q') => {
                    self.mode = LogViewMode::FileList;
                    self.log_content.clear();
                    self.search_query.clear();
                    self.search_results.clear();
                }
                KeyCode::Up | KeyCode::Char('k') => {
                    if self.content_scroll > 0 {
                        self.content_scroll -= 1;
                        self.follow_mode = false;
                    }
                }
                KeyCode::Down | KeyCode::Char('j') => {
                    if self.content_scroll < self.log_content.len().saturating_sub(1) {
                        self.content_scroll += 1;
                    }
                }
                KeyCode::PageUp => {
                    self.content_scroll = self.content_scroll.saturating_sub(20);
                    self.follow_mode = false;
                }
                KeyCode::PageDown => {
                    self.content_scroll =
                        (self.content_scroll + 20).min(self.log_content.len().saturating_sub(1));
                }
                KeyCode::Home | KeyCode::Char('g') => {
                    self.content_scroll = 0;
                    self.follow_mode = false;
                }
                KeyCode::End | KeyCode::Char('G') => {
                    self.content_scroll = self.log_content.len().saturating_sub(1);
                }
                KeyCode::Char('/') => {
                    self.is_searching = true;
                    self.search_query.clear();
                }
                KeyCode::Char('n') => {
                    self.next_search_result();
                }
                KeyCode::Char('N') => {
                    self.prev_search_result();
                }
                KeyCode::Char('f') => {
                    self.follow_mode = !self.follow_mode;
                    if self.follow_mode {
                        self.refresh_log();
                        self.status_message = Some(("Follow mode enabled".to_string(), false));
                    } else {
                        self.status_message = Some(("Follow mode disabled".to_string(), false));
                    }
                }
                KeyCode::F(5) => {
                    self.refresh_log();
                    self.status_message = Some(("Log refreshed".to_string(), false));
                }
                _ => {}
            },
        }
        None
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        match self.mode {
            LogViewMode::FileList => self.render_file_list(frame, area),
            LogViewMode::ViewLog => self.render_log_view(frame, area),
        }
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        match self.mode {
            LogViewMode::FileList => {
                vec![("Enter", "Open"), ("↑/↓", "Navigate"), ("F5", "Refresh")]
            }
            LogViewMode::ViewLog => vec![
                ("q/Esc", "Back"),
                ("/", "Search"),
                ("n/N", "Next/Prev"),
                ("f", "Follow"),
            ],
        }
    }

    fn on_activate(&mut self) {
        self.load_log_files();
    }
}

impl LogViewerComponent {
    fn render_file_list(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(4),
                Constraint::Min(10),
                Constraint::Length(3),
            ])
            .split(area);

        let total_size: u64 = self.log_files.iter().map(|log| log.size).sum();
        let important_count = self
            .log_files
            .iter()
            .filter(|log| IMPORTANT_LOGS.iter().any(|entry| log.name.contains(entry)))
            .count();

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
            .split(chunks[0]);

        let title_panel = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("LOG INDEX", Theme::badge_info()),
                Span::raw(" "),
                Span::styled("Filesystem log inventory", Theme::title()),
            ]),
            Line::from(Span::styled(
                "Browse curated system logs before opening a file for line-by-line inspection.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel(Theme::panel_title("Log viewer")));
        frame.render_widget(title_panel, header[0]);

        let runtime_panel = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Files ", Theme::label()),
                Span::styled(self.log_files.len().to_string(), Theme::badge_neutral()),
                Span::raw(" "),
                Span::styled("Important ", Theme::label()),
                Span::styled(important_count.to_string(), Theme::badge_success()),
            ]),
            Line::from(vec![
                Span::styled("Visible size ", Theme::label()),
                Span::styled(Self::format_size(total_size), Theme::badge_neutral()),
            ]),
        ])
        .block(Theme::panel_alt(Theme::panel_title("Runtime")));
        frame.render_widget(runtime_panel, header[1]);

        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(66), Constraint::Percentage(34)])
            .split(chunks[1]);

        let items: Vec<ListItem> = self
            .log_files
            .iter()
            .map(|log| {
                ListItem::new(Line::from(vec![
                    Span::styled(format!("{:<38}", log.name), Theme::title()),
                    Span::styled(
                        format!("{:>8}", Self::format_size(log.size)),
                        Theme::accent(),
                    ),
                    Span::styled(format!("  {}", log.modified), Theme::muted()),
                ]))
            })
            .collect();

        let list = List::new(items)
            .block(Theme::panel(Theme::panel_title(format!(
                "Log files ({})",
                self.log_files.len()
            ))))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.file_list_state.clone();
        frame.render_stateful_widget(list, content[0], &mut state);

        let inspector_lines = if let Some(log) = self.selected_log() {
            vec![
                Line::from(Span::styled(&log.name, Theme::title())),
                Line::from(vec![
                    Span::styled("Size ", Theme::label()),
                    Span::styled(Self::format_size(log.size), Theme::badge_neutral()),
                ]),
                Line::from(vec![
                    Span::styled("Modified ", Theme::label()),
                    Span::raw(&log.modified),
                ]),
                Line::from(""),
                Line::from(Span::styled("Path", Theme::eyebrow())),
                Line::from(log.path.to_string_lossy().to_string()),
                Line::from(""),
                Line::from(Span::styled(
                    "Press Enter to open and inspect live content.",
                    Theme::subtitle(),
                )),
            ]
        } else {
            vec![
                Line::from(Span::styled("No log selected", Theme::muted())),
                Line::from(""),
                Line::from("Pick a log file to preview its metadata."),
            ]
        };
        let inspector = Paragraph::new(inspector_lines)
            .block(Theme::panel_alt(Theme::panel_title("Inspector")));
        frame.render_widget(inspector, content[1]);

        // Status bar
        let status_content = if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                if *is_error {
                    Theme::error()
                } else {
                    Theme::success()
                },
            ))
        } else if let Some(log) = self.selected_log() {
            Line::from(vec![
                Span::styled("Path: ", Theme::muted()),
                Span::raw(log.path.to_string_lossy().to_string()),
            ])
        } else {
            Line::from(Span::styled("Select a log file", Theme::muted()))
        };

        let status =
            Paragraph::new(status_content).block(Theme::panel_alt(Theme::panel_title("Status")));
        frame.render_widget(status, chunks[2]);
    }

    fn render_log_view(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(4),
                Constraint::Min(10),
                Constraint::Length(3),
            ])
            .split(area);

        // Header with search
        let title = self
            .selected_log()
            .map(|l| l.name.clone())
            .unwrap_or_else(|| "Log".to_string());

        let search_display = if self.is_searching {
            format!("Search: {}█", self.search_query)
        } else if !self.search_query.is_empty() {
            format!(
                "Search: {} ({}/{})",
                self.search_query,
                if self.search_results.is_empty() {
                    0
                } else {
                    self.current_search_idx + 1
                },
                self.search_results.len()
            )
        } else {
            String::new()
        };

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(62), Constraint::Percentage(38)])
            .split(chunks[0]);

        let title_panel = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("LIVE FILE", Theme::badge_info()),
                Span::raw(" "),
                Span::styled(&title, Theme::title()),
            ]),
            Line::from(Span::styled(
                "Navigate by line, search in-place, or follow the live tail.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel(Theme::panel_title("Log viewer")));
        frame.render_widget(title_panel, header[0]);

        let meta_panel = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Search ", Theme::label()),
                Span::styled(
                    if search_display.is_empty() {
                        "idle"
                    } else {
                        search_display.as_str()
                    },
                    if self.is_searching {
                        Theme::warning()
                    } else {
                        Theme::key_hint_secondary()
                    },
                ),
            ]),
            Line::from(vec![
                Span::styled("Mode ", Theme::label()),
                if self.follow_mode {
                    Span::styled(" FOLLOW ", Theme::badge_success())
                } else {
                    Span::styled(" STATIC ", Theme::badge_neutral())
                },
            ]),
            Line::from(vec![
                Span::styled("Lines ", Theme::label()),
                Span::styled(self.log_content.len().to_string(), Theme::badge_neutral()),
                Span::raw(" "),
                Span::styled("Matches ", Theme::label()),
                Span::styled(
                    self.search_results.len().to_string(),
                    Theme::badge_success(),
                ),
            ]),
        ])
        .block(Theme::panel_alt(Theme::panel_title("Runtime")));
        frame.render_widget(meta_panel, header[1]);

        // Log content
        let visible_height = chunks[1].height.saturating_sub(2) as usize;
        let start = self.content_scroll;
        let end = (start + visible_height).min(self.log_content.len());

        let lines: Vec<Line> = self.log_content[start..end]
            .iter()
            .enumerate()
            .map(|(i, line)| {
                let line_num = start + i;
                let is_search_match = self.search_results.contains(&line_num);

                let style = if is_search_match {
                    Style::default().bg(Color::Yellow).fg(Color::Black)
                } else {
                    Style::default().fg(Self::get_log_level_color(line))
                };

                Line::from(vec![
                    Span::styled(format!("{:>6} ", line_num + 1), Theme::muted()),
                    Span::styled(line.clone(), style),
                ])
            })
            .collect();

        let content = Paragraph::new(lines)
            .block(Theme::panel_alt(Theme::panel_title("Stream")))
            .wrap(Wrap { trim: false });
        frame.render_widget(content, chunks[1]);

        // Status bar
        let status = Paragraph::new(Line::from(vec![
            Span::styled("Line: ", Theme::muted()),
            Span::raw(format!(
                "{}/{}",
                self.content_scroll + 1,
                self.log_content.len()
            )),
            if let Some((msg, is_error)) = &self.status_message {
                Span::styled(
                    format!("  {}", msg),
                    Style::default().fg(if *is_error { Color::Red } else { Color::Green }),
                )
            } else {
                Span::raw("")
            },
        ]))
        .block(Theme::panel_alt(Theme::panel_title("Position")));
        frame.render_widget(status, chunks[2]);
    }
}
