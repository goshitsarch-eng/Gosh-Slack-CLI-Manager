use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::Modifier,
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use tokio::sync::mpsc;
use tui_textarea::TextArea;

use super::{AsyncComponent, Component};
use crate::app::Message;
use crate::ui::theme::Theme;

/// Available config files to edit
const CONFIG_FILES: [(&str, &str); 3] = [
    ("/etc/slackpkg/slackpkg.conf", "slackpkg configuration"),
    ("/etc/slackpkg/mirrors", "Package mirrors"),
    ("/etc/sbotools/sbotools.conf", "sbotools configuration"),
];

#[derive(Debug, Clone, Copy, PartialEq)]
enum EditorMode {
    FileSelect,
    Editing,
}

/// Config file editor component
pub struct ConfigEditorComponent {
    mode: EditorMode,
    file_list_state: ListState,
    current_file: Option<String>,
    textarea: TextArea<'static>,
    is_modified: bool,
    is_saving: bool,
    status_message: Option<(String, bool)>,
    progress_tx: Option<mpsc::UnboundedSender<String>>,
}

impl ConfigEditorComponent {
    pub fn new() -> Self {
        let mut textarea = TextArea::default();
        textarea.set_block(
            Block::default()
                .borders(Borders::ALL)
                .title("Editor")
                .border_style(Theme::border()),
        );

        Self {
            mode: EditorMode::FileSelect,
            file_list_state: ListState::default().with_selected(Some(0)),
            current_file: None,
            textarea,
            is_modified: false,
            is_saving: false,
            status_message: None,
            progress_tx: None,
        }
    }

    pub fn load_file(&mut self, path: &str) -> Result<(), String> {
        use std::fs;

        let content = fs::read_to_string(path).map_err(|e| e.to_string())?;

        self.textarea = TextArea::from(content.lines());
        self.textarea.set_block(
            Block::default()
                .borders(Borders::ALL)
                .title(format!("Editing: {}", path))
                .border_style(Theme::border_focused()),
        );

        self.current_file = Some(path.to_string());
        self.mode = EditorMode::Editing;
        self.is_modified = false;
        self.status_message = None;

        Ok(())
    }

    pub fn save_file(&mut self) -> Result<(), String> {
        use std::fs;

        if let Some(ref path) = self.current_file {
            let content = self.textarea.lines().join("\n");
            fs::write(path, content + "\n").map_err(|e| e.to_string())?;
            self.is_modified = false;
            self.status_message = Some(("File saved successfully".to_string(), false));
        }

        Ok(())
    }

    pub fn close_editor(&mut self) {
        self.mode = EditorMode::FileSelect;
        self.current_file = None;
        self.is_modified = false;
        self.textarea = TextArea::default();
        self.textarea.set_block(
            Block::default()
                .borders(Borders::ALL)
                .title("Editor")
                .border_style(Theme::border()),
        );
    }

    pub fn get_selected_file(&self) -> Option<&str> {
        self.file_list_state
            .selected()
            .and_then(|i| CONFIG_FILES.get(i))
            .map(|(path, _)| *path)
    }

    pub fn set_status(&mut self, message: String, is_error: bool) {
        self.status_message = Some((message, is_error));
        self.is_saving = false;
    }
}

impl Default for ConfigEditorComponent {
    fn default() -> Self {
        Self::new()
    }
}

