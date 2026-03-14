use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    text::{Line, Span},
    widgets::Paragraph,
    Frame,
};
use std::fs;
use std::path::{Path, PathBuf};

use super::Component;
use crate::app::Message;
use crate::slackware::{Bootloader, SlackwareVersion};
use crate::ui::theme::Theme;
use crate::ui::widgets::{ProgressList, ProgressStep, StepStatus};

/// System updater component - runs slackpkg update sequence
pub struct UpdaterComponent {
    steps: Vec<ProgressStep>,
    pub current_step: usize,
    output_lines: Vec<String>,
    is_running: bool,
    show_lilo_confirm: bool,
    lilo_confirmed: bool,

    // Safety features
    slackware_version: SlackwareVersion,
    bootloader: Bootloader,
    kernel_updated: bool,
    skip_input: String,
    lilo_skipped: bool,
    show_summary: bool,
    blacklist_entries: Vec<String>,
    changelog_path: Option<PathBuf>,
    new_config_files: Vec<PathBuf>,
}

impl UpdaterComponent {
    pub fn new(slackware_version: SlackwareVersion) -> Self {
        let mut component = Self {
            steps: Self::default_steps(),
            current_step: 0,
            output_lines: Vec::new(),
            is_running: false,
            show_lilo_confirm: false,
            lilo_confirmed: false,
            slackware_version,
            bootloader: Bootloader::detect(),
            kernel_updated: false,
            skip_input: String::new(),
            lilo_skipped: false,
            show_summary: false,
            blacklist_entries: Vec::new(),
            changelog_path: None,
            new_config_files: Vec::new(),
        };
        component.refresh_system_context();
        component
    }

    fn default_steps() -> Vec<ProgressStep> {
        vec![
            ProgressStep::new("Update package list"),
            ProgressStep::new("Install new packages"),
            ProgressStep::new("Upgrade all packages"),
            ProgressStep::new("Clean system"),
            ProgressStep::new("Update bootloader (lilo)"),
        ]
    }

    pub fn reset(&mut self) {
        self.steps = Self::default_steps();
        self.current_step = 0;
        self.output_lines.clear();
        self.is_running = false;
        self.show_lilo_confirm = false;
        self.lilo_confirmed = false;
        self.kernel_updated = false;
        self.skip_input.clear();
        self.lilo_skipped = false;
        self.show_summary = false;
        self.refresh_system_context();
    }

    pub fn start_update(&mut self) {
        self.reset();
        self.is_running = true;
        self.steps[0].status = StepStatus::Running;
        self.output_lines.push(format!(
            "Release track: {} | Bootloader: {}",
            self.release_track_label(),
            self.bootloader.name()
        ));
    }

    pub fn add_output(&mut self, line: String) {
        self.output_lines.push(line);
    }

    #[cfg(test)]
    pub fn last_output(&self) -> Option<&str> {
        self.output_lines.last().map(String::as_str)
    }

    /// Check if output contains kernel package updates
    pub fn check_for_kernel_update(&self, output: &str) -> bool {
        let kernel_patterns = [
            "kernel-generic",
            "kernel-huge",
            "kernel-modules",
            "kernel-source",
            "kernel-headers",
            "kernel-firmware",
        ];
        kernel_patterns
            .iter()
            .any(|pattern| output.contains(pattern))
    }

    /// Set whether kernel was updated (called from app.rs)
    pub fn set_kernel_updated(&mut self, updated: bool) {
        self.kernel_updated = updated;
        if updated {
            self.add_output(
                "*** KERNEL PACKAGES DETECTED - bootloader and config review required ***"
                    .to_string(),
            );
        }
    }

    /// Check if kernel was updated
    pub fn was_kernel_updated(&self) -> bool {
        self.kernel_updated
    }

    /// Check if LILO was skipped (for exit warning)
    pub fn was_lilo_skipped(&self) -> bool {
        self.lilo_skipped
    }

