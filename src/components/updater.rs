use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    text::Line,
    widgets::{Block, Borders, Paragraph},
    Frame,
};
use tokio::sync::mpsc;

use super::{AsyncComponent, Component};
use crate::app::Message;
use crate::slackware::Bootloader;
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
    progress_tx: Option<mpsc::UnboundedSender<String>>,

    // Safety features
    bootloader: Bootloader,
    kernel_updated: bool,
    skip_input: String,
    lilo_skipped: bool,
    show_summary: bool,
}

impl UpdaterComponent {
    pub fn new() -> Self {
        Self {
            steps: vec![
                ProgressStep::new("Update package list"),
                ProgressStep::new("Install new packages"),
                ProgressStep::new("Upgrade all packages"),
                ProgressStep::new("Clean system"),
                ProgressStep::new("Update bootloader (lilo)"),
            ],
            current_step: 0,
            output_lines: Vec::new(),
            is_running: false,
            show_lilo_confirm: false,
            lilo_confirmed: false,
            progress_tx: None,

            bootloader: Bootloader::detect(),
            kernel_updated: false,
            skip_input: String::new(),
            lilo_skipped: false,
            show_summary: false,
        }
    }

    pub fn reset(&mut self) {
        self.steps = vec![
            ProgressStep::new("Update package list"),
            ProgressStep::new("Install new packages"),
            ProgressStep::new("Upgrade all packages"),
            ProgressStep::new("Clean system"),
            ProgressStep::new("Update bootloader (lilo)"),
        ];
        self.current_step = 0;
        self.output_lines.clear();
        self.is_running = false;
        self.show_lilo_confirm = false;
        self.lilo_confirmed = false;
        self.kernel_updated = false;
        self.skip_input.clear();
        self.lilo_skipped = false;
        self.show_summary = false;
        // Re-detect bootloader on reset
        self.bootloader = Bootloader::detect();
    }

    pub fn start_update(&mut self) {
        self.reset();
        self.is_running = true;
        self.steps[0].status = StepStatus::Running;
    }

    pub fn add_output(&mut self, line: String) {
        self.output_lines.push(line);
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
        kernel_patterns.iter().any(|p| output.contains(p))
    }

