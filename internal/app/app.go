// Package app wires the tab components together: input routing, background
// task execution and the shared chrome (header, navigation, status bar).
package app

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/backup"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/configeditor"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/cron"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/disks"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/kernel"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/logs"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/mirror"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/network"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/packagebrowser"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/packagesearch"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/sbotools"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/services"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/settings"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/sysinfo"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/updater"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components/usersetup"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/prefs"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

// App is the main application state.
type App struct {
	Running          bool
	CurrentTab       components.Tab
	SlackwareVersion slackware.Version
	IsRoot           bool

	Updater        *updater.Component
	Sbotools       *sbotools.Component
	UserSetup      *usersetup.Component
	Mirror         *mirror.Component
	PackageSearch  *packagesearch.Component
	ConfigEditor   *configeditor.Component
	SysInfo        *sysinfo.Component
	Services       *services.Component
	PackageBrowser *packagebrowser.Component
	Backup         *backup.Component
	Network        *network.Component
	Logs           *logs.Component
	Kernel         *kernel.Component
	Cron           *cron.Component
	Disks          *disks.Component
	Settings       *settings.Component

	executor slackware.Executor
	events   *queue

	showExitWarning bool
}

// New creates the application.
func New(version slackware.Version, isRoot bool) *App {
	settingsComponent := settings.New()
	return &App{
		Running:          true,
		CurrentTab:       tabFromSetting(settingsComponent.Settings().DefaultTab),
		SlackwareVersion: version,
		IsRoot:           isRoot,

		Updater:        updater.New(version),
		Sbotools:       sbotools.New(),
		UserSetup:      usersetup.New(),
		Mirror:         mirror.New(version),
		PackageSearch:  packagesearch.New(),
		ConfigEditor:   configeditor.New(),
		SysInfo:        sysinfo.New(),
		Services:       services.New(),
		PackageBrowser: packagebrowser.New(),
		Backup:         backup.New(),
		Network:        network.New(),
		Logs:           logs.New(),
		Kernel:         kernel.New(),
		Cron:           cron.New(),
		Disks:          disks.New(),
		Settings:       settingsComponent,

		executor: slackware.NewExecutor(),
		events:   &queue{},
	}
}

func tabFromSetting(setting string) components.Tab {
	switch setting {
	case "sbotools":
		return components.TabSbotools
	case "user_setup":
		return components.TabUserSetup
	case "mirror":
		return components.TabMirror
	case "packages":
		return components.TabPackages
	case "config":
		return components.TabConfig
	default:
		return components.TabUpdater
	}
}

// TryRecv pops the next message produced by a background task.
func (a *App) TryRecv() (msg.Message, bool) { return a.events.tryRecv() }

func (a *App) component(tab components.Tab) components.Component {
	switch tab {
	case components.TabUpdater:
		return a.Updater
	case components.TabSbotools:
		return a.Sbotools
	case components.TabUserSetup:
		return a.UserSetup
	case components.TabMirror:
		return a.Mirror
	case components.TabPackages:
		return a.PackageSearch
	case components.TabConfig:
		return a.ConfigEditor
	case components.TabSysInfo:
		return a.SysInfo
	case components.TabServices:
		return a.Services
	case components.TabPackageBrowser:
		return a.PackageBrowser
	case components.TabBackup:
		return a.Backup
	case components.TabNetwork:
		return a.Network
	case components.TabLogs:
		return a.Logs
	case components.TabKernel:
		return a.Kernel
	case components.TabCron:
		return a.Cron
	case components.TabDisks:
		return a.Disks
	default:
		return a.Settings
	}
}

func renderStream(source slackware.StreamSource, line string) string {
	if source == slackware.Stderr {
		return "stderr: " + line
	}
	return line
}

func (a *App) spawnCommand(target msg.TaskTarget, cmd string, args []string, completion func(slackware.CommandResult) msg.Message) {
	executor, events := a.executor, a.events
	go func() {
		result := executor.ExecuteStreaming(cmd, args, func(source slackware.StreamSource, line string) {
			events.send(msg.TaskProgress{Target: target, Line: renderStream(source, line)})
		})
		events.send(completion(result))
	}()
}

