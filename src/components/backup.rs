use chrono::{DateTime, Local};
use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{List, ListItem, ListState, Paragraph},
    Frame,
};
use std::fs;
use std::fs::Permissions;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

const BACKUP_DIR: &str = "/var/backups/slackware-cli-manager";
const SENSITIVE_FILES: &[&str] = &["/etc/shadow"];

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
    Create,
    Restore(PathBuf),
    Delete(PathBuf),
}

impl BackupComponent {
    pub fn new() -> Self {
        let config_files = CONFIG_FILES
            .iter()
            .map(|(path, desc)| {
                (
                    path.to_string(),
                    desc.to_string(),
                    !SENSITIVE_FILES.contains(path),
                )
            })
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

    fn ensure_backup_dir() -> std::io::Result<()> {
        fs::create_dir_all(BACKUP_DIR)?;
        fs::set_permissions(BACKUP_DIR, Permissions::from_mode(0o700))
    }

    fn load_backups(&mut self) {
        self.backups.clear();

        if Self::ensure_backup_dir().is_err() {
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
                                .map(|dt| {
                                    DateTime::from_naive_utc_and_offset(dt, *Local::now().offset())
                                })
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

    pub fn execute(
        action: BackupAction,
        config_files: &[(String, String, bool)],
    ) -> Result<String, String> {
        match action {
            BackupAction::Create => Self::create_backup(config_files),
            BackupAction::Restore(path) => Self::restore_backup(&path),
            BackupAction::Delete(path) => Self::delete_backup(&path),
        }
    }

    fn create_backup(config_files: &[(String, String, bool)]) -> Result<String, String> {
        Self::ensure_backup_dir().map_err(|err| err.to_string())?;

        let timestamp = Local::now().format("%Y%m%d_%H%M%S");
        let backup_path = PathBuf::from(BACKUP_DIR).join(format!("backup_{}", timestamp));

        fs::create_dir_all(&backup_path).map_err(|err| err.to_string())?;
        fs::set_permissions(&backup_path, Permissions::from_mode(0o700))
            .map_err(|err| err.to_string())?;

        let mut backed_up = 0;
        let mut failed = 0;

        for (path, _, selected) in config_files {
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
                Ok(_) => {
                    backed_up += 1;
                    let _ = fs::set_permissions(&dest, Permissions::from_mode(0o600));
                }
                Err(_) => failed += 1,
            }
        }

        if backed_up > 0 {
            Ok(format!(
                "Backup created: {} files backed up, {} failed",
                backed_up, failed
            ))
        } else {
            let _ = fs::remove_dir(&backup_path);
            Err("No files were backed up".to_string())
        }
    }

    fn restore_backup(backup_path: &Path) -> Result<String, String> {
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

        if restored == 0 && failed == 0 {
            return Err("Backup did not contain any restorable files".to_string());
        }

        Ok(format!(
            "Restore complete: {} files restored, {} failed",
            restored, failed
        ))
    }

    fn delete_backup(backup_path: &Path) -> Result<String, String> {
        fs::remove_dir_all(backup_path)
            .map(|_| "Backup deleted successfully".to_string())
            .map_err(|err| err.to_string())
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

    pub fn set_status(&mut self, message: String, is_error: bool) {
        self.status_message = Some((message, is_error));
    }

    pub fn refresh_backups(&mut self) {
        self.load_backups();
    }

    pub fn config_files(&self) -> &[(String, String, bool)] {
        &self.config_files
    }

    fn is_sensitive(path: &str) -> bool {
        SENSITIVE_FILES.contains(&path)
    }
}

impl Component for BackupComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    if let Some(action) = self.pending_action.take() {
                        return Some(Message::BackupAction(action));
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
            KeyCode::Enter => match self.mode {
                BackupMode::Create => {
                    self.pending_action = Some(BackupAction::Create);
                    self.show_confirm = true;
                }
                BackupMode::Restore => {
                    if let Some(selected) = self.list_state.selected() {
                        if let Some(backup) = self.backups.get(selected) {
                            self.pending_action = Some(BackupAction::Restore(backup.path.clone()));
                            self.show_confirm = true;
                        }
                    }
                }
            },
            KeyCode::Char('d') if self.mode == BackupMode::Restore => {
                if let Some(selected) = self.list_state.selected() {
                    if let Some(backup) = self.backups.get(selected) {
                        self.pending_action = Some(BackupAction::Delete(backup.path.clone()));
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
            Span::styled("Mode ", Theme::label()),
            Span::raw(mode_text),
            Span::raw(" "),
            Span::styled(
                format!(" {} snapshots ", self.backups.len()),
                Theme::badge_neutral(),
            ),
        ]))
        .block(Theme::panel(Theme::panel_title("Vault")));
        frame.render_widget(mode_bar, chunks[0]);

        // Content
        match self.mode {
            BackupMode::Create => self.render_create_mode(frame, chunks[1]),
            BackupMode::Restore => self.render_restore_mode(frame, chunks[1]),
        }

        // Status bar
        let status_content = if self.show_confirm {
            let action_desc = match &self.pending_action {
                Some(BackupAction::Create) => "Create backup?".to_string(),
                Some(BackupAction::Restore(_)) => "Restore this backup?".to_string(),
                Some(BackupAction::Delete(_)) => "Delete this backup?".to_string(),
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
                format!(
                    "Backup directory: {} | sensitive files are opt-in",
                    BACKUP_DIR
                ),
                Theme::muted(),
            ))
        };

        let status =
            Paragraph::new(status_content).block(Theme::panel_alt(Theme::panel_title("Status")));
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
    fn selected_config_file(&self) -> Option<&(String, String, bool)> {
        (self.mode == BackupMode::Create)
            .then(|| {
                self.list_state
                    .selected()
                    .and_then(|idx| self.config_files.get(idx))
            })
            .flatten()
    }

    fn selected_backup(&self) -> Option<&BackupEntry> {
        (self.mode == BackupMode::Restore)
            .then(|| {
                self.list_state
                    .selected()
                    .and_then(|idx| self.backups.get(idx))
            })
            .flatten()
    }

    fn render_create_mode(&self, frame: &mut Frame, area: Rect) {
        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(62), Constraint::Percentage(38)])
            .split(area);

        let items: Vec<ListItem> = self
            .config_files
            .iter()
            .map(|(path, desc, selected)| {
                let checkbox = if *selected { " ✓ " } else { "   " };
                let exists = Path::new(path).exists();
                let exists_badge = if exists {
                    Span::styled(" READY ", Theme::badge_success())
                } else {
                    Span::styled(" MISSING ", Theme::badge_warning())
                };
                let sensitivity_badge = if Self::is_sensitive(path) {
                    Some(Span::styled(" SENSITIVE ", Theme::badge_warning()))
                } else {
                    None
                };

                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            checkbox,
                            if *selected {
                                Theme::badge_success()
                            } else {
                                Theme::badge_neutral()
                            },
                        ),
                        Span::raw(" "),
                        Span::styled(
                            path.clone(),
                            Theme::value().add_modifier(if exists {
                                Modifier::empty()
                            } else {
                                Modifier::DIM
                            }),
                        ),
                        Span::raw(" "),
                        exists_badge,
                        if sensitivity_badge.is_some() {
                            Span::raw(" ")
                        } else {
                            Span::raw("")
                        },
                        sensitivity_badge.unwrap_or_else(|| Span::raw("")),
                    ]),
                    Line::from(Span::styled(format!("    {}", desc), Theme::muted())),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(Theme::panel(Theme::panel_title("Selected files")))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, content[0], &mut state);

        let summary = {
            let total = self.config_files.len();
            let selected = self
                .config_files
                .iter()
                .filter(|(_, _, selected)| *selected)
                .count();
            let sensitive = self
                .config_files
                .iter()
                .filter(|(path, _, selected)| *selected && Self::is_sensitive(path))
                .count();

            let lines = if let Some((path, desc, selected_flag)) = self.selected_config_file() {
                vec![
                    Line::from(vec![
                        Span::styled("SELECTION", Theme::badge_info()),
                        Span::raw(" "),
                        Span::styled(path, Theme::title()),
                    ]),
                    Line::from(""),
                    Line::from(vec![
                        Span::styled("Purpose ", Theme::label()),
                        Span::raw(desc),
                    ]),
                    Line::from(vec![
                        Span::styled("Included ", Theme::label()),
                        Span::styled(
                            if *selected_flag { " YES " } else { " NO " },
                            if *selected_flag {
                                Theme::badge_success()
                            } else {
                                Theme::badge_warning()
                            },
                        ),
                    ]),
                    Line::from(vec![
                        Span::styled("Sensitivity ", Theme::label()),
                        Span::styled(
                            if Self::is_sensitive(path) {
                                " HIGH "
                            } else {
                                " NORMAL "
                            },
                            if Self::is_sensitive(path) {
                                Theme::badge_warning()
                            } else {
                                Theme::badge_neutral()
                            },
                        ),
                    ]),
                    Line::from(""),
                    Line::from(vec![
                        Span::styled("Selected set ", Theme::label()),
                        Span::styled(
                            format!(" {} / {} ", selected, total),
                            Theme::badge_neutral(),
                        ),
                    ]),
                    Line::from(vec![
                        Span::styled("Sensitive included ", Theme::label()),
                        Span::styled(format!(" {} ", sensitive), Theme::badge_warning()),
                    ]),
                ]
            } else {
                vec![
                    Line::from(Span::styled("No file selected", Theme::muted())),
                    Line::from(""),
                    Line::from("Move through the vault roster to inspect backup coverage."),
                ]
            };

            Paragraph::new(lines).block(Theme::panel_alt(Theme::panel_title("Inspector")))
        };
        frame.render_widget(summary, content[1]);
    }

    fn render_restore_mode(&self, frame: &mut Frame, area: Rect) {
        if self.backups.is_empty() {
            let empty =
                Paragraph::new(Line::from(Span::styled("No backups found", Theme::muted())))
                    .block(Theme::panel(Theme::panel_title("Available backups")));
            frame.render_widget(empty, area);
            return;
        }

        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(60), Constraint::Percentage(40)])
            .split(area);

        let items: Vec<ListItem> = self
            .backups
            .iter()
            .map(|backup| {
                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            backup.timestamp.format("%Y-%m-%d %H:%M:%S").to_string(),
                            Theme::accent().add_modifier(Modifier::BOLD),
                        ),
                        Span::styled("  ", Style::default()),
                        Span::styled(&backup.name, Theme::muted()),
                    ]),
                    Line::from(vec![
                        Span::styled("    Files ", Theme::muted()),
                        Span::raw(format!("{}", backup.file_count)),
                        Span::styled("  Size ", Theme::muted()),
                        Span::raw(Self::format_size(backup.size)),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(Theme::panel(Theme::panel_title("Available backups")))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, content[0], &mut state);

        let inspector_lines = if let Some(backup) = self.selected_backup() {
            vec![
                Line::from(vec![
                    Span::styled("SNAPSHOT", Theme::badge_info()),
                    Span::raw(" "),
                    Span::styled(&backup.name, Theme::title()),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled("Created ", Theme::label()),
                    Span::raw(backup.timestamp.format("%Y-%m-%d %H:%M:%S").to_string()),
                ]),
                Line::from(vec![
                    Span::styled("Files ", Theme::label()),
                    Span::styled(backup.file_count.to_string(), Theme::badge_neutral()),
                    Span::raw(" "),
                    Span::styled("Size ", Theme::label()),
                    Span::styled(Self::format_size(backup.size), Theme::badge_success()),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled("Path ", Theme::label()),
                    Span::raw(backup.path.display().to_string()),
                ]),
                Line::from(""),
                Line::from(Span::styled(
                    "Restore replaces known managed files; delete removes the snapshot directory.",
                    Theme::subtitle(),
                )),
            ]
        } else {
            vec![
                Line::from(Span::styled("No backup selected", Theme::muted())),
                Line::from(""),
                Line::from("Choose a snapshot to inspect restore scope."),
            ]
        };

        frame.render_widget(
            Paragraph::new(inspector_lines)
                .block(Theme::panel_alt(Theme::panel_title("Inspector"))),
            content[1],
        );
    }
}
