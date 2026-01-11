pub mod config_editor;
pub mod mirror;
pub mod package_search;
pub mod sbotools;
pub mod updater;
pub mod user_setup;

// New components
pub mod backup;
pub mod cron;
pub mod disks;
pub mod kernel;
pub mod logs;
pub mod network;
pub mod package_browser;
pub mod services;
pub mod settings;
pub mod sysinfo;

use crossterm::event::KeyEvent;
use ratatui::Frame;
use tokio::sync::mpsc;

use crate::app::Message;

/// Trait for TUI components
pub trait Component {
    /// Handle keyboard input
    fn handle_input(&mut self, key: KeyEvent) -> Option<Message>;

    /// Render the component
    fn render(&self, frame: &mut Frame, area: ratatui::layout::Rect);

    /// Get help text for status bar
    fn help_text(&self) -> Vec<(&'static str, &'static str)>;

    /// Called when component becomes active
    fn on_activate(&mut self) {}

    /// Called when component becomes inactive
    fn on_deactivate(&mut self) {}
}

/// Async component trait for components that execute commands
pub trait AsyncComponent: Component {
    /// Set the progress channel for async updates
    fn set_progress_channel(&mut self, tx: mpsc::UnboundedSender<String>);

    /// Check if a task is currently running
    fn is_running(&self) -> bool;
}

/// The active tab in the main menu
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Tab {
    // Original tabs (F1-F6)
    Updater,
    Sbotools,
    UserSetup,
    Mirror,
    Packages,
    Config,
    // New tabs (F7-F12)
    SysInfo,
    Services,
    PackageBrowser,
    Backup,
    Network,
    Logs,
    // Additional tabs (Ctrl+shortcuts)
    Kernel,
    Cron,
    Disks,
    Settings,
}

impl Tab {
    pub fn all() -> Vec<Tab> {
        vec![
            Tab::Updater,
            Tab::Sbotools,
            Tab::UserSetup,
            Tab::Mirror,
            Tab::Packages,
            Tab::Config,
            Tab::SysInfo,
            Tab::Services,
            Tab::PackageBrowser,
            Tab::Backup,
            Tab::Network,
            Tab::Logs,
            Tab::Kernel,
            Tab::Cron,
            Tab::Disks,
            Tab::Settings,
        ]
    }

    /// Get tabs shown in the primary tab bar (F1-F6)
    pub fn primary_tabs() -> Vec<Tab> {
        vec![
            Tab::Updater,
            Tab::Sbotools,
            Tab::UserSetup,
            Tab::Mirror,
            Tab::Packages,
            Tab::Config,
        ]
    }

    /// Get tabs shown in the secondary tab bar (F7-F12)
    pub fn secondary_tabs() -> Vec<Tab> {
        vec![
            Tab::SysInfo,
            Tab::Services,
            Tab::PackageBrowser,
            Tab::Backup,
            Tab::Network,
            Tab::Logs,
        ]
    }

    /// Get additional tabs (Ctrl+shortcuts)
    pub fn additional_tabs() -> Vec<Tab> {
        vec![Tab::Kernel, Tab::Cron, Tab::Disks, Tab::Settings]
    }

    pub fn title(&self) -> &'static str {
        match self {
            Tab::Updater => "Update",
            Tab::Sbotools => "sbotools",
            Tab::UserSetup => "Users",
            Tab::Mirror => "Mirrors",
            Tab::Packages => "Search",
            Tab::Config => "Config",
            Tab::SysInfo => "SysInfo",
            Tab::Services => "Services",
            Tab::PackageBrowser => "Packages",
            Tab::Backup => "Backup",
            Tab::Network => "Network",
            Tab::Logs => "Logs",
            Tab::Kernel => "Kernel",
            Tab::Cron => "Cron",
            Tab::Disks => "Disks",
            Tab::Settings => "Settings",
        }
    }

    pub fn shortcut(&self) -> &'static str {
        match self {
            Tab::Updater => "F1",
            Tab::Sbotools => "F2",
            Tab::UserSetup => "F3",
            Tab::Mirror => "F4",
            Tab::Packages => "F5",
            Tab::Config => "F6",
            Tab::SysInfo => "F7",
            Tab::Services => "F8",
            Tab::PackageBrowser => "F9",
            Tab::Backup => "F10",
            Tab::Network => "F11",
            Tab::Logs => "F12",
            Tab::Kernel => "^K",
            Tab::Cron => "^J",
            Tab::Disks => "^D",
            Tab::Settings => "^S",
        }
    }

    pub fn next(&self) -> Tab {
        match self {
            Tab::Updater => Tab::Sbotools,
            Tab::Sbotools => Tab::UserSetup,
            Tab::UserSetup => Tab::Mirror,
            Tab::Mirror => Tab::Packages,
            Tab::Packages => Tab::Config,
            Tab::Config => Tab::SysInfo,
            Tab::SysInfo => Tab::Services,
            Tab::Services => Tab::PackageBrowser,
            Tab::PackageBrowser => Tab::Backup,
            Tab::Backup => Tab::Network,
            Tab::Network => Tab::Logs,
            Tab::Logs => Tab::Kernel,
            Tab::Kernel => Tab::Cron,
            Tab::Cron => Tab::Disks,
            Tab::Disks => Tab::Settings,
            Tab::Settings => Tab::Updater,
        }
    }

    pub fn prev(&self) -> Tab {
        match self {
            Tab::Updater => Tab::Settings,
            Tab::Sbotools => Tab::Updater,
            Tab::UserSetup => Tab::Sbotools,
            Tab::Mirror => Tab::UserSetup,
            Tab::Packages => Tab::Mirror,
            Tab::Config => Tab::Packages,
            Tab::SysInfo => Tab::Config,
            Tab::Services => Tab::SysInfo,
            Tab::PackageBrowser => Tab::Services,
            Tab::Backup => Tab::PackageBrowser,
            Tab::Network => Tab::Backup,
            Tab::Logs => Tab::Network,
            Tab::Kernel => Tab::Logs,
            Tab::Cron => Tab::Kernel,
            Tab::Disks => Tab::Cron,
            Tab::Settings => Tab::Disks,
        }
    }
}
