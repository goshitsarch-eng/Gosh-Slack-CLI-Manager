use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Gauge, List, ListItem, ListState, Paragraph},
    Frame,
};
use std::fs;
use std::process::Command;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// Disk/partition information
#[derive(Debug, Clone)]
pub struct DiskInfo {
    pub name: String,
    pub mount_point: Option<String>,
    pub filesystem: String,
    pub size: u64,
    pub used: u64,
    pub available: u64,
    pub use_percent: u8,
    pub is_mounted: bool,
    pub device_path: String,
}

/// Disk Management Component
pub struct DiskComponent {
    disks: Vec<DiskInfo>,
    list_state: ListState,
    mode: DiskMode,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
    pending_action: Option<DiskAction>,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum DiskMode {
    Overview,
    Details,
}

#[derive(Debug, Clone)]
pub enum DiskAction {
    Mount(String),
    Unmount(String),
    CheckFilesystem(String),
}

impl DiskComponent {
    pub fn new() -> Self {
        let mut component = Self {
            disks: Vec::new(),
            list_state: ListState::default(),
            mode: DiskMode::Overview,
            status_message: None,
            show_confirm: false,
            pending_action: None,
        };
        component.load_disk_info();
        if !component.disks.is_empty() {
            component.list_state.select(Some(0));
        }
        component
    }

    fn load_disk_info(&mut self) {
        self.disks.clear();

        // Use df to get mounted filesystems
        if let Ok(output) = Command::new("df")
            .args(["-B1", "--output=source,target,fstype,size,used,avail,pcent"])
            .output()
        {
            let stdout = String::from_utf8_lossy(&output.stdout);
            for line in stdout.lines().skip(1) {
                // Skip header
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 7 {
                    // Skip pseudo filesystems
                    let source = parts[0];
                    if !source.starts_with("/dev/") {
                        continue;
                    }

                    let name = source
                        .trim_start_matches("/dev/")
                        .to_string();

                    let mount_point = parts[1].to_string();
                    let filesystem = parts[2].to_string();

                    let size: u64 = parts[3].parse().unwrap_or(0);
                    let used: u64 = parts[4].parse().unwrap_or(0);
                    let available: u64 = parts[5].parse().unwrap_or(0);
                    let use_percent: u8 = parts[6]
                        .trim_end_matches('%')
                        .parse()
                        .unwrap_or(0);

                    self.disks.push(DiskInfo {
                        name: name.clone(),
                        mount_point: Some(mount_point),
                        filesystem,
                        size,
                        used,
                        available,
                        use_percent,
                        is_mounted: true,
                        device_path: source.to_string(),
                    });
                }
            }
        }

        // Also scan for unmounted block devices
        self.scan_block_devices();
    }

    fn scan_block_devices(&mut self) {
        // Use lsblk for additional info
        if let Ok(output) = Command::new("lsblk")
            .args(["-b", "-n", "-o", "NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT"])
            .output()
        {
            let stdout = String::from_utf8_lossy(&output.stdout);
            for line in stdout.lines() {
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 3 {
                    let name = parts[0].trim_start_matches("├─").trim_start_matches("└─");
                    let size: u64 = parts[1].parse().unwrap_or(0);
                    let device_type = parts[2];

                    // Only interested in partitions and disks
                    if device_type != "part" && device_type != "disk" {
                        continue;
                    }

                    let filesystem = if parts.len() > 3 { parts[3] } else { "" };
                    let mount_point = if parts.len() > 4 {
                        Some(parts[4].to_string())
                    } else {
                        None
                    };

                    // Check if already in list
                    let exists = self.disks.iter().any(|d| d.name == name);
                    if !exists && !name.starts_with("loop") {
                        self.disks.push(DiskInfo {
                            name: name.to_string(),
                            mount_point,
                            filesystem: filesystem.to_string(),
                            size,
                            used: 0,
                            available: size,
                            use_percent: 0,
                            is_mounted: false,
                            device_path: format!("/dev/{}", name),
                        });
                    }
                }
            }
        }

        // Sort by name
        self.disks.sort_by(|a, b| a.name.cmp(&b.name));
    }

    fn selected_disk(&self) -> Option<&DiskInfo> {
        self.list_state.selected().and_then(|i| self.disks.get(i))
    }

    fn mount_disk(&mut self, device: &str) -> Option<Message> {
        // Find mount point from fstab or create one
        let mount_point = self.find_mount_point(device);

        match Command::new("mount").arg(device).arg(&mount_point).output() {
            Ok(output) => {
                if output.status.success() {
                    self.status_message = Some((
                        format!("Mounted {} at {}", device, mount_point),
                        false,
                    ));
                    self.load_disk_info();
                } else {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    self.status_message = Some((format!("Mount failed: {}", stderr), true));
                }
            }
            Err(e) => {
                self.status_message = Some((format!("Error: {}", e), true));
            }
        }
        None
    }