func (a *App) spawnAsync(work func() msg.Message) {
	events := a.events
	go func() { events.send(work()) }()
}

func (a *App) spawnBlocking(work func() (string, error), completion func(msg.Result) msg.Message) {
	events := a.events
	go func() {
		value, err := work()
		if err != nil {
			events.send(completion(msg.Err(err.Error())))
			return
		}
		events.send(completion(msg.Ok(value)))
	}()
}

func (a *App) handleRootRequired(area msg.TaskTarget, message string) {
	switch area {
	case msg.TargetUpdater:
		a.Updater.Reset()
		a.Updater.AddOutput(message)
	case msg.TargetSbotools:
		a.Sbotools.Reset()
		a.Sbotools.AddOutput(message)
	case msg.TargetMirror:
		a.Mirror.SetStatus(message, true)
	case msg.TargetPackages:
		a.PackageSearch.SetStatus(message, true)
	case msg.TargetServices:
		a.Services.SetStatus(message, true)
	case msg.TargetNetwork:
		a.Network.SetStatus(message, true)
	case msg.TargetKernel:
		a.Kernel.SetStatus(message, true)
	case msg.TargetBackup:
		a.Backup.SetStatus(message, true)
	case msg.TargetDisks:
		a.Disks.SetStatus(message, true)
	}
}

// HandleInput handles a key press, returning a message to apply (or nil).
func (a *App) HandleInput(key tui.KeyEvent) msg.Message {
	if a.showExitWarning {
		switch {
		case key.IsChar('q'), key.IsChar('Q'):
			a.showExitWarning = false
			return msg.Quit{}
		case key.IsChar('l'), key.IsChar('L'):
			a.showExitWarning = false
			a.CurrentTab = components.TabUpdater
			a.Updater.ConfirmLilo(true)
			return msg.ContinueUpdate{}
		case key.Code == tui.KeyEsc:
			a.showExitWarning = false
			return nil
		default:
			return nil
		}
	}

	ctrl := key.Has(tui.ModControl)

	if a.CurrentTab == components.TabConfig && a.ConfigEditor.IsEditing() && ctrl &&
		(key.IsChar('s') || key.IsChar('q') || key.IsChar('x')) {
		return a.ConfigEditor.HandleInput(key)
	}

	if ctrl && key.Code == tui.KeyChar {
		switch key.Rune {
		case 'c', 'q':
			if a.Updater.WasLiloSkipped() && a.Updater.WasKernelUpdated() {
				a.showExitWarning = true
				return nil
			}
			return msg.Quit{}
		case 'k':
			a.SwitchToTab(components.TabKernel)
			return nil
		case 'j':
			a.SwitchToTab(components.TabCron)
			return nil
		case 'd':
			a.SwitchToTab(components.TabDisks)
			return nil
		case 's':
			a.SwitchToTab(components.TabSettings)
			return nil
		}
	}

	if a.Updater.IsRunning() || a.Updater.NeedsLiloConfirm() || a.Updater.IsShowingSummary() {
		if a.CurrentTab == components.TabUpdater {
			return a.Updater.HandleInput(key)
		}
		return nil
	}

	switch key.Code {
	case tui.KeyF:
		switch key.F {
		case 1:
			a.SwitchToTab(components.TabUpdater)
			return nil
		case 2:
			a.SwitchToTab(components.TabSbotools)
			return nil
		case 3:
			a.SwitchToTab(components.TabUserSetup)
			return nil
		case 4:
			a.SwitchToTab(components.TabMirror)
			return nil
		case 5:
			switch a.CurrentTab {
			case components.TabServices, components.TabPackageBrowser, components.TabBackup,
				components.TabNetwork, components.TabLogs, components.TabKernel,
				components.TabCron, components.TabDisks, components.TabSysInfo:
				return a.delegateToComponent(key)
			default:
				a.SwitchToTab(components.TabPackages)
				return nil
			}
		case 6:
			a.SwitchToTab(components.TabConfig)
			return nil
		case 7:
			a.SwitchToTab(components.TabSysInfo)
			return nil
		case 8:
			a.SwitchToTab(components.TabServices)
			return nil
		case 9:
			a.SwitchToTab(components.TabPackageBrowser)
			return nil
		case 10:
			a.SwitchToTab(components.TabBackup)
			return nil
		case 11:
			a.SwitchToTab(components.TabNetwork)
			return nil
		case 12:
			a.SwitchToTab(components.TabLogs)
			return nil
		}
	case tui.KeyLeft:
		if key.Has(tui.ModAlt) {
			a.SwitchToTab(a.CurrentTab.Prev())
			return nil
		}
	case tui.KeyRight:
		if key.Has(tui.ModAlt) {
			a.SwitchToTab(a.CurrentTab.Next())
			return nil
		}
	}

	return a.delegateToComponent(key)
}

