use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::Modifier,
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};
use tokio::sync::mpsc;

use super::{AsyncComponent, Component};
use crate::app::Message;
use crate::ui::theme::Theme;

/// Default groups for new users
const DEFAULT_GROUPS: [(&str, &str); 10] = [
    ("wheel", "Administrative access (sudo)"),
    ("floppy", "Floppy disk access"),
    ("audio", "Audio devices"),
    ("video", "Video devices"),
    ("cdrom", "CD/DVD drives"),
    ("plugdev", "Pluggable devices"),
    ("power", "Power management"),
    ("netdev", "Network devices"),
    ("lp", "Printer access"),
    ("scanner", "Scanner access"),
];

/// User setup component
pub struct UserSetupComponent {
    username: String,
    password: String,
    confirm_password: String,
    groups: Vec<(String, bool)>,
    change_runlevel: bool,
    current_field: usize,
    is_running: bool,
    error_message: Option<String>,
    success_message: Option<String>,
    progress_tx: Option<mpsc::UnboundedSender<String>>,
}

impl UserSetupComponent {
    pub fn new() -> Self {
        let groups = DEFAULT_GROUPS
            .iter()
            .map(|(name, _)| (name.to_string(), true))
            .collect();

        Self {
            username: String::new(),
            password: String::new(),
            confirm_password: String::new(),
            groups,
            change_runlevel: true,
            current_field: 0,
            is_running: false,
            error_message: None,
            success_message: None,
            progress_tx: None,
        }
    }

    pub fn reset(&mut self) {
        self.username.clear();
        self.password.clear();
        self.confirm_password.clear();
        self.groups = DEFAULT_GROUPS
            .iter()
            .map(|(name, _)| (name.to_string(), true))
            .collect();
        self.change_runlevel = true;
        self.current_field = 0;
        self.is_running = false;
        self.error_message = None;
        self.success_message = None;
    }

    pub fn validate(&self) -> Result<(), String> {
        if self.username.is_empty() {
            return Err("Username cannot be empty".to_string());
        }

        if self.username.contains(' ') {
            return Err("Username cannot contain spaces".to_string());
        }

        if self.password.is_empty() {
            return Err("Password cannot be empty".to_string());
        }

        if self.password != self.confirm_password {
            return Err("Passwords do not match".to_string());
        }

        if self.password.len() < 4 {
            return Err("Password must be at least 4 characters".to_string());
        }

        Ok(())
    }

    pub fn get_selected_groups(&self) -> Vec<String> {
        self.groups
            .iter()
            .filter(|(_, selected)| *selected)
            .map(|(name, _)| name.clone())
            .collect()
    }

    pub fn get_username(&self) -> &str {
        &self.username
    }

    pub fn get_password(&self) -> &str {
        &self.password
    }

    pub fn should_change_runlevel(&self) -> bool {
        self.change_runlevel
    }

    pub fn set_error(&mut self, error: String) {
        self.error_message = Some(error);
        self.is_running = false;
    }

    pub fn set_success(&mut self, message: String) {
        self.success_message = Some(message);
        self.is_running = false;
    }

    pub fn start_create(&mut self) {
        self.error_message = None;
        self.success_message = None;
        self.is_running = true;
    }

    fn total_fields(&self) -> usize {
        3 + self.groups.len() + 1 // username, password, confirm, groups, runlevel
    }
}

impl Default for UserSetupComponent {
    fn default() -> Self {
        Self::new()
    }
}

