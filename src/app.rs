use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    text::{Line, Span},
    widgets::{Block, Borders},
    Frame,
};
use tokio::sync::mpsc;

use crate::components::{
    backup::BackupComponent,
    config_editor::ConfigEditorComponent,
    cron::CronComponent,
    disks::DiskComponent,
    kernel::KernelComponent,
    logs::LogViewerComponent,
    mirror::MirrorComponent,
    network::NetworkComponent,
    package_browser::PackageBrowserComponent,
    package_search::PackageSearchComponent,
    sbotools::SbotoolsComponent,
    services::ServiceComponent,
    settings::SettingsComponent,
    sysinfo::SysInfoComponent,
    updater::UpdaterComponent,
    user_setup::UserSetupComponent,
    Component, Tab,
};
use crate::slackware::{CommandExecutor, SlackwareVersion};
use crate::ui::layout::AppLayout;
use crate::ui::theme::Theme;
use crate::ui::widgets::StatusBar;

/// Application messages for state updates
#[derive(Debug, Clone)]
pub enum Message {
    // Navigation
    Quit,
    NextTab,
    PrevTab,

    // System Update
    StartUpdate,
    ContinueUpdate,
    UpdateStepComplete(bool, Option<String>),
    UpdateOutput(String),

    // sbotools
    StartSbotoolsInstall,
    SbotoolsStepComplete(bool, Option<String>),
    SbotoolsOutput(String),

    // User Setup
    CreateUser,
    UserCreated(Result<String, String>),

    // Mirror
    SetMirror(String),
    MirrorSet(Result<(), String>),

    // Package Search
    SearchPackages(String),
    SearchResults(Vec<crate::slackware::packages::PackageInfo>),
    InstallPackage(String),
    PackageInstalled(Result<(), String>),

    // Progress
    ProgressUpdate(String),
}

/// Main application state
pub struct App {
    pub running: bool,
    pub current_tab: Tab,
    pub slackware_version: SlackwareVersion,

    // Original Components
    pub updater: UpdaterComponent,
    pub sbotools: SbotoolsComponent,
    pub user_setup: UserSetupComponent,
    pub mirror: MirrorComponent,
    pub package_search: PackageSearchComponent,
    pub config_editor: ConfigEditorComponent,

    // New Components
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

    // Command executor
    pub executor: CommandExecutor,

    // Progress channel
    pub progress_tx: mpsc::UnboundedSender<String>,
    pub progress_rx: mpsc::UnboundedReceiver<String>,

    // Exit warning state
    show_exit_warning: bool,
}

impl App {
    pub fn new(version: SlackwareVersion) -> Self {
        let (progress_tx, progress_rx) = mpsc::unbounded_channel();

        Self {
            running: true,
            current_tab: Tab::Updater,
            slackware_version: version.clone(),

            // Original components
            updater: UpdaterComponent::new(),
            sbotools: SbotoolsComponent::new(),
            user_setup: UserSetupComponent::new(),
            mirror: MirrorComponent::new(version),
            package_search: PackageSearchComponent::new(),
            config_editor: ConfigEditorComponent::new(),

            // New components
            sysinfo: SysInfoComponent::new(),
            services: ServiceComponent::new(),
            package_browser: PackageBrowserComponent::new(),
            backup: BackupComponent::new(),
            network: NetworkComponent::new(),
            logs: LogViewerComponent::new(),
            kernel: KernelComponent::new(),
            cron: CronComponent::new(),
            disks: DiskComponent::new(),
            settings: SettingsComponent::new(),

            executor: CommandExecutor::new(),
            progress_tx,
            progress_rx,

            show_exit_warning: false,
        }
    }

    /// Check if exit warning dialog is showing
    pub fn is_showing_exit_warning(&self) -> bool {
        self.show_exit_warning
    }