    pub fn step_complete(&mut self, success: bool, error: Option<String>) {
        if self.current_step >= self.steps.len() {
            return;
        }

        self.steps[self.current_step].status = if success {
            StepStatus::Complete
        } else {
            StepStatus::Failed(error.unwrap_or_default())
        };

        self.current_step += 1;

        if self.current_step >= self.steps.len() {
            self.is_running = false;
            self.show_summary = true;
            self.refresh_system_context();
            return;
        }

        if self.current_step == 4 {
            match self.bootloader {
                Bootloader::Grub => {
                    self.steps[self.current_step].status = StepStatus::Complete;
                    self.add_output(
                        "GRUB detected - skipping lilo. Run 'grub-mkconfig -o /boot/grub/grub.cfg' if the kernel changed.".to_string(),
                    );
                    self.current_step += 1;
                    self.is_running = false;
                    self.show_summary = true;
                    self.refresh_system_context();
                    return;
                }
                Bootloader::Unknown => {
                    self.steps[self.current_step].status =
                        StepStatus::Failed("No bootloader detected".to_string());
                    self.add_output(
                        "WARNING: no bootloader configuration found. Update your bootloader manually before reboot if needed.".to_string(),
                    );
                    self.current_step += 1;
                    self.is_running = false;
                    self.show_summary = true;
                    self.refresh_system_context();
                    return;
                }
                Bootloader::Lilo => {
                    if !self.lilo_confirmed {
                        self.show_lilo_confirm = true;
                        self.skip_input.clear();
                        return;
                    }
                }
            }
        }

        self.steps[self.current_step].status = StepStatus::Running;
    }

    pub fn confirm_lilo(&mut self, confirmed: bool) {
        self.show_lilo_confirm = false;
        self.lilo_confirmed = confirmed;

        if confirmed {
            self.steps[self.current_step].status = StepStatus::Running;
            return;
        }

        self.lilo_skipped = true;
        self.steps[self.current_step].status = if self.kernel_updated {
            StepStatus::Failed("SKIPPED - KERNEL WAS UPDATED!".to_string())
        } else {
            StepStatus::Failed("Skipped by user".to_string())
        };
        self.is_running = false;
        self.show_summary = true;
        self.refresh_system_context();
    }

    /// Dismiss the summary screen
    pub fn dismiss_summary(&mut self) {
        self.show_summary = false;
    }

    /// Check if summary is showing
    pub fn is_showing_summary(&self) -> bool {
        self.show_summary
    }

    /// Check if update is running
    pub fn is_running(&self) -> bool {
        self.is_running
    }

    pub fn get_current_command(&self) -> Option<(&str, Vec<&str>)> {
        if !self.is_running {
            return None;
        }

        match self.current_step {
            0 => Some(("slackpkg", vec!["update"])),
            1 => Some(("slackpkg", vec!["install-new"])),
            2 => Some(("slackpkg", vec!["upgrade-all"])),
            3 => Some(("slackpkg", vec!["clean-system"])),
            4 if self.lilo_confirmed => Some(("lilo", vec![])),
            _ => None,
        }
    }

    pub fn needs_lilo_confirm(&self) -> bool {
        self.show_lilo_confirm
    }

