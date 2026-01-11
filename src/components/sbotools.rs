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
use crate::ui::theme::Theme;
use crate::ui::widgets::{ProgressList, ProgressStep, StepStatus};

const SBOPKG_URL: &str = "https://github.com/sbopkg/sbopkg/releases/download/0.38.2/sbopkg-0.38.2-noarch-1_wsr.tgz";
const SBOPKG_FILENAME: &str = "sbopkg-0.38.2-noarch-1_wsr.tgz";
const SBO_REPO_URL: &str = "https://gitlab.com/SlackBuilds.org/slackbuilds.git";

/// sbotools installer component
pub struct SbotoolsComponent {
    steps: Vec<ProgressStep>,
    current_step: usize,
    output_lines: Vec<String>,
    is_running: bool,
    progress_tx: Option<mpsc::UnboundedSender<String>>,
}

impl SbotoolsComponent {
    pub fn new() -> Self {
        Self {
            steps: vec![
                ProgressStep::new("Download sbopkg"),
                ProgressStep::new("Install sbopkg"),
                ProgressStep::new("Sync sbopkg repository"),
                ProgressStep::new("Install sbotools"),
                ProgressStep::new("Configure sbotools repository"),
                ProgressStep::new("Fetch SlackBuilds snapshot"),
            ],
            current_step: 0,
            output_lines: Vec::new(),
            is_running: false,
            progress_tx: None,
        }
    }

    pub fn reset(&mut self) {
        self.steps = vec![
            ProgressStep::new("Download sbopkg"),
            ProgressStep::new("Install sbopkg"),
            ProgressStep::new("Sync sbopkg repository"),
            ProgressStep::new("Install sbotools"),
            ProgressStep::new("Configure sbotools repository"),
            ProgressStep::new("Fetch SlackBuilds snapshot"),
        ];
        self.current_step = 0;
        self.output_lines.clear();
        self.is_running = false;
    }

    pub fn start_install(&mut self) {
        self.reset();
        self.is_running = true;
        self.steps[0].status = StepStatus::Running;
    }

    pub fn add_output(&mut self, line: String) {
        self.output_lines.push(line);
    }

    pub fn step_complete(&mut self, success: bool, error: Option<String>) {
        if self.current_step < self.steps.len() {
            self.steps[self.current_step].status = if success {
                StepStatus::Complete
            } else {
                StepStatus::Failed(error.unwrap_or_default())
            };

            if !success {
                self.is_running = false;
                return;
            }

            self.current_step += 1;

            if self.current_step < self.steps.len() {
                self.steps[self.current_step].status = StepStatus::Running;
            } else {
                self.is_running = false;
                self.add_output("All steps completed successfully!".to_string());
            }
        }
    }

    pub fn get_current_command(&self) -> Option<SbotoolsCommand> {
        if !self.is_running {
            return None;
        }

        match self.current_step {
            0 => Some(SbotoolsCommand::Download {
                url: SBOPKG_URL.to_string(),
                filename: SBOPKG_FILENAME.to_string(),
            }),
            1 => Some(SbotoolsCommand::InstallPkg {
                path: format!("/tmp/{}", SBOPKG_FILENAME),
            }),
            2 => Some(SbotoolsCommand::SbopkgSync),
            3 => Some(SbotoolsCommand::SbopkgInstall {
                package: "sbotools".to_string(),
            }),
            4 => Some(SbotoolsCommand::SboconfigRepo {
                url: SBO_REPO_URL.to_string(),
            }),
            5 => Some(SbotoolsCommand::SbosnapFetch),
            _ => None,
        }
    }
}

impl Default for SbotoolsComponent {
    fn default() -> Self {
        Self::new()
    }
}

/// Commands that the sbotools installer needs to run
pub enum SbotoolsCommand {
    Download { url: String, filename: String },
    InstallPkg { path: String },
    SbopkgSync,
    SbopkgInstall { package: String },
    SboconfigRepo { url: String },
    SbosnapFetch,
}

impl Component for SbotoolsComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        match key.code {
            KeyCode::Enter if !self.is_running => {
                self.start_install();
                Some(Message::StartSbotoolsInstall)
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
                Constraint::Length(12), // Progress steps
                Constraint::Min(5),     // Output
            ])
            .split(area);

        // Title
        let title = Paragraph::new(Line::from(vec![
            ratatui::text::Span::styled("sbotools Installer", Theme::title()),
        ]))
        .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(title, chunks[0]);

        // Description
        let desc = Paragraph::new(vec![
            Line::from(""),
            Line::from("This will install sbopkg and sbotools for SlackBuilds.org packages."),
            Line::from(""),
        ])
        .style(Theme::muted());

        let progress_chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Length(3), Constraint::Min(5)])
            .split(chunks[1]);

        frame.render_widget(desc, progress_chunks[0]);

        // Progress steps
        let progress = ProgressList::new(&self.steps)
            .block(Block::default().borders(Borders::ALL).title("Installation Steps"));
        frame.render_widget(progress, progress_chunks[1]);

        // Output
        let output_block = Block::default().borders(Borders::ALL).title("Output");
        let inner = output_block.inner(chunks[2]);
        frame.render_widget(output_block, chunks[2]);

        let visible = inner.height as usize;
        let start = self.output_lines.len().saturating_sub(visible);
        let lines: Vec<Line> = self.output_lines[start..]
            .iter()
            .map(|s| Line::from(s.as_str()))
            .collect();
        let output = Paragraph::new(lines).style(Theme::muted());
        frame.render_widget(output, inner);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        if self.is_running {
            vec![]
        } else {
            vec![("Enter", "Start Installation"), ("R", "Reset")]
        }
    }
}

impl AsyncComponent for SbotoolsComponent {
    fn set_progress_channel(&mut self, tx: mpsc::UnboundedSender<String>) {
        self.progress_tx = Some(tx);
    }

    fn is_running(&self) -> bool {
        self.is_running
    }
}