// SwitchToTab deactivates the current tab and activates tab.
func (a *App) SwitchToTab(tab components.Tab) {
	old := a.CurrentTab
	a.CurrentTab = tab
	a.component(old).OnDeactivate()
	a.activateTab(tab)
}

func (a *App) activateTab(tab components.Tab) {
	switch tab {
	case components.TabUpdater, components.TabSbotools, components.TabUserSetup,
		components.TabPackages, components.TabConfig, components.TabSettings:
		// These tabs keep their state across visits.
	default:
		a.component(tab).OnActivate()
	}
}

func (a *App) delegateToComponent(key tui.KeyEvent) msg.Message {
	return a.component(a.CurrentTab).HandleInput(key)
}

func resultMsg(ok bool, okText, errText string) msg.Result {
	if ok {
		return msg.Ok(okText)
	}
	return msg.Err(errText)
}

func (a *App) applyResult(r msg.Result, setStatus func(string, bool)) {
	if r.Ok {
		setStatus(r.Value, false)
	} else {
		setStatus(r.Err, true)
	}
}

// Update applies a message to the application state.
func (a *App) Update(m msg.Message) {
	switch m := m.(type) {
	case msg.Quit:
		a.Running = false

	case msg.StartUpdate, msg.ContinueUpdate:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetUpdater, "Root privileges are required for system updates.")
		} else {
			a.spawnUpdateStep()
		}
	case msg.UpdateStepFinished:
		result := m.Result
		if result.Stdout != "" && !result.Success && result.Stderr == "" {
			a.Updater.AddOutput(result.Stdout)
		}
		if m.Step == 2 && result.Success {
			a.Updater.SetKernelUpdated(a.Updater.CheckForKernelUpdate(result.Stdout))
		}
		a.Updater.StepComplete(result.Success, result.Stderr)
		if _, _, ok := a.Updater.CurrentCommand(); !a.Updater.NeedsLiloConfirm() && ok {
			a.spawnUpdateStep()
		}

	case msg.StartSbotoolsInstall:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetSbotools, "Root privileges are required for sbotools installation.")
		} else {
			a.spawnSbotoolsStep()
		}
	case msg.SbotoolsStepFinished:
		a.Sbotools.StepComplete(m.Result.Success, m.Result.Stderr)
		if _, ok := a.Sbotools.CurrentCommand(); ok {
			a.spawnSbotoolsStep()
		}

	case msg.CreateUser:
		if !a.IsRoot {
			a.UserSetup.SetError("Root privileges are required to create users.")
		} else {
			a.spawnUserCreation()
		}
	case msg.UserCreated:
		if m.Result.Ok {
			a.UserSetup.SetSuccess(m.Result.Value)
		} else {
			a.UserSetup.SetError(m.Result.Err)
		}

	case msg.SetMirror:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetMirror, "Root privileges are required to change mirrors.")
		} else {
			a.spawnSetMirror(m.URL)
		}
	case msg.MirrorSet:
		if m.Result.Ok {
			a.Mirror.SetStatus("Mirror updated successfully", false)
			a.Mirror.LoadMirrors()
		} else {
			a.Mirror.SetStatus("Error: "+m.Result.Err, true)
		}

	case msg.SearchPackages:
		a.spawnSearch(m.Query)
	case msg.SearchResults:
		if m.Ok {
			a.PackageSearch.SetResults(m.Packages)
		} else {
			a.PackageSearch.SetStatus(m.Err, true)
		}
	case msg.InstallPackage:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetPackages, "Root privileges are required to install packages.")
		} else {
			a.spawnInstallPackage(m.Name)
		}
	case msg.PackageInstalled:
		a.applyResult(m.Result, a.PackageSearch.SetStatus)
	case msg.RemoveInstalledPackage:
		if !a.IsRoot {
			a.PackageBrowser.SetStatus("Root privileges are required to remove packages.", true)
		} else {
			name := m.Name
			a.PackageBrowser.SetStatus(fmt.Sprintf("Removing %s...", name), false)
			a.spawnCommand(msg.TargetPackages, "removepkg", []string{name}, func(r slackware.CommandResult) msg.Message {
				return msg.PackageRemoved{Result: resultMsg(r.Success,
					fmt.Sprintf("Package '%s' removed successfully", name),
					"Failed to remove package: "+r.Stderr)}
			})
		}
	case msg.PackageRemoved:
		if m.Result.Ok {
			a.PackageBrowser.RefreshPackages()
		}
		a.applyResult(m.Result, a.PackageBrowser.SetStatus)

	case msg.SaveConfig:
		if !a.IsRoot {
			a.ConfigEditor.SetStatus("Root privileges are required to save system configuration files.", true)
		} else {
			path, content := m.Path, m.Content
			a.spawnBlocking(func() (string, error) {
				if err := utils.AtomicWrite(path, content); err != nil {
					return "", err
				}
				return "Saved " + path, nil
			}, func(r msg.Result) msg.Message { return msg.ConfigSaved{Result: r} })
		}
	case msg.ConfigSaved:
		a.applyResult(m.Result, a.ConfigEditor.SetStatus)

	case msg.SaveSettings:
		s := m.Settings
		a.spawnBlocking(func() (string, error) { return prefs.Save(s) },
			func(r msg.Result) msg.Message { return msg.SettingsSaved{Result: r} })
	case msg.SettingsSaved:
		a.applyResult(m.Result, a.Settings.SetStatus)

	case msg.RestartNetwork:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetNetwork, "Root privileges are required to restart networking.")
		} else {
			a.Network.RestartStarted()
			a.spawnCommand(msg.TargetNetwork, "/etc/rc.d/rc.inet1", []string{"restart"}, func(r slackware.CommandResult) msg.Message {
				return msg.NetworkRestarted{Result: resultMsg(r.Success,
					"Network restarted successfully", "Failed to restart network: "+r.Stderr)}
			})
		}
	case msg.NetworkRestarted:
		a.Network.Refresh()
		a.applyResult(m.Result, a.Network.SetStatus)

	case msg.ServiceActionMsg:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetServices, "Root privileges are required to modify services.")
		} else {
			a.Services.ActionStarted(m.Action)
			a.spawnServiceAction(m.Action)
		}
	case msg.ServiceActionComplete:
		a.Services.LoadServices()
		a.applyResult(m.Result, a.Services.SetStatus)

	case msg.KernelActionMsg:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetKernel, "Root privileges are required to modify kernel settings.")
		} else {
			a.spawnKernelAction(m.Action)
		}
	case msg.KernelActionComplete:
		a.Kernel.Refresh()
		a.applyResult(m.Result, a.Kernel.SetStatus)

	case msg.BackupActionMsg:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetBackup, "Root privileges are required to manage system backups.")
		} else {
			files := append([]backup.ConfigFile(nil), a.Backup.ConfigFiles()...)
			action := m.Action
			a.Backup.SetStatus("Running backup operation...", false)
			a.spawnBlocking(func() (string, error) { return backup.Execute(action, files) },
				func(r msg.Result) msg.Message { return msg.BackupActionComplete{Result: r} })
		}
	case msg.BackupActionComplete:
		a.Backup.RefreshBackups()
		a.applyResult(m.Result, a.Backup.SetStatus)

	case msg.DiskActionMsg:
		if !a.IsRoot {
			a.handleRootRequired(msg.TargetDisks, "Root privileges are required to mount or unmount disks.")
		} else {
			a.Disks.ActionStarted(m.Action)
			a.spawnDiskAction(m.Action)
		}
	case msg.DiskActionComplete:
		a.Disks.RefreshDisks()
		a.applyResult(m.Result, a.Disks.SetStatus)

	case msg.TaskProgress:
		switch m.Target {
		case msg.TargetUpdater:
			a.Updater.AddOutput(m.Line)
		case msg.TargetSbotools:
			a.Sbotools.AddOutput(m.Line)
		case msg.TargetMirror:
			a.Mirror.SetStatus(m.Line, false)
		case msg.TargetPackages:
			a.PackageSearch.SetStatus(m.Line, false)
		case msg.TargetServices:
			a.Services.SetStatus(m.Line, false)
		case msg.TargetNetwork:
			a.Network.SetStatus(m.Line, false)
		case msg.TargetKernel:
			a.Kernel.SetStatus(m.Line, false)
		case msg.TargetBackup:
			a.Backup.SetStatus(m.Line, false)
		case msg.TargetDisks:
			a.Disks.SetStatus(m.Line, false)
		}
	}
}

