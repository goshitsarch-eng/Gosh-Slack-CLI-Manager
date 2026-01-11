use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Gauge, List, ListItem, Paragraph},
    Frame,
};
use sysinfo::{CpuRefreshKind, Disks, MemoryRefreshKind, Networks, RefreshKind, System};
use std::time::Instant;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// System Information Dashboard Component
pub struct SysInfoComponent {
    system: System,
    disks: Disks,
    networks: Networks,
    last_refresh: Instant,
    selected_section: usize,
    scroll_offset: usize,
}

impl SysInfoComponent {
    pub fn new() -> Self {
        let system = System::new_with_specifics(
            RefreshKind::new()
                .with_cpu(CpuRefreshKind::everything())
                .with_memory(MemoryRefreshKind::everything()),
        );
        let disks = Disks::new_with_refreshed_list();
        let networks = Networks::new_with_refreshed_list();

        Self {
            system,
            disks,
            networks,
            last_refresh: Instant::now(),
            selected_section: 0,
            scroll_offset: 0,
        }
    }

    pub fn refresh(&mut self) {
        self.system.refresh_cpu_all();
        self.system.refresh_memory();
        self.disks.refresh();
        self.networks.refresh();
        self.last_refresh = Instant::now();
    }

    fn format_bytes(bytes: u64) -> String {
        const KB: u64 = 1024;
        const MB: u64 = KB * 1024;
        const GB: u64 = MB * 1024;
        const TB: u64 = GB * 1024;

        if bytes >= TB {
            format!("{:.2} TB", bytes as f64 / TB as f64)
        } else if bytes >= GB {
            format!("{:.2} GB", bytes as f64 / GB as f64)
        } else if bytes >= MB {
            format!("{:.2} MB", bytes as f64 / MB as f64)
        } else if bytes >= KB {
            format!("{:.2} KB", bytes as f64 / KB as f64)
        } else {
            format!("{} B", bytes)
        }
    }

    fn render_system_info(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .title(" System Information ")
            .borders(Borders::ALL)
            .border_style(if self.selected_section == 0 {
                Theme::highlight()
            } else {
                Theme::default()
            });

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let info_lines = vec![
            Line::from(vec![
                Span::styled("Hostname: ", Style::default().fg(Color::Cyan)),
                Span::raw(System::host_name().unwrap_or_else(|| "Unknown".to_string())),
            ]),
            Line::from(vec![
                Span::styled("OS: ", Style::default().fg(Color::Cyan)),
                Span::raw(format!(
                    "{} {}",
                    System::name().unwrap_or_else(|| "Unknown".to_string()),
                    System::os_version().unwrap_or_else(|| "".to_string())
                )),
            ]),
            Line::from(vec![
                Span::styled("Kernel: ", Style::default().fg(Color::Cyan)),
                Span::raw(System::kernel_version().unwrap_or_else(|| "Unknown".to_string())),
            ]),
            Line::from(vec![
                Span::styled("Uptime: ", Style::default().fg(Color::Cyan)),
                Span::raw(Self::format_uptime(System::uptime())),
            ]),
            Line::from(vec![
                Span::styled("Architecture: ", Style::default().fg(Color::Cyan)),
                Span::raw(System::cpu_arch().unwrap_or_else(|| "Unknown".to_string())),
            ]),
        ];

        let paragraph = Paragraph::new(info_lines);
        frame.render_widget(paragraph, inner);
    }

    fn format_uptime(seconds: u64) -> String {
        let days = seconds / 86400;
        let hours = (seconds % 86400) / 3600;
        let minutes = (seconds % 3600) / 60;

        if days > 0 {
            format!("{}d {}h {}m", days, hours, minutes)
        } else if hours > 0 {
            format!("{}h {}m", hours, minutes)
        } else {
            format!("{}m", minutes)
        }
    }

