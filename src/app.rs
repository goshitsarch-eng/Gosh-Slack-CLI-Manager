use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    text::{Line, Span},
    widgets::{Block, Borders},
    Frame,
};
use std::fs;
use std::os::unix::fs::PermissionsExt;
use tokio::sync::mpsc;
use tokio::task;

use crate::components::{
    backup::{BackupAction, BackupComponent},
    config_editor::ConfigEditorComponent,
    cron::CronComponent,
    disks::{DiskAction, DiskComponent},
    kernel::{BootloaderType, KernelAction, KernelComponent},
    logs::LogViewerComponent,
    mirror::MirrorComponent,
    network::NetworkComponent,
    package_browser::PackageBrowserComponent,
    package_search::PackageSearchComponent,
    sbotools::{SbotoolsCommand, SbotoolsComponent},
    services::{ServiceAction, ServiceComponent},
    settings::{AppSettings, SettingsComponent},
    sysinfo::SysInfoComponent,
    updater::UpdaterComponent,
    user_setup::UserSetupComponent,
    Component, Tab,
};
use crate::slackware::commands::{CommandExecutor, CommandResult, StreamSource};
use crate::slackware::packages::{PackageInfo, PackageManager};
use crate::slackware::{config::SlackwareConfig, SlackwareVersion};
use crate::ui::layout::AppLayout;
use crate::ui::theme::Theme;
use crate::ui::widgets::StatusBar;
use crate::utils::fs::atomic_write;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TaskTarget {
    Updater,
    Sbotools,
    Mirror,
    Packages,
    Services,
    Network,
    Kernel,
    Backup,
    Disks,
}

/// Application messages for state updates
#[derive(Debug, Clone)]
pub enum Message {
    Quit,

    // System Update
    StartUpdate,
    ContinueUpdate,
    UpdateStepFinished { step: usize, result: CommandResult },

    // sbotools
    StartSbotoolsInstall,
    SbotoolsStepFinished(CommandResult),

    // User Setup
    CreateUser,
    UserCreated(Result<String, String>),

    // Mirror
    SetMirror(String),
    MirrorSet(Result<(), String>),

    // Package Search / Package Management
    SearchPackages(String),
    SearchResults(Result<Vec<PackageInfo>, String>),
    InstallPackage(String),
    PackageInstalled(Result<String, String>),
    RemoveInstalledPackage(String),
    PackageRemoved(Result<String, String>),

    // Config / Settings
    SaveConfig { path: String, content: String },
    ConfigSaved(Result<String, String>),
    SaveSettings(AppSettings),
    SettingsSaved(Result<String, String>),

    // Services / Network / Kernel / Backup / Disks
    RestartNetwork,
    NetworkRestarted(Result<String, String>),
    ServiceAction(ServiceAction),
    ServiceActionComplete(Result<String, String>),
    KernelAction(KernelAction),
    KernelActionComplete(Result<String, String>),
    BackupAction(BackupAction),
    BackupActionComplete(Result<String, String>),
    DiskAction(DiskAction),
    DiskActionComplete(Result<String, String>),

    TaskProgress(TaskTarget, String),
}

/// Main application state
pub struct App {
    pub running: bool,
    pub current_tab: Tab,
    pub slackware_version: SlackwareVersion,
    pub is_root: bool,

    pub updater: UpdaterComponent,
    pub sbotools: SbotoolsComponent,
    pub user_setup: UserSetupComponent,
    pub mirror: MirrorComponent,
    pub package_search: PackageSearchComponent,
    pub config_editor: ConfigEditorComponent,
    pub sysinfo: SysInfoComponent,
    pub services: ServiceComponent,
    pub package_browser: PackageBrowserComponent,
    pub backup: BackupComponent,
    pub network: NetworkComponent,
    pub logs: LogViewerComponent,
    pub kernel: KernelComponent,
    pub cron: CronComponent,
    pub disks: DiskComponent,
    pub settings: SettingsComponent,

    pub executor: CommandExecutor,
    pub event_tx: mpsc::UnboundedSender<Message>,
    pub event_rx: mpsc::UnboundedReceiver<Message>,

    show_exit_warning: bool,
}

impl App {
    pub fn new(version: SlackwareVersion, is_root: bool) -> Self {
        let (event_tx, event_rx) = mpsc::unbounded_channel();
        let settings = SettingsComponent::new();
        let current_tab = Self::tab_from_setting(&settings.settings().default_tab);

        Self {
            running: true,
            current_tab,
            slackware_version: version.clone(),
            is_root,

            updater: UpdaterComponent::new(version.clone()),
            sbotools: SbotoolsComponent::new(),
            user_setup: UserSetupComponent::new(),
            mirror: MirrorComponent::new(version),
            package_search: PackageSearchComponent::new(),
            config_editor: ConfigEditorComponent::new(),
            sysinfo: SysInfoComponent::new(),
            services: ServiceComponent::new(),
            package_browser: PackageBrowserComponent::new(),
            backup: BackupComponent::new(),
            network: NetworkComponent::new(),
            logs: LogViewerComponent::new(),
            kernel: KernelComponent::new(),
            cron: CronComponent::new(),
            disks: DiskComponent::new(),
            settings,

            executor: CommandExecutor::new(),
            event_tx,
            event_rx,
            show_exit_warning: false,
        }
    }

