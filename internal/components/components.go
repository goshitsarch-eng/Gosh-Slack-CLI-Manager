// Package components defines the tab model and the interface every tab's
// component implements. Each tab lives in its own subpackage.
package components

import (
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// Component is a tab's UI component.
type Component interface {
	// HandleInput handles a key press, optionally returning a message for
	// the app (nil means none).
	HandleInput(key tui.KeyEvent) msg.Message
	// Render draws the component. It must not change component state.
	Render(f *tui.Frame, area tui.Rect)
	// HelpText returns key hints for the status bar.
	HelpText() []msg.KeyHelp
	// OnActivate is called when the tab becomes active.
	OnActivate()
	// OnDeactivate is called when the tab becomes inactive.
	OnDeactivate()
}

// Tab identifies a tab.
type Tab uint8

const (
	// Primary tabs (F1-F6)
	TabUpdater Tab = iota
	TabSbotools
	TabUserSetup
	TabMirror
	TabPackages
	TabConfig
	// Secondary tabs (F7-F12)
	TabSysInfo
	TabServices
	TabPackageBrowser
	TabBackup
	TabNetwork
	TabLogs
	// Additional tabs (Ctrl shortcuts)
	TabKernel
	TabCron
	TabDisks
	TabSettings

	tabCount
)

// AllTabs returns every tab in navigation order.
func AllTabs() []Tab {
	tabs := make([]Tab, 0, tabCount)
	for t := TabUpdater; t < tabCount; t++ {
		tabs = append(tabs, t)
	}
	return tabs
}

// PrimaryTabs returns the F1-F6 tabs.
func PrimaryTabs() []Tab {
	return []Tab{TabUpdater, TabSbotools, TabUserSetup, TabMirror, TabPackages, TabConfig}
}

// SecondaryTabs returns the F7-F12 tabs.
func SecondaryTabs() []Tab {
	return []Tab{TabSysInfo, TabServices, TabPackageBrowser, TabBackup, TabNetwork, TabLogs}
}

// AdditionalTabs returns the Ctrl-shortcut tabs.
func AdditionalTabs() []Tab {
	return []Tab{TabKernel, TabCron, TabDisks, TabSettings}
}

var tabTitles = [...]string{
	"Update", "sbotools", "Users", "Mirrors", "Search", "Config",
	"SysInfo", "Services", "Packages", "Backup", "Network", "Logs",
	"Kernel", "Cron", "Disks", "Settings",
}

var tabShortcuts = [...]string{
	"F1", "F2", "F3", "F4", "F5", "F6",
	"F7", "F8", "F9", "F10", "F11", "F12",
	"^K", "^J", "^D", "^S",
}

// Title returns the tab's display title.
func (t Tab) Title() string { return tabTitles[t] }

// Shortcut returns the tab's key shortcut label.
func (t Tab) Shortcut() string { return tabShortcuts[t] }

// Next returns the following tab, wrapping around.
func (t Tab) Next() Tab { return (t + 1) % tabCount }

// Prev returns the preceding tab, wrapping around.
func (t Tab) Prev() Tab { return (t + tabCount - 1) % tabCount }
