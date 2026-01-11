use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use std::fs;
use std::path::{Path, PathBuf};
use chrono::{DateTime, Local};

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

const BACKUP_DIR: &str = "/var/backups/slackware-cli-manager";

/// Predefined config files to backup
const CONFIG_FILES: &[(&str, &str)] = &[
    ("/etc/slackpkg/slackpkg.conf", "Slackpkg configuration"),
    ("/etc/slackpkg/mirrors", "Slackpkg mirrors"),
    ("/etc/sbotools/sbotools.conf", "sbotools configuration"),
    ("/etc/lilo.conf", "LILO bootloader configuration"),
    ("/etc/fstab", "Filesystem table"),
    ("/etc/rc.d/rc.local", "Local startup script"),
    ("/etc/rc.d/rc.inet1.conf", "Network configuration"),
    ("/etc/inittab", "Init configuration"),
    ("/etc/passwd", "User accounts"),
    ("/etc/group", "Group definitions"),
    ("/etc/shadow", "Password hashes"),
    ("/etc/sudoers", "Sudo configuration"),
    ("/etc/hosts", "Host mappings"),
    ("/etc/resolv.conf", "DNS configuration"),
];

/// Backup entry information
#[derive(Debug, Clone)]
pub struct BackupEntry {
    pub name: String,
    pub path: PathBuf,
    pub timestamp: DateTime<Local>,
    pub size: u64,
    pub file_count: usize,
}

/// Backup & Restore Component
pub struct BackupComponent {
    mode: BackupMode,
    config_files: Vec<(String, String, bool)>, // path, description, selected
    backups: Vec<BackupEntry>,
    list_state: ListState,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
    pending_action: Option<BackupAction>,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum BackupMode {
    Create,
    Restore,
}

#[derive(Debug, Clone)]
pub enum BackupAction {
    CreateBackup,
    RestoreBackup(PathBuf),
    DeleteBackup(PathBuf),
}

impl BackupComponent {
    pub fn new() -> Self {
        let config_files = CONFIG_FILES
            .iter()
            .map(|(path, desc)| (path.to_string(), desc.to_string(), true))
            .collect();

        let mut component = Self {
            mode: BackupMode::Create,
            config_files,
            backups: Vec::new(),
            list_state: ListState::default(),
            status_message: None,
            show_confirm: false,
            pending_action: None,
        };
        component.load_backups();
        component
    }

    fn ensure_backup_dir(&self) -> std::io::Result<()> {
        fs::create_dir_all(BACKUP_DIR)
    }

    fn load_backups(&mut self) {
        self.backups.clear();

        if let Err(_) = self.ensure_backup_dir() {
            return;
        }

        if let Ok(entries) = fs::read_dir(BACKUP_DIR) {
            for entry in entries.filter_map(|e| e.ok()) {
                let path = entry.path();
                if path.is_dir() {
                    if let Some(name) = path.file_name() {
                        let name = name.to_string_lossy().to_string();

                        // Parse timestamp from directory name (format: backup_YYYYMMDD_HHMMSS)
                        let timestamp = if name.starts_with("backup_") {
                            let ts_str = name.trim_start_matches("backup_");
                            chrono::NaiveDateTime::parse_from_str(ts_str, "%Y%m%d_%H%M%S")
                                .map(|dt| DateTime::from_naive_utc_and_offset(dt, *Local::now().offset()))
                                .unwrap_or_else(|_| Local::now())
                        } else {
                            Local::now()
                        };

                        // Count files and calculate size
                        let (file_count, size) = Self::calculate_backup_stats(&path);

                        self.backups.push(BackupEntry {
                            name,
                            path,
                            timestamp,
                            size,
                            file_count,
                        });
                    }
                }
            }
        }

        self.backups.sort_by(|a, b| b.timestamp.cmp(&a.timestamp));
    }

    fn calculate_backup_stats(path: &Path) -> (usize, u64) {
        let mut count = 0;
        let mut size = 0;

        if let Ok(entries) = fs::read_dir(path) {
            for entry in entries.filter_map(|e| e.ok()) {
                if let Ok(metadata) = entry.metadata() {
                    count += 1;
                    size += metadata.len();
                }
            }
        }

        (count, size)
    }