    /// Set whether kernel was updated (called from app.rs)
    pub fn set_kernel_updated(&mut self, updated: bool) {
        self.kernel_updated = updated;
        if updated {
            self.add_output("*** KERNEL PACKAGES DETECTED - Bootloader update will be required ***".to_string());
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

    /// Get detected bootloader
    pub fn get_bootloader(&self) -> Bootloader {
        self.bootloader
    }

    pub fn step_complete(&mut self, success: bool, error: Option<String>) {
        if self.current_step < self.steps.len() {
            self.steps[self.current_step].status = if success {
                StepStatus::Complete
            } else {
                StepStatus::Failed(error.unwrap_or_default())
            };

            self.current_step += 1;

            if self.current_step < self.steps.len() {
                // Check if we're at the bootloader step (step 4)
                if self.current_step == 4 {
                    // Handle based on detected bootloader
                    match self.bootloader {
                        Bootloader::Grub => {
                            // GRUB detected - skip LILO step with informative message
                            self.steps[self.current_step].status = StepStatus::Complete;
                            self.add_output("GRUB detected - skipping LILO. Run 'grub-mkconfig -o /boot/grub/grub.cfg' if kernel was updated.".to_string());
                            self.current_step += 1;
                            self.is_running = false;
                            self.show_summary = true;
                            return;
                        }
                        Bootloader::Unknown => {
                            // Unknown bootloader - skip with warning
                            self.steps[self.current_step].status = StepStatus::Failed("No bootloader detected".to_string());
                            self.add_output("WARNING: No bootloader configuration found. Update your bootloader manually if needed.".to_string());
                            self.current_step += 1;
                            self.is_running = false;
                            self.show_summary = true;
                            return;
                        }
                        Bootloader::Lilo => {
                            // LILO detected - show confirmation
                            if !self.lilo_confirmed {
                                self.show_lilo_confirm = true;
                                self.skip_input.clear();
                                return;
                            }
                        }
                    }
                }
                self.steps[self.current_step].status = StepStatus::Running;
            } else {
                self.is_running = false;
                self.show_summary = true;
            }
        }
    }

    pub fn confirm_lilo(&mut self, confirmed: bool) {
        self.show_lilo_confirm = false;
        self.lilo_confirmed = confirmed;

        if confirmed {
            self.steps[self.current_step].status = StepStatus::Running;
        } else {
            self.lilo_skipped = true;
            if self.kernel_updated {
                self.steps[self.current_step].status = StepStatus::Failed("SKIPPED - KERNEL WAS UPDATED!".to_string());
            } else {
                self.steps[self.current_step].status = StepStatus::Failed("Skipped by user".to_string());
            }
            self.is_running = false;
            self.show_summary = true;
        }
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

    fn render_lilo_confirm(&self, frame: &mut Frame, area: Rect) {
        let dialog_area = crate::ui::centered_rect(60, 50, area);
        frame.render_widget(ratatui::widgets::Clear, dialog_area);

        if self.kernel_updated {
            // Mandatory LILO - kernel was updated
            let dialog = Block::default()
                .title(" !! KERNEL UPDATED - BOOTLOADER REQUIRED !! ")
                .borders(Borders::ALL)
                .border_style(Theme::error());
            let inner = dialog.inner(dialog_area);
            frame.render_widget(dialog, dialog_area);

            let skip_display: String = self.skip_input.chars().map(|_| '*').collect();
            let remaining = 4 - self.skip_input.len();
            let underscores = "_".repeat(remaining);

            let text = Paragraph::new(vec![
                Line::from(""),
                Line::from(ratatui::text::Span::styled(
                    "Your kernel was updated.",
                    Theme::warning(),
                )),
                Line::from(""),
                Line::from("You MUST update the bootloader or your"),
                Line::from("system will NOT boot after reboot!"),
                Line::from(""),
                Line::from(vec![
                    ratatui::text::Span::styled("[Y]", Theme::key_hint()),
                    ratatui::text::Span::raw(" Update bootloader now (Recommended)"),
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
            // Optional LILO - no kernel update detected
            let dialog = Block::default()
                .title(" Update Bootloader? ")
                .borders(Borders::ALL)
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
                    ratatui::text::Span::raw(" Yes - Update bootloader"),
                ]),
                Line::from(vec![
                    ratatui::text::Span::styled("[N]", Theme::key_hint()),
                    ratatui::text::Span::raw(" No - Skip this step"),
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

        let dialog = Block::default()
            .title(title)
            .borders(Borders::ALL)
            .border_style(border_style);
        let inner = dialog.inner(dialog_area);
        frame.render_widget(dialog, dialog_area);

        let mut lines = vec![Line::from("")];

        // Show status of each step
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

        // Show warning if LILO was skipped after kernel update
        if self.lilo_skipped && self.kernel_updated {
            lines.push(Line::from(ratatui::text::Span::styled(
                "  !! WARNING !!",
                Theme::error(),
            )));
            lines.push(Line::from(""));
            lines.push(Line::from(ratatui::text::Span::styled(
                "  Bootloader was NOT updated after kernel change!",
                Theme::error(),
            )));
            lines.push(Line::from(ratatui::text::Span::styled(
                "  Run 'lilo' manually BEFORE rebooting!",
                Theme::error(),
            )));
            lines.push(Line::from(""));
        } else if self.lilo_skipped {
            lines.push(Line::from(ratatui::text::Span::styled(
                "  Note: Bootloader was skipped.",
                Theme::warning(),
            )));
            lines.push(Line::from(""));
        }

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
        Self::new()
    }
}

impl Component for UpdaterComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        // Handle summary dismissal
        if self.show_summary {
            if let KeyCode::Enter = key.code {
                self.dismiss_summary();
            }
            return None;
        }

        // Handle LILO confirmation
        if self.show_lilo_confirm {
            if self.kernel_updated {
                // Mandatory LILO mode - require Y or typing "SKIP"
                match key.code {
                    KeyCode::Char('y') | KeyCode::Char('Y') => {
                        self.confirm_lilo(true);
                        return Some(Message::ContinueUpdate);
                    }
                    KeyCode::Char(c) => {
                        // Build up SKIP input
                        let c_upper = c.to_ascii_uppercase();
                        let expected = ['S', 'K', 'I', 'P'];
                        let next_idx = self.skip_input.len();

                        if next_idx < 4 && c_upper == expected[next_idx] {
                            self.skip_input.push(c_upper);
                            if self.skip_input == "SKIP" {
                                self.confirm_lilo(false);
                            }
                        } else {
                            // Wrong character - reset
                            self.skip_input.clear();
                        }
                        return None;
                    }
                    KeyCode::Backspace => {
                        self.skip_input.pop();
                        return None;
                    }
                    KeyCode::Esc => {
                        // ESC just clears input, doesn't skip
                        self.skip_input.clear();
                        return None;
                    }
                    _ => return None,
                }
            } else {
                // Optional LILO mode - Y/N works
                match key.code {
                    KeyCode::Char('y') | KeyCode::Char('Y') => {
                        self.confirm_lilo(true);
                        return Some(Message::ContinueUpdate);
                    }
                    KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                        self.confirm_lilo(false);
                        return None;
                    }
                    _ => return None,
                }
            }
        }

        match key.code {
            KeyCode::Enter if !self.is_running => {
                self.start_update();
                Some(Message::StartUpdate)
            }
            KeyCode::Char('r') | KeyCode::Char('R') if !self.is_running => {
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
                Constraint::Length(3),  // Title
                Constraint::Length(10), // Progress steps
                Constraint::Min(5),     // Output
            ])
            .split(area);

        // Title with bootloader info
        let bootloader_info = format!(" [Bootloader: {}]", self.bootloader.name());
        let title = Paragraph::new(Line::from(vec![
            ratatui::text::Span::styled("Slackware System Updater", Theme::title()),
            ratatui::text::Span::styled(bootloader_info, Theme::muted()),
        ]))
        .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(title, chunks[0]);

        // Progress steps
        let progress = ProgressList::new(&self.steps)
            .block(Block::default().borders(Borders::ALL).title("Progress"));
        frame.render_widget(progress, chunks[1]);

        // Output
        let output_block = Block::default().borders(Borders::ALL).title("Output");
        let inner = output_block.inner(chunks[2]);
        frame.render_widget(output_block, chunks[2]);

        let visible = inner.height as usize;
        let start = self.output_lines.len().saturating_sub(visible);
        let lines: Vec<Line> = self.output_lines[start..]
            .iter()
            .map(|s| {
                // Highlight kernel warnings
                if s.contains("KERNEL") {
                    Line::from(ratatui::text::Span::styled(s.as_str(), Theme::warning()))
                } else {
                    Line::from(s.as_str())
                }
            })
            .collect();
        let output = Paragraph::new(lines).style(Theme::muted());
        frame.render_widget(output, inner);

        // Show dialogs on top
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
            vec![("Enter", "Start Update"), ("R", "Reset")]
        }
    }
}

impl AsyncComponent for UpdaterComponent {
    fn set_progress_channel(&mut self, tx: mpsc::UnboundedSender<String>) {
        self.progress_tx = Some(tx);
    }

    fn is_running(&self) -> bool {
        self.is_running
    }
}
