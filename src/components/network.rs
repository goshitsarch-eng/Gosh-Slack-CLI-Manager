use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph},
    Frame,
};
use std::fs;

use crate::app::Message;
use crate::components::Component;
use crate::ui::theme::Theme;

/// Network interface information
#[derive(Debug, Clone)]
pub struct NetworkInterface {
    pub name: String,
    pub ip_address: String,
    pub netmask: String,
    pub gateway: String,
    pub use_dhcp: bool,
    pub is_up: bool,
    pub mac_address: String,
}

/// Network Configuration Component
pub struct NetworkComponent {
    interfaces: Vec<NetworkInterface>,
    list_state: ListState,
    mode: NetworkMode,
    edit_field: usize,
    edit_buffer: String,
    is_editing: bool,
    dns_servers: Vec<String>,
    hostname: String,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum NetworkMode {
    Overview,
    EditInterface,
    DNS,
}

impl NetworkComponent {
    pub fn new() -> Self {
        let mut component = Self {
            interfaces: Vec::new(),
            list_state: ListState::default(),
            mode: NetworkMode::Overview,
            edit_field: 0,
            edit_buffer: String::new(),
            is_editing: false,
            dns_servers: Vec::new(),
            hostname: String::new(),
            status_message: None,
            show_confirm: false,
        };
        component.load_network_info();
        if !component.interfaces.is_empty() {
            component.list_state.select(Some(0));
        }
        component
    }

    fn load_network_info(&mut self) {
        self.interfaces.clear();
        self.load_interfaces();
        self.load_dns();
        self.load_hostname();
    }

    fn load_interfaces(&mut self) {
        // Read from /sys/class/net for interface list
        if let Ok(entries) = fs::read_dir("/sys/class/net") {
            for entry in entries.filter_map(|e| e.ok()) {
                let name = entry.file_name().to_string_lossy().to_string();

                // Skip loopback
                if name == "lo" {
                    continue;
                }

                let mut iface = NetworkInterface {
                    name: name.clone(),
                    ip_address: String::new(),
                    netmask: String::new(),
                    gateway: String::new(),
                    use_dhcp: true,
                    is_up: false,
                    mac_address: String::new(),
                };

                // Read MAC address
                let mac_path = format!("/sys/class/net/{}/address", name);
                if let Ok(mac) = fs::read_to_string(&mac_path) {
                    iface.mac_address = mac.trim().to_string();
                }

                // Check if interface is up
                let flags_path = format!("/sys/class/net/{}/flags", name);
                if let Ok(flags) = fs::read_to_string(&flags_path) {
                    if let Ok(flags_val) = u32::from_str_radix(flags.trim().trim_start_matches("0x"), 16) {
                        iface.is_up = flags_val & 1 != 0; // IFF_UP
                    }
                }

                // Try to get IP address using ip command
                if let Ok(output) = std::process::Command::new("ip")
                    .args(["addr", "show", &name])
                    .output()
                {
                    let stdout = String::from_utf8_lossy(&output.stdout);
                    for line in stdout.lines() {
                        let line = line.trim();
                        if line.starts_with("inet ") {
                            let parts: Vec<&str> = line.split_whitespace().collect();
                            if parts.len() >= 2 {
                                let ip_cidr = parts[1];
                                if let Some((ip, cidr)) = ip_cidr.split_once('/') {
                                    iface.ip_address = ip.to_string();
                                    iface.netmask = Self::cidr_to_netmask(cidr);
                                }
                            }
                        }
                    }
                }

                // Read from rc.inet1.conf for static config
                if let Ok(config) = fs::read_to_string("/etc/rc.d/rc.inet1.conf") {
                    iface.use_dhcp = Self::is_dhcp_enabled(&config, &name);
                    if !iface.use_dhcp {
                        if let Some(gw) = Self::get_config_value(&config, &format!("GATEWAY")) {
                            iface.gateway = gw;
                        }
                    }
                }

                self.interfaces.push(iface);
            }
        }

        self.interfaces.sort_by(|a, b| a.name.cmp(&b.name));
    }

    fn cidr_to_netmask(cidr: &str) -> String {
        let bits: u32 = cidr.parse().unwrap_or(24);
        let mask = if bits == 0 {
            0
        } else {
            !0u32 << (32 - bits)
        };
        format!(
            "{}.{}.{}.{}",
            (mask >> 24) & 255,
            (mask >> 16) & 255,
            (mask >> 8) & 255,
            mask & 255
        )
    }

    fn is_dhcp_enabled(config: &str, iface: &str) -> bool {
        // Check for USE_DHCP[n]="yes" where n is interface number
        let iface_num = if iface.starts_with("eth") {
            iface.trim_start_matches("eth")
        } else if iface.starts_with("enp") {
            // For enp* interfaces, try to find the index
            "0"
        } else {
            "0"
        };

        for line in config.lines() {
            let line = line.trim();
            if line.starts_with(&format!("USE_DHCP[{}]", iface_num)) {
                return line.contains("yes") || line.contains("YES");
            }
        }
        true // Default to DHCP
    }

