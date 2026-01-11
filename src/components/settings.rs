use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

const CONFIG_DIR: &str = "/etc/slackware-cli-manager";
const CONFIG_FILE: &str = "config.toml";

/// Application settings
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AppSettings {
    pub theme: ThemeChoice,
    pub confirm_actions: bool,
    pub show_hidden_files: bool,
    pub auto_refresh: bool,
    pub refresh_interval: u32,
    pub default_tab: String,
    pub log_lines: usize,
}

impl Default for AppSettings {
    fn default() -> Self {
        Self {
            theme: ThemeChoice::Default,
            confirm_actions: true,
            show_hidden_files: false,
            auto_refresh: false,
            refresh_interval: 5,
            default_tab: "updater".to_string(),
            log_lines: 1000,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub enum ThemeChoice {
    Default,
    Dark,
    Light,
    Solarized,
    Nord,
    Dracula,
}

impl ThemeChoice {
    pub fn all() -> Vec<ThemeChoice> {
        vec![
            ThemeChoice::Default,
            ThemeChoice::Dark,
            ThemeChoice::Light,
            ThemeChoice::Solarized,
            ThemeChoice::Nord,
            ThemeChoice::Dracula,
        ]
    }

    pub fn name(&self) -> &'static str {
        match self {
            ThemeChoice::Default => "Default",
            ThemeChoice::Dark => "Dark",
            ThemeChoice::Light => "Light",
            ThemeChoice::Solarized => "Solarized",
            ThemeChoice::Nord => "Nord",
            ThemeChoice::Dracula => "Dracula",
        }
    }

    pub fn colors(&self) -> ThemeColors {
        match self {
            ThemeChoice::Default => ThemeColors {
                primary: Color::Cyan,
                secondary: Color::Yellow,
                success: Color::Green,
                error: Color::Red,
                warning: Color::Yellow,
                muted: Color::DarkGray,
                background: Color::Reset,
                foreground: Color::White,
            },
            ThemeChoice::Dark => ThemeColors {
                primary: Color::Blue,
                secondary: Color::Magenta,
                success: Color::Green,
                error: Color::Red,
                warning: Color::Yellow,
                muted: Color::DarkGray,
                background: Color::Rgb(30, 30, 30),
                foreground: Color::White,
            },
            ThemeChoice::Light => ThemeColors {
                primary: Color::Blue,
                secondary: Color::Magenta,
                success: Color::Green,
                error: Color::Red,
                warning: Color::Rgb(200, 150, 0),
                muted: Color::Gray,
                background: Color::White,
                foreground: Color::Black,
            },
            ThemeChoice::Solarized => ThemeColors {
                primary: Color::Rgb(38, 139, 210),   // Blue
                secondary: Color::Rgb(211, 54, 130), // Magenta
                success: Color::Rgb(133, 153, 0),    // Green
                error: Color::Rgb(220, 50, 47),      // Red
                warning: Color::Rgb(181, 137, 0),    // Yellow
                muted: Color::Rgb(88, 110, 117),     // Base01
                background: Color::Rgb(0, 43, 54),   // Base03
                foreground: Color::Rgb(131, 148, 150), // Base0
            },
            ThemeChoice::Nord => ThemeColors {
                primary: Color::Rgb(136, 192, 208),  // Nord8
                secondary: Color::Rgb(180, 142, 173), // Nord15
                success: Color::Rgb(163, 190, 140),  // Nord14
                error: Color::Rgb(191, 97, 106),     // Nord11
                warning: Color::Rgb(235, 203, 139),  // Nord13
                muted: Color::Rgb(76, 86, 106),      // Nord3
                background: Color::Rgb(46, 52, 64),  // Nord0
                foreground: Color::Rgb(236, 239, 244), // Nord6
            },
            ThemeChoice::Dracula => ThemeColors {
                primary: Color::Rgb(139, 233, 253),  // Cyan
                secondary: Color::Rgb(255, 121, 198), // Pink
                success: Color::Rgb(80, 250, 123),   // Green
                error: Color::Rgb(255, 85, 85),      // Red
                warning: Color::Rgb(241, 250, 140),  // Yellow
                muted: Color::Rgb(98, 114, 164),     // Comment
                background: Color::Rgb(40, 42, 54),  // Background
                foreground: Color::Rgb(248, 248, 242), // Foreground
            },
        }
    }
}

#[derive(Debug, Clone, Copy)]
pub struct ThemeColors {
    pub primary: Color,
    pub secondary: Color,
    pub success: Color,
    pub error: Color,
    pub warning: Color,
    pub muted: Color,
    pub background: Color,
    pub foreground: Color,
}

/// Settings section
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum SettingsSection {
    Theme,
    Behavior,
    Display,
}

/// Settings Component
pub struct SettingsComponent {
    settings: AppSettings,
    list_state: ListState,
    section: SettingsSection,
    editing: bool,
    status_message: Option<(String, bool)>,
    unsaved_changes: bool,
}

impl SettingsComponent {
    pub fn new() -> Self {
        let settings = Self::load_settings();
        Self {
            settings,
            list_state: ListState::default().with_selected(Some(0)),
            section: SettingsSection::Theme,
            editing: false,
            status_message: None,
            unsaved_changes: false,
        }
    }

    fn config_path() -> PathBuf {
        PathBuf::from(CONFIG_DIR).join(CONFIG_FILE)
    }

    fn load_settings() -> AppSettings {
        let path = Self::config_path();
        if path.exists() {
            if let Ok(content) = fs::read_to_string(&path) {
                if let Ok(settings) = toml::from_str(&content) {
                    return settings;
                }
            }
        }
        AppSettings::default()
    }

    fn save_settings(&mut self) -> bool {
        // Ensure config directory exists
        if let Err(e) = fs::create_dir_all(CONFIG_DIR) {
            self.status_message = Some((format!("Failed to create config dir: {}", e), true));
            return false;
        }

        let path = Self::config_path();
        match toml::to_string_pretty(&self.settings) {
            Ok(content) => {
                if let Err(e) = fs::write(&path, content) {
                    self.status_message = Some((format!("Failed to save: {}", e), true));
                    return false;
                }
                self.unsaved_changes = false;
                self.status_message = Some(("Settings saved".to_string(), false));
                true
            }
            Err(e) => {
                self.status_message = Some((format!("Serialization error: {}", e), true));
                false
            }
        }
    }

    fn get_section_items(&self) -> Vec<(&'static str, String, bool)> {
        match self.section {
            SettingsSection::Theme => {
                vec![(
                    "Color Theme",
                    self.settings.theme.name().to_string(),
                    true,
                )]
            }
            SettingsSection::Behavior => {
                vec![
                    (
                        "Confirm Actions",
                        if self.settings.confirm_actions {
                            "Yes"
                        } else {
                            "No"
                        }
                        .to_string(),
                        true,
                    ),
                    (
                        "Auto Refresh",
                        if self.settings.auto_refresh {
                            "Yes"
                        } else {
                            "No"
                        }
                        .to_string(),
                        true,
                    ),
                    (
                        "Refresh Interval",
                        format!("{} seconds", self.settings.refresh_interval),
                        self.settings.auto_refresh,
                    ),
                ]
            }
            SettingsSection::Display => {
                vec![
                    (
                        "Show Hidden Files",
                        if self.settings.show_hidden_files {
                            "Yes"
                        } else {
                            "No"
                        }
                        .to_string(),
                        true,
                    ),
                    (
                        "Log Buffer Size",
                        format!("{} lines", self.settings.log_lines),
                        true,
                    ),
                    ("Default Tab", self.settings.default_tab.clone(), true),
                ]
            }
        }
    }

    fn cycle_current_option(&mut self, forward: bool) {
        let items = self.get_section_items();
        let selected = self.list_state.selected().unwrap_or(0);

        if selected >= items.len() {
            return;
        }

        let (name, _, enabled) = &items[selected];
        if !enabled {
            return;
        }

        match self.section {
            SettingsSection::Theme => {
                let themes = ThemeChoice::all();
                let current_idx = themes
                    .iter()
                    .position(|t| *t == self.settings.theme)
                    .unwrap_or(0);
                let new_idx = if forward {
                    (current_idx + 1) % themes.len()
                } else {
                    (current_idx + themes.len() - 1) % themes.len()
                };
                self.settings.theme = themes[new_idx];
            }
            SettingsSection::Behavior => {
                match *name {
                    "Confirm Actions" => {
                        self.settings.confirm_actions = !self.settings.confirm_actions;
                    }
                    "Auto Refresh" => {
                        self.settings.auto_refresh = !self.settings.auto_refresh;
                    }
                    "Refresh Interval" => {
                        if forward {
                            self.settings.refresh_interval =
                                (self.settings.refresh_interval + 1).min(60);
                        } else {
                            self.settings.refresh_interval =
                                self.settings.refresh_interval.saturating_sub(1).max(1);
                        }
                    }
                    _ => {}
                }
            }
            SettingsSection::Display => {
                match *name {
                    "Show Hidden Files" => {
                        self.settings.show_hidden_files = !self.settings.show_hidden_files;
                    }
                    "Log Buffer Size" => {
                        if forward {
                            self.settings.log_lines = (self.settings.log_lines + 100).min(10000);
                        } else {
                            self.settings.log_lines =
                                self.settings.log_lines.saturating_sub(100).max(100);
                        }
                    }
                    "Default Tab" => {
                        let tabs = [
                            "updater",
                            "sbotools",
                            "user_setup",
                            "mirror",
                            "packages",
                            "config",
                        ];
                        let current_idx = tabs
                            .iter()
                            .position(|&t| t == self.settings.default_tab)
                            .unwrap_or(0);
                        let new_idx = if forward {
                            (current_idx + 1) % tabs.len()
                        } else {
                            (current_idx + tabs.len() - 1) % tabs.len()
                        };
                        self.settings.default_tab = tabs[new_idx].to_string();
                    }
                    _ => {}
                }
            }
        }

        self.unsaved_changes = true;
    }

    pub fn get_theme(&self) -> ThemeChoice {
        self.settings.theme
    }
}

impl Component for SettingsComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        let items_len = self.get_section_items().len();

        match key.code {
            KeyCode::Tab => {
                self.section = match self.section {
                    SettingsSection::Theme => SettingsSection::Behavior,
                    SettingsSection::Behavior => SettingsSection::Display,
                    SettingsSection::Display => SettingsSection::Theme,
                };
                self.list_state.select(Some(0));
            }
            KeyCode::Up | KeyCode::Char('k') => {
                if let Some(selected) = self.list_state.selected() {
                    if selected > 0 {
                        self.list_state.select(Some(selected - 1));
                    }
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                if let Some(selected) = self.list_state.selected() {
                    if selected < items_len.saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                }
            }
            KeyCode::Left | KeyCode::Char('h') => {
                self.cycle_current_option(false);
            }
            KeyCode::Right | KeyCode::Char('l') | KeyCode::Enter => {
                self.cycle_current_option(true);
            }
            KeyCode::Char('s') => {
                self.save_settings();
            }
            KeyCode::Char('r') => {
                self.settings = AppSettings::default();
                self.unsaved_changes = true;
                self.status_message = Some(("Settings reset to defaults".to_string(), false));
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
                Constraint::Length(8),
                Constraint::Length(3),
            ])
            .split(area);

        // Section tabs
        let section_text = match self.section {
            SettingsSection::Theme => "[Theme]  Behavior  Display",
            SettingsSection::Behavior => " Theme  [Behavior]  Display",
            SettingsSection::Display => " Theme   Behavior  [Display]",
        };

        let section_bar = Paragraph::new(Line::from(vec![
            Span::styled("Section: ", Style::default().fg(Color::Cyan)),
            Span::raw(section_text),
            if self.unsaved_changes {
                Span::styled(" (unsaved)", Style::default().fg(Color::Yellow))
            } else {
                Span::raw("")
            },
        ]))
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title(" Settings "),
        );
        frame.render_widget(section_bar, chunks[0]);

        // Settings list
        let items: Vec<ListItem> = self
            .get_section_items()
            .iter()
            .map(|(name, value, enabled)| {
                let style = if *enabled {
                    Style::default()
                } else {
                    Style::default().fg(Color::DarkGray)
                };

                ListItem::new(Line::from(vec![
                    Span::styled(format!("{:<20}", name), style),
                    Span::styled(
                        format!("< {} >", value),
                        if *enabled {
                            Style::default().fg(Color::Cyan)
                        } else {
                            style
                        },
                    ),
                ]))
            })
            .collect();

        let list = List::new(items)
            .block(Block::default().borders(Borders::ALL))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, chunks[1], &mut state);

