// Package prefs holds the persisted application settings: the settings
// model, the theme choices, where the settings file lives and how it is
// loaded and saved (TOML, byte-compatible with the original tool's files).
package prefs

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

const (
	configDir     = "/etc/slackware-cli-manager"
	userConfigDir = ".config/slackware-cli-manager"
	configFile    = "config.toml"
)

// isRoot is swapped out by tests.
var isRoot = utils.IsRoot

// ThemeChoice is the selected color theme.
type ThemeChoice uint8

// Theme choices, in cycling order.
const (
	ThemeDefault ThemeChoice = iota
	ThemeDark
	ThemeLight
	ThemeSolarized
	ThemeNord
	ThemeDracula
)

var themeNames = [...]string{"Default", "Dark", "Light", "Solarized", "Nord", "Dracula"}

// AllThemes returns every theme in cycling order.
func AllThemes() []ThemeChoice {
	return []ThemeChoice{ThemeDefault, ThemeDark, ThemeLight, ThemeSolarized, ThemeNord, ThemeDracula}
}

// Name returns the display (and serialized) name of the theme.
func (t ThemeChoice) Name() string { return themeNames[t] }

// ThemeColors is a theme's palette.
type ThemeColors struct {
	Primary    tui.Color
	Secondary  tui.Color
	Success    tui.Color
	Error      tui.Color
	Warning    tui.Color
	Muted      tui.Color
	Background tui.Color
	Foreground tui.Color
}

// Colors returns the theme's palette.
func (t ThemeChoice) Colors() ThemeColors {
	switch t {
	case ThemeDark:
		return ThemeColors{
			Primary:    tui.Blue,
			Secondary:  tui.Magenta,
			Success:    tui.Green,
			Error:      tui.Red,
			Warning:    tui.Yellow,
			Muted:      tui.DarkGray,
			Background: tui.RGB(30, 30, 30),
			Foreground: tui.White,
		}
	case ThemeLight:
		return ThemeColors{
			Primary:    tui.Blue,
			Secondary:  tui.Magenta,
			Success:    tui.Green,
			Error:      tui.Red,
			Warning:    tui.RGB(200, 150, 0),
			Muted:      tui.Gray,
			Background: tui.White,
			Foreground: tui.Black,
		}
	case ThemeSolarized:
		return ThemeColors{
			Primary:    tui.RGB(38, 139, 210),  // Blue
			Secondary:  tui.RGB(211, 54, 130),  // Magenta
			Success:    tui.RGB(133, 153, 0),   // Green
			Error:      tui.RGB(220, 50, 47),   // Red
			Warning:    tui.RGB(181, 137, 0),   // Yellow
			Muted:      tui.RGB(88, 110, 117),  // Base01
			Background: tui.RGB(0, 43, 54),     // Base03
			Foreground: tui.RGB(131, 148, 150), // Base0
		}
	case ThemeNord:
		return ThemeColors{
			Primary:    tui.RGB(136, 192, 208), // Nord8
			Secondary:  tui.RGB(180, 142, 173), // Nord15
			Success:    tui.RGB(163, 190, 140), // Nord14
			Error:      tui.RGB(191, 97, 106),  // Nord11
			Warning:    tui.RGB(235, 203, 139), // Nord13
			Muted:      tui.RGB(76, 86, 106),   // Nord3
			Background: tui.RGB(46, 52, 64),    // Nord0
			Foreground: tui.RGB(236, 239, 244), // Nord6
		}
	case ThemeDracula:
		return ThemeColors{
			Primary:    tui.RGB(139, 233, 253), // Cyan
			Secondary:  tui.RGB(255, 121, 198), // Pink
			Success:    tui.RGB(80, 250, 123),  // Green
			Error:      tui.RGB(255, 85, 85),   // Red
			Warning:    tui.RGB(241, 250, 140), // Yellow
			Muted:      tui.RGB(98, 114, 164),  // Comment
			Background: tui.RGB(40, 42, 54),    // Background
			Foreground: tui.RGB(248, 248, 242), // Foreground
		}
	default:
		return ThemeColors{
			Primary:    tui.Cyan,
			Secondary:  tui.Yellow,
			Success:    tui.Green,
			Error:      tui.Red,
			Warning:    tui.Yellow,
			Muted:      tui.DarkGray,
			Background: tui.Reset,
			Foreground: tui.White,
		}
	}
}

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

// joinPath appends a relative component the way Rust's PathBuf::join does:
// no cleaning, a separator only when base doesn't already end with one, and
// an absolute component replacing the base.
func joinPath(base, part string) string {
	switch {
	case strings.HasPrefix(part, "/"), base == "":
		return part
	case strings.HasSuffix(base, "/"):
		return base + part
	default:
		return base + "/" + part
	}
}

// ConfigPath is the settings file location: the system directory for root,
// else $XDG_CONFIG_HOME (when absolute), else ~/.config, else the working
// directory.
func ConfigPath() string {
	if isRoot() {
		return joinPath(configDir, configFile)
	}
	if configHome, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok && strings.HasPrefix(configHome, "/") {
		return joinPath(joinPath(configHome, "slackware-cli-manager"), configFile)
	}
	if home, ok := os.LookupEnv("HOME"); ok && utf8.ValidString(home) {
		return joinPath(joinPath(home, userConfigDir), configFile)
	}
	return configFile
}

// Load reads the saved settings. A missing file yields the defaults with no
// message; an unreadable or invalid file yields the defaults and an error
// message for the status line.
func Load() (settings AppSettings, errMessage string) {
	path := ConfigPath()
	content, err := utils.ReadFileString(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), ""
		}
		return Default(), "Unable to load settings at " + path + ": " + utils.IOErrorString(err) + ". Using defaults."
	}
	settings, err = decodeSettings(content)
	if err != nil {
		return Default(), "Invalid settings at " + path + ": " + err.Error() + ". Using defaults."
	}
	return settings, ""
}

// Save writes settings to the config path, creating its directory, and
// returns the status message.
func Save(s AppSettings) (string, error) {
	path := ConfigPath()
	parent, ok := parentPath(path)
	if !ok {
		parent = "."
	}
	if err := createDirAll(parent); err != nil {
		return "", errors.New(utils.IOErrorString(err))
	}
	if err := utils.AtomicWrite(path, encodeSettings(s)); err != nil {
		return "", err
	}
	return "Settings saved to " + path, nil
}

// parentPath mirrors Rust's Path::parent for plain paths: the path without
// its last component ("" for a bare relative name, false for the root).
func parentPath(path string) (string, bool) {
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" {
		return "", false // "" and "/" have no parent
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "", true
	}
	parent := strings.TrimRight(trimmed[:idx], "/")
	if parent == "" {
		return "/", true
	}
	return parent, true
}

// createDirAll mirrors Rust's fs::create_dir_all (including which error is
// reported when a path component is in the way).
func createDirAll(path string) error {
	if path == "" {
		return nil
	}
	err := os.Mkdir(path, 0o777)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		if isDir(path) {
			return nil
		}
		return err
	}
	parent, ok := parentPath(path)
	if !ok {
		return errors.New("failed to create whole tree")
	}
	if err := createDirAll(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o777); err != nil {
		if isDir(path) {
			return nil
		}
		return err
	}
	return nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