    fn tab_from_setting(setting: &str) -> Tab {
        match setting {
            "sbotools" => Tab::Sbotools,
            "user_setup" => Tab::UserSetup,
            "mirror" => Tab::Mirror,
            "packages" => Tab::Packages,
            "config" => Tab::Config,
            _ => Tab::Updater,
        }
    }

    fn spawn_command<F>(&self, target: TaskTarget, cmd: String, args: Vec<String>, completion: F)
    where
        F: FnOnce(CommandResult) -> Message + Send + 'static,
    {
        let tx = self.event_tx.clone();
        let progress_tx = tx.clone();
        let executor = self.executor.clone();

        tokio::spawn(async move {
            let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();
            let result = executor
                .execute_streaming(&cmd, &arg_refs, move |source, line| {
                    let rendered = match source {
                        StreamSource::Stdout => line.to_string(),
                        StreamSource::Stderr => format!("stderr: {line}"),
                    };
                    let _ = progress_tx.send(Message::TaskProgress(target, rendered));
                })
                .await;

            let _ = tx.send(completion(result));
        });
    }

    fn spawn_async<Fut>(&self, future: Fut)
    where
        Fut: std::future::Future<Output = Message> + Send + 'static,
    {
        let tx = self.event_tx.clone();
        tokio::spawn(async move {
            let _ = tx.send(future.await);
        });
    }

    fn spawn_blocking<F, G>(&self, work: F, completion: G)
    where
        F: FnOnce() -> Result<String, String> + Send + 'static,
        G: FnOnce(Result<String, String>) -> Message + Send + 'static,
    {
        let tx = self.event_tx.clone();
        task::spawn_blocking(move || {
            let _ = tx.send(completion(work()));
        });
    }

    fn handle_root_required(&mut self, area: TaskTarget, message: &str) {
        match area {
            TaskTarget::Updater => {
                self.updater.reset();
                self.updater.add_output(message.to_string());
            }
            TaskTarget::Sbotools => {
                self.sbotools.reset();
                self.sbotools.add_output(message.to_string());
            }
            TaskTarget::Mirror => self.mirror.set_status(message.to_string(), true),
            TaskTarget::Packages => self.package_search.set_status(message.to_string(), true),
            TaskTarget::Services => self.services.set_status(message.to_string(), true),
            TaskTarget::Network => self.network.set_status(message.to_string(), true),
            TaskTarget::Kernel => self.kernel.set_status(message.to_string(), true),
            TaskTarget::Backup => self.backup.set_status(message.to_string(), true),
            TaskTarget::Disks => self.disks.set_status(message.to_string(), true),
        }
    }

    /// Handle keyboard input
    pub fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        if self.show_exit_warning {
            match key.code {
                KeyCode::Char('q') | KeyCode::Char('Q') => {
                    self.show_exit_warning = false;
                    return Some(Message::Quit);
                }
                KeyCode::Char('l') | KeyCode::Char('L') => {
                    self.show_exit_warning = false;
                    self.current_tab = Tab::Updater;
                    self.updater.confirm_lilo(true);
                    return Some(Message::ContinueUpdate);
                }
                KeyCode::Esc => {
                    self.show_exit_warning = false;
                    return None;
                }
                _ => return None,
            }
        }

        if key.modifiers.contains(KeyModifiers::CONTROL) {
            match key.code {
                KeyCode::Char('c') | KeyCode::Char('q') => {
                    if self.updater.was_lilo_skipped() && self.updater.was_kernel_updated() {
                        self.show_exit_warning = true;
                        return None;
                    }
                    return Some(Message::Quit);
                }
                KeyCode::Char('k') => {
                    self.switch_to_tab(Tab::Kernel);
                    return None;
                }
                KeyCode::Char('j') => {
                    self.switch_to_tab(Tab::Cron);
                    return None;
                }
                KeyCode::Char('d') => {
                    self.switch_to_tab(Tab::Disks);
                    return None;
                }
                KeyCode::Char('s') => {
                    self.switch_to_tab(Tab::Settings);
                    return None;
                }
                _ => {}
            }
        }

        if self.updater.is_running()
            || self.updater.needs_lilo_confirm()
            || self.updater.is_showing_summary()
        {
            if self.current_tab == Tab::Updater {
                return self.updater.handle_input(key);
            }
            return None;
        }

