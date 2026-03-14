use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    text::{Line, Span},
    widgets::{List, ListItem, ListState, Paragraph},
    Frame,
};
use std::fs;
use std::path::Path;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// Kernel information
#[derive(Debug, Clone)]
pub struct KernelInfo {
    pub version: String,
    pub variant: String, // generic, huge, etc.
    pub path: String,
    pub is_current: bool,
    pub is_default: bool,
    pub size: u64,
}

/// Kernel Manager Component
pub struct KernelComponent {
    kernels: Vec<KernelInfo>,
    list_state: ListState,
    current_kernel: String,
    bootloader: BootloaderType,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
    pending_action: Option<KernelAction>,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum BootloaderType {
    Lilo,
    Grub,
    Unknown,
}

#[derive(Debug, Clone)]
pub enum KernelAction {
    SetDefault(String),
    RunLilo,
}

impl KernelComponent {
    pub fn new() -> Self {
        let mut component = Self {
            kernels: Vec::new(),
            list_state: ListState::default(),
            current_kernel: String::new(),
            bootloader: BootloaderType::Unknown,
            status_message: None,
            show_confirm: false,
            pending_action: None,
        };
        component.load_kernel_info();
        if !component.kernels.is_empty() {
            component.list_state.select(Some(0));
        }
        component
    }

    fn load_kernel_info(&mut self) {
        self.kernels.clear();

        // Get current running kernel
        if let Ok(output) = std::process::Command::new("uname").arg("-r").output() {
            self.current_kernel = String::from_utf8_lossy(&output.stdout).trim().to_string();
        }

        // Detect bootloader
        self.bootloader = Self::detect_bootloader();

        // Scan for installed kernels
        self.scan_kernels();
    }

    fn detect_bootloader() -> BootloaderType {
        if Path::new("/etc/lilo.conf").exists() {
            BootloaderType::Lilo
        } else if Path::new("/boot/grub/grub.cfg").exists()
            || Path::new("/boot/grub2/grub.cfg").exists()
        {
            BootloaderType::Grub
        } else {
            BootloaderType::Unknown
        }
    }

    fn scan_kernels(&mut self) {
        let boot_path = Path::new("/boot");

        if let Ok(entries) = fs::read_dir(boot_path) {
            for entry in entries.filter_map(|e| e.ok()) {
                let name = entry.file_name().to_string_lossy().to_string();

                // Look for vmlinuz-* files
                if name.starts_with("vmlinuz-") {
                    let version = name.trim_start_matches("vmlinuz-").to_string();

                    // Determine variant
                    let variant = if version.contains("-generic") {
                        "generic"
                    } else if version.contains("-huge") {
                        "huge"
                    } else {
                        "custom"
                    }
                    .to_string();

                    let size = entry.metadata().map(|m| m.len()).unwrap_or(0);

                    let is_current = self
                        .current_kernel
                        .contains(&version.replace("-generic", "").replace("-huge", ""));
                    let is_default = self.is_default_kernel(&name);

                    self.kernels.push(KernelInfo {
                        version: version.clone(),
                        variant,
                        path: entry.path().to_string_lossy().to_string(),
                        is_current,
                        is_default,
                        size,
                    });
                }
            }
        }

        // Sort by version (newest first)
        self.kernels.sort_by(|a, b| b.version.cmp(&a.version));
    }

    fn is_default_kernel(&self, kernel_name: &str) -> bool {
        match self.bootloader {
            BootloaderType::Lilo => {
                if let Ok(content) = fs::read_to_string("/etc/lilo.conf") {
                    // Find the default entry and check if it matches this kernel
                    let mut default_label = String::new();
                    let mut current_image = String::new();

                    for line in content.lines() {
                        let line = line.trim();
                        if line.starts_with("default") {
                            if let Some(label) = line.split('=').nth(1) {
                                default_label = label.trim().to_string();
                            }
                        }
                        if line.starts_with("image") {
                            if let Some(path) = line.split('=').nth(1) {
                                current_image = path.trim().to_string();
                            }
                        }
                        if line.starts_with("label") {
                            if let Some(label) = line.split('=').nth(1) {
                                if label.trim() == default_label
                                    && current_image.contains(kernel_name)
                                {
                                    return true;
                                }
                            }
                        }
                    }
                }
                false
            }
            _ => false,
        }
    }

