package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

// testdata/toml_cases.json was generated with the original implementation
// (toml 0.8.23 + serde): for every document, either the decoded settings or
// the exact error text, and for every settings value the exact
// to_string_pretty output.
type tomlCases struct {
	Decode []struct {
		Doc string            `json:"doc"`
		Ok  []json.RawMessage `json:"ok"`
		Err *string           `json:"err"`
	} `json:"decode"`
	Encode []struct {
		Settings []json.RawMessage `json:"settings"`
		TOML     string            `json:"toml"`
	} `json:"encode"`
}

func loadCases(t *testing.T) tomlCases {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "toml_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c tomlCases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func settingsFromJSON(t *testing.T, raw []json.RawMessage) AppSettings {
	t.Helper()
	var s AppSettings
	var theme uint8
	var interval uint32
	var lines int
	for i, dst := range []any{&theme, &s.ConfirmActions, &s.ShowHiddenFiles, &s.AutoRefresh, &interval, &s.DefaultTab, &lines} {
		if err := json.Unmarshal(raw[i], dst); err != nil {
			t.Fatal(err)
		}
	}
	s.Theme, s.RefreshInterval, s.LogLines = ThemeChoice(theme), interval, lines
	return s
}

func TestDecodeMatchesOriginal(t *testing.T) {
	cases := loadCases(t)
	if len(cases.Decode) < 100 {
		t.Fatalf("expected many decode cases, got %d", len(cases.Decode))
	}
	for i, c := range cases.Decode {
		got, err := decodeSettings(c.Doc)
		switch {
		case c.Err != nil:
			if err == nil {
				t.Errorf("case %d %q: expected error %q, got %+v", i, c.Doc, *c.Err, got)
			} else if err.Error() != *c.Err {
				t.Errorf("case %d %q:\nwant %q\ngot  %q", i, c.Doc, *c.Err, err.Error())
			}
		case err != nil:
			t.Errorf("case %d %q: unexpected error %q", i, c.Doc, err.Error())
		default:
			if want := settingsFromJSON(t, c.Ok); got != want {
				t.Errorf("case %d %q: want %+v, got %+v", i, c.Doc, want, got)
			}
		}
	}
}

func TestEncodeMatchesOriginal(t *testing.T) {
	cases := loadCases(t)
	for i, c := range cases.Encode {
		s := settingsFromJSON(t, c.Settings)
		if got := encodeSettings(s); got != c.TOML {
			t.Errorf("case %d %+v:\nwant %q\ngot  %q", i, s, c.TOML, got)
		}
	}
}

func TestDefaultSettingsEncoding(t *testing.T) {
	want := "theme = \"Default\"\nconfirm_actions = true\nshow_hidden_files = false\nauto_refresh = false\nrefresh_interval = 5\ndefault_tab = \"updater\"\nlog_lines = 1000\n"
	if got := encodeSettings(Default()); got != want {
		t.Fatalf("got %q", got)
	}
	back, err := decodeSettings(want)
	if err != nil || back != Default() {
		t.Fatalf("round trip: %+v, %v", back, err)
	}
}

func withConfigEnv(t *testing.T, root bool, xdg, home string) {
	t.Helper()
	old := isRoot
	isRoot = func() bool { return root }
	t.Cleanup(func() { isRoot = old })
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", home)
}

func TestConfigPathResolution(t *testing.T) {
	withConfigEnv(t, true, "/xdg", "/home/u")
	if got := ConfigPath(); got != "/etc/slackware-cli-manager/config.toml" {
		t.Errorf("root: %q", got)
	}

	withConfigEnv(t, false, "/xdg/", "/home/u")
	if got := ConfigPath(); got != "/xdg/slackware-cli-manager/config.toml" {
		t.Errorf("xdg: %q", got)
	}

	withConfigEnv(t, false, "relative/xdg", "/home/u")
	if got := ConfigPath(); got != "/home/u/.config/slackware-cli-manager/config.toml" {
		t.Errorf("relative xdg ignored: %q", got)
	}

	withConfigEnv(t, false, "", "/home/u/")
	if got := ConfigPath(); got != "/home/u/.config/slackware-cli-manager/config.toml" {
		t.Errorf("home: %q", got)
	}

	withConfigEnv(t, false, "", "")
	os.Unsetenv("XDG_CONFIG_HOME")
	os.Unsetenv("HOME")
	if got := ConfigPath(); got != "config.toml" {
		t.Errorf("fallback: %q", got)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	withConfigEnv(t, false, dir, "/nonexistent")
	path := filepath.Join(dir, "slackware-cli-manager", "config.toml")

	// Missing file: defaults, no message.
	if s, msg := Load(); s != Default() || msg != "" {
		t.Fatalf("missing file: %+v %q", s, msg)
	}

	s := Default()
	s.Theme = ThemeNord
	s.AutoRefresh = true
	s.RefreshInterval = 12
	s.DefaultTab = "it's \"quoted\""
	s.LogLines = 2500
	msg, err := Save(s)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "Settings saved to "+path {
		t.Fatalf("message %q", msg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != encodeSettings(s) {
		t.Fatalf("file content %q", data)
	}
	if got, msg := Load(); got != s || msg != "" {
		t.Fatalf("reload: %+v %q", got, msg)
	}
}

func TestLoadReportsInvalidAndUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	withConfigEnv(t, false, dir, "/nonexistent")
	cfgDir := filepath.Join(dir, "slackware-cli-manager")
	path := filepath.Join(cfgDir, "config.toml")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("theme = \"Dark\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, msg := Load()
	want := "Invalid settings at " + path + ": TOML parse error at line 1, column 1\n  |\n1 | theme = \"Dark\"\n  | ^^^^^^^^^^^^^^\nmissing field `confirm_actions`\n. Using defaults."
	if s != Default() || msg != want {
		t.Fatalf("invalid: %+v\n%q\n%q", s, msg, want)
	}

	if err := os.WriteFile(path, []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, msg := Load(); msg != "Unable to load settings at "+path+": stream did not contain valid UTF-8. Using defaults." {
		t.Fatalf("utf8: %q", msg)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, msg := Load(); msg != "Unable to load settings at "+path+": Is a directory (os error 21). Using defaults." {
		t.Fatalf("dir: %q", msg)
	}
}

func TestSaveReportsDirectoryErrors(t *testing.T) {
	dir := t.TempDir()
	// A regular file where the config directory should be.
	blocker := filepath.Join(dir, "slackware-cli-manager")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	withConfigEnv(t, false, dir, "/nonexistent")
	if _, err := Save(Default()); err == nil || err.Error() != "File exists (os error 17)" {
		t.Fatalf("got %v", err)
	}
}

func TestThemeChoices(t *testing.T) {
	var names []string
	for _, th := range AllThemes() {
		names = append(names, th.Name())
	}
	if got := strings.Join(names, ","); got != "Default,Dark,Light,Solarized,Nord,Dracula" {
		t.Fatal(got)
	}
	if c := ThemeNord.Colors(); c.Primary != tui.RGB(136, 192, 208) || c.Background != tui.RGB(46, 52, 64) {
		t.Fatalf("nord colors %+v", c)
	}
}
