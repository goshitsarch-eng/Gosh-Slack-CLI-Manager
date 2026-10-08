// Package prefs holds the persisted application settings model.
package prefs

// ThemeChoice is the selected color theme.
type ThemeChoice uint8

const (
	ThemeDefault ThemeChoice = iota
	ThemeDark
	ThemeLight
	ThemeSolarized
	ThemeNord
	ThemeDracula
)

// AppSettings are the user's saved preferences.
type AppSettings struct {
	Theme           ThemeChoice
	ConfirmActions  bool
	ShowHiddenFiles bool
	AutoRefresh     bool
	RefreshInterval uint32
	DefaultTab      string
	LogLines        int
}

// Default returns the default settings.
func Default() AppSettings {
	return AppSettings{
		Theme:           ThemeDefault,
		ConfirmActions:  true,
		ShowHiddenFiles: false,
		AutoRefresh:     false,
		RefreshInterval: 5,
		DefaultTab:      "updater",
		LogLines:        1000,
	}
}

// Save writes settings to the config path. STUB: implemented with the
// settings component port.
func Save(s AppSettings) (string, error) { return "", nil }
