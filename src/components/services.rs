use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// Service information
#[derive(Debug, Clone)]
pub struct ServiceInfo {
    pub name: String,
    pub path: String,
    pub is_running: bool,
    pub is_enabled: bool,
    pub description: String,
}

impl ServiceInfo {
    pub fn status_display(&self) -> (&'static str, Color) {
        match (self.is_running, self.is_enabled) {
            (true, true) => ("● Running", Color::Green),
            (true, false) => ("● Running (disabled)", Color::Yellow),
            (false, true) => ("○ Stopped", Color::Red),
            (false, false) => ("○ Stopped (disabled)", Color::DarkGray),
        }
    }
}

/// Service Manager Component
pub struct ServiceComponent {
    services: Vec<ServiceInfo>,
    list_state: ListState,
    filter: ServiceFilter,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
    pending_action: Option<ServiceAction>,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum ServiceFilter {
    All,
    Running,
    Stopped,
    Enabled,
}

#[derive(Debug, Clone)]
pub enum ServiceAction {
    Start(String),
    Stop(String),
    Restart(String),
    Toggle(String),
}

impl ServiceComponent {
    pub fn new() -> Self {
        let mut component = Self {
            services: Vec::new(),
            list_state: ListState::default(),
            filter: ServiceFilter::All,
            status_message: None,
            show_confirm: false,
            pending_action: None,
        };
        component.load_services();
        if !component.services.is_empty() {
            component.list_state.select(Some(0));
        }
        component
    }

    pub fn load_services(&mut self) {
        let rc_d_path = Path::new("/etc/rc.d");
        let mut services = Vec::new();

        if let Ok(entries) = fs::read_dir(rc_d_path) {
            for entry in entries.filter_map(|e| e.ok()) {
                let path = entry.path();
                let name = entry.file_name().to_string_lossy().to_string();

                // Only include rc.* scripts (init scripts)
                if name.starts_with("rc.") && !name.ends_with("~") && !name.ends_with(".new") {
                    // Skip rc.M, rc.K, rc.S, etc. (runlevel scripts)
                    if name.len() == 4 && name.chars().nth(3).map(|c| c.is_uppercase()).unwrap_or(false) {
                        continue;
                    }
                    // Skip rc.local_shutdown and similar
                    if name == "rc.local_shutdown" {
                        continue;
                    }

                    let is_enabled = if let Ok(metadata) = fs::metadata(&path) {
                        metadata.permissions().mode() & 0o111 != 0
                    } else {
                        false
                    };

                    let is_running = Self::check_if_running(&name);
                    let description = Self::get_service_description(&path);

                    services.push(ServiceInfo {
                        name: name.clone(),
                        path: path.to_string_lossy().to_string(),
                        is_running,
                        is_enabled,
                        description,
                    });
                }
            }
        }

        services.sort_by(|a, b| a.name.cmp(&b.name));
        self.services = services;
    }

    fn check_if_running(service_name: &str) -> bool {
        // Try to determine if service is running based on common patterns
        let daemon_name = service_name
            .trim_start_matches("rc.")
            .replace("_", "");

        // Check for PID file
        let pid_files = [
            format!("/var/run/{}.pid", daemon_name),
            format!("/var/run/{}/{}.pid", daemon_name, daemon_name),
            format!("/run/{}.pid", daemon_name),
        ];

        for pid_file in &pid_files {
            if Path::new(pid_file).exists() {
                if let Ok(pid_str) = fs::read_to_string(pid_file) {
                    if let Ok(pid) = pid_str.trim().parse::<i32>() {
                        if Path::new(&format!("/proc/{}", pid)).exists() {
                            return true;
                        }
                    }
                }
            }
        }

        // Check for common process patterns
        if let Ok(output) = std::process::Command::new("pgrep")
            .arg("-x")
            .arg(&daemon_name)
            .output()
        {
            if output.status.success() && !output.stdout.is_empty() {
                return true;
            }
        }

        false
    }

    fn get_service_description(path: &Path) -> String {
        if let Ok(content) = fs::read_to_string(path) {
            // Look for description in script comments
            for line in content.lines().take(20) {
                let line = line.trim();
                if line.starts_with('#') && !line.starts_with("#!") {
                    let desc = line.trim_start_matches('#').trim();
                    if !desc.is_empty() && desc.len() > 10 && !desc.contains('/') {
                        return desc.chars().take(60).collect();
                    }
                }
            }
        }
        "No description available".to_string()
    }

    fn filtered_services(&self) -> Vec<&ServiceInfo> {
        self.services
            .iter()
            .filter(|s| match self.filter {
                ServiceFilter::All => true,
                ServiceFilter::Running => s.is_running,
                ServiceFilter::Stopped => !s.is_running,
                ServiceFilter::Enabled => s.is_enabled,
            })
            .collect()
    }

    fn selected_service(&self) -> Option<&ServiceInfo> {
        let filtered = self.filtered_services();
        self.list_state.selected().and_then(|i| filtered.get(i).copied())
    }