    fn get_config_value(config: &str, key: &str) -> Option<String> {
        for line in config.lines() {
            let line = line.trim();
            if line.starts_with(key) && line.contains('=') {
                let value = line.split('=').nth(1)?;
                return Some(value.trim().trim_matches('"').to_string());
            }
        }
        None
    }

    fn load_dns(&mut self) {
        self.dns_servers.clear();
        if let Ok(content) = fs::read_to_string("/etc/resolv.conf") {
            for line in content.lines() {
                let line = line.trim();
                if line.starts_with("nameserver ") {
                    let server = line.trim_start_matches("nameserver ").trim();
                    self.dns_servers.push(server.to_string());
                }
            }
        }
    }

    fn load_hostname(&mut self) {
        if let Ok(hostname) = fs::read_to_string("/etc/HOSTNAME") {
            self.hostname = hostname.trim().to_string();
        } else if let Ok(hostname) = fs::read_to_string("/etc/hostname") {
            self.hostname = hostname.trim().to_string();
        }
    }

    fn selected_interface(&self) -> Option<&NetworkInterface> {
        self.list_state.selected().and_then(|i| self.interfaces.get(i))
    }

    fn restart_network(&mut self) {
        self.status_message = Some(("Restarting network...".to_string(), false));

        match std::process::Command::new("/etc/rc.d/rc.inet1")
            .arg("restart")
            .output()
        {
            Ok(output) => {
                if output.status.success() {
                    self.status_message = Some(("Network restarted successfully".to_string(), false));
                } else {
                    self.status_message = Some(("Failed to restart network".to_string(), true));
                }
            }
            Err(e) => {
                self.status_message = Some((format!("Error: {}", e), true));
            }
        }

        self.load_network_info();
    }
}

impl Component for NetworkComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    self.restart_network();
                }
                KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                    self.show_confirm = false;
                }
                _ => {}
            }
            return None;
        }

        if self.is_editing {
            match key.code {
                KeyCode::Enter | KeyCode::Esc => {
                    self.is_editing = false;
                }
                KeyCode::Backspace => {
                    self.edit_buffer.pop();
                }
                KeyCode::Char(c) => {
                    self.edit_buffer.push(c);
                }
                _ => {}
            }
            return None;
        }

        match key.code {
            KeyCode::Tab => {
                self.mode = match self.mode {
                    NetworkMode::Overview => NetworkMode::DNS,
                    NetworkMode::DNS => NetworkMode::Overview,
                    NetworkMode::EditInterface => NetworkMode::Overview,
                };
            }
            KeyCode::Up | KeyCode::Char('k') => {
                let len = match self.mode {
                    NetworkMode::Overview => self.interfaces.len(),
                    NetworkMode::DNS => self.dns_servers.len(),
                    NetworkMode::EditInterface => 4, // IP, Netmask, Gateway, DHCP
                };
                if let Some(selected) = self.list_state.selected() {
                    if selected > 0 {
                        self.list_state.select(Some(selected - 1));
                    }
                } else if len > 0 {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                let len = match self.mode {
                    NetworkMode::Overview => self.interfaces.len(),
                    NetworkMode::DNS => self.dns_servers.len(),
                    NetworkMode::EditInterface => 4,
                };
                if let Some(selected) = self.list_state.selected() {
                    if selected < len.saturating_sub(1) {
                        self.list_state.select(Some(selected + 1));
                    }
                } else if len > 0 {
                    self.list_state.select(Some(0));
                }
            }
            KeyCode::Char('r') => {
                self.show_confirm = true;
            }
            KeyCode::F(5) => {
                self.load_network_info();
                self.status_message = Some(("Network info refreshed".to_string(), false));
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

        // Mode bar
        let mode_text = match self.mode {
            NetworkMode::Overview => "[Interfaces]  DNS",
            NetworkMode::DNS => " Interfaces  [DNS]",
            NetworkMode::EditInterface => " Edit Interface ",
        };
        let mode_bar = Paragraph::new(Line::from(vec![
            Span::styled("View: ", Style::default().fg(Color::Cyan)),
            Span::raw(mode_text),
            Span::styled(
                format!("  Hostname: {}", self.hostname),
                Style::default().fg(Color::DarkGray),
            ),
        ]))
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title(" Network Configuration "),
        );
        frame.render_widget(mode_bar, chunks[0]);

        // Main content
        match self.mode {
            NetworkMode::Overview => self.render_interfaces(frame, chunks[1]),
            NetworkMode::DNS => self.render_dns(frame, chunks[1]),
            NetworkMode::EditInterface => self.render_edit(frame, chunks[1]),
        }

        // Info panel
        self.render_info(frame, chunks[2]);

        // Status bar
        let status_content = if self.show_confirm {
            Line::from(vec![
                Span::styled("Restart network? ", Style::default().fg(Color::Yellow)),
                Span::raw("[Y]es / [N]o"),
            ])
        } else if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                Style::default().fg(if *is_error { Color::Red } else { Color::Green }),
            ))
        } else {
            Line::from(Span::styled(
                "Press 'r' to restart network",
                Style::default().fg(Color::DarkGray),
            ))
        };

        let status = Paragraph::new(status_content)
            .block(Block::default().borders(Borders::ALL));
        frame.render_widget(status, chunks[3]);
    }

    fn help_text(&self) -> Vec<(&'static str, &'static str)> {
        vec![
            ("Tab", "Switch View"),
            ("r", "Restart Network"),
            ("F5", "Refresh"),
        ]
    }

    fn on_activate(&mut self) {
        self.load_network_info();
    }
}

