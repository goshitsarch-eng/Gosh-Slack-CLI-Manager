use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    text::{Line, Span},
    widgets::{List, ListItem, ListState, Paragraph},
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
    dns_servers: Vec<String>,
    default_gateway: String,
    hostname: String,
    status_message: Option<(String, bool)>,
    show_confirm: bool,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum NetworkMode {
    Overview,
    Dns,
}

impl NetworkComponent {
    pub fn new() -> Self {
        let mut component = Self {
            interfaces: Vec::new(),
            list_state: ListState::default(),
            mode: NetworkMode::Overview,
            dns_servers: Vec::new(),
            default_gateway: "Not set".to_string(),
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
        self.load_default_gateway();
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
                    if let Ok(flags_val) =
                        u32::from_str_radix(flags.trim().trim_start_matches("0x"), 16)
                    {
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
                        if let Some(gw) = Self::get_config_value(&config, "GATEWAY") {
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
        let mask = if bits == 0 { 0 } else { !0u32 << (32 - bits) };
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

    fn load_default_gateway(&mut self) {
        self.default_gateway = std::process::Command::new("ip")
            .args(["route", "show", "default"])
            .output()
            .ok()
            .and_then(|output| {
                let stdout = String::from_utf8_lossy(&output.stdout);
                stdout.lines().next().and_then(|line| {
                    line.split_whitespace()
                        .skip_while(|word| *word != "via")
                        .nth(1)
                        .map(str::to_string)
                })
            })
            .unwrap_or_else(|| "Not set".to_string());
    }

    pub fn set_status(&mut self, message: String, is_error: bool) {
        self.status_message = Some((message, is_error));
    }

    pub fn restart_started(&mut self) {
        self.status_message = Some(("Restarting network...".to_string(), false));
    }

    pub fn refresh(&mut self) {
        self.load_network_info();
    }
}

impl Component for NetworkComponent {
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_confirm {
            match key.code {
                KeyCode::Char('y') | KeyCode::Char('Y') => {
                    self.show_confirm = false;
                    return Some(Message::RestartNetwork);
                }
                KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                    self.show_confirm = false;
                }
                _ => {}
            }
            return None;
        }

        match key.code {
            KeyCode::Tab => {
                self.mode = match self.mode {
                    NetworkMode::Overview => NetworkMode::Dns,
                    NetworkMode::Dns => NetworkMode::Overview,
                };
            }
            KeyCode::Up | KeyCode::Char('k') => {
                let len = match self.mode {
                    NetworkMode::Overview => self.interfaces.len(),
                    NetworkMode::Dns => self.dns_servers.len(),
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
                    NetworkMode::Dns => self.dns_servers.len(),
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
            NetworkMode::Dns => " Interfaces  [DNS]",
        };
        let mode_bar = Paragraph::new(Line::from(vec![
            Span::styled("View ", Theme::label()),
            Span::raw(mode_text),
            Span::raw(" "),
            Span::styled(" HOST ", Theme::badge_neutral()),
            Span::raw(" "),
            Span::styled(self.hostname.as_str(), Theme::subtitle()),
        ]))
        .block(Theme::panel(Theme::panel_title("Network fabric")));
        frame.render_widget(mode_bar, chunks[0]);

        // Main content
        match self.mode {
            NetworkMode::Overview => self.render_interfaces(frame, chunks[1]),
            NetworkMode::Dns => self.render_dns(frame, chunks[1]),
        }

        // Info panel
        self.render_info(frame, chunks[2]);

        // Status bar
        let status_content = if self.show_confirm {
            Line::from(vec![
                Span::styled("Restart network? ", Theme::warning()),
                Span::raw("[Y]es / [N]o"),
            ])
        } else if let Some((msg, is_error)) = &self.status_message {
            Line::from(Span::styled(
                msg.clone(),
                if *is_error {
                    Theme::error()
                } else {
                    Theme::success()
                },
            ))
        } else {
            Line::from(Span::styled("Press 'r' to restart network", Theme::muted()))
        };

        let status =
            Paragraph::new(status_content).block(Theme::panel_alt(Theme::panel_title("Status")));
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
    fn selected_interface(&self) -> Option<&NetworkInterface> {
        self.list_state
            .selected()
            .and_then(|idx| self.interfaces.get(idx))
    }

    fn selected_dns(&self) -> Option<&str> {
        self.list_state
            .selected()
            .and_then(|idx| self.dns_servers.get(idx))
            .map(String::as_str)
    }

    fn render_interfaces(&self, frame: &mut Frame, area: Rect) {
        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(62), Constraint::Percentage(38)])
            .split(area);

        let items: Vec<ListItem> = self
            .interfaces
            .iter()
            .map(|iface| {
                let status = if iface.is_up {
                    Span::styled(" UP ", Theme::badge_success())
                } else {
                    Span::styled(" DOWN ", Theme::badge_warning())
                };

                let dhcp = if iface.use_dhcp { "DHCP" } else { "Static" };

                ListItem::new(vec![
                    Line::from(vec![
                        Span::styled(format!("{:<12}", iface.name), Theme::title()),
                        status,
                        Span::raw(" "),
                        Span::styled(format!(" {} ", dhcp), Theme::badge_neutral()),
                        Span::raw(format!(" {}", iface.mac_address)),
                    ]),
                    Line::from(vec![
                        Span::styled("    Address ", Theme::muted()),
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
            .block(Theme::panel(Theme::panel_title("Interfaces")))
            .highlight_style(Theme::list_selected())
            .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, content[0], &mut state);

        let inspector_lines = if let Some(iface) = self.selected_interface() {
            vec![
                Line::from(vec![
                    Span::styled("INTERFACE", Theme::badge_info()),
                    Span::raw(" "),
                    Span::styled(&iface.name, Theme::title()),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled("Link ", Theme::label()),
                    if iface.is_up {
                        Span::styled(" UP ", Theme::badge_success())
                    } else {
                        Span::styled(" DOWN ", Theme::badge_warning())
                    },
                ]),
                Line::from(vec![
                    Span::styled("Address ", Theme::label()),
                    Span::raw(if iface.ip_address.is_empty() {
                        "not assigned".to_string()
                    } else {
                        format!("{}/{}", iface.ip_address, iface.netmask)
                    }),
                ]),
                Line::from(vec![
                    Span::styled("Mode ", Theme::label()),
                    Span::styled(
                        if iface.use_dhcp { " DHCP " } else { " STATIC " },
                        Theme::badge_neutral(),
                    ),
                ]),
                Line::from(vec![
                    Span::styled("Gateway ", Theme::label()),
                    Span::raw(if iface.gateway.is_empty() {
                        self.default_gateway.clone()
                    } else {
                        iface.gateway.clone()
                    }),
                ]),
                Line::from(vec![
                    Span::styled("MAC ", Theme::label()),
                    Span::raw(&iface.mac_address),
                ]),
            ]
        } else {
            vec![
                Line::from(Span::styled("No interface selected", Theme::muted())),
                Line::from(""),
                Line::from("Choose an interface to inspect link state and addressing."),
            ]
        };

        frame.render_widget(
            Paragraph::new(inspector_lines)
                .block(Theme::panel_alt(Theme::panel_title("Inspector"))),
            content[1],
        );
    }

    fn render_dns(&self, frame: &mut Frame, area: Rect) {
        let content = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(58), Constraint::Percentage(42)])
            .split(area);

        let items: Vec<ListItem> = self
            .dns_servers
            .iter()
            .enumerate()
            .map(|(i, server)| {
                ListItem::new(Line::from(vec![
                    Span::styled(format!("DNS {} ", i + 1), Theme::accent()),
                    Span::raw(server),
                ]))
            })
            .collect();

        let list = if items.is_empty() {
            List::new(vec![ListItem::new(Span::styled(
                "No DNS servers configured",
                Theme::muted(),
            ))])
        } else {
            List::new(items)
        }
        .block(Theme::panel(Theme::panel_title("DNS servers")))
        .highlight_style(Theme::list_selected())
        .highlight_symbol("▶ ");

        let mut state = self.list_state.clone();
        frame.render_stateful_widget(list, content[0], &mut state);

        let inspector_lines = if let Some(server) = self.selected_dns() {
            vec![
                Line::from(vec![
                    Span::styled("RESOLVER", Theme::badge_info()),
                    Span::raw(" "),
                    Span::styled(server, Theme::title()),
                ]),
                Line::from(""),
                Line::from(vec![
                    Span::styled("Order ", Theme::label()),
                    Span::raw(
                        self.list_state
                            .selected()
                            .map(|idx| format!("#{}", idx + 1))
                            .unwrap_or_else(|| "n/a".to_string()),
                    ),
                ]),
                Line::from(vec![
                    Span::styled("Source ", Theme::label()),
                    Span::raw("/etc/resolv.conf"),
                ]),
                Line::from(""),
                Line::from(Span::styled(
                    "Higher entries are consulted first during name resolution.",
                    Theme::subtitle(),
                )),
            ]
        } else {
            vec![
                Line::from(Span::styled("No DNS server selected", Theme::muted())),
                Line::from(""),
                Line::from("Select a resolver entry to inspect its order and source."),
            ]
        };

        frame.render_widget(
            Paragraph::new(inspector_lines)
                .block(Theme::panel_alt(Theme::panel_title("Inspector"))),
            content[1],
        );
    }

    fn render_info(&self, frame: &mut Frame, area: Rect) {
        let sections = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(54), Constraint::Percentage(46)])
            .split(area);

        let route_info = vec![
            Line::from(vec![
                Span::styled("Gateway ", Theme::label()),
                Span::styled(&self.default_gateway, Theme::badge_neutral()),
            ]),
            Line::from(vec![
                Span::styled("Hostname ", Theme::label()),
                Span::raw(&self.hostname),
            ]),
            Line::from(vec![
                Span::styled("Interfaces ", Theme::label()),
                Span::styled(self.interfaces.len().to_string(), Theme::badge_success()),
            ]),
        ];
        frame.render_widget(
            Paragraph::new(route_info).block(Theme::panel_alt(Theme::panel_title("Route summary"))),
            sections[0],
        );

        let config_info = vec![
            Line::from(vec![
                Span::styled("Resolver count ", Theme::label()),
                Span::styled(self.dns_servers.len().to_string(), Theme::badge_neutral()),
            ]),
            Line::from(vec![
                Span::styled("Config ", Theme::label()),
                Span::raw("/etc/rc.d/rc.inet1.conf"),
            ]),
            Line::from(Span::styled(
                "Use Tab to switch between interface and DNS views.",
                Theme::subtitle(),
            )),
        ];
        frame.render_widget(
            Paragraph::new(config_info)
                .block(Theme::panel_alt(Theme::panel_title("Control plane"))),
            sections[1],
        );
    }
}