func (a *App) spawnUpdateStep() {
	step := a.Updater.CurrentStep()
	cmd, args, ok := a.Updater.CurrentCommand()
	if !ok {
		return
	}
	a.Updater.AddOutput(fmt.Sprintf("Running: %s %s", cmd, strings.Join(args, " ")))
	a.spawnCommand(msg.TargetUpdater, cmd, args, func(r slackware.CommandResult) msg.Message {
		return msg.UpdateStepFinished{Step: step, Result: r}
	})
}

func (a *App) spawnSbotoolsStep() {
	command, ok := a.Sbotools.CurrentCommand()
	if !ok {
		return
	}
	finished := func(r slackware.CommandResult) msg.Message { return msg.SbotoolsStepFinished{Result: r} }
	switch command.Kind {
	case sbotools.CmdDownload:
		a.Sbotools.AddOutput(fmt.Sprintf("Downloading %s...", command.Filename))
		a.spawnCommand(msg.TargetSbotools, "wget", []string{"-O", "/tmp/" + command.Filename, command.URL}, finished)
	case sbotools.CmdInstallPkg:
		a.Sbotools.AddOutput(fmt.Sprintf("Installing %s...", command.Path))
		a.spawnCommand(msg.TargetSbotools, "installpkg", []string{command.Path}, finished)
	case sbotools.CmdSbopkgSync:
		a.Sbotools.AddOutput("Syncing sbopkg repository...")
		a.spawnCommand(msg.TargetSbotools, "sbopkg", []string{"-r"}, finished)
	case sbotools.CmdSbopkgInstall:
		a.Sbotools.AddOutput(fmt.Sprintf("Installing %s...", command.Package))
		a.spawnCommand(msg.TargetSbotools, "sbopkg", []string{"-i", command.Package}, finished)
	case sbotools.CmdSboconfigRepo:
		a.Sbotools.AddOutput(fmt.Sprintf("Configuring repo: %s", command.URL))
		a.spawnCommand(msg.TargetSbotools, "sboconfig", []string{"-r", command.URL}, finished)
	case sbotools.CmdSbosnapFetch:
		a.Sbotools.AddOutput("Fetching SlackBuilds snapshot...")
		a.spawnCommand(msg.TargetSbotools, "sbosnap", []string{"fetch"}, finished)
	}
}