    fn selected_kernel(&self) -> Option<&KernelInfo> {
        self.list_state.selected().and_then(|i| self.kernels.get(i))
    }

    pub fn build_lilo_default_config(version: &str) -> Result<String, String> {
        let content = fs::read_to_string("/etc/lilo.conf").map_err(|err| err.to_string())?;
        let mut new_content = String::new();
        let mut found_label = String::new();

        let mut current_image = String::new();
        for line in content.lines() {
            let line_trimmed = line.trim();
            if line_trimmed.starts_with("image") {
                if let Some(path) = line_trimmed.split('=').nth(1) {
                    current_image = path.trim().to_string();
                }
            }
            if line_trimmed.starts_with("label") && current_image.contains(version) {
                if let Some(label) = line_trimmed.split('=').nth(1) {
                    found_label = label.trim().to_string();
                    break;
                }
            }
        }

        if found_label.is_empty() {
            return Err("Kernel not found in lilo.conf".to_string());
        }

        let mut default_set = false;
        for line in content.lines() {
            if line.trim().starts_with("default") {
                new_content.push_str(&format!("default = {found_label}\n"));
                default_set = true;
            } else {
                new_content.push_str(line);
                new_content.push('\n');
            }
        }

        if !default_set {
            new_content = format!("default = {found_label}\n{new_content}");
        }

        Ok(new_content)
    }

    pub fn refresh(&mut self) {
        self.load_kernel_info();
    }

    pub fn set_status(&mut self, message: String, is_error: bool) {
        self.status_message = Some((message, is_error));
    }

    fn render_selected_kernel(&self, frame: &mut Frame, area: Rect) {
        let lines = if let Some(kernel) = self.selected_kernel() {
            vec![
                Line::from(vec![
                    Span::styled("KERNEL", Theme::badge_info()),
                    Span::raw(" "),
                    Span::styled(&kernel.version, Theme::title()),
                ]),
                Line::from(vec![
                    Span::styled("Variant ", Theme::label()),
                    Span::styled(&kernel.variant, Theme::badge_neutral()),
                ]),
                Line::from(vec![
                    Span::styled("Size ", Theme::label()),
                    Span::raw(Self::format_size(kernel.size)),
                ]),
                Line::from(vec![
                    Span::styled("Path ", Theme::label()),
                    Span::raw(&kernel.path),
                ]),
                Line::from(""),
                Line::from(Span::styled("Flags", Theme::eyebrow())),
                Line::from(if kernel.is_current {
                    "RUNNING kernel image"
                } else {
                    "Not the running image"
                }),
                Line::from(if kernel.is_default {
                    "Configured default boot target"
                } else {
                    "Not the configured default"
                }),
                Line::from(""),
                Line::from(Span::styled("Actions", Theme::eyebrow())),
                Line::from("d/Enter  set default"),
                Line::from("l        run lilo"),
            ]
        } else {
            vec![
                Line::from(Span::styled("No kernel selected", Theme::muted())),
                Line::from(""),
                Line::from("Choose a kernel image to inspect its boot role."),
            ]
        };

        let panel = Paragraph::new(lines).block(Theme::panel_alt(Theme::panel_title("Inspector")));
        frame.render_widget(panel, area);
    }

    pub fn bootloader(&self) -> BootloaderType {
        self.bootloader
    }

