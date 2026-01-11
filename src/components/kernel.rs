use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
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
    RemoveKernel(String),
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

                    let is_current = self.current_kernel.contains(&version.replace("-generic", "").replace("-huge", ""));
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
                                if label.trim() == default_label && current_image.contains(kernel_name) {
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

    fn set_default_kernel(&mut self, version: &str) -> Option<Message> {
        match self.bootloader {
            BootloaderType::Lilo => {
                // Modify lilo.conf to set default
                if let Ok(content) = fs::read_to_string("/etc/lilo.conf") {
                    let mut new_content = String::new();
                    let mut found_label = String::new();

                    // First pass: find the label for this kernel
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
                        self.status_message = Some(("Kernel not found in lilo.conf".to_string(), true));
                        return None;
                    }

                    // Second pass: update default
                    let mut default_set = false;
                    for line in content.lines() {
                        if line.trim().starts_with("default") {
                            new_content.push_str(&format!("default = {}\n", found_label));
                            default_set = true;
                        } else {
                            new_content.push_str(line);
                            new_content.push('\n');
                        }
                    }

                    if !default_set {
                        // Add default if not present
                        new_content = format!("default = {}\n{}", found_label, new_content);
                    }

                    if let Err(e) = fs::write("/etc/lilo.conf", new_content) {
                        self.status_message = Some((format!("Failed to update lilo.conf: {}", e), true));
                        return None;
                    }

                    self.status_message = Some((
                        format!("Default set to {}. Run lilo to apply!", found_label),
                        false,
                    ));
                }
            }
            BootloaderType::Grub => {
                self.status_message = Some(("GRUB configuration editing not yet supported".to_string(), true));
            }
            BootloaderType::Unknown => {
                self.status_message = Some(("No known bootloader detected".to_string(), true));
            }
        }

        self.load_kernel_info();
        None
    }

    fn run_lilo(&mut self) -> Option<Message> {
        match std::process::Command::new("lilo").output() {
            Ok(output) => {
                if output.status.success() {
                    self.status_message = Some(("LILO updated successfully".to_string(), false));
                } else {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    self.status_message = Some((format!("LILO failed: {}", stderr), true));
                }
            }
            Err(e) => {
                self.status_message = Some((format!("Failed to run lilo: {}", e), true));
            }
        }
        None
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
                        return match action {
                            KernelAction::SetDefault(version) => self.set_default_kernel(&version),
                            KernelAction::RemoveKernel(_) => {
                                self.status_message = Some(("Kernel removal not implemented for safety".to_string(), true));
                                None
                            }
                            KernelAction::RunLilo => self.run_lilo(),
                        };
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

        let info = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Running Kernel: ", Style::default().fg(Color::Cyan)),
                Span::styled(
                    &self.current_kernel,
                    Style::default()
                        .fg(Color::Green)
                        .add_modifier(Modifier::BOLD),
                ),
            ]),
            Line::from(vec![
                Span::styled("Bootloader:     ", Style::default().fg(Color::Cyan)),
                Span::raw(bootloader_str),
            ]),
        ])
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title(" Kernel Manager "),
        );
        frame.render_widget(info, chunks[0]);

        // Kernel list
        let items: Vec<ListItem> = self
            .kernels
            .iter()
            .map(|kernel| {
                let mut status_parts = Vec::new();

                if kernel.is_current {
                    status_parts.push(Span::styled(
                        " [RUNNING]",
                        Style::default().fg(Color::Green),
                    ));
                }
                if kernel.is_default {
                    status_parts.push(Span::styled(
                        " [DEFAULT]",
                        Style::default().fg(Color::Yellow),
                    ));
                }

                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            format!("{:<40}", kernel.version),
                            Style::default().add_modifier(Modifier::BOLD),
                        ),
                        Span::styled(
                            format!("{:<10}", kernel.variant),
                            Style::default().fg(Color::Cyan),
                        ),
                    ]
                    .into_iter()
                    .chain(status_parts)
                    .collect::<Vec<_>>()),
                    Line::from(vec![
                        Span::styled("    Path: ", Style::default().fg(Color::DarkGray)),
                        Span::raw(&kernel.path),
                        Span::styled("  Size: ", Style::default().fg(Color::DarkGray)),
                        Span::raw(Self::format_size(kernel.size)),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(format!(" Installed Kernels ({}) ", self.kernels.len())),
            )
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, chunks[1], &mut state);

        // Status bar
        let status_content = if self.show_confirm {
            let action_desc = match &self.pending_action {
                Some(KernelAction::SetDefault(v)) => format!("Set {} as default?", v),
                Some(KernelAction::RemoveKernel(v)) => format!("Remove kernel {}?", v),
                Some(KernelAction::RunLilo) => "Run lilo to update bootloader?".to_string(),
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
        } else {
            Line::from(Span::styled(
                "Press 'd' to set default, 'l' to run lilo",
                Style::default().fg(Color::DarkGray),
            ))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
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