func (a *App) spawnUserCreation() {
	executor, events := a.executor, a.events
	username := a.UserSetup.Username()
	password := a.UserSetup.Password()
	groups := a.UserSetup.SelectedGroups()
	changeRunlevel := a.UserSetup.ShouldChangeRunlevel()

	go func() {
		if r := executor.Useradd(username, groups, "/bin/bash"); !r.Success {
			events.send(msg.UserCreated{Result: msg.Err("Failed to create user: " + r.Stderr)})
			return
		}
		if r := executor.SetPassword(username, password); !r.Success {
			events.send(msg.UserCreated{Result: msg.Err("Failed to set password: " + r.Stderr)})
			return
		}
		if changeRunlevel {
			if err := slackware.SetDefaultRunlevel(4); err != nil {
				events.send(msg.UserCreated{Result: msg.Err("User created but runlevel change failed: " + err.Error())})
				return
			}
		}
		suffix := ""
		if changeRunlevel {
			suffix = " Runlevel changed to 4."
		}
		events.send(msg.UserCreated{Result: msg.Ok(fmt.Sprintf("User '%s' created successfully!%s", username, suffix))})
	}()
}

func (a *App) spawnSetMirror(url string) {
	a.Mirror.SetStatus("Updating mirror configuration...", false)
	executor, events := a.executor, a.events
	progress := func(source slackware.StreamSource, line string) {
		events.send(msg.TaskProgress{Target: msg.TargetMirror, Line: renderStream(source, line)})
	}

	go func() {
		if err := slackware.SetActiveMirror(url); err != nil {
			events.send(msg.MirrorSet{Result: msg.Err(err.Error())})
			return
		}

		events.send(msg.TaskProgress{Target: msg.TargetMirror, Line: "Updating GPG key..."})
		if r := executor.ExecuteStreaming("slackpkg", []string{"update", "gpg"}, progress); !r.Success {
			events.send(msg.MirrorSet{Result: msg.Err("GPG update failed: " + r.Stderr)})
			return
		}

		events.send(msg.TaskProgress{Target: msg.TargetMirror, Line: "Updating package list..."})
		if r := executor.ExecuteStreaming("slackpkg", []string{"update"}, progress); r.Success {
			events.send(msg.MirrorSet{Result: msg.Ok("")})
		} else {
			events.send(msg.MirrorSet{Result: msg.Err("Package list update failed: " + r.Stderr)})
		}
	}()
}