    pub fn release_track_label(&self) -> &'static str {
        match self.slackware_version {
            SlackwareVersion::Current => "-current",
            _ => "stable",
        }
    }

    fn refresh_system_context(&mut self) {
        self.bootloader = Bootloader::detect();
        self.blacklist_entries = Self::read_blacklist_entries();
        self.changelog_path = Self::detect_changelog_path();
        self.new_config_files = Self::scan_new_config_files(Path::new("/etc"), 12);
    }

    fn read_blacklist_entries() -> Vec<String> {
        fs::read_to_string("/etc/slackpkg/blacklist")
            .map(|content| Self::parse_blacklist_entries(&content))
            .unwrap_or_default()
    }

    fn parse_blacklist_entries(content: &str) -> Vec<String> {
        content
            .lines()
            .map(str::trim)
            .filter(|line| !line.is_empty() && !line.starts_with('#'))
            .map(ToString::to_string)
            .collect()
    }

    fn detect_changelog_path() -> Option<PathBuf> {
        [
            "/var/lib/slackpkg/ChangeLog.txt",
            "/var/log/ChangeLog.txt",
            "/patches/ChangeLog.txt",
        ]
        .iter()
        .map(PathBuf::from)
        .find(|path| path.exists())
    }

    fn scan_new_config_files(root: &Path, max_results: usize) -> Vec<PathBuf> {
        fn walk(dir: &Path, found: &mut Vec<PathBuf>, max_results: usize) {
            if found.len() >= max_results {
                return;
            }

            let Ok(entries) = fs::read_dir(dir) else {
                return;
            };

            for entry in entries.flatten() {
                if found.len() >= max_results {
                    break;
                }

                let path = entry.path();
                if path.is_dir() {
                    walk(&path, found, max_results);
                } else if path
                    .file_name()
                    .and_then(|name| name.to_str())
                    .is_some_and(|name| name.ends_with(".new"))
                {
                    found.push(path);
                }
            }
        }

        let mut found = Vec::new();
        walk(root, &mut found, max_results);
        found
    }

    fn advisory_lines(&self) -> Vec<Line<'static>> {
        let mut lines = vec![
            Line::from(format!("Release track: {}", self.release_track_label())),
            Line::from(match self.changelog_path.as_ref() {
                Some(path) => format!("Review changelog: {}", path.display()),
                None => "Review changelog: no local ChangeLog.txt found".to_string(),
            }),
        ];

        if self.blacklist_entries.is_empty() {
            lines.push(Line::from("Blacklist: no active entries"));
        } else {
            lines.push(Line::from(format!(
                "Blacklist: {} active entr{}",
                self.blacklist_entries.len(),
                if self.blacklist_entries.len() == 1 {
                    "y"
                } else {
                    "ies"
                }
            )));
        }

        if self.new_config_files.is_empty() {
            lines.push(Line::from(
                "Config updates: no .new files detected under /etc",
            ));
        } else {
            let preview = self
                .new_config_files
                .iter()
                .take(2)
                .map(|path| path.display().to_string())
                .collect::<Vec<_>>()
                .join(", ");
            lines.push(Line::from(format!(
                "Config updates: {} .new file(s) detected",
                self.new_config_files.len()
            )));
            lines.push(Line::from(format!("Examples: {preview}")));
        }

        match self.bootloader {
            Bootloader::Lilo => lines.push(Line::from(
                "Bootloader: LILO detected. Run lilo after kernel changes.",
            )),
            Bootloader::Grub => lines.push(Line::from(
                "Bootloader: GRUB detected. Regenerate grub.cfg after kernel changes.",
            )),
            Bootloader::Unknown => lines.push(Line::from(
                "Bootloader: no known bootloader configuration detected.",
            )),
        }

        if self.release_track_label() == "-current" {
            lines.push(Line::from(
                "Warning: -current requires closer review of changelog and config changes.",
            ));
        }

        lines
    }

    fn step_totals(&self) -> (usize, usize, usize) {
        let mut complete = 0;
        let mut running = 0;
        let mut failed = 0;

        for step in &self.steps {
            match step.status {
                StepStatus::Complete => complete += 1,
                StepStatus::Running => running += 1,
                StepStatus::Failed(_) => failed += 1,
                StepStatus::Pending => {}
            }
        }

        (complete, running, failed)
    }

    fn render_summary_panel(&self, frame: &mut Frame, area: Rect) {
        let (complete, running, failed) = self.step_totals();
        let next_action = if self.show_lilo_confirm {
            "Awaiting bootloader confirmation"
        } else if self.show_summary {
            "Review summary before leaving"
        } else if self.is_running {
            "Streaming package manager output"
        } else {
            "Ready to start maintenance cycle"
        };

        let lines = vec![
            Line::from(vec![
                Span::styled("Done ", Theme::label()),
                Span::styled(format!("{complete}"), Theme::badge_success()),
                Span::raw(" "),
                Span::styled("Running ", Theme::label()),
                Span::styled(format!("{running}"), Theme::badge_neutral()),
                Span::raw(" "),
                Span::styled("Failed ", Theme::label()),
                Span::styled(format!("{failed}"), Theme::badge_warning()),
            ]),
            Line::from(vec![
                Span::styled("Next ", Theme::label()),
                Span::styled(next_action, Theme::subtitle()),
            ]),
        ];

        let panel = Paragraph::new(lines).block(Theme::panel_alt(Theme::panel_title("Run state")));
        frame.render_widget(panel, area);
    }

    fn show_changelog_preview(&mut self) {
        let Some(path) = self.changelog_path.clone() else {
            self.add_output("No local ChangeLog.txt found.".to_string());
            return;
        };

        self.add_output(format!("Showing changelog preview from {}", path.display()));
        match fs::read_to_string(&path) {
            Ok(content) => {
                for line in content.lines().take(12) {
                    self.add_output(line.to_string());
                }
            }
            Err(err) => self.add_output(format!("Failed to read {}: {err}", path.display())),
        }
    }

    fn render_lilo_confirm(&self, frame: &mut Frame, area: Rect) {
        let dialog_area = crate::ui::centered_rect(60, 50, area);
        frame.render_widget(ratatui::widgets::Clear, dialog_area);

        if self.kernel_updated {
            let dialog = Theme::panel(Theme::panel_title("Kernel updated - bootloader required"))
                .border_style(Theme::error());
            let inner = dialog.inner(dialog_area);
            frame.render_widget(dialog, dialog_area);

            let skip_display: String = self.skip_input.chars().map(|_| '*').collect();
            let remaining = 4usize.saturating_sub(self.skip_input.len());
            let underscores = "_".repeat(remaining);

            let text = Paragraph::new(vec![
                Line::from(""),
                Line::from(ratatui::text::Span::styled(
                    "Your kernel was updated.",
                    Theme::warning(),
                )),
                Line::from(""),
                Line::from("You must update the bootloader or your"),
                Line::from("system may not boot after reboot."),
                Line::from(""),
                Line::from(vec![
                    ratatui::text::Span::styled("[Y]", Theme::key_hint()),
                    ratatui::text::Span::raw(" Update bootloader now (recommended)"),
                ]),
                Line::from(""),
                Line::from(vec![
                    ratatui::text::Span::styled("Type SKIP", Theme::error()),
                    ratatui::text::Span::raw(" to bypass at your own risk"),
                ]),
                Line::from(""),
                Line::from(vec![
                    ratatui::text::Span::raw("Input: "),
                    ratatui::text::Span::styled(
                        format!("{}{}", skip_display, underscores),
                        Theme::muted(),
                    ),
                ]),
                Line::from(""),
                Line::from(ratatui::text::Span::styled(
                    "[Backspace] to correct",
                    Theme::muted(),
                )),
            ])
            .style(Theme::default());
            frame.render_widget(text, inner);
        } else {
            let dialog = Theme::panel(Theme::panel_title("Update bootloader"))
                .border_style(Theme::warning());
            let inner = dialog.inner(dialog_area);
            frame.render_widget(dialog, dialog_area);

            let text = Paragraph::new(vec![
                Line::from(""),
                Line::from("Run 'lilo' to update the bootloader?"),
                Line::from(""),
                Line::from(ratatui::text::Span::styled(
                    "No kernel changes detected - safe to skip.",
                    Theme::muted(),
                )),
                Line::from(""),
                Line::from(vec![
                    ratatui::text::Span::styled("[Y]", Theme::key_hint()),
                    ratatui::text::Span::raw(" Yes - update bootloader"),
                ]),
                Line::from(vec![
                    ratatui::text::Span::styled("[N]", Theme::key_hint()),
                    ratatui::text::Span::raw(" No - skip this step"),
                ]),
            ])
            .style(Theme::default());
            frame.render_widget(text, inner);
        }
    }

    fn render_summary(&self, frame: &mut Frame, area: Rect) {
        let dialog_area = crate::ui::centered_rect(60, 60, area);
        frame.render_widget(ratatui::widgets::Clear, dialog_area);

        let title = if self.lilo_skipped && self.kernel_updated {
            " !! UPDATE COMPLETE - WARNING !! "
        } else {
            " Update Complete "
        };

        let border_style = if self.lilo_skipped && self.kernel_updated {
            Theme::error()
        } else {
            Theme::success()
        };

        let dialog = Theme::panel(Theme::panel_title(title)).border_style(border_style);
        let inner = dialog.inner(dialog_area);
        frame.render_widget(dialog, dialog_area);

        let mut lines = vec![Line::from("")];

        for step in &self.steps {
            let (symbol, style) = match &step.status {
                StepStatus::Complete => ("OK", Theme::success()),
                StepStatus::Failed(msg) if msg.contains("SKIPPED") => ("!!", Theme::error()),
                StepStatus::Failed(_) => ("X", Theme::error()),
                _ => ("?", Theme::muted()),
            };

            lines.push(Line::from(vec![
                ratatui::text::Span::styled(format!(" [{}] ", symbol), style),
                ratatui::text::Span::raw(&step.name),
            ]));
        }

        lines.push(Line::from(""));

        if self.lilo_skipped && self.kernel_updated {
            lines.push(Line::from(ratatui::text::Span::styled(
                "  !! WARNING !!",
                Theme::error(),
            )));
            lines.push(Line::from(""));
            lines.push(Line::from(ratatui::text::Span::styled(
                "  Bootloader was not updated after kernel change.",
                Theme::error(),
            )));
            lines.push(Line::from(ratatui::text::Span::styled(
                "  Run 'lilo' manually before rebooting.",
                Theme::error(),
            )));
            lines.push(Line::from(""));
        } else if self.lilo_skipped {
            lines.push(Line::from(ratatui::text::Span::styled(
                "  Note: bootloader update was skipped.",
                Theme::warning(),
            )));
            lines.push(Line::from(""));
        }

        lines.push(Line::from("Review before reboot:"));
        lines.push(Line::from(format!(
            "  - release track: {}",
            self.release_track_label()
        )));
        lines.push(Line::from(format!(
            "  - changelog: {}",
            self.changelog_path
                .as_ref()
                .map(|path| path.display().to_string())
                .unwrap_or_else(|| "not found locally".to_string())
        )));
        lines.push(Line::from(format!(
            "  - blacklist entries: {}",
            self.blacklist_entries.len()
        )));
        lines.push(Line::from(format!(
            "  - .new files under /etc: {}",
            self.new_config_files.len()
        )));
        lines.push(Line::from(""));
        lines.push(Line::from(vec![
            ratatui::text::Span::styled("[Enter]", Theme::key_hint()),
            ratatui::text::Span::raw(" Acknowledge"),
        ]));

        let text = Paragraph::new(lines).style(Theme::default());
        frame.render_widget(text, inner);
    }
}

