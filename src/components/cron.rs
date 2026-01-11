use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use std::fs;
use std::path::Path;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// Cron job entry
#[derive(Debug, Clone)]
pub struct CronJob {
    pub minute: String,
    pub hour: String,
    pub day: String,
    pub month: String,
    pub weekday: String,
    pub command: String,
    pub source: CronSource,
    pub enabled: bool,
    pub raw_line: String,
}

#[derive(Debug, Clone)]
pub enum CronSource {
    System(String),    // Path to file in /etc/cron.*
    User(String),      // Username
}

/// Cron Job Manager Component
pub struct CronComponent {
    jobs: Vec<CronJob>,
    list_state: ListState,
    mode: CronMode,
    filter: CronFilter,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
    pending_action: Option<CronAction>,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum CronMode {
    View,
    Add,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum CronFilter {
    All,
    System,
    User,
    Hourly,
    Daily,
    Weekly,
    Monthly,
}

#[derive(Debug, Clone)]
pub enum CronAction {
    Delete(usize),
    Toggle(usize),
}

impl CronComponent {
    pub fn new() -> Self {
        let mut component = Self {
            jobs: Vec::new(),
            list_state: ListState::default(),
            mode: CronMode::View,
            filter: CronFilter::All,
            status_message: None,
            show_confirm: false,
            pending_action: None,
        };
        component.load_cron_jobs();
        if !component.jobs.is_empty() {
            component.list_state.select(Some(0));
        }
        component
    }

    fn load_cron_jobs(&mut self) {
        self.jobs.clear();

        // Load system cron directories
        self.load_cron_dir("/etc/cron.hourly", "hourly");
        self.load_cron_dir("/etc/cron.daily", "daily");
        self.load_cron_dir("/etc/cron.weekly", "weekly");
        self.load_cron_dir("/etc/cron.monthly", "monthly");

        // Load /etc/crontab
        self.load_crontab("/etc/crontab");

        // Load /etc/cron.d/*
        if let Ok(entries) = fs::read_dir("/etc/cron.d") {
            for entry in entries.filter_map(|e| e.ok()) {
                let path = entry.path();
                if path.is_file() {
                    self.load_crontab_file(&path);
                }
            }
        }

        // Load user crontabs
        self.load_user_crontabs();
    }

    fn load_cron_dir(&mut self, dir: &str, period: &str) {
        let path = Path::new(dir);
        if !path.exists() {
            return;
        }

        if let Ok(entries) = fs::read_dir(path) {
            for entry in entries.filter_map(|e| e.ok()) {
                let entry_path = entry.path();
                if entry_path.is_file() {
                    let name = entry.file_name().to_string_lossy().to_string();

                    // Skip backup files
                    if name.ends_with("~") || name.starts_with('.') {
                        continue;
                    }

                    let is_executable = entry
                        .metadata()
                        .map(|m| m.permissions().mode() & 0o111 != 0)
                        .unwrap_or(false);

                    self.jobs.push(CronJob {
                        minute: "*".to_string(),
                        hour: match period {
                            "hourly" => "*".to_string(),
                            _ => "0".to_string(),
                        },
                        day: "*".to_string(),
                        month: "*".to_string(),
                        weekday: match period {
                            "weekly" => "0".to_string(),
                            _ => "*".to_string(),
                        },
                        command: name.clone(),
                        source: CronSource::System(format!("{}/{}", dir, name)),
                        enabled: is_executable,
                        raw_line: format!("@{} {}", period, name),
                    });
                }
            }
        }
    }

    fn load_crontab(&mut self, path: &str) {
        if let Ok(content) = fs::read_to_string(path) {
            for line in content.lines() {
                if let Some(job) = self.parse_cron_line(line, CronSource::System(path.to_string())) {
                    self.jobs.push(job);
                }
            }
        }
    }

    fn load_crontab_file(&mut self, path: &Path) {
        if let Ok(content) = fs::read_to_string(path) {
            let source = CronSource::System(path.to_string_lossy().to_string());
            for line in content.lines() {
                if let Some(job) = self.parse_cron_line(line, source.clone()) {
                    self.jobs.push(job);
                }
            }
        }
    }

    fn load_user_crontabs(&mut self) {
        let crontab_dir = Path::new("/var/spool/cron/crontabs");
        if !crontab_dir.exists() {
            return;
        }

        if let Ok(entries) = fs::read_dir(crontab_dir) {
            for entry in entries.filter_map(|e| e.ok()) {
                let username = entry.file_name().to_string_lossy().to_string();
                if let Ok(content) = fs::read_to_string(entry.path()) {
                    for line in content.lines() {
                        if let Some(job) = self.parse_cron_line(line, CronSource::User(username.clone())) {
                            self.jobs.push(job);
                        }
                    }
                }
            }
        }
    }

    fn parse_cron_line(&self, line: &str, source: CronSource) -> Option<CronJob> {
        let line = line.trim();

        // Skip empty lines and comments
        if line.is_empty() || line.starts_with('#') {
            return None;
        }

        // Skip variable assignments
        if line.contains('=') && !line.contains(' ') {
            return None;
        }

        let parts: Vec<&str> = line.split_whitespace().collect();

        // Handle special time specifications
        if parts[0].starts_with('@') {
            let (minute, hour, day, month, weekday) = match parts[0] {
                "@reboot" => ("@reboot".to_string(), "-".to_string(), "-".to_string(), "-".to_string(), "-".to_string()),
                "@yearly" | "@annually" => ("0".to_string(), "0".to_string(), "1".to_string(), "1".to_string(), "*".to_string()),
                "@monthly" => ("0".to_string(), "0".to_string(), "1".to_string(), "*".to_string(), "*".to_string()),
                "@weekly" => ("0".to_string(), "0".to_string(), "*".to_string(), "*".to_string(), "0".to_string()),
                "@daily" | "@midnight" => ("0".to_string(), "0".to_string(), "*".to_string(), "*".to_string(), "*".to_string()),
                "@hourly" => ("0".to_string(), "*".to_string(), "*".to_string(), "*".to_string(), "*".to_string()),
                _ => return None,
            };

            let command = parts[1..].join(" ");
            return Some(CronJob {
                minute,
                hour,
                day,
                month,
                weekday,
                command,
                source,
                enabled: true,
                raw_line: line.to_string(),
            });
        }

        // Standard cron format: min hour day month weekday command
        if parts.len() >= 6 {
            // Check if 6th field is a username (system crontab format)
            let (cmd_start, _) = if matches!(&source, CronSource::System(p) if p == "/etc/crontab" || p.starts_with("/etc/cron.d")) {
                (6, Some(parts[5])) // Skip username field
            } else {
                (5, None)
            };

            if parts.len() > cmd_start {
                return Some(CronJob {
                    minute: parts[0].to_string(),
                    hour: parts[1].to_string(),
                    day: parts[2].to_string(),
                    month: parts[3].to_string(),
                    weekday: parts[4].to_string(),
                    command: parts[cmd_start..].join(" "),
                    source,
                    enabled: true,
                    raw_line: line.to_string(),
                });
            }
        }

        None
    }

    fn filtered_jobs(&self) -> Vec<(usize, &CronJob)> {
        self.jobs
            .iter()
            .enumerate()
            .filter(|(_, job)| match self.filter {
                CronFilter::All => true,
                CronFilter::System => matches!(job.source, CronSource::System(_)),
                CronFilter::User => matches!(job.source, CronSource::User(_)),
                CronFilter::Hourly => job.raw_line.contains("hourly") || (job.minute == "0" && job.hour == "*"),
                CronFilter::Daily => job.raw_line.contains("daily") || (job.hour == "0" && job.day == "*"),
                CronFilter::Weekly => job.raw_line.contains("weekly") || job.weekday != "*",
                CronFilter::Monthly => job.raw_line.contains("monthly") || (job.day == "1" && job.month == "*"),
            })
            .collect()
    }

    fn selected_job(&self) -> Option<(usize, &CronJob)> {
        let filtered = self.filtered_jobs();
        self.list_state.selected().and_then(|i| filtered.get(i).copied())
    }

    fn format_schedule(&self, job: &CronJob) -> String {
        if job.minute == "@reboot" {
            return "At reboot".to_string();
        }

        // Try to create human-readable schedule
        let mut parts = Vec::new();

        if job.minute != "*" && job.minute != "0" {
            parts.push(format!(":{}", job.minute));
        }

        if job.hour == "*" {
            parts.push("Every hour".to_string());
        } else {
            parts.push(format!("{}:00", job.hour));
        }

        if job.day != "*" {
            parts.push(format!("day {}", job.day));
        }

        if job.weekday != "*" {
            let day_name = match job.weekday.as_str() {
                "0" | "7" => "Sun",
                "1" => "Mon",
                "2" => "Tue",
                "3" => "Wed",
                "4" => "Thu",
                "5" => "Fri",
                "6" => "Sat",
                _ => &job.weekday,
            };
            parts.push(day_name.to_string());
        }

        if parts.is_empty() {
            format!("{} {} {} {} {}", job.minute, job.hour, job.day, job.month, job.weekday)
        } else {
            parts.join(" ")
        }
    }

    fn source_display(&self, source: &CronSource) -> (String, Color) {
        match source {
            CronSource::System(path) => {
                if path.contains("hourly") {
                    ("hourly".to_string(), Color::Cyan)
                } else if path.contains("daily") {
                    ("daily".to_string(), Color::Green)
                } else if path.contains("weekly") {
                    ("weekly".to_string(), Color::Yellow)
                } else if path.contains("monthly") {
                    ("monthly".to_string(), Color::Magenta)
                } else {
                    ("system".to_string(), Color::Blue)
                }
            }
            CronSource::User(name) => (name.clone(), Color::White),
        }
    }
}

impl Component for CronComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    self.pending_action = None;
                    self.status_message = Some(("Action not implemented for safety".to_string(), true));
                }
                KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                    self.show_confirm = false;
                    self.pending_action = None;
                }
                _ => {}
            }
            return None;
        }

        let filtered_len = self.filtered_jobs().len();

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
                    if selected < filtered_len.saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                } else if filtered_len > 0 {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::Tab => {
                self.filter = match self.filter {
                    CronFilter::All => CronFilter::System,
                    CronFilter::System => CronFilter::User,
                    CronFilter::User => CronFilter::Hourly,
                    CronFilter::Hourly => CronFilter::Daily,
                    CronFilter::Daily => CronFilter::Weekly,
                    CronFilter::Weekly => CronFilter::Monthly,
                    CronFilter::Monthly => CronFilter::All,
                };
                self.list_state.select(Some(0));
            }
            KeyCode::F(5) => {
                self.load_cron_jobs();
                self.status_message = Some(("Cron jobs refreshed".to_string(), false));
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

        // Filter bar
        let filter_text = match self.filter {
            CronFilter::All => "[All] System User Hourly Daily Weekly Monthly",
            CronFilter::System => " All [System] User Hourly Daily Weekly Monthly",
            CronFilter::User => " All System [User] Hourly Daily Weekly Monthly",
            CronFilter::Hourly => " All System User [Hourly] Daily Weekly Monthly",
            CronFilter::Daily => " All System User Hourly [Daily] Weekly Monthly",
            CronFilter::Weekly => " All System User Hourly Daily [Weekly] Monthly",
            CronFilter::Monthly => " All System User Hourly Daily Weekly [Monthly]",
        };

        let filtered_jobs = self.filtered_jobs();
        let filter_bar = Paragraph::new(Line::from(vec![
            Span::styled("Filter: ", Style::default().fg(Color::Cyan)),
            Span::raw(filter_text),
            Span::styled(
                format!("  ({} jobs)", filtered_jobs.len()),
                Style::default().fg(Color::DarkGray),
            ),
        ]))
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title(" Cron Job Manager "),
        );
        frame.render_widget(filter_bar, chunks[0]);

        // Job list
        let items: Vec<ListItem> = filtered_jobs
            .iter()
            .map(|(_, job)| {
                let (source_name, source_color) = self.source_display(&job.source);

                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            format!("{:<10}", source_name),
                            Style::default().fg(source_color),
                        ),
                        Span::styled(
                            if job.enabled { "●" } else { "○" },
                            Style::default().fg(if job.enabled {
                                Color::Green
                            } else {
                                Color::Red
                            }),
                        ),
                        Span::raw(" "),
                        Span::styled(
                            format!("{:<20}", self.format_schedule(job)),
                            Style::default().fg(Color::Yellow),
                        ),
                    ]),
                    Line::from(vec![
                        Span::styled("    ", Style::default()),
                        Span::raw(if job.command.len() > 60 {
                            format!("{}...", &job.command[..57])
                        } else {
                            job.command.clone()
                        }),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(Block::default().borders(Borders::ALL))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, chunks[1], &mut state);

        // Status bar
        let status_content = if self.show_confirm {
            Line::from(vec![
                Span::styled("Confirm action? ", Style::default().fg(Color::Yellow)),
                Span::raw("[Y]es / [N]o"),
            ])
        } else if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                Style::default().fg(if *is_error { Color::Red } else { Color::Green }),
            ))
        } else if let Some((_, job)) = self.selected_job() {
            let source_path = match &job.source {
                CronSource::System(p) => p.clone(),
                CronSource::User(u) => format!("/var/spool/cron/crontabs/{}", u),
            };
            Line::from(vec![
                Span::styled("Source: ", Style::default().fg(Color::Cyan)),
                Span::raw(source_path),
            ])
        } else {
            Line::from(Span::raw("No job selected"))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
        frame.render_widget(status, chunks[2]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("Tab", "Filter"),
            ("↑/↓", "Navigate"),
            ("F5", "Refresh"),
        ]
    }

    fn on_activate(&mut self) {
        self.load_cron_jobs();
    }
}

use std::os::unix::fs::PermissionsExt;