impl Component for ConfigEditorComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        match self.mode {
            EditorMode::FileSelect => match key.code {
                KeyCode::Up | KeyCode::Char('k') => {
                    if let Some(selected) = self.file_list_state.selected() {
                        if selected > 0 {
                            self.file_list_state.select(Some(selected - 1));
                        }
                    }
                    None
                }
                KeyCode::Down | KeyCode::Char('j') => {
                    if let Some(selected) = self.file_list_state.selected() {
                        if selected < CONFIG_FILES.len() - 1 {
                            self.file_list_state.select(Some(selected + 1));
                        }
                    }
                    None
                }
                KeyCode::Enter => {
                    if let Some(path) = self.get_selected_file() {
                        let path = path.to_string();
                        if let Err(e) = self.load_file(&path) {
                            self.status_message = Some((format!("Error: {}", e), true));
                        }
                    }
                    None
                }
                _ => None,
            },
            EditorMode::Editing => {
                // Handle editor-specific keys
                if key.modifiers.contains(KeyModifiers::CONTROL) {
                    match key.code {
                        KeyCode::Char('s') => {
                            if let Err(e) = self.save_file() {
                                self.status_message = Some((format!("Save error: {}", e), true));
                            }
                            return None;
                        }
                        KeyCode::Char('q') => {
                            if self.is_modified {
                                self.status_message =
                                    Some(("Unsaved changes! Ctrl+S to save, Ctrl+X to discard".to_string(), true));
                            } else {
                                self.close_editor();
                            }
                            return None;
                        }
                        KeyCode::Char('x') => {
                            // Force close without saving
                            self.close_editor();
                            return None;
                        }
                        _ => {}
                    }
                }

                // Pass to textarea
                if self.textarea.input(key) {
                    self.is_modified = true;
                }
                None
            }
        }
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(3), // Title
                Constraint::Min(10),   // Content
                Constraint::Length(3), // Status
            ])
            .split(area);

        // Title
        let title = Paragraph::new(Line::from(vec![Span::styled(
            "Configuration Editor",
            Theme::title(),
        )]))
        .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(title, chunks[0]);

        match self.mode {
            EditorMode::FileSelect => {
                // File list
                let items: Vec<ListItem> = CONFIG_FILES
                    .iter()
                    .map(|(path, desc)| {
                        ListItem::new(Line::from(vec![
                            Span::styled(*path, Theme::default().add_modifier(Modifier::BOLD)),
                            Span::styled(format!(" - {}", desc), Theme::muted()),
                        ]))
                    })
                    .collect();

                let list = List::new(items)
                    .block(
                        Block::default()
                            .borders(Borders::ALL)
                            .title("Select file to edit"),
                    )
                    .highlight_style(Theme::highlight().add_modifier(Modifier::BOLD))
                    .highlight_symbol("→ ");

                frame.render_stateful_widget(list, chunks[1], &mut self.file_list_state.clone());
            }
            EditorMode::Editing => {
                // Text editor
                frame.render_widget(&self.textarea, chunks[1]);
            }
        }

        // Status
        let status_text = if let Some((ref msg, is_error)) = self.status_message {
            Paragraph::new(msg.as_str()).style(if is_error {
                Theme::error()
            } else {
                Theme::success()
            })
        } else {
            match self.mode {
                EditorMode::FileSelect => {
                    Paragraph::new("Press Enter to edit file").style(Theme::muted())
                }
                EditorMode::Editing => {
                    let modified = if self.is_modified { " [Modified]" } else { "" };
                    Paragraph::new(format!(
                        "Ctrl+S: Save  Ctrl+Q: Close  Ctrl+X: Discard{}",
                        modified
                    ))
                    .style(Theme::muted())
                }
            }
        };
        frame.render_widget(
            status_text.block(Block::default().borders(Borders::TOP)),
            chunks[3],
        );
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        match self.mode {
            EditorMode::FileSelect => vec![("↑/↓", "Navigate"), ("Enter", "Edit")],
            EditorMode::Editing => vec![
                ("Ctrl+S", "Save"),
                ("Ctrl+Q", "Close"),
                ("Ctrl+X", "Discard"),
            ],
        }
    }
}

impl AsyncComponent for ConfigEditorComponent {
    fn set_progress_channel(&mut self, tx: mpsc::UnboundedSender<String>) {
        self.progress_tx = Some(tx);
    }

    fn is_running(&self) -> bool {
        self.is_saving
    }
}
