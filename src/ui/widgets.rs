use ratatui::{
    layout::Rect,
    style::Modifier,
    text::{Line, Span},
    widgets::{Block, Widget},
};

use super::theme::Theme;

/// A step in a progress wizard
#[derive(Debug, Clone)]
pub struct ProgressStep {
    pub name: String,
    pub status: StepStatus,
}

#[derive(Debug, Clone, PartialEq)]
pub enum StepStatus {
    Pending,
    Running,
    Complete,
    Failed(String),
}

impl ProgressStep {
    pub fn new(name: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            status: StepStatus::Pending,
        }
    }
}

/// Widget to display a list of progress steps
pub struct ProgressList<'a> {
    steps: &'a [ProgressStep],
    block: Option<Block<'a>>,
}

impl<'a> ProgressList<'a> {
    pub fn new(steps: &'a [ProgressStep]) -> Self {
        Self { steps, block: None }
    }

    pub fn block(mut self, block: Block<'a>) -> Self {
        self.block = Some(block);
        self
    }
}

impl Widget for ProgressList<'_> {
    fn render(self, area: Rect, buf: &mut ratatui::buffer::Buffer) {
        let inner_area = if let Some(block) = &self.block {
            let inner = block.inner(area);
            block.clone().render(area, buf);
            inner
        } else {
            area
        };

        for (i, step) in self.steps.iter().enumerate() {
            if i as u16 >= inner_area.height {
                break;
            }

            let (icon, style) = match &step.status {
                StepStatus::Pending => ("○", Theme::progress_pending()),
                StepStatus::Running => ("◐", Theme::progress_running()),
                StepStatus::Complete => ("●", Theme::progress_complete()),
                StepStatus::Failed(_) => ("✗", Theme::error()),
            };

            let line = Line::from(vec![
                Span::styled(format!(" {} ", icon), style),
                Span::styled(&step.name, style),
            ]);

            let y = inner_area.y + i as u16;
            buf.set_line(inner_area.x, y, &line, inner_area.width);
        }
    }
}

/// Status bar at the bottom of the screen
pub struct StatusBar<'a> {
    message: &'a str,
    keys: Vec<(&'a str, &'a str)>,
}

impl<'a> StatusBar<'a> {
    pub fn new(message: &'a str) -> Self {
        Self {
            message,
            keys: Vec::new(),
        }
    }

    pub fn keys(mut self, keys: Vec<(&'a str, &'a str)>) -> Self {
        self.keys = keys;
        self
    }
}

impl Widget for StatusBar<'_> {
    fn render(self, area: Rect, buf: &mut ratatui::buffer::Buffer) {
        // Fill background
        for x in area.x..area.x + area.width {
            buf[(x, area.y)].set_style(Theme::status_bar());
        }

        // Build key hints
        let mut spans = Vec::new();
        for (key, desc) in &self.keys {
            spans.push(Span::styled(
                format!(" {} ", key),
                Theme::key_hint().add_modifier(Modifier::REVERSED),
            ));
            spans.push(Span::styled(format!("{} ", desc), Theme::status_bar()));
        }

        // Add message at the end
        if !self.message.is_empty() {
            spans.push(Span::styled(
                format!(" {} ", self.message),
                Theme::status_bar(),
            ));
        }

        let line = Line::from(spans);
        buf.set_line(area.x, area.y, &line, area.width);
    }
}