    fn render_cpu(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .title(" CPU ")
            .borders(Borders::ALL)
            .border_style(if self.selected_section == 1 {
                Theme::highlight()
            } else {
                Theme::default()
            });

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let cpus = self.system.cpus();
        let overall_usage: f32 = if !cpus.is_empty() {
            cpus.iter().map(|cpu| cpu.cpu_usage()).sum::<f32>() / cpus.len() as f32
        } else {
            0.0
        };

        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Length(2), Constraint::Min(1)])
            .split(inner);

        // Overall CPU gauge
        let gauge = Gauge::default()
            .block(Block::default())
            .gauge_style(Style::default().fg(Self::usage_color(overall_usage as u16)))
            .percent(overall_usage as u16)
            .label(format!("Overall: {:.1}%", overall_usage));
        frame.render_widget(gauge, chunks[0]);

        // Per-core display
        let cpu_items: Vec<ListItem> = cpus
            .iter()
            .enumerate()
            .skip(self.scroll_offset)
            .take(chunks[1].height as usize)
            .map(|(i, cpu)| {
                let usage = cpu.cpu_usage();
                let bar_width = 20;
                let filled = (usage / 100.0 * bar_width as f32) as usize;
                let bar = format!(
                    "[{}{}]",
                    "█".repeat(filled),
                    "░".repeat(bar_width - filled)
                );
                ListItem::new(Line::from(vec![
                    Span::styled(format!("CPU{:2}: ", i), Style::default().fg(Color::Cyan)),
                    Span::styled(bar, Style::default().fg(Self::usage_color(usage as u16))),
                    Span::raw(format!(" {:5.1}%", usage)),
                ]))
            })
            .collect();

        let list = List::new(cpu_items);
        frame.render_widget(list, chunks[1]);
    }

    fn usage_color(percent: u16) -> Color {
        if percent >= 90 {
            Color::Red
        } else if percent >= 70 {
            Color::Yellow
        } else {
            Color::Green
        }
    }

    fn render_memory(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .title(" Memory ")
            .borders(Borders::ALL)
            .border_style(if self.selected_section == 2 {
                Theme::highlight()
            } else {
                Theme::default()
            });

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let total_mem = self.system.total_memory();
        let used_mem = self.system.used_memory();
        let total_swap = self.system.total_swap();
        let used_swap = self.system.used_swap();

        let mem_percent = if total_mem > 0 {
            (used_mem as f64 / total_mem as f64 * 100.0) as u16
        } else {
            0
        };

        let swap_percent = if total_swap > 0 {
            (used_swap as f64 / total_swap as f64 * 100.0) as u16
        } else {
            0
        };

        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Length(3), Constraint::Length(3)])
            .split(inner);

        // RAM gauge
        let ram_gauge = Gauge::default()
            .block(Block::default().title("RAM"))
            .gauge_style(Style::default().fg(Self::usage_color(mem_percent)))
            .percent(mem_percent)
            .label(format!(
                "{} / {} ({:.1}%)",
                Self::format_bytes(used_mem),
                Self::format_bytes(total_mem),
                mem_percent
            ));
        frame.render_widget(ram_gauge, chunks[0]);

        // Swap gauge
        let swap_gauge = Gauge::default()
            .block(Block::default().title("Swap"))
            .gauge_style(Style::default().fg(Self::usage_color(swap_percent)))
            .percent(swap_percent)
            .label(format!(
                "{} / {} ({:.1}%)",
                Self::format_bytes(used_swap),
                Self::format_bytes(total_swap),
                swap_percent
            ));
        frame.render_widget(swap_gauge, chunks[1]);
    }

    fn render_network(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .title(" Network ")
            .borders(Borders::ALL)
            .border_style(if self.selected_section == 3 {
                Theme::highlight()
            } else {
                Theme::default()
            });

        let inner = block.inner(area);
        frame.render_widget(block, area);

        let items: Vec<ListItem> = self
            .networks
            .iter()
            .map(|(name, data)| {
                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            format!("{}: ", name),
                            Style::default()
                                .fg(Color::Cyan)
                                .add_modifier(Modifier::BOLD),
                        ),
                    ]),
                    Line::from(vec![
                        Span::styled("  ↓ ", Style::default().fg(Color::Green)),
                        Span::raw(format!(
                            "{}/s  ",
                            Self::format_bytes(data.received())
                        )),
                        Span::styled("↑ ", Style::default().fg(Color::Red)),
                        Span::raw(format!(
                            "{}/s",
                            Self::format_bytes(data.transmitted())
                        )),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items);
        frame.render_widget(list, inner);
    }

    fn render_processes(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .title(" Top Processes ")
            .borders(Borders::ALL)
            .border_style(if self.selected_section == 4 {
                Theme::highlight()
            } else {
                Theme::default()
            });

        let inner = block.inner(area);
        frame.render_widget(block, area);

        // Get process info
        let mut sys = System::new();
        sys.refresh_processes(sysinfo::ProcessesToUpdate::All, true);

        let mut processes: Vec<_> = sys.processes().values().collect();
        processes.sort_by(|a, b| {
            b.cpu_usage()
                .partial_cmp(&a.cpu_usage())
                .unwrap_or(std::cmp::Ordering::Equal)
        });

        let items: Vec<ListItem> = processes
            .iter()
            .take(inner.height as usize)
            .map(|p| {
                ListItem::new(Line::from(vec![
                    Span::styled(
                        format!("{:>6} ", p.pid()),
                        Style::default().fg(Color::DarkGray),
                    ),
                    Span::styled(
                        format!("{:>5.1}% ", p.cpu_usage()),
                        Style::default().fg(Self::usage_color(p.cpu_usage() as u16)),
                    ),
                    Span::styled(
                        format!("{:>8} ", Self::format_bytes(p.memory())),
                        Style::default().fg(Color::Yellow),
                    ),
                    Span::raw(p.name().to_string_lossy().chars().take(30).collect::<String>()),
                ]))
            })
            .collect();

        let list = List::new(items);
        frame.render_widget(list, inner);
    }
}

impl Component for SysInfoComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        match key.code {
            KeyCode::Tab => {
                self.selected_section = (self.selected_section + 1) % 5;
                self.scroll_offset = 0;
            }
            KeyCode::BackTab => {
                self.selected_section = if self.selected_section == 0 {
                    4
                } else {
                    self.selected_section - 1
                };
                self.scroll_offset = 0;
            }
            KeyCode::Up | KeyCode::Char('k') => {
                if self.scroll_offset > 0 {
                    self.scroll_offset -= 1;
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                self.scroll_offset += 1;
            }
            KeyCode::Char('r') => {
                self.refresh();
            }
            _ => {}
        }
        None
    }

    fn render(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(7),  // System info
                Constraint::Min(10),    // CPU + Memory
                Constraint::Length(10), // Network + Processes
            ])
            .split(area);

        self.render_system_info(frame, chunks[0]);

        let middle = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(60), Constraint::Percentage(40)])
            .split(chunks[1]);

        self.render_cpu(frame, middle[0]);
        self.render_memory(frame, middle[1]);

        let bottom = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(40), Constraint::Percentage(60)])
            .split(chunks[2]);

        self.render_network(frame, bottom[0]);
        self.render_processes(frame, bottom[1]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("Tab", "Next Section"),
            ("↑/↓", "Scroll"),
            ("r", "Refresh"),
        ]
    }

    fn on_activate(&mut self) {
        self.refresh();
    }
}