impl Component for UserSetupComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.is_running {
            return None;
        }

        match key.code {
            KeyCode::Tab | KeyCode::Down => {
                self.current_field = (self.current_field + 1) % self.total_fields();
                None
            }
            KeyCode::BackTab | KeyCode::Up => {
                if self.current_field == 0 {
                    self.current_field = self.total_fields() - 1;
                } else {
                    self.current_field -= 1;
                }
                None
            }
            KeyCode::Char(' ') if self.current_field >= 3 => {
                let idx = self.current_field - 3;
                if idx < self.groups.len() {
                    self.groups[idx].1 = !self.groups[idx].1;
                } else if idx == self.groups.len() {
                    self.change_runlevel = !self.change_runlevel;
                }
                None
            }
            KeyCode::Char(c) if !key.modifiers.contains(KeyModifiers::CONTROL) => {
                match self.current_field {
                    0 => self.username.push(c),
                    1 => self.password.push(c),
                    2 => self.confirm_password.push(c),
                    _ => {}
                }
                None
            }
            KeyCode::Backspace => {
                match self.current_field {
                    0 => {
                        self.username.pop();
                    }
                    1 => {
                        self.password.pop();
                    }
                    2 => {
                        self.confirm_password.pop();
                    }
                    _ => {}
                }
                None
            }
            KeyCode::Enter => {
                match self.validate() {
                    Ok(()) => {
                        self.start_create();
                        Some(Message::CreateUser)
                    }
                    Err(e) => {
                        self.error_message = Some(e);
                        None
                    }
                }
            }
            KeyCode::Char('r') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                self.reset();
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
                Constraint::Min(10),   // Form
                Constraint::Length(3), // Status/Error
            ])
            .split(area);

        // Title
        let title = Paragraph::new(Line::from(vec![Span::styled(
            "User Setup",
            Theme::title(),
        )]))
        .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(title, chunks[0]);

        // Form
        let form_chunks = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
            .split(chunks[1]);

        // Left side - text inputs
        let input_block = Block::default().borders(Borders::ALL).title("User Info");
        let input_inner = input_block.inner(form_chunks[0]);
        frame.render_widget(input_block, form_chunks[0]);

        let input_chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(3),
                Constraint::Length(3),
                Constraint::Length(3),
            ])
            .split(input_inner);

        // Username field
        let username_style = if self.current_field == 0 {
            Theme::input_active()
        } else {
            Theme::input_inactive()
        };
        let username_block = Block::default()
            .borders(Borders::ALL)
            .title("Username")
            .border_style(if self.current_field == 0 {
                Theme::border_focused()
            } else {
                Theme::border()
            });
        let username = Paragraph::new(&*self.username)
            .style(username_style)
            .block(username_block);
        frame.render_widget(username, input_chunks[0]);

        // Password field
        let password_style = if self.current_field == 1 {
            Theme::input_active()
        } else {
            Theme::input_inactive()
        };
        let password_display = "*".repeat(self.password.len());
        let password_block = Block::default()
            .borders(Borders::ALL)
            .title("Password")
            .border_style(if self.current_field == 1 {
                Theme::border_focused()
            } else {
                Theme::border()
            });
        let password = Paragraph::new(password_display)
            .style(password_style)
            .block(password_block);
        frame.render_widget(password, input_chunks[1]);

        // Confirm password field
        let confirm_style = if self.current_field == 2 {
            Theme::input_active()
        } else {
            Theme::input_inactive()
        };
        let confirm_display = "*".repeat(self.confirm_password.len());
        let confirm_block = Block::default()
            .borders(Borders::ALL)
            .title("Confirm Password")
            .border_style(if self.current_field == 2 {
                Theme::border_focused()
            } else {
                Theme::border()
            });
        let confirm = Paragraph::new(confirm_display)
            .style(confirm_style)
            .block(confirm_block);
        frame.render_widget(confirm, input_chunks[2]);

        // Right side - groups checkboxes
        let groups_block = Block::default().borders(Borders::ALL).title("Groups");
        let groups_inner = groups_block.inner(form_chunks[1]);
        frame.render_widget(groups_block, form_chunks[1]);

        let mut lines = Vec::new();
        for (i, (name, selected)) in self.groups.iter().enumerate() {
            let checkbox = if *selected { "[x]" } else { "[ ]" };
            let desc = DEFAULT_GROUPS
                .iter()
                .find(|(n, _)| *n == name)
                .map(|(_, d)| *d)
                .unwrap_or("");

            let field_idx = 3 + i;
            let style = if self.current_field == field_idx {
                Theme::highlight()
            } else {
                Theme::default()
            };

            lines.push(Line::from(vec![
                Span::styled(format!("{} ", checkbox), style.add_modifier(Modifier::BOLD)),
                Span::styled(format!("{:<10}", name), style),
                Span::styled(desc, Theme::muted()),
            ]));
        }

        // Runlevel option
        let runlevel_checkbox = if self.change_runlevel { "[x]" } else { "[ ]" };
        let runlevel_style = if self.current_field == 3 + self.groups.len() {
            Theme::highlight()
        } else {
            Theme::default()
        };
        lines.push(Line::from(""));
        lines.push(Line::from(vec![
            Span::styled(
                format!("{} ", runlevel_checkbox),
                runlevel_style.add_modifier(Modifier::BOLD),
            ),
            Span::styled("Change runlevel 3→4 (GUI)", runlevel_style),
        ]));

        let groups_para = Paragraph::new(lines);
        frame.render_widget(groups_para, groups_inner);

        // Status/Error message
        let status = if let Some(ref err) = self.error_message {
            Paragraph::new(err.as_str())
                .style(Theme::error())
                .block(Block::default().borders(Borders::TOP))
        } else if let Some(ref msg) = self.success_message {
            Paragraph::new(msg.as_str())
                .style(Theme::success())
                .block(Block::default().borders(Borders::TOP))
        } else if self.is_running {
            Paragraph::new("Creating user...")
                .style(Theme::warning())
                .block(Block::default().borders(Borders::TOP))
        } else {
            Paragraph::new("Press Enter to create user, Ctrl+R to reset")
                .style(Theme::muted())
                .block(Block::default().borders(Borders::TOP))
        };
        frame.render_widget(status, chunks[2]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("Tab", "Next field"),
            ("Space", "Toggle"),
            ("Enter", "Create"),
            ("Ctrl+R", "Reset"),
        ]
    }
}

impl AsyncComponent for UserSetupComponent {
    fn set_progress_channel(&mut self, tx: mpsc::UnboundedSender<String>) {
        self.progress_tx = Some(tx);
    }

    fn is_running(&self) -> bool {
        self.is_running
    }
}