        match key.code {
            KeyCode::F(1) => {
                self.switch_to_tab(Tab::Updater);
                return None;
            }
            KeyCode::F(2) => {
                self.switch_to_tab(Tab::Sbotools);
                return None;
            }
            KeyCode::F(3) => {
                self.switch_to_tab(Tab::UserSetup);
                return None;
            }
            KeyCode::F(4) => {
                self.switch_to_tab(Tab::Mirror);
                return None;
            }
            KeyCode::F(5) => match self.current_tab {
                Tab::Services
                | Tab::PackageBrowser
                | Tab::Backup
                | Tab::Network
                | Tab::Logs
                | Tab::Kernel
                | Tab::Cron
                | Tab::Disks
                | Tab::SysInfo => return self.delegate_to_component(key),
                _ => {
                    self.switch_to_tab(Tab::Packages);
                    return None;
                }
            },
            KeyCode::F(6) => {
                self.switch_to_tab(Tab::Config);
                return None;
            }
            KeyCode::F(7) => {
                self.switch_to_tab(Tab::SysInfo);
                return None;
            }
            KeyCode::F(8) => {
                self.switch_to_tab(Tab::Services);
                return None;
            }
            KeyCode::F(9) => {
                self.switch_to_tab(Tab::PackageBrowser);
                return None;
            }
            KeyCode::F(10) => {
                self.switch_to_tab(Tab::Backup);
                return None;
            }
            KeyCode::F(11) => {
                self.switch_to_tab(Tab::Network);
                return None;
            }
            KeyCode::F(12) => {
                self.switch_to_tab(Tab::Logs);
                return None;
            }
            KeyCode::Left if key.modifiers.contains(KeyModifiers::ALT) => {
                self.switch_to_tab(self.current_tab.prev());
                return None;
            }
            KeyCode::Right if key.modifiers.contains(KeyModifiers::ALT) => {
                self.switch_to_tab(self.current_tab.next());
                return None;
            }
            _ => {}
        }

