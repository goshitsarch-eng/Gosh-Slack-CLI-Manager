package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func TestFormatSize(t *testing.T) {
	cases := map[uint64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1280: "1.2 KB", 1536: "1.5 KB", 1048576: "1.0 MB", 11010048: "10.5 MB"}
	for in, want := range cases {
		if got := formatSize(in); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParseBackupStampChronoSemantics(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"20250101_120000", "2025-01-01 12:00:00", true},
		{"2025011_120000", "2025-01-01 12:00:00", true}, // chrono accepts short fields
		{" 20250102_ 1 2 3", "2025-01-02 01:02:03", true},
		{"20250630_235960", "2025-06-30 23:59:60", true}, // leap second
		{"20251301_000000", "", false},
		{"20250230_000000", "", false},
		{"20250101_120000x", "", false},
		{"20250101120000", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		st, ok := parseBackupStamp(c.in)
		if ok != c.ok {
			t.Errorf("parseBackupStamp(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && st.format() != c.want {
			t.Errorf("parseBackupStamp(%q) = %q, want %q", c.in, st.format(), c.want)
		}
	}
	a, _ := parseBackupStamp("20250630_235959")
	b, _ := parseBackupStamp("20250630_235960")
	if !a.less(b) || b.less(a) {
		t.Error("leap second must sort after :59")
	}
}

func TestTrimStartMatches(t *testing.T) {
	if got := trimStartMatches(".._etc", "_"); got != ".._etc" {
		t.Errorf("got %q", got)
	}
	if got := trimStartMatches("__etc_fstab", "_"); got != "etc_fstab" {
		t.Errorf("got %q", got)
	}
}

func TestCopyFileAndRemoveDirAll(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(dst)
	if info.Mode().Perm() != 0o640 {
		t.Errorf("copied mode %v, want 0640", info.Mode().Perm())
	}
	if err := copyFile(dir, dst); err == nil {
		t.Error("copying a directory must fail")
	}
	if _, err := deleteBackup(filepath.Join(dir, "missing")); err == nil || err.Error() != "No such file or directory (os error 2)" {
		t.Errorf("delete missing: %v", err)
	}
	if _, err := deleteBackup(src); err == nil || err.Error() != "Not a directory (os error 20)" {
		t.Errorf("delete file: %v", err)
	}
	sub := filepath.Join(dir, "snap")
	_ = os.MkdirAll(filepath.Join(sub, "x"), 0o700)
	if got, err := deleteBackup(sub); err != nil || got != "Backup deleted successfully" {
		t.Errorf("delete dir: %q %v", got, err)
	}
	if _, err := restoreBackup(filepath.Join(dir, "missing")); err == nil || err.Error() != "Backup did not contain any restorable files" {
		t.Errorf("restore missing: %v", err)
	}
}

func newTestComponent() *Component {
	files := make([]ConfigFile, 0, len(configFiles))
	for _, f := range configFiles {
		files = append(files, ConfigFile{Path: f[0], Description: f[1], Include: !isSensitive(f[0])})
	}
	return &Component{configFiles: files, backups: []entry{{name: "backup_x", path: "/nonexistent/backup_x", timestamp: stamp{t: time.Now()}}}}
}

func TestDefaultsExcludeSensitiveFiles(t *testing.T) {
	c := newTestComponent()
	for _, f := range c.ConfigFiles() {
		if f.Include == isSensitive(f.Path) {
			t.Errorf("%s include=%v", f.Path, f.Include)
		}
	}
}

func TestInputFlow(t *testing.T) {
	c := newTestComponent()
	key := func(code tui.KeyCode) tui.KeyEvent { return tui.NewKey(code, tui.ModNone) }
	ch := func(r rune) tui.KeyEvent { return tui.CharKey(r, tui.ModNone) }

	c.HandleInput(key(tui.KeyUp))
	if sel, ok := c.listState.Selected(); !ok || sel != 0 {
		t.Fatal("Up without selection selects 0")
	}
	c.HandleInput(ch(' '))
	if c.configFiles[0].Include {
		t.Fatal("space toggles")
	}
	c.HandleInput(ch('a'))
	for _, f := range c.configFiles {
		if !f.Include {
			t.Fatal("a selects all when not all selected")
		}
	}
	c.HandleInput(ch('a'))
	for _, f := range c.configFiles {
		if f.Include {
			t.Fatal("a deselects all when all selected")
		}
	}
	c.HandleInput(key(tui.KeyEnter))
	if m := c.HandleInput(ch('y')); m != (msg.BackupActionMsg{Action: msg.BackupAction{Kind: msg.BackupCreate}}) {
		t.Fatalf("confirm create: %#v", m)
	}
	c.HandleInput(key(tui.KeyTab))
	c.HandleInput(ch('d'))
	if !c.showConfirm {
		t.Fatal("d asks for confirmation in restore mode")
	}
	if m := c.HandleInput(key(tui.KeyEsc)); m != nil || c.showConfirm || c.pendingAction != nil {
		t.Fatal("Esc cancels")
	}
	c.HandleInput(key(tui.KeyEnter))
	if m := c.HandleInput(ch('Y')); m != (msg.BackupActionMsg{Action: msg.BackupAction{Kind: msg.BackupRestore, Path: "/nonexistent/backup_x"}}) {
		t.Fatalf("confirm restore: %#v", m)
	}
}