    fn create_backup(&mut self) -> Option<Message> {
        if let Err(e) = self.ensure_backup_dir() {
            self.status_message = Some((format!("Failed to create backup directory: {}", e), true));
            return None;
        }

        let timestamp = Local::now().format("%Y%m%d_%H%M%S");
        let backup_path = PathBuf::from(BACKUP_DIR).join(format!("backup_{}", timestamp));

        if let Err(e) = fs::create_dir_all(&backup_path) {
            self.status_message = Some((format!("Failed to create backup: {}", e), true));
            return None;
        }

        let mut backed_up = 0;
        let mut failed = 0;

        for (path, _, selected) in &self.config_files {
            if !*selected {
                continue;
            }

            let source = Path::new(path);
            if !source.exists() {
                continue;
            }

            // Create destination path preserving directory structure
            let dest_name = path.replace('/', "_").trim_start_matches('_').to_string();
            let dest = backup_path.join(&dest_name);

            match fs::copy(source, &dest) {
                Ok(_) => backed_up += 1,
                Err(_) => failed += 1,
            }
        }

        if backed_up > 0 {
            self.status_message = Some((
                format!("Backup created: {} files backed up, {} failed", backed_up, failed),
                failed > 0,
            ));
            self.load_backups();
        } else {
            self.status_message = Some(("No files were backed up".to_string(), true));
            // Remove empty backup directory
            let _ = fs::remove_dir(&backup_path);
        }

        None
    }

    fn restore_backup(&mut self, backup_path: &Path) -> Option<Message> {
        let mut restored = 0;
        let mut failed = 0;

        if let Ok(entries) = fs::read_dir(backup_path) {
            for entry in entries.filter_map(|e| e.ok()) {
                let filename = entry.file_name().to_string_lossy().to_string();

                // Convert filename back to path
                let original_path = format!("/{}", filename.replace('_', "/"));

                // Verify this is a known config file
                if CONFIG_FILES.iter().any(|(p, _)| *p == original_path) {
                    match fs::copy(entry.path(), &original_path) {
                        Ok(_) => restored += 1,
                        Err(_) => failed += 1,
                    }
                }
            }
        }

        self.status_message = Some((
            format!("Restore complete: {} files restored, {} failed", restored, failed),
            failed > 0,
        ));

        None
    }

    fn delete_backup(&mut self, backup_path: &Path) -> Option<Message> {
        match fs::remove_dir_all(backup_path) {
            Ok(_) => {
                self.status_message = Some(("Backup deleted successfully".to_string(), false));
                self.load_backups();
            }
            Err(e) => {
                self.status_message = Some((format!("Failed to delete backup: {}", e), true));
            }
        }
        None
    }