        self.delegate_to_component(key)
    }

    fn switch_to_tab(&mut self, tab: Tab) {
        let old_tab = self.current_tab;
        self.current_tab = tab;
        self.deactivate_tab(old_tab);
        self.activate_tab(tab);
    }

    fn activate_tab(&mut self, tab: Tab) {
        match tab {
            Tab::Updater => {}
            Tab::Sbotools => {}
            Tab::UserSetup => {}
            Tab::Mirror => self.mirror.on_activate(),
            Tab::Packages => {}
            Tab::Config => {}
            Tab::SysInfo => self.sysinfo.on_activate(),
            Tab::Services => self.services.on_activate(),
            Tab::PackageBrowser => self.package_browser.on_activate(),
            Tab::Backup => self.backup.on_activate(),
            Tab::Network => self.network.on_activate(),
            Tab::Logs => self.logs.on_activate(),
            Tab::Kernel => self.kernel.on_activate(),
            Tab::Cron => self.cron.on_activate(),
            Tab::Disks => self.disks.on_activate(),
            Tab::Settings => {}
        }
    }

    fn deactivate_tab(&mut self, tab: Tab) {
        match tab {
            Tab::Updater => self.updater.on_deactivate(),
            Tab::Sbotools => self.sbotools.on_deactivate(),
            Tab::UserSetup => self.user_setup.on_deactivate(),
            Tab::Mirror => self.mirror.on_deactivate(),
            Tab::Packages => self.package_search.on_deactivate(),
            Tab::Config => self.config_editor.on_deactivate(),
            Tab::SysInfo => self.sysinfo.on_deactivate(),
            Tab::Services => self.services.on_deactivate(),
            Tab::PackageBrowser => self.package_browser.on_deactivate(),
            Tab::Backup => self.backup.on_deactivate(),
            Tab::Network => self.network.on_deactivate(),
            Tab::Logs => self.logs.on_deactivate(),
            Tab::Kernel => self.kernel.on_deactivate(),
            Tab::Cron => self.cron.on_deactivate(),
            Tab::Disks => self.disks.on_deactivate(),
            Tab::Settings => self.settings.on_deactivate(),
        }
    }

    fn delegate_to_component(&mut self, key: KeyEvent) -> Option<Message> {
        match self.current_tab {
            Tab::Updater => self.updater.handle_input(key),
            Tab::Sbotools => self.sbotools.handle_input(key),
            Tab::UserSetup => self.user_setup.handle_input(key),
            Tab::Mirror => self.mirror.handle_input(key),
            Tab::Packages => self.package_search.handle_input(key),
            Tab::Config => self.config_editor.handle_input(key),
            Tab::SysInfo => self.sysinfo.handle_input(key),
            Tab::Services => self.services.handle_input(key),
            Tab::PackageBrowser => self.package_browser.handle_input(key),
            Tab::Backup => self.backup.handle_input(key),
            Tab::Network => self.network.handle_input(key),
            Tab::Logs => self.logs.handle_input(key),
            Tab::Kernel => self.kernel.handle_input(key),
            Tab::Cron => self.cron.handle_input(key),
            Tab::Disks => self.disks.handle_input(key),
            Tab::Settings => self.settings.handle_input(key),
        }
    }

    pub fn update(&mut self, msg: Message) {
        match msg {
            Message::Quit => self.running = false,

            Message::StartUpdate | Message::ContinueUpdate => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Updater,
                        "Root privileges are required for system updates.",
                    );
                } else {
                    self.spawn_update_step();
                }
            }
            Message::UpdateStepFinished { step, result } => {
                if !result.stdout.is_empty() && !result.success && result.stderr.is_empty() {
                    self.updater.add_output(result.stdout.clone());
                }
                if step == 2 && result.success {
                    let has_kernel = self.updater.check_for_kernel_update(&result.stdout);
                    self.updater.set_kernel_updated(has_kernel);
                }
                self.updater.step_complete(
                    result.success,
                    if result.success {
                        None
                    } else {
                        Some(result.stderr)
                    },
                );

                if !self.updater.needs_lilo_confirm()
                    && self.updater.get_current_command().is_some()
                {
                    self.spawn_update_step();
                }
            }

            Message::StartSbotoolsInstall => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Sbotools,
                        "Root privileges are required for sbotools installation.",
                    );
                } else {
                    self.spawn_sbotools_step();
                }
            }
            Message::SbotoolsStepFinished(result) => {
                self.sbotools.step_complete(
                    result.success,
                    if result.success {
                        None
                    } else {
                        Some(result.stderr)
                    },
                );
                if self.sbotools.get_current_command().is_some() {
                    self.spawn_sbotools_step();
                }
            }

            Message::CreateUser => {
                if !self.is_root {
                    self.user_setup
                        .set_error("Root privileges are required to create users.".to_string());
                } else {
                    self.spawn_user_creation();
                }
            }
            Message::UserCreated(result) => match result {
                Ok(message) => self.user_setup.set_success(message),
                Err(error) => self.user_setup.set_error(error),
            },

            Message::SetMirror(url) => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Mirror,
                        "Root privileges are required to change mirrors.",
                    );
                } else {
                    self.spawn_set_mirror(url);
                }
            }
            Message::MirrorSet(result) => match result {
                Ok(()) => {
                    self.mirror
                        .set_status("Mirror updated successfully".to_string(), false);
                    self.mirror.load_mirrors();
                }
                Err(error) => self.mirror.set_status(format!("Error: {error}"), true),
            },

            Message::SearchPackages(query) => self.spawn_search(query),
            Message::SearchResults(result) => match result {
                Ok(results) => self.package_search.set_results(results),
                Err(error) => self.package_search.set_status(error, true),
            },
            Message::InstallPackage(name) => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Packages,
                        "Root privileges are required to install packages.",
                    );
                } else {
                    self.spawn_install_package(name);
                }
            }
            Message::PackageInstalled(result) => match result {
                Ok(message) => self.package_search.set_status(message, false),
                Err(error) => self.package_search.set_status(error, true),
            },
            Message::RemoveInstalledPackage(name) => {
                if !self.is_root {
                    self.package_browser.set_status(
                        "Root privileges are required to remove packages.".to_string(),
                        true,
                    );
                } else {
                    self.package_browser
                        .set_status(format!("Removing {name}..."), false);
                    self.spawn_command(
                        TaskTarget::Packages,
                        "removepkg".to_string(),
                        vec![name.clone()],
                        move |result| {
                            if result.success {
                                Message::PackageRemoved(Ok(format!(
                                    "Package '{name}' removed successfully"
                                )))
                            } else {
                                Message::PackageRemoved(Err(format!(
                                    "Failed to remove package: {}",
                                    result.stderr
                                )))
                            }
                        },
                    );
                }
            }
            Message::PackageRemoved(result) => match result {
                Ok(message) => {
                    self.package_browser.refresh_packages();
                    self.package_browser.set_status(message, false);
                }
                Err(error) => self.package_browser.set_status(error, true),
            },

            Message::SaveConfig { path, content } => {
                if !self.is_root {
                    self.config_editor.set_status(
                        "Root privileges are required to save system configuration files."
                            .to_string(),
                        true,
                    );
                } else {
                    self.spawn_blocking(
                        move || {
                            atomic_write(&path, &content)
                                .map(|_| format!("Saved {}", path))
                                .map_err(|err| err.to_string())
                        },
                        Message::ConfigSaved,
                    );
                }
            }
            Message::ConfigSaved(result) => match result {
                Ok(message) => self.config_editor.set_status(message, false),
                Err(error) => self.config_editor.set_status(error, true),
            },

            Message::SaveSettings(settings) => {
                self.spawn_blocking(
                    move || SettingsComponent::save(&settings),
                    Message::SettingsSaved,
                );
            }
            Message::SettingsSaved(result) => match result {
                Ok(message) => self.settings.set_status(message, false),
                Err(error) => self.settings.set_status(error, true),
            },

            Message::RestartNetwork => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Network,
                        "Root privileges are required to restart networking.",
                    );
                } else {
                    self.network.restart_started();
                    self.spawn_command(
                        TaskTarget::Network,
                        "/etc/rc.d/rc.inet1".to_string(),
                        vec!["restart".to_string()],
                        |result| {
                            if result.success {
                                Message::NetworkRestarted(Ok(
                                    "Network restarted successfully".to_string()
                                ))
                            } else {
                                Message::NetworkRestarted(Err(format!(
                                    "Failed to restart network: {}",
                                    result.stderr
                                )))
                            }
                        },
                    );
                }
            }
            Message::NetworkRestarted(result) => {
                self.network.refresh();
                match result {
                    Ok(message) => self.network.set_status(message, false),
                    Err(error) => self.network.set_status(error, true),
                }
            }

            Message::ServiceAction(action) => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Services,
                        "Root privileges are required to modify services.",
                    );
                } else {
                    self.services.action_started(&action);
                    self.spawn_service_action(action);
                }
            }
            Message::ServiceActionComplete(result) => {
                self.services.load_services();
                match result {
                    Ok(message) => self.services.set_status(message, false),
                    Err(error) => self.services.set_status(error, true),
                }
            }

            Message::KernelAction(action) => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Kernel,
                        "Root privileges are required to modify kernel settings.",
                    );
                } else {
                    self.spawn_kernel_action(action);
                }
            }
            Message::KernelActionComplete(result) => {
                self.kernel.refresh();
                match result {
                    Ok(message) => self.kernel.set_status(message, false),
                    Err(error) => self.kernel.set_status(error, true),
                }
            }

            Message::BackupAction(action) => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Backup,
                        "Root privileges are required to manage system backups.",
                    );
                } else {
                    let files = self.backup.config_files().to_vec();
                    self.backup
                        .set_status("Running backup operation...".to_string(), false);
                    self.spawn_blocking(
                        move || BackupComponent::execute(action, &files),
                        Message::BackupActionComplete,
                    );
                }
            }
            Message::BackupActionComplete(result) => {
                self.backup.refresh_backups();
                match result {
                    Ok(message) => self.backup.set_status(message, false),
                    Err(error) => self.backup.set_status(error, true),
                }
            }

            Message::DiskAction(action) => {
                if !self.is_root {
                    self.handle_root_required(
                        TaskTarget::Disks,
                        "Root privileges are required to mount or unmount disks.",
                    );
                } else {
                    self.disks.action_started(&action);
                    self.spawn_disk_action(action);
                }
            }
            Message::DiskActionComplete(result) => {
                self.disks.refresh_disks();
                match result {
                    Ok(message) => self.disks.set_status(message, false),
                    Err(error) => self.disks.set_status(error, true),
                }
            }

            Message::TaskProgress(target, line) => match target {
                TaskTarget::Updater => self.updater.add_output(line),
                TaskTarget::Sbotools => self.sbotools.add_output(line),
                TaskTarget::Mirror => self.mirror.set_status(line, false),
                TaskTarget::Packages => self.package_search.set_status(line, false),
                TaskTarget::Services => self.services.set_status(line, false),
                TaskTarget::Network => self.network.set_status(line, false),
                TaskTarget::Kernel => self.kernel.set_status(line, false),
                TaskTarget::Backup => self.backup.set_status(line, false),
                TaskTarget::Disks => self.disks.set_status(line, false),
            },
        }
    }

    fn spawn_update_step(&mut self) {
        let step = self.updater.current_step;
        if let Some((cmd, args)) = self.updater.get_current_command() {
            let cmd = cmd.to_string();
            let args = args.iter().map(|arg| arg.to_string()).collect::<Vec<_>>();
            self.updater
                .add_output(format!("Running: {} {}", cmd, args.join(" ")));
            self.spawn_command(TaskTarget::Updater, cmd, args, move |result| {
                Message::UpdateStepFinished { step, result }
            });
        }
    }

    fn spawn_sbotools_step(&mut self) {
        let step = self.sbotools.get_current_command();

        if let Some(command) = step {
            match command {
                SbotoolsCommand::Download { url, filename } => {
                    self.sbotools
                        .add_output(format!("Downloading {filename}..."));
                    self.spawn_command(
                        TaskTarget::Sbotools,
                        "wget".to_string(),
                        vec!["-O".to_string(), format!("/tmp/{filename}"), url],
                        Message::SbotoolsStepFinished,
                    );
                }
                SbotoolsCommand::InstallPkg { path } => {
                    self.sbotools.add_output(format!("Installing {path}..."));
                    self.spawn_command(
                        TaskTarget::Sbotools,
                        "installpkg".to_string(),
                        vec![path],
                        Message::SbotoolsStepFinished,
                    );
                }
                SbotoolsCommand::SbopkgSync => {
                    self.sbotools
                        .add_output("Syncing sbopkg repository...".to_string());
                    self.spawn_command(
                        TaskTarget::Sbotools,
                        "sbopkg".to_string(),
                        vec!["-r".to_string()],
                        Message::SbotoolsStepFinished,
                    );
                }
                SbotoolsCommand::SbopkgInstall { package } => {
                    self.sbotools.add_output(format!("Installing {package}..."));
                    self.spawn_command(
                        TaskTarget::Sbotools,
                        "sbopkg".to_string(),
                        vec!["-i".to_string(), package],
                        Message::SbotoolsStepFinished,
                    );
                }
                SbotoolsCommand::SboconfigRepo { url } => {
                    self.sbotools.add_output(format!("Configuring repo: {url}"));
                    self.spawn_command(
                        TaskTarget::Sbotools,
                        "sboconfig".to_string(),
                        vec!["-r".to_string(), url],
                        Message::SbotoolsStepFinished,
                    );
                }
                SbotoolsCommand::SbosnapFetch => {
                    self.sbotools
                        .add_output("Fetching SlackBuilds snapshot...".to_string());
                    self.spawn_command(
                        TaskTarget::Sbotools,
                        "sbosnap".to_string(),
                        vec!["fetch".to_string()],
                        Message::SbotoolsStepFinished,
                    );
                }
            }
        }
    }

    fn spawn_user_creation(&self) {
        let tx = self.event_tx.clone();
        let executor = self.executor.clone();
        let username = self.user_setup.get_username().to_string();
        let password = self.user_setup.get_password().to_string();
        let groups = self.user_setup.get_selected_groups();
        let change_runlevel = self.user_setup.should_change_runlevel();

        tokio::spawn(async move {
            let groups_ref: Vec<&str> = groups.iter().map(String::as_str).collect();
            let result = executor.useradd(&username, &groups_ref, "/bin/bash").await;
            if !result.success {
                let _ = tx.send(Message::UserCreated(Err(format!(
                    "Failed to create user: {}",
                    result.stderr
                ))));
                return;
            }

            let result = executor.set_password(&username, &password).await;
            if !result.success {
                let _ = tx.send(Message::UserCreated(Err(format!(
                    "Failed to set password: {}",
                    result.stderr
                ))));
                return;
            }

            if change_runlevel {
                if let Err(error) = SlackwareConfig::set_default_runlevel(4) {
                    let _ = tx.send(Message::UserCreated(Err(format!(
                        "User created but runlevel change failed: {error}"
                    ))));
                    return;
                }
            }

            let _ = tx.send(Message::UserCreated(Ok(format!(
                "User '{}' created successfully!{}",
                username,
                if change_runlevel {
                    " Runlevel changed to 4."
                } else {
                    ""
                }
            ))));
        });
    }

    fn spawn_set_mirror(&mut self, url: String) {
        self.mirror
            .set_status("Updating mirror configuration...".to_string(), false);
        let tx = self.event_tx.clone();
        let executor = self.executor.clone();

        tokio::spawn(async move {
            if let Err(error) = SlackwareConfig::set_active_mirror(&url) {
                let _ = tx.send(Message::MirrorSet(Err(error.to_string())));
                return;
            }

            let _ = tx.send(Message::TaskProgress(
                TaskTarget::Mirror,
                "Updating GPG key...".to_string(),
            ));
            let gpg_result = executor
                .execute_streaming("slackpkg", &["update", "gpg"], {
                    let tx = tx.clone();
                    move |source, line| {
                        let rendered = match source {
                            StreamSource::Stdout => line.to_string(),
                            StreamSource::Stderr => format!("stderr: {line}"),
                        };
                        let _ = tx.send(Message::TaskProgress(TaskTarget::Mirror, rendered));
                    }
                })
                .await;

            if !gpg_result.success {
                let _ = tx.send(Message::MirrorSet(Err(format!(
                    "GPG update failed: {}",
                    gpg_result.stderr
                ))));
                return;
            }

            let _ = tx.send(Message::TaskProgress(
                TaskTarget::Mirror,
                "Updating package list...".to_string(),
            ));
            let update_result = executor
                .execute_streaming("slackpkg", &["update"], {
                    let tx = tx.clone();
                    move |source, line| {
                        let rendered = match source {
                            StreamSource::Stdout => line.to_string(),
                            StreamSource::Stderr => format!("stderr: {line}"),
                        };
                        let _ = tx.send(Message::TaskProgress(TaskTarget::Mirror, rendered));
                    }
                })
                .await;

            if update_result.success {
                let _ = tx.send(Message::MirrorSet(Ok(())));
            } else {
                let _ = tx.send(Message::MirrorSet(Err(format!(
                    "Package list update failed: {}",
                    update_result.stderr
                ))));
            }
        });
    }

    fn spawn_search(&self, query: String) {
        self.spawn_async(async move {
            let manager = PackageManager::new();
            Message::SearchResults(manager.search(&query).await)
        });
    }

    fn spawn_install_package(&mut self, name: String) {
        self.package_search
            .set_status(format!("Installing package '{name}'..."), false);
        self.spawn_command(
            TaskTarget::Packages,
            "sboinstall".to_string(),
            vec!["-j".to_string(), name.clone()],
            move |result| {
                if result.success {
                    Message::PackageInstalled(Ok(format!(
                        "Package '{name}' installed successfully"
                    )))
                } else {
                    Message::PackageInstalled(Err(format!(
                        "Installation failed: {}",
                        result.stderr
                    )))
                }
            },
        );
    }

    fn spawn_service_action(&self, action: ServiceAction) {
        match action.clone() {
            ServiceAction::Toggle(name) => self.spawn_blocking(
                move || {
                    let path = format!("/etc/rc.d/{name}");
                    let metadata = fs::metadata(&path).map_err(|err| err.to_string())?;
                    let mut permissions = metadata.permissions();
                    let mode = permissions.mode();
                    if mode & 0o111 != 0 {
                        permissions.set_mode(mode & !0o111);
                    } else {
                        permissions.set_mode(mode | 0o755);
                    }
                    fs::set_permissions(&path, permissions).map_err(|err| err.to_string())?;
                    Ok(format!("Toggled {name} executable bit"))
                },
                Message::ServiceActionComplete,
            ),
            ServiceAction::Start(name) => self.spawn_command(
                TaskTarget::Services,
                format!("/etc/rc.d/{name}"),
                vec!["start".to_string()],
                move |result| {
                    if result.success {
                        Message::ServiceActionComplete(Ok(format!(
                            "Service {name} started successfully"
                        )))
                    } else {
                        Message::ServiceActionComplete(Err(format!(
                            "Failed to start service: {}",
                            result.stderr
                        )))
                    }
                },
            ),
            ServiceAction::Stop(name) => self.spawn_command(
                TaskTarget::Services,
                format!("/etc/rc.d/{name}"),
                vec!["stop".to_string()],
                move |result| {
                    if result.success {
                        Message::ServiceActionComplete(Ok(format!(
                            "Service {name} stopped successfully"
                        )))
                    } else {
                        Message::ServiceActionComplete(Err(format!(
                            "Failed to stop service: {}",
                            result.stderr
                        )))
                    }
                },
            ),
            ServiceAction::Restart(name) => self.spawn_command(
                TaskTarget::Services,
                format!("/etc/rc.d/{name}"),
                vec!["restart".to_string()],
                move |result| {
                    if result.success {
                        Message::ServiceActionComplete(Ok(format!(
                            "Service {name} restarted successfully"
                        )))
                    } else {
                        Message::ServiceActionComplete(Err(format!(
                            "Failed to restart service: {}",
                            result.stderr
                        )))
                    }
                },
            ),
        }
    }

    fn spawn_kernel_action(&self, action: KernelAction) {
        match action.clone() {
            KernelAction::RunLilo => self.spawn_command(
                TaskTarget::Kernel,
                "lilo".to_string(),
                Vec::new(),
                |result| {
                    if result.success {
                        Message::KernelActionComplete(Ok("LILO updated successfully".to_string()))
                    } else {
                        Message::KernelActionComplete(Err(format!(
                            "LILO failed: {}",
                            result.stderr
                        )))
                    }
                },
            ),
            KernelAction::SetDefault(version) => {
                let bootloader = self.kernel.bootloader();
                self.spawn_blocking(
                    move || match bootloader {
                        BootloaderType::Lilo => {
                            let new_content = KernelComponent::build_lilo_default_config(&version)?;
                            atomic_write("/etc/lilo.conf", &new_content)
                                .map_err(|err| err.to_string())?;
                            Ok(format!(
                                "Default kernel set to {version}. Run lilo to apply."
                            ))
                        }
                        BootloaderType::Grub => {
                            Err("GRUB configuration editing is not yet supported".to_string())
                        }
                        BootloaderType::Unknown => Err("No known bootloader detected".to_string()),
                    },
                    Message::KernelActionComplete,
                );
            }
        }
    }

    fn spawn_disk_action(&self, action: DiskAction) {
        match action.clone() {
            DiskAction::Mount(device) => {
                let mount_point = self.disks.find_mount_point(&device);
                self.spawn_command(
                    TaskTarget::Disks,
                    "mount".to_string(),
                    vec![device.clone(), mount_point.clone()],
                    move |result| {
                        if result.success {
                            Message::DiskActionComplete(Ok(format!(
                                "Mounted {device} at {mount_point}"
                            )))
                        } else {
                            Message::DiskActionComplete(Err(format!(
                                "Mount failed: {}",
                                result.stderr
                            )))
                        }
                    },
                );
            }
            DiskAction::Unmount(mount_point) => self.spawn_command(
                TaskTarget::Disks,
                "umount".to_string(),
                vec![mount_point.clone()],
                move |result| {
                    if result.success {
                        Message::DiskActionComplete(Ok(format!("Unmounted {mount_point}")))
                    } else {
                        Message::DiskActionComplete(Err(format!(
                            "Unmount failed: {}",
                            result.stderr
                        )))
                    }
                },
            ),
            DiskAction::CheckFilesystem(device) => {
                self.spawn_blocking(
                    move || Err(format!(
                        "Filesystem check for {device} requires an unmounted partition. Run fsck manually."
                    )),
                    Message::DiskActionComplete,
                );
            }
        }
    }

    /// Render the UI
    pub fn render(&self, frame: &mut Frame) {
        let layout = AppLayout::new(frame.area());

        let mut title_spans = vec![
            Span::styled(" Slackware CLI Manager ", Theme::title()),
            Span::styled(
                format!(" - {} ", self.slackware_version.display_name()),
                Theme::muted(),
            ),
        ];
        if !self.is_root {
            title_spans.push(Span::styled(" [Read-only mode] ", Theme::warning()));
        }

        let header = ratatui::widgets::Paragraph::new(Line::from(title_spans))
            .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(header, layout.header);

        self.render_tabs(frame, layout.tabs);

        match self.current_tab {
            Tab::Updater => self.updater.render(frame, layout.content),
            Tab::Sbotools => self.sbotools.render(frame, layout.content),
            Tab::UserSetup => self.user_setup.render(frame, layout.content),
            Tab::Mirror => self.mirror.render(frame, layout.content),
            Tab::Packages => self.package_search.render(frame, layout.content),
            Tab::Config => self.config_editor.render(frame, layout.content),
            Tab::SysInfo => self.sysinfo.render(frame, layout.content),
            Tab::Services => self.services.render(frame, layout.content),
            Tab::PackageBrowser => self.package_browser.render(frame, layout.content),
            Tab::Backup => self.backup.render(frame, layout.content),
            Tab::Network => self.network.render(frame, layout.content),
            Tab::Logs => self.logs.render(frame, layout.content),
            Tab::Kernel => self.kernel.render(frame, layout.content),
            Tab::Cron => self.cron.render(frame, layout.content),
            Tab::Disks => self.disks.render(frame, layout.content),
            Tab::Settings => self.settings.render(frame, layout.content),
        }

        let help = self.get_current_help();
        let mut keys = vec![("Alt+←/→", "Tab"), ("Ctrl+Q", "Quit")];
        keys.extend(help);
        if !self.is_root {
            keys.push(("Read-only", "Mutating actions disabled"));
        }

        let status = StatusBar::new("").keys(keys);
        frame.render_widget(status, layout.status_bar);

        if self.show_exit_warning {
            self.render_exit_warning(frame, frame.area());
        }
    }

    fn render_tabs(&self, frame: &mut Frame, area: Rect) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Length(1), Constraint::Length(1)])
            .split(area);

        let primary_tabs: Vec<Span> = Tab::primary_tabs()
            .iter()
            .map(|tab| {
                let style = if *tab == self.current_tab {
                    Theme::tab_active()
                } else {
                    Theme::tab_inactive()
                };
                Span::styled(format!(" {} {} ", tab.shortcut(), tab.title()), style)
            })
            .collect();
        frame.render_widget(
            ratatui::widgets::Paragraph::new(Line::from(primary_tabs)),
            chunks[0],
        );

        let mut secondary_spans: Vec<Span> = Tab::secondary_tabs()
            .iter()
            .map(|tab| {
                let style = if *tab == self.current_tab {
                    Theme::tab_active()
                } else {
                    Theme::tab_inactive()
                };
                Span::styled(format!(" {} {} ", tab.shortcut(), tab.title()), style)
            })
            .collect();
        secondary_spans.push(Span::styled(" │ ", Theme::muted()));

        for tab in Tab::additional_tabs() {
            let style = if tab == self.current_tab {
                Theme::tab_active()
            } else {
                Theme::tab_inactive()
            };
            secondary_spans.push(Span::styled(
                format!(" {} {} ", tab.shortcut(), tab.title()),
                style,
            ));
        }

        frame.render_widget(
            ratatui::widgets::Paragraph::new(Line::from(secondary_spans)),
            chunks[1],
        );
    }

    fn get_current_help(&self) -> Vec<(&'static str, &'static str)> {
        match self.current_tab {
            Tab::Updater => self.updater.help_text(),
            Tab::Sbotools => self.sbotools.help_text(),
            Tab::UserSetup => self.user_setup.help_text(),
            Tab::Mirror => self.mirror.help_text(),
            Tab::Packages => self.package_search.help_text(),
            Tab::Config => self.config_editor.help_text(),
            Tab::SysInfo => self.sysinfo.help_text(),
            Tab::Services => self.services.help_text(),
            Tab::PackageBrowser => self.package_browser.help_text(),
            Tab::Backup => self.backup.help_text(),
            Tab::Network => self.network.help_text(),
            Tab::Logs => self.logs.help_text(),
            Tab::Kernel => self.kernel.help_text(),
            Tab::Cron => self.cron.help_text(),
            Tab::Disks => self.disks.help_text(),
            Tab::Settings => self.settings.help_text(),
        }
    }

    fn render_exit_warning(&self, frame: &mut Frame, area: Rect) {
        use crate::ui::centered_rect;
        use ratatui::widgets::{Clear, Paragraph};

        let dialog_area = centered_rect(55, 45, area);
        frame.render_widget(Clear, dialog_area);

        let dialog = Block::default()
            .title(" !! BOOTLOADER NOT UPDATED !! ")
            .borders(Borders::ALL)
            .border_style(Theme::error());
        let inner = dialog.inner(dialog_area);
        frame.render_widget(dialog, dialog_area);

        let text = Paragraph::new(vec![
            Line::from(""),
            Line::from(Span::styled(
                "You skipped the bootloader update after",
                Theme::warning(),
            )),
            Line::from(Span::styled(
                "a kernel update. Your system may not",
                Theme::warning(),
            )),
            Line::from(Span::styled("boot after reboot!", Theme::warning())),
            Line::from(""),
            Line::from(""),
            Line::from(vec![
                Span::styled("[Q]", Theme::key_hint()),
                Span::raw(" Quit anyway"),
            ]),
            Line::from(vec![
                Span::styled("[L]", Theme::key_hint()),
                Span::raw(" Run lilo now"),
            ]),
            Line::from(vec![
                Span::styled("[Esc]", Theme::key_hint()),
                Span::raw(" Cancel"),
            ]),
        ])
        .style(Theme::default());
        frame.render_widget(text, inner);
    }
}

#[cfg(test)]
mod tests {
    use ratatui::{backend::TestBackend, Terminal};

    use super::*;

    #[test]
    fn renders_all_tabs_without_panicking() {
        let backend = TestBackend::new(140, 45);
        let mut terminal = Terminal::new(backend).unwrap();
        let mut app = App::new(SlackwareVersion::Current, false);

        for tab in Tab::all() {
            app.switch_to_tab(tab);
            terminal.draw(|frame| app.render(frame)).unwrap();
        }
    }

    #[test]
    fn read_only_mode_blocks_update_start() {
        let mut app = App::new(SlackwareVersion::Current, false);

        app.update(Message::StartUpdate);

        assert!(!app.updater.is_running());
        assert_eq!(
            app.updater.last_output(),
            Some("Root privileges are required for system updates.")
        );
    }

    #[test]
    fn updater_progress_routes_even_when_other_tab_is_active() {
        let mut app = App::new(SlackwareVersion::Current, true);
        app.switch_to_tab(Tab::Logs);

        app.update(Message::TaskProgress(
            TaskTarget::Updater,
            "slackpkg update output".to_string(),
        ));

        assert_eq!(app.updater.last_output(), Some("slackpkg update output"));
    }
}