    fn execute_action(&mut self, action: ServiceAction) -> Option<Message> {
        let (script_path, action_str) = match &action {
            ServiceAction::Start(name) => {
                (format!("/etc/rc.d/{}", name), "start")
            }
            ServiceAction::Stop(name) => {
                (format!("/etc/rc.d/{}", name), "stop")
            }
            ServiceAction::Restart(name) => {
                (format!("/etc/rc.d/{}", name), "restart")
            }
            ServiceAction::Toggle(name) => {
                // Toggle executable bit
                let path = format!("/etc/rc.d/{}", name);
                if let Ok(metadata) = fs::metadata(&path) {
                    let mut perms = metadata.permissions();
                    let mode = perms.mode();
                    if mode & 0o111 != 0 {
                        perms.set_mode(mode & !0o111);
                    } else {
                        perms.set_mode(mode | 0o755);
                    }
                    if let Err(e) = fs::set_permissions(&path, perms) {
                        self.status_message = Some((format!("Failed to toggle: {}", e), true));
                    } else {
                        self.status_message = Some((format!("Toggled {} executable bit", name), false));
                        self.load_services();
                    }
                }
                return None;
            }
        };

        match std::process::Command::new(&script_path)
            .arg(action_str)
            .output()
        {
            Ok(output) => {
                if output.status.success() {
                    self.status_message = Some((
                        format!("Service {} {}ed successfully", script_path, action_str),
                        false,
                    ));
                } else {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    self.status_message = Some((
                        format!("Failed to {} service: {}", action_str, stderr),
                        true,
                    ));
                }
            }
            Err(e) => {
                self.status_message = Some((format!("Failed to execute: {}", e), true));
            }
        }

        self.load_services();
        None
    }
}

impl Component for ServiceComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    if let Some(action) = self.pending_action.take() {
                        return self.execute_action(action);
                    }
                }
                KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                    self.show_confirm = false;
                    self.pending_action = None;
                }
                _ => {}
            }
            return None;
        }

        let filtered_len = self.filtered_services().len();

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
            KeyCode::Home => {
                if filtered_len > 0 {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::End => {
                if filtered_len > 0 {
                    self.list_state.select(Some(filtered_len - 1));
                }
            }
            KeyCode::Char('s') => {
                if let Some(service) = self.selected_service() {
                    self.pending_action = Some(ServiceAction::Start(service.name.clone()));
                    self.show_confirm = true;
                }
            }
            KeyCode::Char('x') => {
                if let Some(service) = self.selected_service() {
                    self.pending_action = Some(ServiceAction::Stop(service.name.clone()));
                    self.show_confirm = true;
                }
            }
            KeyCode::Char('r') => {
                if let Some(service) = self.selected_service() {
                    self.pending_action = Some(ServiceAction::Restart(service.name.clone()));
                    self.show_confirm = true;
                }
            }
            KeyCode::Char('e') => {
                if let Some(service) = self.selected_service() {
                    return self.execute_action(ServiceAction::Toggle(service.name.clone()));
                }
            }
            KeyCode::Tab => {
                self.filter = match self.filter {
                    ServiceFilter::All => ServiceFilter::Running,
                    ServiceFilter::Running => ServiceFilter::Stopped,
                    ServiceFilter::Stopped => ServiceFilter::Enabled,
                    ServiceFilter::Enabled => ServiceFilter::All,
                };
                self.list_state.select(Some(0));
            }
            KeyCode::F(5) => {
                self.load_services();
                self.status_message = Some(("Services refreshed".to_string(), false));
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
            ServiceFilter::All => "[All]  Running  Stopped  Enabled",
            ServiceFilter::Running => " All  [Running]  Stopped  Enabled",
            ServiceFilter::Stopped => " All   Running  [Stopped]  Enabled",
            ServiceFilter::Enabled => " All   Running   Stopped  [Enabled]",
        };
        let filter_bar = Paragraph::new(Line::from(vec![
            Span::styled("Filter: ", Style::default().fg(Color::Cyan)),
            Span::raw(filter_text),
            Span::styled(
                format!("  ({} services)", self.filtered_services().len()),
                Style::default().fg(Color::DarkGray),
            ),
        ]))
        .block(Block::default().borders(Borders::ALL).title(" Services "));
        frame.render_widget(filter_bar, chunks[0]);

        // Service list
        let filtered = self.filtered_services();
        let items: Vec<ListItem> = filtered
            .iter()
            .map(|service| {
                let (status, color) = service.status_display();
                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            format!("{:<20}", service.name),
                            Style::default().add_modifier(Modifier::BOLD),
                        ),
                        Span::styled(format!(" {:<20}", status), Style::default().fg(color)),
                    ]),
                    Line::from(vec![Span::styled(
                        format!("  {}", service.description),
                        Style::default().fg(Color::DarkGray),
                    )]),
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
            let action_desc = match &self.pending_action {
                Some(ServiceAction::Start(n)) => format!("Start {}?", n),
                Some(ServiceAction::Stop(n)) => format!("Stop {}?", n),
                Some(ServiceAction::Restart(n)) => format!("Restart {}?", n),
                Some(ServiceAction::Toggle(n)) => format!("Toggle {}?", n),
                None => "Confirm action?".to_string(),
            };
            Line::from(vec![
                Span::styled(action_desc, Style::default().fg(Color::Yellow)),
                Span::raw(" [Y]es / [N]o"),
            ])
        } else if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                Style::default().fg(if *is_error { Color::Red } else { Color::Green }),
            ))
        } else if let Some(service) = self.selected_service() {
            Line::from(vec![
                Span::styled("Path: ", Style::default().fg(Color::Cyan)),
                Span::raw(&service.path),
            ])
        } else {
            Line::from(Span::raw("Select a service"))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
        frame.render_widget(status, chunks[2]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("s", "Start"),
            ("x", "Stop"),
            ("r", "Restart"),
            ("e", "Enable/Disable"),
            ("Tab", "Filter"),
        ]
    }

    fn on_activate(&mut self) {
        self.load_services();
    }
}
