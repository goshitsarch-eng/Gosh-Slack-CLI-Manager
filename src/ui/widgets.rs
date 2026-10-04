use ratatui::{
    layout::Rect,
    text::{Line, Span},
    widgets::{Block, Paragraph, Widget, Wrap},
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

        for y in inner_area.top()..inner_area.bottom() {
            for x in inner_area.left()..inner_area.right() {
                buf[(x, y)].set_style(Theme::surface());
            }
        }

        for (i, step) in self.steps.iter().enumerate() {
            if i as u16 >= inner_area.height {
                break;
            }

            let (icon, style, detail) = match &step.status {
                StepStatus::Pending => ("·", Theme::progress_pending(), "Queued"),
                StepStatus::Running => ("◆", Theme::progress_running(), "Running"),
                StepStatus::Complete => ("■", Theme::progress_complete(), "Done"),
                StepStatus::Failed(_) => ("×", Theme::error(), "Failed"),
            };

            let line = Line::from(vec![
                Span::styled(" ", Theme::surface()),
                Span::styled(format!(" {} ", icon), style),
                Span::styled(&step.name, style),
                Span::styled("  ", Theme::surface()),
                Span::styled(detail, Theme::muted()),
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
        if area.is_empty() {
            return;
        }
        for y in area.top()..area.bottom() {
            for x in area.left()..area.right() {
                buf[(x, y)].set_style(Theme::status_bar());
            }
        }

        let mut spans = Vec::new();
        if !self.keys.is_empty() {
            spans.push(Span::styled(" CONTROLS ", Theme::badge_info()));
            spans.push(Span::styled("  ", Theme::status_bar()));
        }

        for (key, desc) in &self.keys {
            spans.push(Span::styled(format!(" {} ", key), Theme::key_hint()));
            spans.push(Span::styled(" ", Theme::status_bar()));
            spans.push(Span::styled(desc.to_string(), Theme::status_bar()));
            spans.push(Span::styled("  •  ", Theme::muted()));
        }

        if !self.keys.is_empty() {
            spans.pop();
        }

        if self.keys.is_empty() && !self.message.is_empty() {
            if !spans.is_empty() {
                spans.push(Span::styled("  //  ", Theme::muted()));
            }
            spans.push(Span::styled(self.message, Theme::status_bar()));
        }

        let line = Line::from(spans);
        Paragraph::new(line)
            .wrap(Wrap { trim: true })
            .render(area, buf);
    }
}