    fn unmount_disk(&mut self, mount_point: &str) -> Option<Message> {
        match Command::new("umount").arg(mount_point).output() {
            Ok(output) => {
                if output.status.success() {
                    self.status_message = Some((format!("Unmounted {}", mount_point), false));
                    self.load_disk_info();
                } else {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    self.status_message = Some((format!("Unmount failed: {}", stderr), true));
                }
            }
            Err(e) => {
                self.status_message = Some((format!("Error: {}", e), true));
            }
        }
        None
    }

    fn check_filesystem(&mut self, device: &str) -> Option<Message> {
        // Note: filesystem check usually requires unmounted partition
        self.status_message = Some((
            "Filesystem check requires unmounted partition. Use 'fsck' manually.".to_string(),
            true,
        ));
        None
    }

    fn find_mount_point(&self, device: &str) -> String {
        // Check fstab for configured mount point
        if let Ok(content) = fs::read_to_string("/etc/fstab") {
            for line in content.lines() {
                let line = line.trim();
                if line.starts_with('#') || line.is_empty() {
                    continue;
                }
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 2 && parts[0] == device {
                    return parts[1].to_string();
                }
            }
        }

        // Create mount point based on device name
        let name = device.trim_start_matches("/dev/");
        format!("/mnt/{}", name)
    }

    fn format_size(bytes: u64) -> String {
        const KB: u64 = 1024;
        const MB: u64 = KB * 1024;
        const GB: u64 = MB * 1024;
        const TB: u64 = GB * 1024;

        if bytes >= TB {
            format!("{:.1} TB", bytes as f64 / TB as f64)
        } else if bytes >= GB {
            format!("{:.1} GB", bytes as f64 / GB as f64)
        } else if bytes >= MB {
            format!("{:.1} MB", bytes as f64 / MB as f64)
        } else if bytes >= KB {
            format!("{:.1} KB", bytes as f64 / KB as f64)
        } else {
            format!("{} B", bytes)
        }
    }

    fn usage_color(percent: u8) -> Color {
        if percent >= 90 {
            Color::Red
        } else if percent >= 75 {
            Color::Yellow
        } else {
            Color::Green
        }
    }
}

impl Component for DiskComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    if let Some(action) = self.pending_action.take() {
                        return match action {
                            DiskAction::Mount(dev) => self.mount_disk(&dev),
                            DiskAction::Unmount(mp) => self.unmount_disk(&mp),
                            DiskAction::CheckFilesystem(dev) => self.check_filesystem(&dev),
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
                    if selected < self.disks.len().saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                }
            }
            KeyCode::Char('m') => {
                if let Some(disk) = self.selected_disk() {
                    if !disk.is_mounted {
                        self.pending_action = Some(DiskAction::Mount(disk.device_path.clone()));
                        self.show_confirm = true;
                    }
                }
            }
            KeyCode::Char('u') => {
                if let Some(disk) = self.selected_disk() {
                    if disk.is_mounted {
                        if let Some(mp) = &disk.mount_point {
                            self.pending_action = Some(DiskAction::Unmount(mp.clone()));
                            self.show_confirm = true;
                        }
                    }
                }
            }
            KeyCode::Enter => {
                self.mode = match self.mode {
                    DiskMode::Overview => DiskMode::Details,
                    DiskMode::Details => DiskMode::Overview,
                };
            }
            KeyCode::F(5) => {
                self.load_disk_info();
                self.status_message = Some(("Disk info refreshed".to_string(), false));
            }
            _ => {}
        }
        None
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(6),
                Constraint::Min(10),
                Constraint::Length(3),
            ])
            .split(area);

        // Summary
        self.render_summary(frame, chunks[0]);

        // Disk list or details
        match self.mode {
            DiskMode::Overview => self.render_list(frame, chunks[1]),
            DiskMode::Details => {
                if let Some(disk) = self.selected_disk() {
                    self.render_details(frame, chunks[1], disk);
                } else {
                    self.render_list(frame, chunks[1]);
                }
            }
        }

        // Status bar
        let status_content = if self.show_confirm {
            let action_desc = match &self.pending_action {
                Some(DiskAction::Mount(d)) => format!("Mount {}?", d),
                Some(DiskAction::Unmount(m)) => format!("Unmount {}?", m),
                Some(DiskAction::CheckFilesystem(d)) => format!("Check {}?", d),
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
        } else if let Some(disk) = self.selected_disk() {
            Line::from(vec![
                Span::styled("Device: ", Style::default().fg(Color::Cyan)),
                Span::raw(&disk.device_path),
            ])
        } else {
            Line::from(Span::raw("Select a disk"))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
        frame.render_widget(status, chunks[2]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("m", "Mount"),
            ("u", "Unmount"),
            ("Enter", "Details"),
            ("F5", "Refresh"),
        ]
    }

    fn on_activate(&mut self) {
        self.load_disk_info();
    }
}