func (a *App) spawnSearch(query string) {
	a.spawnAsync(func() msg.Message {
		packages, errText, ok := slackware.NewPackageManager().Search(query)
		return msg.SearchResults{Packages: packages, Err: errText, Ok: ok}
	})
}

func (a *App) spawnInstallPackage(name string) {
	a.PackageSearch.SetStatus(fmt.Sprintf("Installing package '%s'...", name), false)
	a.spawnCommand(msg.TargetPackages, "sboinstall", []string{"-j", name}, func(r slackware.CommandResult) msg.Message {
		return msg.PackageInstalled{Result: resultMsg(r.Success,
			fmt.Sprintf("Package '%s' installed successfully", name), "Installation failed: "+r.Stderr)}
	})
}

func (a *App) spawnServiceAction(action msg.ServiceAction) {
	name := action.Name
	complete := func(r msg.Result) msg.Message { return msg.ServiceActionComplete{Result: r} }
	run := func(verb, okText, errPrefix string) {
		a.spawnCommand(msg.TargetServices, "/etc/rc.d/"+name, []string{verb}, func(r slackware.CommandResult) msg.Message {
			return complete(resultMsg(r.Success, okText, errPrefix+r.Stderr))
		})
	}

	switch action.Kind {
	case msg.ServiceToggle:
		a.spawnBlocking(func() (string, error) {
			path := "/etc/rc.d/" + name
			info, err := os.Stat(path)
			if err != nil {
				return "", utils.IOError(err)
			}
			mode := info.Mode().Perm()
			if mode&0o111 != 0 {
				mode &^= 0o111
			} else {
				mode |= 0o755
			}
			if err := os.Chmod(path, mode|(info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky))); err != nil {
				return "", utils.IOError(err)
			}
			return fmt.Sprintf("Toggled %s executable bit", name), nil
		}, complete)
	case msg.ServiceStart:
		run("start", fmt.Sprintf("Service %s started successfully", name), "Failed to start service: ")
	case msg.ServiceStop:
		run("stop", fmt.Sprintf("Service %s stopped successfully", name), "Failed to stop service: ")
	case msg.ServiceRestart:
		run("restart", fmt.Sprintf("Service %s restarted successfully", name), "Failed to restart service: ")
	}
}