        // Theme preview
        self.render_theme_preview(frame, chunks[2]);

        // Status bar
        let status_content = if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                Style::default().fg(if *is_error { Color::Red } else { Color::Green }),
            ))
        } else {
            Line::from(Span::styled(
                "Use ←/→ to change values, 's' to save, 'r' to reset",
                Style::default().fg(Color::DarkGray),
            ))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
        frame.render_widget(status, chunks[3]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("Tab", "Section"),
            ("←/→", "Change"),
            ("s", "Save"),
            ("r", "Reset"),
        ]
    }
}

impl SettingsComponent {
    fn render_theme_preview(&self, frame: &mut Frame, area: Rect) {
        let colors = self.settings.theme.colors();

        let block = Block::default()
            .borders(Borders::ALL)
            .title(format!(" {} Theme Preview ", self.settings.theme.name()))
            .border_style(Style::default().fg(colors.primary));

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let preview = vec![
            Line::from(vec![
                Span::styled("Primary ", Style::default().fg(colors.primary)),
                Span::styled("Secondary ", Style::default().fg(colors.secondary)),
                Span::styled("Success ", Style::default().fg(colors.success)),
                Span::styled("Error ", Style::default().fg(colors.error)),
            ]),
            Line::from(vec![
                Span::styled("Warning ", Style::default().fg(colors.warning)),
                Span::styled("Muted ", Style::default().fg(colors.muted)),
                Span::styled(
                    "Selected ",
                    Style::default()
                        .fg(colors.foreground)
                        .bg(colors.primary),
                ),
            ]),
            Line::from(Span::styled(
                "The quick brown fox jumps over the lazy dog",
                Style::default().fg(colors.foreground),
            )),
        ];

        let paragraph = Paragraph::new(preview);
        frame.render_widget(paragraph, inner);
    }
}
