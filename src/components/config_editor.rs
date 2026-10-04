use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::Modifier,
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use tui_textarea::TextArea;

use super::Component;
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
    status_message: Option<(String, bool)>,
}

impl ConfigEditorComponent {
    pub fn is_editing(&self) -> bool {
        self.mode == EditorMode::Editing
    }

    fn selected_file_entry(&self) -> Option<(&'static str, &'static str)> {
        self.file_list_state
            .selected()
            .and_then(|i| CONFIG_FILES.get(i).copied())
    }

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
            status_message: None,
        }
    }

    pub fn load_file(&mut self, path: &str) -> Result<(), String> {
        use std::fs;

        let content =
            fs::read_to_string(path).map_err(|e| format!("Unable to open {path}: {e}"))?;

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
        if !is_error {
            self.is_modified = false;
        }
    }

    pub fn save_request(&self) -> Option<(String, String)> {
        self.current_file.as_ref().map(|path| {
            let content = format!("{}\n", self.textarea.lines().join("\n"));
            (path.clone(), content)
        })
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
                            if let Some((path, content)) = self.save_request() {
                                return Some(Message::SaveConfig { path, content });
                            }
                            return None;
                        }
                        KeyCode::Char('q') => {
                            if self.is_modified {
                                self.status_message = Some((
                                    "Unsaved changes! Ctrl+S to save, Ctrl+X to discard"
                                        .to_string(),
                                    true,
                                ));
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
                Constraint::Length(4), // Title
                Constraint::Min(10),   // Content
                Constraint::Length(3), // Status
            ])
            .split(area);

        let header = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
            .split(chunks[0]);

        let title = Paragraph::new(vec![
            Line::from(Span::styled("Configuration Editor", Theme::title())),
            Line::from(Span::styled(
                "Edit Slackware config files inside a safer, atomic-write workflow.",
                Theme::subtitle(),
            )),
        ])
        .block(Theme::panel(Theme::panel_title("Editor")));
        frame.render_widget(title, header[0]);

        let runtime = Paragraph::new(vec![
            Line::from(vec![
                Span::styled("Mode ", Theme::label()),
                Span::styled(
                    match self.mode {
                        EditorMode::FileSelect => " SELECT ",
                        EditorMode::Editing => " EDIT ",
                    },
                    Theme::badge_neutral(),
                ),
            ]),
            Line::from(vec![
                Span::styled("Modified ", Theme::label()),
                if self.is_modified {
                    Span::styled(" YES ", Theme::badge_warning())
                } else {
                    Span::styled(" NO ", Theme::badge_success())
                },
            ]),
        ])
        .block(Theme::panel_alt(Theme::panel_title("Runtime")));
        frame.render_widget(runtime, header[1]);

        match self.mode {
            EditorMode::FileSelect => {
                let content = Layout::default()
                    .direction(Direction::Horizontal)
                    .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
                    .split(chunks[1]);

                let items: Vec<ListItem> = CONFIG_FILES
                    .iter()
                    .map(|(path, desc)| {
                        ListItem::new(vec![
                            Line::from(Span::styled(
                                *path,
                                Theme::default().add_modifier(Modifier::BOLD),
                            )),
                            Line::from(Span::styled(format!("  {}", desc), Theme::muted())),
                        ])
                    })
                    .collect();

                let list = List::new(items)
                    .block(Theme::panel(Theme::panel_title("Select file to edit")))
                    .highlight_style(Theme::highlight().add_modifier(Modifier::BOLD))
                    .highlight_symbol("▸ ");

                frame.render_stateful_widget(list, content[0], &mut self.file_list_state.clone());

                let inspector_lines = if let Some((path, desc)) = self.selected_file_entry() {
                    vec![
                        Line::from(vec![
                            Span::styled("TARGET", Theme::badge_info()),
                            Span::raw(" "),
                            Span::styled(path, Theme::title()),
                        ]),
                        Line::from(""),
                        Line::from(vec![
                            Span::styled("Purpose ", Theme::label()),
                            Span::raw(desc),
                        ]),
                        Line::from(vec![
                            Span::styled("Write path ", Theme::label()),
                            Span::raw("atomic replace"),
                        ]),
                        Line::from(""),
                        Line::from(Span::styled(
                            "Open the file, edit in place, then save with Ctrl+S.",
                            Theme::subtitle(),
                        )),
                    ]
                } else {
                    vec![
                        Line::from(Span::styled("No file selected", Theme::muted())),
                        Line::from(""),
                        Line::from("Pick a managed config file to inspect before editing."),
                    ]
                };
                frame.render_widget(
                    Paragraph::new(inspector_lines)
                        .wrap(ratatui::widgets::Wrap { trim: true })
                        .block(Theme::panel_alt(Theme::panel_title("Inspector"))),
                    content[1],
                );
            }
            EditorMode::Editing => {
                let content = Layout::default()
                    .direction(Direction::Horizontal)
                    .constraints([Constraint::Percentage(70), Constraint::Percentage(30)])
                    .split(chunks[1]);

                frame.render_widget(&self.textarea, content[0]);

                let inspector_lines = vec![
                    Line::from(vec![
                        Span::styled("FILE", Theme::badge_info()),
                        Span::raw(" "),
                        Span::styled(
                            self.current_file.as_deref().unwrap_or("unknown"),
                            Theme::title(),
                        ),
                    ]),
                    Line::from(""),
                    Line::from(vec![
                        Span::styled("State ", Theme::label()),
                        if self.is_modified {
                            Span::styled(" MODIFIED ", Theme::badge_warning())
                        } else {
                            Span::styled(" CLEAN ", Theme::badge_success())
                        },
                    ]),
                    Line::from(""),
                    Line::from(Span::styled("Shortcuts", Theme::eyebrow())),
                    Line::from("Ctrl+S  save changes"),
                    Line::from("Ctrl+Q  close editor"),
                    Line::from("Ctrl+X  discard session"),
                    Line::from(""),
                    Line::from(Span::styled(
                        "Edits are written through the app's atomic file-write path.",
                        Theme::subtitle(),
                    )),
                ];
                frame.render_widget(
                    Paragraph::new(inspector_lines)
                        .wrap(ratatui::widgets::Wrap { trim: true })
                        .block(Theme::panel_alt(Theme::panel_title("Inspector"))),
                    content[1],
                );
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
                EditorMode::Editing => Paragraph::new(Line::from(vec![
                    Span::styled("Ctrl+S ", Theme::key_hint()),
                    Span::styled("save  ", Theme::muted()),
                    Span::styled("Ctrl+Q ", Theme::key_hint()),
                    Span::styled("close  ", Theme::muted()),
                    Span::styled("Ctrl+X ", Theme::key_hint()),
                    Span::styled("discard", Theme::muted()),
                ]))
                .style(Theme::muted()),
            }
        };
        frame.render_widget(
            status_text.block(Theme::panel_alt(Theme::panel_title("Status"))),
            chunks[2],
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