func (a *App) spawnKernelAction(action msg.KernelAction) {
	complete := func(r msg.Result) msg.Message { return msg.KernelActionComplete{Result: r} }
	switch action.Kind {
	case msg.KernelRunLilo:
		a.spawnCommand(msg.TargetKernel, "lilo", nil, func(r slackware.CommandResult) msg.Message {
			return complete(resultMsg(r.Success, "LILO updated successfully", "LILO failed: "+r.Stderr))
		})
	case msg.KernelSetDefault:
		version := action.Version
		bootloader := a.Kernel.Bootloader()
		a.spawnBlocking(func() (string, error) {
			switch bootloader {
			case kernel.BootloaderLilo:
				content, err := kernel.BuildLiloDefaultConfig(version)
				if err != nil {
					return "", err
				}
				if err := utils.AtomicWrite("/etc/lilo.conf", content); err != nil {
					return "", err
				}
				return fmt.Sprintf("Default kernel set to %s. Run lilo to apply.", version), nil
			case kernel.BootloaderGrub:
				return "", fmt.Errorf("GRUB configuration editing is not yet supported")
			default:
				return "", fmt.Errorf("No known bootloader detected")
			}
		}, complete)
	}
}

func (a *App) spawnDiskAction(action msg.DiskAction) {
	complete := func(r msg.Result) msg.Message { return msg.DiskActionComplete{Result: r} }
	switch action.Kind {
	case msg.DiskMount:
		device := action.Target
		mountPoint := a.Disks.FindMountPoint(device)
		a.spawnCommand(msg.TargetDisks, "mount", []string{device, mountPoint}, func(r slackware.CommandResult) msg.Message {
			return complete(resultMsg(r.Success, fmt.Sprintf("Mounted %s at %s", device, mountPoint), "Mount failed: "+r.Stderr))
		})
	case msg.DiskUnmount:
		mountPoint := action.Target
		a.spawnCommand(msg.TargetDisks, "umount", []string{mountPoint}, func(r slackware.CommandResult) msg.Message {
			return complete(resultMsg(r.Success, "Unmounted "+mountPoint, "Unmount failed: "+r.Stderr))
		})
	case msg.DiskCheckFilesystem:
		device := action.Target
		a.spawnBlocking(func() (string, error) {
			return "", fmt.Errorf("Filesystem check for %s requires an unmounted partition. Run fsck manually.", device)
		}, complete)
	}
}

// Render draws the whole UI.
func (a *App) Render(f *tui.Frame) {
	area := f.Area()
	if area.Width < 80 || area.Height < 24 {
		f.RenderWidget(tui.NewParagraph(tui.TextStr("Terminal too small. Resize to at least 80x24. Ctrl+Q quits.")).Wrap(true), area)
		return
	}
	f.RenderWidget(tui.NewBlock().Style(ui.AppStyle()), area)

	keys := []msg.KeyHelp{{Key: "Alt+←/→", Desc: "Tab"}, {Key: "Ctrl+Q", Desc: "Quit"}}
	keys = append(keys, a.component(a.CurrentTab).HelpText()...)
	if !a.IsRoot {
		keys = append(keys, msg.KeyHelp{Key: "Read-only", Desc: "Mutating actions disabled"})
	}
	controlsWidth := 12
	for _, k := range keys {
		controlsWidth += utf8.RuneCountInString(k.Key) + utf8.RuneCountInString(k.Desc) + 8
	}
	statusHeight := min((controlsWidth+area.Width-1)/area.Width+1, 4)
	layout := ui.NewAppLayout(area, statusHeight)
	headerChunks := tui.Split(layout.Header, tui.Horizontal, tui.Min(32), tui.Length(36))

	header := tui.NewParagraph(tui.TextLines(
		tui.LineFrom(tui.Styled("SLACKWARE", ui.HeroStyle()), tui.Styled(" CLI MANAGER", ui.TitleStyle())),
		tui.LineFrom(tui.Styled("System track ", ui.LabelStyle()), tui.Styled(a.SlackwareVersion.DisplayName(), ui.SubtitleStyle())),
		tui.LineFrom(tui.Styled("Layout ", ui.LabelStyle()), tui.Styled("responsive terminal panels", ui.AccentStyle())),
	)).Block(ui.Panel(ui.PanelTitle("Command Deck")))
	f.RenderWidget(header, headerChunks[0])

	modeBadge := tui.Styled(" READ ONLY ", ui.BadgeWarning())
	if a.IsRoot {
		modeBadge = tui.Styled(" ROOT ENABLED ", ui.BadgeSuccess())
	}
	summary := tui.NewParagraph(tui.TextLines(
		tui.LineFrom(tui.Styled("Current tab ", ui.LabelStyle()), tui.Styled(a.CurrentTab.Title(), ui.TitleStyle())),
		tui.LineFrom(modeBadge, tui.Raw(" "), tui.Styled(fmt.Sprintf("%d tabs online", len(components.AllTabs())), ui.BadgeNeutral())),
		tui.LineFrom(tui.Styled("Navigation ", ui.LabelStyle()), tui.Styled("F1–F12 / Ctrl / Alt", ui.SubtitleStyle())),
	)).Block(ui.PanelAltBlock(ui.PanelTitle("Runtime")))
	f.RenderWidget(summary, headerChunks[1])

	a.renderTabs(f, layout.Tabs)

	a.component(a.CurrentTab).Render(f, layout.Content)

	accessMode := "read-only"
	if a.IsRoot {
		accessMode = "root"
	}
	statusMessage := fmt.Sprintf("%s tab active on %s track in %s mode",
		a.CurrentTab.Title(), a.SlackwareVersion.DisplayName(), accessMode)
	f.RenderWidget(ui.NewStatusBar(statusMessage).Keys(keys), layout.StatusBar)

	if a.showExitWarning {
		a.renderExitWarning(f, area)
	}
}