    fn format_size(bytes: u64) -> String {
        const MB: u64 = 1024 * 1024;
        format!("{:.1} MB", bytes as f64 / MB as f64)
    }
}

impl Component for KernelComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    if let Some(action) = self.pending_action.take() {
                        return Some(Message::KernelAction(action));
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
                    if selected < self.kernels.len().saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                }
            }
            KeyCode::Enter | KeyCode::Char('d') => {
                if let Some(kernel) = self.selected_kernel() {
                    self.pending_action = Some(KernelAction::SetDefault(kernel.version.clone()));
                    self.show_confirm = true;
                }
            }
            KeyCode::Char('l') => {
                if self.bootloader == BootloaderType::Lilo {
                    self.pending_action = Some(KernelAction::RunLilo);
                    self.show_confirm = true;
                }
            }
            KeyCode::F(5) => {
                self.load_kernel_info();
                self.status_message = Some(("Kernel list refreshed".to_string(), false));
            }
            _ => {}
        }
        None
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(5),
                Constraint::Min(10),
                Constraint::Length(3),
            ])
            .split(area);

        // Info header
        let bootloader_str = match self.bootloader {
            BootloaderType::Lilo => "LILO",
            BootloaderType::Grub => "GRUB",
            BootloaderType::Unknown => "Unknown",
        };

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(56), Constraint::Percentage(44)])
            .split(chunks[0]);

        let info = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Running ", Theme::label()),
                Span::styled(&self.current_kernel, Theme::badge_success()),
            ]),
            Line::from(vec![
                Span::styled("Bootloader ", Theme::label()),
                Span::styled(bootloader_str, Theme::badge_neutral()),
            ]),
        ])
        .block(Theme::panel(Theme::panel_title("Kernel manager")));
        frame.render_widget(info, header[0]);

        let boot_path = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Installed ", Theme::label()),
                Span::styled(self.kernels.len().to_string(), Theme::badge_success()),
                Span::raw(" "),
                Span::styled("Bootloader ", Theme::label()),
                Span::styled(bootloader_str, Theme::badge_neutral()),
            ]),
            Line::from(Span::styled(
                "Default changes rewrite config first; run lilo afterward if LILO is in play.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel_alt(Theme::panel_title("Boot path")));
        frame.render_widget(boot_path, header[1]);

        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(64), Constraint::Percentage(36)])
            .split(chunks[1]);

        let items: Vec<ListItem> = self
            .kernels
            .iter()
            .map(|kernel| {
                let mut status_parts = Vec::new();

                if kernel.is_current {
                    status_parts.push(Span::styled(" RUNNING ", Theme::badge_success()));
                }
                if kernel.is_default {
                    status_parts.push(Span::styled(" DEFAULT ", Theme::badge_warning()));
                }

                ListItem::new(vec![
                    Line::from(
                        vec![
                            Span::styled(format!("{:<40}", kernel.version), Theme::title()),
                            Span::styled(format!("{:<10}", kernel.variant), Theme::accent()),
                        ]
                        .into_iter()
                        .chain(status_parts)
                        .collect::<Vec<_>>(),
                    ),
                    Line::from(vec![
                        Span::styled("    Path ", Theme::muted()),
                        Span::raw(&kernel.path),
                        Span::styled("  Size ", Theme::muted()),
                        Span::raw(Self::format_size(kernel.size)),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(Theme::panel_alt(Theme::panel_title(format!(
                "Installed kernels ({})",
                self.kernels.len()
            ))))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, content[0], &mut state);
        self.render_selected_kernel(frame, content[1]);

        // Status bar
        let status_content = if self.show_confirm {
            let action_desc = match &self.pending_action {
                Some(KernelAction::SetDefault(v)) => format!("Set {} as default?", v),
                Some(KernelAction::RunLilo) => "Run lilo to update bootloader?".to_string(),
                None => "Confirm action?".to_string(),
            };
            Line::from(vec![
                Span::styled(action_desc, Theme::warning()),
                Span::raw(" [Y]es / [N]o"),
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
        } else {
            Line::from(Span::styled(
                "Press 'd' to set default, 'l' to run lilo",
                Theme::muted(),
            ))
        };

        let status =
            Paragraph::new(status_content).block(Theme::panel_alt(Theme::panel_title("Status")));
        frame.render_widget(status, chunks[2]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("d/Enter", "Set Default"),
            ("l", "Run LILO"),
            ("F5", "Refresh"),
        ]
    }

    fn on_activate(&mut self) {
        self.load_kernel_info();
    }
}