    fn format_size(bytes: u64) -> String {
        const KB: u64 = 1024;
        const MB: u64 = KB * 1024;

        if bytes >= MB {
            format!("{:.1} MB", bytes as f64 / MB as f64)
        } else if bytes >= KB {
            format!("{:.1} KB", bytes as f64 / KB as f64)
        } else {
            format!("{} B", bytes)
        }
    }
}

impl Component for BackupComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    if let Some(action) = self.pending_action.take() {
                        return match action {
                            BackupAction::CreateBackup => self.create_backup(),
                            BackupAction::RestoreBackup(path) => self.restore_backup(&path),
                            BackupAction::DeleteBackup(path) => self.delete_backup(&path),
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
            KeyCode::Tab => {
                self.mode = match self.mode {
                    BackupMode::Create => BackupMode::Restore,
                    BackupMode::Restore => BackupMode::Create,
                };
                self.list_state.select(Some(0));
            }
            KeyCode::Up | KeyCode::Char('k') => {
                let len = match self.mode {
                    BackupMode::Create => self.config_files.len(),
                    BackupMode::Restore => self.backups.len(),
                };
                if let Some(selected) = self.list_state.selected() {
                    if selected > 0 {
                        self.list_state.select(Some(selected - 1));
                    }
                } else if len > 0 {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                let len = match self.mode {
                    BackupMode::Create => self.config_files.len(),
                    BackupMode::Restore => self.backups.len(),
                };
                if let Some(selected) = self.list_state.selected() {
                    if selected < len.saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                } else if len > 0 {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::Char(' ') if self.mode == BackupMode::Create => {
                if let Some(selected) = self.list_state.selected() {
                    if let Some(file) = self.config_files.get_mut(selected) {
                        file.2 = !file.2;
                    }
                }
            }
            KeyCode::Char('a') if self.mode == BackupMode::Create => {
                let all_selected = self.config_files.iter().all(|(_, _, s)| *s);
                for file in &mut self.config_files {
                    file.2 = !all_selected;
                }
            }
            KeyCode::Enter => {
                match self.mode {
                    BackupMode::Create => {
                        self.pending_action = Some(BackupAction::CreateBackup);
                        self.show_confirm = true;
                    }
                    BackupMode::Restore => {
                        if let Some(selected) = self.list_state.selected() {
                            if let Some(backup) = self.backups.get(selected) {
                                self.pending_action =
                                    Some(BackupAction::RestoreBackup(backup.path.clone()));
                                self.show_confirm = true;
                            }
                        }
                    }
                }
            }
            KeyCode::Char('d') if self.mode == BackupMode::Restore => {
                if let Some(selected) = self.list_state.selected() {
                    if let Some(backup) = self.backups.get(selected) {
                        self.pending_action = Some(BackupAction::DeleteBackup(backup.path.clone()));
                        self.show_confirm = true;
                    }
                }
            }
            KeyCode::F(5) => {
                self.load_backups();
                self.status_message = Some(("Backup list refreshed".to_string(), false));
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

        // Mode tabs
        let mode_text = match self.mode {
            BackupMode::Create => "[Create Backup]  Restore Backup",
            BackupMode::Restore => " Create Backup  [Restore Backup]",
        };
        let mode_bar = Paragraph::new(Line::from(vec![
            Span::styled("Mode: ", Style::default().fg(Color::Cyan)),
            Span::raw(mode_text),
        ]))
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title(" Backup & Restore "),
        );
        frame.render_widget(mode_bar, chunks[0]);

        // Content
        match self.mode {
            BackupMode::Create => self.render_create_mode(frame, chunks[1]),
            BackupMode::Restore => self.render_restore_mode(frame, chunks[1]),
        }

        // Status bar
        let status_content = if self.show_confirm {
            let action_desc = match &self.pending_action {
                Some(BackupAction::CreateBackup) => "Create backup?".to_string(),
                Some(BackupAction::RestoreBackup(_)) => "Restore this backup?".to_string(),
                Some(BackupAction::DeleteBackup(_)) => "Delete this backup?".to_string(),
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
                format!("Backup directory: {}", BACKUP_DIR),
                Style::default().fg(Color::DarkGray),
            ))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
        frame.render_widget(status, chunks[2]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        match self.mode {
            BackupMode::Create => vec![
                ("Tab", "Switch Mode"),
                ("Space", "Toggle"),
                ("a", "Select All"),
                ("Enter", "Backup"),
            ],
            BackupMode::Restore => vec![
                ("Tab", "Switch Mode"),
                ("Enter", "Restore"),
                ("d", "Delete"),
            ],
        }
    }

    fn on_activate(&mut self) {
        self.load_backups();
    }
}

impl BackupComponent {
    fn render_create_mode(&self, frame: &mut Frame, area: Rect) {
        let items: Vec<ListItem> = self
            .config_files
            .iter()
            .map(|(path, desc, selected)| {
                let checkbox = if *selected { "[✓]" } else { "[ ]" };
                let exists = Path::new(path).exists();
                let status = if exists { "" } else { " (not found)" };

                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            checkbox,
                            Style::default().fg(if *selected {
                                Color::Green
                            } else {
                                Color::DarkGray
                            }),
                        ),
                        Span::raw(" "),
                        Span::styled(
                            path.clone(),
                            Style::default().add_modifier(if exists {
                                Modifier::empty()
                            } else {
                                Modifier::DIM
                            }),
                        ),
                        Span::styled(status, Style::default().fg(Color::Red)),
                    ]),
                    Line::from(Span::styled(
                        format!("    {}", desc),
                        Style::default().fg(Color::DarkGray),
                    )),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(" Select files to backup "),
            )
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, area, &mut state);
    }

    fn render_restore_mode(&self, frame: &mut Frame, area: Rect) {
        if self.backups.is_empty() {
            let empty = Paragraph::new(Line::from(Span::styled(
                "No backups found",
                Style::default().fg(Color::DarkGray),
            )))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(" Available Backups "),
            );
            frame.render_widget(empty, area);
            return;
        }

        let items: Vec<ListItem> = self
            .backups
            .iter()
            .map(|backup| {
                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            backup.timestamp.format("%Y-%m-%d %H:%M:%S").to_string(),
                            Style::default()
                                .fg(Color::Cyan)
                                .add_modifier(Modifier::BOLD),
                        ),
                    ]),
                    Line::from(vec![
                        Span::styled("    Files: ", Style::default().fg(Color::DarkGray)),
                        Span::raw(format!("{}", backup.file_count)),
                        Span::styled("  Size: ", Style::default().fg(Color::DarkGray)),
                        Span::raw(Self::format_size(backup.size)),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(" Available Backups "),
            )
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, area, &mut state);
    }
}