func (a *App) tabSpans(tabs []components.Tab) []tui.Span {
	spans := make([]tui.Span, 0, len(tabs))
	for _, tab := range tabs {
		style := ui.TabInactive()
		if tab == a.CurrentTab {
			style = ui.TabActive()
		}
		spans = append(spans, tui.Styled(fmt.Sprintf(" %s %s ", tab.Shortcut(), tab.Title()), style))
	}
	return spans
}

func (a *App) renderTabs(f *tui.Frame, area tui.Rect) {
	block := ui.Panel(ui.PanelTitle("Navigation"))
	inner := block.Inner(area)
	f.RenderWidget(block, area)

	chunks := tui.Split(inner, tui.Vertical, tui.Length(1), tui.Length(1), tui.Length(1))
	lanes := [][]components.Tab{components.PrimaryTabs(), components.SecondaryTabs(), components.AdditionalTabs()}
	for i, lane := range lanes {
		f.RenderWidget(tui.NewParagraph(tui.TextLines(tui.LineFrom(a.tabSpans(lane)...))).Style(ui.SurfaceStyle()), chunks[i])
	}
}

func (a *App) renderExitWarning(f *tui.Frame, area tui.Rect) {
	dialogArea := ui.CenteredRect(55, 45, area)
	f.RenderWidget(tui.Clear{}, dialogArea)

	dialog := ui.Panel(ui.PanelTitle("Bootloader warning")).BorderStyle(ui.ErrorStyle())
	inner := dialog.Inner(dialogArea)
	f.RenderWidget(dialog, dialogArea)

	text := tui.NewParagraph(tui.TextLines(
		tui.LineStr(""),
		tui.LineSpan(tui.Styled("You skipped the bootloader update after", ui.WarningStyle())),
		tui.LineSpan(tui.Styled("a kernel update. Your system may not", ui.WarningStyle())),
		tui.LineSpan(tui.Styled("boot after reboot!", ui.WarningStyle())),
		tui.LineStr(""),
		tui.LineStr(""),
		tui.LineFrom(tui.Styled("[Q]", ui.KeyHint()), tui.Raw(" Quit anyway")),
		tui.LineFrom(tui.Styled("[L]", ui.KeyHint()), tui.Raw(" Run lilo now")),
		tui.LineFrom(tui.Styled("[Esc]", ui.KeyHint()), tui.Raw(" Cancel")),
	)).Style(ui.DefaultStyle())
	f.RenderWidget(text, inner)
}
