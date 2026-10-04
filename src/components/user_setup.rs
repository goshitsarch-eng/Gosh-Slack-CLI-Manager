use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::Modifier,
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use super::Component;
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
            KeyCode::Enter => match self.validate() {
                Ok(()) => {
                    self.start_create();
                    Some(Message::CreateUser)
                }
                Err(e) => {
                    self.error_message = Some(e);
                    None
                }
            },
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
                Constraint::Length(if area.height < 20 { 0 } else { 4 }), // Title
                Constraint::Min(0), // Form; reserve space for validation feedback
                Constraint::Length(3), // Status/Error
            ])
            .split(area);

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
            .split(chunks[0]);

        let title = Paragraph::new(vec![
            Line::from(Span::styled("User Setup", Theme::title())),
            Line::from(Span::styled(
                "Create accounts and assign groups.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel(Theme::panel_title("Identity")));
        frame.render_widget(title, header[0]);

        let selected_groups = self.groups.iter().filter(|(_, selected)| *selected).count();
        let runtime = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Selected groups ", Theme::label()),
                Span::styled(selected_groups.to_string(), Theme::badge_neutral()),
            ]),
            Line::from(vec![
                Span::styled("Runlevel ", Theme::label()),
                if self.change_runlevel {
                    Span::styled(" GUI LOGIN ", Theme::badge_success())
                } else {
                    Span::styled(" TEXT LOGIN ", Theme::badge_neutral())
                },
            ]),
        ])
        .block(Theme::panel_alt(Theme::panel_title("Runtime")));
        frame.render_widget(runtime, header[1]);

        let form_chunks = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([
                Constraint::Percentage(38),
                Constraint::Percentage(36),
                Constraint::Percentage(26),
            ])
            .split(chunks[1]);

        // Left side - text inputs
        let input_block = Theme::panel(Theme::panel_title("User info"));
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
        let password_display = "*".repeat(self.password.chars().count());
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
        let confirm_display = "*".repeat(self.confirm_password.chars().count());
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

        if input_inner.height < 9 {
            frame.render_widget(ratatui::widgets::Clear, input_inner);
            let values = [
                format!("User: {}", self.username),
                format!("Password: {}", "*".repeat(self.password.chars().count())),
                format!(
                    "Confirm: {}",
                    "*".repeat(self.confirm_password.chars().count())
                ),
            ];
            let lines: Vec<Line> = values
                .into_iter()
                .enumerate()
                .map(|(index, value)| {
                    Line::styled(
                        value,
                        if self.current_field == index {
                            Theme::input_active()
                        } else {
                            Theme::input_inactive()
                        },
                    )
                })
                .collect();
            frame.render_widget(Paragraph::new(lines), input_inner);
        }

        // Middle - groups checkboxes
        let groups_block = Theme::panel(Theme::panel_title("Groups"));
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

        let selected_line = self.current_field.saturating_sub(3)
            + usize::from(self.current_field == 3 + self.groups.len());
        let group_scroll = (selected_line + 1).saturating_sub(groups_inner.height as usize);
        let groups_para = Paragraph::new(lines).scroll((group_scroll as u16, 0));
        frame.render_widget(groups_para, groups_inner);

        let current_group = if self.current_field >= 3 && self.current_field < 3 + self.groups.len()
        {
            self.groups
                .get(self.current_field - 3)
                .map(|(name, selected)| (name.as_str(), *selected))
        } else {
            None
        };
        let inspector_lines = vec![
            Line::from(vec![
                Span::styled("PROFILE", Theme::badge_info()),
                Span::raw(" "),
                Span::styled(
                    if self.username.is_empty() {
                        "<new user>"
                    } else {
                        &self.username
                    },
                    Theme::title(),
                ),
            ]),
            Line::from(""),
            Line::from(vec![
                Span::styled("Password ", Theme::label()),
                Span::styled(
                    if self.password.is_empty() {
                        " EMPTY "
                    } else if self.password == self.confirm_password {
                        " MATCH "
                    } else {
                        " MISMATCH "
                    },
                    if self.password.is_empty() {
                        Theme::badge_warning()
                    } else if self.password == self.confirm_password {
                        Theme::badge_success()
                    } else {
                        Theme::badge_warning()
                    },
                ),
            ]),
            Line::from(vec![
                Span::styled("Current field ", Theme::label()),
                Span::raw(match self.current_field {
                    0 => "username",
                    1 => "password",
                    2 => "confirm password",
                    idx if idx == 3 + self.groups.len() => "runlevel",
                    _ => "group selection",
                }),
            ]),
            Line::from(""),
            Line::from(Span::styled("Focus", Theme::eyebrow())),
            if let Some((name, selected)) = current_group {
                Line::from(vec![
                    Span::styled(name, Theme::title()),
                    Span::raw(" "),
                    if selected {
                        Span::styled(" INCLUDED ", Theme::badge_success())
                    } else {
                        Span::styled(" EXCLUDED ", Theme::badge_neutral())
                    },
                ])
            } else {
                Line::from(Span::styled(
                    "Move through fields and groups with Tab/Shift+Tab.",
                    Theme::subtitle(),
                ))
            },
            Line::from(""),
            Line::from(Span::styled(
                "Press Enter only after validation passes and the account plan looks right.",
                Theme::subtitle(),
            )),
        ];
        frame.render_widget(
            Paragraph::new(inspector_lines)
                .wrap(ratatui::widgets::Wrap { trim: true })
                .block(Theme::panel_alt(Theme::panel_title("Inspector"))),
            form_chunks[2],
        );

        // Status/Error message
        let status = if let Some(ref err) = self.error_message {
            Paragraph::new(err.as_str())
                .style(Theme::error())
                .block(Theme::panel_alt(Theme::panel_title("Status")))
        } else if let Some(ref msg) = self.success_message {
            Paragraph::new(msg.as_str())
                .style(Theme::success())
                .block(Theme::panel_alt(Theme::panel_title("Status")))
        } else if self.is_running {
            Paragraph::new("Creating user...")
                .style(Theme::warning())
                .block(Theme::panel_alt(Theme::panel_title("Status")))
        } else {
            Paragraph::new("Press Enter to create user, Ctrl+R to reset")
                .style(Theme::muted())
                .block(Theme::panel_alt(Theme::panel_title("Status")))
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