impl DiskComponent {
    fn render_summary(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .borders(Borders::ALL)
            .title(" Disk Management ");

        let inner = block.inner(area);
        frame.render_widget(block, area);

        // Calculate totals
        let total_size: u64 = self.disks.iter().filter(|d| d.is_mounted).map(|d| d.size).sum();
        let total_used: u64 = self.disks.iter().filter(|d| d.is_mounted).map(|d| d.used).sum();
        let mounted_count = self.disks.iter().filter(|d| d.is_mounted).count();
        let unmounted_count = self.disks.iter().filter(|d| !d.is_mounted).count();

        let overall_percent = if total_size > 0 {
            ((total_used as f64 / total_size as f64) * 100.0) as u16
        } else {
            0
        };

        let chunks = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
            .split(inner);

        // Stats
        let stats = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Mounted:   ", Style::default().fg(Color::Cyan)),
                Span::raw(format!("{} partitions", mounted_count)),
            ]),
            Line::from(vec![
                Span::styled("Unmounted: ", Style::default().fg(Color::Cyan)),
                Span::raw(format!("{} partitions", unmounted_count)),
            ]),
        ]);
        frame.render_widget(stats, chunks[0]);

        // Overall usage gauge
        let gauge = Gauge::default()
            .block(Block::default().title("Total Usage"))
            .gauge_style(Style::default().fg(Self::usage_color(overall_percent as u8)))
            .percent(overall_percent)
            .label(format!(
                "{} / {} ({:.1}%)",
                Self::format_size(total_used),
                Self::format_size(total_size),
                overall_percent
            ));
        frame.render_widget(gauge, chunks[1]);
    }

    fn render_list(&self, frame: &mut Frame, area: Rect) {
        let items: Vec<ListItem> = self
            .disks
            .iter()
            .map(|disk| {
                let mount_str = disk
                    .mount_point
                    .as_ref()
                    .map(|m| m.as_str())
                    .unwrap_or("-");

                let status = if disk.is_mounted {
                    Span::styled("●", Style::default().fg(Color::Green))
                } else {
                    Span::styled("○", Style::default().fg(Color::DarkGray))
                };

                ListItem::new(vec![
                    Line::from(vec![
                        status,
                        Span::raw(" "),
                        Span::styled(
                            format!("{:<12}", disk.name),
                            Style::default().add_modifier(Modifier::BOLD),
                        ),
                        Span::styled(
                            format!("{:<10}", disk.filesystem),
                            Style::default().fg(Color::Cyan),
                        ),
                        Span::styled(
                            format!("{:>10}", Self::format_size(disk.size)),
                            Style::default().fg(Color::Yellow),
                        ),
                        if disk.is_mounted {
                            Span::styled(
                                format!(" {:>3}%", disk.use_percent),
                                Style::default().fg(Self::usage_color(disk.use_percent)),
                            )
                        } else {
                            Span::raw("     ")
                        },
                    ]),
                    Line::from(vec![
                        Span::styled("  Mount: ", Style::default().fg(Color::DarkGray)),
                        Span::raw(mount_str),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(format!(" Partitions ({}) ", self.disks.len())),
            )
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, area, &mut state);
    }

    fn render_details(&self, frame: &mut Frame, area: Rect, disk: &DiskInfo) {
        let block = Block::default()
            .borders(Borders::ALL)
            .title(format!(" {} Details ", disk.name));

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Length(8), Constraint::Min(3)])
            .split(inner);

        // Info
        let info = vec![
            Line::from(vec![
                Span::styled("Device:     ", Style::default().fg(Color::Cyan)),
                Span::raw(&disk.device_path),
            ]),
            Line::from(vec![
                Span::styled("Filesystem: ", Style::default().fg(Color::Cyan)),
                Span::raw(&disk.filesystem),
            ]),
            Line::from(vec![
                Span::styled("Mount:      ", Style::default().fg(Color::Cyan)),
                Span::raw(
                    disk.mount_point
                        .as_ref()
                        .map(|s| s.as_str())
                        .unwrap_or("Not mounted"),
                ),
            ]),
            Line::from(vec![
                Span::styled("Size:       ", Style::default().fg(Color::Cyan)),
                Span::raw(Self::format_size(disk.size)),
            ]),
            Line::from(vec![
                Span::styled("Used:       ", Style::default().fg(Color::Cyan)),
                Span::raw(Self::format_size(disk.used)),
            ]),
            Line::from(vec![
                Span::styled("Available:  ", Style::default().fg(Color::Cyan)),
                Span::raw(Self::format_size(disk.available)),
            ]),
        ];

        let info_paragraph = Paragraph::new(info);
        frame.render_widget(info_paragraph, chunks[0]);

        // Usage gauge
        if disk.is_mounted {
            let gauge = Gauge::default()
                .block(Block::default().title("Usage"))
                .gauge_style(Style::default().fg(Self::usage_color(disk.use_percent)))
                .percent(disk.use_percent as u16)
                .label(format!("{}%", disk.use_percent));
            frame.render_widget(gauge, chunks[1]);
        }
    }
}