impl Default for UpdaterComponent {
    fn default() -> Self {
        Self::new(SlackwareVersion::Current)
    }
}

impl Component for UpdaterComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_summary {
            if let KeyCode::Enter = key.code {
                self.dismiss_summary();
            }
            return None;
        }

        if self.show_lilo_confirm {
            if self.kernel_updated {
                match key.code {
                    KeyCode::Char('y') | KeyCode::Char('Y') => {
                        self.confirm_lilo(true);
                        return Some(Message::ContinueUpdate);
                    }
                    KeyCode::Char(c) => {
                        let c_upper = c.to_ascii_uppercase();
                        let expected = ['S', 'K', 'I', 'P'];
                        let next_idx = self.skip_input.len();

                        if next_idx < expected.len() && c_upper == expected[next_idx] {
                            self.skip_input.push(c_upper);
                            if self.skip_input == "SKIP" {
                                self.confirm_lilo(false);
                            }
                        } else {
                            self.skip_input.clear();
                        }
                        return None;
                    }
                    KeyCode::Backspace => {
                        self.skip_input.pop();
                        return None;
                    }
                    KeyCode::Esc => {
                        self.skip_input.clear();
                        return None;
                    }
                    _ => return None,
                }
            }

            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.confirm_lilo(true);
                    Some(Message::ContinueUpdate)
                }
                KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                    self.confirm_lilo(false);
                    None
                }
                _ => None,
            }
        } else {
            match key.code {
                KeyCode::Enter if !self.is_running => {
                    self.start_update();
                    Some(Message::StartUpdate)
                }
                KeyCode::Char('c') | KeyCode::Char('C') if !self.is_running => {
                    self.show_changelog_preview();
                    None
                }
                KeyCode::Char('r') | KeyCode::Char('R') if !self.is_running => {
                    self.reset();
                    None
                }
                _ => None,
            }
        }
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(5),
                Constraint::Length(12),
                Constraint::Min(5),
            ])
            .split(area);

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(62), Constraint::Percentage(38)])
            .split(chunks[0]);

        let title = Paragraph::new(vec![
            Line::from(vec![
                ratatui::text::Span::styled("Slackware System Updater", Theme::title()),
                ratatui::text::Span::raw(" "),
                ratatui::text::Span::styled(
                    format!(" {} ", self.release_track_label()),
                    Theme::badge_neutral(),
                ),
                ratatui::text::Span::raw(" "),
                ratatui::text::Span::styled(
                    format!(" {} ", self.bootloader.name()),
                    Theme::badge_info(),
                ),
            ]),
            Line::from(ratatui::text::Span::styled(
                "Review the changelog, run the slackpkg cycle, then resolve bootloader and config fallout.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel(Theme::panel_title("Update runway")));
        frame.render_widget(title, header[0]);
        self.render_summary_panel(frame, header[1]);

        let middle = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(42), Constraint::Percentage(58)])
            .split(chunks[1]);

        let progress =
            ProgressList::new(&self.steps).block(Theme::panel(Theme::panel_title("Progress")));
        frame.render_widget(progress, middle[0]);

        let advisories = Paragraph::new(self.advisory_lines())
            .style(Theme::subtitle())
            .block(Theme::panel_alt(Theme::panel_title("Maintenance notes")));
        frame.render_widget(advisories, middle[1]);

        let output_block = Theme::panel_alt(Theme::panel_title("Output"));
        let inner = output_block.inner(chunks[2]);
        frame.render_widget(output_block, chunks[2]);

        let visible = inner.height as usize;
        let start = self.output_lines.len().saturating_sub(visible);
        let lines: Vec<Line> = self.output_lines[start..]
            .iter()
            .map(|line| {
                if line.contains("KERNEL") || line.contains("Warning:") {
                    Line::from(ratatui::text::Span::styled(line.as_str(), Theme::warning()))
                } else {
                    Line::from(line.as_str())
                }
            })
            .collect();
        let output = Paragraph::new(lines).style(Theme::default());
        frame.render_widget(output, inner);

        if self.show_lilo_confirm {
            self.render_lilo_confirm(frame, area);
        } else if self.show_summary {
            self.render_summary(frame, area);
        }
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        if self.show_summary {
            vec![("Enter", "Acknowledge")]
        } else if self.show_lilo_confirm {
            if self.kernel_updated {
                vec![("Y", "Update"), ("Type SKIP", "Bypass")]
            } else {
                vec![("Y", "Yes"), ("N", "No")]
            }
        } else if self.is_running {
            vec![]
        } else {
            vec![
                ("Enter", "Start Update"),
                ("C", "Show Changelog"),
                ("R", "Reset"),
            ]
        }
    }
}

