use ratatui::{
    layout::Rect,
    style::Modifier,
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph, Widget},
    Frame,
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

    pub fn running(mut self) -> Self {
        self.status = StepStatus::Running;
        self
    }

    pub fn complete(mut self) -> Self {
        self.status = StepStatus::Complete;
        self
    }

    pub fn failed(mut self, error: impl Into<String>) -> Self {
        self.status = StepStatus::Failed(error.into());
        self
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

/// Render a confirmation dialog
pub fn render_confirm_dialog(
    frame: &mut Frame,
    area: Rect,
    title: &str,
    message: &str,
    confirm_selected: bool,
) {
    let block = Block::default()
        .title(title)
        .borders(Borders::ALL)
        .border_style(Theme::border_focused());

    let inner = block.inner(area);
    frame.render_widget(block, area);

    // Message
    let msg = Paragraph::new(message).style(Theme::default());
    let msg_area = Rect {
        x: inner.x + 1,
        y: inner.y + 1,
        width: inner.width.saturating_sub(2),
        height: inner.height.saturating_sub(3),
    };
    frame.render_widget(msg, msg_area);

    // Buttons
    let button_y = inner.y + inner.height - 2;
    let confirm_style = if confirm_selected {
        Theme::selected()
    } else {
        Theme::default()
    };
    let cancel_style = if !confirm_selected {
        Theme::selected()
    } else {
        Theme::default()
    };

    let buttons = Line::from(vec![
        Span::raw("  "),
        Span::styled(" Yes ", confirm_style),
        Span::raw("  "),
        Span::styled(" No ", cancel_style),
    ]);

    frame.render_widget(
        Paragraph::new(buttons),
        Rect {
            x: inner.x + (inner.width / 2).saturating_sub(8),
            y: button_y,
            width: 20,
            height: 1,
        },
    );
}

/// Render an output panel for command output
pub fn render_output_panel(frame: &mut Frame, area: Rect, title: &str, lines: &[String]) {
    let block = Block::default()
        .title(title)
        .borders(Borders::ALL)
        .border_style(Theme::border());

    let inner = block.inner(area);
    frame.render_widget(block, area);

    // Show last N lines that fit
    let visible_lines = inner.height as usize;
    let start = lines.len().saturating_sub(visible_lines);
    let display_lines: Vec<Line> = lines[start..]
        .iter()
        .map(|s| Line::from(s.as_str()))
        .collect();

    let paragraph = Paragraph::new(display_lines).style(Theme::muted());
    frame.render_widget(paragraph, inner);
}