impl NetworkComponent {
    fn render_interfaces(&self, frame: &mut Frame, area: Rect) {
        let items: Vec<ListItem> = self
            .interfaces
            .iter()
            .map(|iface| {
                let status = if iface.is_up {
                    Span::styled("UP  ", Style::default().fg(Color::Green))
                } else {
                    Span::styled("DOWN", Style::default().fg(Color::Red))
                };

                let dhcp = if iface.use_dhcp { "DHCP" } else { "Static" };

                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(
                            format!("{:<12}", iface.name),
                            Style::default().add_modifier(Modifier::BOLD),
                        ),
                        status,
                        Span::styled(format!(" {:<6}", dhcp), Style::default().fg(Color::Cyan)),
                        Span::raw(format!(" {}", iface.mac_address)),
                    ]),
                    Line::from(vec![
                        Span::styled("    IP: ", Style::default().fg(Color::DarkGray)),
                        Span::raw(if iface.ip_address.is_empty() {
                            "Not assigned".to_string()
                        } else {
                            format!("{}/{}", iface.ip_address, iface.netmask)
                        }),
                    ]),
                ])
            })
            .collect();

        let list = List::new(items)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(" Network Interfaces "),
            )
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, area, &mut state);
    }

    fn render_dns(&self, frame: &mut Frame, area: Rect) {
        let items: Vec<ListItem> = self
            .dns_servers
            .iter()
            .enumerate()
            .map(|(i, server)| {
                ListItem::new(Line::from(vec![
                    Span::styled(
                        format!("DNS {}: ", i + 1),
                        Style::default().fg(Color::Cyan),
                    ),
                    Span::raw(server),
                ]))
            })
            .collect();

        let list = if items.is_empty() {
            List::new(vec![ListItem::new(Span::styled(
                "No DNS servers configured",
                Style::default().fg(Color::DarkGray),
            ))])
        } else {
            List::new(items)
        }
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title(" DNS Servers (/etc/resolv.conf) "),
        )
        .highlight_style(Theme::list_selected())
        .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, area, &mut state);
    }

    fn render_edit(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .borders(Borders::ALL)
            .title(" Edit Interface ");
        frame.render_widget(block, area);
    }

    fn render_info(&self, frame: &mut Frame, area: Rect) {
        let block = Block::default()
            .borders(Borders::ALL)
            .title(" Network Info ");

        let inner = block.inner(area);
        frame.render_widget(block, area);

        // Get default gateway
        let gateway = std::process::Command::new("ip")
            .args(["route", "show", "default"])
            .output()
            .ok()
            .and_then(|o| {
                let stdout = String::from_utf8_lossy(&o.stdout);
                stdout
                    .lines()
                    .next()
                    .and_then(|line| {
                        line.split_whitespace()
                            .skip_while(|&w| w != "via")
                            .nth(1)
                            .map(|s| s.to_string())
                    })
            })
            .unwrap_or_else(|| "Not set".to_string());

        let info = vec![
            Line::from(vec![
                Span::styled("Default Gateway: ", Style::default().fg(Color::Cyan)),
                Span::raw(&gateway),
            ]),
            Line::from(vec![
                Span::styled("DNS Servers:     ", Style::default().fg(Color::Cyan)),
                Span::raw(if self.dns_servers.is_empty() {
                    "None".to_string()
                } else {
                    self.dns_servers.join(", ")
                }),
            ]),
            Line::from(vec![
                Span::styled("Config File:     ", Style::default().fg(Color::Cyan)),
                Span::raw("/etc/rc.d/rc.inet1.conf"),
            ]),
        ];

        let paragraph = Paragraph::new(info);
        frame.render_widget(paragraph, inner);
    }
}