#[cfg(test)]
mod tests {
    use std::time::{SystemTime, UNIX_EPOCH};

    use super::UpdaterComponent;
    use crate::slackware::SlackwareVersion;

    #[test]
    fn kernel_detection_matches_kernel_package_names() {
        let updater = UpdaterComponent::new(SlackwareVersion::V15_0);

        assert!(updater.check_for_kernel_update("Upgrading kernel-generic-6.6.1"));
        assert!(!updater.check_for_kernel_update("Upgrading aaa_base-15.0"));
    }

    #[test]
    fn blacklist_parser_ignores_comments_and_blank_lines() {
        let entries = UpdaterComponent::parse_blacklist_entries(
            "# comment\n\nkernel-firmware\nmozilla-firefox\n",
        );

        assert_eq!(entries, vec!["kernel-firmware", "mozilla-firefox"]);
    }

    #[test]
    fn new_config_scan_finds_new_files() {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let root = std::env::temp_dir().join(format!("slackware-cli-manager-updater-{nonce}"));
        let nested = root.join("rc.d");
        std::fs::create_dir_all(&nested).unwrap();
        std::fs::write(nested.join("rc.inet1.conf.new"), "test").unwrap();

        let found = UpdaterComponent::scan_new_config_files(&root, 8);

        assert_eq!(found.len(), 1);
        assert!(found[0]
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap()
            .ends_with(".new"));

        let _ = std::fs::remove_dir_all(root);
    }
}