    /// Handle keyboard input
    pub fn handle_input(&mut self, key: KeyEvent) -> Option<Message> {
        // Handle exit warning dialog
        if self.show_exit_warning {
            match key.code {
                KeyCode::Char('q') | KeyCode::Char('Q') => {
                    // Quit anyway
                    self.show_exit_warning = false;
                    return Some(Message::Quit);
                }
                KeyCode::Char('l') | KeyCode::Char('L') => {
                    // Run lilo now
                    self.show_exit_warning = false;
                    self.current_tab = Tab::Updater;
                    // Trigger lilo run through updater
                    self.updater.confirm_lilo(true);
                    return Some(Message::ContinueUpdate);
                }
                KeyCode::Esc => {
                    // Cancel exit
                    self.show_exit_warning = false;
                    return None;
                }
                _ => return None,
            }
        }

        // Global keys
        if key.modifiers.contains(KeyModifiers::CONTROL) {
            match key.code {
                KeyCode::Char('c') | KeyCode::Char('q') => {
                    // Check if we should show exit warning
                    if self.updater.was_lilo_skipped() && self.updater.was_kernel_updated() {
                        self.show_exit_warning = true;
                        return None;
                    }
                    return Some(Message::Quit);
                }
                // Additional Ctrl+shortcuts for new tabs
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

        // Block tab navigation during update or when showing dialogs
        if self.updater.is_running() || self.updater.needs_lilo_confirm() || self.updater.is_showing_summary() {
            // Only allow updater input during update
            if self.current_tab == Tab::Updater {
                return self.updater.handle_input(key);
            }
            return None;
        }

        // Tab navigation with function keys
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
            KeyCode::F(5) => {
                // F5 is context-sensitive - refresh current tab or switch to Packages
                // Only switch if not already on a tab that uses F5 for refresh
                match self.current_tab {
                    Tab::Services | Tab::PackageBrowser | Tab::Backup | Tab::Network
                    | Tab::Logs | Tab::Kernel | Tab::Cron | Tab::Disks | Tab::SysInfo => {
                        // Let the component handle F5 for refresh
                        return self.delegate_to_component(key);
                    }
                    _ => {
                        self.switch_to_tab(Tab::Packages);
                        return None;
                    }
                }
            }
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

        // Delegate to current component
        self.delegate_to_component(key)
    }

    fn switch_to_tab(&mut self, tab: Tab) {
        let old_tab = self.current_tab;
        self.current_tab = tab;

        // Deactivate old tab
        self.deactivate_tab(old_tab);

        // Activate new tab
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

    /// Process a message
    pub async fn update(&mut self, msg: Message) {
        match msg {
            Message::Quit => {
                self.running = false;
            }
            Message::NextTab => {
                self.switch_to_tab(self.current_tab.next());
            }
            Message::PrevTab => {
                self.switch_to_tab(self.current_tab.prev());
            }

            // System Update
            Message::StartUpdate | Message::ContinueUpdate => {
                self.run_update_step().await;
            }
            Message::UpdateStepComplete(success, error) => {
                self.updater.step_complete(success, error);
                if !self.updater.needs_lilo_confirm() {
                    self.run_update_step().await;
                }
            }
            Message::UpdateOutput(line) => {
                self.updater.add_output(line);
            }

            // sbotools
            Message::StartSbotoolsInstall => {
                self.run_sbotools_step().await;
            }
            Message::SbotoolsStepComplete(success, error) => {
                self.sbotools.step_complete(success, error);
                self.run_sbotools_step().await;
            }
            Message::SbotoolsOutput(line) => {
                self.sbotools.add_output(line);
            }

            // User Setup
            Message::CreateUser => {
                self.create_user().await;
            }
            Message::UserCreated(result) => {
                match result {
                    Ok(msg) => self.user_setup.set_success(msg),
                    Err(e) => self.user_setup.set_error(e),
                }
            }

            // Mirror
            Message::SetMirror(url) => {
                self.set_mirror(&url).await;
            }
            Message::MirrorSet(result) => {
                match result {
                    Ok(()) => {
                        self.mirror.set_status("Mirror updated successfully. Running slackpkg update...".to_string(), false);
                    }
                    Err(e) => {
                        self.mirror.set_status(format!("Error: {}", e), true);
                    }
                }
            }

            // Package Search
            Message::SearchPackages(query) => {
                self.search_packages(&query).await;
            }
            Message::SearchResults(results) => {
                self.package_search.set_results(results);
            }
            Message::InstallPackage(name) => {
                self.install_package(&name).await;
            }
            Message::PackageInstalled(result) => {
                match result {
                    Ok(()) => {
                        self.package_search.set_status("Package installed successfully".to_string(), false);
                    }
                    Err(e) => {
                        self.package_search.set_status(format!("Error: {}", e), true);
                    }
                }
            }

            Message::ProgressUpdate(line) => {
                // Route to appropriate component based on current tab
                match self.current_tab {
                    Tab::Updater => self.updater.add_output(line),
                    Tab::Sbotools => self.sbotools.add_output(line),
                    _ => {}
                }
            }
        }
    }

    /// Run the next update step
    async fn run_update_step(&mut self) {
        let current_step = self.updater.current_step;
        let command = self.updater.get_current_command().map(|(cmd, args)| {
            (cmd.to_string(), args.iter().map(|s| s.to_string()).collect::<Vec<_>>())
        });

        if let Some((cmd, args)) = command {
            self.updater.add_output(format!("Running: {} {}", cmd, args.join(" ")));

            let args_ref: Vec<&str> = args.iter().map(|s| s.as_str()).collect();
            let result = self.executor.execute(&cmd, &args_ref).await;

            if !result.stdout.is_empty() {
                for line in result.stdout.lines().take(10) {
                    self.updater.add_output(line.to_string());
                }
            }

            // Check for kernel updates after upgrade-all step (step 2)
            if current_step == 2 && result.success {
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

            // Continue to next step if not waiting for lilo confirmation
            if !self.updater.needs_lilo_confirm() && self.updater.get_current_command().is_some() {
                Box::pin(self.run_update_step()).await;
            }
        }
    }

    /// Run the next sbotools installation step
    async fn run_sbotools_step(&mut self) {
        use crate::components::sbotools::SbotoolsCommand;

        if let Some(cmd) = self.sbotools.get_current_command() {
            let result = match cmd {
                SbotoolsCommand::Download { url, filename } => {
                    self.sbotools.add_output(format!("Downloading {}...", filename));
                    let output_path = format!("/tmp/{}", filename);
                    self.executor.download_file(&url, &output_path).await
                }
                SbotoolsCommand::InstallPkg { path } => {
                    self.sbotools.add_output(format!("Installing {}...", path));
                    self.executor.installpkg(&path).await
                }
                SbotoolsCommand::SbopkgSync => {
                    self.sbotools.add_output("Syncing sbopkg repository...".to_string());
                    self.executor.sbopkg(&["-r"]).await
                }
                SbotoolsCommand::SbopkgInstall { package } => {
                    self.sbotools.add_output(format!("Installing {}...", package));
                    self.executor.sbopkg(&["-i", &package]).await
                }
                SbotoolsCommand::SboconfigRepo { url } => {
                    self.sbotools.add_output(format!("Configuring repo: {}", url));
                    self.executor.sboconfig(&["-r", &url]).await
                }
                SbotoolsCommand::SbosnapFetch => {
                    self.sbotools.add_output("Fetching SlackBuilds snapshot...".to_string());
                    self.executor.sbosnap(&["fetch"]).await
                }
            };

            if !result.stdout.is_empty() {
                for line in result.stdout.lines().take(5) {
                    self.sbotools.add_output(line.to_string());
                }
            }

            self.sbotools.step_complete(
                result.success,
                if result.success {
                    None
                } else {
                    Some(result.stderr)
                },
            );

            // Continue if more steps
            if self.sbotools.get_current_command().is_some() {
                Box::pin(self.run_sbotools_step()).await;
            }
        }
    }

    /// Create a new user
    async fn create_user(&mut self) {
        let username = self.user_setup.get_username().to_string();
        let password = self.user_setup.get_password().to_string();
        let groups: Vec<String> = self.user_setup.get_selected_groups();
        let groups_ref: Vec<&str> = groups.iter().map(|s| s.as_str()).collect();
        let change_runlevel = self.user_setup.should_change_runlevel();

        // Create user
        let result = self
            .executor
            .useradd(&username, &groups_ref, "/bin/bash")
            .await;

        if !result.success {
            self.user_setup.set_error(format!("Failed to create user: {}", result.stderr));
            return;
        }

        // Set password
        let result = self.executor.set_password(&username, &password).await;

        if !result.success {
            self.user_setup.set_error(format!("Failed to set password: {}", result.stderr));
            return;
        }

        // Change runlevel if requested
        if change_runlevel {
            use crate::slackware::config::SlackwareConfig;
            if let Err(e) = SlackwareConfig::set_default_runlevel(4) {
                self.user_setup.set_error(format!("User created but runlevel change failed: {}", e));
                return;
            }
        }

        self.user_setup.set_success(format!(
            "User '{}' created successfully!{}",
            username,
            if change_runlevel {
                " Runlevel changed to 4."
            } else {
                ""
            }
        ));
    }

    /// Set the active mirror
    async fn set_mirror(&mut self, url: &str) {
        use crate::slackware::config::SlackwareConfig;

        if let Err(e) = SlackwareConfig::set_active_mirror(url) {
            self.mirror.set_status(format!("Failed to set mirror: {}", e), true);
            return;
        }

        // Update GPG key
        self.mirror.set_status("Updating GPG key...".to_string(), false);
        let result = self.executor.slackpkg(&["update", "gpg"]).await;

        if !result.success {
            self.mirror.set_status(format!("GPG update failed: {}", result.stderr), true);
            return;
        }

        // Update package list
        self.mirror.set_status("Updating package list...".to_string(), false);
        let result = self.executor.slackpkg(&["update"]).await;

        if result.success {
            self.mirror.set_status("Mirror updated successfully!".to_string(), false);
            self.mirror.load_mirrors();
        } else {
            self.mirror.set_status(format!("Package list update failed: {}", result.stderr), true);
        }
    }

    /// Search for packages
    async fn search_packages(&mut self, query: &str) {
        use crate::slackware::packages::PackageManager;

        let pm = PackageManager::new();
        let results = pm.search(query).await;
        self.package_search.set_results(results);
    }

    /// Install a package
    async fn install_package(&mut self, name: &str) {
        let result = self.executor.sboinstall(name).await;

        if result.success {
            self.package_search.set_status(format!("Package '{}' installed successfully", name), false);
        } else {
            self.package_search.set_status(format!("Installation failed: {}", result.stderr), true);
        }
    }

    /// Render the UI
    pub fn render(&self, frame: &mut Frame) {
        let layout = AppLayout::new(frame.area());

        // Header
        let header = ratatui::widgets::Paragraph::new(Line::from(vec![
            Span::styled(" Slackware CLI Manager ", Theme::title()),
            Span::styled(
                format!(" - {} ", self.slackware_version.display_name()),
                Theme::muted(),
            ),
        ]))
        .block(Block::default().borders(Borders::BOTTOM));
        frame.render_widget(header, layout.header);

        // Render tabs (two rows for F1-F6 and F7-F12)
        self.render_tabs(frame, layout.tabs);

        // Content - render current component
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

        // Status bar
        let help = self.get_current_help();
        let mut keys = vec![("Alt+←/→", "Tab"), ("Ctrl+Q", "Quit")];
        keys.extend(help);

        let status = StatusBar::new("").keys(keys);
        frame.render_widget(status, layout.status_bar);

        // Exit warning dialog (rendered on top of everything)
        if self.show_exit_warning {
            self.render_exit_warning(frame, frame.area());
        }
    }

    fn render_tabs(&self, frame: &mut Frame, area: Rect) {
        // Split tabs area into two rows
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Length(1), Constraint::Length(1)])
            .split(area);

        // Primary tabs (F1-F6)
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

        let primary = ratatui::widgets::Paragraph::new(Line::from(primary_tabs));
        frame.render_widget(primary, chunks[0]);

        // Secondary tabs (F7-F12) + Ctrl shortcuts
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

        // Add separator and Ctrl shortcuts
        secondary_spans.push(Span::styled(" │ ", Theme::muted()));

        for tab in Tab::additional_tabs() {
            let style = if tab == self.current_tab {
                Theme::tab_active()
            } else {
                Theme::tab_inactive()
            };
            secondary_spans.push(Span::styled(format!(" {} {} ", tab.shortcut(), tab.title()), style));
        }

        let secondary = ratatui::widgets::Paragraph::new(Line::from(secondary_spans));
        frame.render_widget(secondary, chunks[1]);
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

    /// Render exit warning dialog
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
            Line::from(Span::styled(
                "boot after reboot!",
                Theme::warning(),
            )),
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
